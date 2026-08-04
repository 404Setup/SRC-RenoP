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

import "database/sql"

type MySQLDialect struct{}

func (d *MySQLDialect) Name() string {
	return "mysql"
}

func (d *MySQLDialect) InitTables(db *sql.DB) error {
	tokensTable := `
	CREATE TABLE IF NOT EXISTS tokens (
		name VARCHAR(255) PRIMARY KEY,
		type VARCHAR(64) NOT NULL,
		type_value INT NOT NULL,
		encrypted_secret TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		tokens_json TEXT NOT NULL,
		created_at VARCHAR(64) NOT NULL,
		description TEXT NOT NULL,
		expires_at BIGINT NULL,
		permissions_json TEXT NOT NULL,
		INDEX idx_tokens_expires_at (expires_at)
	);`

	sessionsTable := `
	CREATE TABLE IF NOT EXISTS sessions (
		session_token VARCHAR(512) PRIMARY KEY,
		public_id VARCHAR(255) NOT NULL,
		username VARCHAR(255) NOT NULL,
		ip VARCHAR(255) NOT NULL,
		user_agent TEXT NOT NULL,
		created_at BIGINT NOT NULL,
		last_active BIGINT NOT NULL,
		login_method VARCHAR(64) NOT NULL DEFAULT 'password',
		INDEX idx_sessions_username (username),
		INDEX idx_sessions_last_active (last_active)
	);`

	fidoTable := `
	CREATE TABLE IF NOT EXISTS fido_devices (
		id VARCHAR(255) PRIMARY KEY,
		username VARCHAR(255) NOT NULL,
		name VARCHAR(255) NOT NULL,
		credential_id VARBINARY(512) NOT NULL,
		public_key BLOB NOT NULL,
		attestation_type VARCHAR(64) NOT NULL,
		aaguid VARBINARY(64) NOT NULL,
		sign_count INT NOT NULL,
		created_at BIGINT NOT NULL,
		user_present INT NOT NULL DEFAULT 0,
		user_verified INT NOT NULL DEFAULT 0,
		backup_eligible INT NOT NULL DEFAULT 0,
		backup_state INT NOT NULL DEFAULT 0,
		INDEX idx_fido_username (username),
		INDEX idx_fido_credential_id (credential_id)
	);`

	if _, err := db.Exec(tokensTable); err != nil {
		return err
	}
	if _, err := db.Exec(sessionsTable); err != nil {
		return err
	}
	if _, err := db.Exec(fidoTable); err != nil {
		return err
	}

	_ = execIgnoreDuplicateColumn(db, "ALTER TABLE sessions ADD COLUMN login_method VARCHAR(64) NOT NULL DEFAULT 'password';")
	_ = execIgnoreDuplicateColumn(db, "ALTER TABLE fido_devices ADD COLUMN user_present INT NOT NULL DEFAULT 0;")
	_ = execIgnoreDuplicateColumn(db, "ALTER TABLE fido_devices ADD COLUMN user_verified INT NOT NULL DEFAULT 0;")
	_ = execIgnoreDuplicateColumn(db, "ALTER TABLE fido_devices ADD COLUMN backup_eligible INT NOT NULL DEFAULT 0;")
	_ = execIgnoreDuplicateColumn(db, "ALTER TABLE fido_devices ADD COLUMN backup_state INT NOT NULL DEFAULT 0;")

	return nil
}

func (d *MySQLDialect) UpsertTokenQuery() string {
	return `INSERT INTO tokens (name, type, type_value, encrypted_secret, password_hash, tokens_json, created_at, description, expires_at, permissions_json)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
	type=VALUES(type), type_value=VALUES(type_value), encrypted_secret=VALUES(encrypted_secret), password_hash=VALUES(password_hash),
	tokens_json=VALUES(tokens_json), created_at=VALUES(created_at), description=VALUES(description), expires_at=VALUES(expires_at), permissions_json=VALUES(permissions_json)`
}

func (d *MySQLDialect) UpsertSessionQuery() string {
	return `INSERT INTO sessions (session_token, public_id, username, ip, user_agent, created_at, last_active, login_method)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
	public_id=VALUES(public_id), username=VALUES(username), ip=VALUES(ip), user_agent=VALUES(user_agent), created_at=VALUES(created_at), last_active=VALUES(last_active), login_method=VALUES(login_method)`
}
