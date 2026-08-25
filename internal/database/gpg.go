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
	"fmt"
	"strings"
	"time"

	"renop/internal/core"
)

const gpgPublicKeyColumns = `fingerprint, key_id, primary_identity, public_key, key_created_at, key_expires_at, fetched_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanGPGPublicKey(row rowScanner) (*core.GPGPublicKey, error) {
	key := &core.GPGPublicKey{}
	if err := row.Scan(
		&key.Fingerprint,
		&key.KeyID,
		&key.PrimaryIdentity,
		&key.PublicKey,
		&key.KeyCreatedAt,
		&key.KeyExpiresAt,
		&key.FetchedAt,
	); err != nil {
		return nil, err
	}
	return key, nil
}

func (db *DB) FindGPGPublicKeys(identifier string) ([]*core.GPGPublicKey, error) {
	identifier = strings.ToUpper(SanitizeInputString(strings.TrimSpace(identifier), 64))
	if db == nil || db.SQLDB == nil || identifier == "" {
		return []*core.GPGPublicKey{}, nil
	}
	rows, err := db.Query(`SELECT k.`+strings.ReplaceAll(gpgPublicKeyColumns, ", ", ", k.")+`
		FROM gpg_public_keys k INNER JOIN gpg_key_aliases a ON a.fingerprint = k.fingerprint
		WHERE a.identifier = ? ORDER BY k.fingerprint`, identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to find cached GPG keys: %w", err)
	}
	defer rows.Close()

	keys := make([]*core.GPGPublicKey, 0, 1)
	for rows.Next() {
		key, scanErr := scanGPGPublicKey(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan cached GPG key: %w", scanErr)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate cached GPG keys: %w", err)
	}
	return keys, nil
}

func (db *DB) GetGPGPublicKey(fingerprint string) (*core.GPGPublicKey, error) {
	fingerprint = strings.ToUpper(SanitizeInputString(strings.TrimSpace(fingerprint), 64))
	if db == nil || db.SQLDB == nil || fingerprint == "" {
		return nil, nil
	}
	key, err := scanGPGPublicKey(db.QueryRow(
		`SELECT `+gpgPublicKeyColumns+` FROM gpg_public_keys WHERE fingerprint = ?`,
		fingerprint,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get cached GPG key: %w", err)
	}
	return key, nil
}

func (db *DB) ListUserGPGKeys(username string) ([]*core.UserGPGKey, error) {
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), 255))
	if db == nil || db.SQLDB == nil || username == "" {
		return []*core.UserGPGKey{}, nil
	}
	rows, err := db.Query(`SELECT k.`+strings.ReplaceAll(gpgPublicKeyColumns, ", ", ", k.")+`, u.requested_id, u.added_at
		FROM user_gpg_keys u INNER JOIN gpg_public_keys k ON k.fingerprint = u.fingerprint
		WHERE u.username = ? ORDER BY u.added_at, k.fingerprint`, username)
	if err != nil {
		return nil, fmt.Errorf("failed to list GPG keys for user (%s): %w", username, err)
	}
	defer rows.Close()

	keys := make([]*core.UserGPGKey, 0, 10)
	for rows.Next() {
		key := &core.UserGPGKey{}
		if err := rows.Scan(
			&key.Fingerprint,
			&key.KeyID,
			&key.PrimaryIdentity,
			&key.PublicKey,
			&key.KeyCreatedAt,
			&key.KeyExpiresAt,
			&key.FetchedAt,
			&key.RequestedID,
			&key.AddedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan user GPG key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate user GPG keys: %w", err)
	}
	return keys, nil
}

func (db *DB) upsertGPGPublicKey(tx *Tx, key *core.GPGPublicKey) error {
	query := db.Dialect.UpsertGPGPublicKeyQuery()
	_, err := tx.Exec(query,
		key.Fingerprint,
		key.KeyID,
		key.PrimaryIdentity,
		key.PublicKey,
		key.KeyCreatedAt,
		key.KeyExpiresAt,
		key.FetchedAt,
	)
	return err
}

func replaceGPGKeyAliases(tx *Tx, fingerprint string, aliases []string) error {
	if _, err := tx.Exec(`DELETE FROM gpg_key_aliases WHERE fingerprint = ?`, fingerprint); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(aliases)+1)
	for _, alias := range aliases {
		alias = strings.ToUpper(SanitizeInputString(strings.TrimSpace(alias), 64))
		if alias == "" {
			continue
		}
		if _, exists := seen[alias]; exists {
			continue
		}
		seen[alias] = struct{}{}
		if _, err := tx.Exec(`INSERT INTO gpg_key_aliases (identifier, fingerprint) VALUES (?, ?)`, alias, fingerprint); err != nil {
			return err
		}
	}
	return nil
}

func normalizeGPGPublicKey(key *core.GPGPublicKey) *core.GPGPublicKey {
	if key == nil {
		return nil
	}
	return &core.GPGPublicKey{
		Fingerprint:     strings.ToUpper(SanitizeInputString(strings.TrimSpace(key.Fingerprint), 64)),
		KeyID:           strings.ToUpper(SanitizeInputString(strings.TrimSpace(key.KeyID), 16)),
		PrimaryIdentity: SanitizeInputString(key.PrimaryIdentity, 2048),
		PublicKey:       append([]byte(nil), key.PublicKey...),
		KeyCreatedAt:    key.KeyCreatedAt,
		KeyExpiresAt:    key.KeyExpiresAt,
		FetchedAt:       key.FetchedAt,
	}
}

func (db *DB) RegisterUserGPGKey(username, requestedID string, key *core.GPGPublicKey, aliases []string) error {
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), 255))
	requestedID = strings.ToUpper(SanitizeInputString(strings.TrimSpace(requestedID), 64))
	key = normalizeGPGPublicKey(key)
	if db == nil || db.SQLDB == nil || username == "" || requestedID == "" || key == nil || key.Fingerprint == "" || len(key.PublicKey) == 0 {
		return errors.New("invalid GPG key registration")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin GPG key registration: %w", err)
	}
	defer tx.Rollback()

	if err := db.upsertGPGPublicKey(tx, key); err != nil {
		return fmt.Errorf("failed to cache GPG public key: %w", err)
	}
	if err := replaceGPGKeyAliases(tx, key.Fingerprint, aliases); err != nil {
		return fmt.Errorf("failed to cache GPG key aliases: %w", err)
	}

	var exists int
	err = tx.QueryRow(`SELECT 1 FROM user_gpg_keys WHERE username = ? AND fingerprint = ?`, username, key.Fingerprint).Scan(&exists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to check user GPG key: %w", err)
	}
	if exists == 0 {
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM user_gpg_keys WHERE username = ?`, username).Scan(&count); err != nil {
			return fmt.Errorf("failed to count user GPG keys: %w", err)
		}
		if count >= 10 {
			return core.ErrGPGKeyLimit
		}
		if _, err := tx.Exec(`INSERT INTO user_gpg_keys (username, fingerprint, requested_id, added_at) VALUES (?, ?, ?, ?)`,
			username, key.Fingerprint, requestedID, time.Now().UnixMilli()); err != nil {
			return fmt.Errorf("failed to register user GPG key: %w", err)
		}
	} else if _, err := tx.Exec(`UPDATE user_gpg_keys SET requested_id = ? WHERE username = ? AND fingerprint = ?`,
		requestedID, username, key.Fingerprint); err != nil {
		return fmt.Errorf("failed to update user GPG key: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit GPG key registration: %w", err)
	}
	return nil
}

