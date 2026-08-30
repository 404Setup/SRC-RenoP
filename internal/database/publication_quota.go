/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"renop/internal/core"
)

const publicationQuotaUsageRetention = 45 * 24 * time.Hour

var publicationQuotaMutationLock sync.Mutex

type publicationQuotaStoredOverride struct {
	fileLimit        int64
	byteLimit        int64
	publicationLimit int64
	period           string
	unlimited        int64
}

type publicationQuotaUsage struct {
	files        int64
	bytes        int64
	publications int64
}

func publicationQuotaLock(subject core.PublicationQuotaSubject) *sync.Mutex {
	_ = subject
	return &publicationQuotaMutationLock
}

func validatePublicationQuotaLimits(limits core.PublicationQuotaLimits) bool {
	return limits.FileLimit >= 0 && limits.ByteLimit >= 0 && limits.PublicationLimit >= 0 &&
		core.ValidPublicationQuotaPeriod(limits.Period)
}

func normalizePublicationQuotaSubjectTx(tx *Tx, subject core.PublicationQuotaSubject,
	createMissingUser bool,
) (core.PublicationQuotaSubject, error) {
	subject.OwnerType = strings.ToLower(strings.TrimSpace(subject.OwnerType))
	subject.OwnerKey = strings.ToLower(strings.TrimSpace(subject.OwnerKey))
	if subject.OwnerKey == "" {
		return core.PublicationQuotaSubject{}, core.ErrPublicationQuotaInvalid
	}
	switch subject.OwnerType {
	case core.PublicationQuotaOwnerUser:
		userID, err := userIDForUsernameTx(tx, subject.OwnerKey)
		if errors.Is(err, core.ErrUserProfileNotFound) && createMissingUser {
			userID = uuid.NewString()
			if _, insertErr := tx.Exec(`INSERT INTO user_profiles
				(user_id, username, nickname, rename_window_started_at, rename_count, updated_at)
				VALUES (?, ?, '', 0, 0, ?)`, userID, subject.OwnerKey, time.Now().UnixMilli()); insertErr != nil {
				return core.PublicationQuotaSubject{}, fmt.Errorf("create publication quota user identity: %w", insertErr)
			}
			err = nil
		}
		if err != nil {
			return core.PublicationQuotaSubject{}, err
		}
		subject.OwnerKey = userID
	case core.PublicationQuotaOwnerSuperTeam:
		prefix, valid := sanitizeSuperTeamPrefix(subject.OwnerKey)
		if !valid {
			return core.PublicationQuotaSubject{}, core.ErrSuperTeamNotFound
		}
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM super_teams WHERE prefix = ?`, prefix).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return core.PublicationQuotaSubject{}, core.ErrSuperTeamNotFound
		} else if err != nil {
			return core.PublicationQuotaSubject{}, fmt.Errorf("inspect publication quota team: %w", err)
		}
		subject.OwnerKey = prefix
	default:
		return core.PublicationQuotaSubject{}, core.ErrPublicationQuotaInvalid
	}
	return subject, nil
}

func effectivePublicationQuotaTx(tx *Tx, subject core.PublicationQuotaSubject,
	defaults core.PublicationQuotaLimits,
) (core.PublicationQuotaLimits, bool, bool, error) {
	if !validatePublicationQuotaLimits(defaults) {
		return core.PublicationQuotaLimits{}, false, false, core.ErrPublicationQuotaInvalid
	}
	stored := publicationQuotaStoredOverride{}
	err := tx.QueryRow(`SELECT file_limit, byte_limit, publication_limit, quota_period, unlimited
		FROM publication_quota_overrides WHERE owner_type = ? AND owner_key = ?`,
		subject.OwnerType, subject.OwnerKey).Scan(&stored.fileLimit, &stored.byteLimit,
		&stored.publicationLimit, &stored.period, &stored.unlimited)
	if errors.Is(err, sql.ErrNoRows) {
		return defaults, true, false, nil
	}
	if err != nil {
		return core.PublicationQuotaLimits{}, false, false, fmt.Errorf("load publication quota override: %w", err)
	}
	effective := defaults
	if stored.fileLimit >= 0 {
		effective.FileLimit = stored.fileLimit
	}
	if stored.byteLimit >= 0 {
		effective.ByteLimit = stored.byteLimit
	}
	if stored.publicationLimit >= 0 {
		effective.PublicationLimit = stored.publicationLimit
	}
	if core.ValidPublicationQuotaPeriod(stored.period) {
		effective.Period = stored.period
	}
	return effective, false, stored.unlimited == 1, nil
}

func loadPublicationQuotaUsageTx(tx *Tx, subject core.PublicationQuotaSubject, periodStart int64) (publicationQuotaUsage, error) {
	var usage publicationQuotaUsage
	err := tx.QueryRow(`SELECT files_used, bytes_used, publications_used FROM publication_quota_usage
		WHERE owner_type = ? AND owner_key = ? AND period_start = ?`,
		subject.OwnerType, subject.OwnerKey, periodStart).Scan(&usage.files, &usage.bytes, &usage.publications)
	if errors.Is(err, sql.ErrNoRows) {
		return publicationQuotaUsage{}, nil
	}
	if err != nil {
		return publicationQuotaUsage{}, fmt.Errorf("load publication quota usage: %w", err)
	}
	return usage, nil
}

func loadPublicationQuotaReservationsTx(tx *Tx, subject core.PublicationQuotaSubject,
	periodStart, now int64,
) (publicationQuotaUsage, error) {
	var reserved publicationQuotaUsage
	if err := tx.QueryRow(`SELECT COALESCE(SUM(files_reserved), 0), COALESCE(SUM(bytes_reserved), 0),
		COALESCE(SUM(publications_reserved), 0) FROM publication_quota_reservations
		WHERE owner_type = ? AND owner_key = ? AND period_start = ? AND expires_at > ?`,
		subject.OwnerType, subject.OwnerKey, periodStart, now).Scan(
		&reserved.files, &reserved.bytes, &reserved.publications); err != nil {
		return publicationQuotaUsage{}, fmt.Errorf("load publication quota reservations: %w", err)
	}
	return reserved, nil
}

func publicationQuotaExceeded(used, reserved, requested, limit int64) bool {
	if used < 0 || reserved < 0 || requested < 0 || limit < 0 || used > limit || reserved > limit-used {
		return true
	}
	return requested > limit-used-reserved
}

// GetPublicationQuotaStatus returns effective limits and current committed and reserved usage.
func (db *DB) GetPublicationQuotaStatus(subject core.PublicationQuotaSubject, defaults core.PublicationQuotaLimits,
	now int64,
) (*core.PublicationQuotaStatus, error) {
	if db == nil || db.SQLDB == nil || now <= 0 {
		return nil, core.ErrDatabaseUnavailable
	}
	lock := publicationQuotaLock(subject)
	lock.Lock()
	defer lock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin publication quota lookup: %w", err)
	}
	defer tx.Rollback()
	resolved, err := normalizePublicationQuotaSubjectTx(tx, subject, false)
	if err != nil {
		return nil, err
	}
	limits, inherited, unlimited, err := effectivePublicationQuotaTx(tx, resolved, defaults)
	if err != nil {
		return nil, err
	}
	periodStart, periodEnd, valid := core.PublicationQuotaWindow(limits.Period, time.UnixMilli(now))
	if !valid {
		return nil, core.ErrPublicationQuotaInvalid
	}
	usage, err := loadPublicationQuotaUsageTx(tx, resolved, periodStart.UnixMilli())
	if err != nil {
		return nil, err
	}
	reserved, err := loadPublicationQuotaReservationsTx(tx, resolved, periodStart.UnixMilli(), now)
	if err != nil {
		return nil, err
	}
	return &core.PublicationQuotaStatus{
		OwnerType: subject.OwnerType, OwnerKey: subject.OwnerKey,
		FileLimit: limits.FileLimit, ByteLimit: limits.ByteLimit, PublicationLimit: limits.PublicationLimit,
		Period: limits.Period, PeriodStart: periodStart.UnixMilli(), PeriodEnd: periodEnd.UnixMilli(),
		FilesUsed: usage.files, BytesUsed: usage.bytes, PublicationsUsed: usage.publications,
		FilesReserved: reserved.files, BytesReserved: reserved.bytes, PublicationsReserved: reserved.publications,
		Inherited: inherited, Unlimited: unlimited,
	}, nil
}

// SetPublicationQuotaOverride stores nullable per-owner policy overrides.
func (db *DB) SetPublicationQuotaOverride(subject core.PublicationQuotaSubject,
	override core.PublicationQuotaOverride, updatedAt int64,
) error {
	if db == nil || db.SQLDB == nil || updatedAt <= 0 {
		return core.ErrDatabaseUnavailable
	}
	lock := publicationQuotaLock(subject)
	lock.Lock()
	defer lock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin publication quota override: %w", err)
	}
	defer tx.Rollback()
	resolved, err := normalizePublicationQuotaSubjectTx(tx, subject, false)
	if err != nil {
		return err
	}
	if override.FileLimit == nil && override.ByteLimit == nil && override.PublicationLimit == nil &&
		override.Period == nil && override.Unlimited == nil {
		if _, err := tx.Exec(`DELETE FROM publication_quota_overrides WHERE owner_type = ? AND owner_key = ?`,
			resolved.OwnerType, resolved.OwnerKey); err != nil {
			return fmt.Errorf("clear publication quota override: %w", err)
		}
		return tx.Commit()
	}
	fileLimit, byteLimit, publicationLimit, period, unlimited := int64(-1), int64(-1), int64(-1), "", int64(-1)
	if override.FileLimit != nil {
		fileLimit = *override.FileLimit
	}
	if override.ByteLimit != nil {
		byteLimit = *override.ByteLimit
	}
	if override.PublicationLimit != nil {
		publicationLimit = *override.PublicationLimit
	}
	if override.Period != nil {
		period = strings.ToLower(strings.TrimSpace(*override.Period))
	}
	if override.Unlimited != nil {
		if *override.Unlimited {
			unlimited = 1
		} else {
			unlimited = 0
		}
	}
	if fileLimit < -1 || byteLimit < -1 || publicationLimit < -1 ||
		period != "" && !core.ValidPublicationQuotaPeriod(period) {
		return core.ErrPublicationQuotaInvalid
	}
	var exists int
	err = tx.QueryRow(`SELECT 1 FROM publication_quota_overrides WHERE owner_type = ? AND owner_key = ?`,
		resolved.OwnerType, resolved.OwnerKey).Scan(&exists)
	switch {
	case err == nil:
		_, err = tx.Exec(`UPDATE publication_quota_overrides SET file_limit = ?, byte_limit = ?,
			publication_limit = ?, quota_period = ?, unlimited = ?, updated_at = ?
			WHERE owner_type = ? AND owner_key = ?`, fileLimit, byteLimit, publicationLimit, period,
			unlimited, updatedAt, resolved.OwnerType, resolved.OwnerKey)
	case errors.Is(err, sql.ErrNoRows):
		_, err = tx.Exec(`INSERT INTO publication_quota_overrides
			(owner_type, owner_key, file_limit, byte_limit, publication_limit, quota_period, unlimited, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, resolved.OwnerType, resolved.OwnerKey, fileLimit,
			byteLimit, publicationLimit, period, unlimited, updatedAt)
	}
	if err != nil {
		return fmt.Errorf("store publication quota override: %w", err)
	}
	return tx.Commit()
}

