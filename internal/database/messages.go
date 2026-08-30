/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/goccy/go-json"
	"github.com/google/uuid"

	"renop/internal/core"
)

const messageColumns = `id, recipient, sender, kind, severity, title, body, payload_json,
	action_kind, action_status, created_at, read_at, acted_at, expires_at, dedupe_key`

func insertAcceptedMembershipMessage(tx *Tx, recipient, sender, actionKind, title, body string, payload any, now int64) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode membership message payload: %w", err)
	}
	_, err = tx.Exec(`INSERT INTO user_messages (`+messageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), recipient, sender, actionKind, "info", title, body, string(payloadBytes),
		actionKind, core.MessageActionAccepted, now, 0, now, now+7*24*3600*1000, nil)
	return err
}

func insertTeamRemovalMessage(tx *Tx, recipient, format, repository, resource string, now int64) error {
	payloadBytes, err := json.Marshal(map[string]string{
		"format": format, "repository": repository, "package": resource,
	})
	if err != nil {
		return fmt.Errorf("encode team removal message payload: %w", err)
	}
	target := resource
	if repository != "" {
		target = repository + " - " + resource
	}
	_, err = tx.Exec(`INSERT INTO user_messages (`+messageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), recipient, "", "package_team_removed", "warning", "Team membership removed",
		"You were removed from "+target+".", string(payloadBytes), "", "", now, 0, 0, 0, nil)
	return err
}

type messageScanner interface {
	Scan(dest ...any) error
}

func scanMessage(scanner messageScanner) (*core.UserMessage, error) {
	message := &core.UserMessage{}
	var payload string
	var dedupeKey sql.NullString
	if err := scanner.Scan(
		&message.ID, &message.Recipient, &message.Sender, &message.Kind, &message.Severity,
		&message.Title, &message.Body, &payload, &message.ActionKind, &message.ActionStatus,
		&message.CreatedAt, &message.ReadAt, &message.ActedAt, &message.ExpiresAt, &dedupeKey,
	); err != nil {
		return nil, err
	}
	message.Payload = []byte(payload)
	if dedupeKey.Valid {
		message.DedupeKey = dedupeKey.String
	}
	return message, nil
}

func normalizeMessage(message *core.UserMessage) error {
	if message == nil {
		return errors.New("message is nil")
	}
	message.ID = SanitizeInputString(strings.TrimSpace(message.ID), 64)
	message.Recipient = strings.ToLower(SanitizeInputString(strings.TrimSpace(message.Recipient), maxTokenNameLen))
	message.Sender = strings.ToLower(SanitizeInputString(strings.TrimSpace(message.Sender), maxTokenNameLen))
	message.Kind = strings.ToLower(SanitizeInputString(strings.TrimSpace(message.Kind), 64))
	message.Severity = strings.ToLower(SanitizeInputString(strings.TrimSpace(message.Severity), 16))
	message.Title = SanitizeInputString(strings.TrimSpace(message.Title), 240)
	message.Body = SanitizeInputString(strings.TrimSpace(message.Body), 8000)
	message.ActionKind = strings.ToLower(SanitizeInputString(strings.TrimSpace(message.ActionKind), 64))
	message.ActionStatus = strings.ToLower(SanitizeInputString(strings.TrimSpace(message.ActionStatus), 16))
	message.DedupeKey = SanitizeInputString(strings.TrimSpace(message.DedupeKey), 255)
	if message.ID == "" || message.Recipient == "" || message.Kind == "" || message.Title == "" || message.CreatedAt <= 0 {
		return errors.New("message is missing required fields")
	}
	if message.Severity == "" {
		message.Severity = "info"
	}
	switch message.Severity {
	case "info", "success", "warning", "error":
	default:
		return errors.New("invalid message severity")
	}
	if len(message.Payload) == 0 {
		message.Payload = []byte("{}")
	}
	if !json.Valid(message.Payload) {
		return errors.New("message payload is invalid JSON")
	}
	if message.ActionKind == "" {
		message.ActionStatus = ""
	} else if message.ActionStatus == "" {
		message.ActionStatus = core.MessageActionPending
	}
	return nil
}

