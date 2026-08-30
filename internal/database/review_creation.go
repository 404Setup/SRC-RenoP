/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database

import (
	"errors"
	"fmt"
	"strings"

	"renop/internal/core"
)

type packageCreationMutation func(*Tx, *core.ReviewTask) error

func (db *DB) approvePackageCreationReview(id, reviewer, resourceType, repository, resourceKey string,
	decidedAt int64, mutate packageCreationMutation,
) (*core.ReviewTask, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	id = SanitizeInputString(strings.TrimSpace(id), 64)
	reviewer = strings.ToLower(SanitizeInputString(strings.TrimSpace(reviewer), maxTokenNameLen))
	if id == "" || reviewer == "" || resourceKey == "" || decidedAt <= 0 || mutate == nil {
		return nil, core.ErrReviewInvalidRequest
	}
	superTeamMutationLock.Lock()
	defer superTeamMutationLock.Unlock()
	reviewTaskMutationLock.Lock()
	defer reviewTaskMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin package creation approval: %w", err)
	}
	defer tx.Rollback()
	task, err := loadReviewTaskTx(tx, id)
	if err != nil {
		return nil, err
	}
	if task.Status != core.ReviewStatusPending || task.Kind != core.ReviewKindPublication ||
		task.ResourceType != resourceType || task.Repository != repository || task.ResourceKey != resourceKey ||
		task.ResourceVersion != core.ReviewVersionPackageCreation {
		return nil, core.ErrReviewTaskConflict
	}
	reviewerID, err := userIDForUsernameTx(tx, reviewer)
	if err != nil {
		return nil, core.ErrReviewPermissionDenied
	}
	reviewerUser, err := reviewUserTx(tx, reviewerID)
	if err != nil || !reviewerUser.CheckModeratePermission(task.Repository) {
		return nil, core.ErrReviewPermissionDenied
	}
	var latestFileAt int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(added_at), ?) FROM review_task_files WHERE task_id = ?`,
		task.CreatedAt, task.ID).Scan(&latestFileAt); err != nil {
		return nil, fmt.Errorf("inspect package creation review activity: %w", err)
	}
	if decidedAt-latestFileAt < core.PublicationReviewSettleMillis {
		return nil, core.ErrReviewPublicationActive
	}
	requester, err := reviewUserTx(tx, task.RequestedByID)
	if err != nil || (!requester.IsManager() && !requester.CheckUpdatePermission(task.Repository)) {
		return nil, core.ErrReviewResourceConflict
	}
	if err := mutate(tx, task); err != nil {
		if reviewResourceStateChanged(err) {
			return nil, errors.Join(core.ErrReviewResourceConflict, err)
		}
		return nil, err
	}
	result, err := tx.Exec(`UPDATE review_tasks SET status = ?, decision_reason = '', decided_by_id = ?,
		decided_by_name = ?, decided_at = ?, active_key = NULL WHERE id = ? AND status = ?`,
		core.ReviewStatusApproved, reviewerID, reviewer, decidedAt, task.ID, core.ReviewStatusPending)
	if err != nil {
		return nil, fmt.Errorf("complete package creation review: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return nil, core.ErrReviewTaskConflict
	}
	if _, err := tx.Exec(`DELETE FROM review_task_payloads WHERE task_id = ?`, task.ID); err != nil {
		return nil, fmt.Errorf("delete package creation payload: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit package creation approval: %w", err)
	}
	task.Status = core.ReviewStatusApproved
	task.DecidedByID = reviewerID
	task.DecidedBy = reviewer
	task.DecidedAt = decidedAt
	task.DecisionReason = ""
	return task, nil
}

// ApproveDockerImageCreationReview atomically reserves an approved Docker image and completes its task.
func (db *DB) ApproveDockerImageCreationReview(id, reviewer, repository, imageName,
	superTeamPrefix string, private bool, createdAt, decidedAt int64,
) (*core.ReviewTask, error) {
	repository, imageName = sanitizeDockerKey(repository, imageName)
	superTeamPrefix, err := normalizeOptionalSuperTeamPrefix(superTeamPrefix)
	if err != nil || repository == "" || imageName == "" || createdAt <= 0 {
		return nil, core.ErrReviewInvalidRequest
	}
	requiredPrefix, namespaced := core.DockerImageSuperTeamPrefix(imageName)
	invalidNamespace := strings.Contains(imageName, "/") && !namespaced
	invalidBinding := namespaced && (superTeamPrefix == "" || superTeamPrefix != requiredPrefix)
	if invalidNamespace || invalidBinding {
		return nil, core.ErrSuperTeamBindingMismatch
	}
	return db.approvePackageCreationReview(id, reviewer, core.ReviewResourceDockerImage,
		repository, imageName, decidedAt, func(tx *Tx, task *core.ReviewTask) error {
			_, err := createDockerImageTx(tx, repository, imageName, task.RequestedBy,
				task.RequestedByID, superTeamPrefix, private, createdAt)
			return err
		})
}

// ApproveNPMPackageCreationReview atomically reserves an approved npm package and completes its task.
func (db *DB) ApproveNPMPackageCreationReview(id, reviewer, repository, packageName,
	superTeamPrefix string, private bool, createdAt, decidedAt int64,
) (*core.ReviewTask, error) {
	repository, packageName = sanitizeNPMKey(repository, packageName)
	superTeamPrefix, err := normalizeOptionalSuperTeamPrefix(superTeamPrefix)
	if err != nil || repository == "" || packageName == "" || createdAt <= 0 {
		return nil, core.ErrReviewInvalidRequest
	}
	requiredPrefix, scoped := core.NPMPackageSuperTeamPrefix(packageName)
	invalidScope := strings.HasPrefix(packageName, "@") && !scoped
	invalidBinding := scoped && (superTeamPrefix == "" || superTeamPrefix != requiredPrefix)
	privateUnscoped := private && !strings.HasPrefix(packageName, "@")
	if invalidScope || invalidBinding || privateUnscoped {
		return nil, core.ErrSuperTeamBindingMismatch
	}
	return db.approvePackageCreationReview(id, reviewer, core.ReviewResourceNPMPackage,
		repository, packageName, decidedAt, func(tx *Tx, task *core.ReviewTask) error {
			_, err := createNPMPackageTx(tx, repository, packageName, task.RequestedBy,
				task.RequestedByID, superTeamPrefix, private, createdAt)
			return err
		})
}
