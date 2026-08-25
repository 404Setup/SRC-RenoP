/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import "errors"

const (
	CargoPermissionRead    = 0
	CargoPermissionPublish = 1
	CargoPermissionVersion = 2
	CargoPermissionManage  = 3
	CargoPermissionOwner   = 4
	CargoPermissionFull    = 4
)

var (
	ErrCargoPackageNotFound   = errors.New("cargo package was not found")
	ErrCargoVersionNotFound   = errors.New("cargo package version was not found")
	ErrCargoVersionExists     = errors.New("cargo package version already exists")
	ErrCargoPermissionDenied  = errors.New("cargo package permission denied")
	ErrCargoPackageArchived   = errors.New("cargo package is archived")
	ErrCargoAdminArchived     = errors.New("cargo package was archived by an administrator")
	ErrCargoAdminYanked       = errors.New("cargo package version was yanked by an administrator")
	ErrCargoLastFullMember    = errors.New("cargo package must retain at least one L4 owner")
	ErrCargoOwnerCannotLeave  = errors.New("cargo L4 owner must transfer ownership before leaving")
	ErrCargoMemberExists      = errors.New("cargo package member already exists")
	ErrCargoInvitationExists  = errors.New("cargo package invitation is already pending")
	ErrCargoInvitationInvalid = errors.New("cargo package invitation is no longer valid")
)

// CargoDependency represents one dependency requirement in an index entry or package detail.
type CargoDependency struct {
	Name            string   `json:"name"`
	Requirement     string   `json:"req"`
	Features        []string `json:"features,omitempty"`
	Optional        bool     `json:"optional"`
	DefaultFeatures bool     `json:"default_features"`
	Target          *string  `json:"target,omitempty"`
	Kind            string   `json:"kind"`
	Registry        *string  `json:"registry,omitempty"`
	Package         *string  `json:"package,omitempty"`
}

// CargoPackage is durable registry metadata for a local or mirrored crate.
type CargoPackage struct {
	Repository      string `json:"repository"`
	Name            string `json:"name"`
	NormalizedName  string `json:"-"`
	Description     string `json:"description"`
	RepositoryURL   string `json:"repository_url,omitempty"`
	Homepage        string `json:"homepage,omitempty"`
	Documentation   string `json:"documentation,omitempty"`
	Archived        bool   `json:"archived"`
	AdminArchived   bool   `json:"admin_archived"`
	Mirrored        bool   `json:"mirrored"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
	PermissionLevel int    `json:"permission_level,omitempty"`
	MaxVersion      string `json:"-"`
}

// CargoVersion tracks local or mirrored versions and the origin of yank state.
type CargoVersion struct {
	Features      map[string][]string `json:"features,omitempty"`
	Links         *string             `json:"links,omitempty"`
	Repository    string              `json:"-"`
	Package       string              `json:"-"`
	Version       string              `json:"version"`
	Description   string              `json:"description,omitempty"`
	Publisher     string              `json:"publisher"`
	Checksum      string              `json:"checksum,omitempty"`
	RustVersion   string              `json:"rust_version,omitempty"`
	License       string              `json:"license,omitempty"`
	Documentation string              `json:"documentation,omitempty"`
	Homepage      string              `json:"homepage,omitempty"`
	RepositoryURL string              `json:"repository_url,omitempty"`
	Deps          []CargoDependency   `json:"deps,omitempty"`
	CreatedAt     int64               `json:"created_at"`
	Size          int64               `json:"size,omitempty"`
	Yanked        bool                `json:"yanked"`
	AdminYanked   bool                `json:"admin_yanked"`
	ArchiveYanked bool                `json:"-"`
	HasDocs       bool                `json:"has_docs,omitempty"`
	Mirrored      bool                `json:"mirrored"`
}

// CargoMember is one package-team membership.
type CargoMember struct {
	UserID   string `json:"user_id"`
	Username string `json:"login"`
	Level    int    `json:"level"`
	AddedAt  int64  `json:"added_at"`
}

// CargoPackageDetails combines a package with its versions and team.
type CargoPackageDetails struct {
	Package  *CargoPackage   `json:"package"`
	Versions []*CargoVersion `json:"versions"`
	Members  []*CargoMember  `json:"members"`
}

// CargoInvitation is stored until its matching message-center action is
// accepted or rejected.
type CargoInvitation struct {
	ID             string
	Repository     string
	Package        string
	NormalizedName string
	Inviter        string
	Recipient      string
	Level          int
	CreatedAt      int64
}
