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

	"renop/internal/core"
)

const maxGitHubPrincipals = 1001

func scanGitHubIdentity(row *sql.Row) (*core.GitHubIdentity, error) {
	identity := &core.GitHubIdentity{}
	err := row.Scan(&identity.UserID, &identity.Username, &identity.GitHubUserID,
		&identity.GitHubLogin, &identity.AuthorizedAt, &identity.PrincipalCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return identity, nil
}

// GetGitHubIdentity returns the GitHub identity linked to a RenoP username.
func (db *DB) GetGitHubIdentity(username string) (*core.GitHubIdentity, error) {
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	if username == "" {
		return nil, nil
	}
	identity, err := scanGitHubIdentity(db.QueryRow(`SELECT i.user_id, p.username, i.github_user_id,
		i.github_login, i.authorized_at,
		(SELECT COUNT(*) FROM github_principals principal WHERE principal.user_id = i.user_id)
		FROM github_identities i JOIN user_profiles p ON p.user_id = i.user_id
		WHERE p.username = ?`, username))
	if err != nil {
		return nil, fmt.Errorf("get GitHub identity for %s: %w", username, err)
	}
	return identity, nil
}

// GetGitHubIdentityByProviderID returns the RenoP account linked to an immutable GitHub user ID.
func (db *DB) GetGitHubIdentityByProviderID(githubUserID int64) (*core.GitHubIdentity, error) {
	if githubUserID <= 0 {
		return nil, nil
	}
	identity, err := scanGitHubIdentity(db.QueryRow(`SELECT i.user_id, p.username, i.github_user_id,
		i.github_login, i.authorized_at,
		(SELECT COUNT(*) FROM github_principals principal WHERE principal.user_id = i.user_id)
		FROM github_identities i JOIN user_profiles p ON p.user_id = i.user_id
		WHERE i.github_user_id = ?`, githubUserID))
	if err != nil {
		return nil, fmt.Errorf("get GitHub identity %d: %w", githubUserID, err)
	}
	return identity, nil
}

// StoreGitHubIdentity links an identity and atomically replaces its authorized principal snapshot.
func (db *DB) StoreGitHubIdentity(userID string, githubUserID int64, githubLogin string,
	principals []core.GitHubPrincipal, authorizedAt int64) error {
	userID = SanitizeInputString(strings.TrimSpace(userID), 36)
	githubLogin = strings.ToLower(SanitizeInputString(strings.TrimSpace(githubLogin), 39))
	if userID == "" || githubUserID <= 0 || githubLogin == "" || authorizedAt <= 0 ||
		len(principals) == 0 || len(principals) > maxGitHubPrincipals {
		return errors.New("GitHub identity payload is invalid")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin GitHub identity update: %w", err)
	}
	defer tx.Rollback()

	var profileExists int
	if err := tx.QueryRow(`SELECT 1 FROM user_profiles WHERE user_id = ?`, userID).Scan(&profileExists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.ErrUserProfileNotFound
		}
		return fmt.Errorf("resolve GitHub identity owner: %w", err)
	}

	var linkedUserID string
	err = tx.QueryRow(`SELECT user_id FROM github_identities WHERE github_user_id = ?`, githubUserID).Scan(&linkedUserID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect GitHub provider identity: %w", err)
	}
	if err == nil && linkedUserID != userID {
		return core.ErrGitHubIdentityLinked
	}

	var linkedGitHubID int64
	err = tx.QueryRow(`SELECT github_user_id FROM github_identities WHERE user_id = ?`, userID).Scan(&linkedGitHubID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect RenoP GitHub identity: %w", err)
	}
	if err == nil && linkedGitHubID != githubUserID {
		return core.ErrGitHubIdentityLinked
	}

	if linkedUserID == "" {
		if _, err := tx.Exec(`INSERT INTO github_identities
			(github_user_id, user_id, github_login, authorized_at) VALUES (?, ?, ?, ?)`,
			githubUserID, userID, githubLogin, authorizedAt); err != nil {
			return fmt.Errorf("create GitHub identity: %w", err)
		}
	} else if _, err := tx.Exec(`UPDATE github_identities SET github_login = ?, authorized_at = ?
		WHERE github_user_id = ? AND user_id = ?`, githubLogin, authorizedAt, githubUserID, userID); err != nil {
		return fmt.Errorf("refresh GitHub identity: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM github_principals WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("replace GitHub principals: %w", err)
	}
	seen := make(map[string]struct{}, len(principals))
	for _, principal := range principals {
		principal.Type = strings.ToLower(strings.TrimSpace(principal.Type))
		principal.Login = strings.ToLower(SanitizeInputString(strings.TrimSpace(principal.Login), 39))
		if (principal.Type != core.GitHubPrincipalUser && principal.Type != core.GitHubPrincipalOrganization) ||
			principal.GitHubID <= 0 || principal.Login == "" {
			return errors.New("GitHub principal payload is invalid")
		}
		key := principal.Type + ":" + fmt.Sprint(principal.GitHubID)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		if _, err := tx.Exec(`INSERT INTO github_principals
			(user_id, principal_type, github_principal_id, github_login, authorized_at)
			VALUES (?, ?, ?, ?, ?)`, userID, principal.Type, principal.GitHubID,
			principal.Login, authorizedAt); err != nil {
			return fmt.Errorf("store GitHub principal: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit GitHub identity update: %w", err)
	}
	return nil
}

// DeleteGitHubIdentity removes the identity and authorized principal snapshot for one account.
func (db *DB) DeleteGitHubIdentity(username string) error {
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	if username == "" {
		return core.ErrGitHubIdentityNotFound
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin GitHub identity deletion: %w", err)
	}
	defer tx.Rollback()
	var userID string
	if err := tx.QueryRow(`SELECT i.user_id FROM github_identities i
		JOIN user_profiles p ON p.user_id = i.user_id WHERE p.username = ?`, username).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.ErrGitHubIdentityNotFound
		}
		return fmt.Errorf("resolve GitHub identity for deletion: %w", err)
	}
	if err := lockAccountLoginMethodsTx(tx, userID); err != nil {
		return fmt.Errorf("lock account login methods before GitHub deletion: %w", err)
	}
	hasAlternate, err := hasLoginWithoutGitHubTx(tx, userID, username)
	if err != nil {
		return fmt.Errorf("inspect login methods before GitHub deletion: %w", err)
	}
	if !hasAlternate {
		return core.ErrLastLoginMethod
	}
	if _, err := tx.Exec(`DELETE FROM github_principals WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("delete GitHub principals: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM github_identities WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("delete GitHub identity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit GitHub identity deletion: %w", err)
	}
	return nil
}

// HasRecentGitHubPrincipal reports whether a recent OAuth snapshot contains an account or organization login.
func (db *DB) HasRecentGitHubPrincipal(username, login string, authorizedAfter int64) (bool, error) {
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	login = strings.ToLower(SanitizeInputString(strings.TrimSpace(login), 39))
	if username == "" || login == "" {
		return false, nil
	}
	var exists int
	err := db.QueryRow(`SELECT 1 FROM github_principals principal
		JOIN user_profiles profile ON profile.user_id = principal.user_id
		WHERE profile.username = ? AND principal.github_login = ? AND principal.authorized_at >= ?`,
		username, login, authorizedAfter).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect GitHub principal %s for %s: %w", login, username, err)
	}
	return true, nil
}
