/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database

import "database/sql"

func initReviewTables(db *sql.DB, mysql bool) error {
	unique := "UNIQUE (active_key)"
	payloadType := "TEXT"
	if mysql {
		unique = "UNIQUE KEY uq_review_tasks_active (active_key)"
		payloadType = "MEDIUMTEXT"
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS review_tasks (
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
	);`); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS review_task_files (
		task_id CHAR(36) NOT NULL,
		file_id CHAR(64) NOT NULL,
		path VARCHAR(1024) NOT NULL,
		size BIGINT NOT NULL DEFAULT 0,
		critical INT NOT NULL DEFAULT 0,
		added_at BIGINT NOT NULL,
		PRIMARY KEY (task_id, file_id)
	);`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS review_task_payloads (
		task_id CHAR(36) PRIMARY KEY,
		payload_json ` + payloadType + ` NOT NULL
	);`)
	return err
}
