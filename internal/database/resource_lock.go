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
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS resource_lock_versions (
		lock_id VARCHAR(64) NOT NULL, source VARCHAR(16) NOT NULL,
		version VARCHAR(255) NOT NULL, PRIMARY KEY (lock_id, source, version)
	);`)
	return err
}

// Parent restrictions follow live bindings; Docker indexes also retain captured content references.
func resourceLocksQuery(format string) string {
	const columns = `id, source, package_id, format, repository, resource_name, version, mode, reason, locked_at`
	query := `SELECT ` + columns + `, 0 AS inherited FROM resource_locks`
	if format == "docker" {
		query += ` UNION ALL SELECT l.id, l.source, l.package_id, l.format, l.repository, l.resource_name,
			v.version, l.mode, l.reason, l.locked_at, 1 AS inherited
			FROM resource_locks l JOIN resource_lock_versions v ON v.lock_id = l.id AND v.source = l.source`
	}
	table, name := "", ""
	join := ""
	binding := `p.super_team_prefix = l.resource_name`
	switch format {
	case "cargo":
		table, name = "cargo_packages", "p.normalized_name"
	case "npm":
		table, name = "npm_packages", "p.package_name"
	case "docker":
		table, name = "docker_images", "p.image_name"
	case "maven":
		table, name = "maven_artifacts", resourceLockVersionColumn("maven", "CONCAT(p.group_id, ':', p.artifact_id)")
		join = ` LEFT JOIN maven_domains d ON d.repository = '' AND d.domain = p.domain`
		binding = `(` + binding + ` OR d.super_team_prefix = l.resource_name)`
	case "maven-domain":
		table, name = "maven_domains", "p.domain"
		binding += ` AND p.repository = ''`
	}
	if table != "" {
		query += ` UNION ALL SELECT l.id, l.source, '', '` + format + `', p.repository, ` + name + `,
			'', l.mode, l.reason, l.locked_at, 1 FROM ` + table + ` p` + join + `
			JOIN resource_locks l ON l.format = 'superteam' AND l.repository = '' AND ` + binding
	}
	return `(` + query + `)`
}

func normalizeResourceLockTarget(target core.ResourceLockTarget) (core.ResourceLockTarget, error) {
	if target.Format == "maven-domain" {
		domain := sanitizeMavenDomain(target.Name)
		if target.Repository != "" || target.Version != "" || domain == "" || len(domain) > 253 || !core.ValidMavenCoordinatePart(domain) {
			return core.ResourceLockTarget{}, core.ErrResourceLockInvalid
		}
		return core.ResourceLockTarget{Format: "maven-domain", Name: domain}, nil
	}
	if strings.EqualFold(strings.TrimSpace(target.Format), "superteam") {
		prefix, valid := core.NormalizeSuperTeamPrefix(target.Name)
		if !valid || target.Repository != "" || target.Version != "" {
			return core.ResourceLockTarget{}, core.ErrResourceLockInvalid
		}
		return core.ResourceLockTarget{Format: "superteam", Name: prefix}, nil
	}
	var valid bool
	target.Format, target.Repository, target.Name, valid = normalizePackageDeprecation(
		target.Format, target.Repository, target.Name)
	if target.Format == "docker" {
		target.Version = strings.ToLower(target.Version)
	}
	if target.Format == "maven" {
		group, artifact, ok := strings.Cut(target.Name, ":")
		if !ok || group == "" || artifact == "" || strings.Contains(artifact, ":") {
			return core.ResourceLockTarget{}, core.ErrResourceLockInvalid
		}
		target.Name = strings.ToLower(group) + ":" + artifact
		if runtime.GOOS == "windows" {
			target.Name = strings.ToLower(target.Name)
		}
	}
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
	if runtime.GOOS == "windows" && (format == "cargo" || format == "npm" || format == "maven") {
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
	if target.Format == "superteam" {
		superTeamMutationLock.Lock()
		defer superTeamMutationLock.Unlock()
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := authorizeResourceLockTx(tx, target, lock.Source, actor, session); err != nil {
		return err
	}
	if target.Format == "superteam" {
		if err := lockSuperTeamTx(tx, target.Name); err != nil {
			return err
		}
	}
	if target.Format == "maven" {
		if err := lockMavenArtifactTargetTx(tx, target); err != nil {
			return err
		}
	}
	if err := setDockerLockVersionsTx(tx, target, lock.Source); err != nil {
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
	if target.Format == "superteam" {
		superTeamMutationLock.Lock()
		defer superTeamMutationLock.Unlock()
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := authorizeResourceLockTx(tx, target, source, actor, session); err != nil {
		return err
	}
	if target.Format == "superteam" {
		if err := lockSuperTeamTx(tx, target.Name); err != nil {
			return err
		}
	}
	if target.Format == "maven" {
		if err := lockMavenArtifactTargetTx(tx, target); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM resource_lock_versions WHERE lock_id = ? AND source = ?`, resourceLockID(target), source); err != nil {
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
	query := `SELECT format, repository, resource_name, version, source, mode, reason, locked_at, inherited
		FROM ` + resourceLocksQuery(target.Format) + ` l WHERE format = ? AND repository = ? AND resource_name = ?`
	args := []any{target.Format, target.Repository, target.Name}
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
		var inherited int
		if err := rows.Scan(&lock.Format, &lock.Repository, &lock.Name, &lock.Version,
			&lock.Source, &lock.Mode, &lock.Reason, &lock.LockedAt, &inherited); err != nil {
			return nil, err
		}
		lock.Inherited = inherited != 0
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
	query := `SELECT 1 FROM ` + resourceLocksQuery(target.Format) + ` l WHERE format = ? AND repository = ? AND resource_name = ?`
	args := []any{target.Format, target.Repository, target.Name}
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

// ResourceMetadataVisibility checks a bounded package page without per-entry membership or lock queries.
func (db *DB) ResourceMetadataVisibility(format, repository, username string, moderator bool, targets []core.ResourceLockTarget) ([]bool, error) {
	if format == "maven-domain" {
		return db.mavenDomainMetadataVisibility(username, moderator, targets)
	}
	if format == "maven" {
		return db.mavenMetadataVisibility(repository, username, moderator, targets)
	}
	table, members, nameColumn := "cargo_packages", "cargo_members", "normalized_name"
	switch format {
	case "cargo":
	case "npm":
		table, members, nameColumn = "npm_packages", "npm_members", "package_name"
	case "docker":
		table, members, nameColumn = "docker_images", "docker_members", "image_name"
	default:
		return nil, core.ErrResourceLockInvalid
	}
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
	args := []any{userID, userID, format, strings.ToLower(repository)}
	conditions := make([]string, len(targets))
	normalized := make([]core.ResourceLockTarget, len(targets))
	for i, target := range targets {
		target.Repository, target.Format = repository, format
		var err error
		normalized[i], err = normalizeResourceLockTarget(target)
		if err != nil {
			return nil, err
		}
		normalized[i].Version = core.ResourceLockVersionKey(format, normalized[i].Version)
		conditions[i] = `(l.resource_name = ? AND (l.version = '' OR ` + resourceLockVersionColumn(format, "l.version") + ` = ?))`
		args = append(args, normalized[i].Name, normalized[i].Version)
	}
	rows, err := db.Query(`SELECT l.resource_name, l.version FROM `+resourceLocksQuery(format)+` l
		LEFT JOIN `+table+` p ON p.repository = l.repository AND p.`+nameColumn+` = l.resource_name
		LEFT JOIN `+members+` m ON m.repository = p.repository AND m.`+nameColumn+` = p.`+nameColumn+` AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = p.super_team_prefix AND stm.user_id = ?
		WHERE l.format = ? AND l.repository = ? AND l.mode = 'read'
		AND m.user_id IS NULL AND stm.user_id IS NULL AND (`+strings.Join(conditions, " OR ")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hidden := make(map[core.ResourceLockTarget]bool)
	for rows.Next() {
		var target core.ResourceLockTarget
		target.Format, target.Repository = format, strings.ToLower(repository)
		if err := rows.Scan(&target.Name, &target.Version); err != nil {
			return nil, err
		}
		target.Version = core.ResourceLockVersionKey(format, target.Version)
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
	for _, format := range []string{"cargo", "npm", "docker", "maven"} {
		var exists int
		err := db.QueryRow(`SELECT 1 FROM `+resourceLocksQuery(format)+` l WHERE repository = ? LIMIT 1`, strings.ToLower(repository)).Scan(&exists)
		if err == nil {
			return core.ErrResourceLocked
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

func ensureSuperTeamMutableQuery(queryRow func(string, ...any) row, prefix string) error {
	if prefix == "" {
		return nil
	}
	return ensureResourceMutableQuery(queryRow, core.ResourceLockTarget{Format: "superteam", Name: prefix}, false)
}

func lockSuperTeamTx(tx *Tx, prefix string) error {
	if _, err := tx.Exec(`UPDATE super_teams SET updated_at = updated_at WHERE prefix = ?`, prefix); err != nil {
		return err
	}
	var exists int
	err := tx.QueryRow(`SELECT 1 FROM super_teams WHERE prefix = ?`, prefix).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrSuperTeamNotFound
	}
	return err
}
