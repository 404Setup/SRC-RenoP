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

	"github.com/google/uuid"

	"renop/internal/core"
)

// GetUserProfile returns the public identity and durable rename state for an account.
func (db *DB) GetUserProfile(username string) (*core.UserProfile, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	if username == "" {
		return nil, core.ErrUserProfileNotFound
	}
	return db.getUserProfile(`p.username = ?`, username)
}

// GetUserProfileByID returns a public profile through its immutable identifier.
func (db *DB) GetUserProfileByID(userID string) (*core.UserProfile, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	userID = strings.ToLower(strings.TrimSpace(userID))
	if _, err := uuid.Parse(userID); err != nil {
		return nil, core.ErrUserProfileNotFound
	}
	return db.getUserProfile(`p.user_id = ?`, userID)
}

func (db *DB) getUserProfile(whereClause, value string) (*core.UserProfile, error) {
	profile := &core.UserProfile{}
	err := db.QueryRow(`SELECT p.user_id, p.username, t.created_at, p.nickname,
		p.rename_window_started_at, p.rename_count,
		(SELECT COUNT(*) FROM maven_domain_members mm JOIN maven_domains md
			ON md.repository = mm.repository AND md.domain = mm.domain
			WHERE mm.user_id = p.user_id AND md.repository = '' AND md.verified = 1),
		(SELECT COUNT(*) FROM cargo_members cm WHERE cm.user_id = p.user_id),
		(SELECT COUNT(*) FROM docker_members dm WHERE dm.user_id = p.user_id),
		(SELECT COUNT(*) FROM npm_members nm WHERE nm.user_id = p.user_id)
		FROM user_profiles p JOIN tokens t ON t.name = p.username WHERE `+whereClause, value).Scan(
		&profile.UserID, &profile.Username, &profile.CreatedAt, &profile.Nickname,
		&profile.UsernameChangeWindowAt, &profile.UsernameChangeCount,
		&profile.MavenDomainCount, &profile.CargoPackageCount, &profile.DockerImageCount, &profile.NPMPackageCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrUserProfileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load user profile %s: %w", value, err)
	}
	db.cacheUserProfile(profile)
	return profile, nil
}

func profileSummary(profile *core.UserProfile) core.UserProfile {
	if profile == nil {
		return core.UserProfile{}
	}
	return core.UserProfile{
		UserID: profile.UserID, Username: profile.Username, CreatedAt: profile.CreatedAt,
		Nickname: profile.Nickname, UsernameChangeWindowAt: profile.UsernameChangeWindowAt,
		UsernameChangeCount: profile.UsernameChangeCount,
	}
}

func (db *DB) cacheUserProfile(profile *core.UserProfile) {
	if db == nil || profile == nil || profile.UserID == "" || profile.Username == "" {
		return
	}
	username := strings.ToLower(profile.Username)
	if db.userIDCache != nil {
		db.userIDCache.Set(username, profile.UserID, 30*time.Minute)
	}
	if db.profileCache != nil {
		db.profileCache.Set(username, profileSummary(profile), 10*time.Minute)
	}
}

func (db *DB) invalidateUserProfileCaches(usernames ...string) {
	if db == nil {
		return
	}
	for _, username := range usernames {
		username = strings.ToLower(strings.TrimSpace(username))
		if username == "" {
			continue
		}
		if db.userIDCache != nil {
			db.userIDCache.Delete(username)
		}
		if db.profileCache != nil {
			db.profileCache.Delete(username)
		}
	}
}

