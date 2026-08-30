/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

// Package reviewnotify delivers deduplicated review lifecycle notifications.
package reviewnotify

import (
	"errors"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/message"
)

type reviewMessagePayload struct {
	TaskID          string `json:"task_id"`
	Kind            string `json:"kind"`
	ResourceType    string `json:"resource_type"`
	Repository      string `json:"repository,omitempty"`
	ResourceName    string `json:"resource_name"`
	ResourceVersion string `json:"resource_version,omitempty"`
	Status          string `json:"status,omitempty"`
	DecisionReason  string `json:"decision_reason,omitempty"`
}

func pendingDedupeKey(taskID string) string {
	return "review:pending:" + taskID
}

func resultDedupeKey(taskID, status string) string {
	return "review:result:" + taskID + ":" + status
}

func activeReviewAccount(token *core.AccessToken, now int64) (*config.User, bool) {
	if token == nil || strings.TrimSpace(token.Name) == "" ||
		token.ExpiresAt != nil && now >= *token.ExpiresAt {
		return nil, false
	}
	return &config.User{
		Username: strings.ToLower(strings.TrimSpace(token.Name)), Roles: token.Permissions,
	}, true
}

func reviewRecipients(state *core.AppState, task *core.ReviewTask) ([]string, error) {
	if state == nil || state.GetDB() == nil || task == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	tokens, err := state.GetDB().GetAllTokens()
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	unique := make(map[string]struct{}, len(tokens))
	activeAccounts := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		user, active := activeReviewAccount(token, now)
		if !active {
			continue
		}
		activeAccounts[user.Username] = struct{}{}
		if strings.EqualFold(user.Username, task.RequestedBy) {
			continue
		}
		if user.IsManager() || (task.Kind == core.ReviewKindPublication &&
			user.CheckModeratePermission(task.Repository)) {
			unique[user.Username] = struct{}{}
		}
	}
	if task.Kind == core.ReviewKindSuperTeamTransfer && task.ReviewTeamPrefix != "" {
		teamReviewers, err := state.GetDB().ListSuperTeamReviewerNames(task.ReviewTeamPrefix)
		if err != nil {
			return nil, err
		}
		for _, reviewer := range teamReviewers {
			reviewer = strings.ToLower(strings.TrimSpace(reviewer))
			_, active := activeAccounts[reviewer]
			if active && reviewer != "" && !strings.EqualFold(reviewer, task.RequestedBy) {
				unique[reviewer] = struct{}{}
			}
		}
	}
	recipients := make([]string, 0, len(unique))
	for recipient := range unique {
		recipients = append(recipients, recipient)
	}
	sort.Strings(recipients)
	return recipients, nil
}

func payloadBytes(task *core.ReviewTask, includeDecision bool) ([]byte, error) {
	payload := reviewMessagePayload{
		TaskID: task.ID, Kind: task.Kind, ResourceType: task.ResourceType,
		Repository: task.Repository, ResourceName: task.ResourceName,
		ResourceVersion: task.ResourceVersion,
	}
	if includeDecision {
		payload.Status = task.Status
		payload.DecisionReason = task.DecisionReason
	}
	return json.Marshal(&payload)
}

// NotifyPendingByID delivers one deduplicated pending-review notice to every eligible reviewer.
func NotifyPendingByID(state *core.AppState, taskID string) error {
	if state == nil || state.GetDB() == nil {
		return core.ErrDatabaseUnavailable
	}
	task, err := state.GetDB().GetReviewTask(taskID)
	if err != nil {
		return err
	}
	return NotifyPending(state, task)
}

