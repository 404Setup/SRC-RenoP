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
	"time"

	"github.com/3JoB/unsafeConvert"
	"github.com/goccy/go-json"

	"renop/internal/core"
)

const (
	maxTokenNameLen   = 255
	maxTokenSecretLen = 1024
)

func parseTokenRow(name, tokenType string, typeValue int32, encryptedSecret, passwordHash, tokensJSON, createdAt, description string, expiresAt sql.NullInt64, permissionsJSON string) *core.AccessToken {
	var tokList []string
	if tokensJSON != "" {
		_ = json.Unmarshal(unsafeConvert.ByteSlice(tokensJSON), &tokList)
	}
	if tokList == nil {
		tokList = []string{}
	}

	var permList []string
	if permissionsJSON != "" {
		_ = json.Unmarshal(unsafeConvert.ByteSlice(permissionsJSON), &permList)
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
	if db == nil || db.SQLDB == nil || name == "" {
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

	var tokenName, tokenType, encryptedSecret, passwordHash, tokensJSON, createdAt, description, permissionsJSON string
	var typeValue int32
	var expiresAt sql.NullInt64

	err := row.Scan(&tokenName, &tokenType, &typeValue, &encryptedSecret, &passwordHash, &tokensJSON, &createdAt, &description, &expiresAt, &permissionsJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			db.tokenCache.Set(lowerName, nil, 30*time.Second)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query token by name (%s): %w", lowerName, err)
	}

	tok := parseTokenRow(tokenName, tokenType, typeValue, encryptedSecret, passwordHash, tokensJSON, createdAt, description, expiresAt, permissionsJSON)
	db.tokenCache.Set(lowerName, tok, 10*time.Minute)
	for _, t := range tok.Tokens {
		db.tokenSecretCache.Set(t, tok, 10*time.Minute)
	}

	return tok, nil
}

func (db *DB) GetTokenBySecret(secret string) (*core.AccessToken, error) {
	if db == nil || db.SQLDB == nil || secret == "" {
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
		var tokenName, tokenType, encryptedSecret, passwordHash, tokensJSON, createdAt, description, permissionsJSON string
		var typeValue int32
		var expiresAt sql.NullInt64

		if err := rows.Scan(&tokenName, &tokenType, &typeValue, &encryptedSecret, &passwordHash, &tokensJSON, &createdAt, &description, &expiresAt, &permissionsJSON); err != nil {
			return nil, fmt.Errorf("failed to scan token: %w", err)
		}

		tok := parseTokenRow(tokenName, tokenType, typeValue, encryptedSecret, passwordHash, tokensJSON, createdAt, description, expiresAt, permissionsJSON)
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
	if db == nil || db.SQLDB == nil || token == nil {
		return nil
	}
	token.Name = SanitizeInputString(strings.TrimSpace(token.Name), maxTokenNameLen)
	token.Description = SanitizeInputString(token.Description, 2048)
	if token.Name == "" {
		return nil
	}

	token.Name = strings.ToLower(token.Name)
	name := token.Name
	tokensJSON, _ := json.Marshal(token.Tokens)
	permissionsJSON, _ := json.Marshal(token.Permissions)

	var expiresAt sql.NullInt64
	if token.ExpiresAt != nil {
		expiresAt = sql.NullInt64{Int64: *token.ExpiresAt, Valid: true}
	}

	query := db.Dialect.UpsertTokenQuery()
	_, err := db.Exec(query, name, string(token.Identifier.Type), token.Identifier.Value, token.EncryptedSecret, token.PasswordHash, string(tokensJSON), token.CreatedAt, token.Description, expiresAt, string(permissionsJSON))
	if err != nil {
		return fmt.Errorf("failed to save token (%s): %w", name, err)
	}
	if _, err := db.ensureUserProfile(name); err != nil {
		return err
	}

	db.finishTokenUpdate(name, token)

	return nil
}

func (db *DB) DeleteToken(name string) error {
	if db == nil || db.SQLDB == nil || name == "" {
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
	var userID string
	if err := tx.QueryRow(`SELECT user_id FROM user_profiles WHERE username = ?`, lowerName).Scan(&userID); err != nil {
		return fmt.Errorf("failed to resolve stable identity for token (%s): %w", lowerName, err)
	}
	var soleMavenOwnerships int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM maven_domain_members current_member
		WHERE current_member.user_id = ? AND current_member.permission_level = ? AND NOT EXISTS (
			SELECT 1 FROM maven_domain_members other_member
			WHERE other_member.repository = current_member.repository AND other_member.domain = current_member.domain
			AND other_member.permission_level = ? AND other_member.user_id <> current_member.user_id
		)`, userID, core.MavenPermissionOwner, core.MavenPermissionOwner).Scan(&soleMavenOwnerships); err != nil {
		return fmt.Errorf("failed to inspect Maven domain ownership for token (%s): %w", lowerName, err)
	}
	if soleMavenOwnerships > 0 {
		return fmt.Errorf("cannot delete token %s: user is the last L4 member of %d Maven domain(s)", lowerName, soleMavenOwnerships)
	}
	if err := cancelMavenInvitations(tx, `recipient = ? OR inviter = ?`, []any{lowerName, lowerName}, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("failed to cancel Maven invitations for token (%s): %w", lowerName, err)
	}
	if _, err := tx.Exec(`DELETE FROM maven_domain_members WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("failed to delete Maven memberships for token (%s): %w", lowerName, err)
	}
	var soleCargoOwnerships int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cargo_members current_member
		WHERE current_member.user_id = ? AND current_member.permission_level = ? AND NOT EXISTS (
			SELECT 1 FROM cargo_members other_member
			WHERE other_member.repository = current_member.repository
			AND other_member.normalized_name = current_member.normalized_name
			AND other_member.permission_level = ? AND other_member.user_id <> current_member.user_id
		)`, userID, core.CargoPermissionOwner, core.CargoPermissionOwner).Scan(&soleCargoOwnerships); err != nil {
		return fmt.Errorf("failed to inspect Cargo package ownership for token (%s): %w", lowerName, err)
	}
	if soleCargoOwnerships > 0 {
		return fmt.Errorf("cannot delete token %s: user is the last L4 member of %d Cargo package(s)", lowerName, soleCargoOwnerships)
	}
	if err := cancelCargoInvitations(tx, `recipient = ? OR inviter = ?`, []any{lowerName, lowerName}, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("failed to cancel Cargo invitations for token (%s): %w", lowerName, err)
	}
	if _, err := tx.Exec(`DELETE FROM cargo_members WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("failed to delete Cargo memberships for token (%s): %w", lowerName, err)
	}

	var soleDockerOwnerships int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM docker_members current_member
		WHERE current_member.user_id = ? AND current_member.permission_level = ? AND NOT EXISTS (
			SELECT 1 FROM docker_members other_member
			WHERE other_member.repository = current_member.repository
			AND other_member.image_name = current_member.image_name
			AND other_member.permission_level = ? AND other_member.user_id <> current_member.user_id
		)`, userID, core.DockerPermissionOwner, core.DockerPermissionOwner).Scan(&soleDockerOwnerships); err != nil {
		return fmt.Errorf("failed to inspect Docker image ownership for token (%s): %w", lowerName, err)
	}
	if soleDockerOwnerships > 0 {
		return fmt.Errorf("cannot delete token %s: user is the last L4 member of %d Docker image(s)", lowerName, soleDockerOwnerships)
	}
	if err := cancelDockerInvitations(tx, `recipient = ? OR inviter = ?`, []any{lowerName, lowerName}, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("failed to cancel Docker invitations for token (%s): %w", lowerName, err)
	}
	if _, err := tx.Exec(`DELETE FROM docker_members WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("failed to delete Docker memberships for token (%s): %w", lowerName, err)
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
	if _, err := tx.Exec(`DELETE FROM user_profiles WHERE username = ?`, lowerName); err != nil {
		return fmt.Errorf("failed to delete user profile for token (%s): %w", lowerName, err)
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
	if db == nil || db.SQLDB == nil || token == nil {
		return core.ErrDatabaseUnavailable
	}
	lowerOld := strings.ToLower(SanitizeInputString(strings.TrimSpace(oldName), maxTokenNameLen))
	lowerNew := strings.ToLower(SanitizeInputString(strings.TrimSpace(newName), maxTokenNameLen))
	if lowerOld == "" || lowerNew == "" {
		return errors.New("token name is invalid")
	}
	if lowerOld == lowerNew {
		token.Name = lowerNew
		return db.SaveToken(token)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin token rename: %w", err)
	}
	defer tx.Rollback()
	if err := db.renameTokenInTx(tx, lowerOld, lowerNew, token); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE user_profiles SET username = ?, updated_at = ? WHERE username = ?`,
		lowerNew, time.Now().UnixMilli(), lowerOld); err != nil {
		return fmt.Errorf("rename user profile from %s to %s: %w", lowerOld, lowerNew, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit token rename: %w", err)
	}
	db.finishTokenRename(lowerOld, lowerNew, token)
	return nil
}

func (db *DB) renameTokenInTx(tx *Tx, oldName, newName string, token *core.AccessToken) error {
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM tokens WHERE name = ?`, newName).Scan(&exists); err == nil {
		return core.ErrUsernameAlreadyExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect target username %s: %w", newName, err)
	}
	tokensJSON, err := json.Marshal(token.Tokens)
	if err != nil {
		return fmt.Errorf("encode token secrets: %w", err)
	}
	permissionsJSON, err := json.Marshal(token.Permissions)
	if err != nil {
		return fmt.Errorf("encode token permissions: %w", err)
	}
	var expiresAt sql.NullInt64
	if token.ExpiresAt != nil {
		expiresAt = sql.NullInt64{Int64: *token.ExpiresAt, Valid: true}
	}
	if _, err := tx.Exec(`INSERT INTO tokens
		(name, type, type_value, encrypted_secret, password_hash, tokens_json, created_at, description, expires_at, permissions_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		newName, string(token.Identifier.Type), token.Identifier.Value, token.EncryptedSecret, token.PasswordHash,
		string(tokensJSON), token.CreatedAt, token.Description, expiresAt, string(permissionsJSON)); err != nil {
		return fmt.Errorf("create renamed token %s: %w", newName, err)
	}
	if _, err := tx.Exec(`DELETE FROM tokens WHERE name = ?`, oldName); err != nil {
		return fmt.Errorf("remove previous token %s: %w", oldName, err)
	}

	updates := []struct {
		query string
		name  string
	}{
		{`UPDATE sessions SET username = ? WHERE username = ?`, "sessions"},
		{`UPDATE fido_devices SET username = ? WHERE username = ?`, "FIDO devices"},
		{`UPDATE user_gpg_keys SET username = ? WHERE username = ?`, "GPG keys"},
		{`UPDATE gpg_signatures SET uploader = ? WHERE uploader = ?`, "GPG signatures"},
		{`UPDATE gpg_releases SET uploader = ? WHERE uploader = ?`, "GPG releases"},
		{`UPDATE audit_logs SET username = ? WHERE username = ?`, "audit subjects"},
		{`UPDATE audit_logs SET operator = ? WHERE operator = ?`, "audit operators"},
		{`UPDATE user_messages SET recipient = ? WHERE recipient = ?`, "message recipients"},
		{`UPDATE user_messages SET sender = ? WHERE sender = ?`, "message senders"},
		{`UPDATE maven_domain_members SET username = ? WHERE username = ?`, "Maven memberships"},
		{`UPDATE maven_artifacts SET publisher = ? WHERE publisher = ?`, "Maven artifact publishers"},
		{`UPDATE maven_versions SET publisher = ? WHERE publisher = ?`, "Maven version publishers"},
		{`UPDATE maven_domain_invitations SET inviter = ? WHERE inviter = ?`, "Maven invitation senders"},
		{`UPDATE maven_domain_invitations SET recipient = ? WHERE recipient = ?`, "Maven invitation recipients"},
		{`UPDATE cargo_members SET username = ? WHERE username = ?`, "Cargo memberships"},
		{`UPDATE cargo_versions SET publisher = ? WHERE publisher = ?`, "Cargo publishers"},
		{`UPDATE cargo_invitations SET inviter = ? WHERE inviter = ?`, "Cargo invitation senders"},
		{`UPDATE cargo_invitations SET recipient = ? WHERE recipient = ?`, "Cargo invitation recipients"},
		{`UPDATE docker_members SET username = ? WHERE username = ?`, "Docker memberships"},
		{`UPDATE docker_images SET publisher = ? WHERE publisher = ?`, "Docker image publishers"},
		{`UPDATE docker_tags SET publisher = ? WHERE publisher = ?`, "Docker tag publishers"},
		{`UPDATE docker_manifests SET publisher = ? WHERE publisher = ?`, "Docker manifest publishers"},
		{`UPDATE docker_invitations SET inviter = ? WHERE inviter = ?`, "Docker invitation senders"},
		{`UPDATE docker_invitations SET recipient = ? WHERE recipient = ?`, "Docker invitation recipients"},
	}
	for _, update := range updates {
		if _, err := tx.Exec(update.query, newName, oldName); err != nil {
			return fmt.Errorf("rename %s from %s to %s: %w", update.name, oldName, newName, err)
		}
	}
	return nil
}

func (db *DB) saveTokenInTx(tx *Tx, name string, token *core.AccessToken) error {
	tokensJSON, err := json.Marshal(token.Tokens)
	if err != nil {
		return fmt.Errorf("encode token secrets: %w", err)
	}
	permissionsJSON, err := json.Marshal(token.Permissions)
	if err != nil {
		return fmt.Errorf("encode token permissions: %w", err)
	}
	var expiresAt sql.NullInt64
	if token.ExpiresAt != nil {
		expiresAt = sql.NullInt64{Int64: *token.ExpiresAt, Valid: true}
	}
	if _, err := tx.Exec(db.Dialect.UpsertTokenQuery(), name, string(token.Identifier.Type), token.Identifier.Value,
		token.EncryptedSecret, token.PasswordHash, string(tokensJSON), token.CreatedAt, token.Description,
		expiresAt, string(permissionsJSON)); err != nil {
		return fmt.Errorf("update token %s with profile: %w", name, err)
	}
	return nil
}

func (db *DB) finishTokenRename(oldName, newName string, token *core.AccessToken) {
	token.Name = newName
	db.tokenCache.Delete(oldName)
	db.tokenSecretCache.DeleteFunc(func(_ string, val *core.AccessToken) bool {
		return val == nil || strings.EqualFold(val.Name, oldName)
	})
	db.sessionCache.DeleteFunc(func(_ string, session *core.Session) bool {
		return session == nil || strings.EqualFold(session.Username, oldName)
	})
	db.tokenCache.Set(newName, token, 10*time.Minute)
	for _, secret := range token.Tokens {
		db.tokenSecretCache.Set(secret, token, 10*time.Minute)
	}
}

func (db *DB) finishTokenUpdate(name string, token *core.AccessToken) {
	token.Name = name
	db.tokenCache.Set(name, token, 10*time.Minute)
	db.tokenSecretCache.DeleteFunc(func(_ string, value *core.AccessToken) bool {
		return value == nil || strings.EqualFold(value.Name, name)
	})
	for _, secret := range token.Tokens {
		db.tokenSecretCache.Set(secret, token, 10*time.Minute)
	}
}

func (db *DB) CountTokens() (uint64, error) {
	if db == nil || db.SQLDB == nil {
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
	if db == nil || db.SQLDB == nil {
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
		var tokenName, tokenType, encryptedSecret, passwordHash, tokensJSON, createdAt, description, permissionsJSON string
		var typeValue int32
		var expiresAt sql.NullInt64

		if err := rows.Scan(&tokenName, &tokenType, &typeValue, &encryptedSecret, &passwordHash, &tokensJSON, &createdAt, &description, &expiresAt, &permissionsJSON); err != nil {
			return nil, fmt.Errorf("failed to scan token: %w", err)
		}

		tok := parseTokenRow(tokenName, tokenType, typeValue, encryptedSecret, passwordHash, tokensJSON, createdAt, description, expiresAt, permissionsJSON)
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
	if db == nil || db.SQLDB == nil {
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