// ListUserPackageMemberships returns format-specific teams linked to an immutable user ID.
func (db *DB) ListUserPackageMemberships(userID, format string) ([]*core.UserPackageMembership, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	userID = strings.ToLower(strings.TrimSpace(userID))
	if _, err := uuid.Parse(userID); err != nil {
		return nil, core.ErrUserProfileNotFound
	}
	format = strings.ToLower(strings.TrimSpace(format))
	var query string
	switch format {
	case "maven":
		query = `SELECT '', d.domain, '', m.permission_level, 0
			FROM maven_domain_members m JOIN maven_domains d ON d.repository = m.repository
			AND d.domain = m.domain WHERE m.user_id = ? AND d.repository = '' AND d.verified = 1 ORDER BY d.domain`
	case "cargo":
		query = `SELECT p.repository, p.package_name, p.description, m.permission_level, p.archived
			FROM cargo_members m JOIN cargo_packages p ON p.repository = m.repository
			AND p.normalized_name = m.normalized_name WHERE m.user_id = ?
			ORDER BY p.repository, p.normalized_name`
	case "docker":
		query = `SELECT i.repository, i.image_name, i.description, m.permission_level, 0
			FROM docker_members m JOIN docker_images i ON i.repository = m.repository
			AND i.image_name = m.image_name WHERE m.user_id = ?
			ORDER BY i.repository, i.image_name`
	case "npm":
		query = `SELECT p.repository, p.package_name, p.description, m.permission_level, p.archived
			FROM npm_members m JOIN npm_packages p ON p.repository = m.repository
			AND p.package_name = m.package_name WHERE m.user_id = ?
			ORDER BY p.repository, p.package_name`
	default:
		return nil, errors.New("package membership format must be maven, cargo, docker, or npm")
	}
	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("list %s memberships for user %s: %w", format, userID, err)
	}
	defer rows.Close()
	memberships := make([]*core.UserPackageMembership, 0)
	for rows.Next() {
		membership := &core.UserPackageMembership{Format: format}
		var archived int
		if err := rows.Scan(&membership.Repository, &membership.Name, &membership.Description,
			&membership.PermissionLevel, &archived); err != nil {
			return nil, fmt.Errorf("scan %s membership for user %s: %w", format, userID, err)
		}
		membership.Archived = archived != 0
		memberships = append(memberships, membership)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s memberships for user %s: %w", format, userID, err)
	}
	return memberships, nil
}

// GetUserProfiles loads a bounded account batch for nickname-first list rendering.
func (db *DB) GetUserProfiles(usernames []string) (map[string]*core.UserProfile, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	if len(usernames) == 0 || len(usernames) > 50 {
		return nil, errors.New("user profile batch must contain between 1 and 50 usernames")
	}
	requested := make([]string, 0, len(usernames))
	seen := make(map[string]struct{}, len(usernames))
	for _, candidate := range usernames {
		username := strings.ToLower(SanitizeInputString(strings.TrimSpace(candidate), maxTokenNameLen))
		if username == "" {
			continue
		}
		if _, duplicate := seen[username]; duplicate {
			continue
		}
		seen[username] = struct{}{}
		requested = append(requested, username)
	}
	if len(requested) == 0 {
		return map[string]*core.UserProfile{}, nil
	}
	profiles := make(map[string]*core.UserProfile, len(requested))
	missing := make([]string, 0, len(requested))
	for _, username := range requested {
		if db.profileCache != nil {
			if cached, ok := db.profileCache.Get(username); ok {
				if cached.UserID != "" {
					copy := cached
					profiles[username] = &copy
				}
				continue
			}
		}
		missing = append(missing, username)
	}
	if len(missing) == 0 {
		return profiles, nil
	}
	arguments := make([]any, len(missing))
	placeholders := make([]string, len(missing))
	for index, username := range missing {
		arguments[index] = username
		placeholders[index] = "?"
	}
	rows, err := db.Query(`SELECT p.user_id, p.username, t.created_at, p.nickname,
		p.rename_window_started_at, p.rename_count
		FROM user_profiles p JOIN tokens t ON t.name = p.username
		WHERE p.username IN (`+strings.Join(placeholders, ",")+`)`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("load user profile batch: %w", err)
	}
	defer rows.Close()
	found := make(map[string]struct{}, len(missing))
	for rows.Next() {
		profile := &core.UserProfile{}
		if err := rows.Scan(&profile.UserID, &profile.Username, &profile.CreatedAt, &profile.Nickname,
			&profile.UsernameChangeWindowAt, &profile.UsernameChangeCount); err != nil {
			return nil, fmt.Errorf("scan user profile batch: %w", err)
		}
		profiles[profile.Username] = profile
		found[profile.Username] = struct{}{}
		db.cacheUserProfile(profile)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user profile batch: %w", err)
	}
	if db.profileCache != nil {
		for _, username := range missing {
			if _, ok := found[username]; !ok {
				db.profileCache.Set(username, core.UserProfile{}, 30*time.Second)
			}
		}
	}
	return profiles, nil
}

