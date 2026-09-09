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
	"runtime"
	"strings"
	"unicode"

	"renop/internal/core"
)

func initResourceLockTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS resource_locks (
		id VARCHAR(64) NOT NULL, source VARCHAR(16) NOT NULL,
		package_id VARCHAR(64) NOT NULL, format VARCHAR(16) NOT NULL,
		repository VARCHAR(255) NOT NULL, resource_name TEXT NOT NULL,
		version VARCHAR(255) NOT NULL, mode VARCHAR(16) NOT NULL,
		reason VARCHAR(32) NOT NULL, locked_at BIGINT NOT NULL,
		PRIMARY KEY (id, source)
	);`)
	return err
}

func normalizeResourceLockTarget(target core.ResourceLockTarget) (core.ResourceLockTarget, error) {
	var valid bool
	target.Format, target.Repository, target.Name, valid = normalizePackageDeprecation(
		target.Format, target.Repository, target.Name)
	if !valid || len(target.Version) > 255 || strings.IndexFunc(target.Version, unicode.IsControl) >= 0 ||
		strings.TrimSpace(target.Version) != target.Version {
		return core.ResourceLockTarget{}, core.ErrResourceLockInvalid
	}
	return target, nil
}

func resourceLockID(target core.ResourceLockTarget) string {
	return packageDeprecationID(target.Format, target.Repository,
		fmt.Sprintf("%d:%s%d:%s", len(target.Name), target.Name, len(target.Version), target.Version))
}

func resourceLockVersionColumn(format, column string) string {
	if runtime.GOOS == "windows" && format == "cargo" {
		return "LOWER(" + column + ")"
	}
	return column
}

// SetResourceLock replaces only the selected source's lock, preserving other restrictions.
func (db *DB) SetResourceLock(lock *core.ResourceLock, actor, session string) error {
	if lock == nil || lock.LockedAt <= 0 || !core.ValidResourceLockReason(lock.Reason) ||
		(lock.Mode != core.ResourceLockWrite && lock.Mode != core.ResourceLockRead) ||
		(lock.Source != core.ResourceLockManual && lock.Source != core.ResourceLockSystem) {
		return core.ErrResourceLockInvalid
	}
	target, err := normalizeResourceLockTarget(lock.ResourceLockTarget)
	if err != nil {
		return err
	}
	id := resourceLockID(target)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := authorizeResourceLockTx(tx, target, lock.Source, actor, session); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM resource_locks WHERE id = ? AND source = ?`, id, lock.Source); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO resource_locks
		(id, source, package_id, format, repository, resource_name, version, mode, reason, locked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, lock.Source,
		packageDeprecationID(target.Format, target.Repository, target.Name), target.Format,
		target.Repository, target.Name, target.Version, lock.Mode, lock.Reason, lock.LockedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteResourceLock removes one exact source without removing inherited or system locks.
func (db *DB) DeleteResourceLock(target core.ResourceLockTarget, source, actor, session string) error {
	target, err := normalizeResourceLockTarget(target)
	if err != nil || (source != core.ResourceLockManual && source != core.ResourceLockSystem) {
		return core.ErrResourceLockInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := authorizeResourceLockTx(tx, target, source, actor, session); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM resource_locks WHERE id = ? AND source = ?`, resourceLockID(target), source); err != nil {
		return err
	}
	return tx.Commit()
}

func authorizeResourceLockTx(tx *Tx, target core.ResourceLockTarget, source, actor, session string) error {
	if source == core.ResourceLockSystem && actor == "" && session == "" {
		return nil
	}
	if source != core.ResourceLockManual {
		return core.ErrResourceLockPermission
	}
	account, err := accountEmailSessionTx(tx, strings.ToLower(actor), session)
	if err != nil {
		if errors.Is(err, core.ErrEmailCodeInvalid) || errors.Is(err, core.ErrUserProfileNotFound) ||
			errors.Is(err, core.ErrAccountDeleted) || errors.Is(err, core.ErrAccountBanned) {
			return core.ErrResourceLockPermission
		}
		return err
	}
	user, err := reviewUserTx(tx, account.UserID)
	if err != nil {
		if errors.Is(err, core.ErrReviewPermissionDenied) {
			return core.ErrResourceLockPermission
		}
		return err
	}
	if !user.CheckModeratePermission(target.Repository) {
		return core.ErrResourceLockPermission
	}
	return nil
}

// GetResourceLocks reads package restrictions and either one version or all child versions.
func (db *DB) GetResourceLocks(target core.ResourceLockTarget, allVersions bool) ([]*core.ResourceLock, error) {
	target, err := normalizeResourceLockTarget(target)
	if err != nil {
		return nil, err
	}
	query := `SELECT format, repository, resource_name, version, source, mode, reason, locked_at
		FROM resource_locks WHERE package_id = ?`
	args := []any{packageDeprecationID(target.Format, target.Repository, target.Name)}
	if !allVersions {
		query += ` AND (version = '' OR ` + resourceLockVersionColumn(target.Format, "version") + ` = ?)`
		args = append(args, core.ResourceLockVersionKey(target.Format, target.Version))
	}
	rows, err := db.Query(query+` ORDER BY version, source`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	locks := make([]*core.ResourceLock, 0)
	for rows.Next() {
		lock := &core.ResourceLock{}
		if err := rows.Scan(&lock.Format, &lock.Repository, &lock.Name, &lock.Version,
			&lock.Source, &lock.Mode, &lock.Reason, &lock.LockedAt); err != nil {
			return nil, err
		}
		locks = append(locks, lock)
	}
	return locks, rows.Err()
}

// EnsureResourceMutable includes child locks when an operation rewrites an entire package.
func (db *DB) EnsureResourceMutable(target core.ResourceLockTarget, allVersions bool) error {
	return ensureResourceMutableQuery(db.QueryRow, target, allVersions)
}

func ensureResourceMutableQuery(queryRow func(string, ...any) row, target core.ResourceLockTarget, allVersions bool) error {
	target, err := normalizeResourceLockTarget(target)
	if err != nil {
		return err
	}
	query := `SELECT 1 FROM resource_locks WHERE package_id = ?`
	args := []any{packageDeprecationID(target.Format, target.Repository, target.Name)}
	if !allVersions {
		query += ` AND (version = '' OR ` + resourceLockVersionColumn(target.Format, "version") + ` = ?)`
		args = append(args, core.ResourceLockVersionKey(target.Format, target.Version))
	}
	var exists int
	err = queryRow(query+` LIMIT 1`, args...).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return core.ErrResourceLocked
}

// CargoMetadataVisibility checks a bounded page without one membership or lock query per entry.
func (db *DB) CargoMetadataVisibility(repository, username string, moderator bool, targets []core.ResourceLockTarget) ([]bool, error) {
	if len(targets) > 128 {
		return nil, core.ErrResourceLockInvalid
	}
	visible := make([]bool, len(targets))
	for i := range visible {
		visible[i] = true
	}
	if moderator || len(targets) == 0 {
		return visible, nil
	}
	userID := ""
	if username != "" && !strings.EqualFold(username, "guest") {
		var err error
		userID, err = db.userIDForUsername(username)
		if err != nil && !errors.Is(err, core.ErrUserProfileNotFound) {
			return nil, err
		}
	}
	args := []any{userID, userID, strings.ToLower(repository)}
	conditions := make([]string, len(targets))
	normalized := make([]core.ResourceLockTarget, len(targets))
	for i, target := range targets {
		target.Repository, target.Format = repository, "cargo"
		var err error
		normalized[i], err = normalizeResourceLockTarget(target)
		if err != nil {
			return nil, err
		}
		normalized[i].Version = core.ResourceLockVersionKey("cargo", normalized[i].Version)
		conditions[i] = `(l.resource_name = ? AND (l.version = '' OR ` + resourceLockVersionColumn("cargo", "l.version") + ` = ?))`
		args = append(args, normalized[i].Name, normalized[i].Version)
	}
	rows, err := db.Query(`SELECT l.resource_name, l.version FROM resource_locks l
		LEFT JOIN cargo_packages p ON p.repository = l.repository AND p.normalized_name = l.resource_name
		LEFT JOIN cargo_members m ON m.repository = p.repository AND m.normalized_name = p.normalized_name AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = p.super_team_prefix AND stm.user_id = ?
		WHERE l.format = 'cargo' AND l.repository = ? AND l.mode = 'read'
		AND m.user_id IS NULL AND stm.user_id IS NULL AND (`+strings.Join(conditions, " OR ")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hidden := make(map[core.ResourceLockTarget]bool)
	for rows.Next() {
		var target core.ResourceLockTarget
		target.Format, target.Repository = "cargo", strings.ToLower(repository)
		if err := rows.Scan(&target.Name, &target.Version); err != nil {
			return nil, err
		}
		target.Version = core.ResourceLockVersionKey("cargo", target.Version)
		hidden[target] = true
	}
	for i, target := range normalized {
		visible[i] = !hidden[target]
		target.Version = ""
		visible[i] = visible[i] && !hidden[target]
	}
	return visible, rows.Err()
}

// EnsureRepositoryResourcesMutable prevents reconfiguration or deletion from bypassing resource locks.
func (db *DB) EnsureRepositoryResourcesMutable(repository string) error {
	var exists int
	err := db.QueryRow(`SELECT 1 FROM resource_locks WHERE repository = ? LIMIT 1`, strings.ToLower(repository)).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return core.ErrResourceLocked
}
