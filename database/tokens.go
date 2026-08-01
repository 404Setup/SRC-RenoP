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

	"github.com/goccy/go-json"

	"renop/core"
)

const (
	maxTokenNameLen   = 255
	maxTokenSecretLen = 1024
)

func parseTokenRow(name, tokenType string, typeValue int32, encryptedSecret, passwordHash, tokensJson, createdAt, description string, expiresAt sql.NullInt64, permissionsJson string) *core.AccessToken {
	var tokList []string
	if tokensJson != "" {
		_ = json.Unmarshal([]byte(tokensJson), &tokList)
	}
	if tokList == nil {
		tokList = []string{}
	}

	var permList []string
	if permissionsJson != "" {
		_ = json.Unmarshal([]byte(permissionsJson), &permList)
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
	if len(name) > maxTokenNameLen {
		return nil, nil
	}

	lowerName := strings.ToLower(name)
	if tok, ok := db.tokenCache.Get(lowerName); ok {
		return tok, nil
	}

	query := `SELECT name, type, type_value, encrypted_secret, password_hash, tokens_json, created_at, description, expires_at, permissions_json FROM tokens WHERE name = ?`
	row := db.SqlDB.QueryRow(query, lowerName)

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
	if len(secret) > maxTokenSecretLen {
		return nil, nil
	}

	if tok, ok := db.tokenSecretCache.Get(secret); ok {
		return tok, nil
	}

	escapedSecret := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(secret)
	query := `SELECT name, type, type_value, encrypted_secret, password_hash, tokens_json, created_at, description, expires_at, permissions_json FROM tokens WHERE tokens_json LIKE ? ESCAPE '\'`
	rows, err := db.SqlDB.Query(query, "%"+escapedSecret+"%")
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

	db.tokenSecretCache.Set(secret, nil, 30*time.Second)
	return nil, nil
}

func (db *DB) SaveToken(token *core.AccessToken) error {
	if db == nil || db.SqlDB == nil || token == nil {
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
	_, err := db.SqlDB.Exec(query, name, string(token.Identifier.Type), token.Identifier.Value, token.EncryptedSecret, token.PasswordHash, string(tokensJson), token.CreatedAt, token.Description, expiresAt, string(permissionsJson))
	if err != nil {
		return fmt.Errorf("failed to save token (%s): %w", name, err)
	}

	db.tokenCache.Set(name, token, 10*time.Minute)
	for _, t := range token.Tokens {
		db.tokenSecretCache.Set(t, token, 10*time.Minute)
	}

	return nil
}

func (db *DB) DeleteToken(name string) error {
	if db == nil || db.SqlDB == nil {
		return nil
	}

	lowerName := strings.ToLower(name)
	_, err := db.SqlDB.Exec(`DELETE FROM tokens WHERE name = ?`, lowerName)
	if err != nil {
		return fmt.Errorf("failed to delete token (%s): %w", lowerName, err)
	}

	db.tokenCache.Delete(lowerName)
	db.tokenSecretCache.DeleteFunc(func(_ string, val *core.AccessToken) bool {
		return val == nil || strings.EqualFold(val.Name, lowerName)
	})

	return nil
}

func (db *DB) RenameToken(oldName, newName string, token *core.AccessToken) error {
	if db == nil || db.SqlDB == nil {
		return nil
	}

	lowerOld := strings.ToLower(oldName)
	lowerNew := strings.ToLower(newName)

	tx, err := db.SqlDB.Begin()
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
	err := db.SqlDB.QueryRow(`SELECT COUNT(*) FROM tokens`).Scan(&count)
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
	rows, err := db.SqlDB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*core.AccessToken
	for rows.Next() {
		var tokenName, tokenType, encryptedSecret, passwordHash, tokensJson, createdAt, description, permissionsJson string
		var typeValue int32
		var expiresAt sql.NullInt64

		if err := rows.Scan(&tokenName, &tokenType, &typeValue, &encryptedSecret, &passwordHash, &tokensJson, &createdAt, &description, &expiresAt, &permissionsJson); err != nil {
			return nil, fmt.Errorf("failed to scan token: %w", err)
		}

		tok := parseTokenRow(tokenName, tokenType, typeValue, encryptedSecret, passwordHash, tokensJson, createdAt, description, expiresAt, permissionsJson)
		tokens = append(tokens, tok)
	}

	return tokens, nil
}

func (db *DB) MigrateTokens(tokens map[string]*core.AccessToken) error {
	if db == nil || db.SqlDB == nil || len(tokens) == 0 {
		return nil
	}

	tx, err := db.SqlDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := db.Dialect.UpsertTokenQuery()
	for _, token := range tokens {
		if token == nil {
			continue
		}
		name := strings.ToLower(token.Name)
		tokensJson, _ := json.Marshal(token.Tokens)
		permissionsJson, _ := json.Marshal(token.Permissions)

		var expiresAt sql.NullInt64
		if token.ExpiresAt != nil {
			expiresAt = sql.NullInt64{Int64: *token.ExpiresAt, Valid: true}
		}

		if _, err := tx.Exec(query, name, string(token.Identifier.Type), token.Identifier.Value, token.EncryptedSecret, token.PasswordHash, string(tokensJson), token.CreatedAt, token.Description, expiresAt, string(permissionsJson)); err != nil {
			return err
		}
	}

	return tx.Commit()
}