// UpdateUserProfile atomically updates a nickname and, when requested, renames all account references.
func (db *DB) UpdateUserProfile(oldUsername, newUsername, nickname string, token *core.AccessToken,
	changedAt int64, changes core.AccountTokenChanges) (*core.UserProfile, error) {
	if db == nil || db.SQLDB == nil || token == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	oldUsername = strings.ToLower(SanitizeInputString(strings.TrimSpace(oldUsername), maxTokenNameLen))
	newUsername = strings.ToLower(SanitizeInputString(strings.TrimSpace(newUsername), maxTokenNameLen))
	nickname, nicknameValid := core.NormalizeNickname(nickname)
	if oldUsername == "" || newUsername == "" || changedAt <= 0 || !nicknameValid ||
		(oldUsername != newUsername && !isValidProfileUsername(newUsername)) {
		return nil, errors.New("user profile update is invalid")
	}
	renamed := oldUsername != newUsername

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin user profile update: %w", err)
	}
	defer tx.Rollback()

	profile := &core.UserProfile{Username: oldUsername}
	profileExists := true
	if err := tx.QueryRow(`SELECT user_id, nickname, rename_window_started_at, rename_count
		FROM user_profiles WHERE username = ?`, oldUsername).Scan(
		&profile.UserID, &profile.Nickname, &profile.UsernameChangeWindowAt, &profile.UsernameChangeCount,
	); errors.Is(err, sql.ErrNoRows) {
		profileExists = false
		profile.UserID = uuid.NewString()
		profile.Nickname = ""
		profile.UsernameChangeWindowAt = 0
		profile.UsernameChangeCount = 0
	} else if err != nil {
		return nil, fmt.Errorf("inspect durable user profile %s: %w", oldUsername, err)
	}
	if profileExists {
		if err := lockAccountLoginMethodsTx(tx, profile.UserID); err != nil {
			return nil, fmt.Errorf("lock user profile %s: %w", oldUsername, err)
		}
	}
	currentToken, err := tokenByNameTx(tx, oldUsername)
	if err != nil {
		return nil, fmt.Errorf("inspect profile account %s: %w", oldUsername, err)
	}
	if currentToken == nil {
		return nil, core.ErrUserProfileNotFound
	}
	profile.CreatedAt = currentToken.CreatedAt
	desiredToken := token
	token = currentToken
	if changes.Password {
		token.EncryptedSecret = desiredToken.EncryptedSecret
		token.PasswordHash = desiredToken.PasswordHash
	}
	if changes.Permissions {
		token.Permissions = append([]string(nil), desiredToken.Permissions...)
	}
	if !renamed {
		if err := db.saveTokenInTx(tx, oldUsername, token); err != nil {
			return nil, err
		}
	}

	if renamed {
		windowExpired := profile.UsernameChangeWindowAt <= 0 || changedAt < profile.UsernameChangeWindowAt ||
			changedAt-profile.UsernameChangeWindowAt >= core.UsernameChangeWindowMillis
		if windowExpired {
			profile.UsernameChangeWindowAt = changedAt
			profile.UsernameChangeCount = 0
		}
		if profile.UsernameChangeCount >= core.MaxUsernameChangesPerDay {
			return nil, &core.UsernameChangeRateError{
				RetryAt: profile.UsernameChangeWindowAt + core.UsernameChangeWindowMillis,
			}
		}
		if err := db.renameTokenInTx(tx, oldUsername, newUsername, token); err != nil {
			return nil, err
		}
		profile.Username = newUsername
		profile.UsernameChangeCount++
	}
	profile.Nickname = nickname
	if profileExists {
		if _, err := tx.Exec(`UPDATE user_profiles SET username = ?, nickname = ?,
			rename_window_started_at = ?, rename_count = ?, updated_at = ? WHERE user_id = ?`,
			profile.Username, profile.Nickname, profile.UsernameChangeWindowAt,
			profile.UsernameChangeCount, changedAt, profile.UserID); err != nil {
			return nil, fmt.Errorf("update user profile %s: %w", oldUsername, err)
		}
	} else if _, err := tx.Exec(`INSERT INTO user_profiles
		(user_id, username, nickname, rename_window_started_at, rename_count, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		profile.UserID, profile.Username, profile.Nickname, profile.UsernameChangeWindowAt,
		profile.UsernameChangeCount, changedAt); err != nil {
		return nil, fmt.Errorf("create user profile %s: %w", profile.Username, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit user profile update: %w", err)
	}
	if renamed {
		db.finishTokenRename(oldUsername, newUsername, token)
	} else {
		db.finishTokenUpdate(oldUsername, token)
	}
	db.invalidateUserProfileCaches(oldUsername, newUsername)
	db.cacheUserProfile(profile)
	return profile, nil
}

func isValidProfileUsername(username string) bool {
	normalized, valid := core.NormalizeUsername(username)
	return valid && normalized == username
}

func (db *DB) userIDForUsername(username string) (string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return "", core.ErrUserProfileNotFound
	}
	userID, err := db.userIDCache.GetOrLoad(username, func() (string, time.Duration, error) {
		var loaded string
		if scanErr := db.QueryRow(`SELECT user_id FROM user_profiles WHERE username = ?`, username).Scan(&loaded); errors.Is(scanErr, sql.ErrNoRows) {
			return "", 30 * time.Second, nil
		} else if scanErr != nil {
			return "", 0, fmt.Errorf("resolve user ID for %s: %w", username, scanErr)
		}
		return loaded, 30 * time.Minute, nil
	})
	if err != nil {
		return "", err
	}
	if userID == "" {
		return "", core.ErrUserProfileNotFound
	}
	return userID, nil
}

func (db *DB) userIDForExistingAccount(username string) (string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return "", core.ErrUserProfileNotFound
	}
	token, err := db.GetTokenByName(username)
	if err != nil {
		return "", err
	}
	if token == nil {
		return "", core.ErrUserProfileNotFound
	}
	return db.userIDForUsername(username)
}

func userIDForUsernameTx(tx *Tx, username string) (string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	var userID string
	if err := tx.QueryRow(`SELECT user_id FROM user_profiles WHERE username = ?`, username).Scan(&userID); errors.Is(err, sql.ErrNoRows) {
		return "", core.ErrUserProfileNotFound
	} else if err != nil {
		return "", fmt.Errorf("resolve transaction user ID for %s: %w", username, err)
	}
	return userID, nil
}

func (db *DB) ensureUserProfile(username string) (string, error) {
	if userID, err := db.userIDForUsername(username); err == nil {
		return userID, nil
	} else if !errors.Is(err, core.ErrUserProfileNotFound) {
		return "", err
	}
	userID := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO user_profiles
		(user_id, username, nickname, rename_window_started_at, rename_count, updated_at)
		VALUES (?, ?, '', 0, 0, ?)`, userID, strings.ToLower(strings.TrimSpace(username)), time.Now().UnixMilli()); err != nil {
		if existingID, lookupErr := db.userIDForUsername(username); lookupErr == nil {
			return existingID, nil
		}
		return "", fmt.Errorf("create stable user identity for %s: %w", username, err)
	}
	if db.userIDCache != nil {
		db.userIDCache.Set(strings.ToLower(strings.TrimSpace(username)), userID, 30*time.Minute)
	}
	return userID, nil
}

