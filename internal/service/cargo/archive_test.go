/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"archive/tar"
	"bytes"
	"testing"

	"github.com/klauspost/compress/gzip"
)

func makeCrateArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range entries {
		data := []byte(content)
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestValidateArchive(t *testing.T) {
	valid := makeCrateArchive(t, map[string]string{
		"demo-1.0.0/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"1.0.0\"\n",
		"demo-1.0.0/src/lib.rs": "pub fn demo() {}\n",
	})
	if err := validateArchive(bytes.NewReader(valid), "demo", "1.0.0"); err != nil {
		t.Fatalf("valid crate rejected: %v", err)
	}

	unsafe := makeCrateArchive(t, map[string]string{
		"../Cargo.toml": "[package]\n",
	})
	if err := validateArchive(bytes.NewReader(unsafe), "demo", "1.0.0"); err == nil {
		t.Fatal("expected unsafe path rejection")
	}

	missingManifest := makeCrateArchive(t, map[string]string{
		"demo-1.0.0/src/lib.rs": "pub fn demo() {}\n",
	})
	if err := validateArchive(bytes.NewReader(missingManifest), "demo", "1.0.0"); err == nil {
		t.Fatal("expected missing Cargo.toml rejection")
	}

	mismatchedManifest := makeCrateArchive(t, map[string]string{
		"demo-1.0.0/Cargo.toml": "[package]\nname = \"other\"\nversion = \"1.0.0\"\n",
	})
	if err := validateArchive(bytes.NewReader(mismatchedManifest), "demo", "1.0.0"); err == nil {
		t.Fatal("expected mismatched Cargo.toml rejection")
	}
}

type validatePackageTestCase struct {
	name    string
	version string
	valid   bool
}

func TestValidatePackage(t *testing.T) {
	for _, test := range []validatePackageTestCase{
		{name: "serde", version: "1.0.203", valid: true},
		{name: "my_crate", version: "2.0.0-beta.1+meta", valid: true},
		{name: "-bad", version: "1.0.0"},
		{name: "crate/path", version: "1.0.0"},
		{name: "demo", version: "../1.0.0"},
		{name: "crate-é", version: "1.0.0"},
	} {
		err := validatePackage(test.name, test.version)
		if (err == nil) != test.valid {
			t.Errorf("validatePackage(%q, %q) error = %v, valid = %v", test.name, test.version, err, test.valid)
		}
	}
}
