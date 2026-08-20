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
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/utils"
)

var artifactCompanionExts = []string{".md5", ".sha1", ".sha256", ".sha512", ".asc"}

var (
	uniqueSnapshotTimestampRE = regexp.MustCompile(`^(.+)-(\d{8}\.\d{6}-\d+)(-[^.]+)?(\.[A-Za-z0-9]+)$`)
	uniqueSnapshotBuildRE     = regexp.MustCompile(`^(.+)-(\d+)(-[^.]+)?(\.[A-Za-z0-9]+)$`)
)

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
	m := uniqueSnapshotTimestampRE.FindStringSubmatch(baseName)
	if m == nil {
		m = uniqueSnapshotBuildRE.FindStringSubmatch(baseName)
	}
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

func isMavenMetadataPath(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	return name == "maven-metadata.xml"
}

// cleanupSnapshotArtifactsFromMetadata removes unique snapshot builds that are
// no longer advertised by version-level Maven metadata. Maven Central and
// compatible repositories publish the authoritative build value in
// <snapshotVersions>, so using the metadata avoids retaining stale builds when
// an upstream repository advances from build 1 to build 2.
func cleanupSnapshotArtifactsFromMetadata(state *core.AppState, metadataPath string) error {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil || !isMavenMetadataPath(metadataPath) {
		return nil
	}

	versionDir := filepath.Dir(metadataPath)
	versionName := filepath.Base(versionDir)
	if !strings.Contains(strings.ToUpper(versionName), "SNAPSHOT") {
		return nil
	}

	const maxMetadataSize = 2 * 1024 * 1024
	var r io.ReadCloser
	var err error
	if IsS3Enabled(metadataPath) {
		r, _, err = DownloadFromS3(utils.GetS3Key(metadataPath))
	} else {
		r, err = os.Open(metadataPath)
	}
	if err != nil {
		return err
	}
	limited := &io.LimitedReader{R: r, N: maxMetadataSize + 1}
	var metadata config.Metadata
	decodeErr := xml.NewDecoder(limited).Decode(&metadata)
	closeErr := r.Close()
	if decodeErr != nil {
		return decodeErr
	}
	if limited.N <= 0 {
		return io.ErrShortBuffer
	}
	if closeErr != nil {
		return closeErr
	}

	if metadata.Versioning == nil || metadata.Versioning.SnapshotVersions == nil ||
		len(metadata.Versioning.SnapshotVersions.SnapshotVersion) == 0 {
		return nil
	}

	artifactID := ""
	if metadata.ArtifactId != nil {
		artifactID = strings.TrimSpace(*metadata.ArtifactId)
	}
	if artifactID == "" {
		artifactID = filepath.Base(filepath.Dir(versionDir))
	}
	if artifactID == "" || strings.ContainsAny(artifactID, `/\\`) {
		return nil
	}

	baseVersion := versionName
	if idx := strings.LastIndex(strings.ToUpper(baseVersion), "-SNAPSHOT"); idx >= 0 {
		baseVersion = baseVersion[:idx]
	}
	if baseVersion == "" {
		return nil
	}
	prefix := artifactID + "-" + baseVersion
	allowed := make(map[string]struct{}, len(metadata.Versioning.SnapshotVersions.SnapshotVersion)*2)
	for _, snapshotVersion := range metadata.Versioning.SnapshotVersions.SnapshotVersion {
		if snapshotVersion.Value == nil || snapshotVersion.Extension == nil {
			continue
		}
		value := strings.TrimSpace(*snapshotVersion.Value)
		extension := strings.TrimPrefix(strings.TrimSpace(*snapshotVersion.Extension), ".")
		if value == "" || extension == "" || strings.ContainsAny(value+extension, `/\\`) || len(value) > 512 || len(extension) > 32 {
			continue
		}
		classifier := ""
		if snapshotVersion.Classifier != nil {
			classifier = strings.TrimSpace(*snapshotVersion.Classifier)
			if classifier != "" && (strings.ContainsAny(classifier, `/\\`) || len(classifier) > 128) {
				continue
			}
		}
		stem := artifactID + "-" + value
		if classifier != "" {
			stem += "-" + classifier
		}
		allowed[stem+"."+extension] = struct{}{}
		// A few Maven-compatible repositories omit the base version in value;
		// accept both spellings while still constraining the artifact prefix.
		if !strings.HasPrefix(value, baseVersion+"-") {
			prefixed := prefix + "-" + value
			if classifier != "" {
				prefixed += "-" + classifier
			}
			allowed[prefixed+"."+extension] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return nil
	}

	entriesFromIndex := state.Inner.FileIndex.GetChildren(filepath.ToSlash(versionDir))
	seen := make(map[string]struct{}, len(entriesFromIndex)+16)
	children := make([]string, 0, len(entriesFromIndex)+16)
	for _, child := range entriesFromIndex {
		if _, exists := seen[child]; !exists {
			seen[child] = struct{}{}
			children = append(children, child)
		}
	}
	if !IsS3Enabled(metadataPath) {
		if entries, readErr := os.ReadDir(versionDir); readErr == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if _, exists := seen[entry.Name()]; !exists {
					seen[entry.Name()] = struct{}{}
					children = append(children, entry.Name())
				}
			}
		}
	}

	for _, child := range children {
		if strings.HasPrefix(strings.ToLower(child), "maven-metadata") {
			continue
		}
		artifactBase := stripArtifactCompanionSuffix(child)
		parts, ok := parseUniqueSnapshotBaseName(artifactBase)
		if !ok || parts.prefix != prefix {
			continue
		}
		if _, keep := allowed[artifactBase]; keep {
			continue
		}
		if err := removeIndexedFile(state, filepath.Join(versionDir, child)); err != nil {
			return err
		}
	}
	return nil
}
