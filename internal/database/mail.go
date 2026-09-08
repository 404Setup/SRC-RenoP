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
	"net/netip"
	"slices"
	"strings"
	"time"

	"renop/internal/core"
	"renop/internal/mail"

	"github.com/goccy/go-json"
)

const mailJobColumns = "id, account_id, user_id, actor, scene, status, payload, result_json, checks, created_at, updated_at, next_at, expires_at"

func (db *DB) ensureMailControl() error {
	var id string
	err := db.QueryRow(`SELECT id FROM mail_control WHERE id = 'queue'`).Scan(&id)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = db.Exec(`INSERT INTO mail_control (id, lease_owner, lease_until, next_send_at, audit_cursor, enabled_since) VALUES ('queue', '', 0, 0, 0, 0)`)
	if uniqueConstraintError(err) {
		return nil
	}
	return err
}

func lockMailTx(tx *Tx) error {
	_, err := tx.Exec(`UPDATE mail_control SET next_send_at = next_send_at WHERE id = 'queue'`)
	return err
}

// AcquireMailLease permits one serial network worker across restarts and server processes.
func (db *DB) AcquireMailLease(owner string, now int64) (mail.Control, bool, error) {
	if owner == "" || len(owner) > 64 {
		return mail.Control{}, false, errors.New("invalid mail lease owner")
	}
	db.mailWriteMu.Lock()
	defer db.mailWriteMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return mail.Control{}, false, err
	}
	defer tx.Rollback()
	if err = lockMailTx(tx); err != nil {
		return mail.Control{}, false, err
	}
	var control mail.Control
	if err = tx.QueryRow(`SELECT lease_owner, lease_until, next_send_at, audit_cursor, enabled_since FROM mail_control WHERE id = 'queue'`).Scan(&control.Owner, &control.LeaseUntil, &control.NextSendAt, &control.AuditCursor, &control.EnabledSince); err != nil {
		return control, false, err
	}
	if control.Owner != owner && control.LeaseUntil > now {
		return control, false, nil
	}
	control.Owner = owner
	control.LeaseUntil = now + int64(90*time.Second/time.Millisecond)
	if control.EnabledSince == 0 {
		control.EnabledSince = now
		if err = tx.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM audit_logs WHERE kind = 'audit'`).Scan(&control.AuditCursor); err != nil {
			return control, false, err
		}
	}
	if _, err = tx.Exec(`UPDATE mail_control SET lease_owner = ?, lease_until = ?, audit_cursor = ?, enabled_since = ? WHERE id = 'queue'`, owner, control.LeaseUntil, control.AuditCursor, control.EnabledSince); err != nil {
		return control, false, err
	}
	return control, true, tx.Commit()
}

// ReleaseMailLease allows a replacement worker to start without waiting for the lease timeout.
func (db *DB) ReleaseMailLease(owner string) error {
	_, err := db.Exec(`UPDATE mail_control SET lease_owner = '', lease_until = 0 WHERE id = 'queue' AND lease_owner = ?`, owner)
	return err
}

func requireMailLeaseTx(tx *Tx, owner string, now int64) error {
	if err := lockMailTx(tx); err != nil {
		return err
	}
	var current string
	var until int64
	if err := tx.QueryRow(`SELECT lease_owner, lease_until FROM mail_control WHERE id = 'queue'`).Scan(&current, &until); err != nil {
		return err
	}
	if current != owner || until <= now {
		return mail.ErrLeaseLost
	}
	return nil
}

// QueueMailJob atomically deduplicates, bounds the queue, and debits manual IP limits.
func (db *DB) QueueMailJob(job *mail.Job, key, ip string, rate mail.Rate) (bool, error) {
	if job == nil || job.Message.Validate() != nil || job.ID != job.Message.ID || len(job.AccountID) > 64 || len(job.UserID) > 36 || len(job.Actor) > 64 || !slices.Contains(mail.Scenes, job.Scene) || job.ExpiresAt <= job.CreatedAt || job.ExpiresAt-job.CreatedAt > int64(24*time.Hour/time.Millisecond) {
		return false, errors.New("invalid mail job")
	}
	payload, err := mail.Seal(key, "job:"+job.ID, mail.JobPayload{Message: job.Message, TicketHash: job.TicketHash})
	if err != nil {
		return false, err
	}
	db.mailWriteMu.Lock()
	defer db.mailWriteMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if err = lockMailTx(tx); err != nil {
		return false, err
	}
	var exists int
	err = tx.QueryRow(`SELECT 1 FROM mail_jobs WHERE id = ?`, job.ID).Scan(&exists)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM mail_jobs WHERE status IN ('queued', 'paused', 'sending', 'checking')`).Scan(&count); err != nil {
		return false, err
	}
	if count >= mail.MaxPendingJobs {
		return false, mail.ErrQueueFull
	}
	if err = tx.QueryRow(`SELECT COUNT(*) FROM mail_jobs`).Scan(&count); err != nil {
		return false, err
	}
	if count >= mail.MaxPendingJobs+10000 {
		return false, mail.ErrQueueFull
	}
	if job.UserID != "" {
		if err = lockAccountLoginMethodsTx(tx, job.UserID); err != nil {
			return false, err
		}
	}
	if ip != "" {
		address, err := netip.ParseAddr(ip)
		if err != nil {
			return false, errors.New("invalid mail request IP")
		}
		ip = address.Unmap().String()
		if rate.Limit <= 0 || rate.Interval.Duration() <= 0 {
			return false, errors.New("invalid mail rate")
		}
		var used, expires int64
		err = tx.QueryRow(`SELECT used, expires_at FROM mail_rate_limits WHERE ip = ?`, ip).Scan(&used, &expires)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		if err == nil && expires > job.CreatedAt && used >= rate.Limit {
			return false, mail.ErrRateLimited
		}
		if err == nil {
			if expires <= job.CreatedAt {
				used = 0
				expires = job.CreatedAt + rate.Interval.Duration().Milliseconds()
			}
			_, err = tx.Exec(`UPDATE mail_rate_limits SET used = ?, period_start = ?, expires_at = ? WHERE ip = ?`, used+1, expires-rate.Interval.Duration().Milliseconds(), expires, ip)
		} else {
			if _, err = tx.Exec(`DELETE FROM mail_rate_limits WHERE expires_at <= ?`, job.CreatedAt); err != nil {
				return false, err
			}
			if err = tx.QueryRow(`SELECT COUNT(*) FROM mail_rate_limits WHERE expires_at > ?`, job.CreatedAt).Scan(&count); err != nil {
				return false, err
			}
			if count >= 10000 {
				return false, mail.ErrRateLimited
			}
			_, err = tx.Exec(`INSERT INTO mail_rate_limits (ip, period_start, used, expires_at) VALUES (?, ?, 1, ?)`, ip, job.CreatedAt, job.CreatedAt+rate.Interval.Duration().Milliseconds())
		}
		if err != nil {
			return false, err
		}
	}
	_, err = tx.Exec(`INSERT INTO mail_jobs (`+mailJobColumns+`) VALUES (?, ?, ?, ?, ?, 'queued', ?, '{}', 0, ?, ?, ?, ?)`, job.ID, job.AccountID, job.UserID, job.Actor, job.Scene, payload, job.CreatedAt, job.CreatedAt, job.CreatedAt, job.ExpiresAt)
	if err != nil {
		return false, err
	}
	job.Status = "queued"
	job.UpdatedAt = job.CreatedAt
	job.NextAt = job.CreatedAt
	return true, tx.Commit()
}

