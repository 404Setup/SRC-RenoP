/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package core

import "errors"

const (
	NPMPermissionRead      = 0
	NPMPermissionPublish   = 1
	NPMPermissionLifecycle = 2
	NPMPermissionTeam      = 3
	NPMPermissionOwner     = 4
	NPMPermissionFull      = 4
)

var (
	ErrNPMPackageNotFound   = errors.New("npm package was not found")
	ErrNPMPackageExists     = errors.New("npm package already exists")
	ErrNPMVersionNotFound   = errors.New("npm package version was not found")
	ErrNPMVersionExists     = errors.New("npm package version already exists")
	ErrNPMPermissionDenied  = errors.New("npm package permission denied")
	ErrNPMPackageArchived   = errors.New("npm package is archived")
	ErrNPMPackageMirrored   = errors.New("mirrored npm package is pull-only")
	ErrNPMPackageLimit      = errors.New("npm package metadata limit reached")
	ErrNPMMemberExists      = errors.New("npm package member already exists")
	ErrNPMInvitationExists  = errors.New("npm package invitation is already pending")
	ErrNPMInvitationInvalid = errors.New("npm package invitation is no longer valid")
	ErrNPMLastFullMember    = errors.New("npm package must retain at least one L4 owner")
	ErrNPMOwnerCannotLeave  = errors.New("npm L4 owner must transfer ownership before leaving")
	ErrNPMRevisionConflict  = errors.New("npm package revision is stale")
)

// NPMPackage is durable metadata for a reserved local or mirrored npm package.
type NPMPackage struct {
	Repository      string `json:"repository"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Publisher       string `json:"publisher"`
	LatestVersion   string `json:"latest_version"`
	VersionCount    int    `json:"version_count"`
	Private         bool   `json:"private"`
	Archived        bool   `json:"archived"`
	Mirrored        bool   `json:"mirrored"`
	PublishEnabled  bool   `json:"publish_enabled"`
	SuperTeamPrefix string `json:"super_team_prefix,omitempty"`
	PermissionLevel int    `json:"permission_level,omitempty"`
	Revision        int64  `json:"-"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

// NPMVersion stores one immutable npm version and its canonical tarball metadata.
type NPMVersion struct {
	Repository   string `json:"-"`
	Package      string `json:"-"`
	Version      string `json:"version"`
	ManifestJSON string `json:"-"`
	Publisher    string `json:"publisher"`
	TarballPath  string `json:"-"`
	Shasum       string `json:"shasum"`
	Integrity    string `json:"integrity"`
	Size         int64  `json:"size"`
	Deprecated   string `json:"deprecated,omitempty"`
	Unpublished  bool   `json:"unpublished"`
	Mirrored     bool   `json:"mirrored"`
	CreatedAt    int64  `json:"created_at"`
}

// NPMMember is one npm package-team membership.
type NPMMember struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Level    int    `json:"level"`
	AddedAt  int64  `json:"added_at"`
}

// NPMProjectPerson is one bounded author or contributor identity from package metadata.
type NPMProjectPerson struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// NPMProjectMetadata contains bounded presentation metadata from the selected npm version.
type NPMProjectMetadata struct {
	Readme         string             `json:"readme,omitempty"`
	ReadmeFilename string             `json:"readme_filename,omitempty"`
	License        string             `json:"license,omitempty"`
	Homepage       string             `json:"homepage,omitempty"`
	Repository     string             `json:"repository,omitempty"`
	Bugs           string             `json:"bugs,omitempty"`
	Author         *NPMProjectPerson  `json:"author,omitempty"`
	Contributors   []NPMProjectPerson `json:"contributors,omitempty"`
	Maintainers    []NPMProjectPerson `json:"maintainers,omitempty"`
	Funding        []string           `json:"funding,omitempty"`
	Keywords       []string           `json:"keywords,omitempty"`
	NodeEngine     string             `json:"node_engine,omitempty"`
	PackageManager string             `json:"package_manager,omitempty"`
}

// NPMInvitation is a pending npm package-team invitation.
type NPMInvitation struct {
	ID         string
	Repository string
	Package    string
	Inviter    string
	Recipient  string
	Level      int
	CreatedAt  int64
}

// NPMPackageDetails combines package, version, tag, and visible team metadata.
type NPMPackageDetails struct {
	Package       *NPMPackage         `json:"package"`
	Versions      []*NPMVersion       `json:"versions"`
	DistTags      map[string]string   `json:"dist_tags"`
	Members       []*NPMMember        `json:"members,omitempty"`
	Project       *NPMProjectMetadata `json:"project,omitempty"`
	MemberCount   int                 `json:"member_count"`
	Member        bool                `json:"member"`
	Administrator bool                `json:"administrator"`
}
