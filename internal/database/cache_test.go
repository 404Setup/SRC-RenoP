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
	"testing"
	"time"

	"renop/internal/config"
	"renop/internal/core"
)

func TestTTLCacheCoalescesConcurrentLoads(t *testing.T) {
	cache := NewTTLCacheWithCapacity[string, int](time.Minute, 64)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	loader := func() (int, time.Duration, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return 42, time.Minute, nil
	}
	results := make(chan int, 32)
	errors := make(chan error, 32)
	var workers sync.WaitGroup
	workers.Add(32)
	for range 32 {
		go func() {
			defer workers.Done()
			value, err := cache.GetOrLoad("shared", loader)
			results <- value
			errors <- err
		}()
	}
	<-started
	close(release)
	workers.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("coalesced load failed: %v", err)
		}
	}
	for value := range results {
		if value != 42 {
			t.Fatalf("coalesced value = %d, want 42", value)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", calls.Load())
	}
}

func TestTTLCacheCapacityAndInvalidationDuringLoad(t *testing.T) {
	cache := NewTTLCacheWithCapacity[string, int](time.Minute, 64)
	for index := range 1024 {
		cache.Set(fmt.Sprintf("key-%d", index), index, time.Minute)
	}
	if entries := cache.Len(); entries > 64 {
		t.Fatalf("bounded cache retained %d entries, want at most 64", entries)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	loaded := make(chan int, 1)
	go func() {
		value, _ := cache.GetOrLoad("invalidated", func() (int, time.Duration, error) {
			close(started)
			<-release
			return 7, time.Minute, nil
		})
		loaded <- value
	}()
	<-started
	cache.Delete("invalidated")
	close(release)
	if value := <-loaded; value != 7 {
		t.Fatalf("in-flight value = %d, want 7", value)
	}
	if _, ok := cache.Get("invalidated"); ok {
		t.Fatal("an invalidated in-flight load repopulated the cache")
	}
}

func TestTTLCacheLoaderPanicDoesNotPoisonKey(t *testing.T) {
	cache := NewTTLCacheWithCapacity[string, int](time.Minute, 64)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("cache loader panic was not propagated")
			}
		}()
		_, _ = cache.GetOrLoad("panic", func() (int, time.Duration, error) {
			panic("loader failure")
		})
	}()
	value, err := cache.GetOrLoad("panic", func() (int, time.Duration, error) {
		return 9, time.Minute, nil
	})
	if err != nil || value != 9 {
		t.Fatalf("cache remained blocked after loader panic: value=%d err=%v", value, err)
	}
}

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
		userIDCache:      NewTTLCache[string, string](time.Minute),
		profileCache:     NewTTLCache[string, core.UserProfile](time.Minute),
	}
	db.tokenCache.Set("token", &core.AccessToken{}, time.Minute)
	db.tokenSecretCache.Set("secret", &core.AccessToken{}, time.Minute)
	db.sessionCache.Set("session", &core.Session{}, time.Minute)
	db.userIDCache.Set("user", "id", time.Minute)
	db.profileCache.Set("profile", core.UserProfile{UserID: "id"}, time.Minute)
	expireCacheKey(db.tokenCache, "token")
	expireCacheKey(db.tokenSecretCache, "secret")
	expireCacheKey(db.sessionCache, "session")
	expireCacheKey(db.userIDCache, "user")
	expireCacheKey(db.profileCache, "profile")
	db.EvictExpiredCaches()
	if cacheContains(db.tokenCache, "token") || cacheContains(db.tokenSecretCache, "secret") ||
		cacheContains(db.sessionCache, "session") || cacheContains(db.userIDCache, "user") ||
		cacheContains(db.profileCache, "profile") {
		t.Fatal("database cache eviction left expired entries")
	}
}

func TestUserIdentityAndProfileSummaryCaches(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{
		Driver: "sqlite3", Dsn: "file:user-cache-test?mode=memory&cache=shared", MaxOpenConns: 2, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("initialize cache test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close cache test database: %v", err)
		}
	})
	now := time.Now().UnixMilli()
	if err := db.SaveToken(&core.AccessToken{Name: "alice", CreatedAt: "2026-08-28T00:00:00Z"}); err != nil {
		t.Fatalf("save cache test account: %v", err)
	}
	db.userIDCache = NewTTLCacheWithCapacity[string, string](time.Minute, 64)
	firstID, err := db.userIDForUsername("alice")
	if err != nil {
		t.Fatalf("load uncached user ID: %v", err)
	}
	secondID, err := db.userIDForUsername("alice")
	if err != nil || secondID != firstID {
		t.Fatalf("load cached user ID: id=%q err=%v", secondID, err)
	}
	hits, misses := db.userIDCache.Stats()
	if hits != 1 || misses != 1 {
		t.Fatalf("identity cache stats = %d hits, %d misses; want 1 and 1", hits, misses)
	}

	db.profileCache = NewTTLCacheWithCapacity[string, core.UserProfile](time.Minute, 64)
	for range 2 {
		profiles, err := db.GetUserProfiles([]string{"alice"})
		if err != nil || profiles["alice"] == nil {
			t.Fatalf("load profile summary: profile=%v err=%v", profiles["alice"], err)
		}
	}
	hits, misses = db.profileCache.Stats()
	if hits != 1 || misses != 1 {
		t.Fatalf("profile cache stats = %d hits, %d misses; want 1 and 1", hits, misses)
	}

	if profiles, err := db.GetUserProfiles([]string{"bobby"}); err != nil || len(profiles) != 0 {
		t.Fatalf("prime missing profile cache: profiles=%v err=%v", profiles, err)
	}
	if err := db.CreateToken(&core.AccessToken{Name: "bobby", CreatedAt: "2026-08-28T00:00:00Z"}, "Bobby", now); err != nil {
		t.Fatalf("create account over negative cache: %v", err)
	}
	profiles, err := db.GetUserProfiles([]string{"bobby"})
	if err != nil || profiles["bobby"] == nil || profiles["bobby"].Nickname != "Bobby" {
		t.Fatalf("new account did not replace negative cache: profile=%v err=%v", profiles["bobby"], err)
	}

	account, err := db.GetTokenByName("alice")
	if err != nil {
		t.Fatalf("load account before rename: %v", err)
	}
	if _, err := db.UpdateUserProfile("alice", "alice_renamed", "Alice", account, now+1,
		core.AccountTokenChanges{}); err != nil {
		t.Fatalf("rename account with populated caches: %v", err)
	}
	profiles, err = db.GetUserProfiles([]string{"alice", "alice_renamed"})
	if err != nil || profiles["alice"] != nil || profiles["alice_renamed"] == nil {
		t.Fatalf("renamed profile cache is stale: profiles=%v err=%v", profiles, err)
	}
}

// BenchmarkTTLCacheParallelHit measures the hot read path shared by authentication and identity lookups.
func BenchmarkTTLCacheParallelHit(b *testing.B) {
	cache := NewTTLCacheWithCapacity[string, int](time.Minute, 4096)
	cache.Set("hot", 42, time.Minute)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			value, ok := cache.Get("hot")
			if !ok || value != 42 {
				b.Fatalf("cache hit failed: value=%d ok=%v", value, ok)
			}
		}
	})
}
