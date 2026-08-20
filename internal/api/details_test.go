/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package api

import (
	"os"
	"path/filepath"
	"testing"

	"renop/internal/core"
	"renop/internal/service/index"
)

func TestCreateFileDetailsDoesNotExposeBlockedPhysicalFile(t *testing.T) {
	artifactPath := filepath.Join(t.TempDir(), "releases", "org", "example", "demo.jar")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte("quarantined"), 0644); err != nil {
		t.Fatal(err)
	}

	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndexCustom(true)
	state.Inner.FileIndex.InsertFile(artifactPath, index.FileInfo{Size: 11, ModTime: 1})
	state.Inner.FileIndex.BlockFile(artifactPath)

	if details := CreateFileDetails(state, artifactPath, false); details != nil {
		t.Fatalf("blocked physical file was exposed: %+v", details)
	}
}
