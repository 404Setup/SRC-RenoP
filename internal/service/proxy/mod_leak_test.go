/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
)

func testAppState() *core.AppState {
	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()
	state.Inner.ProxyClientSemaphore = make(chan struct{}, 8)
	return state
}

func largeCLHandler(status, size int, written *atomic.Int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(size))
		w.WriteHeader(status)
		chunk := bytes.Repeat([]byte("M"), 64*1024)
		left := size
		for left > 0 {
			n := min(len(chunk), left)
			nw, err := w.Write(chunk[:n])
			if written != nil {
				written.Add(int64(nw))
			}
			if err != nil {
				return
			}
			if left == size {
				time.Sleep(20 * time.Millisecond)
			}
			left -= n
		}
	})
}

func TestProxyArtifactNonOKLargeBodyDoesNotSpikeHeap(t *testing.T) {
	const size = 40 << 20
	var written atomic.Int64
	ts := httptest.NewServer(largeCLHandler(http.StatusNotFound, size, &written))
	t.Cleanup(ts.Close)

	state := testAppState()
	repo := &config.Repository{
		Name: "leak-repo",
		Mirrors: []config.Mirror{
			{URL: ts.URL, TimeoutSecs: 5},
		},
	}
	storage := t.TempDir()
	path := "com/example/big.jar"
	pathStr := filepath.ToSlash(filepath.Join(storage, repo.Name, path))
	dl, _ := state.Inner.InFlightDownloads.LockPath(pathStr)

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	_, err := ProxyArtifact(state, repo, path, storage, pathStr, dl)
	if err == nil {
		t.Fatal("expected not found / error from proxy")
	}

	time.Sleep(100 * time.Millisecond)

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	const maxGrowth = 8 << 20
	var growth uint64
	if after.HeapAlloc > before.HeapAlloc {
		growth = after.HeapAlloc - before.HeapAlloc
	}
	if growth > maxGrowth {
		t.Fatalf("HeapAlloc grew by %d after non-OK large mirror body (limit %d)", growth, maxGrowth)
	}
	if got := written.Load(); got > 8<<20 {
		t.Fatalf("server wrote %d bytes of 404 body; client should have aborted early", got)
	}
}

func TestProxyArtifactOKLargeThenAbortDoesNotSpikeHeap(t *testing.T) {
	const size = 40 << 20
	ts := httptest.NewServer(largeCLHandler(http.StatusOK, size, nil))
	t.Cleanup(ts.Close)

	state := testAppState()
	repo := &config.Repository{
		Name: "leak-repo-ok",
		Mirrors: []config.Mirror{
			{URL: ts.URL, TimeoutSecs: 5},
		},
	}
	storage := t.TempDir()
	path := "com/example/huge.jar"
	pathStr := filepath.ToSlash(filepath.Join(storage, repo.Name, path))
	dl, _ := state.Inner.InFlightDownloads.LockPath(pathStr)

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	stream, err := ProxyArtifact(state, repo, path, storage, pathStr, dl)
	if err != nil {
		t.Fatalf("ProxyArtifact: %v", err)
	}
	buf := make([]byte, 4096)
	_, _ = stream.Read(buf)
	_ = stream.Close()

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	const maxGrowth = 8 << 20
	var growth uint64
	if after.HeapAlloc > before.HeapAlloc {
		growth = after.HeapAlloc - before.HeapAlloc
	}
	if growth > maxGrowth {
		t.Fatalf("HeapAlloc grew by %d after aborting large OK stream (limit %d)", growth, maxGrowth)
	}
}

func TestProxyArtifactSmallOK(t *testing.T) {
	payload := []byte(`{"ok":true}`)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(ts.Close)

	state := testAppState()
	repo := &config.Repository{
		Name: "small-repo",
		Mirrors: []config.Mirror{
			{URL: ts.URL, TimeoutSecs: 5},
		},
	}
	storage := t.TempDir()
	path := "com/example/meta.xml"
	pathStr := filepath.ToSlash(filepath.Join(storage, repo.Name, path))
	dl, _ := state.Inner.InFlightDownloads.LockPath(pathStr)

	rc, err := ProxyArtifact(state, repo, path, storage, pathStr, dl)
	if err != nil {
		t.Fatalf("ProxyArtifact: %v", err)
	}
	data, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("got %q", data)
	}
}

func TestProxyArtifactSmallSequentialMemory(t *testing.T) {
	payload := []byte(`<metadata><version>1.0.0</version></metadata>`)
	var hits atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(ts.Close)

	state := testAppState()
	repo := &config.Repository{
		Name: "small-mem-repo",
		Mirrors: []config.Mirror{
			{URL: ts.URL, TimeoutSecs: 5},
		},
	}
	storage := t.TempDir()

	{
		path := "warm/meta.xml"
		pathStr := filepath.ToSlash(filepath.Join(storage, repo.Name, path))
		dl, _ := state.Inner.InFlightDownloads.LockPath(pathStr)
		rc, err := ProxyArtifact(state, repo, path, storage, pathStr, dl)
		if err != nil {
			t.Fatalf("warm: %v", err)
		}
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}

	runtime.GC()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	const n = 20
	for i := range n {
		path := "com/example/m" + strconv.Itoa(i) + "/maven-metadata.xml"
		pathStr := filepath.ToSlash(filepath.Join(storage, repo.Name, path))
		dl, _ := state.Inner.InFlightDownloads.LockPath(pathStr)
		rc, err := ProxyArtifact(state, repo, path, storage, pathStr, dl)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, payload) {
			t.Fatalf("iter %d: bad body", i)
		}
	}

	runtime.GC()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	var growth uint64
	if after.HeapAlloc > before.HeapAlloc {
		growth = after.HeapAlloc - before.HeapAlloc
	}
	const maxGrowth = 1 << 20
	if growth > maxGrowth {
		t.Fatalf("HeapAlloc grew by %d after %d small proxy GETs (limit %d); hits=%d",
			growth, n, maxGrowth, hits.Load())
	}
}
