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
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"renop/internal/config"
	"renop/internal/core"
)

const (
	maxPublicationReviewFiles       = 256
	maxPendingPublicationReviews    = 4096
	maxPendingPublicationPerAccount = 64
	maxPublicationReviewPayload     = 6 << 20
)

func publicationReviewKey(resourceKey, version string) string {
	return "v1." + base64.RawURLEncoding.EncodeToString([]byte(resourceKey)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(version))
}

func decodePublicationReviewKey(value string) (string, string, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return "", "", false
	}
	resourceKey, resourceErr := base64.RawURLEncoding.DecodeString(parts[1])
	version, versionErr := base64.RawURLEncoding.DecodeString(parts[2])
	if resourceErr != nil || versionErr != nil || len(resourceKey) == 0 || len(version) == 0 {
		return "", "", false
	}
	return string(resourceKey), string(version), true
}

func publicationReviewActiveKey(resourceType, repository, storedResourceKey string) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		core.ReviewKindPublication, resourceType, repository, storedResourceKey,
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func normalizePublicationReviewFile(file *core.ReviewFile, createdAt int64) (*core.ReviewFile, error) {
	if file == nil {
		return nil, core.ErrReviewInvalidRequest
	}
	path := strings.Trim(strings.ReplaceAll(strings.TrimSpace(file.Path), `\`, "/"), "/")
	if path == "" || len(path) > maxReviewResourceKey || strings.ContainsAny(path, "\x00\r\n") {
		return nil, core.ErrReviewInvalidRequest
	}
	for segment := range strings.SplitSeq(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return nil, core.ErrReviewInvalidRequest
		}
	}
	if file.Size < 0 {
		return nil, core.ErrReviewInvalidRequest
	}
	addedAt := file.AddedAt
	if addedAt <= 0 {
		addedAt = createdAt
	}
	digest := sha256.Sum256([]byte(path))
	name := path
	if separator := strings.LastIndexByte(path, '/'); separator >= 0 {
		name = path[separator+1:]
	}
	return &core.ReviewFile{
		ID: hex.EncodeToString(digest[:]), Path: path, Name: name, Size: file.Size,
		Critical: file.Critical, AddedAt: addedAt,
	}, nil
}

func savePublicationReviewFileTx(tx *Tx, taskID, repository string, file *core.ReviewFile) error {
	for {
		var otherTaskID string
		err := tx.QueryRow(`SELECT f.task_id FROM review_task_files f JOIN review_tasks r ON r.id = f.task_id
			WHERE f.file_id = ? AND f.task_id != ? AND r.kind = ? AND r.status = ? AND r.repository = ? LIMIT 1`,
			file.ID, taskID, core.ReviewKindPublication, core.ReviewStatusPending, repository).Scan(&otherTaskID)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return fmt.Errorf("inspect prior publication review file: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM review_task_files WHERE task_id = ? AND file_id = ?`,
			otherTaskID, file.ID); err != nil {
			return fmt.Errorf("move publication review file: %w", err)
		}
	}
	var exists int
	err := tx.QueryRow(`SELECT 1 FROM review_task_files WHERE task_id = ? AND file_id = ?`,
		taskID, file.ID).Scan(&exists)
	if err == nil {
		_, err = tx.Exec(`UPDATE review_task_files SET path = ?, size = ?, critical = ?, added_at = ?
			WHERE task_id = ? AND file_id = ?`, file.Path, file.Size, boolInt(file.Critical),
			file.AddedAt, taskID, file.ID)
		if err != nil {
			return fmt.Errorf("update publication review file: %w", err)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect publication review file: %w", err)
	}
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM review_task_files WHERE task_id = ?`, taskID).Scan(&count); err != nil {
		return fmt.Errorf("count publication review files: %w", err)
	}
	if count >= maxPublicationReviewFiles {
		return core.ErrReviewFileLimit
	}
	if _, err := tx.Exec(`INSERT INTO review_task_files
		(task_id, file_id, path, size, critical, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
		taskID, file.ID, file.Path, file.Size, boolInt(file.Critical), file.AddedAt); err != nil {
		return fmt.Errorf("create publication review file: %w", err)
	}
	return nil
}

func savePublicationReviewPayloadTx(tx *Tx, taskID string, payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	if len(payload) > maxPublicationReviewPayload {
		return core.ErrReviewInvalidRequest
	}
	var exists int
	err := tx.QueryRow(`SELECT 1 FROM review_task_payloads WHERE task_id = ?`, taskID).Scan(&exists)
	if err == nil {
		if _, err := tx.Exec(`UPDATE review_task_payloads SET payload_json = ? WHERE task_id = ?`,
			string(payload), taskID); err != nil {
			return fmt.Errorf("update publication review payload: %w", err)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect publication review payload: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO review_task_payloads (task_id, payload_json) VALUES (?, ?)`,
		taskID, string(payload)); err != nil {
		return fmt.Errorf("create publication review payload: %w", err)
	}
	return nil
}

// CreateOrUpdatePublicationReview creates one hidden publication review or appends committed files to it.
func (db *DB) CreateOrUpdatePublicationReview(request core.PublicationReviewRequest) (*core.PublicationReviewResult, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	policy, policyValid := config.NormalizePublicationReviewPolicy(request.Policy)
	if !policyValid {
		return nil, core.ErrReviewInvalidRequest
	}
	resourceType, valid := normalizeReviewResourceType(request.ResourceType)
	repository := sanitizeMavenRepository(request.Repository)
	resourceKey := SanitizeInputString(strings.TrimSpace(request.ResourceKey), 512)
	resourceName := SanitizeInputString(strings.TrimSpace(request.ResourceName), 512)
	version := SanitizeInputString(strings.TrimSpace(request.Version), 255)
	actor := sanitizeMavenUsername(request.RequestedBy)
	reviewTeam, reviewTeamErr := normalizeOptionalSuperTeamPrefix(request.ReviewTeamPrefix)
	targetTeam, targetTeamErr := normalizeOptionalSuperTeamPrefix(request.TargetTeamPrefix)
	teamCreationReview := reviewTeam != "" && targetTeam == reviewTeam &&
		version == core.ReviewVersionPackageCreation
	if !valid || repository == "" || resourceKey == "" || resourceName == "" || version == "" ||
		actor == "" || request.CreatedAt <= 0 || len(request.Files) == 0 || len(request.Files) > maxPublicationReviewFiles ||
		reviewTeamErr != nil || targetTeamErr != nil || (reviewTeam == "") != (targetTeam == "") ||
		reviewTeam != "" && (!teamCreationReview || policy == config.PublicationReviewOff) {
		return nil, core.ErrReviewInvalidRequest
	}
	files := make([]*core.ReviewFile, 0, len(request.Files))
	for _, file := range request.Files {
		normalized, err := normalizePublicationReviewFile(file, request.CreatedAt)
		if err != nil {
			return nil, err
		}
		files = append(files, normalized)
	}
	storedResourceKey := publicationReviewKey(resourceKey, version)
	if len(storedResourceKey) > maxReviewResourceKey {
		return nil, core.ErrReviewInvalidRequest
	}
	activeKey := publicationReviewActiveKey(resourceType, repository, storedResourceKey)

	reviewTaskMutationLock.Lock()
	defer reviewTaskMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin publication review: %w", err)
	}
	defer tx.Rollback()
	actorID := ""
	var taskID, taskActorID, taskReviewTeam, taskTargetTeam string
	err = tx.QueryRow(`SELECT id, requested_by_id, review_team_prefix, target_team_prefix
		FROM review_tasks WHERE active_key = ? AND status = ?`,
		activeKey, core.ReviewStatusPending).Scan(&taskID, &taskActorID, &taskReviewTeam, &taskTargetTeam)
	if errors.Is(err, sql.ErrNoRows) {
		if policy == config.PublicationReviewOff {
			return &core.PublicationReviewResult{}, nil
		}
		var approvedID string
		approvedErr := tx.QueryRow(`SELECT id FROM review_tasks WHERE kind = ? AND resource_type = ?
			AND repository = ? AND resource_key = ? AND status = ? LIMIT 1`, core.ReviewKindPublication,
			resourceType, repository, storedResourceKey, core.ReviewStatusApproved).Scan(&approvedID)
		if approvedErr == nil && version != core.ReviewVersionPackageCreation {
			return nil, core.ErrReviewPublicationSealed
		}
		if approvedErr != nil && !errors.Is(approvedErr, sql.ErrNoRows) {
			return nil, fmt.Errorf("inspect approved publication review: %w", approvedErr)
		}
		if policy == config.PublicationReviewNewPackages && request.PackageExists {
			return &core.PublicationReviewResult{}, nil
		}
		var identityErr error
		actorID, identityErr = userIDForUsernameTx(tx, actor)
		if identityErr != nil {
			return nil, core.ErrReviewPermissionDenied
		}
		if teamCreationReview {
			role, member, roleErr := superTeamRoleTx(tx, reviewTeam, actorID)
			if roleErr != nil {
				return nil, roleErr
			}
			if !member || role != core.SuperTeamRoleWrite {
				return nil, core.ErrReviewPermissionDenied
			}
		}
		var pendingTotal, pendingForActor int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM review_tasks WHERE kind = ? AND status = ?`,
			core.ReviewKindPublication, core.ReviewStatusPending).Scan(&pendingTotal); err != nil {
			return nil, fmt.Errorf("count pending publication reviews: %w", err)
		}
		if err := tx.QueryRow(`SELECT COUNT(*) FROM review_tasks WHERE kind = ? AND status = ? AND requested_by_id = ?`,
			core.ReviewKindPublication, core.ReviewStatusPending, actorID).Scan(&pendingForActor); err != nil {
			return nil, fmt.Errorf("count account publication reviews: %w", err)
		}
		if pendingTotal >= maxPendingPublicationReviews || pendingForActor >= maxPendingPublicationPerAccount {
			return nil, core.ErrReviewFileLimit
		}
		taskID = uuid.NewString()
		if _, err := tx.Exec(`INSERT INTO review_tasks
			(id, kind, resource_type, repository, resource_key, resource_name, source_team_prefix,
			target_team_prefix, review_team_prefix, requested_by_id, requested_by_name, status,
			decision_reason, decided_by_id, decided_by_name, created_at, decided_at, active_key)
			VALUES (?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, '', '', '', ?, 0, ?)`, taskID,
			core.ReviewKindPublication, resourceType, repository, storedResourceKey, resourceName,
			targetTeam, reviewTeam, actorID, actor, core.ReviewStatusPending, request.CreatedAt, activeKey); err != nil {
			return nil, fmt.Errorf("create publication review: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("inspect pending publication review: %w", err)
	} else {
		var identityErr error
		actorID, identityErr = userIDForUsernameTx(tx, actor)
		if identityErr != nil || taskActorID != actorID {
			return nil, core.ErrReviewPermissionDenied
		}
		if taskReviewTeam == "" && taskTargetTeam == targetTeam && reviewTeam == targetTeam && targetTeam != "" {
			return &core.PublicationReviewResult{Pending: true, TaskID: taskID}, nil
		}
		if taskReviewTeam != reviewTeam || taskTargetTeam != targetTeam {
			return nil, core.ErrReviewPermissionDenied
		}
	}
	for _, file := range files {
		if err := savePublicationReviewFileTx(tx, taskID, repository, file); err != nil {
			return nil, err
		}
	}
	if err := savePublicationReviewPayloadTx(tx, taskID, request.Payload); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit publication review: %w", err)
	}
	return &core.PublicationReviewResult{Pending: true, TaskID: taskID}, nil
}

// GetReviewTaskPayload returns one bounded engine-specific pending publication payload.
func (db *DB) GetReviewTaskPayload(id string) ([]byte, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	id = SanitizeInputString(strings.TrimSpace(id), 64)
	if id == "" {
		return nil, core.ErrReviewTaskNotFound
	}
	var payload string
	err := db.QueryRow(`SELECT payload_json FROM review_task_payloads WHERE task_id = ?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrReviewTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load publication review payload: %w", err)
	}
	if len(payload) == 0 || len(payload) > maxPublicationReviewPayload {
		return nil, core.ErrReviewInvalidRequest
	}
	return []byte(payload), nil
}

// ListReviewTaskFiles returns the bounded repository-relative file list attached to one task.
func (db *DB) ListReviewTaskFiles(id string) ([]*core.ReviewFile, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	id = SanitizeInputString(strings.TrimSpace(id), 64)
	if id == "" {
		return nil, core.ErrReviewTaskNotFound
	}
	rows, err := db.Query(`SELECT f.file_id, f.path, f.size, f.critical, f.added_at, r.repository, r.resource_type
		FROM review_task_files f JOIN review_tasks r ON r.id = f.task_id
		WHERE f.task_id = ? ORDER BY f.path, f.file_id`, id)
	if err != nil {
		return nil, fmt.Errorf("list review files: %w", err)
	}
	defer rows.Close()
	files := make([]*core.ReviewFile, 0)
	for rows.Next() {
		file := &core.ReviewFile{}
		var critical int
		var resourceType string
		if err := rows.Scan(&file.ID, &file.Path, &file.Size, &critical, &file.AddedAt,
			&file.Repository, &resourceType); err != nil {
			return nil, fmt.Errorf("scan review file: %w", err)
		}
		file.Critical = critical != 0
		file.Virtual = resourceType == core.ReviewResourceDockerImage ||
			strings.HasPrefix(file.Path, "review-requests/")
		file.Name = file.Path
		if separator := strings.LastIndexByte(file.Path, '/'); separator >= 0 {
			file.Name = file.Path[separator+1:]
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate review files: %w", err)
	}
	if len(files) == 0 {
		if _, err := db.GetReviewTask(id); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// ListPendingPublicationReviewFiles returns every file that must remain hidden after startup.
func (db *DB) ListPendingPublicationReviewFiles() ([]*core.ReviewFile, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	rows, err := db.Query(`SELECT f.file_id, f.path, f.size, f.critical, f.added_at, r.repository, r.resource_type
		FROM review_task_files f JOIN review_tasks r ON r.id = f.task_id
		WHERE r.kind = ? AND r.status = ? ORDER BY r.repository, f.path`,
		core.ReviewKindPublication, core.ReviewStatusPending)
	if err != nil {
		return nil, fmt.Errorf("list pending publication files: %w", err)
	}
	defer rows.Close()
	files := make([]*core.ReviewFile, 0)
	for rows.Next() {
		file := &core.ReviewFile{}
		var critical int
		var resourceType string
		if err := rows.Scan(&file.ID, &file.Path, &file.Size, &critical, &file.AddedAt,
			&file.Repository, &resourceType); err != nil {
			return nil, fmt.Errorf("scan pending publication file: %w", err)
		}
		file.Critical = critical != 0
		file.Virtual = resourceType == core.ReviewResourceDockerImage ||
			strings.HasPrefix(file.Path, "review-requests/")
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending publication files: %w", err)
	}
	return files, nil
}

// IsPublicationReviewPathPending reports whether a repository-relative file belongs to a pending review.
func (db *DB) IsPublicationReviewPathPending(repository, path string) (bool, error) {
	if db == nil || db.SQLDB == nil {
		return false, core.ErrDatabaseUnavailable
	}
	repository = sanitizeMavenRepository(repository)
	normalized, err := normalizePublicationReviewFile(&core.ReviewFile{Path: path}, 1)
	if err != nil {
		return false, err
	}
	var exists int
	err = db.QueryRow(`SELECT 1 FROM review_task_files f JOIN review_tasks r ON r.id = f.task_id
		WHERE r.kind = ? AND r.status = ? AND r.repository = ? AND f.file_id = ? LIMIT 1`,
		core.ReviewKindPublication, core.ReviewStatusPending, repository, normalized.ID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect pending publication file: %w", err)
	}
	return exists != 0, nil
}

// HasPendingPublicationReviews reports whether a repository still owns hidden review files.
func (db *DB) HasPendingPublicationReviews(repository string) (bool, error) {
	if db == nil || db.SQLDB == nil {
		return false, core.ErrDatabaseUnavailable
	}
	repository = sanitizeMavenRepository(repository)
	if repository == "" {
		return false, core.ErrReviewInvalidRequest
	}
	var exists int
	err := db.QueryRow(`SELECT 1 FROM review_tasks WHERE kind = ? AND status = ? AND repository = ? LIMIT 1`,
		core.ReviewKindPublication, core.ReviewStatusPending, repository).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect pending repository reviews: %w", err)
	}
	return exists != 0, nil
}

// ListPublicationReviews returns the latest review state for each version of one resource.
func (db *DB) ListPublicationReviews(repository, resourceType, resourceName string) ([]*core.ReviewTask, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository = sanitizeMavenRepository(repository)
	resourceType, valid := normalizeReviewResourceType(resourceType)
	resourceName = SanitizeInputString(strings.TrimSpace(resourceName), 512)
	if repository == "" || !valid || resourceName == "" {
		return nil, core.ErrReviewInvalidRequest
	}
	rows, err := db.Query(`SELECT `+reviewTaskSelectColumns+`
		FROM review_tasks r`+reviewTaskProfileJoins+` WHERE r.kind = ? AND r.repository = ?
		AND r.resource_type = ? AND r.resource_name = ? ORDER BY r.created_at DESC, r.id DESC LIMIT ?`,
		core.ReviewKindPublication, repository, resourceType, resourceName, maxPublicationReviewFiles)
	if err != nil {
		return nil, fmt.Errorf("list publication reviews: %w", err)
	}
	defer rows.Close()
	result := make([]*core.ReviewTask, 0)
	seenVersions := make(map[string]struct{})
	for rows.Next() {
		task, scanErr := scanReviewTask(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan publication review: %w", scanErr)
		}
		if _, seen := seenVersions[task.ResourceVersion]; seen {
			continue
		}
		seenVersions[task.ResourceVersion] = struct{}{}
		result = append(result, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate publication reviews: %w", err)
	}
	if err := db.hydrateReviewTaskSummaries(result); err != nil {
		return nil, err
	}
	return result, nil
}
