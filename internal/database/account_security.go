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
	"strings"
	"time"

	"renop/internal/core"
)

func uniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "duplicate entry") ||
		strings.Contains(message, "duplicate key")
}

func ensureAccountSecurityTx(tx *Tx, userID string, updatedAt int64) error {
	var exists int
	err := tx.QueryRow(`SELECT 1 FROM user_account_security WHERE user_id = ?`, userID).Scan(&exists)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.Exec(`INSERT INTO user_account_security
		(user_id, email, password_login_enabled, updated_at) VALUES (?, NULL, 1, ?)`, userID, updatedAt)
	return err
}

func lockAccountLoginMethodsTx(tx *Tx, userID string) error {
	_, err := tx.Exec(`UPDATE user_profiles SET updated_at = updated_at WHERE user_id = ?`, userID)
	return err
}

func hasExternalLoginTx(tx *Tx, userID, username string) (bool, error) {
	var fidoCount, githubCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM fido_devices WHERE username = ?`, username).Scan(&fidoCount); err != nil {
		return false, err
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM github_identities WHERE user_id = ?`, userID).Scan(&githubCount); err != nil {
		return false, err
	}
	return fidoCount > 0 || githubCount > 0, nil
}

func hasLoginWithoutFidoTx(tx *Tx, userID, username, excludedDeviceID string) (bool, error) {
	var encryptedSecret string
	var passwordEnabled, githubCount, remainingFido int
	if err := tx.QueryRow(`SELECT t.encrypted_secret, COALESCE(security.password_login_enabled, 1)
		FROM tokens t JOIN user_profiles profile ON profile.username = t.name
		LEFT JOIN user_account_security security ON security.user_id = profile.user_id
		WHERE profile.user_id = ?`, userID).Scan(&encryptedSecret, &passwordEnabled); err != nil {
		return false, err
	}
	if passwordEnabled != 0 && encryptedSecret != "" {
		return true, nil
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM github_identities WHERE user_id = ?`, userID).Scan(&githubCount); err != nil {
		return false, err
	}
	if githubCount > 0 {
		return true, nil
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM fido_devices WHERE username = ? AND id <> ?`,
		username, excludedDeviceID).Scan(&remainingFido); err != nil {
		return false, err
	}
	return remainingFido > 0, nil
}