// SaveMessages inserts a bounded batch atomically. Dedupe keys are unique per
// recipient so workflow producers can safely retry delivery.
func (db *DB) SaveMessages(messages []*core.UserMessage) error {
	if db == nil || db.SQLDB == nil || len(messages) == 0 {
		return nil
	}
	if len(messages) > 100000 {
		return errors.New("message batch is too large")
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin message batch: %w", err)
	}
	defer tx.Rollback()
	query := `INSERT INTO user_messages (` + messageColumns + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	for _, message := range messages {
		if err := normalizeMessage(message); err != nil {
			return err
		}
		var dedupeKey any
		if message.DedupeKey != "" {
			dedupeKey = message.DedupeKey
		}
		if _, err := tx.Exec(query,
			message.ID, message.Recipient, message.Sender, message.Kind, message.Severity,
			message.Title, message.Body, string(message.Payload), message.ActionKind, message.ActionStatus,
			message.CreatedAt, message.ReadAt, message.ActedAt, message.ExpiresAt, dedupeKey,
		); err != nil {
			return fmt.Errorf("insert message for %s: %w", message.Recipient, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit message batch: %w", err)
	}
	return nil
}

// SaveMessageIfAbsent inserts one message unless its recipient-scoped dedupe key already exists.
func (db *DB) SaveMessageIfAbsent(message *core.UserMessage) (bool, error) {
	if db == nil || db.SQLDB == nil {
		return false, core.ErrDatabaseUnavailable
	}
	if err := normalizeMessage(message); err != nil {
		return false, err
	}
	if message.DedupeKey == "" {
		if err := db.SaveMessages([]*core.UserMessage{message}); err != nil {
			return false, err
		}
		return true, nil
	}
	query := `INSERT INTO user_messages (` + messageColumns + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	arguments := []any{
		message.ID, message.Recipient, message.Sender, message.Kind, message.Severity,
		message.Title, message.Body, string(message.Payload), message.ActionKind, message.ActionStatus,
		message.CreatedAt, message.ReadAt, message.ActedAt, message.ExpiresAt, message.DedupeKey,
	}
	switch db.Dialect.Name() {
	case "mysql":
		query += ` ON DUPLICATE KEY UPDATE id = id`
	case "clickhouse":
		query = `/* renop:ignore-if-exists */ INSERT INTO user_messages (` + messageColumns + `)
			SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
			WHERE NOT EXISTS (SELECT 1 FROM user_messages WHERE recipient = ? AND dedupe_key = ?)`
		arguments = append(arguments, message.Recipient, message.DedupeKey)
	default:
		query += ` ON CONFLICT (recipient, dedupe_key) DO NOTHING`
	}
	result, err := db.Exec(query, arguments...)
	if err != nil {
		return false, fmt.Errorf("insert deduplicated message for %s: %w", message.Recipient, err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count deduplicated message insert: %w", err)
	}
	return inserted == 1, nil
}

// ListMessages returns a stable newest-first cursor page. Expired messages are
// excluded without requiring a cleanup job on the request path.
func (db *DB) ListMessages(username string, limit int, beforeCreatedAt int64, beforeID string, now int64) ([]*core.UserMessage, error) {
	if db == nil || db.SQLDB == nil || username == "" {
		return []*core.UserMessage{}, nil
	}
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	if limit < 1 || limit > 100 {
		limit = 30
	}
	query := `SELECT ` + messageColumns + ` FROM user_messages
		WHERE recipient = ? AND (expires_at = 0 OR expires_at > ?)`
	args := []any{username, now}
	if beforeCreatedAt > 0 && beforeID != "" {
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, beforeCreatedAt, beforeCreatedAt, beforeID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	messages := make([]*core.UserMessage, 0, limit)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return messages, nil
}

func (db *DB) CountUnreadMessages(username string, now int64) (int, error) {
	if db == nil || db.SQLDB == nil || username == "" {
		return 0, nil
	}
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_messages
		WHERE recipient = ? AND read_at = 0 AND (expires_at = 0 OR expires_at > ?)`, username, now).Scan(&count); err != nil {
		return 0, fmt.Errorf("count unread messages: %w", err)
	}
	return count, nil
}

func (db *DB) GetUserMessage(id, username string, now int64) (*core.UserMessage, error) {
	if db == nil || db.SQLDB == nil || id == "" || username == "" {
		return nil, nil
	}
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	message, err := scanMessage(db.QueryRow(`SELECT `+messageColumns+` FROM user_messages
		WHERE id = ? AND recipient = ? AND (expires_at = 0 OR expires_at > ?)`, id, username, now))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user message: %w", err)
	}
	return message, nil
}

func (db *DB) MarkMessageRead(id, username string, readAt int64) (bool, error) {
	if db == nil || db.SQLDB == nil || id == "" || username == "" || readAt <= 0 {
		return false, nil
	}
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	result, err := db.Exec(`UPDATE user_messages SET read_at = ? WHERE id = ? AND recipient = ? AND read_at = 0`, readAt, id, username)
	if err != nil {
		return false, fmt.Errorf("mark message read: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (db *DB) MarkAllMessagesRead(username string, readAt int64) (int64, error) {
	if db == nil || db.SQLDB == nil || username == "" || readAt <= 0 {
		return 0, nil
	}
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	result, err := db.Exec(`UPDATE user_messages SET read_at = ? WHERE recipient = ? AND read_at = 0`, readAt, username)
	if err != nil {
		return 0, fmt.Errorf("mark all messages read: %w", err)
	}
	return result.RowsAffected()
}

func (db *DB) TransitionMessageAction(id, username, expectedStatus, newStatus string, actedAt int64) (bool, error) {
	if db == nil || db.SQLDB == nil || id == "" || username == "" || expectedStatus == "" || newStatus == "" || actedAt <= 0 {
		return false, nil
	}
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	result, err := db.Exec(`UPDATE user_messages SET action_status = ?, acted_at = ?, read_at = CASE WHEN read_at = 0 THEN ? ELSE read_at END
		WHERE id = ? AND recipient = ? AND action_kind <> '' AND action_status = ?`,
		newStatus, actedAt, actedAt, id, username, expectedStatus)
	if err != nil {
		return false, fmt.Errorf("transition message action: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (db *DB) DeleteUserMessage(id, username string) (bool, error) {
	if db == nil || db.SQLDB == nil || id == "" || username == "" {
		return false, nil
	}
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	result, err := db.Exec(`DELETE FROM user_messages
		WHERE id = ? AND recipient = ? AND (action_kind = '' OR action_status <> ?)`,
		id, username, core.MessageActionPending)
	if err != nil {
		return false, fmt.Errorf("delete user message: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

// DeleteUserMessages removes every dismissible message owned by one user.
// Pending action messages remain available until their workflow is resolved.
func (db *DB) DeleteUserMessages(username string) (int64, error) {
	if db == nil || db.SQLDB == nil || username == "" {
		return 0, nil
	}
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	result, err := db.Exec(`DELETE FROM user_messages
		WHERE recipient = ? AND (action_kind = '' OR action_status <> ?)`,
		username, core.MessageActionPending)
	if err != nil {
		return 0, fmt.Errorf("delete user messages: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted user messages: %w", err)
	}
	return rows, nil
}

// DeleteMessagesByDedupeKey removes every non-action notification for one workflow event.
func (db *DB) DeleteMessagesByDedupeKey(dedupeKey string) (int64, error) {
	if db == nil || db.SQLDB == nil {
		return 0, core.ErrDatabaseUnavailable
	}
	dedupeKey = SanitizeInputString(strings.TrimSpace(dedupeKey), 255)
	if dedupeKey == "" {
		return 0, nil
	}
	result, err := db.Exec(`DELETE FROM user_messages WHERE dedupe_key = ? AND action_kind = ''`, dedupeKey)
	if err != nil {
		return 0, fmt.Errorf("delete workflow notifications: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted workflow notifications: %w", err)
	}
	return rows, nil
}
