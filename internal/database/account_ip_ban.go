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
	"math"
	"net/netip"
	"strings"
	"time"

	"renop/internal/core"
)

const maxAccountBanIPs = 64

func normalizedBanIP(ip string) string {
	address, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || address.IsUnspecified() || address.IsMulticast() {
		return ""
	}
	return address.WithZone("").Unmap().String()
}

func accountBanIPsTx(tx *Tx, userID, username string, retain bool) ([]string, error) {
	addresses := make([]string, 0, maxAccountBanIPs)
	seen := make(map[string]bool)
	collect := func(query string, args ...any) error {
		rows, err := tx.Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ip string
			if err := rows.Scan(&ip); err != nil {
				return err
			}
			ip = normalizedBanIP(ip)
			if ip != "" && !seen[ip] && len(addresses) < maxAccountBanIPs {
				seen[ip] = true
				addresses = append(addresses, ip)
			}
		}
		return rows.Err()
	}
	if retain {
		if err := collect(`SELECT ip FROM account_ip_bans WHERE user_id = ? ORDER BY ip LIMIT ?`, userID, maxAccountBanIPs); err != nil {
			return nil, fmt.Errorf("load current account IP restrictions: %w", err)
		}
	}
	if err := collect(`SELECT ip FROM sessions WHERE username = ? ORDER BY last_active DESC LIMIT ?`,
		username, maxAccountBanIPs); err != nil {
		return nil, fmt.Errorf("collect account session addresses: %w", err)
	}
	if err := collect(`SELECT ip FROM audit_logs WHERE username = ? AND operator = ? AND action = 'LOGIN'
        AND created_at >= ? ORDER BY created_at DESC, id DESC LIMIT ?`,
		username, username, time.Now().Add(-30*24*time.Hour).UnixMilli(), 256); err != nil {
		return nil, fmt.Errorf("collect recent successful login addresses: %w", err)
	}
	return addresses, nil
}

// GetAccountBanStatus returns the current suspension and its IP restriction count.
func (db *DB) GetAccountBanStatus(username string) (*core.AccountBanStatus, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	username = strings.ToLower(strings.TrimSpace(username))
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	token, err := tokenByNameTx(tx, username)
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, core.ErrUserProfileNotFound
	}
	if token.DeletedAt > 0 {
		return nil, core.ErrAccountDeleted
	}
	status := &core.AccountBanStatus{ProtectedRole: protectedAccountRole(token.Permissions)}
	if token.Ban.IsActive(time.Now().UnixMilli()) {
		status.Ban = token.Ban
		if err := tx.QueryRow(`SELECT COUNT(*) FROM account_ip_bans banned
            JOIN user_profiles profile ON profile.user_id = banned.user_id WHERE profile.username = ?`,
			username).Scan(&status.IPCount); err != nil {
			return nil, err
		}
	}
	return status, nil
}

// IsIPBanned checks all current suspensions that cover a normalized client address.
func (db *DB) IsIPBanned(ip string) (bool, error) {
	if db == nil || db.SQLDB == nil {
		return false, core.ErrDatabaseUnavailable
	}
	ip = normalizedBanIP(ip)
	if ip == "" {
		return false, nil
	}
	for range 3 {
		generation := db.ipBanCache.Generation()
		expiresAt, err := db.ipBanCache.GetOrLoad(ip, func() (int64, time.Duration, error) {
			var expires sql.NullInt64
			err := db.QueryRow(`SELECT MAX(CASE WHEN token.banned_until IS NULL THEN ? ELSE token.banned_until END)
                FROM account_ip_bans banned
                JOIN user_profiles profile ON profile.user_id = banned.user_id
                JOIN tokens token ON token.name = profile.username
                WHERE banned.ip = ? AND token.banned_at > 0
                AND (token.banned_until IS NULL OR token.banned_until > ?)`,
				int64(math.MaxInt64), ip, time.Now().UnixMilli()).Scan(&expires)
			if errors.Is(err, sql.ErrNoRows) {
				return 0, 0, nil
			}
			return expires.Int64, 0, err
		})
		if err != nil {
			return false, err
		}
		if generation == db.ipBanCache.Generation() {
			return expiresAt > time.Now().UnixMilli(), nil
		}
	}
	return false, core.ErrDatabaseUnavailable
}
