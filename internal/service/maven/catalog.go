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

	"renop/internal/config"
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

func isMirroredMavenCompanion(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	for _, suffix := range []string{".md5", ".sha1", ".sha256", ".sha512", ".asc"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// RecordPublishedPath updates the Maven catalog for one successfully published file.
func RecordPublishedPath(state *core.AppState, repository, path, username string, size, modTime int64) error {
	coordinate, ok := ParseArtifactPath(path)
	if !ok || state == nil || state.GetDB() == nil {
		return nil
	}
	domains, err := state.GetDB().ListMavenDomains(username, true)
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

// RecordMirroredPath catalogs one Maven version fetched through a configured upstream mirror.
func RecordMirroredPath(state *core.AppState, repository, path string, size, modTime int64) error {
	if isMirroredMavenCompanion(path) {
		return nil
	}
	coordinate, ok := ParseArtifactPath(path)
	if !ok || state == nil || state.GetDB() == nil {
		return nil
	}
	domains, err := state.GetDB().ListMavenDomains("", true)
	if err != nil {
		return err
	}
	domainName := ""
	for _, domain := range domains {
		if domain == nil || !domain.Verified || !domainContainsGroup(domain.Domain, strings.ToLower(coordinate.GroupID)) {
			continue
		}
		if len(domain.Domain) > len(domainName) {
			domainName = domain.Domain
		}
	}
	if domainName == "" {
		var inferred bool
		domainName, inferred = legacyDomainForGroup(coordinate.GroupID)
		if !inferred {
			return nil
		}
		timestamp := normalizeCatalogTimestamp(modTime)
		if err := state.GetDB().EnsureMirroredMavenDomain(domainName, timestamp); err != nil {
			return err
		}
	}
	timestamp := normalizeCatalogTimestamp(modTime)
	return state.GetDB().RecordMavenMirrorPublication(&core.MavenArtifact{
		Repository: repository, Domain: domainName, GroupID: coordinate.GroupID,
		ArtifactID: coordinate.ArtifactID, LatestVersion: coordinate.Version,
		CreatedAt: timestamp, UpdatedAt: timestamp, Mirrored: true,
	}, &core.MavenVersion{
		Repository: repository, GroupID: coordinate.GroupID, ArtifactID: coordinate.ArtifactID,
		Version: coordinate.Version, Size: size, CreatedAt: timestamp, Mirrored: true,
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

// ReconcileGlobalDomainCatalog derives catalog rows for a domain in every Maven repository.
func ReconcileGlobalDomainCatalog(state *core.AppState, domain, publisher string) error {
	if state == nil || state.Inner == nil {
		return core.ErrDatabaseUnavailable
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return core.ErrDatabaseUnavailable
	}
	for repository, repo := range cfg.Maven.Repositories {
		if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatMaven {
			continue
		}
		if err := ReconcileDomainCatalog(state, repository, domain, publisher); err != nil {
			return err
		}
	}
	return nil
}
