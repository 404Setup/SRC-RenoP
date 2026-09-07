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

	"renop/internal/core"
)

func (db *DB) SaveAuditLog(entry *core.AuditLogEntry) error {
	if db == nil || db.SQLDB == nil || entry == nil {
		return nil
	}
	db.auditWriteMu.Lock()
	defer db.auditWriteMu.Unlock()
	username := SanitizeInputString(strings.ToLower(entry.Username), 255)
	operator := SanitizeInputString(strings.ToLower(entry.Operator), 255)
	initiator := SanitizeInputString(strings.ToLower(entry.Initiator), 255)
	if initiator == "" {
		initiator = operator
	}
	kind, severity := entry.Kind, entry.Severity
	if kind == "" {
		kind = "audit"
	}
	if severity == "" {
		severity = "info"
	}
	query := `INSERT INTO audit_logs (username, operator, action, details, auth_method, session_id, ip, created_at, kind, trigger_source, severity, initiator)
	SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? WHERE NOT EXISTS (
		SELECT 1 FROM tokens WHERE (name = ? OR name = ? OR name = ?) AND deleted_at > 0
		AND (audit_purged_at > 0 OR deleted_at <= ?))`
	_, err := db.Exec(query,
		username, operator,
		SanitizeInputString(entry.Action, 64),
		SanitizeInputString(entry.Details, 4096),
		SanitizeInputString(entry.AuthMethod, 64),
		SanitizeInputString(core.SafeAuditSessionID(entry.SessionID), 255),
		SanitizeInputString(entry.IP, 255),
		entry.CreatedAt, SanitizeInputString(kind, 16), SanitizeInputString(core.AuditTrigger(entry), 64), SanitizeInputString(severity, 16), initiator,
		username, operator, initiator, time.Now().UnixMilli()-core.AccountAuditRetentionMillis,
	)
	return err
}

// GetAuditLogs returns activity records in their existing account scope.
func (db *DB) GetAuditLogs(username string, limit, offset int) ([]*core.AuditLogEntry, int, error) {
	return db.FilterAuditLogs(core.AuditLogFilter{Username: username, Kind: "audit"}, limit, offset)
}

// FilterAuditLogs applies bounded, parameterized filters to activity and system records.
func (db *DB) FilterAuditLogs(filter core.AuditLogFilter, limit, offset int) ([]*core.AuditLogEntry, int, error) {
	if db == nil || db.SQLDB == nil {
		return []*core.AuditLogEntry{}, 0, nil
	}
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 500)
	offset = max(0, min(offset, 1000000))
	conditions := []string{"1 = 1"}
	args := []any{}
	for _, field := range []struct{ column, value string }{
		{"username", strings.ToLower(strings.TrimSpace(filter.Username))},
		{"operator", strings.ToLower(strings.TrimSpace(filter.Operator))},
		{"COALESCE(NULLIF(initiator, ''), operator)", strings.ToLower(strings.TrimSpace(filter.Initiator))},
		{"kind", filter.Kind}, {"action", filter.Action},
		{"trigger_source", filter.Trigger}, {"severity", filter.Severity},
	} {
		if field.value != "" {
			conditions = append(conditions, field.column+" = ?")
			args = append(args, field.value)
		}
	}
	if filter.ExcludeOperator != "" {
		conditions = append(conditions, "operator <> ?")
		args = append(args, strings.ToLower(filter.ExcludeOperator))
	}
	if filter.ExcludeInitiator != "" {
		conditions = append(conditions, "COALESCE(NULLIF(initiator, ''), operator) <> ?")
		args = append(args, strings.ToLower(filter.ExcludeInitiator))
	}
	if filter.From > 0 {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, filter.From)
	}
	if filter.Until > 0 {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, filter.Until)
	}
	where := strings.Join(conditions, " AND ")
	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 || offset >= total {
		return []*core.AuditLogEntry{}, total, nil
	}
	rows, err := db.Query(`SELECT id, username, operator, action, details, auth_method, session_id, ip, created_at,
		kind, trigger_source, severity, COALESCE(NULLIF(initiator, ''), operator) FROM audit_logs WHERE `+where+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	entries := make([]*core.AuditLogEntry, 0, min(total-offset, limit))
	for rows.Next() {
		e := &core.AuditLogEntry{}
		if err := rows.Scan(&e.ID, &e.Username, &e.Operator, &e.Action, &e.Details, &e.AuthMethod, &e.SessionID,
			&e.IP, &e.CreatedAt, &e.Kind, &e.Trigger, &e.Severity, &e.Initiator); err != nil {
			return nil, 0, err
		}
		e.SessionID = core.SafeAuditSessionID(e.SessionID)
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

func (db *DB) DeleteAuditLogsByUsername(username string) error {
	if db == nil || db.SQLDB == nil {
		return nil
	}
	lowerUser := SanitizeInputString(strings.ToLower(strings.TrimSpace(username)), 255)
	if lowerUser == "" {
		return nil
	}
	_, err := db.Exec("DELETE FROM audit_logs WHERE username = ? AND kind = 'audit'", lowerUser)
	return err
}

func (db *DB) CleanExpiredAuditLogs(retentionDays int, maxRows int) error {
	if db == nil || db.SQLDB == nil {
		return nil
	}
	if err := db.cleanExpiredLogKind("audit", retentionDays, maxRows); err != nil {
		return err
	}
	if retentionDays <= 0 {
		retentionDays = 30
	}
	if maxRows <= 0 {
		maxRows = 10000
	}
	return db.cleanExpiredLogKind("system", min(retentionDays, 30), min(maxRows, 10000))
}

func (db *DB) cleanExpiredLogKind(kind string, retentionDays, maxRows int) error {
	now := time.Now()
	retirementCutoff := now.UnixMilli() - core.AccountAuditRetentionMillis
	unprotected := `kind = '` + kind + `' AND username NOT IN (SELECT name FROM tokens
		WHERE deleted_at > ? AND audit_purged_at = 0) AND operator NOT IN (SELECT name FROM tokens
		WHERE deleted_at > ? AND audit_purged_at = 0) AND initiator NOT IN (SELECT name FROM tokens
		WHERE deleted_at > ? AND audit_purged_at = 0)`
	if retentionDays > 0 {
		cutoff := now.AddDate(0, 0, -retentionDays).UnixMilli()
		if _, err := db.Exec("DELETE FROM audit_logs WHERE created_at < ? AND "+unprotected,
			cutoff, retirementCutoff, retirementCutoff, retirementCutoff); err != nil {
			return err
		}
	}

	if maxRows > 0 && maxRows < 100000000 {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE "+unprotected,
			retirementCutoff, retirementCutoff, retirementCutoff).Scan(&count); err != nil {
			return err
		}
		if count > maxRows {
			trimQuery := `DELETE FROM audit_logs WHERE ` + unprotected + ` AND id < (
				SELECT min_id FROM (
					SELECT MIN(id) AS min_id FROM (
						SELECT id FROM audit_logs WHERE ` + unprotected + ` ORDER BY id DESC LIMIT ?
					) AS t1
				) AS t2
			)`
			if _, err := db.Exec(trimQuery, retirementCutoff, retirementCutoff, retirementCutoff,
				retirementCutoff, retirementCutoff, retirementCutoff, maxRows); err != nil {
				return err
			}
		}
	}

	return nil
}
