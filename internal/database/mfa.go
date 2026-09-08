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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"renop/internal/core"
)

func mfaStateQuery(query func(string, ...any) row, username string) (*core.MFAState, error) {
	state := &core.MFAState{}
	var password, githubID string
	var passwordEnabled, passkey int
	var updatedAt, deletedAt, bannedAt, bannedUntil, expiresAt int64
	err := query(`SELECT profile.user_id, token.encrypted_secret, token.deleted_at,
		COALESCE(security.password_login_enabled, 1), COALESCE(security.updated_at, 0),
		token.banned_at, COALESCE(token.banned_until, 0), COALESCE(token.expires_at, 0)
		FROM user_profiles profile JOIN tokens token ON token.name = profile.username
		LEFT JOIN user_account_security security ON security.user_id = profile.user_id
		WHERE profile.username = ?`, strings.ToLower(username)).Scan(&state.UserID, &password, &deletedAt, &passwordEnabled, &updatedAt, &bannedAt, &bannedUntil, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrUserProfileNotFound
	}
	if err != nil {
		return nil, err
	}
	if deletedAt > 0 {
		return nil, core.ErrAccountDeleted
	}
	state.PasswordHash = password
	state.PasswordLoginEnabled = passwordEnabled != 0
	err = query(`SELECT secret, revision, passkey_enabled, last_step, window_start, failures FROM user_mfa WHERE user_id = ?`, state.UserID).
		Scan(&state.Secret, &state.Revision, &passkey, &state.LastStep, &state.WindowStart, &state.Failures)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	state.Passkey = passkey != 0
	var identity int64
	err = query(`SELECT github_user_id FROM github_identities WHERE user_id = ?`, state.UserID).Scan(&identity)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		githubID = fmt.Sprint(identity)
	}
	state.GitHubID = identity
	state.Snapshot = fmt.Sprintf("%x", sha256.Sum256(fmt.Appendf(nil, "%s\x00%s\x00%d\x00%d\x00%s\x00%s\x00%d\x00%d\x00%d", state.UserID, password, passwordEnabled, updatedAt, githubID, state.Revision, bannedAt, bannedUntil, expiresAt)))
	return state, nil
}

// GetMFAState loads the current policy and credential snapshot without caching reusable secrets.
func (db *DB) GetMFAState(username string) (*core.MFAState, error) {
	return mfaStateQuery(db.QueryRow, username)
}

// UpdateMFA changes confirmed second factors and revokes other sessions under the account lock.
func (db *DB) UpdateMFA(username, snapshot, secret string, passkey bool, lastStep int64, keepSession string) error {
	if len(secret) > 1024 || len(snapshot) != 64 {
		return core.ErrMFAInvalid
	}
	username = strings.ToLower(username)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockAccountByUsernameTx(tx, username); err != nil {
		return err
	}
	current, err := mfaStateQuery(tx.QueryRow, username)
	if err != nil {
		return err
	}
	if current.Snapshot != snapshot {
		return core.ErrMFAInvalid
	}
	var sessions int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM sessions WHERE session_token = ? AND username = ?`, keepSession, username).Scan(&sessions); err != nil {
		return err
	}
	if sessions != 1 {
		return core.ErrMFAInvalid
	}
	if passkey {
		var count int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM fido_devices WHERE username = ?`, username).Scan(&count); err != nil {
			return err
		}
		primary, err := hasPrimaryWithoutFidoTx(tx, current.UserID, username)
		if err != nil {
			return err
		}
		if count == 0 || !primary {
			return core.ErrLastLoginMethod
		}
	}
	if _, err = tx.Exec(`DELETE FROM user_mfa WHERE user_id = ?`, current.UserID); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO user_mfa (user_id, secret, revision, passkey_enabled, last_step, window_start, failures) VALUES (?, ?, ?, ?, ?, 0, 0)`, current.UserID, secret, uuid.NewString(), boolInt(passkey), lastStep); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM sessions WHERE username = ? AND session_token <> ?`, username, keepSession); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	db.invalidateRecoveredAccount(username)
	return nil
}

// ConsumeMFACode prevents replay and limits guesses across login challenges and processes.
func (db *DB) ConsumeMFACode(username, revision string, step, now int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockAccountByUsernameTx(tx, username); err != nil {
		return err
	}
	state, err := mfaStateQuery(tx.QueryRow, username)
	if err != nil {
		return err
	}
	if state.Secret == "" || state.Revision != revision {
		return core.ErrMFAInvalid
	}
	if now-state.WindowStart >= int64(5*time.Minute/time.Millisecond) {
		state.WindowStart, state.Failures = now, 0
	}
	if state.Failures >= 5 {
		return core.ErrMFAInvalid
	}
	valid := step > state.LastStep && step >= 0
	if valid {
		state.LastStep = step
	} else {
		state.Failures++
	}
	if _, err = tx.Exec(`UPDATE user_mfa SET last_step = ?, failures = ?, window_start = ? WHERE user_id = ?`, state.LastStep, state.Failures, state.WindowStart, state.UserID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if !valid {
		return core.ErrMFAInvalid
	}
	return nil
}

func passkeySecondFactorTx(tx *Tx, userID string) (bool, error) {
	var enabled int
	err := tx.QueryRow(`SELECT passkey_enabled FROM user_mfa WHERE user_id = ?`, userID).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return enabled != 0, err
}

func hasPrimaryWithoutFidoTx(tx *Tx, userID, username string) (bool, error) {
	var password string
	var enabled, github int
	if err := tx.QueryRow(`SELECT token.encrypted_secret, COALESCE(security.password_login_enabled, 1)
		FROM tokens token JOIN user_profiles profile ON profile.username = token.name
		LEFT JOIN user_account_security security ON security.user_id = profile.user_id
		WHERE profile.user_id = ? AND token.name = ?`, userID, username).Scan(&password, &enabled); err != nil {
		return false, err
	}
	if password != "" && enabled != 0 {
		return true, nil
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM github_identities WHERE user_id = ?`, userID).Scan(&github); err != nil {
		return false, err
	}
	return github > 0, nil
}
