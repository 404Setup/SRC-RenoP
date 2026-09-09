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
	"path/filepath"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/testutil"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLegacyAuditMigrationAndSeparateRetention(t *testing.T) {
	path := filepath.Join(testutil.TempDir(t), "legacy-audit.db")
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = raw.Exec(`CREATE TABLE audit_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL,
        operator TEXT NOT NULL, action TEXT NOT NULL, details TEXT NOT NULL, auth_method TEXT NOT NULL,
        session_id TEXT NOT NULL DEFAULT '', ip TEXT NOT NULL, created_at BIGINT NOT NULL)`)
	require.NoError(t, err)
	now := time.Now().UnixMilli()
	_, err = raw.Exec(`INSERT INTO audit_logs (username, operator, action, details, auth_method, ip, created_at)
        VALUES ('alice', 'alice', 'LOGIN', 'Legacy row', 'Web', '', ?)`, now)
	require.NoError(t, err)
	require.NoError(t, raw.Close())
	db, err := InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: path, MaxOpenConns: 2})
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	entries, total, err := db.GetAuditLogs("alice", 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "audit", entries[0].Kind)
	require.Equal(t, "unknown", entries[0].Trigger)
	for i := range 3 {
		require.NoError(t, db.SaveAuditLog(&core.AuditLogEntry{Kind: "system", Operator: "system", Action: "SYSTEM_LOG", CreatedAt: now + int64(i)}))
	}
	require.NoError(t, db.CleanExpiredAuditLogs(14, 1))
	_, total, err = db.GetAuditLogs("alice", 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total, "system noise cannot evict the activity row")
	_, total, err = db.FilterAuditLogs(core.AuditLogFilter{Kind: "system"}, 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
}
