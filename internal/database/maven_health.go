/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"renop/internal/core"
)

func validMavenProviderIdentity(verificationType string, health *core.MavenDomainHealth) bool {
	if verificationType != core.MavenVerificationGitHub && verificationType != core.MavenVerificationGitLab {
		return true
	}
	if health == nil || (health.ProviderType != core.GitHubPrincipalUser && health.ProviderType != core.GitHubPrincipalOrganization) {
		return false
	}
	id, err := strconv.ParseInt(health.ProviderID, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == health.ProviderID
}

// ListMavenDomainHealthChecks returns the oldest due verified or security-locked namespaces.
func (db *DB) ListMavenDomainHealthChecks(now int64, limit int) ([]*core.MavenDomain, error) {
	if limit < 1 || limit > 128 {
		return nil, core.ErrMavenVerificationFailed
	}
	rows, err := db.Query(`SELECT `+mavenDomainSelectColumns+` FROM maven_domains d
		LEFT JOIN maven_domain_members m ON m.repository = d.repository AND m.domain = d.domain AND m.user_id = ''
		LEFT JOIN super_team_members stm ON stm.team_prefix = d.super_team_prefix AND stm.user_id = ''
		WHERE d.repository = '' AND d.closed_at = 0 AND (d.verified = 1 OR d.health_locked_at > 0)
		AND d.verification_type IN (?, ?, ?) AND d.health_next_check_at <= ?
		ORDER BY d.health_next_check_at, d.domain LIMIT ?`, core.MavenVerificationDNS,
		core.MavenVerificationGitHub, core.MavenVerificationGitLab, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	domains := make([]*core.MavenDomain, 0, limit)
	for rows.Next() {
		domain, err := scanMavenDomain(rows)
		if err != nil {
			return nil, err
		}
		domains = append(domains, domain)
	}
	return domains, rows.Err()
}

// RecordMavenDomainHealth preserves a security lock until an explicit redemption, discarding stale checks.
func (db *DB) RecordMavenDomainHealth(expected *core.MavenDomain, health *core.MavenDomainHealth, releaseAt int64, code string) error {
	if expected == nil || expected.Health == nil || health == nil || health.CheckedAt <= 0 || health.NextCheckAt <= health.CheckedAt {
		return core.ErrMavenVerificationFailed
	}
	switch health.Status {
	case "active", "unavailable", "hold", "expired", "prohibited":
	default:
		return core.ErrMavenVerificationFailed
	}
	lockedAt, reservedUntil, reason := expected.Health.LockedAt, expected.Health.ReleaseAt, expected.Health.LockReason
	if lockedAt == 0 && health.Status != "active" && health.Status != "unavailable" {
		if releaseAt <= health.CheckedAt || code == "" || code == expected.VerificationCode || len(code) > 128 {
			return core.ErrMavenVerificationFailed
		}
		lockedAt, reservedUntil, reason = health.CheckedAt, releaseAt, health.Status
	} else {
		code = expected.VerificationCode
	}
	expiresAt := health.ExpiresAt
	if expiresAt == 0 {
		expiresAt = expected.Health.ExpiresAt
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockMavenDomainRow(tx, expected.Domain); errors.Is(err, core.ErrMavenDomainNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE maven_domains SET health_status = ?, health_checked_at = ?, health_next_check_at = ?,
		health_expires_at = ?, health_locked_at = ?, health_release_at = ?, health_lock_reason = ?, verification_code = ?
		WHERE repository = '' AND domain = ? AND verification_code = ? AND created_at = ? AND closed_at = 0
		AND health_checked_at = ? AND health_locked_at = ? AND provider_id = ? AND provider_type = ?`,
		health.Status, health.CheckedAt, health.NextCheckAt, expiresAt, lockedAt, reservedUntil, reason, code,
		expected.Domain, expected.VerificationCode, expected.CreatedAt, expected.Health.CheckedAt,
		expected.Health.LockedAt, expected.Health.ProviderID, expected.Health.ProviderType)
	if err != nil {
		return fmt.Errorf("record Maven domain health: %w", err)
	}
	return tx.Commit()
}

// ReserveMavenRedemptionAttempt permits current owners to prove control without restoring any other permission.
func (db *DB) ReserveMavenRedemptionAttempt(domain, actor, session string, checkedAt, minimumPrevious int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	account, err := accountEmailSessionTx(tx, sanitizeMavenUsername(actor), session)
	if err != nil {
		return core.ErrMavenPermissionDenied
	}
	if err := lockMavenDomainRow(tx, domain); err != nil {
		return err
	}
	if err := requireMavenMemberPermission(tx, domain, account.UserID, core.MavenPermissionOwner); err != nil {
		return err
	}
	result, err := tx.Exec(`UPDATE maven_domains SET last_check_at = ? WHERE repository = '' AND domain = ?
		AND health_locked_at > 0 AND closed_at = 0 AND last_check_at <= ?`, checkedAt, domain, minimumPrevious)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return core.ErrMavenVerificationRateLimit
	}
	return tx.Commit()
}

// RedeemMavenDomain clears only the health restriction after fresh proof and a live ownership recheck.
func (db *DB) RedeemMavenDomain(expected *core.MavenDomain, health *core.MavenDomainHealth, actor, session string) error {
	if expected == nil || expected.Health == nil || expected.Health.LockedAt == 0 || health == nil ||
		health.Status != "active" || health.CheckedAt <= 0 || !validMavenProviderIdentity(expected.VerificationType, health) {
		return core.ErrMavenVerificationFailed
	}
	if expected.Health.ProviderID != "" && (expected.Health.ProviderID != health.ProviderID || expected.Health.ProviderType != health.ProviderType) {
		return core.ErrMavenVerificationFailed
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	account, err := accountEmailSessionTx(tx, sanitizeMavenUsername(actor), session)
	if err != nil {
		return core.ErrMavenPermissionDenied
	}
	if err := lockMavenDomainRow(tx, expected.Domain); err != nil {
		return err
	}
	if err := requireMavenMemberPermission(tx, expected.Domain, account.UserID, core.MavenPermissionOwner); err != nil {
		return err
	}
	if err := ensureMavenDomainAncestorMutableTx(tx, expected.Domain); err != nil {
		return err
	}
	result, err := tx.Exec(`UPDATE maven_domains SET provider_type = ?, provider_id = ?, health_status = 'active',
		health_checked_at = ?, health_next_check_at = ?, health_expires_at = ?, health_locked_at = 0,
		health_release_at = 0, health_lock_reason = '' WHERE repository = '' AND domain = ?
		AND verification_code = ? AND created_at = ? AND closed_at = 0 AND health_locked_at = ?
		AND provider_id = ? AND provider_type = ? AND health_checked_at <= ?`, health.ProviderType, health.ProviderID,
		health.CheckedAt, health.NextCheckAt, health.ExpiresAt, expected.Domain, expected.VerificationCode,
		expected.CreatedAt, expected.Health.LockedAt, expected.Health.ProviderID, expected.Health.ProviderType, health.CheckedAt)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return core.ErrMavenVerificationFailed
	}
	return tx.Commit()
}

func ensureMavenDomainOtherLocksMutableTx(tx *Tx, domain, excludedID string) error {
	var exists int
	err := tx.QueryRow(`SELECT 1 FROM `+resourceLocksQuery("maven-domain")+`
		WHERE format = 'maven-domain' AND repository = '' AND resource_name = ? AND id <> ? LIMIT 1`, domain, excludedID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return core.ErrResourceLocked
}
