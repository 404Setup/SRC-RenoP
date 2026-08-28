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
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"github.com/klauspost/compress/gzip"
)

const (
	maxArchiveEntries  = 10000
	maxUnpackedSize    = 512 << 20
	maxManifestSize    = 1 << 20
	maxCargoReadmeSize = 512 << 10
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
	Readme        any    `toml:"readme"`
	readmeContent string
}

type cargoManifestMetadata struct {
	Package cargoManifestPackage `toml:"package"`
}

func cargoReadmePath(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "", true
	case bool:
		return "", typed
	case string:
		if strings.ContainsAny(typed, "\\\x00") {
			return "", false
		}
		clean := path.Clean(strings.TrimSpace(typed))
		if clean == "." || clean == ".." || path.IsAbs(clean) || strings.HasPrefix(clean, "../") || strings.Contains(clean, ":") {
			return "", false
		}
		return clean, true
	default:
		return "", false
	}
}

func defaultCargoReadmeRank(relativePath string) int {
	for index, candidate := range []string{"README.md", "README.txt", "README"} {
		if strings.EqualFold(relativePath, candidate) {
			return index
		}
	}
	return -1
}

func readCargoReadmeFile(reader io.Reader, size int64) (string, error) {
	if size <= 0 {
		return "", nil
	}
	limited := &io.LimitedReader{R: reader, N: maxCargoReadmeSize + 1}
	var content strings.Builder
	content.Grow(int(min(size, maxCargoReadmeSize)))
	if _, err := io.Copy(&content, limited); err != nil {
		return "", err
	}
	readme := content.String()
	truncated := len(readme) > maxCargoReadmeSize
	if truncated {
		const suffix = "\n\n…"
		readme = readme[:maxCargoReadmeSize-len(suffix)]
		for !utf8.ValidString(readme) && len(readme) > 0 {
			readme = readme[:len(readme)-1]
		}
		readme = strings.TrimSpace(readme)
		if readme != "" {
			readme += suffix
		}
	}
	if !utf8.ValidString(readme) {
		return "", nil
	}
	if !truncated {
		readme = strings.TrimSpace(readme)
	}
	return readme, nil
}

func readDeclaredCargoReadme(reader io.Reader, crateName, version, relativePath string) (string, error) {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return "", errors.New("cargo crate is not a valid gzip archive")
	}
	defer gzipReader.Close()
	target := crateName + "-" + version + "/" + relativePath
	tarReader := tar.NewReader(io.LimitReader(gzipReader, maxUnpackedSize+1))
	for {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			return "", nil
		}
		if nextErr != nil {
			return "", errors.New("cargo crate contains an invalid tar archive")
		}
		if path.Clean(header.Name) == target && header.Typeflag == tar.TypeReg {
			return readCargoReadmeFile(tarReader, header.Size)
		}
	}
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
	var declaredReadme string
	var defaultReadmes [3]string
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
			continue
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		relativePath := strings.TrimPrefix(clean, root+"/")
		readmePath, readmeEnabled := cargoReadmePath(manifestMetadata.Package.Readme)
		defaultRank := defaultCargoReadmeRank(relativePath)
		captureDeclared := hasManifest && readmeEnabled && readmePath != "" && relativePath == readmePath
		if defaultRank < 0 && !captureDeclared {
			continue
		}
		readme, readErr := readCargoReadmeFile(tarReader, header.Size)
		if readErr != nil {
			return nil, errors.New("cargo crate README could not be read")
		}
		if defaultRank >= 0 {
			defaultReadmes[defaultRank] = readme
		}
		if captureDeclared {
			declaredReadme = readme
		}
	}
	if !hasManifest {
		return nil, errors.New("cargo crate does not contain Cargo.toml")
	}
	readmePath, readmeEnabled := cargoReadmePath(manifestMetadata.Package.Readme)
	if readmeEnabled {
		if readmePath != "" {
			if rank := defaultCargoReadmeRank(readmePath); rank >= 0 {
				manifestMetadata.Package.readmeContent = defaultReadmes[rank]
			} else {
				manifestMetadata.Package.readmeContent = declaredReadme
			}
		} else {
			for _, readme := range defaultReadmes {
				if readme != "" {
					manifestMetadata.Package.readmeContent = readme
					break
				}
			}
		}
	}
	return &manifestMetadata.Package, nil
}
