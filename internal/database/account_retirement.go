/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"renop/internal/core"
)

func protectedRetirementRole(permissions []string) bool {
	for _, permission := range permissions {
		permission = strings.ToLower(strings.TrimSpace(permission))
		if permission == "manager" || permission == "admin" || permission == "m" ||
			permission == "access-token:manager" || strings.HasPrefix(permission, "canmoderate:") {
			return true
		}
	}
	return false
}

func accountRetirementPlanTx(tx *Tx, username, userID string,
	token *core.AccessToken) (*core.AccountRetirementPlan, error) {
	plan := &core.AccountRetirementPlan{Username: username, ProtectedRole: protectedRetirementRole(token.Permissions)}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM super_team_members WHERE user_id = ? AND role_level = ?`,
		userID, core.SuperTeamRoleOwner).Scan(&plan.SuperTeamOwnerCount); err != nil {
		return nil, fmt.Errorf("count global-team ownerships: %w", err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM maven_domain_members member JOIN maven_domains domain
		ON domain.repository = member.repository AND domain.domain = member.domain
		WHERE member.user_id = ? AND member.permission_level = ? AND domain.closed_at = 0`,
		userID, core.MavenPermissionOwner).Scan(&plan.MavenDomainOwnerCount); err != nil {
		return nil, fmt.Errorf("count active Maven domain ownerships: %w", err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM review_tasks WHERE requested_by_id = ? AND status = ?`,
		userID, core.ReviewStatusPending).Scan(&plan.PendingReviewCount); err != nil {
		return nil, fmt.Errorf("count pending account reviews: %w", err)
	}
	rows, err := tx.Query(`SELECT COUNT(*) FROM cargo_members member
		LEFT JOIN package_deprecations deprecated ON deprecated.format = 'cargo'
		AND deprecated.repository = member.repository AND deprecated.package_key = member.normalized_name
		WHERE member.user_id = ? AND member.permission_level = ? AND COALESCE(deprecated.id, '') = ''
		UNION ALL SELECT COUNT(*) FROM docker_members member
		LEFT JOIN package_deprecations deprecated ON deprecated.format = 'docker'
		AND deprecated.repository = member.repository AND deprecated.package_key = member.image_name
		WHERE member.user_id = ? AND member.permission_level = ? AND COALESCE(deprecated.id, '') = ''
		UNION ALL SELECT COUNT(*) FROM npm_members member
		LEFT JOIN package_deprecations deprecated ON deprecated.format = 'npm'
		AND deprecated.repository = member.repository AND deprecated.package_key = member.package_name
		WHERE member.user_id = ? AND member.permission_level = ? AND COALESCE(deprecated.id, '') = ''
		UNION ALL SELECT COUNT(*) FROM docker_images image
		LEFT JOIN docker_members member ON member.repository = image.repository
		AND member.image_name = image.image_name AND member.user_id = ?
		LEFT JOIN package_deprecations deprecated ON deprecated.format = 'docker'
		AND deprecated.repository = image.repository AND deprecated.package_key = image.image_name
		WHERE image.publisher = ? AND COALESCE(member.user_id, '') = '' AND COALESCE(deprecated.id, '') = ''`,
		userID, core.CargoPermissionOwner, userID, core.DockerPermissionOwner,
		userID, core.NPMPermissionOwner, userID, username)
	if err != nil {
		return nil, fmt.Errorf("list package ownerships: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var count uint64
		if err := rows.Scan(&count); err != nil {
			return nil, fmt.Errorf("scan package ownership: %w", err)
		}
		plan.PackageOwnerCount += int(count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate package ownerships: %w", err)
	}
	plan.Eligible = !plan.ProtectedRole && plan.SuperTeamOwnerCount == 0 &&
		plan.MavenDomainOwnerCount == 0 && plan.PackageOwnerCount == 0 && plan.PendingReviewCount == 0
	return plan, nil
}

// GetAccountRetirementPlan returns ownership and review prerequisites without changing account data.
func (db *DB) GetAccountRetirementPlan(username string) (*core.AccountRetirementPlan, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	username = strings.ToLower(strings.TrimSpace(username))
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin account retirement plan: %w", err)
	}
	defer tx.Rollback()
	token, err := tokenByNameTx(tx, username)
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, core.ErrUserProfileNotFound
	}
	if token.DeletedAt > 0 {
		return nil, core.ErrAccountDeleted
	}
	var userID string
	if err := tx.QueryRow(`SELECT user_id FROM user_profiles WHERE username = ?`, username).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, core.ErrUserProfileNotFound
		}
		return nil, fmt.Errorf("resolve retirement account: %w", err)
	}
	return accountRetirementPlanTx(tx, username, userID, token)
}

func accountRetirementStatus(token *core.AccessToken) *core.AccountRetirementStatus {
	if token == nil || token.DeletedAt <= 0 {
		return nil
	}
	return &core.AccountRetirementStatus{
		Username: token.Name, DeletedAt: token.DeletedAt,
		EmailReleaseAt:  token.DeletedAt + core.AccountEmailHoldMillis,
		EmailReleasedAt: token.EmailReleasedAt,
		AuditPurgeAt:    token.DeletedAt + core.AccountAuditRetentionMillis,
		AuditPurgedAt:   token.AuditPurgedAt,
	}
}

// GetAccountRetirementStatus returns retention deadlines for a permanently retired account.
func (db *DB) GetAccountRetirementStatus(username string) (*core.AccountRetirementStatus, error) {
	token, err := db.GetTokenByName(strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		return nil, err
	}
	status := accountRetirementStatus(token)
	if status == nil {
		return nil, core.ErrAccountNotRetired
	}
	return status, nil
}

// ReleaseRetiredAccountEmail releases a retired account's email without unlocking its username.
func (db *DB) ReleaseRetiredAccountEmail(username string, releasedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" || releasedAt <= 0 {
		return core.ErrAccountNotRetired
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin retired email release: %w", err)
	}
	defer tx.Rollback()
	var userID string
	var deletedAt, alreadyReleased int64
	if err := tx.QueryRow(`SELECT profile.user_id, token.deleted_at, token.email_released_at
		FROM tokens token JOIN user_profiles profile ON profile.username = token.name WHERE token.name = ?`, username).
		Scan(&userID, &deletedAt, &alreadyReleased); errors.Is(err, sql.ErrNoRows) {
		return core.ErrAccountNotRetired
	} else if err != nil {
		return fmt.Errorf("inspect retired account email: %w", err)
	}
	if deletedAt <= 0 {
		return core.ErrAccountNotRetired
	}
	if _, err := tx.Exec(`UPDATE user_account_security SET email = NULL, password_login_enabled = 0, updated_at = ?
		WHERE user_id = ?`, releasedAt, userID); err != nil {
		return fmt.Errorf("release retired account email: %w", err)
	}
	if alreadyReleased == 0 {
		if _, err := tx.Exec(`UPDATE tokens SET email_released_at = ? WHERE name = ? AND deleted_at > 0
			AND email_released_at = 0`, releasedAt, username); err != nil {
			return fmt.Errorf("record retired email release: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit retired email release: %w", err)
	}
	db.tokenCache.Delete(username)
	return nil
}

// PurgeRetiredAccountAuditLogs removes retained activity and prevents queued entries from restoring it.
func (db *DB) PurgeRetiredAccountAuditLogs(username string, purgedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" || purgedAt <= 0 {
		return core.ErrAccountNotRetired
	}
	db.auditWriteMu.Lock()
	defer db.auditWriteMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin retired audit purge: %w", err)
	}
	defer tx.Rollback()
	var deletedAt int64
	if err := tx.QueryRow(`SELECT deleted_at FROM tokens WHERE name = ?`, username).Scan(&deletedAt); errors.Is(err, sql.ErrNoRows) {
		return core.ErrAccountNotRetired
	} else if err != nil {
		return fmt.Errorf("inspect retired account audit: %w", err)
	}
	if deletedAt <= 0 {
		return core.ErrAccountNotRetired
	}
	if _, err := tx.Exec(`DELETE FROM audit_logs WHERE username = ? OR operator = ?`, username, username); err != nil {
		return fmt.Errorf("purge retired account audit logs: %w", err)
	}
	if _, err := tx.Exec(`UPDATE tokens SET audit_purged_at = ? WHERE name = ? AND deleted_at > 0`,
		purgedAt, username); err != nil {
		return fmt.Errorf("record retired audit purge: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit retired audit purge: %w", err)
	}
	db.tokenCache.Delete(username)
	return nil
}

// CleanupRetiredAccountData processes a bounded batch of expired email and audit retention periods.
func (db *DB) CleanupRetiredAccountData(now int64, limit int) error {
	if db == nil || db.SQLDB == nil || now <= 0 {
		return nil
	}
	if limit < 1 || limit > 256 {
		limit = 100
	}
	collect := func(column string, cutoff int64) ([]string, error) {
		rows, err := db.Query(`SELECT name FROM tokens WHERE deleted_at > 0 AND `+column+` = 0
			AND deleted_at <= ? ORDER BY deleted_at, name LIMIT ?`, cutoff, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		names := make([]string, 0, limit)
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}
			names = append(names, name)
		}
		return names, rows.Err()
	}
	emails, err := collect("email_released_at", now-core.AccountEmailHoldMillis)
	if err != nil {
		return fmt.Errorf("list retired emails for release: %w", err)
	}
	for _, username := range emails {
		if err := db.ReleaseRetiredAccountEmail(username, now); err != nil && !errors.Is(err, core.ErrAccountNotRetired) {
			return err
		}
	}
	audits, err := collect("audit_purged_at", now-core.AccountAuditRetentionMillis)
	if err != nil {
		return fmt.Errorf("list retired audits for purge: %w", err)
	}
	for _, username := range audits {
		if err := db.PurgeRetiredAccountAuditLogs(username, now); err != nil && !errors.Is(err, core.ErrAccountNotRetired) {
			return err
		}
	}
	return nil
}
