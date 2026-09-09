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
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"renop/internal/config"
	"renop/internal/core"
)

func supportTicket(task *core.ReviewTask) bool {
	return task.Kind == core.TicketKindFeedback || task.Kind == core.TicketKindSuggestion || task.Kind == core.TicketKindReport
}

func normalizeTicketText(value string, limit int) (string, bool) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > limit {
		return "", false
	}
	for _, ch := range value {
		if unicode.IsControl(ch) && ch != '\n' && ch != '\t' {
			return "", false
		}
	}
	return value, true
}

func ticketActorTx(tx *Tx, actorID string) (*config.User, error) {
	if err := lockAccountLoginMethodsTx(tx, actorID); err != nil {
		return nil, err
	}
	user, err := reviewUserTx(tx, actorID)
	if err != nil {
		return nil, err
	}
	token, err := tokenByNameTx(tx, user.Username)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	if token == nil || token.DeletedAt > 0 || token.Ban.IsActive(now) || token.ExpiresAt != nil && *token.ExpiresAt <= now {
		return nil, core.ErrReviewPermissionDenied
	}
	return user, nil
}

func ticketHandlerTx(tx *Tx, task *core.ReviewTask, actorID string) (*config.User, error) {
	user, err := ticketActorTx(tx, actorID)
	if err != nil {
		return nil, err
	}
	if task.Kind == core.TicketKindReport && (actorID == task.RequestedByID || strings.Contains(task.TargetUserIDs, `"`+actorID+`"`)) {
		return nil, core.ErrReviewPermissionDenied
	}
	if task.AdminOnly && !user.IsManager() {
		return nil, core.ErrReviewPermissionDenied
	}
	if user.IsManager() {
		return user, nil
	}
	if task.ReviewTeamPrefix != "" && !supportTicket(task) {
		if err := requireSuperTeamRoleTx(tx, task.ReviewTeamPrefix, actorID, core.SuperTeamRoleManage); err != nil {
			return nil, core.ErrReviewPermissionDenied
		}
	} else if !user.CheckModeratePermission(task.Repository) {
		return nil, core.ErrReviewPermissionDenied
	}
	return user, nil
}

func insertTicketStateTx(tx *Tx, task *core.ReviewTask) error {
	_, err := tx.Exec(`INSERT INTO ticket_state (task_id, status, title, body, target_user_ids, response, changed_at)
		VALUES (?, ?, ?, ?, ?, '', ?)`, task.ID, core.TicketUnprocessed, task.Title, task.Body, task.TargetUserIDs, task.CreatedAt)
	return err
}

// requireTicketAssigneeTx locks assignment through the workflow's metadata commit.
func requireTicketAssigneeTx(tx *Tx, task *core.ReviewTask, actorID string) error {
	if _, err := ticketHandlerTx(tx, task, actorID); err != nil {
		return err
	}
	if task.Status != core.ReviewStatusPending || task.AssigneeID != actorID {
		return core.ErrTicketClaimRequired
	}
	result, err := tx.Exec(`UPDATE ticket_state SET revision = revision + 1
		WHERE task_id = ? AND assignee_id = ? AND revision = ? AND status IN (?, ?)`,
		task.ID, actorID, task.Revision, core.TicketInProgress, core.TicketProcessed)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return core.ErrTicketClaimRequired
	}
	return nil
}

