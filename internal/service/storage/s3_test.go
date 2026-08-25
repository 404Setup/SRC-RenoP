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
	"renop/internal/utils"
)

type normalizeS3KeyPrefixTestCase struct {
	name    string
	input   string
	want    string
	wantErr bool
}

func TestNormalizeS3KeyPrefix(t *testing.T) {
	tests := []normalizeS3KeyPrefixTestCase{
		{name: "empty"},
		{name: "slashes only", input: "/", want: ""},
		{name: "trimmed", input: " /renop/production/ ", want: "renop/production"},
		{name: "single segment", input: "renop", want: "renop"},
		{name: "empty segment", input: "renop//production", wantErr: true},
		{name: "current segment", input: "renop/./production", wantErr: true},
		{name: "parent segment", input: "renop/../production", wantErr: true},
		{name: "backslash", input: `renop\production`, wantErr: true},
		{name: "segment whitespace", input: "renop /production", wantErr: true},
		{name: "control character", input: "renop\nproduction", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeS3KeyPrefix(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeS3KeyPrefix(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("NormalizeS3KeyPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestS3ObjectKeyPreservesLegacyLayoutWithoutPrefix(t *testing.T) {
	logicalKey := `D:/renop/storage/releases/com/example/demo.jar`
	got, err := s3ObjectKey(&config.S3Config{}, logicalKey)
	if err != nil {
		t.Fatal(err)
	}
	if got != logicalKey {
		t.Fatalf("object key = %q, want unchanged key %q", got, logicalKey)
	}
}

func TestS3ObjectKeyUsesStorageRelativeLayoutWithPrefix(t *testing.T) {
	originalConfig := currentConfig.Load()
	t.Cleanup(func() { currentConfig.Store(originalConfig) })

	storagePath := filepath.Join(storageTestTempDir(t), "storage")
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	currentConfig.Store(cfg)
	s3Cfg := &config.S3Config{KeyPrefix: "/renop/production/"}
	logicalKey := utils.GetS3Key(filepath.Join(storagePath, "releases", "com", "example", "demo.jar"))

	got, err := s3ObjectKey(s3Cfg, logicalKey)
	if err != nil {
		t.Fatal(err)
	}
	if want := "renop/production/releases/com/example/demo.jar"; got != want {
		t.Fatalf("object key = %q, want %q", got, want)
	}

	directoryKey := utils.GetS3Key(filepath.Join(storagePath, "releases")) + "/"
	got, err = s3ObjectKey(s3Cfg, directoryKey)
	if err != nil {
		t.Fatal(err)
	}
	if want := "renop/production/releases/"; got != want {
		t.Fatalf("directory object key = %q, want %q", got, want)
	}
}

func TestS3ObjectKeyIsStableAcrossStoragePaths(t *testing.T) {
	originalConfig := currentConfig.Load()
	t.Cleanup(func() { currentConfig.Store(originalConfig) })

	cfg := config.DefaultConfig()
	s3Cfg := &config.S3Config{KeyPrefix: "renop/production"}
	keys := make([]string, 0, 2)
	for _, storagePath := range []string{
		filepath.Join(storageTestTempDir(t), "first"),
		filepath.Join(storageTestTempDir(t), "second"),
	} {
		cfg.StoragePath = storagePath
		currentConfig.Store(cfg)
		logicalKey := utils.GetS3Key(filepath.Join(storagePath, "releases", "demo.jar"))
		key, err := s3ObjectKey(s3Cfg, logicalKey)
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key)
	}
	if keys[0] != keys[1] || keys[0] != "renop/production/releases/demo.jar" {
		t.Fatalf("object keys are not stable across storage paths: %q", keys)
	}
}

func TestS3ObjectKeyRejectsPathOutsideStorage(t *testing.T) {
	originalConfig := currentConfig.Load()
	t.Cleanup(func() { currentConfig.Store(originalConfig) })

	cfg := config.DefaultConfig()
	cfg.StoragePath = filepath.Join(storageTestTempDir(t), "storage")
	currentConfig.Store(cfg)

	_, err := s3ObjectKey(
		&config.S3Config{KeyPrefix: "renop"},
		utils.GetS3Key(filepath.Join(filepath.Dir(cfg.StoragePath), "outside", "demo.jar")),
	)
	if err == nil {
		t.Fatal("object key outside storage path was accepted")
	}
}

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
	blockedParent := filepath.Join(storageTestTempDir(t), "not-a-directory")
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
	dir := filepath.Join(storageTestTempDir(t), "non-empty")
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

func TestLocalPathFromS3ObjectRestoresAbsoluteStoragePath(t *testing.T) {
	repoDir := filepath.FromSlash("/srv/renop/releases")
	prefix := "srv/renop/releases/"
	objectKey := prefix + "com/example/demo/1.0/demo-1.0.jar"

	got, ok := localPathFromS3Object(repoDir, prefix, objectKey)
	if !ok {
		t.Fatal("valid S3 object key was rejected")
	}
	want := filepath.Join(repoDir, "com", "example", "demo", "1.0", "demo-1.0.jar")
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("local path = %q, want %q", got, want)
	}
}

func TestLocalPathFromS3ObjectRejectsTraversal(t *testing.T) {
	repoDir := filepath.Join(storageTestTempDir(t), "releases")
	prefix := utils.GetS3Key(repoDir) + "/"
	if _, ok := localPathFromS3Object(repoDir, prefix, prefix+"../../outside"); ok {
		t.Fatal("traversal object key was accepted")
	}
}

func TestBuildS3IndexSyncReturnsClientError(t *testing.T) {
	originalConfig := currentConfig.Load()
	t.Cleanup(func() { currentConfig.Store(originalConfig) })

	cfg := config.DefaultConfig()
	cfg.StoragePath = storageTestTempDir(t)
	cfg.Maven.Repositories["releases"].S3 = &config.S3Config{
		Enabled:  true,
		Endpoint: "ftp://example.com",
	}
	InitS3(cfg)

	err := BuildS3IndexSync(cfg.StoragePath, index.NewFileIndex())
	if err == nil {
		t.Fatal("S3 index build ignored an invalid client configuration")
	}
}
