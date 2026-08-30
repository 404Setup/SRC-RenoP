/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
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
