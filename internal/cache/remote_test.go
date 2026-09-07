/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cache

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRemoteCacheEncryptionAndRevocation(t *testing.T) {
	address := os.Getenv("RENOP_TEST_CACHE_ADDRESS")
	if address == "" {
		t.Skip("RENOP_TEST_CACHE_ADDRESS is not set")
	}
	mode := os.Getenv("RENOP_TEST_CACHE_MODE")
	if mode == "" {
		mode = "redis"
	}
	r, err := Open(Config{Mode: mode, Address: address, Password: os.Getenv("RENOP_TEST_CACHE_PASSWORD")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	ctx := context.Background()
	first, err := r.Put([]byte("secret-a"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Delete()
	sealed, err := r.client.Get(ctx, first.key).Bytes()
	if err != nil || bytes.Contains(sealed, []byte("secret-a")) {
		t.Fatal("plaintext cache value was exposed", err)
	}
	value, err := first.Read()
	if err != nil || string(value) != "secret-a" {
		t.Fatal("encrypted round trip failed", err)
	}
	second, err := r.Put([]byte("secret-b"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Delete()
	replacement, err := r.client.Get(ctx, second.key).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.client.Set(ctx, first.key, replacement, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Read(); err == nil {
		t.Fatal("ciphertext substitution was accepted")
	}
	if _, err := r.Put(make([]byte, maxValueBytes+1), time.Minute); err == nil {
		t.Fatal("oversized cache value accepted")
	}
	expired, err := r.Put([]byte("expired"), time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := expired.Read(); err == nil {
		t.Fatal("expired value returned")
	}

	type credential struct {
		User   string
		Secret string
	}
	var index IndexedMap[string, credential]
	index.Bind(r, func(value credential) credential { return credential{User: value.User} })
	index.Store("credential", credential{User: "alice", Secret: "private"})
	index.Range(func(_ string, value credential) bool {
		if value.User != "alice" || value.Secret != "" {
			t.Fatal("invalidation index retained sensitive payload")
		}
		return true
	})
	entry, _ := index.entries.Load("credential")
	options := r.client.Options()
	if err := r.client.Close(); err != nil {
		t.Fatal(err)
	}
	index.Delete("credential")
	r.client = redis.NewClient(options)
	r.retryAt.Store(0)
	if _, ok := index.Load("credential"); ok {
		t.Fatal("revoked credential returned after reconnect")
	}
	if _, err := entry.Blob.Read(); err != nil {
		t.Fatal("outage scenario did not leave its old remote value available", err)
	}
	entry.Blob.Delete()
	index.Store("alice-one", credential{User: "alice", Secret: "one"})
	index.Store("alice-two", credential{User: "alice", Secret: "two"})
	index.Store("bob", credential{User: "bob", Secret: "kept"})
	if count := index.DeleteMatching(func(_ string, value credential) bool { return value.User == "alice" }, 0); count != 2 {
		t.Fatal("incorrect batched invalidation count", count)
	}
	if _, ok := index.Load("alice-one"); ok {
		t.Fatal("batch left a revoked reference")
	}
	if value, ok := index.Load("bob"); !ok || value.Secret != "kept" {
		t.Fatal("batch removed another account")
	}
	index.Delete("bob")
	r.sequence.Store(1 << 32)
	if _, err := r.Put([]byte("limit"), time.Minute); err == nil {
		t.Fatal("GCM per-key message limit was exceeded")
	}
}

func TestCacheDefaultsAndBounds(t *testing.T) {
	var cfg Config
	cfg.Normalize()
	if cfg.Mode != "memory" || cfg.Address != "localhost:6379" || cfg.TimeoutMS != 1000 {
		t.Fatal(cfg)
	}
	if remote, err := Open(cfg); err != nil || remote != nil {
		t.Fatal("default backend is not memory", err)
	}
	for _, bad := range []Config{
		{Mode: "unsupported"}, {Mode: "redis", Address: "https://example.com"},
		{Mode: "valkey", Database: -1}, {TimeoutMS: -1}, {TimeoutMS: 10001},
	} {
		if bad.Validate() == nil {
			t.Fatal("invalid cache settings accepted")
		}
	}
}
