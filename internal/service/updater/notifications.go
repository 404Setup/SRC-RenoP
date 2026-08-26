/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package updater

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"renop/internal/core"
	"renop/internal/service/message"
	"renop/internal/version"
)

const (
	updateNoticeAvailable         = "available"
	updateNoticeCurrent           = "current"
	updateNoticeCheckFailed       = "check_failed"
	updateNoticeInstallFailed     = "install_failed"
	updateNoticeInsufficientSpace = "insufficient_space"
	updateNoticeRestartFailed     = "restart_failed"
)

type updateNotificationPayload struct {
	Event          string `json:"event"`
	Version        string `json:"version,omitempty"`
	Current        string `json:"current,omitempty"`
	Channel        string `json:"channel,omitempty"`
	ReleaseDate    string `json:"release_date,omitempty"`
	IsRelease      bool   `json:"is_release"`
	RequiresAction bool   `json:"requires_action,omitempty"`
}

func updateNotificationPresentation(event, targetVersion string) (severity, title, body string) {
	switch event {
	case updateNoticeAvailable:
		return "warning", "System update available", "RenoP " + targetVersion + " is available. Review and install it from Dashboard."
	case updateNoticeCurrent:
		return "success", "RenoP is up to date", "This server is already running the latest version."
	case updateNoticeCheckFailed:
		return "error", "Update check failed", "RenoP could not check the configured update channel. Try again from Dashboard."
	case updateNoticeInsufficientSpace:
		return "error", "Insufficient update space", "There is not enough disk space to prepare the system update."
	case updateNoticeRestartFailed:
		return "error", "Update restart failed", "RenoP could not restart to apply the prepared update. Review the server logs and try again."
	default:
		return "error", "Update preparation failed", "The system update could not be prepared. Review the server logs and try again."
	}
}

func updateNotificationVersion(result *CheckResult) string {
	if result != nil && strings.TrimSpace(result.LatestVersion) != "" {
		return strings.TrimSpace(result.LatestVersion)
	}
	if state := GetUpdateState(); state != nil && strings.TrimSpace(state.LatestVersion) != "" {
		return strings.TrimSpace(state.LatestVersion)
	}
	return strings.TrimSpace(version.Version)
}

func deliverUpdateNotification(state *core.AppState, recipient, event string, result *CheckResult) error {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	if state == nil || recipient == "" || recipient == "guest" {
		return nil
	}
	targetVersion := updateNotificationVersion(result)
	severity, title, body := updateNotificationPresentation(event, targetVersion)
	payload := updateNotificationPayload{
		Event: event, Version: targetVersion, Current: version.Version,
		RequiresAction: event == updateNoticeAvailable,
	}
	if result != nil {
		payload.Channel = result.Channel
		payload.ReleaseDate = result.ReleaseDate
		payload.IsRelease = result.IsRelease
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = message.DeliverOnce(state, &core.UserMessage{
		Recipient: recipient, Kind: "system_update", Severity: severity,
		Title: title, Body: body, Payload: payloadBytes,
		DedupeKey: "system-update:" + event + ":" + strings.ToLower(targetVersion),
	})
	return err
}

func tokenHasManagerPermission(token *core.AccessToken, now int64) bool {
	if token == nil || strings.TrimSpace(token.Name) == "" ||
		(token.ExpiresAt != nil && now >= *token.ExpiresAt) {
		return false
	}
	for _, permission := range token.Permissions {
		switch strings.ToLower(strings.TrimSpace(permission)) {
		case "manager", "admin", "m", "access-token:manager":
			return true
		}
	}
	return false
}

func updateManagerRecipients(state *core.AppState) ([]string, error) {
	if state == nil || state.GetDB() == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	tokens, err := state.GetDB().GetAllTokens()
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	unique := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		if tokenHasManagerPermission(token, now) {
			unique[strings.ToLower(strings.TrimSpace(token.Name))] = struct{}{}
		}
	}
	recipients := make([]string, 0, len(unique))
	for recipient := range unique {
		recipients = append(recipients, recipient)
	}
	sort.Strings(recipients)
	return recipients, nil
}

func deliverUpdateNotificationToManagers(state *core.AppState, event string, result *CheckResult) error {
	recipients, err := updateManagerRecipients(state)
	if err != nil {
		return err
	}
	var deliveryErrors []error
	for _, recipient := range recipients {
		if err := deliverUpdateNotification(state, recipient, event, result); err != nil {
			deliveryErrors = append(deliveryErrors, err)
		}
	}
	return errors.Join(deliveryErrors...)
}
