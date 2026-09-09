/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import "database/sql"

func initTicketTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS ticket_state (
		task_id CHAR(36) PRIMARY KEY,
		status VARCHAR(16) NOT NULL DEFAULT 'unprocessed',
		title VARCHAR(160) NOT NULL DEFAULT '', body TEXT NOT NULL,
		assignee_id VARCHAR(36) NOT NULL DEFAULT '', assignee_admin INT NOT NULL DEFAULT 0,
		admin_only INT NOT NULL DEFAULT 0, escalations INT NOT NULL DEFAULT 0,
		escalated_by_id VARCHAR(36) NOT NULL DEFAULT '', revision BIGINT NOT NULL DEFAULT 0,
		target_user_ids TEXT NOT NULL, outcome VARCHAR(16) NOT NULL DEFAULT '',
		response TEXT NOT NULL, changed_at BIGINT NOT NULL DEFAULT 0
	);`)
	return err
}
