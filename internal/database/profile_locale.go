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

	"renop/internal/core"
	"renop/internal/locale"
)

// SetUserLocale saves a private preference without changing credential revisions.
func (db *DB) SetUserLocale(username, session, language, userID string) error {
	code := locale.Match(language)
	if code == "" {
		return locale.ErrUnsupported
	}
	username = strings.ToLower(strings.TrimSpace(username))
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	account, err := accountEmailSessionTx(tx, username, session)
	if err != nil {
		return err
	}
	if account.UserID != userID {
		return core.ErrUserProfileNotFound
	}
	if _, err = tx.Exec(`UPDATE user_profiles SET locale = ? WHERE user_id = ? AND locale <> ?`, code, account.UserID, code); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// GetEmailLocale resolves a retained login address without exposing account identity.
func (db *DB) GetEmailLocale(email string) (string, error) {
	email, valid := core.NormalizeEmail(email)
	if !valid || email == "" {
		return "", nil
	}
	var code string
	err := db.QueryRow(`SELECT profile.locale FROM user_email_addresses address
		JOIN user_profiles profile ON profile.user_id = address.user_id WHERE address.email = ?`, email).Scan(&code)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return code, err
}
