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
	"bytes"
	"testing"

	"github.com/goccy/go-json"
)

func TestFileIndexGetChildren(t *testing.T) {
	idx := NewFileIndex()

	idx.InsertDir("parent")
	idx.InsertFile("parent/child1.txt")
	idx.InsertFile("parent/child2.txt")
	idx.InsertDir("parent/subdir")
	idx.InsertFile("parent/subdir/nested.txt")

	children := idx.GetChildren("parent")
	if len(children) != 3 {
		t.Errorf("Expected 3 children under parent, got %d: %v", len(children), children)
	}

	hasChild1 := false
	hasChild2 := false
	hasSubdir := false
	for _, c := range children {
		if c == "child1.txt" {
			hasChild1 = true
		} else if c == "child2.txt" {
			hasChild2 = true
		} else if c == "subdir" {
			hasSubdir = true
		}
	}

	if !hasChild1 || !hasChild2 || !hasSubdir {
		t.Errorf("Expected children to contain child1.txt, child2.txt, and subdir, got: %v", children)
	}

	idx.RemoveFile("parent/child1.txt")
	children = idx.GetChildren("parent")
	if len(children) != 2 {
		t.Errorf("Expected 2 children after removal, got %v", children)
	}

	idx.RemoveDir("parent/subdir")
	children = idx.GetChildren("parent")
	if len(children) != 1 {
		t.Errorf("Expected 1 child after subdir removal, got %v", children)
	}
	if len(children) > 0 && children[0] != "child2.txt" {
		t.Errorf("Expected only child2.txt, got %v", children)
	}
	if idx.HasDir("parent/subdir") || idx.HasFile("parent/subdir/nested.txt") {
		t.Errorf("Expected RemoveDir to purge nested dir and files")
	}
}

func TestInsertFileClearsNotFound(t *testing.T) {
	idx := NewFileIndex()
	path := "storage/releases/com/example/a.jar"
	idx.InsertNotFound(path, 4102444800)
	if !idx.IsNotFound(path) {
		t.Fatal("expected path to be negatively cached")
	}
	idx.InsertFile(path, FileInfo{Size: 1, ModTime: 2})
	if idx.IsNotFound(path) {
		t.Fatal("expected InsertFile to clear negative cache entry")
	}
	if !idx.HasFile(path) {
		t.Fatal("expected file to be present after insert")
	}
}

func TestRemoveDirPurgesNotFoundDescendants(t *testing.T) {
	idx := NewFileIndex()
	idx.InsertDir("storage/repo")
	idx.InsertDir("storage/repo/a")
	idx.InsertFile("storage/repo/a/file.txt")
	idx.InsertNotFound("storage/repo/a/missing.jar", 4102444800)
	idx.InsertNotFound("storage/repo/other/missing.jar", 4102444800)

	idx.RemoveDir("storage/repo")

	if idx.HasDir("storage/repo") || idx.HasDir("storage/repo/a") || idx.HasFile("storage/repo/a/file.txt") {
		t.Fatal("expected RemoveDir to remove files and dirs under the tree")
	}
	if idx.IsNotFound("storage/repo/a/missing.jar") || idx.IsNotFound("storage/repo/other/missing.jar") {
		t.Fatal("expected RemoveDir to purge not-found descendants")
	}
	if len(idx.GetChildren("storage/repo")) != 0 {
		t.Fatalf("expected no children under removed dir, got %v", idx.GetChildren("storage/repo"))
	}
}

func TestEnsureParentDirsIndexesIntermediateFolders(t *testing.T) {
	idx := NewFileIndex()
	idx.InsertDir("storage/repo")
	filePath := "storage/repo/com/example/lib/1.0/lib.jar"
	idx.EnsureParentDirs(filePath)
	idx.InsertFile(filePath, FileInfo{Size: 10, ModTime: 1})

	for _, dir := range []string{"storage/repo/com", "storage/repo/com/example", "storage/repo/com/example/lib", "storage/repo/com/example/lib/1.0"} {
		if !idx.HasDir(dir) {
			t.Fatalf("expected intermediate dir %q in index", dir)
		}
	}
	children := idx.GetChildren("storage/repo")
	if len(children) != 1 || children[0] != "com" {
		t.Fatalf("expected storage/repo children [com], got %v", children)
	}
}

func TestFileIndexWriteJSONEscapesPaths(t *testing.T) {
	idx := NewFileIndexCustom(true)
	idx.InsertFile(`storage/repo/quoted"name.jar`)
	var buf bytes.Buffer
	if err := idx.WriteJSONTo(&buf); err != nil {
		t.Fatal(err)
	}
	var decoded FileIndexSnapshot
	if err := json.NewDecoder(&buf).Decode(&decoded); err != nil {
		t.Fatalf("index JSON is invalid: %v", err)
	}
	if _, ok := decoded.Files[`storage/repo/quoted"name.jar`]; !ok {
		t.Fatalf("escaped path was not preserved: %#v", decoded.Files)
	}
}

func TestFileIndexReadJSONFromStreamsEntries(t *testing.T) {
	const snapshot = `{"files":{"storage/releases/a.jar":{"size":42,"mod_time":99}},"dirs":["storage","storage/releases"],"not_found":{"storage/releases/missing.jar":4102444800}}`
	idx := NewFileIndexCustom(true)
	if err := idx.ReadJSONFrom(bytes.NewBufferString(snapshot)); err != nil {
		t.Fatal(err)
	}
	if info, ok := idx.GetFileInfo("storage/releases/a.jar"); !ok || info.Size != 42 || info.ModTime != 99 {
		t.Fatalf("streamed file info = %+v, %v", info, ok)
	}
	if !idx.HasDir("storage/releases") {
		t.Fatal("streamed directory was not restored")
	}
	if !idx.IsNotFound("storage/releases/missing.jar") {
		t.Fatal("streamed negative-cache entry was not restored")
	}
}
