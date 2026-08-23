/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package docker

import (
	"strings"
	"testing"
)

func TestParseManifestSchema2(t *testing.T) {
	manifestJSON := []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		"config": {
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size": 1450,
			"digest": "sha256:70f3118c4420e64b85ee4901f4c7f0fa9ff9c0ed9be0900dfcbb3e01bc6f391b"
		},
		"layers": [
			{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size": 2814521,
				"digest": "sha256:c0d2a45a339f372173f495734bd63eca7c2b454470ac2880d861732d119b3cb2"
			},
			{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size": 1542,
				"digest": "sha256:4a0c8b3506c547fb0662d515a4c9eb38c6eb53e7f4c5409600e1cf59f518e38d"
			}
		]
	}`)

	parsed, err := ParseManifest(manifestJSON, "")
	if err != nil {
		t.Fatalf("ParseManifest failed: %v", err)
	}

	if parsed.IsIndex {
		t.Fatal("expected IsIndex to be false")
	}
	if parsed.MediaType != MediaTypeDockerManifest2 {
		t.Fatalf("expected mediaType '%s', got '%s'", MediaTypeDockerManifest2, parsed.MediaType)
	}
	if parsed.ConfigDigest != "sha256:70f3118c4420e64b85ee4901f4c7f0fa9ff9c0ed9be0900dfcbb3e01bc6f391b" {
		t.Fatalf("unexpected config digest: %s", parsed.ConfigDigest)
	}
	if len(parsed.Layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(parsed.Layers))
	}
	if parsed.Size != int64(len(manifestJSON)) {
		t.Fatalf("expected size %d, got %d", len(manifestJSON), parsed.Size)
	}

	expectedDigest := CalculateDigest(manifestJSON)
	if parsed.Digest != expectedDigest {
		t.Fatalf("expected digest %s, got %s", expectedDigest, parsed.Digest)
	}
}

func TestParseOCIManifest(t *testing.T) {
	ociJSON := []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.oci.image.manifest.v1+json",
		"config": {
			"mediaType": "application/vnd.oci.image.config.v1+json",
			"size": 1024,
			"digest": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
		},
		"layers": [
			{
				"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
				"size": 512,
				"digest": "sha256:ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"
			}
		]
	}`)

	parsed, err := ParseManifest(ociJSON, "application/vnd.oci.image.manifest.v1+json")
	if err != nil {
		t.Fatalf("ParseManifest failed: %v", err)
	}

	if parsed.IsIndex {
		t.Fatal("expected IsIndex to be false")
	}
	if parsed.MediaType != MediaTypeOCIManifest1 {
		t.Fatalf("expected mediaType '%s', got '%s'", MediaTypeOCIManifest1, parsed.MediaType)
	}
	if len(parsed.Layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(parsed.Layers))
	}
}

func TestParseMultiArchManifestIndex(t *testing.T) {
	indexJSON := []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
		"manifests": [
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"size": 7143,
				"digest": "sha256:e692418e4cbaf90ca69d05a3da4dc47979911f36406aa010436946ae81f3d2a9",
				"platform": {
					"architecture": "amd64",
					"os": "linux"
				}
			},
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"size": 7682,
				"digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270",
				"platform": {
					"architecture": "arm64",
					"os": "linux",
					"variant": "v8"
				}
			}
		]
	}`)

	parsed, err := ParseManifest(indexJSON, "")
	if err != nil {
		t.Fatalf("ParseManifest failed: %v", err)
	}

	if !parsed.IsIndex {
		t.Fatal("expected IsIndex to be true")
	}
	if parsed.MediaType != MediaTypeDockerManifestList {
		t.Fatalf("expected mediaType '%s', got '%s'", MediaTypeDockerManifestList, parsed.MediaType)
	}
	if len(parsed.Manifests) != 2 {
		t.Fatalf("expected 2 sub-manifests, got %d", len(parsed.Manifests))
	}
	if parsed.Manifests[0].Platform.Architecture != "amd64" {
		t.Fatalf("expected amd64 architecture, got '%s'", parsed.Manifests[0].Platform.Architecture)
	}
	if parsed.Manifests[1].Platform.Architecture != "arm64" {
		t.Fatalf("expected arm64 architecture, got '%s'", parsed.Manifests[1].Platform.Architecture)
	}
}

func TestParseManifestErrors(t *testing.T) {
	if _, err := ParseManifest([]byte{}, ""); err == nil {
		t.Fatal("expected error on empty data")
	}

	if _, err := ParseManifest([]byte("this is definitely not JSON"), ""); err == nil {
		t.Fatal("expected error on non-JSON data")
	}

	if _, err := ParseManifest([]byte(`{"schemaVersion": 2, "mediaType": `), ""); err == nil {
		t.Fatal("expected error on truncated JSON")
	}
}

func TestCalculateDigest(t *testing.T) {
	emptyData := []byte("")
	emptyDigest := CalculateDigest(emptyData)
	expected := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if emptyDigest != expected {
		t.Fatalf("expected %s, got %s", expected, emptyDigest)
	}

	testData := []byte("hello docker world")
	d1 := CalculateDigest(testData)
	d2 := CalculateDigest(testData)
	if d1 != d2 {
		t.Fatal("CalculateDigest should be deterministic")
	}
	if !strings.HasPrefix(d1, "sha256:") {
		t.Fatal("CalculateDigest must have sha256: prefix")
	}
}