func hasLoginWithoutGitHubTx(tx *Tx, userID, username string) (bool, error) {
	var encryptedSecret string
	var passwordEnabled, fidoCount int
	if err := tx.QueryRow(`SELECT t.encrypted_secret, COALESCE(security.password_login_enabled, 1)
		FROM tokens t JOIN user_profiles profile ON profile.username = t.name
		LEFT JOIN user_account_security security ON security.user_id = profile.user_id
		WHERE profile.user_id = ?`, userID).Scan(&encryptedSecret, &passwordEnabled); err != nil {
		return false, err
	}
	if passwordEnabled != 0 && encryptedSecret != "" {
		return true, nil
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM fido_devices WHERE username = ?`, username).Scan(&fidoCount); err != nil {
		return false, err
	}
	return fidoCount > 0, nil
}

// GetTokenByEmail resolves a private normalized email to its access-token account.
func (db *DB) GetTokenByEmail(email string) (*core.AccessToken, error) {
	email, valid := core.NormalizeEmail(email)
	if !valid || email == "" {
		return nil, nil
	}
	row := db.QueryRow(`SELECT token.name, token.type, token.type_value, token.encrypted_secret,
		token.password_hash, token.tokens_json, token.created_at, token.description,
		token.expires_at, token.permissions_json
		FROM user_account_security security
		JOIN user_profiles profile ON profile.user_id = security.user_id
		JOIN tokens token ON token.name = profile.username
		WHERE security.email = ?`, email)
	var tokenName, tokenType, encryptedSecret, passwordHash, tokensJSON, createdAt, description, permissionsJSON string
	var typeValue int32
	var expiresAt sql.NullInt64
	if err := row.Scan(&tokenName, &tokenType, &typeValue, &encryptedSecret, &passwordHash,
		&tokensJSON, &createdAt, &description, &expiresAt, &permissionsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get token by private email: %w", err)
	}
	token, err := parseTokenRow(tokenName, tokenType, typeValue, encryptedSecret, passwordHash,
		tokensJSON, createdAt, description, expiresAt, permissionsJSON)
	if err != nil {
		return nil, err
	}
	db.tokenCache.Set(token.Name, token, 10*time.Minute)
	return token, nil
}

// GetAccountSecurity returns private authentication state for one username.
func (db *DB) GetAccountSecurity(username string) (*core.AccountSecurity, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if _, valid := core.NormalizeUsername(username); !valid {
		return nil, core.ErrUserProfileNotFound
	}
	security := &core.AccountSecurity{}
	var passwordEnabled, passwordConfigured, githubLinked int
	err := db.QueryRow(`SELECT COALESCE(security.email, ''),
		COALESCE(security.password_login_enabled, 1),
		CASE WHEN token.encrypted_secret <> '' THEN 1 ELSE 0 END,
		(SELECT COUNT(*) FROM fido_devices fido WHERE fido.username = profile.username),
		CASE WHEN EXISTS (SELECT 1 FROM github_identities github WHERE github.user_id = profile.user_id)
			THEN 1 ELSE 0 END,
		(SELECT COUNT(*) FROM user_recovery_codes recovery WHERE recovery.user_id = profile.user_id),
		(SELECT COUNT(*) FROM user_recovery_codes recovery
			WHERE recovery.user_id = profile.user_id AND recovery.used_at = 0),
		COALESCE((SELECT MAX(recovery.created_at) FROM user_recovery_codes recovery
			WHERE recovery.user_id = profile.user_id), 0)
		FROM user_profiles profile JOIN tokens token ON token.name = profile.username
		LEFT JOIN user_account_security security ON security.user_id = profile.user_id
		WHERE profile.username = ?`, username).Scan(
		&security.Email, &passwordEnabled, &passwordConfigured, &security.FidoDeviceCount,
		&githubLinked, &security.RecoveryCodeCount, &security.RecoveryCodesRemaining,
		&security.RecoveryGeneratedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrUserProfileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get account security for %s: %w", username, err)
	}
	security.PasswordLoginEnabled = passwordEnabled != 0
	security.PasswordConfigured = passwordConfigured != 0
	security.GitHubLinked = githubLinked != 0
	security.CanDisablePasswordLogin = security.FidoDeviceCount > 0 || security.GitHubLinked
	return security, nil
}

// PasswordLoginEnabled returns the lightweight password-login policy used on authentication hot paths.
func (db *DB) PasswordLoginEnabled(username string) (bool, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if _, valid := core.NormalizeUsername(username); !valid {
		return false, core.ErrUserProfileNotFound
	}
	var enabled int
	err := db.QueryRow(`SELECT COALESCE(security.password_login_enabled, 1)
		FROM user_profiles profile
		LEFT JOIN user_account_security security ON security.user_id = profile.user_id
		WHERE profile.username = ?`, username).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, core.ErrUserProfileNotFound
	}
	if err != nil {
		return false, fmt.Errorf("get password-login policy for %s: %w", username, err)
	}
	return enabled != 0, nil
}

// UpdateAccountEmail stores or clears a private normalized login email.
func (db *DB) UpdateAccountEmail(username, email string, updatedAt int64) (*core.AccountSecurity, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	email, valid := core.NormalizeEmail(email)
	if !valid {
		return nil, errors.New("email address is invalid")
	}
	userID, err := db.userIDForExistingAccount(username)
	if err != nil {
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin private email update: %w", err)
	}
	defer tx.Rollback()
	if err := lockAccountLoginMethodsTx(tx, userID); err != nil {
		return nil, fmt.Errorf("lock account security for private email update: %w", err)
	}
	if email != "" {
		var ownerID string
		err = tx.QueryRow(`SELECT user_id FROM user_account_security WHERE email = ?`, email).Scan(&ownerID)
		if err == nil && ownerID != userID {
			return nil, core.ErrEmailAlreadyExists
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("inspect private email ownership: %w", err)
		}
	}
	if err := ensureAccountSecurityTx(tx, userID, updatedAt); err != nil {
		return nil, fmt.Errorf("initialize account security: %w", err)
	}
	var emailValue any
	if email != "" {
		emailValue = email
	}
	if _, err := tx.Exec(`UPDATE user_account_security SET email = ?, updated_at = ? WHERE user_id = ?`,
		emailValue, updatedAt, userID); err != nil {
		if uniqueConstraintError(err) {
			return nil, core.ErrEmailAlreadyExists
		}
		return nil, fmt.Errorf("update private email: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit private email update: %w", err)
	}
	return db.GetAccountSecurity(username)
}

// SetPasswordLoginEnabled updates password-login policy while preserving another login method.
func (db *DB) SetPasswordLoginEnabled(username string, enabled bool, updatedAt int64) (*core.AccountSecurity, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	userID, err := db.userIDForExistingAccount(username)
	if err != nil {
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin password-login update: %w", err)
	}
	defer tx.Rollback()
	if err := lockAccountLoginMethodsTx(tx, userID); err != nil {
		return nil, fmt.Errorf("lock account login methods: %w", err)
	}
	var encryptedSecret string
	if err := tx.QueryRow(`SELECT encrypted_secret FROM tokens WHERE name = ?`, username).Scan(&encryptedSecret); err != nil {
		return nil, fmt.Errorf("inspect account password: %w", err)
	}
	if enabled && encryptedSecret == "" {
		return nil, core.ErrPasswordNotConfigured
	}
	if !enabled {
		hasExternalLogin, err := hasExternalLoginTx(tx, userID, username)
		if err != nil {
			return nil, fmt.Errorf("inspect alternate login methods: %w", err)
		}
		if !hasExternalLogin {
			return nil, core.ErrLastLoginMethod
		}
	}
	if err := ensureAccountSecurityTx(tx, userID, updatedAt); err != nil {
		return nil, fmt.Errorf("initialize account security: %w", err)
	}
	if _, err := tx.Exec(`UPDATE user_account_security
		SET password_login_enabled = ?, updated_at = ? WHERE user_id = ?`,
		boolInt(enabled), updatedAt, userID); err != nil {
		return nil, fmt.Errorf("update password-login policy: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit password-login update: %w", err)
	}
	return db.GetAccountSecurity(username)
}

// SetAccountPassword replaces the password hash and enables password login atomically.
func (db *DB) SetAccountPassword(username, passwordHash string, updatedAt int64) error {
	username = strings.ToLower(strings.TrimSpace(username))
	if passwordHash == "" || len(passwordHash) > 255 {
		return errors.New("password hash is invalid")
	}
	userID, err := db.userIDForExistingAccount(username)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin account password update: %w", err)
	}
	defer tx.Rollback()
	if err := lockAccountLoginMethodsTx(tx, userID); err != nil {
		return fmt.Errorf("lock account security for password update: %w", err)
	}
	token, err := tokenByNameTx(tx, username)
	if err != nil {
		return fmt.Errorf("reload account before password update: %w", err)
	}
	if token == nil {
		return core.ErrUserProfileNotFound
	}
	if _, err := tx.Exec(`UPDATE tokens SET encrypted_secret = ? WHERE name = ?`, passwordHash, username); err != nil {
		return fmt.Errorf("update account password: %w", err)
	}
	if err := ensureAccountSecurityTx(tx, userID, updatedAt); err != nil {
		return fmt.Errorf("initialize account security: %w", err)
	}
	if _, err := tx.Exec(`UPDATE user_account_security SET password_login_enabled = 1, updated_at = ?
		WHERE user_id = ?`, updatedAt, userID); err != nil {
		return fmt.Errorf("enable password login: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit account password update: %w", err)
	}
	updatedToken := *token
	updatedToken.EncryptedSecret = passwordHash
	db.finishTokenUpdate(username, &updatedToken)
	return nil
}
