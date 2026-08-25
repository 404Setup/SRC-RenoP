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
	"path/filepath"
	"strings"
	"time"

	"renop/internal/core"
	"renop/internal/service/index"
	"renop/internal/utils"
)

func normalizeCatalogTimestamp(value int64) int64 {
	if value <= 0 {
		return time.Now().UnixMilli()
	}
	if value > 10_000_000_000_000 {
		return value / int64(time.Millisecond)
	}
	return value
}

// RecordPublishedPath updates the Maven catalog for one successfully published file.
func RecordPublishedPath(state *core.AppState, repository, path, username string, size, modTime int64) error {
	coordinate, ok := ParseArtifactPath(path)
	if !ok || state == nil || state.GetDB() == nil {
		return nil
	}
	domains, err := state.GetDB().ListMavenDomains(repository, username, true)
	if err != nil {
		return err
	}
	domain := matchingDomain(domains, strings.ToLower(coordinate.GroupID))
	if domain == nil || !domain.Verified {
		return core.ErrMavenDomainUnverified
	}
	timestamp := normalizeCatalogTimestamp(modTime)
	return state.GetDB().RecordMavenPublication(&core.MavenArtifact{
		Repository: repository, Domain: domain.Domain, GroupID: coordinate.GroupID,
		ArtifactID: coordinate.ArtifactID, Publisher: strings.ToLower(strings.TrimSpace(username)),
		LatestVersion: coordinate.Version, CreatedAt: timestamp, UpdatedAt: timestamp,
	}, &core.MavenVersion{
		Repository: repository, GroupID: coordinate.GroupID, ArtifactID: coordinate.ArtifactID,
		Version: coordinate.Version, Publisher: strings.ToLower(strings.TrimSpace(username)),
		Size: size, CreatedAt: timestamp,
	})
}

// ReconcileDomainCatalog derives missing catalog rows from the in-memory file index.
func ReconcileDomainCatalog(state *core.AppState, repository, domain, publisher string) error {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil {
		return core.ErrDatabaseUnavailable
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return core.ErrDatabaseUnavailable
	}
	root := filepath.Join(cfg.StoragePath, repository, filepath.FromSlash(strings.ReplaceAll(domain, ".", "/")))
	var reconcileErr error
	state.Inner.FileIndex.Walk(root, func(path string, info index.FileInfo, isDir bool) bool {
		if isDir {
			return true
		}
		relative, err := filepath.Rel(filepath.Join(cfg.StoragePath, repository), filepath.FromSlash(path))
		if err != nil || strings.HasPrefix(relative, "..") || !utils.IsSubPath(cfg.StoragePath, filepath.FromSlash(path)) {
			return true
		}
		if err := RecordPublishedPath(state, repository, filepath.ToSlash(relative), publisher, info.Size, info.ModTime); err != nil {
			reconcileErr = err
			return false
		}
		return true
	})
	return reconcileErr
}
