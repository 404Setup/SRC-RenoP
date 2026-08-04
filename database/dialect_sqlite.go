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

type SQLiteDialect struct{}

func (d *SQLiteDialect) Name() string {
	return "sqlite3"
}

func (d *SQLiteDialect) InitTables(db *sql.DB) error {
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
		permissions_json TEXT NOT NULL
	);`

	sessionsTable := `
	CREATE TABLE IF NOT EXISTS sessions (
		session_token VARCHAR(512) PRIMARY KEY,
		public_id VARCHAR(255) NOT NULL,
		username VARCHAR(255) NOT NULL,
		ip VARCHAR(255) NOT NULL,
		user_agent TEXT NOT NULL,
		created_at BIGINT NOT NULL,
		last_active BIGINT NOT NULL
	);`

	fidoTable := `
	CREATE TABLE IF NOT EXISTS fido_devices (
		id VARCHAR(255) PRIMARY KEY,
		username VARCHAR(255) NOT NULL,
		name VARCHAR(255) NOT NULL,
		credential_id BLOB NOT NULL,
		public_key BLOB NOT NULL,
		attestation_type VARCHAR(64) NOT NULL,
		aaguid BLOB NOT NULL,
		sign_count INT NOT NULL,
		created_at BIGINT NOT NULL
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

	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_username ON sessions(username);")
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_last_active ON sessions(last_active);")
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_tokens_expires_at ON tokens(expires_at) WHERE expires_at IS NOT NULL;")
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_fido_username ON fido_devices(username);")
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_fido_credential_id ON fido_devices(credential_id);")
	return nil
}

func (d *SQLiteDialect) UpsertTokenQuery() string {
	return `INSERT INTO tokens (name, type, type_value, encrypted_secret, password_hash, tokens_json, created_at, description, expires_at, permissions_json)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(name) DO UPDATE SET
	type=excluded.type, type_value=excluded.type_value, encrypted_secret=excluded.encrypted_secret, password_hash=excluded.password_hash,
	tokens_json=excluded.tokens_json, created_at=excluded.created_at, description=excluded.description, expires_at=excluded.expires_at, permissions_json=excluded.permissions_json`
}

func (d *SQLiteDialect) UpsertSessionQuery() string {
	return `INSERT INTO sessions (session_token, public_id, username, ip, user_agent, created_at, last_active)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(session_token) DO UPDATE SET
	public_id=excluded.public_id, username=excluded.username, ip=excluded.ip, user_agent=excluded.user_agent, created_at=excluded.created_at, last_active=excluded.last_active`
}
