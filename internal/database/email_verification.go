/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"renop/internal/core"
	"renop/internal/mail"
)

func accountEmailSessionTx(tx *Tx, username, session string) (*core.MFAState, error) {
	if session == "" {
		return nil, core.ErrEmailCodeInvalid
	}
	if err := lockAccountByUsernameTx(tx, username); err != nil {
		return nil, err
	}
	var count int
	now := time.Now().UnixMilli()
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sessions s JOIN tokens token ON token.name = s.username
		WHERE s.session_token = ? AND s.username = ? AND token.deleted_at = 0
		AND (token.expires_at IS NULL OR token.expires_at > ?)
		AND (token.banned_at = 0 OR (token.banned_until > 0 AND token.banned_until <= ?))`, session, username, now, now).Scan(&count); err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, core.ErrEmailCodeInvalid
	}
	return mfaStateQuery(tx.QueryRow, username)
}

// QueueAccountEmailChange stores the verification and its email atomically without replacing the current address.
func (db *DB) QueueAccountEmailChange(username, session string, job *mail.Job, codeHash, key, ip string, rate mail.Rate, alias ...bool) error {
	if job == nil || job.Scene != "email_verify" || !validSelectorHash(codeHash) || job.TicketHash == "" ||
		ip == "" || job.ExpiresAt-job.CreatedAt > (10*time.Minute).Milliseconds() || len(alias) > 1 {
		return core.ErrEmailCodeInvalid
	}
	email, valid := core.NormalizeEmail(job.Message.To)
	if !valid || email == "" {
		return core.ErrEmailCodeInvalid
	}
	username = strings.ToLower(username)
	db.mailWriteMu.Lock()
	defer db.mailWriteMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockMailTx(tx); err != nil {
		return err
	}
	account, err := accountEmailSessionTx(tx, username, session)
	if err != nil {
		return err
	}
	var previous int64
	err = tx.QueryRow(`SELECT created_at FROM user_email_changes WHERE user_id = ?`, account.UserID).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && previous+time.Minute.Milliseconds() > job.CreatedAt {
		return mail.ErrRateLimited
	}
	if _, err = tx.Exec(`DELETE FROM user_email_changes WHERE user_id = ? OR expires_at <= ?`, account.UserID, job.CreatedAt); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM user_email_changes`).Scan(&count); err != nil {
		return err
	}
	if count >= mail.MaxPendingJobs {
		return mail.ErrQueueFull
	}
	job.UserID = account.UserID
	created, err := queueMailJobTx(tx, job, key, ip, rate)
	if err != nil {
		return err
	}
	if !created {
		return core.ErrEmailCodeInvalid
	}
	isAlias := 0
	if len(alias) > 0 && alias[0] {
		isAlias = 1
	}
	_, err = tx.Exec(`INSERT INTO user_email_changes
		(user_id, email, code_hash, snapshot, session_hash, attempts, created_at, expires_at, is_alias)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?)`, account.UserID, email, codeHash, account.Snapshot,
		fmt.Sprintf("%x", sha256.Sum256([]byte(session))), job.CreatedAt, job.ExpiresAt, isAlias)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ConfirmAccountEmailChange consumes one live verification bound to the initiating account and session.
func (db *DB) ConfirmAccountEmailChange(username, session, email, codeHash string, now int64) (*core.AccountSecurity, error) {
	email, valid := core.NormalizeEmail(email)
	if !valid || email == "" || !validSelectorHash(codeHash) || now <= 0 {
		return nil, core.ErrEmailCodeInvalid
	}
	username = strings.ToLower(username)
	db.mailWriteMu.Lock()
	defer db.mailWriteMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = lockMailTx(tx); err != nil {
		return nil, err
	}
	account, err := accountEmailSessionTx(tx, username, session)
	if err != nil {
		return nil, err
	}
	var expected, snapshot, sessionHash, target string
	var expiresAt int64
	var attempts, isAlias int
	err = tx.QueryRow(`SELECT email, code_hash, snapshot, session_hash, attempts, expires_at, is_alias
		FROM user_email_changes WHERE user_id = ?`, account.UserID).
		Scan(&target, &expected, &snapshot, &sessionHash, &attempts, &expiresAt, &isAlias)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrEmailCodeInvalid
	}
	if err != nil {
		return nil, err
	}
	if expiresAt <= now || attempts >= 5 || target != email || snapshot != account.Snapshot ||
		sessionHash != fmt.Sprintf("%x", sha256.Sum256([]byte(session))) {
		return nil, core.ErrEmailCodeInvalid
	}
	if subtle.ConstantTimeCompare([]byte(codeHash), []byte(expected)) != 1 {
		if _, err = tx.Exec(`UPDATE user_email_changes SET attempts = attempts + 1 WHERE user_id = ?`, account.UserID); err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return nil, core.ErrEmailCodeInvalid
	}
	if isAlias != 0 {
		if _, err := reserveAccountEmailsTx(tx, account.UserID, []core.ProviderEmail{{Email: email, Verified: true}}, true); err != nil {
			return nil, err
		}
		if err := touchAccountSecurityTx(tx, account.UserID, now); err != nil {
			return nil, err
		}
	} else if err = updateAccountEmailTx(tx, account.UserID, email, now); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`DELETE FROM user_email_changes WHERE user_id = ?`, account.UserID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return db.GetAccountSecurity(username)
}

// UpdateAccountEmailFromSession updates an address only while its initiating credentials remain current.
func (db *DB) UpdateAccountEmailFromSession(username, session, email, snapshot string, now int64, providerEmails ...core.ProviderEmail) (*core.AccountSecurity, error) {
	email, valid := core.NormalizeEmail(email)
	if !valid || email == "" || !validSelectorHash(snapshot) || now <= 0 {
		return nil, core.ErrEmailCodeInvalid
	}
	username = strings.ToLower(username)
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	account, err := accountEmailSessionTx(tx, username, session)
	if err != nil {
		return nil, err
	}
	if account.Snapshot != snapshot {
		return nil, core.ErrEmailCodeInvalid
	}
	if _, err := reserveAccountEmailsTx(tx, account.UserID, providerEmails, true); err != nil {
		return nil, err
	}
	if err = updateAccountEmailTx(tx, account.UserID, email, now); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`DELETE FROM user_email_changes WHERE user_id = ?`, account.UserID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return db.GetAccountSecurity(username)
}
