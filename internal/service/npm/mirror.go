/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"renop/internal/config"
	"renop/internal/core"
)

// RecordMirroredPath updates catalog size after a mirrored npm tarball is persisted.
func RecordMirroredPath(state *core.AppState, repo *config.Repository, path string, size int64) error {
	if state == nil || state.GetDB() == nil || repo == nil || repo.NormalizedFormat() != config.RepositoryFormatNPM {
		return nil
	}
	if _, _, ok := ClassifyTarballPath(path); !ok {
		return nil
	}
	return state.GetDB().UpdateNPMTarballSize(repo.Name, path, max(size, 0))
}
