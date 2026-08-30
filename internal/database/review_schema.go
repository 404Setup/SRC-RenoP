/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database

import "database/sql"

func initReviewTables(db *sql.DB, mysql bool) error {
	unique := "UNIQUE (active_key)"
	if mysql {
		unique = "UNIQUE KEY uq_review_tasks_active (active_key)"
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS review_tasks (
		id CHAR(36) PRIMARY KEY,
		kind VARCHAR(64) NOT NULL,
		resource_type VARCHAR(64) NOT NULL,
		repository VARCHAR(64) NOT NULL DEFAULT '',
		resource_key VARCHAR(1024) NOT NULL,
		resource_name VARCHAR(512) NOT NULL,
		source_team_prefix VARCHAR(64) NOT NULL DEFAULT '',
		target_team_prefix VARCHAR(64) NOT NULL DEFAULT '',
		review_team_prefix VARCHAR(64) NOT NULL,
		requested_by_id VARCHAR(36) NOT NULL,
		requested_by_name VARCHAR(255) NOT NULL,
		status VARCHAR(16) NOT NULL,
		decision_reason VARCHAR(512) NOT NULL DEFAULT '',
		decided_by_id VARCHAR(36) NOT NULL DEFAULT '',
		decided_by_name VARCHAR(255) NOT NULL DEFAULT '',
		created_at BIGINT NOT NULL,
		decided_at BIGINT NOT NULL DEFAULT 0,
		active_key CHAR(64) NULL,
		` + unique + `
	);`)
	return err
}
