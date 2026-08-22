/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"renop/internal/core"
)

const gpgReleaseColumns = `id, active_key, repository, artifact_path, uploader, status, failure_reason,
	require_signature, artifact_staging_path, signature_staging_path, artifact_size, artifact_mod_time,
	signature_size, signature_mod_time, artifact_existed, signature_existed,
	artifact_generate_checksums, signature_generate_checksums, artifact_md5, artifact_sha1,
	artifact_sha256, artifact_sha512, signature_md5, signature_sha1, signature_sha256,
	signature_sha512, publish_started, created_at, updated_at, completed_at, cleanup_pending`

func scanGPGRelease(row rowScanner) (*core.GPGRelease, error) {
	release := &core.GPGRelease{}
	var activeKey sql.NullString
	if err := row.Scan(
		&release.ID,
		&activeKey,
		&release.Repository,
		&release.ArtifactPath,
		&release.Uploader,
		&release.Status,
		&release.FailureReason,
		&release.RequireSignature,
		&release.ArtifactStagingPath,
		&release.SignatureStagingPath,
		&release.ArtifactSize,
		&release.ArtifactModTime,
		&release.SignatureSize,
		&release.SignatureModTime,
		&release.ArtifactExisted,
		&release.SignatureExisted,
		&release.ArtifactGenerateChecksums,
		&release.SignatureGenerateChecksums,
		&release.ArtifactMD5,
		&release.ArtifactSHA1,
		&release.ArtifactSHA256,
		&release.ArtifactSHA512,
		&release.SignatureMD5,
		&release.SignatureSHA1,
		&release.SignatureSHA256,
		&release.SignatureSHA512,
		&release.PublishStarted,
		&release.CreatedAt,
		&release.UpdatedAt,
		&release.CompletedAt,
		&release.CleanupPending,
	); err != nil {
		return nil, err
	}
	if activeKey.Valid {
		release.ActiveKey = activeKey.String
	}
	return release, nil
}

func normalizeGPGRelease(release *core.GPGRelease) error {
	if release == nil {
		return errors.New("GPG release is nil")
	}
	release.ID = SanitizeInputString(strings.TrimSpace(release.ID), 36)
	release.ActiveKey = SanitizeInputString(strings.TrimSpace(release.ActiveKey), 64)
	release.Repository = SanitizeInputString(strings.TrimSpace(release.Repository), 255)
	release.ArtifactPath = SanitizeInputString(strings.TrimSpace(release.ArtifactPath), 4096)
	release.Uploader = strings.ToLower(SanitizeInputString(strings.TrimSpace(release.Uploader), 255))
	release.Status = strings.ToLower(SanitizeInputString(strings.TrimSpace(release.Status), 16))
	release.FailureReason = SanitizeInputString(strings.TrimSpace(release.FailureReason), 512)
	release.ArtifactStagingPath = SanitizeInputString(release.ArtifactStagingPath, 8192)
	release.SignatureStagingPath = SanitizeInputString(release.SignatureStagingPath, 8192)
	if release.ID == "" || release.Repository == "" || release.ArtifactPath == "" || release.Uploader == "" {
		return errors.New("GPG release identity is incomplete")
	}
	switch release.Status {
	case core.GPGReleaseQueued, core.GPGReleaseValidating, core.GPGReleaseFailed, core.GPGReleaseSuccess:
	default:
		return errors.New("invalid GPG release status")
	}
	if release.CreatedAt <= 0 {
		release.CreatedAt = time.Now().UnixMilli()
	}
	if release.UpdatedAt <= 0 {
		release.UpdatedAt = release.CreatedAt
	}
	return nil
}

