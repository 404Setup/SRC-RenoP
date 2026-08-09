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
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestPutWithoutRedeploymentAllowsMavenMetadataUpdates(t *testing.T) {
	app, state, storagePath, repo := setupSnapshotPutApp(t)
	repo.AllowRedeployment = false

	tests := []struct {
		name    string
		content []byte
	}{
		{name: "maven-metadata.xml", content: []byte("<metadata>new</metadata>")},
		{name: "maven-metadata.xml.sha1", content: []byte("new-checksum")},
		{name: "maven-metadata.xml.asc", content: []byte("new-signature")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(storagePath, "releases", "one", "pkg", "libsl", "modcommon", tt.name)
			mustWriteIndexed(t, state, path, []byte("old"))

			code := putBytes(t, app, "/releases/one/pkg/libsl/modcommon/"+tt.name, tt.content, nil)
			if code != fiber.StatusCreated {
				t.Fatalf("PUT returned %d, want %d", code, fiber.StatusCreated)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.content) {
				t.Fatalf("stored content = %q, want %q", got, tt.content)
			}
		})
	}
}

func TestPutWithoutRedeploymentStillRejectsArtifactOverwrite(t *testing.T) {
	app, state, storagePath, repo := setupSnapshotPutApp(t)
	repo.AllowRedeployment = false

	path := filepath.Join(storagePath, "releases", "one", "pkg", "libsl", "modcommon", "1.0", "modcommon-1.0.jar")
	mustWriteIndexed(t, state, path, []byte("old-artifact"))

	code := putBytes(t, app, "/releases/one/pkg/libsl/modcommon/1.0/modcommon-1.0.jar", []byte("new-artifact"), nil)
	if code != fiber.StatusConflict {
		t.Fatalf("PUT returned %d, want %d", code, fiber.StatusConflict)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("old-artifact")) {
		t.Fatalf("stored artifact was unexpectedly changed: %q", got)
	}
}

func TestMutableMavenMetadataPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "org/example/demo/maven-metadata.xml", want: true},
		{path: "org/example/demo/maven-metadata.xml.md5", want: true},
		{path: "org/example/demo/maven-metadata.xml.asc.sha256", want: true},
		{path: "org/example/demo/maven-metadata.xml.backup", want: false},
		{path: "org/example/demo/not-maven-metadata.xml", want: false},
	}

	for _, tt := range tests {
		if got := isMutableMavenMetadataPath(tt.path); got != tt.want {
			t.Errorf("isMutableMavenMetadataPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
