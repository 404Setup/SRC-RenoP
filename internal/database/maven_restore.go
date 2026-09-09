/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"

	"renop/internal/core"
)

type mavenRestoreSnapshot struct {
	Domain    string `json:"domain"`
	Code      string `json:"code"`
	CreatedAt int64  `json:"created_at"`
	HoldAt    int64  `json:"hold_at"`
}

func mavenRestoreStateTx(tx *Tx, repository, groupID, artifactID, actorID string) (mavenRestoreSnapshot, string, string, error) {
	var snapshot mavenRestoreSnapshot
	if err := lockAccountLoginMethodsTx(tx, actorID); err != nil {
		return snapshot, "", "", core.ErrReviewPermissionDenied
	}
	var live int
	now := time.Now().UnixMilli()
	if err := tx.QueryRow(`SELECT COUNT(*) FROM user_profiles p JOIN tokens t ON t.name = p.username
		WHERE p.user_id = ? AND t.deleted_at = 0 AND (t.expires_at IS NULL OR t.expires_at > ?)
		AND (t.banned_at = 0 OR (t.banned_until > 0 AND t.banned_until <= ?))`, actorID, now, now).Scan(&live); err != nil {
		return snapshot, "", "", err
	}
	if live != 1 {
		return snapshot, "", "", core.ErrReviewPermissionDenied
	}
	var sourceTeam, targetTeam string
	var mirrored, verified int
	var closedAt int64
	if err := tx.QueryRow(`SELECT domain FROM maven_artifacts WHERE repository = ? AND group_id = ? AND artifact_id = ?`,
		repository, groupID, artifactID).Scan(&snapshot.Domain); err != nil {
		return snapshot, "", "", reviewResourceError(err)
	}
	if err := lockMavenDomain(tx, snapshot.Domain); err != nil {
		return snapshot, "", "", err
	}
	if err := requireMavenMemberPermission(tx, snapshot.Domain, actorID, core.MavenPermissionOwner); err != nil {
		return snapshot, "", "", core.ErrReviewPermissionDenied
	}
	if err := tx.QueryRow(`SELECT a.reclaim_hold_at, a.super_team_prefix, a.mirrored,
		d.verification_code, d.created_at, d.super_team_prefix, d.verified, d.closed_at
		FROM maven_artifacts a JOIN maven_domains d ON d.repository = '' AND d.domain = a.domain
		WHERE a.repository = ? AND a.group_id = ? AND a.artifact_id = ?`, repository, groupID, artifactID).Scan(
		&snapshot.HoldAt, &sourceTeam, &mirrored, &snapshot.Code, &snapshot.CreatedAt,
		&targetTeam, &verified, &closedAt); err != nil {
		return snapshot, "", "", reviewResourceError(err)
	}
	if snapshot.HoldAt == 0 || mirrored != 0 || verified == 0 || closedAt != 0 {
		return snapshot, "", "", core.ErrReviewResourceConflict
	}
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM package_deprecations WHERE format = 'maven' AND repository = ? AND package_key = ?`,
		repository, groupID+":"+artifactID).Scan(&exists); err != nil {
		return snapshot, "", "", err
	}
	if exists != 0 {
		return snapshot, "", "", core.ErrPackageDeprecated
	}
	err := tx.QueryRow(`SELECT 1 FROM `+resourceLocksQuery("maven")+`
		WHERE format = 'maven' AND repository = ? AND resource_name = ? AND id <> 'maven-reclaim' LIMIT 1`,
		repository, groupID+":"+artifactID).Scan(&exists)
	if err == nil {
		return snapshot, "", "", core.ErrResourceLocked
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return snapshot, "", "", err
	}
	return snapshot, sourceTeam, targetTeam, nil
}

// CreateMavenRestoreReview requests publication rights for an artifact retained across domain reclamation.
func (db *DB) CreateMavenRestoreReview(repository, groupID, artifactID, actor, session string, createdAt int64) (*core.ReviewTask, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, groupID = sanitizeMavenRepository(repository), sanitizeMavenDomain(groupID)
	artifactID = SanitizeInputString(strings.TrimSpace(artifactID), 255)
	if repository == "" || groupID == "" || artifactID == "" || createdAt <= 0 {
		return nil, core.ErrReviewInvalidRequest
	}
	reviewTaskMutationLock.Lock()
	defer reviewTaskMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	account, err := accountEmailSessionTx(tx, sanitizeMavenUsername(actor), session)
	if err != nil {
		return nil, core.ErrReviewPermissionDenied
	}
	snapshot, sourceTeam, targetTeam, err := mavenRestoreStateTx(tx, repository, groupID, artifactID, account.UserID)
	if err != nil {
		return nil, err
	}
	key := groupID + ":" + artifactID
	digest := sha256.Sum256([]byte(core.ReviewKindMavenRestore + "\x00" + repository + "\x00" + key))
	activeKey := hex.EncodeToString(digest[:])
	var existing string
	if err := tx.QueryRow(`SELECT id FROM review_tasks WHERE active_key = ? AND status = ?`, activeKey, core.ReviewStatusPending).Scan(&existing); err == nil {
		return nil, core.ErrReviewTaskExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var total, owned int
	if err := tx.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN requested_by_id = ? THEN 1 ELSE 0 END), 0)
		FROM review_tasks WHERE kind = ? AND status = ?`, account.UserID, core.ReviewKindMavenRestore, core.ReviewStatusPending).Scan(&total, &owned); err != nil {
		return nil, err
	}
	if total >= maxPendingPublicationReviews || owned >= maxPendingPublicationPerAccount {
		return nil, core.ErrReviewFileLimit
	}
	task := &core.ReviewTask{ID: uuid.NewString(), Kind: core.ReviewKindMavenRestore,
		ResourceType: core.ReviewResourceMavenArtifact, Repository: repository, ResourceKey: key, ResourceName: key,
		SourceTeamPrefix: sourceTeam, TargetTeamPrefix: targetTeam, RequestedByID: account.UserID,
		RequestedBy: sanitizeMavenUsername(actor), Status: core.ReviewStatusPending, CreatedAt: createdAt, ActiveKey: activeKey}
	if _, err := tx.Exec(`INSERT INTO review_tasks
		(id, kind, resource_type, repository, resource_key, resource_name, source_team_prefix,
		target_team_prefix, review_team_prefix, requested_by_id, requested_by_name, status,
		decision_reason, decided_by_id, decided_by_name, created_at, decided_at, active_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, '', '', '', ?, 0, ?)`, task.ID, task.Kind,
		task.ResourceType, task.Repository, key, key, sourceTeam, targetTeam, task.RequestedByID,
		task.RequestedBy, task.Status, createdAt, activeKey); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	if err := savePublicationReviewPayloadTx(tx, task.ID, payload); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return task, nil
}

