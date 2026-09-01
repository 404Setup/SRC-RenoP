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

// SetAccountBan replaces one account's durable suspension. A nil ban clears it.
func (db *DB) SetAccountBan(username string, ban *core.AccountBan) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	username = strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
	if username == "" {
		return core.ErrUserProfileNotFound
	}
	if ban != nil {
		reason, valid := core.NormalizeAccountBanReason(ban.Reason)
		if !valid || ban.CreatedAt <= 0 || (ban.ExpiresAt != nil && *ban.ExpiresAt <= ban.CreatedAt) {
			return core.ErrAccountBanInvalid
		}
		ban = ban.Clone()
		ban.Reason = reason
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin account ban update: %w", err)
	}
	defer tx.Rollback()
	var userID string
	if err := tx.QueryRow(`SELECT user_id FROM user_profiles WHERE username = ?`, username).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.ErrUserProfileNotFound
		}
		return fmt.Errorf("resolve account before ban update: %w", err)
	}
	if err := lockAccountLoginMethodsTx(tx, userID); err != nil {
		return fmt.Errorf("lock account before ban update: %w", err)
	}
	token, err := tokenByNameTx(tx, username)
	if err != nil {
		return fmt.Errorf("reload account before ban update: %w", err)
	}
	if token == nil {
		return core.ErrUserProfileNotFound
	}
	token.Ban = ban
	if err := db.saveTokenInTx(tx, username, token); err != nil {
		return err
	}
	if ban != nil {
		if _, err := tx.Exec(`DELETE FROM sessions WHERE username = ?`, username); err != nil {
			return fmt.Errorf("revoke sessions during account ban: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit account ban update: %w", err)
	}
	db.finishTokenUpdate(username, token)
	if ban != nil {
		db.sessionCache.DeleteFunc(func(_ string, session *core.Session) bool {
			return session == nil || strings.EqualFold(session.Username, username)
		})
	}
	return nil
}
