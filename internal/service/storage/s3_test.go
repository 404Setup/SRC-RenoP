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

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
)

func TestGetS3ConfigForPathRequiresStorageBoundary(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = "storage"
	cfg.Maven.Repositories["releases"].S3 = &config.S3Config{Enabled: true}
	InitS3(cfg)

	if got := GetS3ConfigForPath("storageevil/releases/file.jar"); got != nil {
		t.Fatal("matched an S3 repository outside the configured storage path")
	}
	if got := GetS3ConfigForPath("storage/releases/file.jar"); got == nil {
		t.Fatal("did not match an S3 repository below the configured storage path")
	}
}

func TestSaveAndUploadChecksumDoesNotIndexFailedWrite(t *testing.T) {
	blockedParent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedParent, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	basePath := filepath.Join(blockedParent, "artifact.jar")
	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()

	if err := SaveAndUploadChecksum(state, basePath, ".sha256", "hash"); err == nil {
		t.Fatal("checksum write unexpectedly succeeded")
	}
	if state.Inner.FileIndex.HasFile(basePath + ".sha256") {
		t.Fatal("failed checksum write was inserted into the index")
	}
}

func TestDeleteIndexedFileKeepsIndexOnDeleteFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "non-empty")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "child"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()
	state.Inner.FileIndex.InsertFile(dir)

	if err := deleteIndexedFile(state, dir); err == nil {
		t.Fatal("deleting a non-empty directory as a file unexpectedly succeeded")
	}
	if !state.Inner.FileIndex.HasFile(dir) {
		t.Fatal("failed deletion removed the index entry")
	}
}