func nullableReleaseKey(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func gpgReleaseValues(release *core.GPGRelease) []any {
	return []any{
		release.ID,
		nullableReleaseKey(release.ActiveKey),
		release.Repository,
		release.ArtifactPath,
		release.Uploader,
		release.Status,
		release.FailureReason,
		release.RequireSignature,
		release.ArtifactStagingPath,
		release.SignatureStagingPath,
		release.ArtifactSize,
		release.ArtifactModTime,
		release.SignatureSize,
		release.SignatureModTime,
		release.ArtifactExisted,
		release.SignatureExisted,
		release.ArtifactGenerateChecksums,
		release.SignatureGenerateChecksums,
		release.ArtifactMD5,
		release.ArtifactSHA1,
		release.ArtifactSHA256,
		release.ArtifactSHA512,
		release.SignatureMD5,
		release.SignatureSHA1,
		release.SignatureSHA256,
		release.SignatureSHA512,
		release.PublishStarted,
		release.CreatedAt,
		release.UpdatedAt,
		release.CompletedAt,
		release.CleanupPending,
	}
}

func (db *DB) SaveGPGRelease(release *core.GPGRelease) error {
	if db == nil || db.SqlDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if err := normalizeGPGRelease(release); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin GPG release update: %w", err)
	}
	defer tx.Rollback()

	values := gpgReleaseValues(release)
	updateArgs := append(append([]any(nil), values[1:]...), release.ID)
	result, err := tx.Exec(`UPDATE gpg_releases SET active_key = ?, repository = ?, artifact_path = ?, uploader = ?,
		status = ?, failure_reason = ?, require_signature = ?, artifact_staging_path = ?, signature_staging_path = ?,
		artifact_size = ?, artifact_mod_time = ?, signature_size = ?, signature_mod_time = ?, artifact_existed = ?,
		signature_existed = ?, artifact_generate_checksums = ?, signature_generate_checksums = ?, artifact_md5 = ?,
		artifact_sha1 = ?, artifact_sha256 = ?, artifact_sha512 = ?, signature_md5 = ?, signature_sha1 = ?,
		signature_sha256 = ?, signature_sha512 = ?, publish_started = ?, created_at = ?, updated_at = ?, completed_at = ?, cleanup_pending = ?
		WHERE id = ?`, updateArgs...)
	if err != nil {
		return fmt.Errorf("failed to update GPG release: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect GPG release update: %w", err)
	}
	if rows == 0 {
		var existingID string
		err = tx.QueryRow(`SELECT id FROM gpg_releases WHERE id = ?`, release.ID).Scan(&existingID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("failed to check GPG release existence: %w", err)
		}
		if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.Exec(`INSERT INTO gpg_releases (`+gpgReleaseColumns+`) VALUES (
				?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, values...)
			if err != nil {
				return fmt.Errorf("failed to create GPG release: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit GPG release update: %w", err)
	}
	return nil
}

func (db *DB) GetActiveGPGRelease(activeKey string) (*core.GPGRelease, error) {
	activeKey = SanitizeInputString(strings.TrimSpace(activeKey), 64)
	if db == nil || db.SqlDB == nil || activeKey == "" {
		return nil, nil
	}
	release, err := scanGPGRelease(db.QueryRow(
		`SELECT `+gpgReleaseColumns+` FROM gpg_releases WHERE active_key = ?`, activeKey,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active GPG release: %w", err)
	}
	return release, nil
}

func (db *DB) ClaimNextGPGRelease(optionalReadyBefore int64) (*core.GPGRelease, error) {
	if db == nil || db.SqlDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin GPG release claim: %w", err)
	}
	defer tx.Rollback()

	release, err := scanGPGRelease(tx.QueryRow(`SELECT `+gpgReleaseColumns+` FROM gpg_releases
		WHERE status = ? AND (
			(signature_staging_path <> '' AND artifact_staging_path <> '')
			OR (signature_staging_path <> '' AND artifact_staging_path = '' AND artifact_existed = 1 AND created_at <= ?)
			OR (artifact_staging_path <> '' AND signature_staging_path = '' AND require_signature = 0 AND created_at <= ?)
		) ORDER BY created_at, id LIMIT 1`, core.GPGReleaseQueued, optionalReadyBefore, optionalReadyBefore))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to select queued GPG release: %w", err)
	}
	now := time.Now().UnixMilli()
	result, err := tx.Exec(`UPDATE gpg_releases SET status = ?, updated_at = ? WHERE id = ? AND status = ?`,
		core.GPGReleaseValidating, now, release.ID, core.GPGReleaseQueued)
	if err != nil {
		return nil, fmt.Errorf("failed to claim queued GPG release: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect queued GPG release claim: %w", err)
	}
	if rows != 1 {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit queued GPG release claim: %w", err)
	}
	release.Status = core.GPGReleaseValidating
	release.UpdatedAt = now
	return release, nil
}

func (db *DB) ListGPGReleases(username string, limit, offset int) ([]*core.GPGRelease, int, error) {
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), 255))
	if db == nil || db.SqlDB == nil || username == "" {
		return []*core.GPGRelease{}, 0, nil
	}
	limit = min(max(limit, 1), 100)
	offset = max(offset, 0)
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gpg_releases WHERE uploader = ?`, username).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count GPG releases: %w", err)
	}
	rows, err := db.Query(`SELECT `+gpgReleaseColumns+` FROM gpg_releases
		WHERE uploader = ? ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, username, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list GPG releases: %w", err)
	}
	defer rows.Close()
	releases := make([]*core.GPGRelease, 0, min(limit, total))
	for rows.Next() {
		release, scanErr := scanGPGRelease(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("failed to scan GPG release: %w", scanErr)
		}
		releases = append(releases, release)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate GPG releases: %w", err)
	}
	return releases, total, nil
}

func (db *DB) ListPendingGPGReleases() ([]*core.GPGRelease, error) {
	if db == nil || db.SqlDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	rows, err := db.Query(`SELECT ` + gpgReleaseColumns + ` FROM gpg_releases
		WHERE active_key IS NOT NULL OR cleanup_pending = 1 ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending GPG releases: %w", err)
	}
	defer rows.Close()
	var releases []*core.GPGRelease
	for rows.Next() {
		release, scanErr := scanGPGRelease(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan pending GPG release: %w", scanErr)
		}
		releases = append(releases, release)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate pending GPG releases: %w", err)
	}
	return releases, nil
}

func (db *DB) CountPendingGPGReleases(username string) (int, int, error) {
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), 255))
	if db == nil || db.SqlDB == nil {
		return 0, 0, core.ErrDatabaseUnavailable
	}
	var total, perUser int
	err := db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN uploader = ? THEN 1 ELSE 0 END), 0)
		FROM gpg_releases WHERE active_key IS NOT NULL`, username).Scan(&total, &perUser)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count pending GPG releases: %w", err)
	}
	return total, perUser, nil
}

func (db *DB) ResetValidatingGPGReleases() error {
	if db == nil || db.SqlDB == nil {
		return core.ErrDatabaseUnavailable
	}
	_, err := db.Exec(`UPDATE gpg_releases SET status = ?, updated_at = ? WHERE status = ? AND active_key IS NOT NULL`,
		core.GPGReleaseQueued, time.Now().UnixMilli(), core.GPGReleaseValidating)
	if err != nil {
		return fmt.Errorf("failed to recover validating GPG releases: %w", err)
	}
	return nil
}
