/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import "errors"

const (
	CargoPermissionPublish = 1
	CargoPermissionVersion = 2
	CargoPermissionFull    = 3
)

var (
	ErrCargoPackageNotFound   = errors.New("Cargo package was not found")
	ErrCargoVersionNotFound   = errors.New("Cargo package version was not found")
	ErrCargoVersionExists     = errors.New("Cargo package version already exists")
	ErrCargoPermissionDenied  = errors.New("Cargo package permission denied")
	ErrCargoPackageArchived   = errors.New("Cargo package is archived")
	ErrCargoAdminArchived     = errors.New("Cargo package was archived by an administrator")
	ErrCargoAdminYanked       = errors.New("Cargo package version was yanked by an administrator")
	ErrCargoLastFullMember    = errors.New("Cargo package must retain at least one L3 member")
	ErrCargoMemberExists      = errors.New("Cargo package member already exists")
	ErrCargoInvitationExists  = errors.New("Cargo package invitation is already pending")
	ErrCargoInvitationInvalid = errors.New("Cargo package invitation is no longer valid")
)

// CargoPackage is durable registry metadata for a locally owned crate.
type CargoPackage struct {
	Repository      string `json:"repository"`
	Name            string `json:"name"`
	NormalizedName  string `json:"-"`
	Description     string `json:"description"`
	Archived        bool   `json:"archived"`
	AdminArchived   bool   `json:"admin_archived"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
	PermissionLevel int    `json:"permission_level,omitempty"`
	MaxVersion      string `json:"-"`
}

// CargoVersion tracks local versions and the origin of yank state.
type CargoVersion struct {
	Repository    string `json:"-"`
	Package       string `json:"-"`
	Version       string `json:"version"`
	Description   string `json:"description,omitempty"`
	Publisher     string `json:"publisher"`
	Yanked        bool   `json:"yanked"`
	AdminYanked   bool   `json:"admin_yanked"`
	ArchiveYanked bool   `json:"-"`
	CreatedAt     int64  `json:"created_at"`
}

// CargoMember is one package-team membership.
type CargoMember struct {
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
