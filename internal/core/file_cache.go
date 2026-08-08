/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import (
	"errors"
	"sync"
	"sync/atomic"
)

// ErrFileCacheMiss is returned by FileByteCache.Get when the key is absent.
var ErrFileCacheMiss = errors.New("entry not found in file cache")

// fileCacheShardCount must be a power of two (mask hashing).
const fileCacheShardCount = 16

// FileByteCache is a size-bounded in-memory cache for small artifact metadata.
// It starts empty (no preallocation), shards keys for concurrent access, and
// publishes immutable entry buffers so reads stay allocation- and race-free.
type FileByteCache struct {
	maxBytes int
	used     atomic.Int64
	shards   []fileCacheShard
}

type fileCacheShard struct {
	mu      sync.RWMutex
	entries map[string][]byte

	// order is a FIFO of keys for eviction. May contain stale keys after Delete;
	// those are skipped and compacted lazily.
	order []string
}

// NewFileByteCache creates a cache with a hard cap of maxBytes of stored payload.
// Values larger than maxBytes are not stored. maxBytes <= 0 defaults to 16 MiB.
func NewFileByteCache(maxBytes int) *FileByteCache {
	if maxBytes <= 0 {
		maxBytes = 16 << 20
	}
	shards := make([]fileCacheShard, fileCacheShardCount)
	for i := range shards {
		shards[i] = fileCacheShard{
			entries: make(map[string][]byte),
		}
	}
	return &FileByteCache{
		maxBytes: maxBytes,
		shards:   shards,
	}
}

func (c *FileByteCache) shard(key string) *fileCacheShard {
	return &c.shards[hashKey(key)&(fileCacheShardCount-1)]
}

// FNV-1a 32-bit — cheap, good enough for path keys.
func hashKey(key string) uint32 {
	const (
		offset = 2166136261
		prime  = 16777619
	)
	h := uint32(offset)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= prime
	}
	return h
}

// Get returns a defensive copy of the cached value.
func (c *FileByteCache) Get(key string) ([]byte, error) {
	v, err := c.GetReadOnlyView(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}

// GetReadOnlyView returns a read-only view of the cached value slice without allocation.
// Callers MUST NOT mutate the returned byte slice.
func (c *FileByteCache) GetReadOnlyView(key string) ([]byte, error) {
	if c == nil {
		return nil, ErrFileCacheMiss
	}
	s := c.shard(key)
	s.mu.RLock()
	v, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrFileCacheMiss
	}
	return v, nil
}

// Set stores an immutable copy of value under key, evicting oldest entries until
// under budget. Existing buffers are never mutated because readers may still
// hold a read-only view after the shard lock is released.
func (c *FileByteCache) Set(key string, value []byte) error {
	if c == nil {
		return nil
	}
	if len(value) > c.maxBytes {
		return nil
	}
	data := make([]byte, len(value))
	copy(data, value)

	s := c.shard(key)
	s.mu.Lock()

	delta := int64(len(data))
	if old, ok := s.entries[key]; ok {
		delta -= int64(len(old))
		s.entries[key] = data
	} else {
		s.entries[key] = data
		s.order = append(s.order, key)
	}
	if delta != 0 {
		c.used.Add(delta)
	}
	s.mu.Unlock()

	if c.used.Load() > int64(c.maxBytes) {
		c.trimToMax(key)
	}
	return nil
}

// Delete removes key if present.
func (c *FileByteCache) Delete(key string) error {
	if c == nil {
		return nil
	}
	s := c.shard(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.entries[key]; ok {
		c.used.Add(-int64(len(old)))
		delete(s.entries, key)
	}
	return nil
}

// Stats returns aggregate entry count and payload bytes (for tests/diagnostics).
func (c *FileByteCache) Stats() (entries, usedBytes int) {
	if c == nil {
		return 0, 0
	}
	usedBytes = max(int(c.used.Load()), 0)
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.RLock()
		entries += len(s.entries)
		s.mu.RUnlock()
	}
	return entries, usedBytes
}

// trimToMax drops oldest entries across shards until used fits maxBytes.
// protect is never removed while other entries remain (the entry just written).
// Shards are locked one at a time in index order to avoid deadlock.
func (c *FileByteCache) trimToMax(protect string) {
	for c.used.Load() > int64(c.maxBytes) {
		progress := false
		for i := range c.shards {
			if c.used.Load() <= int64(c.maxBytes) {
				return
			}
			s := &c.shards[i]
			s.mu.Lock()
			if c.evictOneLocked(s, protect) {
				progress = true
			}
			s.compactOrderLocked()
			s.mu.Unlock()
		}
		if !progress {
			return
		}
	}
}

// evictOneLocked removes one non-protect entry from s. Returns true if something was removed.
func (c *FileByteCache) evictOneLocked(s *fileCacheShard, protect string) bool {
	for len(s.order) > 0 {
		k := s.order[0]
		s.order = s.order[1:]
		if k == protect {
			s.order = append(s.order, k)
			if len(s.entries) <= 1 {
				break
			}
			continue
		}
		if v, ok := s.entries[k]; ok {
			c.used.Add(-int64(len(v)))
			delete(s.entries, k)
			return true
		}
	}
	for k, v := range s.entries {
		if k == protect {
			continue
		}
		c.used.Add(-int64(len(v)))
		delete(s.entries, k)
		return true
	}
	return false
}

func (s *fileCacheShard) compactOrderLocked() {
	if len(s.order) <= 2*len(s.entries)+32 {
		return
	}
	n := 0
	for _, k := range s.order {
		if _, ok := s.entries[k]; ok {
			s.order[n] = k
			n++
		}
	}
	s.order = s.order[:n]
}
