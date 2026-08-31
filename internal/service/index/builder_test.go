/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package index

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

type blockingScanSink struct {
	started chan struct{}
	release <-chan struct{}
	seen    atomic.Int32
}

func (sink *blockingScanSink) addFile(string, FileInfo) {
	sink.seen.Add(1)
	sink.started <- struct{}{}
	<-sink.release
}

func (*blockingScanSink) addDir(string) {}

func TestScanLocalDirBoundsTopLevelWorkers(t *testing.T) {
	root := t.TempDir()
	const directoryCount = 32
	for index := range directoryCount {
		directory := filepath.Join(root, "repo-"+strconv.Itoa(index))
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "artifact.txt"), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	release := make(chan struct{})
	sink := &blockingScanSink{started: make(chan struct{}, directoryCount), release: release}
	done := make(chan struct{})
	go func() {
		scanLocalDir(root, sink, true)
		close(done)
	}()
	for range maxLocalScanWorkers {
		<-sink.started
	}
	select {
	case <-sink.started:
		t.Fatalf("more than %d directory scans ran concurrently", maxLocalScanWorkers)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bounded directory scan did not finish")
	}
	if seen := sink.seen.Load(); seen != directoryCount {
		t.Fatalf("scanned files = %d, want %d", seen, directoryCount)
	}
}

func TestReplaceIndexFromScanAddsAndRemoves(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "releases")
	if err := os.MkdirAll(filepath.Join(repo, "com", "example"), 0755); err != nil {
		t.Fatal(err)
	}
	keepPath := filepath.Join(repo, "com", "example", "keep.txt")
	if err := os.WriteFile(keepPath, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}

	idx := NewFileIndex()
	baseSlash := filepath.ToSlash(filepath.Clean(root))
	idx.InsertDir(baseSlash)
	idx.InsertDir(filepath.ToSlash(repo))
	idx.InsertFile(filepath.ToSlash(keepPath), FileInfo{Size: 4, ModTime: 1})
	stale := filepath.ToSlash(filepath.Join(repo, "com", "example", "gone.txt"))
	idx.InsertFile(stale, FileInfo{Size: 1, ModTime: 1})
	idx.InsertDir(filepath.ToSlash(filepath.Join(repo, "orphan-dir")))

	if err := replaceIndexFromScan(root, idx); err != nil {
		t.Fatalf("replace index from local scan: %v", err)
	}

	if !idx.HasFile(filepath.ToSlash(keepPath)) {
		t.Fatalf("expected keep file to remain after rebuild")
	}
	if idx.HasFile(stale) {
		t.Fatalf("expected stale file to be removed")
	}
	if idx.HasDir(filepath.ToSlash(filepath.Join(repo, "orphan-dir"))) {
		t.Fatalf("expected orphan dir to be removed")
	}
	if !idx.HasDir(filepath.ToSlash(filepath.Join(repo, "com", "example"))) {
		t.Fatalf("expected scanned dir to be present")
	}
}

func TestReplaceIndexFromFailedS3ScanPreservesExistingIndex(t *testing.T) {
	originalBuilder := S3IndexBuilder
	t.Cleanup(func() { S3IndexBuilder = originalBuilder })

	root := filepath.ToSlash(t.TempDir())
	existing := filepath.ToSlash(filepath.Join(root, "releases", "existing.jar"))
	partial := filepath.ToSlash(filepath.Join(root, "releases", "partial.jar"))
	idx := NewFileIndex()
	idx.InsertFile(existing, FileInfo{Size: 10, ModTime: 20})

	S3IndexBuilder = func(_ string, scanned *FileIndex) error {
		scanned.InsertFile(partial, FileInfo{Size: 30, ModTime: 40})
		return errors.New("listing interrupted")
	}

	if err := replaceIndexFromScan(root, idx); err == nil {
		t.Fatal("failed S3 scan returned nil error")
	}
	if !idx.HasFile(existing) {
		t.Fatal("failed S3 scan removed an existing index entry")
	}
	if idx.HasFile(partial) {
		t.Fatal("failed S3 scan published a partial index entry")
	}
}

func TestReplaceIndexFromSuccessfulS3ScanPublishesSnapshot(t *testing.T) {
	originalBuilder := S3IndexBuilder
	t.Cleanup(func() { S3IndexBuilder = originalBuilder })

	root := filepath.ToSlash(t.TempDir())
	stale := filepath.ToSlash(filepath.Join(root, "releases", "stale.jar"))
	fresh := filepath.ToSlash(filepath.Join(root, "releases", "fresh.jar"))
	idx := NewFileIndex()
	idx.InsertFile(stale, FileInfo{Size: 10, ModTime: 20})

	S3IndexBuilder = func(_ string, scanned *FileIndex) error {
		scanned.InsertFile(fresh, FileInfo{Size: 30, ModTime: 40})
		return nil
	}

	if err := replaceIndexFromScan(root, idx); err != nil {
		t.Fatalf("successful S3 scan returned error: %v", err)
	}
	if idx.HasFile(stale) {
		t.Fatal("successful S3 scan retained a stale index entry")
	}
	if !idx.HasFile(fresh) {
		t.Fatal("successful S3 scan did not publish the new index entry")
	}
}

func TestAsyncIndexRebuildCoalescesBurst(t *testing.T) {
	originalBuilder := S3IndexBuilder
	t.Cleanup(func() { S3IndexBuilder = originalBuilder })

	started := make(chan string, 3)
	release := make(chan struct{}, 3)
	finished := make(chan string, 3)
	var calls atomic.Int32
	S3IndexBuilder = func(basePath string, _ *FileIndex) error {
		calls.Add(1)
		started <- basePath
		<-release
		finished <- basePath
		return nil
	}
	receive := func(channel <-chan string) string {
		t.Helper()
		select {
		case value := <-channel:
			return value
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for index rebuild")
			return ""
		}
	}

	idx := NewFileIndex()
	RebuildIndexAsync("first", idx)
	if path := receive(started); path != "first" {
		t.Fatalf("first rebuild path = %q", path)
	}
	latest := ""
	for request := range 64 {
		latest = "next-" + strconv.Itoa(request)
		RebuildIndexDiff(latest, idx)
	}
	release <- struct{}{}
	_ = receive(finished)
	if path := receive(started); path != latest {
		t.Fatalf("coalesced rebuild path = %q, want %q", path, latest)
	}
	release <- struct{}{}
	_ = receive(finished)

	deadline := time.Now().Add(2 * time.Second)
	for {
		idx.rebuildMu.Lock()
		running := idx.rebuildRunning
		idx.rebuildMu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("coalesced index rebuild worker did not stop")
		}
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != 2 {
		t.Fatalf("rebuild calls = %d, want 2", calls.Load())
	}
	if !idx.IsDirty.Load() {
		t.Fatal("coalesced diff rebuild did not mark the index dirty")
	}
}

func TestInternStringCanonical(t *testing.T) {
	s1 := "storage/releases/probe"
	s2 := string([]byte(s1))

	got1 := internString(s1)
	got2 := internString(s2)

	if got1 != s1 || got2 != s2 {
		t.Fatalf("interned value mismatch: got1=%q, got2=%q", got1, got2)
	}
	if got1 != got2 {
		t.Fatalf("expected identical string values")
	}
}
