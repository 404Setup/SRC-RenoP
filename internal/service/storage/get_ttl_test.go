/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"renop/internal/core"
	"renop/internal/service/index"
)

func TestLoadMetadataAndCheckTTLExpiresLocalArtifact(t *testing.T) {
	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()
	path := filepath.Join(storageTestTempDir(t), "expired.jar")
	if err := os.WriteFile(path, []byte("expired"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	info := index.FileInfo{Size: 7, ModTime: old.UnixNano()}
	state.Inner.FileIndex.InsertFile(path, info)

	exists, gotInfo, isDir := LoadMetadataAndCheckTTL(state, path, filepath.ToSlash(path), true, true, false, info, false, 1)
	if exists || isDir || gotInfo != (index.FileInfo{}) {
		t.Fatalf("expired path state = exists %v, info %+v, dir %v", exists, gotInfo, isDir)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired local artifact remains: %v", err)
	}
	if state.Inner.FileIndex.HasFile(path) {
		t.Fatal("expired local artifact remains indexed")
	}
}
