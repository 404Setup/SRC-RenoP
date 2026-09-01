/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"fmt"
	"hash/maphash"
	"sync"
	"sync/atomic"
	syncv2 "sync/v2"
	"time"
)

const (
	numShards              = 32
	defaultMaxCacheEntries = 8192
)

type cacheItem[V any] struct {
	value     V
	expiredAt int64
}

type cacheShard[K comparable, V any] struct {
	mu       sync.RWMutex
	items    map[K]cacheItem[V]
	inFlight map[K]*cacheLoad[V]
	_        [128]byte
}

type cacheLoad[V any] struct {
	done  chan struct{}
	value V
	err   error
}

// CacheStats tracks lookup outcomes for one TTL cache.
type CacheStats struct {
	Hits   atomic.Int64
	Misses atomic.Int64
}

// TTLCache is a bounded sharded cache with lazy expiry and miss coalescing.
type TTLCache[K comparable, V any] struct {
	shards             [numShards]*cacheShard[K, V]
	stats              CacheStats
	defaultTTL         time.Duration
	maxEntriesPerShard int
	hashSeed           maphash.Seed
	generation         atomic.Uint64
	evictPool          syncv2.Pool[*[]K]
}

// NewTTLCache creates a cache with the default bounded capacity.
func NewTTLCache[K comparable, V any](defaultTTL time.Duration) *TTLCache[K, V] {
	return NewTTLCacheWithCapacity[K, V](defaultTTL, defaultMaxCacheEntries)
}

// NewTTLCacheWithCapacity creates a sharded cache with a bounded approximate total capacity.
func NewTTLCacheWithCapacity[K comparable, V any](defaultTTL time.Duration, maxEntries int) *TTLCache[K, V] {
	if maxEntries <= 0 {
		maxEntries = defaultMaxCacheEntries
	}
	cache := &TTLCache[K, V]{
		defaultTTL:         defaultTTL,
		maxEntriesPerShard: max((maxEntries+numShards-1)/numShards, 1),
		hashSeed:           maphash.MakeSeed(),
	}
	cache.evictPool = syncv2.Pool[*[]K]{
		New: func() *[]K {
			s := make([]K, 0, 32)
			return &s
		},
	}
	for i := range numShards {
		cache.shards[i] = &cacheShard[K, V]{
			items: make(map[K]cacheItem[V]),
		}
	}

	return cache
}

func (c *TTLCache[K, V]) getShard(key K) *cacheShard[K, V] {
	idx := maphash.Comparable(c.hashSeed, key) % numShards
	return c.shards[idx]
}

// Stats returns cumulative hit and miss counts.
func (c *TTLCache[K, V]) Stats() (hits, misses int64) {
	return c.stats.Hits.Load(), c.stats.Misses.Load()
}

// Len returns the number of currently retained entries, including entries awaiting lazy expiry.
func (c *TTLCache[K, V]) Len() int {
	total := 0
	for _, shard := range c.shards {
		shard.mu.RLock()
		total += len(shard.items)
		shard.mu.RUnlock()
	}
	return total
}

// Get returns one unexpired value.
func (c *TTLCache[K, V]) Get(key K) (V, bool) {
	shard := c.getShard(key)
	shard.mu.RLock()
	item, ok := shard.items[key]
	shard.mu.RUnlock()

	if !ok {
		c.stats.Misses.Add(1)
		var zero V
		return zero, false
	}

	now := time.Now().UnixMilli()
	if now > item.expiredAt {
		shard.mu.Lock()
		if cur, stillOk := shard.items[key]; stillOk && now > cur.expiredAt {
			delete(shard.items, key)
		}
		shard.mu.Unlock()
		c.stats.Misses.Add(1)
		var zero V
		return zero, false
	}

	c.stats.Hits.Add(1)
	return item.value, true
}

// Set stores a value using ttl or the cache default when ttl is non-positive.
func (c *TTLCache[K, V]) Set(key K, val V, ttl time.Duration) {
	if ttl <= 0 {
		ttl = c.defaultTTL
	}

	shard := c.getShard(key)
	now := time.Now()
	exp := now.Add(ttl).UnixMilli()
	nowMs := now.UnixMilli()

	shard.mu.Lock()
	defer shard.mu.Unlock()

	c.setLocked(shard, key, val, exp, nowMs)
}

// Generation returns the current invalidation generation for a read-through load.
func (c *TTLCache[K, V]) Generation() uint64 {
	return c.generation.Load()
}

// SetIfGeneration stores a loaded value only when no invalidation occurred since generation was read.
func (c *TTLCache[K, V]) SetIfGeneration(key K, val V, ttl time.Duration, generation uint64) bool {
	if ttl <= 0 {
		ttl = c.defaultTTL
	}
	shard := c.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if c.generation.Load() != generation {
		return false
	}
	now := time.Now()
	c.setLocked(shard, key, val, now.Add(ttl).UnixMilli(), now.UnixMilli())
	return true
}

