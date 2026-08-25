/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package api

type VersionsResponse struct {
	IsSnapshot bool     `json:"is_snapshot"`
	Versions   []string `json:"versions"`
}

type LatestVersionResponse struct {
	IsSnapshot bool   `json:"is_snapshot"`
	Version    string `json:"version"`
}

type PomDetails struct {
	GroupID    string `json:"group_id"`
	ArtifactID string `json:"artifact_id"`
	Version    string `json:"version"`
}

type VersionQuery struct {
	Filter *string `query:"filter"`
	Sorted *bool   `query:"sorted"`
}

type LatestVersionQuery struct {
	Filter  *string `query:"filter"`
	Sorted  *bool   `query:"sorted"`
	ResType *string `query:"type"`
}

type ArtifactDetailsQuery struct {
	Extension  *string `query:"extension"`
	Classifier *string `query:"classifier"`
	Filter     *string `query:"filter"`
}

type BadgeQuery struct {
	Name   *string `query:"name"`
	Color  *string `query:"color"`
	Prefix *string `query:"prefix"`
	Filter *string `query:"filter"`
}

type FileDetailsType string

const (
	FileDetailsTypeFile      FileDetailsType = "FILE"
	FileDetailsTypeDirectory FileDetailsType = "DIRECTORY"
)

type FileDetails struct {
	Type             FileDetailsType `json:"type"`
	Name             string          `json:"name"`
	ContentLength    *int64          `json:"content_length,omitempty"`
	ContentType      *string         `json:"content_type,omitempty"`
	LastModifiedTime *string         `json:"last_modified_time,omitempty"`
	Signed           bool            `json:"signed,omitempty"`
	Format           string          `json:"format,omitempty"`
	Files            []FileDetails   `json:"files,omitempty"`
}