// ReservePublicationQuota atomically creates a short-lived quota claim.
func (db *DB) ReservePublicationQuota(subject core.PublicationQuotaSubject, defaults core.PublicationQuotaLimits,
	delta core.PublicationQuotaDelta, now, expiresAt int64,
) (*core.PublicationQuotaReservation, error) {
	if db == nil || db.SQLDB == nil || now <= 0 || expiresAt <= now || delta.Files < 0 || delta.Bytes < 0 ||
		delta.Publications < 0 || delta.Files == 0 && delta.Bytes == 0 && delta.Publications == 0 {
		return nil, core.ErrPublicationQuotaInvalid
	}
	lock := publicationQuotaLock(subject)
	lock.Lock()
	defer lock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin publication quota reservation: %w", err)
	}
	defer tx.Rollback()
	resolved, err := normalizePublicationQuotaSubjectTx(tx, subject, true)
	if err != nil {
		return nil, err
	}
	limits, _, unlimited, err := effectivePublicationQuotaTx(tx, resolved, defaults)
	if err != nil {
		return nil, err
	}
	if unlimited {
		return &core.PublicationQuotaReservation{Unlimited: true}, nil
	}
	periodStart, _, valid := core.PublicationQuotaWindow(limits.Period, time.UnixMilli(now))
	if !valid {
		return nil, core.ErrPublicationQuotaInvalid
	}
	startMillis := periodStart.UnixMilli()
	usage, err := loadPublicationQuotaUsageTx(tx, resolved, startMillis)
	if err != nil {
		return nil, err
	}
	reserved, err := loadPublicationQuotaReservationsTx(tx, resolved, startMillis, now)
	if err != nil {
		return nil, err
	}
	switch {
	case publicationQuotaExceeded(usage.files, reserved.files, delta.Files, limits.FileLimit):
		return nil, core.ErrPublicationFileLimit
	case publicationQuotaExceeded(usage.bytes, reserved.bytes, delta.Bytes, limits.ByteLimit):
		return nil, core.ErrPublicationByteLimit
	case publicationQuotaExceeded(usage.publications, reserved.publications, delta.Publications, limits.PublicationLimit):
		return nil, core.ErrPublicationCountLimit
	}
	id := uuid.NewString()
	if _, err := tx.Exec(`INSERT INTO publication_quota_reservations
		(id, owner_type, owner_key, period_start, files_reserved, bytes_reserved, publications_reserved, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, resolved.OwnerType, resolved.OwnerKey, startMillis,
		delta.Files, delta.Bytes, delta.Publications, expiresAt, now); err != nil {
		return nil, fmt.Errorf("create publication quota reservation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit publication quota reservation: %w", err)
	}
	return &core.PublicationQuotaReservation{ID: id}, nil
}

// CommitPublicationQuotaReservation moves one durable reservation into committed usage.
func (db *DB) CommitPublicationQuotaReservation(id string, committedAt int64) error {
	if db == nil || db.SQLDB == nil || strings.TrimSpace(id) == "" || committedAt <= 0 {
		return core.ErrPublicationQuotaInvalid
	}
	subject := core.PublicationQuotaSubject{OwnerType: "reservation", OwnerKey: id}
	lock := publicationQuotaLock(subject)
	lock.Lock()
	defer lock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin publication quota commit: %w", err)
	}
	defer tx.Rollback()
	var ownerType, ownerKey string
	var periodStart, files, bytes, publications int64
	err = tx.QueryRow(`SELECT owner_type, owner_key, period_start, files_reserved, bytes_reserved, publications_reserved
		FROM publication_quota_reservations WHERE id = ?`, id).Scan(
		&ownerType, &ownerKey, &periodStart, &files, &bytes, &publications)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrPublicationQuotaReservation
	}
	if err != nil {
		return fmt.Errorf("load publication quota reservation: %w", err)
	}
	usage, err := loadPublicationQuotaUsageTx(tx,
		core.PublicationQuotaSubject{OwnerType: ownerType, OwnerKey: ownerKey}, periodStart)
	if err != nil {
		return err
	}
	if usage.files > int64(^uint64(0)>>1)-files || usage.bytes > int64(^uint64(0)>>1)-bytes ||
		usage.publications > int64(^uint64(0)>>1)-publications {
		return core.ErrPublicationQuotaInvalid
	}
	var exists int
	err = tx.QueryRow(`SELECT 1 FROM publication_quota_usage WHERE owner_type = ? AND owner_key = ? AND period_start = ?`,
		ownerType, ownerKey, periodStart).Scan(&exists)
	switch {
	case err == nil:
		_, err = tx.Exec(`UPDATE publication_quota_usage SET files_used = ?, bytes_used = ?,
			publications_used = ?, updated_at = ? WHERE owner_type = ? AND owner_key = ? AND period_start = ?`,
			usage.files+files, usage.bytes+bytes, usage.publications+publications, committedAt,
			ownerType, ownerKey, periodStart)
	case errors.Is(err, sql.ErrNoRows):
		_, err = tx.Exec(`INSERT INTO publication_quota_usage
			(owner_type, owner_key, period_start, files_used, bytes_used, publications_used, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, ownerType, ownerKey, periodStart, files, bytes, publications, committedAt)
	}
	if err != nil {
		return fmt.Errorf("store publication quota usage: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM publication_quota_reservations WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete publication quota reservation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit publication quota usage: %w", err)
	}
	return nil
}