func scanMailJob(scanner messageScanner, key string) (*mail.Job, error) {
	job := &mail.Job{}
	var payload, result string
	if err := scanner.Scan(&job.ID, &job.AccountID, &job.UserID, &job.Actor, &job.Scene, &job.Status, &payload, &result, &job.Checks, &job.CreatedAt, &job.UpdatedAt, &job.NextAt, &job.ExpiresAt); err != nil {
		return nil, err
	}
	var private mail.JobPayload
	if err := mail.Open(key, "job:"+job.ID, payload, &private); err != nil {
		return nil, err
	}
	job.Message = private.Message
	job.Reservation = private.Reservation
	job.TicketHash = private.TicketHash
	if err := json.Unmarshal([]byte(result), &job.Result); err != nil {
		return nil, err
	}
	return job, nil
}

// GetMailJob returns private state for a caller that must enforce owner or capability access.
func (db *DB) GetMailJob(id, key string) (*mail.Job, error) {
	job, err := scanMailJob(db.QueryRow(`SELECT `+mailJobColumns+` FROM mail_jobs WHERE id = ?`, id), key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return job, err
}

// ListMailJobs returns a bounded administrator or account-owner view of recent jobs.
func (db *DB) ListMailJobs(userID, status, key string, limit, offset int) ([]*mail.Job, int, error) {
	limit = max(1, min(limit, 50))
	offset = max(0, min(offset, 10000))
	where := "created_at > 0"
	args := []any{}
	if userID != "" {
		where += " AND user_id = ?"
		args = append(args, userID)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM mail_jobs WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := db.Query(`SELECT `+mailJobColumns+` FROM mail_jobs WHERE `+where+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]*mail.Job, 0, min(total, limit))
	for rows.Next() {
		job, err := scanMailJob(rows, key)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, job)
	}
	return result, total, rows.Err()
}

