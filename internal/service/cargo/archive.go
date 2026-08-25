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
	"errors"
	"io"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/klauspost/compress/gzip"
)

const (
	maxArchiveEntries = 10000
	maxUnpackedSize   = 512 << 20
	maxManifestSize   = 1 << 20
)

type cargoManifestPackage struct {
	Name          string `toml:"name"`
	Version       string `toml:"version"`
	Description   string `toml:"description"`
	License       string `toml:"license"`
	LicenseFile   string `toml:"license-file"`
	Documentation string `toml:"documentation"`
	Homepage      string `toml:"homepage"`
	Repository    string `toml:"repository"`
	RustVersion   string `toml:"rust-version"`
	Edition       string `toml:"edition"`
	Links         string `toml:"links"`
}

type cargoManifestMetadata struct {
	Package cargoManifestPackage `toml:"package"`
}

func validateArchive(reader io.Reader, crateName, version string) (*cargoManifestPackage, error) {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return nil, errors.New("cargo crate is not a valid gzip archive")
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(io.LimitReader(gzipReader, maxUnpackedSize+1))
	var entries int
	var unpacked int64
	var hasManifest bool
	seenPaths := make(map[string]struct{})
	root := crateName + "-" + version
	manifest := root + "/Cargo.toml"
	var manifestMetadata cargoManifestMetadata
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("cargo crate contains an invalid tar archive")
		}
		entries++
		if entries > maxArchiveEntries {
			return nil, errors.New("cargo crate contains too many files")
		}
		if strings.ContainsAny(header.Name, "\\\x00") {
			return nil, errors.New("cargo crate contains an unsafe path")
		}
		clean := path.Clean(header.Name)
		if clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, ":") ||
			(clean != root && !strings.HasPrefix(clean, root+"/")) {
			return nil, errors.New("cargo crate contains an unsafe path")
		}
		canonicalName := strings.TrimSuffix(header.Name, "/")
		if canonicalName == "" || clean != canonicalName {
			return nil, errors.New("cargo crate contains a non-canonical path")
		}
		if _, exists := seenPaths[clean]; exists {
			return nil, errors.New("cargo crate contains a duplicate path")
		}
		seenPaths[clean] = struct{}{}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return nil, errors.New("cargo crate contains an unsupported file type")
		}
		if header.Size < 0 || unpacked > maxUnpackedSize-header.Size {
			return nil, errors.New("cargo crate expands beyond the size limit")
		}
		unpacked += header.Size
		if clean == manifest && header.Typeflag == tar.TypeReg {
			if header.Size > maxManifestSize {
				return nil, errors.New("cargo.toml exceeds the size limit")
			}
			if _, err := toml.NewDecoder(tarReader).Decode(&manifestMetadata); err != nil {
				return nil, errors.New("cargo crate contains an invalid Cargo.toml")
			}
			if manifestMetadata.Package.Name != crateName || !sameVersion(manifestMetadata.Package.Version, version) {
				return nil, errors.New("cargo.toml package name or version does not match publish metadata")
			}
			hasManifest = true
		}
	}
	if !hasManifest {
		return nil, errors.New("cargo crate does not contain Cargo.toml")
	}
	return &manifestMetadata.Package, nil
}
