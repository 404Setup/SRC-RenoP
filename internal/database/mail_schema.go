/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"database/sql"
	"fmt"
	"strings"
)

func initMailTables(db *sql.DB, mysql bool) error {
	textType := "TEXT"
	if mysql {
		textType = "MEDIUMTEXT"
	}
	tables := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS mail_jobs (
			id VARCHAR(64) PRIMARY KEY, account_id VARCHAR(64) NOT NULL, user_id VARCHAR(36) NOT NULL,
			actor VARCHAR(64) NOT NULL, scene VARCHAR(40) NOT NULL, status VARCHAR(24) NOT NULL,
			payload %s NOT NULL, result_json TEXT NOT NULL, checks INT NOT NULL,
			created_at BIGINT NOT NULL, updated_at BIGINT NOT NULL, next_at BIGINT NOT NULL, expires_at BIGINT NOT NULL
		)`, textType),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS mail_accounts (id VARCHAR(64) PRIMARY KEY, payload %s NOT NULL, updated_at BIGINT NOT NULL)`, textType),
		`CREATE TABLE IF NOT EXISTS mail_rate_limits (ip VARCHAR(64) PRIMARY KEY, period_start BIGINT NOT NULL, used BIGINT NOT NULL, expires_at BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS mail_control (id VARCHAR(16) PRIMARY KEY, lease_owner VARCHAR(64) NOT NULL,
			lease_until BIGINT NOT NULL, next_send_at BIGINT NOT NULL, audit_cursor BIGINT NOT NULL, enabled_since BIGINT NOT NULL)`,
	}
	for _, query := range tables {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}
	for _, index := range []string{"CREATE INDEX idx_mail_jobs_due ON mail_jobs (status, next_at, created_at)", "CREATE INDEX idx_mail_jobs_owner ON mail_jobs (user_id, created_at)", "CREATE INDEX idx_mail_jobs_expiry ON mail_jobs (expires_at)", "CREATE INDEX idx_mail_rate_expiry ON mail_rate_limits (expires_at)"} {
		query := index
		if !mysql {
			query = strings.Replace(index, "CREATE INDEX ", "CREATE INDEX IF NOT EXISTS ", 1)
		}
		if _, err := db.Exec(query); err != nil {
			if mysql && strings.Contains(strings.ToLower(err.Error()), "duplicate key name") {
				continue
			}
			return err
		}
	}
	return nil
}
