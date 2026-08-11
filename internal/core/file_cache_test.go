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
	"bytes"
	"sync"
	"testing"
)

func TestFileByteCacheGetSetDelete(t *testing.T) {
	c := NewFileByteCache(1024)
	if _, err := c.Get("missing"); err != ErrFileCacheMiss {
		t.Fatalf("expected miss, got %v", err)
	}
	if err := c.Set("a", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	got, err := c.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
	got[0] = 'H'
	got2, _ := c.Get("a")
	if string(got2) != "hello" {
		t.Fatalf("cache mutated: %q", got2)
	}
	_ = c.Delete("a")
	if _, err := c.Get("a"); err != ErrFileCacheMiss {
		t.Fatalf("expected miss after delete, got %v", err)
	}
}

func TestFileByteCacheEvictsToMaxBytes(t *testing.T) {
	c := NewFileByteCache(100)
	_ = c.Set("a", make([]byte, 60))
	_ = c.Set("b", make([]byte, 60))
	n, used := c.Stats()
	if used > 100 {
		t.Fatalf("used %d > max 100", used)
	}
	if n == 0 {
		t.Fatal("expected at least one entry retained")
	}
	_ = c.Set("big", make([]byte, 200))
	if _, err := c.Get("big"); err != ErrFileCacheMiss {
		t.Fatalf("expected oversized entry to be skipped")
	}
}

func TestFileByteCacheSameSizeSetKeepsPublishedViewImmutable(t *testing.T) {
	c := NewFileByteCache(64 << 10)
	v1 := bytes.Repeat([]byte("a"), 1024)
	v2 := bytes.Repeat([]byte("b"), 1024)
	_ = c.Set("k", v1)
	oldView, err := c.GetReadOnlyView("k")
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Set("k", v2)
	got, err := c.Get("k")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, v2) {
		t.Fatalf("expected updated value")
	}
	if !bytes.Equal(oldView, v1) {
		t.Fatal("a published read-only view was mutated by Set")
	}
	n, used := c.Stats()
	if n != 1 {
		t.Fatalf("entries=%d want 1", n)
	}
	if used != 1024 {
		t.Fatalf("used=%d want 1024", used)
	}
}

func TestFileByteCacheConcurrentReadOnlyViewAndSet(t *testing.T) {
	c := NewFileByteCache(64 << 10)
	values := [][]byte{
		bytes.Repeat([]byte{1}, 4096),
		bytes.Repeat([]byte{2}, 4096),
	}
	if err := c.Set("shared", values[0]); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 1000 {
			_ = c.Set("shared", values[i&1])
		}
	}()
	go func() {
		defer wg.Done()
		for range 1000 {
			view, err := c.GetReadOnlyView("shared")
			if err != nil || len(view) == 0 {
				continue
			}
			first := view[0]
			for _, b := range view[1:] {
				if b != first {
					t.Errorf("observed a partially overwritten cache entry")
					return
				}
			}
		}
	}()
	wg.Wait()
}

func TestFileByteCacheConcurrentGetSet(t *testing.T) {
	c := NewFileByteCache(4 << 20)
	val := bytes.Repeat([]byte{1}, 512)
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range 200 {
				k := string(rune('a'+id%26)) + string(rune('0'+i%10))
				_ = c.Set(k, val)
				_, _ = c.Get(k)
			}
		}(g)
	}
	wg.Wait()
	n, used := c.Stats()
	if n < 0 || used < 0 {
		t.Fatalf("bad stats n=%d used=%d", n, used)
	}
	if used > 4<<20 {
		t.Fatalf("used %d exceeds max", used)
	}
}

func TestFileByteCacheDeleteBoundsEvictionMetadata(t *testing.T) {
	c := NewFileByteCache(4 << 20)
	value := []byte("metadata")
	keys := make([]string, 10_000)

	for i := range keys {
		key := string(rune(i/256)) + string(rune(i%256))
		keys[i] = key
		if err := c.Set(key, value); err != nil {
			t.Fatal(err)
		}
	}

	var retained [fileCacheShardCount]bool
	for _, key := range keys {
		shard := hashKey(key) & (fileCacheShardCount - 1)
		if !retained[shard] {
			retained[shard] = true
			continue
		}
		if err := c.Delete(key); err != nil {
			t.Fatal(err)
		}
	}

	entries, used := c.Stats()
	if entries != fileCacheShardCount || used != fileCacheShardCount*len(value) {
		t.Fatalf("unexpected cache stats after churn: entries=%d used=%d", entries, used)
	}
	for i := range c.shards {
		if got := len(c.shards[i].order); got > 34 {
			t.Fatalf("shard %d retained %d eviction keys for one entry", i, got)
		}
		if got := cap(c.shards[i].order); got > 64 {
			t.Fatalf("shard %d retained oversized eviction storage: cap=%d", i, got)
		}
	}
}