// TransitionTicket atomically claims, releases, escalates, or resolves a support workflow.
func (db *DB) TransitionTicket(id, actor, session string, action core.TicketAction, at int64) (*core.ReviewTask, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	if id == "" || at <= 0 {
		return nil, core.ErrReviewInvalidRequest
	}
	reviewTaskMutationLock.Lock()
	defer reviewTaskMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	account, err := accountEmailSessionTx(tx, strings.ToLower(strings.TrimSpace(actor)), session)
	if err != nil {
		return nil, core.ErrReviewPermissionDenied
	}
	// The workflow row also serializes the first claim of a migrated review.
	if _, err := tx.Exec(`UPDATE review_tasks SET created_at = created_at WHERE id = ?`, id); err != nil {
		return nil, err
	}
	task, err := loadReviewTaskTx(tx, id)
	if err != nil {
		return nil, err
	}
	if task.Status != core.ReviewStatusPending {
		return nil, core.ErrReviewTaskConflict
	}
	user, err := ticketHandlerTx(tx, task, account.UserID)
	if err != nil {
		return nil, err
	}
	var existing string
	if err := tx.QueryRow(`SELECT task_id FROM ticket_state WHERE task_id = ?`, id).Scan(&existing); errors.Is(err, sql.ErrNoRows) {
		if err := insertTicketStateTx(tx, task); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	switch action.Action {
	case "claim":
		if task.AssigneeID != "" {
			if task.AssigneeID == account.UserID {
				return nil, core.ErrTicketOccupied
			}
			if !action.Force || !user.IsManager() || task.AssigneeAdmin || task.Escalations >= 3 {
				return nil, core.ErrTicketOccupied
			}
			if err := lockAccountLoginMethodsTx(tx, task.AssigneeID); err != nil {
				return nil, err
			}
			assignee, err := reviewUserTx(tx, task.AssigneeID)
			if err != nil {
				return nil, err
			}
			if assignee.IsManager() {
				return nil, core.ErrTicketOccupied
			}
		} else if task.AdminOnly && task.EscalatedByID == account.UserID {
			return nil, core.ErrReviewPermissionDenied
		}
		task.AssigneeID, task.AssigneeAdmin = account.UserID, user.IsManager()
		task.TicketState.Status = core.TicketInProgress
	case "release", "escalate":
		if task.AssigneeID != account.UserID {
			return nil, core.ErrTicketClaimRequired
		}
		if task.Escalations >= 3 {
			return nil, core.ErrTicketEscalationLimit
		}
		if action.Action == "escalate" {
			task.Escalations++
			task.AdminOnly, task.EscalatedByID = true, account.UserID
		}
		task.AssigneeID, task.AssigneeAdmin = "", false
		task.TicketState.Status = core.TicketUnprocessed
	case "process", "complete", "close":
		if !supportTicket(task) {
			return nil, core.ErrReviewInvalidRequest
		}
		if task.AssigneeID != account.UserID {
			return nil, core.ErrTicketClaimRequired
		}
		response, valid := normalizeTicketText(action.Response, 4096)
		if !valid || response == "" {
			return nil, core.ErrReviewInvalidRequest
		}
		if action.Action == "close" {
			action.Outcome = "closed"
			task.Status = core.ReviewStatusCancelled
		} else if task.Kind == core.TicketKindReport {
			if action.Outcome != "upheld" && action.Outcome != "dismissed" {
				return nil, core.ErrReviewInvalidRequest
			}
		} else if action.Outcome != "resolved" {
			return nil, core.ErrReviewInvalidRequest
		}
		task.Outcome, task.Response = action.Outcome, response
		task.TicketState.Status = core.TicketProcessed
		if action.Action != "process" {
			if action.Action == "complete" {
				task.Status = core.ReviewStatusApproved
				if task.Outcome == "dismissed" {
					task.Status = core.ReviewStatusRejected
				}
			}
			if _, err := tx.Exec(`UPDATE review_tasks SET status = ?, decision_reason = ?, decided_by_id = ?,
				decided_by_name = ?, decided_at = ?, active_key = NULL WHERE id = ? AND status = ?`,
				task.Status, task.Outcome, account.UserID, user.Username, at, id, core.ReviewStatusPending); err != nil {
				return nil, err
			}
		}
	default:
		return nil, core.ErrReviewInvalidRequest
	}
	result, err := tx.Exec(`UPDATE ticket_state SET status = ?, assignee_id = ?, assignee_admin = ?,
		admin_only = ?, escalations = ?, escalated_by_id = ?, outcome = ?, response = ?, changed_at = ?, revision = revision + 1
		WHERE task_id = ? AND revision = ?`, task.TicketState.Status, task.AssigneeID, boolInt(task.AssigneeAdmin),
		boolInt(task.AdminOnly), task.Escalations, task.EscalatedByID, task.Outcome, task.Response, at, id, task.Revision)
	if err != nil {
		return nil, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if changed != 1 {
		return nil, core.ErrTicketOccupied
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return db.GetReviewTask(id)
}
