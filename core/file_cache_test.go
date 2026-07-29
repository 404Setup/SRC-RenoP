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
	// Defensive copy: mutating result must not change cache.
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
	// Oversized value is ignored.
	_ = c.Set("big", make([]byte, 200))
	if _, err := c.Get("big"); err != ErrFileCacheMiss {
		t.Fatalf("expected oversized entry to be skipped")
	}
}

func TestFileByteCacheSameSizeSetReusesBuffer(t *testing.T) {
	c := NewFileByteCache(64 << 10)
	v1 := bytes.Repeat([]byte("a"), 1024)
	v2 := bytes.Repeat([]byte("b"), 1024)
	_ = c.Set("k", v1)
	// Same length update should keep a single live entry of that size.
	_ = c.Set("k", v2)
	got, err := c.Get("k")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, v2) {
		t.Fatalf("expected updated value")
	}
	n, used := c.Stats()
	if n != 1 {
		t.Fatalf("entries=%d want 1", n)
	}
	if used != 1024 {
		t.Fatalf("used=%d want 1024", used)
	}
}

func TestFileByteCacheConcurrentGetSet(t *testing.T) {
	c := NewFileByteCache(4 << 20)
	val := bytes.Repeat([]byte{1}, 512)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
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
