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

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	SuperTeamRoleRead   = 1
	SuperTeamRoleWrite  = 2
	SuperTeamRoleManage = 3
	SuperTeamRoleOwner  = 4

	MinSuperTeamPrefixLength = 2
	MaxSuperTeamPrefixLength = 64
	MaxSuperTeamNameRunes    = 80
	MaxSuperTeamDescription  = 512
)

var (
	ErrSuperTeamNotFound          = errors.New("global team was not found")
	ErrSuperTeamExists            = errors.New("global team prefix already exists")
	ErrSuperTeamPermissionDenied  = errors.New("global team permission denied")
	ErrSuperTeamMemberExists      = errors.New("global team member already exists")
	ErrSuperTeamInvitationExists  = errors.New("global team invitation is already pending")
	ErrSuperTeamInvitationInvalid = errors.New("global team invitation is no longer valid")
	ErrSuperTeamLastOwner         = errors.New("global team must retain at least one T4 owner")
	ErrSuperTeamOwnerCannotLeave  = errors.New("global team T4 owner must transfer ownership before leaving")
	ErrSuperTeamCreateLimit       = errors.New("global team creation limit reached")
	ErrSuperTeamJoinLimit         = errors.New("global team membership limit reached")
	ErrSuperTeamBindingRequired   = errors.New("a global team is required for this package namespace")
	ErrSuperTeamBindingMismatch   = errors.New("package namespace does not match the global team prefix")
	ErrSuperTeamBindingPermission = errors.New("T3 or T4 global team permission is required")
	ErrSuperTeamNotEmpty          = errors.New("global team still owns packages or publishing domains")
)

// SuperTeam is a global, engine-independent package publishing team.
type SuperTeam struct {
	Prefix      string `json:"prefix"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedBy   string `json:"created_by"`
	RoleLevel   int    `json:"role_level"`
	MemberCount int    `json:"member_count"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// SuperTeamMember is one immutable-user membership in a global team.
type SuperTeamMember struct {
	UserID   string `json:"-"`
	Username string `json:"username"`
	Level    int    `json:"level"`
	AddedAt  int64  `json:"added_at"`
}

// SuperTeamDetails combines one team with the members visible to its managers.
type SuperTeamDetails struct {
	Team          *SuperTeam         `json:"team"`
	Members       []*SuperTeamMember `json:"members"`
	Administrator bool               `json:"administrator"`
}

// SuperTeamInvitation is a pending invitation to a global team.
type SuperTeamInvitation struct {
	ID         string
	TeamPrefix string
	Inviter    string
	Recipient  string
	Level      int
	CreatedAt  int64
	ExpiresAt  int64
}

// SuperTeamLimitStatus reports inherited or account-specific team limits and current usage.
type SuperTeamLimitStatus struct {
	CreateLimit          int  `json:"create_limit"`
	JoinLimit            int  `json:"join_limit"`
	CreatedCount         int  `json:"created_count"`
	JoinedCount          int  `json:"joined_count"`
	CreateLimitInherited bool `json:"create_limit_inherited"`
	JoinLimitInherited   bool `json:"join_limit_inherited"`
}

// NormalizeSuperTeamPrefix validates the immutable cross-engine namespace prefix.
func NormalizeSuperTeamPrefix(prefix string) (string, bool) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if len(prefix) < MinSuperTeamPrefixLength || len(prefix) > MaxSuperTeamPrefixLength {
		return "", false
	}
	for index := 0; index < len(prefix); index++ {
		value := prefix[index]
		if (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9') {
			continue
		}
		if (value == '-' || value == '_') && index > 0 && index < len(prefix)-1 {
			continue
		}
		return "", false
	}
	return prefix, true
}

// NormalizeSuperTeamText trims and validates a global team's display text.
func NormalizeSuperTeamText(value string, maxRunes int, allowEmpty bool) (string, bool) {
	value = strings.TrimSpace(value)
	if (!allowEmpty && value == "") || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxRunes {
		return "", false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", false
		}
	}
	return value, true
}

// SuperTeamPackagePermission maps a global-team role onto the cumulative L0-L4 package model.
func SuperTeamPackagePermission(role int) int {
	switch role {
	case SuperTeamRoleRead:
		return 0
	case SuperTeamRoleWrite:
		return 2
	case SuperTeamRoleManage:
		return 3
	case SuperTeamRoleOwner:
		return 4
	default:
		return -1
	}
}

// DockerImageSuperTeamPrefix returns the required global-team prefix for a namespaced Docker image.
func DockerImageSuperTeamPrefix(imageName string) (string, bool) {
	imageName = strings.ToLower(strings.Trim(strings.TrimSpace(imageName), "/"))
	separator := strings.IndexByte(imageName, '/')
	if separator <= 0 {
		return "", false
	}
	prefix, valid := NormalizeSuperTeamPrefix(imageName[:separator])
	return prefix, valid
}

// NPMPackageSuperTeamPrefix returns the required global-team prefix for a scoped npm package.
func NPMPackageSuperTeamPrefix(packageName string) (string, bool) {
	packageName = strings.ToLower(strings.TrimSpace(packageName))
	if !strings.HasPrefix(packageName, "@") {
		return "", false
	}
	separator := strings.IndexByte(packageName, '/')
	if separator <= 1 {
		return "", false
	}
	prefix, valid := NormalizeSuperTeamPrefix(packageName[1:separator])
	return prefix, valid
}
