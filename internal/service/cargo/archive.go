/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
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

func validateArchive(reader io.Reader, crateName, version string) error {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return errors.New("Cargo crate is not a valid gzip archive")
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(io.LimitReader(gzipReader, maxUnpackedSize+1))
	var entries int
	var unpacked int64
	var hasManifest bool
	seenPaths := make(map[string]struct{})
	root := crateName + "-" + version
	manifest := root + "/Cargo.toml"
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.New("Cargo crate contains an invalid tar archive")
		}
		entries++
		if entries > maxArchiveEntries {
			return errors.New("Cargo crate contains too many files")
		}
		if strings.ContainsAny(header.Name, "\\\x00") {
			return errors.New("Cargo crate contains an unsafe path")
		}
		clean := path.Clean(header.Name)
		if clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, ":") ||
			(clean != root && !strings.HasPrefix(clean, root+"/")) {
			return errors.New("Cargo crate contains an unsafe path")
		}
		canonicalName := strings.TrimSuffix(header.Name, "/")
		if canonicalName == "" || clean != canonicalName {
			return errors.New("Cargo crate contains a non-canonical path")
		}
		if _, exists := seenPaths[clean]; exists {
			return errors.New("Cargo crate contains a duplicate path")
		}
		seenPaths[clean] = struct{}{}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA && header.Typeflag != tar.TypeDir {
			return errors.New("Cargo crate contains an unsupported file type")
		}
		if header.Size < 0 || unpacked > maxUnpackedSize-header.Size {
			return errors.New("Cargo crate expands beyond the size limit")
		}
		unpacked += header.Size
		if clean == manifest && (header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA) {
			if header.Size > maxManifestSize {
				return errors.New("Cargo.toml exceeds the size limit")
			}
			var manifestMetadata struct {
				Package struct {
					Name    string `toml:"name"`
					Version string `toml:"version"`
				} `toml:"package"`
			}
			if _, err := toml.NewDecoder(tarReader).Decode(&manifestMetadata); err != nil {
				return errors.New("Cargo crate contains an invalid Cargo.toml")
			}
			if manifestMetadata.Package.Name != crateName || !sameVersion(manifestMetadata.Package.Version, version) {
				return errors.New("Cargo.toml package name or version does not match publish metadata")
			}
			hasManifest = true
		}
	}
	if !hasManifest {
		return errors.New("Cargo crate does not contain Cargo.toml")
	}
	return nil
}
