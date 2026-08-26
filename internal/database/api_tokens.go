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
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"

	"renop/internal/core"
)

var legacyAPITokenScopes = []string{
	core.APITokenScopeRepositoryRead,
	core.APITokenScopeRepositoryPublish,
}

func validAPITokenHash(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func normalizeStoredAPIToken(token *core.APIToken) error {
	if token == nil || uuid.Validate(token.ID) != nil || token.CreatedAt <= 0 {
		return errors.New("API token metadata is invalid")
	}
	token.Name = strings.TrimSpace(SanitizeInputString(token.Name, core.MaxAPITokenNameLength))
	if token.Name == "" || strings.HasPrefix(strings.ToLower(token.Name),
		strings.ToLower(core.LegacyAPITokenNamePrefix)) {
		return errors.New("API token name is required")
	}
	if len(token.Scopes) == 0 || len(token.Scopes) > core.MaxAPITokenScopes {
		return errors.New("API token scopes are invalid")
	}
	slices.Sort(token.Scopes)
	token.Scopes = slices.Compact(token.Scopes)
	for _, scope := range token.Scopes {
		if scope == "" || len(scope) > 64 {
			return errors.New("API token scope is invalid")
		}
	}
	selectedScopes := make(map[string]struct{}, len(token.Scopes))
	for _, scope := range token.Scopes {
		selectedScopes[scope] = struct{}{}
	}
	targetCount := 0
	for scope, targets := range token.Targets {
		if _, ok := selectedScopes[scope]; !ok || len(targets) == 0 {
			return errors.New("API token targets are invalid")
		}
		for index := range targets {
			targets[index] = strings.TrimSpace(targets[index])
			if targets[index] == "" || len(targets[index]) > core.MaxAPITokenTargetLength {
				return errors.New("API token target is invalid")
			}
		}
		slices.Sort(targets)
		targets = slices.Compact(targets)
		targetCount += len(targets)
		token.Targets[scope] = targets
	}
	if targetCount > core.MaxAPITokenTargets {
		return errors.New("API token target limit exceeded")
	}
	if token.ExpiresAt != nil && *token.ExpiresAt <= token.CreatedAt {
		return errors.New("API token expiration is invalid")
	}
	return nil
}

type storedAPITokenAuthorization struct {
	Scopes  []string            `json:"scopes"`
	Targets map[string][]string `json:"targets,omitempty"`
}

func decodeStoredAPITokenAuthorization(value string, token *core.APIToken) error {
	if token == nil {
		return errors.New("API token metadata is unavailable")
	}
	if strings.HasPrefix(strings.TrimSpace(value), "[") {
		return json.Unmarshal([]byte(value), &token.Scopes)
	}
	var authorization storedAPITokenAuthorization
	if err := json.Unmarshal([]byte(value), &authorization); err != nil {
		return err
	}
	token.Scopes = authorization.Scopes
	token.Targets = authorization.Targets
	return nil
}

func encodeStoredAPITokenAuthorization(token *core.APIToken) ([]byte, error) {
	if len(token.Targets) == 0 {
		return json.Marshal(token.Scopes)
	}
	return json.Marshal(storedAPITokenAuthorization{Scopes: token.Scopes, Targets: token.Targets})
}

func scanAPIToken(scanner interface{ Scan(...any) error }) (*core.APIToken, error) {
	token := &core.APIToken{}
	var scopesJSON string
	var expiresAt sql.NullInt64
	if err := scanner.Scan(&token.ID, &token.Name, &scopesJSON, &token.CreatedAt, &expiresAt); err != nil {
		return nil, err
	}
	if err := decodeStoredAPITokenAuthorization(scopesJSON, token); err != nil {
		return nil, fmt.Errorf("decode API token scopes: %w", err)
	}
	if expiresAt.Valid {
		value := expiresAt.Int64
		token.ExpiresAt = &value
	}
	return token, nil
}

// ListAPITokens returns non-secret token metadata for an account.
func (db *DB) ListAPITokens(username string) ([]*core.APIToken, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	rows, err := db.Query(`SELECT api.id, api.name, api.scopes_json, api.created_at, api.expires_at
		FROM user_api_tokens api JOIN user_profiles profile ON profile.user_id = api.user_id
		WHERE profile.username = ? ORDER BY api.created_at DESC, api.id`, username)
	if err != nil {
		return nil, fmt.Errorf("list API tokens for %s: %w", username, err)
	}
	defer rows.Close()
	tokens := make([]*core.APIToken, 0)
	for rows.Next() {
		token, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate API tokens for %s: %w", username, err)
	}
	return tokens, nil
}

// CreateAPIToken stores one pre-hashed high-entropy credential.
func (db *DB) CreateAPIToken(username string, token *core.APIToken, secretHash string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	if err := normalizeStoredAPIToken(token); err != nil {
		return err
	}
	if !validAPITokenHash(secretHash) {
		return errors.New("API token secret hash is invalid")
	}
	userID, err := db.userIDForExistingAccount(username)
	if err != nil {
		return err
	}
	scopesJSON, err := encodeStoredAPITokenAuthorization(token)
	if err != nil {
		return fmt.Errorf("encode API token scopes: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin API token creation: %w", err)
	}
	defer tx.Rollback()
	if err := lockAccountLoginMethodsTx(tx, userID); err != nil {
		return fmt.Errorf("lock account before API token creation: %w", err)
	}
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM user_api_tokens WHERE user_id = ?`, userID).Scan(&count); err != nil {
		return fmt.Errorf("count API tokens: %w", err)
	}
	if count >= core.MaxAPITokensPerUser {
		return core.ErrAPITokenLimit
	}
	var duplicateName int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM user_api_tokens WHERE user_id = ? AND LOWER(name) = LOWER(?)`,
		userID, token.Name).Scan(&duplicateName); err != nil {
		return fmt.Errorf("inspect API token name: %w", err)
	}
	if duplicateName > 0 {
		return core.ErrAPITokenNameExists
	}
	var expiresAt sql.NullInt64
	if token.ExpiresAt != nil {
		expiresAt = sql.NullInt64{Int64: *token.ExpiresAt, Valid: true}
	}
	if _, err := tx.Exec(`INSERT INTO user_api_tokens
		(id, user_id, name, secret_hash, scopes_json, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, token.ID, userID, token.Name, secretHash,
		string(scopesJSON), token.CreatedAt, expiresAt); err != nil {
		if uniqueConstraintError(err) {
			return core.ErrAPITokenNameExists
		}
		return fmt.Errorf("create API token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit API token creation: %w", err)
	}
	return nil
}

// DeleteAPIToken revokes one token owned by username.
func (db *DB) DeleteAPIToken(username, tokenID string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	if uuid.Validate(tokenID) != nil {
		return core.ErrAPITokenNotFound
	}
	result, err := db.Exec(`DELETE FROM user_api_tokens WHERE id = ? AND user_id = (
		SELECT user_id FROM user_profiles WHERE username = ?)`, tokenID, username)
	if err != nil {
		return fmt.Errorf("delete API token: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted API tokens: %w", err)
	}
	if affected != 1 {
		return core.ErrAPITokenNotFound
	}
	return nil
}

// GetAPITokenByHash resolves one credential hash and its owning account.
func (db *DB) GetAPITokenByHash(secretHash, username string) (*core.APITokenCredential, error) {
	if !validAPITokenHash(secretHash) {
		return nil, nil
	}
	username = strings.ToLower(strings.TrimSpace(username))
	query := `SELECT api.id, api.name, api.scopes_json, api.created_at, api.expires_at,
		account.name, account.type, account.type_value, account.encrypted_secret,
		account.password_hash, account.tokens_json, account.created_at, account.description,
		account.expires_at, account.permissions_json
		FROM user_api_tokens api
		JOIN user_profiles profile ON profile.user_id = api.user_id
		JOIN tokens account ON account.name = profile.username
		WHERE api.secret_hash = ?`
	arguments := []any{secretHash}
	if username != "" {
		query += ` AND profile.username = ?`
		arguments = append(arguments, username)
	}
	row := db.QueryRow(query, arguments...)
	token := &core.APIToken{}
	var scopesJSON string
	var tokenExpiresAt, accountExpiresAt sql.NullInt64
	var accountName, accountType, encryptedSecret, passwordHash, tokensJSON, createdAt, description, permissionsJSON string
	var typeValue int32
	if err := row.Scan(&token.ID, &token.Name, &scopesJSON, &token.CreatedAt, &tokenExpiresAt,
		&accountName, &accountType, &typeValue, &encryptedSecret, &passwordHash, &tokensJSON,
		&createdAt, &description, &accountExpiresAt, &permissionsJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve API token credential: %w", err)
	}
	if err := decodeStoredAPITokenAuthorization(scopesJSON, token); err != nil {
		return nil, fmt.Errorf("decode API token credential scopes: %w", err)
	}
	if tokenExpiresAt.Valid {
		value := tokenExpiresAt.Int64
		token.ExpiresAt = &value
	}
	account, err := parseTokenRow(accountName, accountType, typeValue, encryptedSecret, passwordHash,
		tokensJSON, createdAt, description, accountExpiresAt, permissionsJSON)
	if err != nil {
		return nil, err
	}
	return &core.APITokenCredential{Token: token, Account: account}, nil
}

// CountAPITokens returns the number of durable API credentials owned by username.
func (db *DB) CountAPITokens(username string) (int, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM user_api_tokens api
		JOIN user_profiles profile ON profile.user_id = api.user_id
		WHERE profile.username = ?`, username).Scan(&count)
	return count, err
}