// NotifyPending delivers one deduplicated pending-review notice to every eligible reviewer.
func NotifyPending(state *core.AppState, task *core.ReviewTask) error {
	if task == nil || task.Status != core.ReviewStatusPending {
		return nil
	}
	recipients, err := reviewRecipients(state, task)
	if err != nil {
		return err
	}
	payload, err := payloadBytes(task, false)
	if err != nil {
		return err
	}
	var result error
	createdAt := task.UpdatedAt
	if createdAt <= 0 {
		createdAt = task.CreatedAt
	}
	if createdAt <= 0 {
		createdAt = time.Now().UnixMilli()
	}
	for _, recipient := range recipients {
		_, deliveryErr := message.DeliverOnce(state, &core.UserMessage{
			Recipient: recipient, Sender: task.RequestedBy, Kind: "review_pending", Severity: "warning",
			Title: "Review requested", Body: "A review requires your attention.", Payload: payload,
			DedupeKey: pendingDedupeKey(task.ID), CreatedAt: createdAt,
		})
		result = errors.Join(result, deliveryErr)
	}
	return result
}

// NotifyDecision removes stale reviewer notices and delivers one requester result without reviewer identity.
func NotifyDecision(state *core.AppState, task *core.ReviewTask) error {
	if state == nil || state.GetDB() == nil || task == nil || task.ID == "" {
		return core.ErrDatabaseUnavailable
	}
	_, deleteErr := state.GetDB().DeleteMessagesByDedupeKey(pendingDedupeKey(task.ID))
	if task.Status == core.ReviewStatusPending {
		return deleteErr
	}
	profile, err := state.GetDB().GetUserProfileByID(task.RequestedByID)
	if err != nil || profile == nil || profile.Username == "" {
		return errors.Join(deleteErr, err)
	}
	payload, err := payloadBytes(task, true)
	if err != nil {
		return errors.Join(deleteErr, err)
	}
	severity := "info"
	if task.Status == core.ReviewStatusApproved {
		severity = "success"
	} else if task.Status == core.ReviewStatusRejected {
		severity = "warning"
	}
	_, deliveryErr := message.DeliverOnce(state, &core.UserMessage{
		Recipient: profile.Username, Kind: "review_result", Severity: severity,
		Title: "Review completed", Body: "Your review request has been updated.", Payload: payload,
		DedupeKey: resultDedupeKey(task.ID, task.Status), CreatedAt: max(task.DecidedAt, time.Now().UnixMilli()),
	})
	return errors.Join(deleteErr, deliveryErr)
}

// ClearPending removes reviewer notices when a requester cancels before a decision.
func ClearPending(state *core.AppState, taskID string) error {
	if state == nil || state.GetDB() == nil {
		return core.ErrDatabaseUnavailable
	}
	_, err := state.GetDB().DeleteMessagesByDedupeKey(pendingDedupeKey(taskID))
	return err
}

// DeliverPending records notification failures without invalidating an already durable review task.
func DeliverPending(state *core.AppState, result *core.PublicationReviewResult) {
	if result == nil || !result.Pending || result.TaskID == "" {
		return
	}
	if err := NotifyPendingByID(state, result.TaskID); err != nil {
		if state != nil && state.Inner != nil {
			state.Inner.FailuresCount.Add(1)
		}
		log.Printf("failed to deliver pending review notifications for %s: %v", result.TaskID, err)
	}
}

// DeliverTask records notification failures without invalidating an already durable review task.
func DeliverTask(state *core.AppState, task *core.ReviewTask) {
	if task == nil {
		return
	}
	if err := NotifyPending(state, task); err != nil {
		if state != nil && state.Inner != nil {
			state.Inner.FailuresCount.Add(1)
		}
		log.Printf("failed to deliver pending review notifications for %s: %v", task.ID, err)
	}
}

// DeliverDecision records notification failures after a durable task decision.
func DeliverDecision(state *core.AppState, task *core.ReviewTask) {
	if task == nil {
		return
	}
	if err := NotifyDecision(state, task); err != nil {
		if state != nil && state.Inner != nil {
			state.Inner.FailuresCount.Add(1)
		}
		log.Printf("failed to deliver review result notification for %s: %v", task.ID, err)
	}
}
