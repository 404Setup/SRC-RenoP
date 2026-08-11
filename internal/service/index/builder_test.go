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
	"testing"

	"github.com/llxisdsh/pb"
)

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

func TestInternStringPartialEvictionKeepsPoolUsable(t *testing.T) {
	pathInternPool = pb.MapOf[string, string]{}
	pathInternSize.Store(0)
	for i := range 100 {
		internString("seed/" + string(rune('a'+i%26)) + "/" + string(rune('0'+i%10)))
	}
	pathInternSize.Store(50000)

	got := internString("storage/releases/partial-evict-probe")
	if got != "storage/releases/partial-evict-probe" {
		t.Fatalf("interned value = %q", got)
	}
	if pathInternSize.Load() >= 50000 {
		t.Fatalf("expected partial eviction to reduce pool size, got %d", pathInternSize.Load())
	}
}
