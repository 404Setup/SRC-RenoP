/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"strings"
	"time"

	"renop/internal/config"
	"renop/internal/core"
)

// RecordMirroredPath catalogs a Cargo crate archive fetched through a configured mirror.
func RecordMirroredPath(state *core.AppState, repo *config.Repository, path string, size, modTime int64) error {
	if state == nil || state.GetDB() == nil || repo == nil || repo.NormalizedFormat() != config.RepositoryFormatCargo {
		return nil
	}
	parts := strings.Split(strings.Trim(strings.ReplaceAll(path, `\`, "/"), "/"), "/")
	if len(parts) != 6 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "crates" || parts[5] != "download" {
		return nil
	}
	packageName, versionName := parts[3], parts[4]
	if err := validatePackage(packageName, versionName); err != nil {
		return nil
	}
	timestamp := modTime
	if timestamp <= 0 {
		timestamp = time.Now().UnixMilli()
	}
	if timestamp > 10_000_000_000_000 {
		timestamp /= int64(time.Millisecond)
	}
	return state.GetDB().RecordCargoMirrorPublication(&core.CargoPackage{
		Repository: repo.Name, Name: packageName, NormalizedName: normalizeCrateName(packageName),
		CreatedAt: timestamp, UpdatedAt: timestamp, Mirrored: true,
	}, &core.CargoVersion{
		Repository: repo.Name, Package: normalizeCrateName(packageName), Version: versionName,
		Size: size, CreatedAt: timestamp, Mirrored: true,
	})
}
