/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package cargo implements the HTTP protocol pieces specific to a Cargo
// sparse registry. It deliberately knows nothing about the filesystem or S3;
// those concerns are supplied through Store.
package cargo

import "renop/internal/core"

// RegistryConfig is the sparse-index config.json payload.
type RegistryConfig struct {
	DownloadURL string `json:"dl"`
	APIURL      string `json:"api"`
	AuthNeeded  bool   `json:"auth-required,omitempty"`
}

type PublishMetadata struct {
	Name        string              `json:"name"`
	Version     string              `json:"vers"`
	Description string              `json:"description"`
	Deps        []PublishDependency `json:"deps"`
	Features    map[string][]string `json:"features"`
	Features2   map[string][]string `json:"features2,omitempty"`
	Links       *string             `json:"links"`
	RustVersion *string             `json:"rust_version,omitempty"`
}

// PublishDependency follows Cargo's web API representation. It is converted
// to IndexDependency before being persisted because the field names differ.
type PublishDependency struct {
	Name               string   `json:"name"`
	VersionReq         string   `json:"version_req"`
	Features           []string `json:"features"`
	Optional           bool     `json:"optional"`
	DefaultFeatures    *bool    `json:"default_features"`
	Target             *string  `json:"target"`
	Kind               string   `json:"kind"`
	Registry           *string  `json:"registry"`
	ExplicitNameInToml *string  `json:"explicit_name_in_toml"`
}

type IndexDependency struct {
	Name            string   `json:"name"`
	Requirement     string   `json:"req"`
	Features        []string `json:"features"`
	Optional        bool     `json:"optional"`
	DefaultFeatures bool     `json:"default_features"`
	Target          *string  `json:"target"`
	Kind            string   `json:"kind"`
	Registry        *string  `json:"registry"`
	Package         *string  `json:"package"`
}

type IndexEntry struct {
	Name        string              `json:"name"`
	Version     string              `json:"vers"`
	Deps        []IndexDependency   `json:"deps"`
	Checksum    string              `json:"cksum"`
	Features    map[string][]string `json:"features"`
	Yanked      bool                `json:"yanked"`
	Links       *string             `json:"links"`
	Features2   map[string][]string `json:"features2,omitempty"`
	RustVersion *string             `json:"rust_version,omitempty"`
	Schema      int                 `json:"v,omitempty"`
}

type ErrorDetail struct {
	Detail string `json:"detail"`
}

type ErrorResponse struct {
	Errors []ErrorDetail `json:"errors"`
}

type Warnings struct {
	InvalidCategories []string `json:"invalid_categories"`
	InvalidBadges     []string `json:"invalid_badges"`
	Other             []string `json:"other"`
}

type PublishResponse struct {
	Warnings Warnings `json:"warnings"`
}

type OperationResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"msg,omitempty"`
}

type ownerRequest struct {
	Users []string `json:"users"`
	Level int      `json:"level,omitempty"`
}

type ownerResponse struct {
	Users []owner `json:"users"`
}

type owner struct {
	ID     uint32 `json:"id"`
	UserID string `json:"user_id"`
	Login  string `json:"login"`
	Name   string `json:"name,omitempty"`
	Level  int    `json:"level"`
}

type memberLevelRequest struct {
	Level int `json:"level"`
}

type searchResponse struct {
	Crates []searchCrate `json:"crates"`
	Meta   searchMeta    `json:"meta"`
}

type searchCrate struct {
	Name        string `json:"name"`
	MaxVersion  string `json:"max_version"`
	Description string `json:"description"`
	Mirrored    bool   `json:"mirrored"`
}

type searchMeta struct {
	Total int `json:"total"`
}

type invitationPayload struct {
	Repository string `json:"repository"`
	Package    string `json:"package"`
	Inviter    string `json:"inviter"`
	Level      int    `json:"level"`
}

type packageListResponse struct {
	Packages []*core.CargoPackage `json:"packages"`
	Admin    bool                 `json:"administrator"`
}

type packageInfoResponse struct {
	*core.CargoPackageDetails
	Admin bool `json:"administrator"`
}
