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
	MavenPermissionRead    = 0
	MavenPermissionPublish = 1
	MavenPermissionVersion = 2
	MavenPermissionManage  = 3
	MavenPermissionOwner   = 4
	MavenPermissionFull    = 4
)

const (
	MavenVerificationDNS    = "dns"
	MavenVerificationGitHub = "github"
	MavenVerificationGitLab = "gitlab"
)

var (
	ErrMavenDomainNotFound        = errors.New("maven domain was not found")
	ErrMavenDomainExists          = errors.New("maven domain already exists")
	ErrMavenDomainNotEmpty        = errors.New("maven domain still contains artifacts")
	ErrMavenDomainUnverified      = errors.New("maven domain has not been verified")
	ErrMavenVerificationFailed    = errors.New("maven domain verification failed")
	ErrMavenVerificationRateLimit = errors.New("maven domain verification is rate limited")
	ErrMavenPermissionDenied      = errors.New("maven domain permission denied")
	ErrMavenArtifactNotFound      = errors.New("maven artifact was not found")
	ErrMavenVersionNotFound       = errors.New("maven artifact version was not found")
	ErrMavenLastFullMember        = errors.New("maven domain must retain one L4 owner")
	ErrMavenOwnerCannotLeave      = errors.New("maven L4 owner must transfer ownership before leaving")
	ErrMavenMemberExists          = errors.New("maven domain member already exists")
	ErrMavenInvitationExists      = errors.New("maven domain invitation is already pending")
	ErrMavenInvitationInvalid     = errors.New("maven domain invitation is no longer valid")
)

// MavenDomain is one verified or pending Maven publishing namespace.
type MavenDomain struct {
	Repository       string `json:"-"`
	Domain           string `json:"domain"`
	VerificationType string `json:"verification_type"`
	VerificationHost string `json:"verification_host"`
	VerificationCode string `json:"verification_code,omitempty"`
	CreatedAt        int64  `json:"created_at"`
	VerifiedAt       int64  `json:"verified_at,omitempty"`
	LastCheckAt      int64  `json:"last_check_at,omitempty"`
	PermissionLevel  int    `json:"permission_level,omitempty"`
	ArtifactCount    int    `json:"artifact_count"`
	Verified         bool   `json:"verified"`
	Member           bool   `json:"member,omitempty"`
}

// MavenMember is one domain-team membership.
type MavenMember struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Level    int    `json:"level"`
	AddedAt  int64  `json:"added_at"`
}

// MavenDomainDetails combines a publishing domain with its team.
type MavenDomainDetails struct {
	Domain        *MavenDomain   `json:"domain"`
	Members       []*MavenMember `json:"members,omitempty"`
	Administrator bool           `json:"administrator"`
}

// MavenArtifact is durable catalog metadata for one groupId and artifactId.
type MavenArtifact struct {
	Repository      string `json:"repository"`
	Domain          string `json:"domain"`
	GroupID         string `json:"group_id"`
	ArtifactID      string `json:"artifact_id"`
	Description     string `json:"description,omitempty"`
	Publisher       string `json:"publisher,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
	VersionCount    int    `json:"version_count"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
	PermissionLevel int    `json:"permission_level,omitempty"`
}

// MavenVersion is one published version in the Maven catalog.
type MavenVersion struct {
	Repository string `json:"-"`
	GroupID    string `json:"-"`
	ArtifactID string `json:"-"`
	Version    string `json:"version"`
	Publisher  string `json:"publisher,omitempty"`
	Size       int64  `json:"size"`
	CreatedAt  int64  `json:"created_at"`
}

// MavenArtifactDetails combines an artifact with all indexed versions.
type MavenArtifactDetails struct {
	Artifact      *MavenArtifact  `json:"artifact"`
	Versions      []*MavenVersion `json:"versions"`
	Administrator bool            `json:"administrator"`
}

// MavenInvitation is stored until its matching message action is completed.
type MavenInvitation struct {
	ID         string
	Repository string
	Domain     string
	Inviter    string
	Recipient  string
	Level      int
	CreatedAt  int64
}
