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
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"renop/internal/core"
	"renop/internal/service/index"
)

func TestDockerStoreDiskOperations(t *testing.T) {
	tempDir := storageTestTempDir(t)
	store := NewDockerStore(tempDir)

	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()

	repoName := "test-docker-repo"
	uploadUUID := "upload-uuid-12345"

	staged, err := store.StageBlob(repoName, uploadUUID)
	if err != nil {
		t.Fatalf("StageBlob failed: %v", err)
	}

	chunk1 := []byte("first-chunk-of-layer-data-")
	chunk2 := []byte("second-chunk-of-layer-data")
	totalData := append(chunk1, chunk2...)

	n1, err := staged.Write(chunk1)
	if err != nil || n1 != len(chunk1) {
		t.Fatalf("Write chunk1 failed: n=%d, err=%v", n1, err)
	}

	n2, err := staged.Write(chunk2)
	if err != nil || n2 != len(chunk2) {
		t.Fatalf("Write chunk2 failed: n=%d, err=%v", n2, err)
	}

	size, err := staged.Size()
	if err != nil || size != int64(len(totalData)) {
		t.Fatalf("unexpected staged size: %d; expected %d", size, len(totalData))
	}

	calculatedDigest, err := staged.Digest()
	if err != nil {
		t.Fatalf("Digest calculation failed: %v", err)
	}

	h := sha256.Sum256(totalData)
	expectedDigest := "sha256:" + hex.EncodeToString(h[:])
	if calculatedDigest != expectedDigest {
		t.Fatalf("expected digest %s, got %s", expectedDigest, calculatedDigest)
	}

	_ = staged.Close()

	committedSize, err := store.CommitBlob(state, repoName, uploadUUID, calculatedDigest)
	if err != nil {
		t.Fatalf("CommitBlob failed: %v", err)
	}
	if committedSize != int64(len(totalData)) {
		t.Fatalf("expected committed size %d, got %d", len(totalData), committedSize)
	}

	exists, bSize, err := store.BlobExists(repoName, calculatedDigest)
	if err != nil || !exists || bSize != int64(len(totalData)) {
		t.Fatalf("BlobExists failed: exists=%v, size=%d, err=%v", exists, bSize, err)
	}

	rc, rSize, ok, err := store.OpenBlob(repoName, calculatedDigest)
	if err != nil || !ok || rSize != int64(len(totalData)) {
		t.Fatalf("OpenBlob failed: ok=%v, size=%d, err=%v", ok, rSize, err)
	}

	readBytes, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil || string(readBytes) != string(totalData) {
		t.Fatalf("Blob content mismatch: got %q, expected %q", string(readBytes), string(totalData))
	}

	if err := store.DeleteBlob(state, repoName, calculatedDigest); err != nil {
		t.Fatalf("DeleteBlob failed: %v", err)
	}
	existsAfterDelete, _, _ := store.BlobExists(repoName, calculatedDigest)
	if existsAfterDelete {
		t.Fatal("expected blob to no longer exist after deletion")
	}

	discardUUID := "upload-uuid-discard"
	stagedDiscard, err := store.StageBlob(repoName, discardUUID)
	if err != nil {
		t.Fatalf("StageBlob for discard failed: %v", err)
	}
	_, _ = stagedDiscard.Write([]byte("temporary-abandoned-data"))
	stagingFilePath := filepath.Join(tempDir, repoName, ".renop.tmp.docker", discardUUID)

	if err := stagedDiscard.Discard(); err != nil {
		t.Fatalf("Discard failed: %v", err)
	}
	if _, err := os.Stat(stagingFilePath); !os.IsNotExist(err) {
		t.Fatal("expected staging file to be removed upon Discard")
	}

	imageName := "library/alpine"
	manifestData := []byte(`{"schemaVersion": 2, "mediaType": "application/vnd.docker.distribution.manifest.v2+json"}`)
	mh := sha256.Sum256(manifestData)
	manifestDigest := "sha256:" + hex.EncodeToString(mh[:])

	if err := store.PutManifest(state, repoName, imageName, manifestDigest, manifestData); err != nil {
		t.Fatalf("PutManifest failed: %v", err)
	}

	openedData, found, err := store.OpenManifest(repoName, imageName, manifestDigest)
	if err != nil || !found || string(openedData) != string(manifestData) {
		t.Fatalf("OpenManifest failed: found=%v, data=%q, err=%v", found, string(openedData), err)
	}

	if err := store.DeleteManifest(state, repoName, imageName, manifestDigest); err != nil {
		t.Fatalf("DeleteManifest failed: %v", err)
	}

	_, foundAfterDelete, _ := store.OpenManifest(repoName, imageName, manifestDigest)
	if foundAfterDelete {
		t.Fatal("expected manifest to be deleted")
	}
}

func TestDockerStoreCommitDigestMismatch(t *testing.T) {
	tempDir := storageTestTempDir(t)
	store := NewDockerStore(tempDir)
	state := core.NewAppState()

	repoName := "test-docker-repo"
	uploadUUID := "upload-uuid-mismatch"

	staged, err := store.StageBlob(repoName, uploadUUID)
	if err != nil {
		t.Fatalf("StageBlob failed: %v", err)
	}
	_, _ = staged.Write([]byte("payload-abc"))
	_ = staged.Close()

	wrongDigest := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	_, err = store.CommitBlob(state, repoName, uploadUUID, wrongDigest)
	if err == nil {
		t.Fatal("expected CommitBlob to fail on digest mismatch")
	}
}
