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
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"renop/internal/core"
	"renop/internal/mail"
)

// QueueEmailPasswordReset atomically stores a bounded verification code and its queued email.
func (db *DB) QueueEmailPasswordReset(job *mail.Job, codeHash, key, ip string, rate mail.Rate) (bool, error) {
	if job == nil || job.Scene != "password_reset" || !validSelectorHash(codeHash) ||
		job.TicketHash == "" || ip == "" || job.ExpiresAt-job.CreatedAt > (10*time.Minute).Milliseconds() {
		return false, core.ErrEmailCodeInvalid
	}
	email, valid := core.NormalizeEmail(job.Message.To)
	if !valid || email == "" {
		return false, core.ErrEmailCodeInvalid
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
	userID, _, credentialHash, securityUpdatedAt, err := passwordResetIdentityTx(tx, email)
	if err != nil {
		return false, err
	}
	job.UserID = userID
	created, err := queueMailJobTx(tx, job, key, ip, rate)
	if err != nil || !created {
		return created, err
	}
	var previous int64
	err = tx.QueryRow(`SELECT created_at FROM user_password_resets WHERE email = ?`, email).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil && previous+time.Minute.Milliseconds() > job.CreatedAt {
		return false, mail.ErrRateLimited
	}
	if _, err = tx.Exec(`DELETE FROM user_password_resets WHERE email = ? OR expires_at <= ?`, email, job.CreatedAt); err != nil {
		return false, err
	}
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM user_password_resets`).Scan(&count); err != nil {
		return false, err
	}
	if count >= mail.MaxPendingJobs {
		return false, mail.ErrQueueFull
	}
	_, err = tx.Exec(`INSERT INTO user_password_resets
        (email, user_id, code_hash, credential_hash, security_updated_at, attempts, created_at, expires_at)
        VALUES (?, ?, ?, ?, ?, 0, ?, ?)`, email, userID, codeHash, credentialHash, securityUpdatedAt, job.CreatedAt, job.ExpiresAt)
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// passwordResetIdentityTx binds email verification to the live identity, password, and security revision.
func passwordResetIdentityTx(tx *Tx, email string) (userID, username, credentialHash string, updatedAt int64, err error) {
	err = tx.QueryRow(`SELECT user_id FROM user_account_security WHERE email = ?`, email).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", 0, nil
	}
	if err != nil {
		return "", "", "", 0, err
	}
	if err = lockAccountLoginMethodsTx(tx, userID); errors.Is(err, core.ErrAccountDeleted) {
		return "", "", "", 0, nil
	} else if err != nil {
		return "", "", "", 0, err
	}
	var passwordHash string
	err = tx.QueryRow(`SELECT profile.username, token.encrypted_secret, security.updated_at
        FROM user_account_security security JOIN user_profiles profile ON profile.user_id = security.user_id
        JOIN tokens token ON token.name = profile.username
        WHERE security.user_id = ? AND security.email = ? AND token.deleted_at = 0`, userID, email).
		Scan(&username, &passwordHash, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", 0, nil
	}
	if err != nil {
		return "", "", "", 0, err
	}
	digest := sha256.Sum256([]byte(passwordHash))
	return userID, username, hex.EncodeToString(digest[:]), updatedAt, nil
}

// ResetPasswordWithEmailCode consumes one live code and revokes all browser sessions in one transaction.
func (db *DB) ResetPasswordWithEmailCode(email, codeHash, passwordHash string, updatedAt int64) (string, error) {
	email, valid := core.NormalizeEmail(email)
	if !valid || email == "" || !validSelectorHash(codeHash) || passwordHash == "" || len(passwordHash) > 255 || updatedAt <= 0 {
		return "", core.ErrEmailCodeInvalid
	}
	db.mailWriteMu.Lock()
	defer db.mailWriteMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err = lockMailTx(tx); err != nil {
		return "", err
	}
	var userID, expected, credentialHash string
	var securityUpdatedAt, expiresAt int64
	var attempts int
	err = tx.QueryRow(`SELECT user_id, code_hash, credential_hash, security_updated_at, attempts, expires_at
        FROM user_password_resets WHERE email = ?`, email).
		Scan(&userID, &expected, &credentialHash, &securityUpdatedAt, &attempts, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", core.ErrEmailCodeInvalid
	}
	if err != nil {
		return "", err
	}
	if expiresAt <= updatedAt || attempts >= 5 {
		return "", core.ErrEmailCodeInvalid
	}
	if subtle.ConstantTimeCompare([]byte(codeHash), []byte(expected)) != 1 || userID == "" {
		if _, err = tx.Exec(`UPDATE user_password_resets SET attempts = attempts + 1 WHERE email = ?`, email); err != nil {
			return "", err
		}
		if err = tx.Commit(); err != nil {
			return "", err
		}
		return "", core.ErrEmailCodeInvalid
	}
	currentID, username, currentCredentialHash, currentSecurityUpdatedAt, err := passwordResetIdentityTx(tx, email)
	if err != nil {
		return "", err
	}
	if currentID != userID || currentCredentialHash != credentialHash || currentSecurityUpdatedAt != securityUpdatedAt {
		return "", core.ErrEmailCodeInvalid
	}
	if err = resetAccountPasswordTx(tx, userID, username, passwordHash, updatedAt); err != nil {
		return "", err
	}
	if _, err = tx.Exec(`DELETE FROM user_password_resets WHERE email = ?`, email); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	db.invalidateRecoveredAccount(username)
	return username, nil
}
