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

	"renop/core"
)

const (
	maxSessionTokenLen = 512
	maxUsernameLen     = 255
)

func (db *DB) GetSession(sessionToken string) (*core.Session, error) {
	if db == nil || db.SqlDB == nil || sessionToken == "" {
		return nil, nil
	}
	if len(sessionToken) > maxSessionTokenLen {
		return nil, nil
	}

	if sess, ok := db.sessionCache.Get(sessionToken); ok {
		return sess, nil
	}

	query := `SELECT public_id, username, ip, user_agent, created_at, last_active FROM sessions WHERE session_token = ?`
	row := db.SqlDB.QueryRow(query, sessionToken)

	var publicId, username, ip, userAgent string
	var createdAt, lastActive int64

	err := row.Scan(&publicId, &username, &ip, &userAgent, &createdAt, &lastActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			db.sessionCache.Set(sessionToken, nil, 30*time.Second)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query session (%s...): %w", safePrefix(sessionToken, 8), err)
	}

	session := &core.Session{
		PublicId:  publicId,
		Username:  strings.ToLower(username),
		Ip:        ip,
		UserAgent: userAgent,
		CreatedAt: createdAt,
	}
	session.LastActive.Store(lastActive)

	db.sessionCache.Set(sessionToken, session, 15*time.Minute)
	return session, nil
}

func (db *DB) SaveSession(session *core.Session, sessionToken string) error {
	if db == nil || db.SqlDB == nil || session == nil || sessionToken == "" {
		return nil
	}

	lastActive := session.LastActive.Load()
	query := db.Dialect.UpsertSessionQuery()
	_, err := db.SqlDB.Exec(query, sessionToken, session.PublicId, strings.ToLower(session.Username), session.Ip, session.UserAgent, session.CreatedAt, lastActive)
	if err != nil {
		return fmt.Errorf("failed to save session (%s): %w", sessionToken, err)
	}

	db.sessionCache.Set(sessionToken, session, 15*time.Minute)
	return nil
}

func (db *DB) UpdateSessionLastActive(sessionToken string, lastActive int64) error {
	if db == nil || db.SqlDB == nil || sessionToken == "" {
		return nil
	}
	if len(sessionToken) > maxSessionTokenLen {
		return nil
	}

	if sess, ok := db.sessionCache.Get(sessionToken); ok {
		sess.LastActive.Store(lastActive)
	}

	_, err := db.SqlDB.Exec(`UPDATE sessions SET last_active = ? WHERE session_token = ?`, lastActive, sessionToken)
	if err != nil {
		return fmt.Errorf("failed to update session last_active (%s...): %w", safePrefix(sessionToken, 8), err)
	}
	return nil
}

func (db *DB) DeleteSession(sessionToken string) error {
	if db == nil || db.SqlDB == nil || sessionToken == "" {
		return nil
	}

	_, err := db.SqlDB.Exec(`DELETE FROM sessions WHERE session_token = ?`, sessionToken)
	if err != nil {
		return fmt.Errorf("failed to delete session (%s): %w", sessionToken, err)
	}

	db.sessionCache.Delete(sessionToken)
	return nil
}

func (db *DB) DeleteSessionsByUsername(username string) error {
	if db == nil || db.SqlDB == nil || username == "" {
		return nil
	}
	if len(username) > maxUsernameLen {
		return nil
	}

	lowerName := strings.ToLower(username)
	_, err := db.SqlDB.Exec(`DELETE FROM sessions WHERE username = ?`, lowerName)
	if err != nil {
		return fmt.Errorf("failed to delete sessions for user (%s): %w", lowerName, err)
	}

	db.sessionCache.DeleteFunc(func(_ string, sess *core.Session) bool {
		return sess == nil || strings.EqualFold(sess.Username, lowerName)
	})
	return nil
}

