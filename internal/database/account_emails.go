/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
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

	"renop/internal/core"
)

// reserveAccountEmailsTx requires the caller's account lock; the primary key arbitrates other owners.
func reserveAccountEmailsTx(tx *Tx, userID string, proofs []core.ProviderEmail, retained bool) (bool, error) {
	if len(proofs) == 0 {
		return false, nil
	}
	if len(proofs) > core.MaxAccountEmails {
		return false, core.ErrAccountEmailLimit
	}
	verified := make(map[string]bool, len(proofs))
	for _, proof := range proofs {
		email, valid := core.NormalizeEmail(proof.Email)
		if !valid || email == "" {
			return false, core.ErrEmailCodeInvalid
		}
		verified[email] = verified[email] || proof.Verified
	}
	emails := make([]string, 0, len(verified))
	for email := range verified {
		emails = append(emails, email)
	}
	slices.Sort(emails)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(emails)), ",")
	args := []any{userID}
	for _, email := range emails {
		args = append(args, email)
	}
	rows, err := tx.Query(`SELECT email, user_id, retained FROM user_email_addresses
        WHERE user_id = ? OR email IN (`+placeholders+`)`, args...)
	if err != nil {
		return false, err
	}
	owners, kept := make(map[string]string), make(map[string]bool)
	ownCount := 0
	for rows.Next() {
		var email, owner string
		var keep int32
		if err := rows.Scan(&email, &owner, &keep); err != nil {
			_ = rows.Close()
			return false, err
		}
		owners[email], kept[email] = owner, keep != 0
		if owner == userID {
			ownCount++
		}
	}
	err = rows.Err()
	if closeErr := rows.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	values, insertArgs, promote := []string{}, []any{}, []any{userID}
	for _, email := range emails {
		owner := owners[email]
		if owner != "" && owner != userID {
			return false, core.ErrEmailAlreadyExists
		}
		if !verified[email] && owner != userID {
			return false, core.ErrEmailVerificationRequired
		}
		if owner == "" {
			ownCount++
			keep := 0
			if retained {
				keep = 1
			}
			values = append(values, "(?, ?, ?)")
			insertArgs = append(insertArgs, email, userID, keep)
		} else if retained && !kept[email] {
			promote = append(promote, email)
		}
	}
	if ownCount > core.MaxAccountEmails {
		return false, core.ErrAccountEmailLimit
	}
	if len(values) > 0 {
		if _, err := tx.Exec(`INSERT INTO user_email_addresses (email, user_id, retained) VALUES `+strings.Join(values, ","), insertArgs...); err != nil {
			if uniqueConstraintError(err) {
				return false, core.ErrEmailAlreadyExists
			}
			return false, err
		}
	}
	if len(promote) > 1 {
		marks := strings.TrimSuffix(strings.Repeat("?,", len(promote)-1), ",")
		if _, err := tx.Exec(`UPDATE user_email_addresses SET retained = 1 WHERE user_id = ? AND email IN (`+marks+`)`, promote...); err != nil {
			return false, err
		}
	}
	return len(values) > 0 || len(promote) > 1, nil
}

func (db *DB) migrateAccountEmails() error {
	var conflicts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_account_security security
        JOIN user_email_addresses address ON address.email = security.email
        WHERE security.user_id <> address.user_id`).Scan(&conflicts); err != nil {
		return err
	}
	if conflicts > 0 {
		return core.ErrEmailAlreadyExists
	}
	after := ""
	for {
		rows, err := db.Query(`SELECT security.email, security.user_id FROM user_account_security security
            LEFT JOIN user_email_addresses address ON address.email = security.email
			WHERE security.email > ? AND address.email IS NULL ORDER BY security.email LIMIT 256`, after)
		if err != nil {
			return err
		}
		values, args := []string{}, []any{}
		for rows.Next() {
			var email, userID string
			if err := rows.Scan(&email, &userID); err != nil {
				_ = rows.Close()
				return err
			}
			values = append(values, "(?, ?, 0)")
			args = append(args, email, userID)
			after = email
		}
		err = rows.Err()
		if closeErr := rows.Close(); err == nil {
			err = closeErr
		}
		if err != nil || len(values) == 0 {
			return err
		}
		if _, err := db.Exec(`INSERT INTO user_email_addresses (email, user_id, retained) VALUES `+strings.Join(values, ","), args...); err != nil {
			return err
		}
	}
}

func (db *DB) accountEmailAliases(userID, primary string) ([]string, error) {
	rows, err := db.Query(`SELECT email FROM user_email_addresses WHERE user_id = ? AND email <> ? ORDER BY email LIMIT ?`,
		userID, primary, core.MaxAccountEmails)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	emails := []string{}
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

// DeleteAccountEmailAlias removes an owned secondary address without changing the primary address.
func (db *DB) DeleteAccountEmailAlias(username, session, email string, now int64) (*core.AccountSecurity, error) {
	email, valid := core.NormalizeEmail(email)
	if !valid || email == "" || now <= 0 {
		return nil, core.ErrEmailCodeInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	account, err := accountEmailSessionTx(tx, strings.ToLower(username), session)
	if err != nil {
		return nil, err
	}
	var primary string
	if err := tx.QueryRow(`SELECT COALESCE(email, '') FROM user_account_security WHERE user_id = ?`, account.UserID).Scan(&primary); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if email == primary {
		return nil, core.ErrPrimaryEmail
	}
	if _, err := tx.Exec(`DELETE FROM user_email_addresses WHERE user_id = ? AND email = ?`, account.UserID, email); err != nil {
		return nil, err
	}
	if err := touchAccountSecurityTx(tx, account.UserID, now); err != nil {
		return nil, fmt.Errorf("invalidate removed email proofs: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return db.GetAccountSecurity(username)
}
