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
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/repositorygate"
	"renop/internal/service/status"
	"renop/internal/utils"
)

func updateMavenMetadataAfterVersionDelete(state *core.AppState, metadataPath, version string) error {
	if !state.Inner.FileIndex.HasFile(metadataPath) {
		return nil
	}
	return state.Inner.FileIndex.UpdateMetadataCallback(func() error {
		var (
			source io.ReadCloser
			err    error
		)
		if IsS3Enabled(metadataPath) {
			source, _, err = DownloadFromS3(utils.GetS3Key(metadataPath))
		} else {
			source, err = os.Open(metadataPath)
		}
		if err != nil {
			return err
		}
		var metadata config.Metadata
		decodeErr := xml.NewDecoder(source).Decode(&metadata)
		_ = source.Close()
		if decodeErr != nil {
			return decodeErr
		}
		versions := make([]string, 0)
		found := false
		if metadata.Versioning != nil && metadata.Versioning.Versions != nil {
			for _, candidate := range metadata.Versioning.Versions.Version {
				if candidate == version {
					found = true
				} else {
					versions = append(versions, candidate)
				}
			}
		}
		if !found {
			return nil
		}
		if len(versions) == 0 {
			if err := deleteIndexedFile(state, metadataPath); err != nil {
				return err
			}
			for _, suffix := range []string{".md5", ".sha1", ".sha256", ".sha512"} {
				if err := deleteIndexedFile(state, metadataPath+suffix); err != nil {
					return err
				}
			}
			return nil
		}
		metadata.Versioning.Versions.Version = versions
		sorted := append([]string(nil), versions...)
		slices.SortFunc(sorted, utils.CompareVersions)
		latest := sorted[len(sorted)-1]
		metadata.Versioning.Latest = &latest
		metadata.Versioning.Release = nil
		for index := len(sorted) - 1; index >= 0; index-- {
			candidate := sorted[index]
			if !strings.Contains(strings.ToUpper(candidate), "SNAPSHOT") {
				metadata.Versioning.Release = &candidate
				break
			}
		}
		lastUpdated := time.Now().UTC().Format("20060102150405")
		metadata.Versioning.LastUpdated = &lastUpdated
		updatedXML, err := xml.Marshal(metadata)
		if err != nil {
			return err
		}
		if IsS3Enabled(metadataPath) {
			err = UploadStreamToS3(utils.GetS3Key(metadataPath), bytes.NewReader(updatedXML), int64(len(updatedXML)), "application/xml")
		} else {
			temporary := metadataPath + ".tmp"
			err = os.WriteFile(temporary, updatedXML, 0644)
			if err == nil {
				err = utils.SafeRename(temporary, metadataPath)
			}
			if err != nil {
				_ = os.Remove(temporary)
			}
		}
		if err != nil {
			return err
		}
		state.InvalidateFileCache(metadataPath)
		for suffix, hash := range map[string]string{
			".md5": utils.MD5(updatedXML), ".sha1": utils.SHA1(updatedXML),
			".sha256": utils.SHA256(updatedXML), ".sha512": utils.SHA512(updatedXML),
		} {
			if err := SaveAndUploadChecksum(state, metadataPath, suffix, hash); err != nil {
				return err
			}
		}
		return nil
	})
}

// RemoveMavenVersion deletes one complete version directory and updates artifact metadata.
func RemoveMavenVersion(state *core.AppState, repository, groupID, artifactID, version string) error {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil {
		return core.ErrDatabaseUnavailable
	}
	releaseMutation := repositorygate.AcquireMutation(repository)
	defer releaseMutation()
	if state.GetDB() == nil {
		return core.ErrDatabaseUnavailable
	}
	if err := state.GetDB().EnsurePackageMutable(config.RepositoryFormatMaven, repository,
		groupID+":"+artifactID); err != nil {
		return err
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil || cfg.Maven.Repositories[repository] == nil ||
		cfg.Maven.Repositories[repository].NormalizedFormat() != config.RepositoryFormatMaven {
		return core.ErrMavenArtifactNotFound
	}
	groupPath := filepath.FromSlash(strings.ReplaceAll(groupID, ".", "/"))
	artifactDir := filepath.Join(cfg.StoragePath, repository, groupPath, artifactID)
	versionDir := filepath.Join(artifactDir, version)
	if !utils.IsSubPath(filepath.Join(cfg.StoragePath, repository), versionDir) {
		return core.ErrMavenVersionNotFound
	}
	gpgReleaseStorageMutation.Lock()
	defer gpgReleaseStorageMutation.Unlock()
	exists := state.Inner.FileIndex.HasDir(versionDir)
	if !exists && !IsS3Enabled(versionDir) {
		if info, err := os.Stat(versionDir); err == nil && info.IsDir() {
			exists = true
		}
	}
	if !exists {
		return core.ErrMavenVersionNotFound
	}
	if err := discardPendingGPGUploads(state, versionDir, "Maven version was deleted before publication"); err != nil {
		return err
	}
	if IsS3Enabled(versionDir) {
		prefix := utils.GetS3Key(versionDir)
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		if err := DeletePrefixFromS3(prefix); err != nil {
			return err
		}
	} else if err := os.RemoveAll(versionDir); err != nil {
		return err
	}
	state.Inner.FileIndex.RemoveDir(versionDir)
	if err := deleteGPGRecordsByLocalPrefix(state, repository, versionDir); err != nil {
		return err
	}
	if err := updateMavenMetadataAfterVersionDelete(state, filepath.Join(artifactDir, "maven-metadata.xml"), version); err != nil {
		return err
	}
	status.MarkStorageUpdated()
	return nil
}
