/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"log"
	"path/filepath"
	"strings"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/cargo"
	"renop/internal/service/proxy"
	"renop/internal/utils"
)

func init() {
	proxy.OnMirrorArtifactStored = recordMirroredArtifact
}

func recordMirroredArtifact(state *core.AppState, repo *config.Repository, localPath string, size, modTime int64) {
	if state == nil || state.Inner == nil || repo == nil {
		return
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return
	}
	repositoryRoot := filepath.Join(cfg.StoragePath, repo.Name)
	relative, err := filepath.Rel(repositoryRoot, localPath)
	if err != nil || strings.HasPrefix(relative, "..") || !utils.IsSubPath(repositoryRoot, localPath) {
		return
	}
	path := filepath.ToSlash(relative)
	switch repo.NormalizedFormat() {
	case config.RepositoryFormatCargo:
		err = cargo.RecordMirroredPath(state, repo, path, size, modTime)
	case config.RepositoryFormatMaven:
		if MavenMirrorRecorder != nil {
			err = MavenMirrorRecorder(state, repo.Name, path, size, modTime)
		}
	}
	if err != nil {
		log.Printf("failed to record mirrored artifact %s/%s: %v", repo.Name, path, err)
	}
}