// CountAPITokensByUsername returns token counts for administrator list rendering.
func (db *DB) CountAPITokensByUsername() (map[string]int, error) {
	rows, err := db.Query(`SELECT profile.username, COUNT(api.id)
		FROM user_profiles profile LEFT JOIN user_api_tokens api ON api.user_id = profile.user_id
		GROUP BY profile.username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var username string
		var count int
		if err := rows.Scan(&username, &count); err != nil {
			return nil, err
		}
		counts[username] = count
	}
	return counts, rows.Err()
}

func (db *DB) migrateLegacyAPITokens() error {
	rows, err := db.Query(`SELECT profile.user_id, account.name, account.tokens_json
		FROM tokens account JOIN user_profiles profile ON profile.username = account.name
		WHERE account.tokens_json <> '' AND account.tokens_json <> '[]'`)
	if err != nil {
		return err
	}
	type legacyAccount struct {
		userID   string
		username string
		secrets  []string
	}
	accounts := make([]legacyAccount, 0)
	for rows.Next() {
		var account legacyAccount
		var tokensJSON string
		if err := rows.Scan(&account.userID, &account.username, &tokensJSON); err != nil {
			_ = rows.Close()
			return err
		}
		if err := json.Unmarshal([]byte(tokensJSON), &account.secrets); err != nil {
			_ = rows.Close()
			return fmt.Errorf("decode legacy API tokens for %s: %w", account.username, err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, account := range accounts {
		if err := db.migrateLegacyAPITokenAccount(account.userID, account.username,
			account.secrets); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) migrateLegacyAPITokenAccount(userID, username string, secrets []string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := migrateLegacySecretsTx(tx, userID, username, secrets, time.Now().UnixMilli()); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE tokens SET tokens_json = '[]' WHERE name = ?`, username); err != nil {
		return fmt.Errorf("clear legacy API tokens for %s: %w", username, err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func migrateLegacySecretsTx(tx *Tx, userID, username string, secrets []string, createdAt int64) error {
	scopesJSON, err := json.Marshal(legacyAPITokenScopes)
	if err != nil {
		return err
	}
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		digest := sha256.Sum256([]byte(secret))
		hash := hex.EncodeToString(digest[:])
		tokenID := uuid.NewString()
		name := core.LegacyAPITokenNamePrefix + tokenID
		if _, err := tx.Exec(`INSERT INTO user_api_tokens
			(id, user_id, name, secret_hash, scopes_json, created_at, expires_at)
			SELECT ?, ?, ?, ?, ?, ?, NULL WHERE NOT EXISTS (
				SELECT 1 FROM user_api_tokens WHERE secret_hash = ?
			)`, tokenID, userID, name, hash, string(scopesJSON), createdAt, hash); err != nil {
			return fmt.Errorf("migrate legacy API token for %s: %w", username, err)
		}
	}
	return nil
}