// ReleasePublicationQuotaReservation removes a failed publication claim idempotently.
func (db *DB) ReleasePublicationQuotaReservation(id string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	lock := publicationQuotaLock(core.PublicationQuotaSubject{OwnerType: "reservation", OwnerKey: id})
	lock.Lock()
	defer lock.Unlock()
	if _, err := db.Exec(`DELETE FROM publication_quota_reservations WHERE id = ?`, id); err != nil {
		return fmt.Errorf("release publication quota reservation: %w", err)
	}
	return nil
}

// CleanExpiredPublicationQuotaReservations removes abandoned claims and obsolete usage windows.
func (db *DB) CleanExpiredPublicationQuotaReservations(now int64) error {
	if db == nil || db.SQLDB == nil || now <= 0 {
		return core.ErrDatabaseUnavailable
	}
	if _, err := db.Exec(`DELETE FROM publication_quota_reservations WHERE expires_at <= ?`, now); err != nil {
		return fmt.Errorf("clean publication quota reservations: %w", err)
	}
	cutoff := now - publicationQuotaUsageRetention.Milliseconds()
	if _, err := db.Exec(`DELETE FROM publication_quota_usage WHERE period_start < ?`, cutoff); err != nil {
		return fmt.Errorf("clean publication quota usage: %w", err)
	}
	return nil
}
