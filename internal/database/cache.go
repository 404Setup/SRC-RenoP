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
	"sync"
	"sync/atomic"
	syncv2 "sync/v2"
	"time"
)

const (
	numShards        = 32
	maxShardCapacity = 5000
)

type cacheItem[V any] struct {
	value     V
	expiredAt int64
}

type cacheShard[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]cacheItem[V]
	_     [128]byte
}

type CacheStats struct {
	Hits   atomic.Int64
	Misses atomic.Int64
}

type TTLCache[K comparable, V any] struct {
	shards     [numShards]*cacheShard[K, V]
	stats      CacheStats
	defaultTTL time.Duration
	stopEvict  chan struct{}
	closeOnce  sync.Once
	evictPool  syncv2.Pool[*[]K]
}

const (
	fnvOffset64 uint64 = 14695981039346656037
	fnvPrime64  uint64 = 1099511628211
)

func fnv1aHash(s string) uint64 {
	h := fnvOffset64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime64
	}
	return h
}

func hashUint64(val uint64) uint64 {
	val = (val ^ (val >> 30)) * 0xbf58476d1ce4e5b9
	val = (val ^ (val >> 27)) * 0x94d049bb133111eb
	return val ^ (val >> 31)
}

func hashKey[K comparable](key K) uint64 {
	switch v := any(key).(type) {
	case string:
		return fnv1aHash(v)
	case fmt.Stringer:
		return fnv1aHash(v.String())
	case int:
		return hashUint64(uint64(v))
	case int64:
		return hashUint64(uint64(v))
	case uint64:
		return hashUint64(v)
	case int32:
		return hashUint64(uint64(v))
	case uint32:
		return hashUint64(uint64(v))
	case uintptr:
		return hashUint64(uint64(v))
	default:
		return fnv1aHash(fmt.Sprintf("%v", v))
	}
}

func NewTTLCache[K comparable, V any](defaultTTL time.Duration) *TTLCache[K, V] {
	cache := &TTLCache[K, V]{
		defaultTTL: defaultTTL,
		stopEvict:  make(chan struct{}),
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

	evictInterval := min(max(defaultTTL/2, 30*time.Second), 2*time.Minute)
	go cache.startEvictionLoop(evictInterval)

	return cache
}

func (c *TTLCache[K, V]) getShard(key K) *cacheShard[K, V] {
	idx := hashKey(key) % numShards
	return c.shards[idx]
}

func (c *TTLCache[K, V]) startEvictionLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.EvictExpired()
		case <-c.stopEvict:
			return
		}
	}
}

func (c *TTLCache[K, V]) Close() {
	c.closeOnce.Do(func() {
		close(c.stopEvict)
	})
}

func (c *TTLCache[K, V]) Stats() (hits, misses int64) {
	return c.stats.Hits.Load(), c.stats.Misses.Load()
}

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

	if len(shard.items) >= maxShardCapacity {
		for k, it := range shard.items {
			if nowMs > it.expiredAt {
				delete(shard.items, k)
			}
		}
		if len(shard.items) >= maxShardCapacity {
			toDelete := maxShardCapacity / 10
			for k := range shard.items {
				delete(shard.items, k)
				toDelete--
				if toDelete <= 0 {
					break
				}
			}
		}
	}

	shard.items[key] = cacheItem[V]{
		value:     val,
		expiredAt: exp,
	}
}

func (c *TTLCache[K, V]) Delete(key K) {
	shard := c.getShard(key)
	shard.mu.Lock()
	delete(shard.items, key)
	shard.mu.Unlock()
}

func (c *TTLCache[K, V]) Clear() {
	for i := range numShards {
		shard := c.shards[i]
		shard.mu.Lock()
		shard.items = make(map[K]cacheItem[V])
		shard.mu.Unlock()
	}
}

func (c *TTLCache[K, V]) DeleteFunc(predicate func(key K, val V) bool) {
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