// NextMailJob selects one due operation; delayed accounts do not block other accounts.
func (db *DB) NextMailJob(key string, now int64) (*mail.Job, error) {
	job, err := scanMailJob(db.QueryRow(`SELECT `+mailJobColumns+` FROM mail_jobs WHERE status IN ('queued', 'paused', 'checking') AND (next_at <= ? OR expires_at <= ?) ORDER BY next_at ASC, created_at ASC, id ASC LIMIT 1`, now, now), key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return job, err
}

// LoadMailAccount restores encrypted account counters and rotating OAuth credentials.
func (db *DB) LoadMailAccount(id, key string) (mail.AccountState, error) {
	var payload string
	state := mail.AccountState{}
	err := db.QueryRow(`SELECT payload FROM mail_accounts WHERE id = ?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	err = mail.Open(key, "account:"+id, payload, &state)
	return state, err
}

func saveMailAccountTx(tx *Tx, id, key string, state mail.AccountState, now int64) error {
	payload, err := mail.Seal(key, "account:"+id, state)
	if err != nil {
		return err
	}
	var exists int
	err = tx.QueryRow(`SELECT 1 FROM mail_accounts WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(`INSERT INTO mail_accounts (id, payload, updated_at) VALUES (?, ?, ?)`, id, payload, now)
	} else if err == nil {
		_, err = tx.Exec(`UPDATE mail_accounts SET payload = ?, updated_at = ? WHERE id = ?`, payload, now, id)
	}
	return err
}

// SaveMailAccount persists calibration or token rotation while retaining exclusive worker ownership.
func (db *DB) SaveMailAccount(owner, id, key string, state mail.AccountState, now int64) error {
	db.mailWriteMu.Lock()
	defer db.mailWriteMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = requireMailLeaseTx(tx, owner, now); err != nil {
		return err
	}
	if err = saveMailAccountTx(tx, id, key, state, now); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveMailAttempt atomically commits a job transition, credit reservation, and global delay.
func (db *DB) SaveMailAttempt(owner, key string, job *mail.Job, state *mail.AccountState, nextSendAt int64) error {
	if job == nil || !slices.Contains([]string{"queued", "paused", "sending", "checking", "accepted", "sent", "delivered", "failed", "unknown", "expired", "cancelled", "queued_provider"}, job.Status) {
		return errors.New("invalid mail job transition")
	}
	payload, err := mail.Seal(key, "job:"+job.ID, mail.JobPayload{Message: job.Message, Reservation: job.Reservation, TicketHash: job.TicketHash})
	if err != nil {
		return err
	}
	result, err := json.Marshal(job.Result)
	if err != nil {
		return err
	}
	db.mailWriteMu.Lock()
	defer db.mailWriteMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = requireMailLeaseTx(tx, owner, job.UpdatedAt); err != nil {
		return err
	}
	var currentStatus, currentResult string
	err = tx.QueryRow(`SELECT status, result_json FROM mail_jobs WHERE id = ?`, job.ID).Scan(&currentStatus, &currentResult)
	if errors.Is(err, sql.ErrNoRows) {
		return mail.ErrFinalized
	}
	if err != nil {
		return err
	}
	if !slices.Contains([]string{"queued", "paused", "sending", "checking"}, currentStatus) &&
		!(currentStatus == "unknown" && strings.Contains(currentResult, `"mail_submission_interrupted"`)) {
		return mail.ErrFinalized
	}
	if state != nil {
		if err = saveMailAccountTx(tx, job.AccountID, key, *state, job.UpdatedAt); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`UPDATE mail_jobs SET status = ?, payload = ?, result_json = ?, checks = ?, updated_at = ?, next_at = ? WHERE id = ?`, job.Status, payload, string(result), job.Checks, job.UpdatedAt, job.NextAt, job.ID); err != nil {
		return err
	}
	if nextSendAt > 0 {
		if _, err = tx.Exec(`UPDATE mail_control SET next_send_at = ? WHERE id = 'queue' AND lease_owner = ?`, nextSendAt, owner); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RecoverMailAttempts marks interrupted submissions indeterminate instead of sending duplicates.
func (db *DB) RecoverMailAttempts(owner string, now int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = requireMailLeaseTx(tx, owner, now); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE mail_jobs SET status = 'unknown', result_json = ?, updated_at = ? WHERE status = 'sending'`, `{"status":"unknown","code":"mail_submission_interrupted"}`, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// AdvanceMailAuditCursor acknowledges durable audit notifications only after their jobs exist.
func (db *DB) AdvanceMailAuditCursor(owner string, cursor, now int64) error {
	result, err := db.Exec(`UPDATE mail_control SET audit_cursor = ? WHERE id = 'queue' AND lease_owner = ? AND lease_until > ? AND audit_cursor < ?`, cursor, owner, now, cursor)
	if err != nil {
		return err
	}
	_, err = result.RowsAffected()
	return err
}

// MailAuditEvents reads a bounded ascending stream for durable email notifications.
func (db *DB) MailAuditEvents(cursor int64) ([]*core.AuditLogEntry, error) {
	rows, err := db.Query(`SELECT id, username, operator, action, details, ip, created_at FROM audit_logs WHERE id > ? AND kind = 'audit' ORDER BY id ASC LIMIT 32`, cursor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*core.AuditLogEntry, 0, 32)
	for rows.Next() {
		entry := &core.AuditLogEntry{}
		if err = rows.Scan(&entry.ID, &entry.Username, &entry.Operator, &entry.Action, &entry.Details, &entry.IP, &entry.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

// MailMessageEvents finds recent feature messages that have not yet acquired a mail job.
func (db *DB) MailMessageEvents(since int64) ([]*core.UserMessage, error) {
	rows, err := db.Query(`SELECT id, recipient, kind, title, body, created_at FROM user_messages WHERE created_at >= ? AND email_processed_at = 0 ORDER BY created_at ASC, id ASC LIMIT 32`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*core.UserMessage, 0, 32)
	for rows.Next() {
		message := &core.UserMessage{}
		if err = rows.Scan(&message.ID, &message.Recipient, &message.Kind, &message.Title, &message.Body, &message.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, message)
	}
	return result, rows.Err()
}

// AcknowledgeMailMessage marks an email decision after its queue insertion commits.
func (db *DB) AcknowledgeMailMessage(id string, now int64) error {
	_, err := db.Exec(`UPDATE user_messages SET email_processed_at = ? WHERE id = ? AND email_processed_at = 0`, now, id)
	return err
}

// PreviousMailLoginIP returns the preceding login network without exposing unrelated activity.
func (db *DB) PreviousMailLoginIP(username string, before int64) (string, error) {
	var ip string
	err := db.QueryRow(`SELECT ip FROM audit_logs WHERE username = ? AND action = 'LOGIN' AND kind = 'audit' AND id < ? ORDER BY id DESC LIMIT 1`, strings.ToLower(username), before).Scan(&ip)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return ip, err
}

// CleanMailData bounds expired IP rows and completed queue history.
func (db *DB) CleanMailData(now int64, accountIDs []string) error {
	if len(accountIDs) > mail.MaxAccounts {
		return errors.New("invalid mail account count")
	}
	if _, err := db.Exec(`DELETE FROM mail_rate_limits WHERE expires_at <= ?`, now); err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM mail_jobs WHERE updated_at < ? AND status NOT IN ('queued','paused','sending','checking')`, now-int64(7*24*time.Hour/time.Millisecond)); err != nil {
		return err
	}
	rows, err := db.Query(`SELECT id FROM mail_jobs WHERE status NOT IN ('queued','paused','sending','checking') ORDER BY updated_at DESC, id DESC LIMIT 2048 OFFSET 8000`)
	if err != nil {
		return err
	}
	var ids []any
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		if _, err = db.Exec(`DELETE FROM mail_jobs WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`) AND status NOT IN ('queued','paused','sending','checking')`, ids...); err != nil {
			return err
		}
	}
	args := make([]any, 0, len(accountIDs)+1)
	args = append(args, now-int64(24*time.Hour/time.Millisecond))
	query := `DELETE FROM mail_accounts WHERE updated_at < ?`
	if len(accountIDs) > 0 {
		query += ` AND id NOT IN (` + strings.TrimSuffix(strings.Repeat("?,", len(accountIDs)), ",") + `)`
		for _, id := range accountIDs {
			args = append(args, id)
		}
	}
	_, err = db.Exec(query, args...)
	return err
}
