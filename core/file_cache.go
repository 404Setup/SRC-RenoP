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
// reuses same-size entry buffers on Set to avoid needless allocations.
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
	if c == nil {
		return nil, ErrFileCacheMiss
	}
	s := c.shard(key)
	s.mu.RLock()
	v, ok := s.entries[key]
	if !ok {
		s.mu.RUnlock()
		return nil, ErrFileCacheMiss
	}
	out := make([]byte, len(v))
	copy(out, v)
	s.mu.RUnlock()
	return out, nil
}

// Set stores a copy of value under key, evicting oldest entries until under budget.
// When the key already holds a buffer of the same length, the buffer is reused.
func (c *FileByteCache) Set(key string, value []byte) error {
	if c == nil {
		return nil
	}
	if len(value) > c.maxBytes {
		return nil
	}

	s := c.shard(key)
	s.mu.Lock()

	delta := int64(len(value))
	if old, ok := s.entries[key]; ok {
		if len(old) == len(value) {
			copy(old, value)
			s.mu.Unlock()
			return nil
		}
		delta -= int64(len(old))
		data := make([]byte, len(value))
		copy(data, value)
		s.entries[key] = data
	} else {
		data := make([]byte, len(value))
		copy(data, value)
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
			if countLiveOrder(s, protect) <= 1 {
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

func countLiveOrder(s *fileCacheShard, protect string) int {
	n := 0
	for _, k := range s.order {
		if _, ok := s.entries[k]; ok {
			n++
			if k != protect && n > 1 {
				return n
			}
		}
	}
	return n
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
