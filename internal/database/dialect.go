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
	"strings"
)

type Dialect interface {
	Name() string
	InitTables(db *sql.DB) error
	UpsertTokenQuery() string
	UpsertSessionQuery() string
}

type SchemaMigration struct {
	Name  string
	Query string
}

var sharedColumnMigrations = []SchemaMigration{
	{Name: "sessions.login_method", Query: "ALTER TABLE sessions ADD COLUMN login_method VARCHAR(64) NOT NULL DEFAULT 'password';"},
	{Name: "fido_devices.user_present", Query: "ALTER TABLE fido_devices ADD COLUMN user_present INT NOT NULL DEFAULT 0;"},
	{Name: "fido_devices.user_verified", Query: "ALTER TABLE fido_devices ADD COLUMN user_verified INT NOT NULL DEFAULT 0;"},
	{Name: "fido_devices.backup_eligible", Query: "ALTER TABLE fido_devices ADD COLUMN backup_eligible INT NOT NULL DEFAULT 0;"},
	{Name: "fido_devices.backup_state", Query: "ALTER TABLE fido_devices ADD COLUMN backup_state INT NOT NULL DEFAULT 0;"},
}

func NewDialect(driver string) Dialect {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "mysql":
		return &MySQLDialect{}
	default:
		return &SQLiteDialect{}
	}
}
