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
	"fmt"

	"renop/internal/core"
)

// ApproveDockerPublicationReview atomically publishes one manifest and completes its pending review.
func (db *DB) ApproveDockerPublicationReview(id, reviewer string, manifest *core.DockerManifest,
	tag string, decidedAt int64,
) (*core.ReviewTask, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	id = SanitizeInputString(id, 64)
	reviewer = sanitizeDockerUsername(reviewer)
	if id == "" || reviewer == "" || decidedAt <= 0 {
		return nil, core.ErrReviewInvalidRequest
	}
	reviewTaskMutationLock.Lock()
	defer reviewTaskMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin Docker publication approval: %w", err)
	}
	defer tx.Rollback()
	task, err := loadReviewTaskTx(tx, id)
	if err != nil {
		return nil, err
	}
	if task.Status != core.ReviewStatusPending || task.Kind != core.ReviewKindPublication ||
		task.ResourceType != core.ReviewResourceDockerImage {
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
		return nil, fmt.Errorf("inspect Docker publication activity: %w", err)
	}
	if decidedAt-latestFileAt < core.PublicationReviewSettleMillis {
		return nil, core.ErrReviewPublicationActive
	}
	write, err := normalizeDockerManifestWrite(manifest, tag, task.RequestedBy, false, task.CreatedAt)
	if err != nil || write.repository != task.Repository || write.imageName != task.ResourceKey ||
		(tag == "" && write.digest != task.ResourceVersion) || (tag != "" && write.tag != task.ResourceVersion) {
		return nil, core.ErrReviewInvalidRequest
	}
	if err := lockDockerImageTeam(tx, write.repository, write.imageName); err != nil {
		return nil, err
	}
	requester, err := reviewUserTx(tx, task.RequestedByID)
	if err != nil {
		return nil, core.ErrReviewResourceConflict
	}
	if !requester.IsManager() && !requester.CheckUpdatePermission(task.Repository) {
		level, member, permissionErr := dockerEffectivePermissionTx(
			tx, write.repository, write.imageName, task.RequestedByID)
		if permissionErr != nil || !member || level < core.DockerPermissionPublish {
			return nil, core.ErrReviewResourceConflict
		}
	}
	if err := putDockerManifestTx(tx, write); err != nil {
		return nil, err
	}
	result, err := tx.Exec(`UPDATE review_tasks SET status = ?, decision_reason = '', decided_by_id = ?,
		decided_by_name = ?, decided_at = ?, active_key = NULL WHERE id = ? AND status = ?`,
		core.ReviewStatusApproved, reviewerID, reviewer, decidedAt, task.ID, core.ReviewStatusPending)
	if err != nil {
		return nil, fmt.Errorf("complete Docker publication review: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return nil, core.ErrReviewTaskConflict
	}
	if _, err := tx.Exec(`DELETE FROM review_task_payloads WHERE task_id = ?`, task.ID); err != nil {
		return nil, fmt.Errorf("delete Docker publication payload: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit Docker publication approval: %w", err)
	}
	task.Status = core.ReviewStatusApproved
	task.DecidedByID = reviewerID
	task.DecidedBy = reviewer
	task.DecidedAt = decidedAt
	task.DecisionReason = ""
	return task, nil
}
