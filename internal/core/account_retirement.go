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
	AccountEmailHoldMillis      int64 = 14 * 24 * 60 * 60 * 1000
	AccountAuditRetentionMillis int64 = 30 * 24 * 60 * 60 * 1000
)

var (
	ErrAccountDeleted        = errors.New("account is retired")
	ErrAccountNotRetired     = errors.New("account is not retired")
	ErrAccountRetirementBusy = errors.New("account retirement prerequisites are not satisfied")
)

// AccountRetirementPlan summarizes bounded ownership and workflow blockers.
type AccountRetirementPlan struct {
	Username              string `json:"username"`
	Eligible              bool   `json:"eligible"`
	ProtectedRole         bool   `json:"protected_role"`
	SuperTeamOwnerCount   int    `json:"super_team_owner_count"`
	MavenDomainOwnerCount int    `json:"maven_domain_owner_count"`
	PackageOwnerCount     int    `json:"package_owner_count"`
	PendingReviewCount    int    `json:"pending_review_count"`
}

// AccountRetirementStatus exposes fixed retention deadlines without private email content.
type AccountRetirementStatus struct {
	Username        string `json:"username"`
	DeletedAt       int64  `json:"deleted_at"`
	EmailReleaseAt  int64  `json:"email_release_at"`
	EmailReleasedAt int64  `json:"email_released_at,omitempty"`
	AuditPurgeAt    int64  `json:"audit_purge_at"`
	AuditPurgedAt   int64  `json:"audit_purged_at,omitempty"`
}
