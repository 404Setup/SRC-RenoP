/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
)

const legacyMavenVerificationType = "legacy"

var legacyUpgradeMutex sync.Mutex

func legacyDomainForGroup(groupID string) (string, bool) {
	groupID = strings.ToLower(strings.TrimSpace(groupID))
	parts := strings.Split(groupID, ".")
	if len(parts) < 2 {
		return "", false
	}
	verificationType, target, err := VerificationTarget(groupID)
	if err == nil {
		switch verificationType {
		case core.MavenVerificationGitHub, core.MavenVerificationGitLab:
			return strings.Join(parts[:3], "."), true
		case core.MavenVerificationDNS:
			hostParts := strings.Split(target, ".")
			slices.Reverse(hostParts)
			return strings.Join(hostParts, "."), true
		}
	}
	fallback := strings.Join(parts[:2], ".")
	if _, normalizeErr := NormalizeDomain(fallback); normalizeErr != nil {
		return "", false
	}
	return fallback, true
}

func importLegacyMavenPath(state *core.AppState, repository, path string, info index.FileInfo, importedAt int64) error {
	coordinate, ok := ParseArtifactPath(path)
	if !ok {
		return nil
	}
	domainName, ok := legacyDomainForGroup(coordinate.GroupID)
	if !ok {
		return nil
	}
	domain := &core.MavenDomain{
		Repository: repository, Domain: domainName, VerificationType: legacyMavenVerificationType,
		VerificationHost: domainName, VerificationCode: "renop-legacy-import",
		Verified: true, CreatedAt: importedAt, VerifiedAt: importedAt,
	}
	if err := state.GetDB().EnsureImportedMavenDomain(domain); err != nil {
		return err
	}
	timestamp := normalizeCatalogTimestamp(info.ModTime)
	return state.GetDB().RecordMavenPublication(&core.MavenArtifact{
		Repository: repository, Domain: domainName, GroupID: coordinate.GroupID,
		ArtifactID: coordinate.ArtifactID, LatestVersion: coordinate.Version,
		CreatedAt: timestamp, UpdatedAt: timestamp,
	}, &core.MavenVersion{
		Repository: repository, GroupID: coordinate.GroupID, ArtifactID: coordinate.ArtifactID,
		Version: coordinate.Version, Size: info.Size, CreatedAt: timestamp,
	})
}

// UpgradeLegacyRepository catalogs files from a pre-domain Maven repository once.
func UpgradeLegacyRepository(state *core.AppState, repository string) error {
	if state == nil || state.Inner == nil || state.GetDB() == nil || state.Inner.FileIndex == nil {
		return core.ErrDatabaseUnavailable
	}
	legacyUpgradeMutex.Lock()
	defer legacyUpgradeMutex.Unlock()
	completed, err := state.GetDB().IsMavenRepositoryUpgraded(repository)
	if err != nil || completed {
		return err
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return core.ErrDatabaseUnavailable
	}
	repo := cfg.Maven.Repositories[repository]
	if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatMaven {
		return nil
	}
	root := filepath.Join(cfg.StoragePath, repository)
	importedAt := time.Now().UnixMilli()
	if repo.S3 != nil && repo.S3.Enabled {
		if !state.Inner.FileIndex.HasDir(root) {
			return nil
		}
		var importErr error
		state.Inner.FileIndex.Walk(root, func(path string, info index.FileInfo, isDir bool) bool {
			if isDir {
				return true
			}
			relative, relErr := filepath.Rel(root, filepath.FromSlash(path))
			if relErr != nil || strings.HasPrefix(relative, "..") {
				return true
			}
			if err := importLegacyMavenPath(state, repository, filepath.ToSlash(relative), info, importedAt); err != nil {
				importErr = err
				return false
			}
			return true
		})
		if importErr != nil {
			return importErr
		}
	} else {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if errors.Is(walkErr, os.ErrNotExist) {
					return nil
				}
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil || strings.HasPrefix(relative, "..") {
				return relErr
			}
			return importLegacyMavenPath(state, repository, filepath.ToSlash(relative), index.FileInfo{
				Size: info.Size(), ModTime: info.ModTime().UnixNano(),
			}, importedAt)
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return state.GetDB().MarkMavenRepositoryUpgraded(repository, importedAt)
}
