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
	"strings"
	"time"

	"renop/core"
)

func (db *DB) SaveAuditLog(entry *core.AuditLogEntry) error {
	if db == nil || db.SqlDB == nil || entry == nil {
		return nil
	}
	query := `INSERT INTO audit_logs (username, operator, action, details, auth_method, session_id, ip, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := db.SqlDB.Exec(query,
		strings.ToLower(entry.Username),
		strings.ToLower(entry.Operator),
		entry.Action,
		entry.Details,
		entry.AuthMethod,
		entry.SessionID,
		entry.IP,
		entry.CreatedAt,
	)
	return err
}

func (db *DB) GetAuditLogs(username string, limit, offset int) ([]*core.AuditLogEntry, int, error) {
	if db == nil || db.SqlDB == nil {
		return []*core.AuditLogEntry{}, 0, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	lowerUser := strings.ToLower(strings.TrimSpace(username))
	var total int
	var countQuery string
	var args []any

	if lowerUser != "" {
		countQuery = "SELECT COUNT(*) FROM audit_logs WHERE username = ?"
		args = append(args, lowerUser)
	} else {
		countQuery = "SELECT COUNT(*) FROM audit_logs"
	}

	err := db.SqlDB.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return []*core.AuditLogEntry{}, 0, err
	}

	var selectQuery string
	var selectArgs []any
	if lowerUser != "" {
		selectQuery = `SELECT id, username, operator, action, details, auth_method, session_id, ip, created_at
		FROM audit_logs WHERE username = ? ORDER BY id DESC LIMIT ? OFFSET ?`
		selectArgs = append(selectArgs, lowerUser, limit, offset)
	} else {
		selectQuery = `SELECT id, username, operator, action, details, auth_method, session_id, ip, created_at
		FROM audit_logs ORDER BY id DESC LIMIT ? OFFSET ?`
		selectArgs = append(selectArgs, limit, offset)
	}

	rows, err := db.SqlDB.Query(selectQuery, selectArgs...)
	if err != nil {
		return []*core.AuditLogEntry{}, 0, err
	}
	defer rows.Close()

	entries := make([]*core.AuditLogEntry, 0, limit)
	for rows.Next() {
		e := &core.AuditLogEntry{}
		if err := rows.Scan(&e.ID, &e.Username, &e.Operator, &e.Action, &e.Details, &e.AuthMethod, &e.SessionID, &e.IP, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}

	return entries, total, nil
}

func (db *DB) DeleteAuditLogsByUsername(username string) error {
	if db == nil || db.SqlDB == nil {
		return nil
	}
	lowerUser := strings.ToLower(strings.TrimSpace(username))
	if lowerUser == "" {
		return nil
	}
	_, err := db.SqlDB.Exec("DELETE FROM audit_logs WHERE username = ?", lowerUser)
	return err
}

func (db *DB) CleanExpiredAuditLogs(retentionDays int, maxRows int) error {
	if db == nil || db.SqlDB == nil {
		return nil
	}
	if retentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -retentionDays).UnixMilli()
		_, _ = db.SqlDB.Exec("DELETE FROM audit_logs WHERE created_at < ?", cutoff)
	}

	if maxRows > 0 {
		trimQuery := `DELETE FROM audit_logs WHERE id < (
			SELECT min_id FROM (
				SELECT MIN(id) AS min_id FROM (
					SELECT id FROM audit_logs ORDER BY id DESC LIMIT ?
				) AS t1
			) AS t2
		)`
		_, _ = db.SqlDB.Exec(trimQuery, maxRows)
	}

	return nil
}
