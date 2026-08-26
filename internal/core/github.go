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
	// GitHubPrincipalUser identifies an authorized GitHub user account.
	GitHubPrincipalUser = "user"
	// GitHubPrincipalOrganization identifies an authorized GitHub organization.
	GitHubPrincipalOrganization = "organization"
	// GitHubPrincipalFreshnessMillis bounds how long organization membership may authorize a new Maven domain.
	GitHubPrincipalFreshnessMillis = int64(60 * 60 * 1000)
)

var (
	// ErrGitHubIdentityLinked indicates that either side of a requested link is already linked elsewhere.
	ErrGitHubIdentityLinked = errors.New("GitHub identity is already linked")
	// ErrGitHubIdentityNotFound indicates that an account has no GitHub identity.
	ErrGitHubIdentityNotFound = errors.New("GitHub identity was not found")
)

// GitHubIdentity links an immutable GitHub account to an immutable RenoP account.
type GitHubIdentity struct {
	UserID         string `json:"-"`
	Username       string `json:"username"`
	GitHubUserID   int64  `json:"github_user_id"`
	GitHubLogin    string `json:"github_login"`
	AuthorizedAt   int64  `json:"authorized_at"`
	PrincipalCount int    `json:"principal_count"`
}

// GitHubPrincipal is one user or organization confirmed during an OAuth authorization.
type GitHubPrincipal struct {
	Type         string `json:"type"`
	GitHubID     int64  `json:"github_id"`
	Login        string `json:"login"`
	AuthorizedAt int64  `json:"authorized_at"`
}
