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
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"renop/internal/config"
	"renop/internal/core"
)

// ponytail: avatar writes are rare; shard this lock by user only if profile write throughput matters.
var avatarMutationLock sync.Mutex

func initUserAvatarTable(db *sql.DB, binaryType string) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS user_avatars (
		user_id VARCHAR(36) PRIMARY KEY,
		content_type VARCHAR(32) NOT NULL,
		image_data ` + binaryType + ` NOT NULL,
		size BIGINT NOT NULL,
		sha256 CHAR(64) NOT NULL,
		updated_at BIGINT NOT NULL
	)`)
	return err
}

func validUserAvatar(avatar *core.UserAvatar) bool {
	if avatar == nil || avatar.Size <= 0 || avatar.Size != int64(len(avatar.Data)) ||
		avatar.Size > int64(config.MaxAvatarMaxSizeBytes) || avatar.UpdatedAt <= 0 ||
		avatar.ContentType != "image/png" && avatar.ContentType != "image/jpeg" || len(avatar.SHA256) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(avatar.SHA256)
	if err != nil || len(decoded) != sha256.Size {
		return false
	}
	sum := sha256.Sum256(avatar.Data)
	return strings.EqualFold(hex.EncodeToString(sum[:]), avatar.SHA256)
}

// GetUserAvatar loads and verifies one sanitized avatar by current username.
func (db *DB) GetUserAvatar(username string) (*core.UserAvatar, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	avatar := &core.UserAvatar{}
	err := db.QueryRow(`SELECT a.user_id, a.content_type, a.image_data, a.size, a.sha256, a.updated_at
		FROM user_avatars a JOIN user_profiles p ON p.user_id = a.user_id WHERE p.username = ?`, username).Scan(
		&avatar.UserID, &avatar.ContentType, &avatar.Data, &avatar.Size, &avatar.SHA256, &avatar.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrUserAvatarNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load user avatar %s: %w", username, err)
	}
	if !validUserAvatar(avatar) {
		return nil, errors.New("stored user avatar is invalid")
	}
	return avatar, nil
}

// PutUserAvatar atomically replaces one account avatar through its immutable user ID.
func (db *DB) PutUserAvatar(username string, avatar *core.UserAvatar) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	if username == "" || !validUserAvatar(avatar) {
		return errors.New("user avatar is invalid")
	}
	avatarMutationLock.Lock()
	defer avatarMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin user avatar update: %w", err)
	}
	defer tx.Rollback()
	userID, err := userIDForUsernameTx(tx, username)
	if err != nil {
		return err
	}
	result, err := tx.Exec(`UPDATE user_avatars SET content_type = ?, image_data = ?, size = ?, sha256 = ?,
		updated_at = ? WHERE user_id = ?`, avatar.ContentType, avatar.Data, avatar.Size,
		strings.ToLower(avatar.SHA256), avatar.UpdatedAt, userID)
	if err != nil {
		return fmt.Errorf("update user avatar: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect user avatar update: %w", err)
	}
	if changed == 0 {
		var exists int
		err = tx.QueryRow(`SELECT 1 FROM user_avatars WHERE user_id = ?`, userID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.Exec(`INSERT INTO user_avatars
				(user_id, content_type, image_data, size, sha256, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
				userID, avatar.ContentType, avatar.Data, avatar.Size,
				strings.ToLower(avatar.SHA256), avatar.UpdatedAt); err != nil {
				return fmt.Errorf("create user avatar: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("inspect existing user avatar: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit user avatar update: %w", err)
	}
	db.invalidateUserProfileCaches(username)
	return nil
}

// DeleteUserAvatar removes one account avatar if present.
func (db *DB) DeleteUserAvatar(username string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	if username == "" {
		return core.ErrUserProfileNotFound
	}
	avatarMutationLock.Lock()
	defer avatarMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin user avatar deletion: %w", err)
	}
	defer tx.Rollback()
	userID, err := userIDForUsernameTx(tx, username)
	if err != nil {
		return err
	}
	result, err := tx.Exec(`DELETE FROM user_avatars WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("delete user avatar %s: %w", username, err)
	}
	if _, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("inspect user avatar deletion: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit user avatar deletion: %w", err)
	}
	db.invalidateUserProfileCaches(username)
	return nil
}
