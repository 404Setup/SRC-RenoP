/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package upload

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsUploadPartialName(t *testing.T) {
	cases := map[string]bool{
		"artifact.jar":                          false,
		"artifact.jar.tmp":                      true,
		"artifact.jar.tmp.abc-uuid":             true,
		"artifact.jar.chunk.550e8400-e29b-41d4": true,
		"readme.txt":                            false,
	}
	for name, want := range cases {
		if got := IsUploadPartialName(name); got != want {
			t.Fatalf("%q: got %v want %v", name, got, want)
		}
	}
}

func TestCleanupOrphanPartials(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "releases", "com", "ex")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}

	keep := filepath.Join(repo, "keep.jar")
	chunk := filepath.Join(repo, "keep.jar.chunk.deadbeef-uuid")
	tmp := filepath.Join(repo, "other.jar.tmp."+"abc123")
	if err := os.WriteFile(keep, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chunk, []byte("partial"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("partial"), 0644); err != nil {
		t.Fatal(err)
	}

	n := CleanupOrphanPartials(root, 0)
	if n != 2 {
		t.Fatalf("removed %d, want 2", n)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("real artifact should remain")
	}
	if _, err := os.Stat(chunk); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("chunk partial should be gone")
	}
	if _, err := os.Stat(tmp); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("tmp partial should be gone")
	}
}

func TestCleanupOrphanPartialsRespectsMaxAge(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "releases")
	_ = os.MkdirAll(repo, 0755)
	fresh := filepath.Join(repo, "a.jar.chunk.fresh-id")
	if err := os.WriteFile(fresh, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if n := CleanupOrphanPartials(root, 24*time.Hour); n != 0 {
		t.Fatalf("removed %d fresh files, want 0", n)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("fresh partial must remain")
	}
}

func TestAbortRemovesChunkTemp(t *testing.T) {
	mgr := &Manager{sessions: make(map[string]*Session)}
	dir := t.TempDir()
	dest := filepath.Join(dir, "f.bin")
	sess, err := mgr.CreateSession(PurposeStorage, "f.bin", "u", MinChunkSize, MinChunkSize, false, dest, "releases")
	if err != nil {
		t.Fatal(err)
	}
	path := sess.TempPath
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("temp should exist: %v", err)
	}
	sess.Abort()
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("Abort must remove chunk temp")
	}
	sess.Abort()
}

func TestAbortAfterCloseFileStillRemoves(t *testing.T) {
	mgr := &Manager{sessions: make(map[string]*Session)}
	dir := t.TempDir()
	dest := filepath.Join(dir, "g.bin")
	sess, err := mgr.CreateSession(PurposeStorage, "g.bin", "u", 0, DefaultChunkSize, false, dest, "releases")
	if err != nil {
		t.Fatal(err)
	}
	path := sess.TempPath
	if err := sess.CloseFile(); err != nil {
		t.Fatal(err)
	}
	sess.Abort()
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("Abort after CloseFile must still remove temp")
	}
}