func (db *DB) RefreshGPGPublicKey(key *core.GPGPublicKey, aliases []string) error {
	key = normalizeGPGPublicKey(key)
	if db == nil || db.SQLDB == nil || key == nil || key.Fingerprint == "" || len(key.PublicKey) == 0 {
		return errors.New("invalid GPG public key")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := db.upsertGPGPublicKey(tx, key); err != nil {
		return fmt.Errorf("failed to refresh GPG public key: %w", err)
	}
	if err := replaceGPGKeyAliases(tx, key.Fingerprint, aliases); err != nil {
		return fmt.Errorf("failed to refresh GPG key aliases: %w", err)
	}
	return tx.Commit()
}

func (db *DB) DeleteUserGPGKey(username, fingerprint string) error {
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), 255))
	fingerprint = strings.ToUpper(SanitizeInputString(strings.TrimSpace(fingerprint), 64))
	if db == nil || db.SQLDB == nil || username == "" || fingerprint == "" {
		return nil
	}
	_, err := db.Exec(`DELETE FROM user_gpg_keys WHERE username = ? AND fingerprint = ?`, username, fingerprint)
	if err != nil {
		return fmt.Errorf("failed to delete GPG key (%s) for user (%s): %w", fingerprint, username, err)
	}
	return nil
}

const gpgSignatureColumns = `artifact_key, repository, artifact_path, fingerprint, key_id, primary_identity, uploader, signature_created_at, verified_at, hash_algorithm, public_key_algorithm`

func normalizeGPGSignature(signature *core.GPGSignature) *core.GPGSignature {
	if signature == nil {
		return nil
	}
	return &core.GPGSignature{
		ArtifactKey:        strings.ToLower(SanitizeInputString(strings.TrimSpace(signature.ArtifactKey), 64)),
		Repository:         SanitizeInputString(signature.Repository, 255),
		ArtifactPath:       SanitizeInputString(signature.ArtifactPath, 8192),
		Fingerprint:        strings.ToUpper(SanitizeInputString(signature.Fingerprint, 64)),
		KeyID:              strings.ToUpper(SanitizeInputString(signature.KeyID, 16)),
		PrimaryIdentity:    SanitizeInputString(signature.PrimaryIdentity, 2048),
		Uploader:           strings.ToLower(SanitizeInputString(signature.Uploader, 255)),
		SignatureCreatedAt: signature.SignatureCreatedAt,
		VerifiedAt:         signature.VerifiedAt,
		HashAlgorithm:      SanitizeInputString(signature.HashAlgorithm, 32),
		PublicKeyAlgorithm: SanitizeInputString(signature.PublicKeyAlgorithm, 32),
	}
}

