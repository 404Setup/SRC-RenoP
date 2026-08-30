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
	"net/url"
	"strings"

	"renop/internal/config"
)

// ArtifactURL resolves a Cargo download path against a mirror. It is kept in
// this package so proxying code does not need to know Cargo path semantics.
func ArtifactURL(repo *config.Repository, mirror config.Mirror, path string) string {
	encodedPath := EscapePath(path)
	if repo != nil && repo.NormalizedFormat() == config.RepositoryFormatCargo && strings.TrimSpace(mirror.ArtifactURL) != "" {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) == 6 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "crates" && parts[5] == "download" {
			if err := mirror.ValidateArtifactURL(config.RepositoryFormatCargo); err == nil {
				return strings.NewReplacer(
					"{crate}", url.PathEscape(parts[3]),
					"{version}", url.PathEscape(parts[4]),
				).Replace(strings.TrimSpace(mirror.ArtifactURL))
			}
		}
	}
	base := strings.TrimRight(strings.TrimSpace(mirror.URL), "/")
	if base == "" {
		return ""
	}
	return base + "/" + encodedPath
}

// EscapePath applies Cargo-safe URL path escaping while preserving path
// separators for upstream registry requests.
func EscapePath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := range parts {
		parts[i] = strings.ReplaceAll(url.PathEscape(parts[i]), "%2B", "+")
	}
	return strings.Join(parts, "/")
}

// ResponseLimit bounds mirror responses by Cargo path type. Sparse index
// entries stay small while crate downloads retain the package size budget.
func ResponseLimit(path string) int64 {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 6 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "crates" && parts[5] == "download" {
		return maxCrateSize
	}
	return maxIndexSize
}