func applyMavenRestoreTx(tx *Tx, task *core.ReviewTask) error {
	groupID, artifactID, valid := splitMavenArtifactReviewKey(task.ResourceKey)
	if !valid {
		return core.ErrReviewResourceConflict
	}
	var encoded string
	if err := tx.QueryRow(`SELECT payload_json FROM review_task_payloads WHERE task_id = ?`, task.ID).Scan(&encoded); err != nil {
		return reviewResourceError(err)
	}
	var expected mavenRestoreSnapshot
	if len(encoded) > 2048 || json.Unmarshal([]byte(encoded), &expected) != nil {
		return core.ErrReviewResourceConflict
	}
	current, sourceTeam, targetTeam, err := mavenRestoreStateTx(tx, task.Repository, groupID, artifactID, task.RequestedByID)
	if err != nil {
		return err
	}
	if current != expected || sourceTeam != task.SourceTeamPrefix || targetTeam != task.TargetTeamPrefix {
		return core.ErrReviewResourceConflict
	}
	result, err := tx.Exec(`UPDATE maven_artifacts SET reclaim_hold_at = 0, super_team_prefix = ?
		WHERE repository = ? AND group_id = ? AND artifact_id = ? AND domain = ? AND reclaim_hold_at = ?`,
		targetTeam, task.Repository, groupID, artifactID, expected.Domain, expected.HoldAt)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return core.ErrReviewResourceConflict
	}
	return nil
}