func (db *DB) SaveGPGSignature(signature *core.GPGSignature) error {
	signature = normalizeGPGSignature(signature)
	if db == nil || db.SQLDB == nil || signature == nil || len(signature.ArtifactKey) != 64 || signature.Repository == "" || signature.ArtifactPath == "" {
		return errors.New("invalid GPG signature record")
	}
	query := db.Dialect.UpsertGPGSignatureQuery()
	_, err := db.Exec(query,
		signature.ArtifactKey,
		signature.Repository,
		signature.ArtifactPath,
		signature.Fingerprint,
		signature.KeyID,
		signature.PrimaryIdentity,
		signature.Uploader,
		signature.SignatureCreatedAt,
		signature.VerifiedAt,
		signature.HashAlgorithm,
		signature.PublicKeyAlgorithm,
	)
	if err != nil {
		return fmt.Errorf("failed to save GPG signature record: %w", err)
	}
	return nil
}

func scanGPGSignature(row rowScanner) (*core.GPGSignature, error) {
	signature := &core.GPGSignature{}
	if err := row.Scan(
		&signature.ArtifactKey,
		&signature.Repository,
		&signature.ArtifactPath,
		&signature.Fingerprint,
		&signature.KeyID,
		&signature.PrimaryIdentity,
		&signature.Uploader,
		&signature.SignatureCreatedAt,
		&signature.VerifiedAt,
		&signature.HashAlgorithm,
		&signature.PublicKeyAlgorithm,
	); err != nil {
		return nil, err
	}
	return signature, nil
}

func (db *DB) GetGPGSignature(artifactKey string) (*core.GPGSignature, error) {
	artifactKey = strings.ToLower(SanitizeInputString(strings.TrimSpace(artifactKey), 64))
	if db == nil || db.SQLDB == nil || len(artifactKey) != 64 {
		return nil, nil
	}
	signature, err := scanGPGSignature(db.QueryRow(
		`SELECT `+gpgSignatureColumns+` FROM gpg_signatures WHERE artifact_key = ?`, artifactKey,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get GPG signature record: %w", err)
	}
	return signature, nil
}

func (db *DB) GetGPGSignatures(artifactKeys []string) ([]*core.GPGSignature, error) {
	if db == nil || db.SQLDB == nil || len(artifactKeys) == 0 {
		return []*core.GPGSignature{}, nil
	}
	keys := make([]string, 0, len(artifactKeys))
	seen := make(map[string]struct{}, len(artifactKeys))
	for _, key := range artifactKeys {
		key = strings.ToLower(strings.TrimSpace(key))
		if len(key) != 64 {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return []*core.GPGSignature{}, nil
	}
	result := make([]*core.GPGSignature, 0, len(keys))
	const batchSize = 500
	for start := 0; start < len(keys); start += batchSize {
		end := min(start+batchSize, len(keys))
		batch := keys[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		args := make([]any, len(batch))
		for i := range batch {
			args[i] = batch[i]
		}
		rows, err := db.Query(`SELECT `+gpgSignatureColumns+` FROM gpg_signatures WHERE artifact_key IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to list GPG signature records: %w", err)
		}
		for rows.Next() {
			signature, scanErr := scanGPGSignature(rows)
			if scanErr != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("failed to scan GPG signature record: %w", scanErr)
			}
			result = append(result, signature)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (db *DB) DeleteGPGSignature(artifactKey string) error {
	artifactKey = strings.ToLower(SanitizeInputString(strings.TrimSpace(artifactKey), 64))
	if db == nil || db.SQLDB == nil || len(artifactKey) != 64 {
		return nil
	}
	_, err := db.Exec(`DELETE FROM gpg_signatures WHERE artifact_key = ?`, artifactKey)
	return err
}

func escapeLikePrefix(value string) string {
	value = strings.ReplaceAll(value, "!", "!!")
	value = strings.ReplaceAll(value, "%", "!%")
	return strings.ReplaceAll(value, "_", "!_")
}

func (db *DB) DeleteGPGSignaturesByPrefix(repository, artifactPathPrefix string) error {
	repository = SanitizeInputString(repository, 255)
	artifactPathPrefix = strings.TrimSuffix(SanitizeInputString(artifactPathPrefix, 8192), "/")
	if db == nil || db.SQLDB == nil || repository == "" || artifactPathPrefix == "" {
		return nil
	}
	likePrefix := escapeLikePrefix(artifactPathPrefix) + "/%"
	_, err := db.Exec(`DELETE FROM gpg_signatures WHERE repository = ? AND (artifact_path = ? OR artifact_path LIKE ? ESCAPE '!')`,
		repository, artifactPathPrefix, likePrefix)
	return err
}

func (db *DB) DeleteGPGSignaturesByRepository(repository string) error {
	repository = SanitizeInputString(repository, 255)
	if db == nil || db.SQLDB == nil || repository == "" {
		return nil
	}
	_, err := db.Exec(`DELETE FROM gpg_signatures WHERE repository = ?`, repository)
	return err
}
