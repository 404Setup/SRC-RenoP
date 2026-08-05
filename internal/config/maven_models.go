/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package config

type Plugin struct {
	Name       *string `json:"name,omitempty" xml:"name,omitempty"`
	Prefix     *string `json:"prefix,omitempty" xml:"prefix,omitempty"`
	ArtifactId *string `json:"artifact_id,omitempty" xml:"artifactId,omitempty"`
}

type Plugins struct {
	Plugin []Plugin `json:"plugin,omitempty" xml:"plugin,omitempty"`
}

type SnapshotVersion struct {
	Extension *string `json:"extension,omitempty" xml:"extension,omitempty"`
	Value     *string `json:"value,omitempty" xml:"value,omitempty"`
	Updated   *string `json:"updated,omitempty" xml:"updated,omitempty"`
}

type Snapshot struct {
	Timestamp   *string `json:"timestamp,omitempty" xml:"timestamp,omitempty"`
	BuildNumber *int32  `json:"build_number,omitempty" xml:"buildNumber,omitempty"`
	LocalCopy   *bool   `json:"local_copy,omitempty" xml:"localCopy,omitempty"`
}

type Versions struct {
	Version []string `json:"version,omitempty" xml:"version,omitempty"`
}

type SnapshotVersions struct {
	SnapshotVersion []SnapshotVersion `json:"snapshot_version,omitempty" xml:"snapshotVersion,omitempty"`
}

type Versioning struct {
	Release          *string           `json:"release,omitempty" xml:"release,omitempty"`
	Latest           *string           `json:"latest,omitempty" xml:"latest,omitempty"`
	LastUpdated      *string           `json:"last_updated,omitempty" xml:"lastUpdated,omitempty"`
	Snapshot         *Snapshot         `json:"snapshot,omitempty" xml:"snapshot,omitempty"`
	Versions         *Versions         `json:"versions,omitempty" xml:"versions,omitempty"`
	SnapshotVersions *SnapshotVersions `json:"snapshot_versions,omitempty" xml:"snapshotVersions,omitempty"`
}

type Metadata struct {
	GroupId    *string     `json:"group_id,omitempty" xml:"groupId,omitempty"`
	ArtifactId *string     `json:"artifact_id,omitempty" xml:"artifactId,omitempty"`
	Version    *string     `json:"version,omitempty" xml:"version,omitempty"`
	Versioning *Versioning `json:"versioning,omitempty" xml:"versioning,omitempty"`
	Plugins    *Plugins    `json:"plugins,omitempty" xml:"plugins,omitempty"`
}