func (db *DB) initializeUserIdentities() error {
	profileRows, err := db.Query(`SELECT username, user_id FROM user_profiles`)
	if err != nil {
		return fmt.Errorf("list existing user profiles for ID migration: %w", err)
	}
	type profileIdentity struct {
		username string
		userID   sql.NullString
	}
	existingProfiles := make([]profileIdentity, 0)
	for profileRows.Next() {
		var identity profileIdentity
		if err := profileRows.Scan(&identity.username, &identity.userID); err != nil {
			_ = profileRows.Close()
			return fmt.Errorf("scan existing user profile identity: %w", err)
		}
		existingProfiles = append(existingProfiles, identity)
	}
	if err := profileRows.Err(); err != nil {
		_ = profileRows.Close()
		return fmt.Errorf("iterate existing user profile identities: %w", err)
	}
	if err := profileRows.Close(); err != nil {
		return fmt.Errorf("close existing user profile identities: %w", err)
	}
	for _, identity := range existingProfiles {
		if identity.userID.Valid && strings.TrimSpace(identity.userID.String) != "" {
			continue
		}
		if _, err := db.Exec(`UPDATE user_profiles SET user_id = ? WHERE username = ?`,
			uuid.NewString(), identity.username); err != nil {
			return fmt.Errorf("backfill immutable ID for user profile %s: %w", identity.username, err)
		}
	}

	usernames := make([]string, 0)
	seen := make(map[string]struct{})
	for _, query := range []string{
		`SELECT name FROM tokens`,
		`SELECT DISTINCT username FROM maven_domain_members`,
		`SELECT DISTINCT username FROM cargo_members`,
		`SELECT DISTINCT username FROM docker_members`,
		`SELECT DISTINCT username FROM npm_members`,
	} {
		rows, err := db.Query(query)
		if err != nil {
			return fmt.Errorf("list subjects for stable identity migration: %w", err)
		}
		for rows.Next() {
			var username string
			if err := rows.Scan(&username); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan subject for stable identity migration: %w", err)
			}
			username = strings.ToLower(strings.TrimSpace(username))
			if _, exists := seen[username]; username == "" || exists {
				continue
			}
			seen[username] = struct{}{}
			usernames = append(usernames, username)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("iterate stable identity subjects: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close stable identity subject rows: %w", err)
		}
	}
	for _, username := range usernames {
		userID, err := db.ensureUserProfile(username)
		if err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE cargo_members SET user_id = ? WHERE username = ? AND user_id IS NULL`, userID, username); err != nil {
			return fmt.Errorf("backfill Cargo member identity for %s: %w", username, err)
		}
		if _, err := db.Exec(`UPDATE maven_domain_members SET user_id = ? WHERE username = ? AND user_id IS NULL`, userID, username); err != nil {
			return fmt.Errorf("backfill Maven member identity for %s: %w", username, err)
		}
		if _, err := db.Exec(`UPDATE docker_members SET user_id = ? WHERE username = ? AND user_id IS NULL`, userID, username); err != nil {
			return fmt.Errorf("backfill Docker member identity for %s: %w", username, err)
		}
		if _, err := db.Exec(`UPDATE npm_members SET user_id = ? WHERE username = ? AND user_id IS NULL`, userID, username); err != nil {
			return fmt.Errorf("backfill npm member identity for %s: %w", username, err)
		}
	}
	if db.Dialect.Name() == "clickhouse" {
		return nil
	}
	indexQueries := []string{
		`CREATE UNIQUE INDEX uq_user_profiles_user_id ON user_profiles(user_id)`,
		`CREATE UNIQUE INDEX uq_maven_members_user_id ON maven_domain_members(repository, domain, user_id)`,
		`CREATE INDEX idx_maven_members_user ON maven_domain_members(user_id, repository)`,
		`CREATE INDEX idx_maven_artifacts_domain ON maven_artifacts(repository, domain, group_id, artifact_id)`,
		`CREATE INDEX idx_maven_artifacts_global_domain ON maven_artifacts(domain, repository)`,
		`CREATE INDEX idx_maven_versions_artifact ON maven_versions(repository, group_id, artifact_id, created_at)`,
		`CREATE INDEX idx_maven_invitations_recipient ON maven_domain_invitations(recipient, created_at)`,
		`CREATE UNIQUE INDEX uq_cargo_members_user_id ON cargo_members(repository, normalized_name, user_id)`,
		`CREATE UNIQUE INDEX uq_docker_members_user_id ON docker_members(repository, image_name, user_id)`,
		`CREATE UNIQUE INDEX uq_npm_members_user_id ON npm_members(repository, package_name, user_id)`,
	}
	for _, query := range indexQueries {
		if db.Dialect.Name() != "mysql" {
			query = strings.Replace(query, "CREATE UNIQUE INDEX", "CREATE UNIQUE INDEX IF NOT EXISTS", 1)
		}
		if _, err := db.Exec(query); err != nil {
			message := strings.ToLower(err.Error())
			if strings.Contains(message, "already exists") || strings.Contains(message, "duplicate key name") {
				continue
			}
			return fmt.Errorf("create stable membership identity index: %w", err)
		}
	}
	return nil
}