func (c *TTLCache[K, V]) setLocked(shard *cacheShard[K, V], key K, val V, expiredAt, now int64) {
	if _, exists := shard.items[key]; !exists && len(shard.items) >= c.maxEntriesPerShard {
		for candidate, item := range shard.items {
			if now > item.expiredAt {
				delete(shard.items, candidate)
			}
		}
		if len(shard.items) >= c.maxEntriesPerShard {
			var oldestKey K
			oldestExpiry := int64(^uint64(0) >> 1)
			found := false
			for candidate, item := range shard.items {
				if !found || item.expiredAt < oldestExpiry {
					oldestKey = candidate
					oldestExpiry = item.expiredAt
					found = true
				}
			}
			if found {
				delete(shard.items, oldestKey)
			}
		}
	}
	shard.items[key] = cacheItem[V]{value: val, expiredAt: expiredAt}
}

// GetOrLoad returns a cached value or coalesces concurrent misses for the same key.
// The loader controls positive and negative entry lifetimes by returning a TTL.
func (c *TTLCache[K, V]) GetOrLoad(key K, loader func() (V, time.Duration, error)) (V, error) {
	if value, ok := c.Get(key); ok {
		return value, nil
	}
	shard := c.getShard(key)
	shard.mu.Lock()
	now := time.Now().UnixMilli()
	if item, ok := shard.items[key]; ok && now <= item.expiredAt {
		shard.mu.Unlock()
		return item.value, nil
	} else if ok {
		delete(shard.items, key)
	}
	if pending := shard.inFlight[key]; pending != nil {
		shard.mu.Unlock()
		<-pending.done
		return pending.value, pending.err
	}
	if shard.inFlight == nil {
		shard.inFlight = make(map[K]*cacheLoad[V])
	}
	pending := &cacheLoad[V]{done: make(chan struct{})}
	shard.inFlight[key] = pending
	loadGeneration := c.generation.Load()
	shard.mu.Unlock()

	var value V
	var ttl time.Duration
	var loadErr error
	var panicValue any
	func() {
		defer func() { panicValue = recover() }()
		value, ttl, loadErr = loader()
	}()
	if panicValue != nil {
		loadErr = fmt.Errorf("cache loader panicked: %v", panicValue)
	}

	shard.mu.Lock()
	if loadErr == nil && panicValue == nil && loadGeneration == c.generation.Load() {
		if ttl <= 0 {
			ttl = c.defaultTTL
		}
		now = time.Now().UnixMilli()
		c.setLocked(shard, key, value, time.Now().Add(ttl).UnixMilli(), now)
	}
	pending.value = value
	pending.err = loadErr
	delete(shard.inFlight, key)
	close(pending.done)
	shard.mu.Unlock()
	if panicValue != nil {
		panic(panicValue)
	}
	return value, loadErr
}

// Delete removes one key and prevents an overlapping load from repopulating it.
func (c *TTLCache[K, V]) Delete(key K) {
	c.generation.Add(1)
	shard := c.getShard(key)
	shard.mu.Lock()
	delete(shard.items, key)
	shard.mu.Unlock()
}

// Clear removes all retained values.
func (c *TTLCache[K, V]) Clear() {
	c.generation.Add(1)
	for i := range numShards {
		shard := c.shards[i]
		shard.mu.Lock()
		shard.items = make(map[K]cacheItem[V])
		shard.mu.Unlock()
	}
}

// DeleteFunc removes values selected by predicate.
func (c *TTLCache[K, V]) DeleteFunc(predicate func(key K, val V) bool) {
	c.generation.Add(1)
	for i := range numShards {
		shard := c.shards[i]
		shard.mu.Lock()
		for k, item := range shard.items {
			if predicate(k, item.value) {
				delete(shard.items, k)
			}
		}
		shard.mu.Unlock()
	}
}

// EvictExpired eagerly removes all entries whose TTL elapsed.
func (c *TTLCache[K, V]) EvictExpired() {
	now := time.Now().UnixMilli()
	ptrSlice := c.evictPool.Get()
	toDelete := (*ptrSlice)[:0]

	for i := range numShards {
		shard := c.shards[i]

		shard.mu.RLock()
		for k, item := range shard.items {
			if now > item.expiredAt {
				toDelete = append(toDelete, k)
			}
		}
		shard.mu.RUnlock()

		if len(toDelete) == 0 {
			continue
		}

		shard.mu.Lock()
		for _, k := range toDelete {
			if it, ok := shard.items[k]; ok && now > it.expiredAt {
				delete(shard.items, k)
			}
		}
		if len(shard.items) == 0 {
			shard.items = make(map[K]cacheItem[V])
		}
		shard.mu.Unlock()
		toDelete = toDelete[:0]
	}

	if cap(*ptrSlice) <= 1024 {
		*ptrSlice = toDelete
		c.evictPool.Put(ptrSlice)
	}
}
