/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"path/filepath"
	"testing"

	"renop/internal/config"
	"renop/internal/core"
)

func newTestDockerDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := InitDB(config.DatabaseConfig{
		Driver: "sqlite",
		Dsn:    filepath.Join(dir, "docker_test.db"),
	})
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestDockerDatabaseOperations(t *testing.T) {
	db := newTestDockerDB(t)

	manifest := &core.DockerManifest{
		Repository:   "docker-local",
		ImageName:    "ubuntu",
		Digest:       "sha256:abc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		MediaType:    "application/vnd.docker.distribution.manifest.v2+json",
		Size:         1024,
		ConfigDigest: "sha256:cfg1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		RawJSON:      []byte(`{"schemaVersion":2}`),
	}

	err := db.PutDockerManifest(manifest, "latest", "admin")
	if err != nil {
		t.Fatalf("PutDockerManifest failed: %v", err)
	}

	err = db.PutDockerManifest(manifest, "22.04", "admin")
	if err != nil {
		t.Fatalf("PutDockerManifest with tag 22.04 failed: %v", err)
	}

	img, err := db.GetDockerImage("docker-local", "ubuntu")
	if err != nil {
		t.Fatalf("GetDockerImage failed: %v", err)
	}
	if img == nil {
		t.Fatal("expected image to be found")
	}
	if img.TagCount != 2 {
		t.Fatalf("expected 2 tags, got %d", img.TagCount)
	}
	if img.Publisher != "admin" {
		t.Fatalf("expected image publisher to be 'admin', got '%s'", img.Publisher)
	}

	tag, err := db.GetDockerTag("docker-local", "ubuntu", "latest")
	if err != nil {
		t.Fatalf("GetDockerTag failed: %v", err)
	}
	if tag == nil || tag.Digest != manifest.Digest {
		t.Fatalf("unexpected tag result: %+v", tag)
	}
	if tag.Publisher != "admin" {
		t.Fatalf("expected tag publisher to be 'admin', got '%s'", tag.Publisher)
	}

	tags, err := db.ListDockerTags("docker-local", "ubuntu", "", 50)
	if err != nil {
		t.Fatalf("ListDockerTags failed: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}

	storedManifest, err := db.GetDockerManifest("docker-local", "ubuntu", manifest.Digest)
	if err != nil {
		t.Fatalf("GetDockerManifest failed: %v", err)
	}
	if storedManifest == nil || storedManifest.Size != 1024 {
		t.Fatalf("unexpected manifest result: %+v", storedManifest)
	}
	if storedManifest.Publisher != "admin" {
		t.Fatalf("expected manifest publisher to be 'admin', got '%s'", storedManifest.Publisher)
	}

	details, err := db.GetDockerImageDetails("docker-local", "ubuntu")
	if err != nil || details == nil {
		t.Fatalf("GetDockerImageDetails failed: %v", err)
	}
	if len(details.Tags) != 2 || details.Image.Publisher != "admin" {
		t.Fatalf("unexpected details: %+v", details)
	}

	blobDigest := "sha256:blob1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	if err := db.RecordDockerBlob("docker-local", blobDigest, 5000); err != nil {
		t.Fatalf("RecordDockerBlob failed: %v", err)
	}
	exists, size, err := db.HasDockerBlob("docker-local", blobDigest)
	if err != nil || !exists || size != 5000 {
		t.Fatalf("HasDockerBlob failed: exists=%v, size=%d, err=%v", exists, size, err)
	}

	results, total, err := db.SearchDockerImages("docker-local", "ubun", 10, 0)
	if err != nil {
		t.Fatalf("SearchDockerImages failed: %v", err)
	}
	if total != 1 || len(results) != 1 || results[0].ImageName != "ubuntu" {
		t.Fatalf("unexpected search results: total=%d, len=%d", total, len(results))
	}

	totalImages, totalTags, totalSize, err := db.GetDockerRepositoryStats("docker-local")
	if err != nil {
		t.Fatalf("GetDockerRepositoryStats failed: %v", err)
	}
	if totalImages != 1 || totalTags != 2 || totalSize != 5000 {
		t.Fatalf("unexpected stats: images=%d, tags=%d, size=%d", totalImages, totalTags, totalSize)
	}

	if err := db.DeleteDockerTag("docker-local", "ubuntu", "22.04"); err != nil {
		t.Fatalf("DeleteDockerTag failed: %v", err)
	}
	deletedTag, err := db.GetDockerTag("docker-local", "ubuntu", "22.04")
	if err != nil || deletedTag != nil {
		t.Fatalf("expected tag to be deleted, got: %v", deletedTag)
	}

	if err := db.DeleteDockerManifest("docker-local", "ubuntu", manifest.Digest); err != nil {
		t.Fatalf("DeleteDockerManifest failed: %v", err)
	}

	if err := db.DeleteDockerRepository("docker-local"); err != nil {
		t.Fatalf("DeleteDockerRepository failed: %v", err)
	}
}
