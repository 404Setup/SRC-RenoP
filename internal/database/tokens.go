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
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/3JoB/unsafeConvert"
	"github.com/goccy/go-json"

	"renop/internal/core"
)

const (
	maxTokenNameLen   = 255
	maxTokenSecretLen = 1024
)

func parseTokenRow(name, tokenType string, typeValue int32, encryptedSecret, passwordHash, tokensJson, createdAt, description string, expiresAt sql.NullInt64, permissionsJson string) *core.AccessToken {
	var tokList []string
	if tokensJson != "" {
		_ = json.Unmarshal(unsafeConvert.ByteSlice(tokensJson), &tokList)
	}
	if tokList == nil {
		tokList = []string{}
	}

	var permList []string
	if permissionsJson != "" {
		_ = json.Unmarshal(unsafeConvert.ByteSlice(permissionsJson), &permList)
	}
	if permList == nil {
		permList = []string{}
	}

	var exp *int64
	if expiresAt.Valid {
		v := expiresAt.Int64
		exp = &v
	}

	return &core.AccessToken{
		Identifier: core.AccessTokenIdentifier{
			Type:  core.AccessTokenType(tokenType),
			Value: typeValue,
		},
		Name:            strings.ToLower(name),
		EncryptedSecret: encryptedSecret,
		PasswordHash:    passwordHash,
		Tokens:          tokList,
		CreatedAt:       createdAt,
		Description:     description,
		ExpiresAt:       exp,
		Permissions:     permList,
	}
}

