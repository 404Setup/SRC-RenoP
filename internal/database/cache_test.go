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
	"testing"
	"time"

	"renop/internal/core"
)

func expireCacheKey[K comparable, V any](cache *TTLCache[K, V], key K) {
	shard := cache.getShard(key)
	shard.mu.Lock()
	item := shard.items[key]
	item.expiredAt = 1
	shard.items[key] = item
	shard.mu.Unlock()
}

func cacheContains[K comparable, V any](cache *TTLCache[K, V], key K) bool {
	shard := cache.getShard(key)
	shard.mu.RLock()
	_, ok := shard.items[key]
	shard.mu.RUnlock()
	return ok
}

func TestTTLCacheEvictExpired(t *testing.T) {
	cache := NewTTLCache[string, int](time.Minute)
	cache.Set("expired", 1, time.Minute)
	cache.Set("live", 2, time.Minute)
	expireCacheKey(cache, "expired")
	cache.EvictExpired()
	if cacheContains(cache, "expired") {
		t.Fatal("expired key remained after eviction")
	}
	if !cacheContains(cache, "live") {
		t.Fatal("live key was removed during eviction")
	}
}

func TestDBEvictExpiredCaches(t *testing.T) {
	db := &DB{
		tokenCache:       NewTTLCache[string, *core.AccessToken](time.Minute),
		tokenSecretCache: NewTTLCache[string, *core.AccessToken](time.Minute),
		sessionCache:     NewTTLCache[string, *core.Session](time.Minute),
	}
	db.tokenCache.Set("token", &core.AccessToken{}, time.Minute)
	db.tokenSecretCache.Set("secret", &core.AccessToken{}, time.Minute)
	db.sessionCache.Set("session", &core.Session{}, time.Minute)
	expireCacheKey(db.tokenCache, "token")
	expireCacheKey(db.tokenSecretCache, "secret")
	expireCacheKey(db.sessionCache, "session")
	db.EvictExpiredCaches()
	if cacheContains(db.tokenCache, "token") || cacheContains(db.tokenSecretCache, "secret") || cacheContains(db.sessionCache, "session") {
		t.Fatal("database cache eviction left expired entries")
	}
}