func (db *DB) ListUserSessions(username, currentSessionToken string) ([]core.SessionDto, error) {
	if db == nil || db.SqlDB == nil || username == "" {
		return []core.SessionDto{}, nil
	}

	lowerName := strings.ToLower(username)
	query := `SELECT session_token, public_id, username, ip, user_agent, created_at, last_active FROM sessions WHERE username = ?`
	rows, err := db.SqlDB.Query(query, lowerName)
	if err != nil {
		return nil, fmt.Errorf("failed to list user sessions (%s): %w", lowerName, err)
	}
	defer rows.Close()

	var sessions []core.SessionDto
	for rows.Next() {
		var token, publicId, u, ip, userAgent string
		var createdAt, lastActive int64

		if err := rows.Scan(&token, &publicId, &u, &ip, &userAgent, &createdAt, &lastActive); err != nil {
			return nil, fmt.Errorf("failed to scan session row: %w", err)
		}

		sessions = append(sessions, core.SessionDto{
			PublicId:   publicId,
			Username:   strings.ToLower(u),
			Ip:         ip,
			UserAgent:  userAgent,
			CreatedAt:  createdAt,
			LastActive: lastActive,
			ExpiresAt:  lastActive + core.SessionIdleTimeoutMillis,
			Current:    token != "" && token == currentSessionToken,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if sessions == nil {
		return []core.SessionDto{}, nil
	}

	return sessions, nil
}

func (db *DB) DeleteExpiredSessions(minActiveTimestamp int64) error {
	if db == nil || db.SqlDB == nil {
		return nil
	}

	_, err := db.SqlDB.Exec(`DELETE FROM sessions WHERE last_active < ?`, minActiveTimestamp)
	if err != nil {
		return fmt.Errorf("failed to delete expired sessions: %w", err)
	}
	db.sessionCache.EvictExpired()
	return nil
}

func (db *DB) MigrateSessions(sessions []core.SessionDbDto) error {
	if db == nil || db.SqlDB == nil || len(sessions) == 0 {
		return nil
	}

	tx, err := db.SqlDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := db.Dialect.UpsertSessionQuery()
	for _, dto := range sessions {
		if _, err := tx.Exec(query, dto.SessionToken, dto.PublicId, strings.ToLower(dto.Username), dto.Ip, dto.UserAgent, dto.CreatedAt, dto.LastActive); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) DeleteUserSessionByPublicID(username, publicID, currentSessionToken string) (string, bool, bool, error) {
	if db == nil || db.SqlDB == nil || username == "" || publicID == "" {
		return "", false, false, nil
	}
	if len(username) > maxUsernameLen {
		return "", false, false, nil
	}

	lowerName := strings.ToLower(username)
	var sessionToken string
	err := db.SqlDB.QueryRow(`SELECT session_token FROM sessions WHERE username = ? AND public_id = ?`, lowerName, publicID).Scan(&sessionToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, false, nil
		}
		return "", false, false, fmt.Errorf("failed to query session by public id: %w", err)
	}

	wasCurrent := currentSessionToken != "" && sessionToken == currentSessionToken
	if err := db.DeleteSession(sessionToken); err != nil {
		return sessionToken, false, wasCurrent, err
	}
	return sessionToken, true, wasCurrent, nil
}

func (db *DB) DeleteOtherUserSessions(username, keepSessionToken string) ([]string, error) {
	if db == nil || db.SqlDB == nil || username == "" {
		return nil, nil
	}
	if len(username) > maxUsernameLen {
		return nil, nil
	}

	lowerName := strings.ToLower(username)
	var query string
	var args []any
	if keepSessionToken == "" {
		query = `SELECT session_token FROM sessions WHERE username = ?`
		args = []any{lowerName}
	} else {
		query = `SELECT session_token FROM sessions WHERE username = ? AND session_token != ?`
		args = []any{lowerName, keepSessionToken}
	}

	rows, err := db.SqlDB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions for user (%s): %w", lowerName, err)
	}
	defer rows.Close()

	var tokens []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err == nil {
			tokens = append(tokens, t)
		}
	}

	if len(tokens) == 0 {
		return nil, nil
	}

	if keepSessionToken == "" {
		_, err = db.SqlDB.Exec(`DELETE FROM sessions WHERE username = ?`, lowerName)
	} else {
		_, err = db.SqlDB.Exec(`DELETE FROM sessions WHERE username = ? AND session_token != ?`, lowerName, keepSessionToken)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to delete sessions for user (%s): %w", lowerName, err)
	}

	for _, t := range tokens {
		db.sessionCache.Delete(t)
	}
	return tokens, nil
}

