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
	UpsertGPGPublicKeyQuery() string
	UpsertGPGSignatureQuery() string
	Rebind(query string) string
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
	{Name: "cargo_packages.repository_url", Query: "ALTER TABLE cargo_packages ADD COLUMN repository_url VARCHAR(1024) NOT NULL DEFAULT '';"},
	{Name: "cargo_packages.homepage", Query: "ALTER TABLE cargo_packages ADD COLUMN homepage VARCHAR(1024) NOT NULL DEFAULT '';"},
	{Name: "cargo_packages.documentation", Query: "ALTER TABLE cargo_packages ADD COLUMN documentation VARCHAR(1024) NOT NULL DEFAULT '';"},
	{Name: "cargo_versions.size", Query: "ALTER TABLE cargo_versions ADD COLUMN size BIGINT NOT NULL DEFAULT 0;"},
	{Name: "cargo_versions.checksum", Query: "ALTER TABLE cargo_versions ADD COLUMN checksum VARCHAR(64) NOT NULL DEFAULT '';"},
	{Name: "cargo_versions.rust_version", Query: "ALTER TABLE cargo_versions ADD COLUMN rust_version VARCHAR(64) NOT NULL DEFAULT '';"},
	{Name: "cargo_versions.license", Query: "ALTER TABLE cargo_versions ADD COLUMN license VARCHAR(255) NOT NULL DEFAULT '';"},
	{Name: "cargo_versions.repository_url", Query: "ALTER TABLE cargo_versions ADD COLUMN repository_url VARCHAR(1024) NOT NULL DEFAULT '';"},
	{Name: "cargo_versions.homepage", Query: "ALTER TABLE cargo_versions ADD COLUMN homepage VARCHAR(1024) NOT NULL DEFAULT '';"},
	{Name: "cargo_versions.documentation", Query: "ALTER TABLE cargo_versions ADD COLUMN documentation VARCHAR(1024) NOT NULL DEFAULT '';"},
	{Name: "docker_images.publisher", Query: "ALTER TABLE docker_images ADD COLUMN publisher VARCHAR(255) NOT NULL DEFAULT '';"},
	{Name: "docker_tags.publisher", Query: "ALTER TABLE docker_tags ADD COLUMN publisher VARCHAR(255) NOT NULL DEFAULT '';"},
	{Name: "docker_manifests.publisher", Query: "ALTER TABLE docker_manifests ADD COLUMN publisher VARCHAR(255) NOT NULL DEFAULT '';"},
}

func NewDialect(driver string) Dialect {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "mysql":
		return &MySQLDialect{}
	case "postgres", "postgresql", "pgx", "pg":
		return &PostgresDialect{}
	default:
		return &SQLiteDialect{}
	}
}
