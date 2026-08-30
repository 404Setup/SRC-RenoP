/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"path/filepath"
	"strings"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/publicationquota"
)

func publicationQuotaRelativePath(state *core.AppState, repo *config.Repository, absolutePath string) (string, error) {
	if state == nil || state.Inner == nil || repo == nil {
		return "", core.ErrPublicationQuotaInvalid
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return "", core.ErrDatabaseUnavailable
	}
	relative, err := filepath.Rel(filepath.Join(cfg.StoragePath, repo.Name), absolutePath)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", core.ErrPublicationQuotaInvalid
	}
	return filepath.ToSlash(relative), nil
}

func reserveUploadedFileQuota(state *core.AppState, repo *config.Repository,
	upload *PreparedUpload,
) (*publicationquota.Reservation, error) {
	if upload == nil || upload.FileSize < 0 {
		return nil, core.ErrPublicationQuotaInvalid
	}
	if strings.TrimSpace(upload.Username) == "" {
		return publicationquota.Unmetered(state), nil
	}
	relative, err := publicationQuotaRelativePath(state, repo, upload.LocalFilePath)
	if err != nil {
		return nil, err
	}
	teamPrefix := ""
	publications := int64(1)
	if repo.NormalizedFormat() == config.RepositoryFormatMaven {
		publications = 0
		if strings.HasSuffix(strings.ToLower(filepath.Base(relative)), ".pom") {
			publications = 1
		}
		if MavenPublicationQuotaOwner != nil {
			teamPrefix, err = MavenPublicationQuotaOwner(state, upload.Username, repo, relative)
			if err != nil {
				return nil, err
			}
		}
	}
	return publicationquota.Reserve(state, upload.Username, teamPrefix, core.PublicationQuotaDelta{
		Files: 1, Bytes: upload.FileSize, Publications: publications,
	})
}
