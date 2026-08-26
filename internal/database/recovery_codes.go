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
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"renop/internal/core"
)

func validSelectorHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func (db *DB) accountIdentity(identifier string) (userID, username string, err error) {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if strings.Contains(identifier, "@") {
		email, valid := core.NormalizeEmail(identifier)
		if !valid || email == "" {
			return "", "", core.ErrRecoveryCodesInvalid
		}
		err = db.QueryRow(`SELECT p.user_id, p.username FROM user_profiles p
			JOIN user_account_security security ON security.user_id = p.user_id
			WHERE security.email = ?`, email).Scan(&userID, &username)
	} else {
		normalizedUsername, valid := core.NormalizeUsername(identifier)
		if !valid {
			return "", "", core.ErrRecoveryCodesInvalid
		}
		err = db.QueryRow(`SELECT user_id, username FROM user_profiles WHERE username = ?`, normalizedUsername).
			Scan(&userID, &username)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", core.ErrRecoveryCodesInvalid
	}
	if err != nil {
		return "", "", fmt.Errorf("resolve recovery account: %w", err)
	}
	return userID, strings.ToLower(username), nil
}

// ReplaceRecoveryCodes atomically invalidates an earlier set and stores twelve new verifiers.
func (db *DB) ReplaceRecoveryCodes(username string, codes []core.RecoveryCodeHash) error {
	if len(codes) != core.RecoveryCodeCount {
		return errors.New("exactly twelve recovery-code hashes are required")
	}
	username = strings.ToLower(strings.TrimSpace(username))
	userID, err := db.userIDForExistingAccount(username)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		if !validSelectorHash(code.SelectorHash) || code.PasswordHash == "" ||
			len(code.PasswordHash) > 255 || code.CreatedAt <= 0 {
			return errors.New("recovery-code hash is invalid")
		}
		if _, duplicate := seen[code.SelectorHash]; duplicate {
			return errors.New("recovery-code selectors must be unique")
		}
		seen[code.SelectorHash] = struct{}{}
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin recovery-code replacement: %w", err)
	}
	defer tx.Rollback()
	if err := lockAccountLoginMethodsTx(tx, userID); err != nil {
		return fmt.Errorf("lock account security for recovery-code replacement: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM user_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("invalidate previous recovery codes: %w", err)
	}
	for _, code := range codes {
		if _, err := tx.Exec(`INSERT INTO user_recovery_codes
			(user_id, selector_hash, password_hash, created_at, used_at)
			VALUES (?, ?, ?, ?, 0)`, userID, code.SelectorHash, code.PasswordHash, code.CreatedAt); err != nil {
			return fmt.Errorf("store recovery-code verifier: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recovery-code replacement: %w", err)
	}
	return nil
}

func recoveryPlaceholders(selectorHashes []string) (string, []any, error) {
	if len(selectorHashes) != core.RecoveryCodesRequired {
		return "", nil, core.ErrRecoveryCodesInvalid
	}
	selectors := append([]string(nil), selectorHashes...)
	slices.Sort(selectors)
	if slices.ContainsFunc(selectors, func(selector string) bool { return !validSelectorHash(selector) }) {
		return "", nil, core.ErrRecoveryCodesInvalid
	}
	for index := 1; index < len(selectors); index++ {
		if selectors[index] == selectors[index-1] {
			return "", nil, core.ErrRecoveryCodesInvalid
		}
	}
	arguments := make([]any, len(selectors))
	placeholders := make([]string, len(selectors))
	for index, selector := range selectors {
		arguments[index] = selector
		placeholders[index] = "?"
	}
	return strings.Join(placeholders, ","), arguments, nil
}

// GetRecoveryCodes returns unused verifier candidates without exposing whether an account exists to callers.
func (db *DB) GetRecoveryCodes(identifier string, selectorHashes []string) (string, []core.RecoveryCodeRecord, error) {
	placeholders, selectorArguments, err := recoveryPlaceholders(selectorHashes)
	if err != nil {
		return "", nil, err
	}
	userID, username, err := db.accountIdentity(identifier)
	if err != nil {
		return "", nil, err
	}
	arguments := make([]any, 0, len(selectorArguments)+1)
	arguments = append(arguments, userID)
	arguments = append(arguments, selectorArguments...)
	rows, err := db.Query(`SELECT selector_hash, password_hash FROM user_recovery_codes
		WHERE user_id = ? AND used_at = 0 AND selector_hash IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return "", nil, fmt.Errorf("load recovery-code verifiers: %w", err)
	}
	defer rows.Close()
	records := make([]core.RecoveryCodeRecord, 0, core.RecoveryCodesRequired)
	for rows.Next() {
		var record core.RecoveryCodeRecord
		if err := rows.Scan(&record.SelectorHash, &record.PasswordHash); err != nil {
			return "", nil, fmt.Errorf("scan recovery-code verifier: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	return username, records, nil
}

// ResetPasswordWithRecoveryCodes consumes four unused codes, resets the password, and revokes sessions atomically.
func (db *DB) ResetPasswordWithRecoveryCodes(identifier string, selectorHashes []string,
	passwordHash string, updatedAt int64) (string, error) {
	if passwordHash == "" || len(passwordHash) > 255 {
		return "", core.ErrRecoveryCodesInvalid
	}
	placeholders, selectorArguments, err := recoveryPlaceholders(selectorHashes)
	if err != nil {
		return "", err
	}
	userID, username, err := db.accountIdentity(identifier)
	if err != nil {
		return "", err
	}
	tx, err := db.Begin()
	if err != nil {
		return "", fmt.Errorf("begin password recovery: %w", err)
	}
	defer tx.Rollback()
	if err := lockAccountLoginMethodsTx(tx, userID); err != nil {
		return "", fmt.Errorf("lock account security for password recovery: %w", err)
	}
	arguments := make([]any, 0, len(selectorArguments)+1)
	arguments = append(arguments, userID)
	arguments = append(arguments, selectorArguments...)
	var available int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM user_recovery_codes
		WHERE user_id = ? AND used_at = 0 AND selector_hash IN (`+placeholders+`)`,
		arguments...).Scan(&available); err != nil {
		return "", fmt.Errorf("inspect recovery-code state: %w", err)
	}
	if available != core.RecoveryCodesRequired {
		return "", fmt.Errorf("recovery-code availability changed: %w", core.ErrRecoveryCodesInvalid)
	}
	updateArguments := make([]any, 0, len(selectorArguments)+2)
	updateArguments = append(updateArguments, updatedAt, userID)
	updateArguments = append(updateArguments, selectorArguments...)
	result, err := tx.Exec(`UPDATE user_recovery_codes SET used_at = ?
		WHERE user_id = ? AND used_at = 0 AND selector_hash IN (`+placeholders+`)`, updateArguments...)
	if err != nil {
		return "", fmt.Errorf("consume recovery codes: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("count consumed recovery codes: %w", err)
	}
	if affected != core.RecoveryCodesRequired {
		return "", fmt.Errorf("recovery-code consumption changed: %w", core.ErrRecoveryCodesInvalid)
	}
	passwordResult, err := tx.Exec(`UPDATE tokens SET encrypted_secret = ? WHERE name = ?`, passwordHash, username)
	if err != nil {
		return "", fmt.Errorf("reset recovered password: %w", err)
	}
	affected, err = passwordResult.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("count recovered password updates: %w", err)
	}
	if affected != 1 {
		return "", fmt.Errorf("recovered account changed: %w", core.ErrRecoveryCodesInvalid)
	}
	if err := ensureAccountSecurityTx(tx, userID, updatedAt); err != nil {
		return "", fmt.Errorf("initialize recovered account security: %w", err)
	}
	if _, err := tx.Exec(`UPDATE user_account_security SET password_login_enabled = 1, updated_at = ?
		WHERE user_id = ?`, updatedAt, userID); err != nil {
		return "", fmt.Errorf("restore password login: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE username = ?`, username); err != nil {
		return "", fmt.Errorf("revoke sessions after password recovery: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit password recovery: %w", err)
	}
	db.tokenCache.Delete(username)
	db.tokenSecretCache.DeleteFunc(func(_ string, token *core.AccessToken) bool {
		return token == nil || strings.EqualFold(token.Name, username)
	})
	db.sessionCache.DeleteFunc(func(_ string, session *core.Session) bool {
		return session == nil || strings.EqualFold(session.Username, username)
	})
	return username, nil
}
