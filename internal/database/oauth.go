/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/goccy/go-json"
	"renop/internal/core"
)

const oauthIdentityColumns = "i.provider_id, i.subject, i.authority, i.user_id, p.username, i.login, i.namespaces_json, i.authorized_at"

func scanOAuthIdentity(row row) (*core.OAuthIdentity, error) {
	i := &core.OAuthIdentity{}
	var namespaces string
	err := row.Scan(&i.ProviderID, &i.Subject, &i.Authority, &i.UserID, &i.Username, &i.Login, &namespaces, &i.AuthorizedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err == nil {
		err = json.Unmarshal([]byte(namespaces), &i.Namespaces)
	}
	return i, err
}

// GetOAuthIdentities returns the bounded private provider bindings for one account.
func (db *DB) GetOAuthIdentities(username string) ([]core.OAuthIdentity, error) {
	rows, err := db.Query(`SELECT `+oauthIdentityColumns+` FROM oauth_identities i
		JOIN user_profiles p ON p.user_id = i.user_id WHERE p.username = ? ORDER BY i.provider_id LIMIT 32`, strings.ToLower(username))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	identities := []core.OAuthIdentity{}
	for rows.Next() {
		i, err := scanOAuthIdentity(rows)
		if err != nil {
			return nil, err
		}
		identities = append(identities, *i)
	}
	return identities, rows.Err()
}

// GetOAuthIdentity resolves a provider subject only within its original configuration authority.
func (db *DB) GetOAuthIdentity(identity core.OAuthIdentity) (*core.OAuthIdentity, error) {
	if !identity.Valid() {
		return nil, core.ErrOAuthIdentityNotFound
	}
	return scanOAuthIdentity(db.QueryRow(`SELECT `+oauthIdentityColumns+` FROM oauth_identities i
		JOIN user_profiles p ON p.user_id = i.user_id WHERE i.identity_hash = ?`, identity.Key()))
}

func touchOAuthSecurityTx(tx *Tx, userID string, now int64) error {
	if err := ensureAccountSecurityTx(tx, userID, now); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE user_account_security SET updated_at = CASE WHEN updated_at >= ? THEN updated_at + 1 ELSE ? END
		WHERE user_id = ?`, now, now, userID)
	return err
}

func storeOAuthIdentityTx(tx *Tx, userID string, identity core.OAuthIdentity, now int64) error {
	if !identity.Valid() || userID == "" || now <= 0 {
		return core.ErrRegistrationInvalid
	}
	if err := lockAccountLoginMethodsTx(tx, userID); err != nil {
		return err
	}
	var linkedUser, linkedKey string
	err := tx.QueryRow(`SELECT user_id FROM oauth_identities WHERE identity_hash = ?`, identity.Key()).Scan(&linkedUser)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if linkedUser != "" && linkedUser != userID {
		return core.ErrOAuthIdentityLinked
	}
	err = tx.QueryRow(`SELECT identity_hash FROM oauth_identities WHERE user_id = ? AND provider_id = ?`, userID, identity.ProviderID).Scan(&linkedKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if linkedKey != "" && linkedKey != identity.Key() {
		return core.ErrOAuthIdentityLinked
	}
	if linkedUser != "" {
		return refreshOAuthIdentityTx(tx, userID, identity, now)
	}
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM oauth_identities WHERE user_id = ?`, userID).Scan(&count); err != nil {
		return err
	}
	if count >= 32 {
		return core.ErrOAuthIdentityLinked
	}
	namespaces, err := json.Marshal(identity.Namespaces)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO oauth_identities (identity_hash, provider_id, subject, authority, user_id, login, namespaces_json, authorized_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, identity.Key(), identity.ProviderID, identity.Subject, identity.Authority, userID, identity.Login, string(namespaces), now); err != nil {
		if uniqueConstraintError(err) {
			return core.ErrOAuthIdentityLinked
		}
		return err
	}
	return touchOAuthSecurityTx(tx, userID, now)
}

func refreshOAuthIdentityTx(tx *Tx, userID string, identity core.OAuthIdentity, now int64) error {
	namespaces, err := json.Marshal(identity.Namespaces)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE oauth_identities SET login = ?, namespaces_json = ?, authorized_at = ? WHERE identity_hash = ? AND user_id = ?`,
		identity.Login, string(namespaces), now, identity.Key(), userID)
	return err
}

// RefreshOAuthIdentity updates a fresh provider proof without recreating a removed binding.
func (db *DB) RefreshOAuthIdentity(userID string, identity core.OAuthIdentity, now int64) error {
	if !identity.Valid() || userID == "" || now <= 0 {
		return core.ErrOAuthIdentityNotFound
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockAccountLoginMethodsTx(tx, userID); err != nil {
		return err
	}
	if err = refreshOAuthIdentityTx(tx, userID, identity, now); err != nil {
		return err
	}
	return tx.Commit()
}

// LinkOAuthIdentity rechecks the initiating browser session and credentials before attaching a provider.
func (db *DB) LinkOAuthIdentity(username, session, snapshot string, identity core.OAuthIdentity, now int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	account, err := accountEmailSessionTx(tx, strings.ToLower(username), session)
	if err != nil {
		return err
	}
	if snapshot == "" || account.Snapshot != snapshot {
		return core.ErrMFAInvalid
	}
	if err = storeOAuthIdentityTx(tx, account.UserID, identity, now); err != nil {
		return err
	}
	return tx.Commit()
}

func hasOAuthLoginTx(tx *Tx, userID, excludedProvider string) (bool, error) {
	var count int
	err := tx.QueryRow(`SELECT COUNT(*) FROM oauth_identities WHERE user_id = ? AND provider_id <> ?`, userID, excludedProvider).Scan(&count)
	return count > 0, err
}

// DeleteOAuthIdentity removes one binding while preserving another primary login method.
func (db *DB) DeleteOAuthIdentity(username, session, provider string, now int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	account, err := accountEmailSessionTx(tx, strings.ToLower(username), session)
	if err != nil {
		return err
	}
	var exists int
	err = tx.QueryRow(`SELECT 1 FROM oauth_identities WHERE user_id = ? AND provider_id = ?`, account.UserID, provider).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrOAuthIdentityNotFound
	}
	if err != nil {
		return err
	}
	other, err := hasOAuthLoginTx(tx, account.UserID, provider)
	if err != nil {
		return err
	}
	if !other && account.GitHubID == 0 && !(account.PasswordHash != "" && account.PasswordLoginEnabled) {
		var fido int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM fido_devices WHERE username = ?`, username).Scan(&fido); err != nil {
			return err
		}
		if account.Passkey || fido == 0 {
			return core.ErrLastLoginMethod
		}
	}
	if _, err = tx.Exec(`DELETE FROM oauth_identities WHERE user_id = ? AND provider_id = ?`, account.UserID, provider); err != nil {
		return err
	}
	if err = touchOAuthSecurityTx(tx, account.UserID, now); err != nil {
		return err
	}
	return tx.Commit()
}