func (db *DB) GetTokenByName(name string) (*core.AccessToken, error) {
	if db == nil || db.SqlDB == nil || name == "" {
		return nil, nil
	}
	name = SanitizeInputString(strings.TrimSpace(name), maxTokenNameLen)
	if name == "" {
		return nil, nil
	}

	lowerName := strings.ToLower(name)
	if tok, ok := db.tokenCache.Get(lowerName); ok {
		return tok, nil
	}

	query := `SELECT name, type, type_value, encrypted_secret, password_hash, tokens_json, created_at, description, expires_at, permissions_json FROM tokens WHERE name = ?`
	row := db.QueryRow(query, lowerName)

	var tokenName, tokenType, encryptedSecret, passwordHash, tokensJson, createdAt, description, permissionsJson string
	var typeValue int32
	var expiresAt sql.NullInt64

	err := row.Scan(&tokenName, &tokenType, &typeValue, &encryptedSecret, &passwordHash, &tokensJson, &createdAt, &description, &expiresAt, &permissionsJson)
	if err != nil {
		if err == sql.ErrNoRows {
			db.tokenCache.Set(lowerName, nil, 30*time.Second)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query token by name (%s): %w", lowerName, err)
	}

	tok := parseTokenRow(tokenName, tokenType, typeValue, encryptedSecret, passwordHash, tokensJson, createdAt, description, expiresAt, permissionsJson)
	db.tokenCache.Set(lowerName, tok, 10*time.Minute)
	for _, t := range tok.Tokens {
		db.tokenSecretCache.Set(t, tok, 10*time.Minute)
	}

	return tok, nil
}

func (db *DB) GetTokenBySecret(secret string) (*core.AccessToken, error) {
	if db == nil || db.SqlDB == nil || secret == "" {
		return nil, nil
	}
	secret = SanitizeInputString(secret, maxTokenSecretLen)
	if secret == "" {
		return nil, nil
	}

	if tok, ok := db.tokenSecretCache.Get(secret); ok {
		return tok, nil
	}

	escapedSecret := escapeJSONLikeSecret(secret)
	query := `SELECT name, type, type_value, encrypted_secret, password_hash, tokens_json, created_at, description, expires_at, permissions_json FROM tokens WHERE tokens_json LIKE ? ESCAPE '\'`
	rows, err := db.Query(query, "%\""+escapedSecret+"\"%")
	if err != nil {
		return nil, fmt.Errorf("failed to query token by secret: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tokenName, tokenType, encryptedSecret, passwordHash, tokensJson, createdAt, description, permissionsJson string
		var typeValue int32
		var expiresAt sql.NullInt64

		if err := rows.Scan(&tokenName, &tokenType, &typeValue, &encryptedSecret, &passwordHash, &tokensJson, &createdAt, &description, &expiresAt, &permissionsJson); err != nil {
			return nil, fmt.Errorf("failed to scan token: %w", err)
		}

		tok := parseTokenRow(tokenName, tokenType, typeValue, encryptedSecret, passwordHash, tokensJson, createdAt, description, expiresAt, permissionsJson)
		if slices.Contains(tok.Tokens, secret) {
			db.tokenCache.Set(tok.Name, tok, 10*time.Minute)
			db.tokenSecretCache.Set(secret, tok, 10*time.Minute)
			return tok, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate tokens by secret: %w", err)
	}

	db.tokenSecretCache.Set(secret, nil, 30*time.Second)
	return nil, nil
}

func (db *DB) SaveToken(token *core.AccessToken) error {
	if db == nil || db.SqlDB == nil || token == nil {
		return nil
	}
	token.Name = SanitizeInputString(strings.TrimSpace(token.Name), maxTokenNameLen)
	token.Description = SanitizeInputString(token.Description, 2048)
	if token.Name == "" {
		return nil
	}

	token.Name = strings.ToLower(token.Name)
	name := token.Name
	tokensJson, _ := json.Marshal(token.Tokens)
	permissionsJson, _ := json.Marshal(token.Permissions)

	var expiresAt sql.NullInt64
	if token.ExpiresAt != nil {
		expiresAt = sql.NullInt64{Int64: *token.ExpiresAt, Valid: true}
	}

	query := db.Dialect.UpsertTokenQuery()
	_, err := db.Exec(query, name, string(token.Identifier.Type), token.Identifier.Value, token.EncryptedSecret, token.PasswordHash, string(tokensJson), token.CreatedAt, token.Description, expiresAt, string(permissionsJson))
	if err != nil {
		return fmt.Errorf("failed to save token (%s): %w", name, err)
	}

	db.tokenCache.Set(name, token, 10*time.Minute)
	db.tokenSecretCache.DeleteFunc(func(_ string, val *core.AccessToken) bool {
		return val == nil || strings.EqualFold(val.Name, name)
	})
	for _, t := range token.Tokens {
		db.tokenSecretCache.Set(t, token, 10*time.Minute)
	}

	return nil
}

func (db *DB) DeleteToken(name string) error {
	if db == nil || db.SqlDB == nil || name == "" {
		return nil
	}
	name = SanitizeInputString(strings.TrimSpace(name), maxTokenNameLen)
	if name == "" {
		return nil
	}

	lowerName := strings.ToLower(name)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin token deletion (%s): %w", lowerName, err)
	}
	defer tx.Rollback()
	var soleCargoOwnerships int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cargo_members current_member
		WHERE current_member.username = ? AND current_member.permission_level = ? AND NOT EXISTS (
			SELECT 1 FROM cargo_members other_member
			WHERE other_member.repository = current_member.repository
			AND other_member.normalized_name = current_member.normalized_name
			AND other_member.permission_level = ? AND other_member.username <> current_member.username
		)`, lowerName, core.CargoPermissionFull, core.CargoPermissionFull).Scan(&soleCargoOwnerships); err != nil {
		return fmt.Errorf("failed to inspect Cargo package ownership for token (%s): %w", lowerName, err)
	}
	if soleCargoOwnerships > 0 {
		return fmt.Errorf("cannot delete token %s: user is the last L3 member of %d Cargo package(s)", lowerName, soleCargoOwnerships)
	}
	if err := cancelCargoInvitations(tx, `recipient = ? OR inviter = ?`, []any{lowerName, lowerName}, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("failed to cancel Cargo invitations for token (%s): %w", lowerName, err)
	}
	if _, err := tx.Exec(`DELETE FROM cargo_members WHERE username = ?`, lowerName); err != nil {
		return fmt.Errorf("failed to delete Cargo memberships for token (%s): %w", lowerName, err)
	}

	if _, err := tx.Exec(`DELETE FROM fido_devices WHERE username = ?`, lowerName); err != nil {
		return fmt.Errorf("failed to delete fido devices for token (%s): %w", lowerName, err)
	}
	if _, err := tx.Exec(`DELETE FROM user_gpg_keys WHERE username = ?`, lowerName); err != nil {
		return fmt.Errorf("failed to delete GPG keys for token (%s): %w", lowerName, err)
	}
	if _, err := tx.Exec(`DELETE FROM gpg_releases WHERE uploader = ? AND active_key IS NULL AND cleanup_pending = 0`, lowerName); err != nil {
		return fmt.Errorf("failed to delete completed GPG releases for token (%s): %w", lowerName, err)
	}
	if _, err := tx.Exec(`UPDATE gpg_releases SET status = ?, failure_reason = ?, cleanup_pending = 1, updated_at = ?
		WHERE uploader = ? AND active_key IS NOT NULL`, core.GPGReleaseFailed, "Uploader account was deleted", time.Now().UnixMilli(), lowerName); err != nil {
		return fmt.Errorf("failed to cancel pending GPG releases for token (%s): %w", lowerName, err)
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE username = ?`, lowerName); err != nil {
		return fmt.Errorf("failed to delete sessions for token (%s): %w", lowerName, err)
	}
	if _, err := tx.Exec(`DELETE FROM user_messages WHERE recipient = ?`, lowerName); err != nil {
		return fmt.Errorf("failed to delete messages for token (%s): %w", lowerName, err)
	}
	if _, err := tx.Exec(`DELETE FROM tokens WHERE name = ?`, lowerName); err != nil {
		return fmt.Errorf("failed to delete token (%s): %w", lowerName, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit token deletion (%s): %w", lowerName, err)
	}

	db.tokenCache.Delete(lowerName)
	db.tokenSecretCache.DeleteFunc(func(_ string, val *core.AccessToken) bool {
		return val == nil || strings.EqualFold(val.Name, lowerName)
	})
	db.sessionCache.DeleteFunc(func(_ string, sess *core.Session) bool {
		return sess == nil || strings.EqualFold(sess.Username, lowerName)
	})

	return nil
}

func (db *DB) RenameToken(oldName, newName string, token *core.AccessToken) error {
	if db == nil || db.SqlDB == nil || oldName == "" || newName == "" {
		return nil
	}
	oldName = SanitizeInputString(strings.TrimSpace(oldName), maxTokenNameLen)
	newName = SanitizeInputString(strings.TrimSpace(newName), maxTokenNameLen)
	if oldName == "" || newName == "" {
		return nil
	}

	lowerOld := strings.ToLower(oldName)
	lowerNew := strings.ToLower(newName)

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM tokens WHERE name = ?`, lowerOld); err != nil {
		return err
	}

	if token != nil {
		tokensJson, _ := json.Marshal(token.Tokens)
		permissionsJson, _ := json.Marshal(token.Permissions)

		var expiresAt sql.NullInt64
		if token.ExpiresAt != nil {
			expiresAt = sql.NullInt64{Int64: *token.ExpiresAt, Valid: true}
		}

		query := db.Dialect.UpsertTokenQuery()
		if _, err := tx.Exec(query, lowerNew, string(token.Identifier.Type), token.Identifier.Value, token.EncryptedSecret, token.PasswordHash, string(tokensJson), token.CreatedAt, token.Description, expiresAt, string(permissionsJson)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE sessions SET username = ? WHERE username = ?`, lowerNew, lowerOld); err != nil {
		return fmt.Errorf("failed to rename sessions from %s to %s: %w", lowerOld, lowerNew, err)
	}
	if _, err := tx.Exec(`UPDATE fido_devices SET username = ? WHERE username = ?`, lowerNew, lowerOld); err != nil {
		return fmt.Errorf("failed to rename fido devices from %s to %s: %w", lowerOld, lowerNew, err)
	}
	if _, err := tx.Exec(`UPDATE user_gpg_keys SET username = ? WHERE username = ?`, lowerNew, lowerOld); err != nil {
		return fmt.Errorf("failed to rename GPG keys from %s to %s: %w", lowerOld, lowerNew, err)
	}
	if _, err := tx.Exec(`UPDATE gpg_signatures SET uploader = ? WHERE uploader = ?`, lowerNew, lowerOld); err != nil {
		return fmt.Errorf("failed to rename GPG signature uploader from %s to %s: %w", lowerOld, lowerNew, err)
	}
	if _, err := tx.Exec(`UPDATE gpg_releases SET uploader = ? WHERE uploader = ?`, lowerNew, lowerOld); err != nil {
		return fmt.Errorf("failed to rename GPG release uploader from %s to %s: %w", lowerOld, lowerNew, err)
	}
	if _, err := tx.Exec(`UPDATE user_messages SET recipient = ? WHERE recipient = ?`, lowerNew, lowerOld); err != nil {
		return fmt.Errorf("failed to rename message recipient from %s to %s: %w", lowerOld, lowerNew, err)
	}
	if _, err := tx.Exec(`UPDATE user_messages SET sender = ? WHERE sender = ?`, lowerNew, lowerOld); err != nil {
		return fmt.Errorf("failed to rename message sender from %s to %s: %w", lowerOld, lowerNew, err)
	}
	if _, err := tx.Exec(`UPDATE cargo_members SET username = ? WHERE username = ?`, lowerNew, lowerOld); err != nil {
		return fmt.Errorf("failed to rename Cargo memberships from %s to %s: %w", lowerOld, lowerNew, err)
	}
	if _, err := tx.Exec(`UPDATE cargo_versions SET publisher = ? WHERE publisher = ?`, lowerNew, lowerOld); err != nil {
		return fmt.Errorf("failed to rename Cargo publishers from %s to %s: %w", lowerOld, lowerNew, err)
	}
	if _, err := tx.Exec(`UPDATE cargo_invitations SET inviter = ? WHERE inviter = ?`, lowerNew, lowerOld); err != nil {
		return fmt.Errorf("failed to rename Cargo invitation senders from %s to %s: %w", lowerOld, lowerNew, err)
	}
	if _, err := tx.Exec(`UPDATE cargo_invitations SET recipient = ? WHERE recipient = ?`, lowerNew, lowerOld); err != nil {
		return fmt.Errorf("failed to rename Cargo invitation recipients from %s to %s: %w", lowerOld, lowerNew, err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	db.tokenCache.Delete(lowerOld)
	db.tokenSecretCache.DeleteFunc(func(_ string, val *core.AccessToken) bool {
		return val == nil || strings.EqualFold(val.Name, lowerOld)
	})
	if token != nil {
		db.tokenCache.Set(lowerNew, token, 10*time.Minute)
		for _, t := range token.Tokens {
			db.tokenSecretCache.Set(t, token, 10*time.Minute)
		}
	}

	return nil
}

func (db *DB) CountTokens() (uint64, error) {
	if db == nil || db.SqlDB == nil {
		return 0, nil
	}

	var count uint64
	err := db.QueryRow(`SELECT COUNT(*) FROM tokens`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count tokens: %w", err)
	}

	return count, nil
}

func (db *DB) GetAllTokens() ([]*core.AccessToken, error) {
	if db == nil || db.SqlDB == nil {
		return nil, nil
	}

	query := `SELECT name, type, type_value, encrypted_secret, password_hash, tokens_json, created_at, description, expires_at, permissions_json FROM tokens`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all tokens: %w", err)
	}
	defer rows.Close()

	tokens := make([]*core.AccessToken, 0, 16)
	for rows.Next() {
		var tokenName, tokenType, encryptedSecret, passwordHash, tokensJson, createdAt, description, permissionsJson string
		var typeValue int32
		var expiresAt sql.NullInt64

		if err := rows.Scan(&tokenName, &tokenType, &typeValue, &encryptedSecret, &passwordHash, &tokensJson, &createdAt, &description, &expiresAt, &permissionsJson); err != nil {
			return nil, fmt.Errorf("failed to scan token: %w", err)
		}

		tok := parseTokenRow(tokenName, tokenType, typeValue, encryptedSecret, passwordHash, tokensJson, createdAt, description, expiresAt, permissionsJson)
		tokens = append(tokens, tok)
		db.tokenCache.Set(tok.Name, tok, 10*time.Minute)
		for _, t := range tok.Tokens {
			db.tokenSecretCache.Set(t, tok, 10*time.Minute)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tokens, nil
}

// SearchTokenNames returns a bounded, index-friendly prefix match without
// loading token secrets or permission data into the autocomplete request path.
func (db *DB) SearchTokenNames(prefix string, limit int, now int64) ([]string, error) {
	if db == nil || db.SqlDB == nil {
		return []string{}, nil
	}
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return []string{}, nil
	}
	if limit < 1 || limit > 20 {
		limit = 8
	}
	escapedPrefix := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(prefix) + "%"
	rows, err := db.Query(`SELECT name FROM tokens
		WHERE name LIKE ? ESCAPE '!' AND (expires_at IS NULL OR expires_at > ?)
		ORDER BY name ASC LIMIT ?`, escapedPrefix, now, limit)
	if err != nil {
		return nil, fmt.Errorf("search token names: %w", err)
	}
	defer rows.Close()
	names := make([]string, 0, limit)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan token name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate token names: %w", err)
	}
	return names, nil
}

func escapeJSONLikeSecret(secret string) string {
	var b strings.Builder
	b.Grow(len(secret)*2 + 8)
	for i := 0; i < len(secret); i++ {
		c := secret[i]
		switch c {
		case '\\':
			b.WriteString(`\\\\`)
		case '"':
			b.WriteString(`\\\"`)
		case '%':
			b.WriteString(`\%`)
		case '_':
			b.WriteString(`\_`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
