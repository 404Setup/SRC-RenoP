/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package tasks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"renop/internal/core"
	"renop/internal/service/index"
)

func TestPruneAuthCache(t *testing.T) {
	state := core.NewAppState()
	now := time.UnixMilli(10_000)
	state.StoreAuthCache("expired", core.AuthCacheEntry{ExpiredAt: now.UnixMilli() - 1})
	state.StoreAuthCache("live", core.AuthCacheEntry{ExpiredAt: now.UnixMilli() + 1})
	if removed := PruneAuthCache(state, now); removed != 1 {
		t.Fatalf("removed entries = %d, want 1", removed)
	}
	if _, ok := state.Inner.AuthCache.Load("expired"); ok {
		t.Fatal("expired auth cache entry remained")
	}
	if _, ok := state.Inner.AuthCache.Load("live"); !ok {
		t.Fatal("live auth cache entry was removed")
	}
	if got := state.Inner.AuthCacheEntries.Load(); got != 1 {
		t.Fatalf("auth cache entry count = %d, want 1", got)
	}
}

func TestCleanExpiredSessions(t *testing.T) {
	state := core.NewAppState()
	now := time.Now()
	expired := &core.Session{}
	expired.LastActive.Store(now.UnixMilli() - core.SessionIdleTimeoutMillis - 1)
	live := &core.Session{}
	live.LastActive.Store(now.UnixMilli())
	state.Inner.Sessions.Store("expired", expired)
	state.Inner.Sessions.Store("live", live)
	state.StoreAuthCache("Session expired", core.AuthCacheEntry{ExpiredAt: now.Add(time.Hour).UnixMilli()})
	if err := CleanExpiredSessions(state, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Inner.Sessions.Load("expired"); ok {
		t.Fatal("expired session remained")
	}
	if _, ok := state.Inner.Sessions.Load("live"); !ok {
		t.Fatal("live session was removed")
	}
	if _, ok := state.Inner.AuthCache.Load("Session expired"); ok {
		t.Fatal("expired session auth cache remained")
	}
}

func TestIndexSaveTaskPersistsDirtyIndex(t *testing.T) {
	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()
	state.Inner.FileIndex.InsertFile("storage/releases/example.jar", index.FileInfo{Size: 42, ModTime: 7})
	path := filepath.Join(t.TempDir(), "index.json")
	save := NewIndexSaveTask(state, path)
	save(context.Background())
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("saved index is empty")
	}
	if state.Inner.FileIndex.IsDirty.Load() {
		t.Fatal("saved index remained dirty")
	}
}

func TestIndexSaveTaskHonorsCanceledContext(t *testing.T) {
	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()
	path := filepath.Join(t.TempDir(), "index.json")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	NewIndexSaveTask(state, path)(ctx)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("canceled task created index file: %v", err)
	}
	NewIndexSaveTask(state, path)(nil)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("nil-context task created index file: %v", err)
	}
}
