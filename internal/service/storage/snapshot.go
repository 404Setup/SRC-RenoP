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
	"regexp"
	"strings"

	"renop/internal/core"
)

var artifactCompanionExts = []string{".md5", ".sha1", ".sha256", ".sha512", ".asc"}

var uniqueSnapshotBaseRE = regexp.MustCompile(`^(.+)-(\d{8}\.\d{6}-\d+)(-[^.]+)?(\.[A-Za-z0-9]+)$`)

func isArtifactCompanionPath(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range artifactCompanionExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func isSnapshotArtifactPath(localFilePath string) bool {
	slash := filepath.ToSlash(localFilePath)
	return strings.Contains(strings.ToUpper(slash), "SNAPSHOT")
}

func removeArtifactCompanions(state *core.AppState, artifactPath string) error {
	if state == nil || artifactPath == "" {
		return nil
	}
	for _, ext := range artifactCompanionExts {
		companion := artifactPath + ext
		if err := deleteIndexedFile(state, companion); err != nil {
			return err
		}
	}
	return nil
}

func removeIndexedFile(state *core.AppState, path string) error {
	if state == nil || path == "" {
		return nil
	}
	return deleteIndexedFile(state, path)
}

type uniqueSnapshotParts struct {
	prefix     string
	uniqueVer  string
	classifier string
	primaryExt string
}

func parseUniqueSnapshotBaseName(baseName string) (uniqueSnapshotParts, bool) {
	m := uniqueSnapshotBaseRE.FindStringSubmatch(baseName)
	if m == nil {
		return uniqueSnapshotParts{}, false
	}
	return uniqueSnapshotParts{
		prefix:     m[1],
		uniqueVer:  m[2],
		classifier: m[3],
		primaryExt: m[4],
	}, true
}

func stripArtifactCompanionSuffix(name string) string {
	lower := strings.ToLower(name)
	for _, ext := range artifactCompanionExts {
		if strings.HasSuffix(lower, ext) {
			return name[:len(name)-len(ext)]
		}
	}
	return name
}

func cleanupSupersededUniqueSnapshots(state *core.AppState, localFilePath string) error {
	if state == nil || state.Inner.FileIndex == nil {
		return nil
	}

	baseName := filepath.Base(localFilePath)
	if isArtifactCompanionPath(baseName) {
		return nil
	}
	parts, ok := parseUniqueSnapshotBaseName(baseName)
	if !ok {
		return nil
	}

	dir := filepath.Dir(localFilePath)
	dirSlash := filepath.ToSlash(dir)
	if !strings.Contains(strings.ToUpper(filepath.Base(dirSlash)), "SNAPSHOT") {
		return nil
	}

	entriesFromIndex := state.Inner.FileIndex.GetChildren(dirSlash)

	estimatedCap := len(entriesFromIndex) + 16
	seen := make(map[string]struct{}, estimatedCap)
	children := make([]string, 0, estimatedCap)

	for _, child := range entriesFromIndex {
		if _, ok := seen[child]; !ok {
			seen[child] = struct{}{}
			children = append(children, child)
		}
	}
	if !IsS3Enabled(localFilePath) {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					children = append(children, name)
				}
			}
		}
	}

	for _, child := range children {
		if child == baseName {
			continue
		}
		if strings.HasPrefix(child, "maven-metadata") {
			continue
		}
		artifactBase := stripArtifactCompanionSuffix(child)
		if strings.Contains(strings.ToUpper(artifactBase), "SNAPSHOT") &&
			!uniqueSnapshotBaseRE.MatchString(artifactBase) {
			continue
		}

		other, ok := parseUniqueSnapshotBaseName(artifactBase)
		if !ok {
			continue
		}
		if other.prefix != parts.prefix {
			continue
		}
		if other.uniqueVer == parts.uniqueVer {
			continue
		}
		if err := removeIndexedFile(state, filepath.Join(dir, child)); err != nil {
			return err
		}
	}
	return nil
}
