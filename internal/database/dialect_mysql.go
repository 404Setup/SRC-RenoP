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
	"fmt"
)

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
		INDEX idx_sessions_last_active (last_active),
		INDEX idx_sessions_user_public (username, public_id)
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

	gpgPublicKeysTable := `
	CREATE TABLE IF NOT EXISTS gpg_public_keys (
		fingerprint VARCHAR(64) PRIMARY KEY,
		key_id VARCHAR(16) NOT NULL,
		primary_identity TEXT NOT NULL,
		public_key MEDIUMBLOB NOT NULL,
		key_created_at BIGINT NOT NULL,
		key_expires_at BIGINT NOT NULL DEFAULT 0,
		fetched_at BIGINT NOT NULL
	);`

	gpgKeyAliasesTable := `
	CREATE TABLE IF NOT EXISTS gpg_key_aliases (
		identifier VARCHAR(64) NOT NULL,
		fingerprint VARCHAR(64) NOT NULL,
		PRIMARY KEY (identifier, fingerprint),
		INDEX idx_gpg_alias_identifier (identifier),
		INDEX idx_gpg_alias_fingerprint (fingerprint),
		CONSTRAINT fk_gpg_alias_key FOREIGN KEY (fingerprint) REFERENCES gpg_public_keys(fingerprint) ON DELETE CASCADE
	);`

	userGPGKeysTable := `
	CREATE TABLE IF NOT EXISTS user_gpg_keys (
		username VARCHAR(255) NOT NULL,
		fingerprint VARCHAR(64) NOT NULL,
		requested_id VARCHAR(64) NOT NULL,
		added_at BIGINT NOT NULL,
		PRIMARY KEY (username, fingerprint),
		INDEX idx_user_gpg_username (username),
		CONSTRAINT fk_user_gpg_key FOREIGN KEY (fingerprint) REFERENCES gpg_public_keys(fingerprint) ON DELETE CASCADE
	);`

	gpgSignaturesTable := `
	CREATE TABLE IF NOT EXISTS gpg_signatures (
		artifact_key CHAR(64) PRIMARY KEY,
		repository VARCHAR(255) NOT NULL,
		artifact_path TEXT NOT NULL,
		fingerprint VARCHAR(64) NOT NULL,
		key_id VARCHAR(16) NOT NULL,
		primary_identity TEXT NOT NULL,
		uploader VARCHAR(255) NOT NULL,
		signature_created_at BIGINT NOT NULL,
		verified_at BIGINT NOT NULL,
		hash_algorithm VARCHAR(32) NOT NULL,
		public_key_algorithm VARCHAR(32) NOT NULL,
		INDEX idx_gpg_signatures_repository (repository)
	);`

	gpgReleasesTable := `
	CREATE TABLE IF NOT EXISTS gpg_releases (
		id CHAR(36) PRIMARY KEY,
		active_key CHAR(64) NULL UNIQUE,
		repository VARCHAR(255) NOT NULL,
		artifact_path TEXT NOT NULL,
		uploader VARCHAR(255) NOT NULL,
		status VARCHAR(16) NOT NULL,
		failure_reason TEXT NOT NULL,
		require_signature TINYINT(1) NOT NULL DEFAULT 0,
		artifact_staging_path TEXT NOT NULL,
		signature_staging_path TEXT NOT NULL,
		artifact_size BIGINT NOT NULL DEFAULT 0,
		artifact_mod_time BIGINT NOT NULL DEFAULT 0,
		signature_size BIGINT NOT NULL DEFAULT 0,
		signature_mod_time BIGINT NOT NULL DEFAULT 0,
		artifact_existed TINYINT(1) NOT NULL DEFAULT 0,
		signature_existed TINYINT(1) NOT NULL DEFAULT 0,
		artifact_generate_checksums TINYINT(1) NOT NULL DEFAULT 0,
		signature_generate_checksums TINYINT(1) NOT NULL DEFAULT 0,
		artifact_md5 VARCHAR(64) NOT NULL DEFAULT '',
		artifact_sha1 VARCHAR(64) NOT NULL DEFAULT '',
		artifact_sha256 VARCHAR(128) NOT NULL DEFAULT '',
		artifact_sha512 VARCHAR(128) NOT NULL DEFAULT '',
		signature_md5 VARCHAR(64) NOT NULL DEFAULT '',
		signature_sha1 VARCHAR(64) NOT NULL DEFAULT '',
		signature_sha256 VARCHAR(128) NOT NULL DEFAULT '',
		signature_sha512 VARCHAR(128) NOT NULL DEFAULT '',
		publish_started TINYINT(1) NOT NULL DEFAULT 0,
		created_at BIGINT NOT NULL,
		updated_at BIGINT NOT NULL,
		completed_at BIGINT NOT NULL DEFAULT 0,
		cleanup_pending TINYINT(1) NOT NULL DEFAULT 0,
		INDEX idx_gpg_releases_user_time (uploader, created_at),
		INDEX idx_gpg_releases_queue (status, created_at),
		INDEX idx_gpg_releases_cleanup (cleanup_pending, updated_at)
	);`

	auditLogsTable := `
	CREATE TABLE IF NOT EXISTS audit_logs (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		username VARCHAR(255) NOT NULL,
		operator VARCHAR(255) NOT NULL,
		action VARCHAR(64) NOT NULL,
		details TEXT NOT NULL,
		auth_method VARCHAR(64) NOT NULL,
		session_id VARCHAR(255) NOT NULL DEFAULT '',
		ip VARCHAR(255) NOT NULL,
		created_at BIGINT NOT NULL,
		INDEX idx_audit_logs_user_time (username, created_at),
		INDEX idx_audit_logs_user_id (username, id DESC),
		INDEX idx_audit_logs_created_at (created_at)
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
	if _, err := db.Exec(gpgPublicKeysTable); err != nil {
		return err
	}
	if _, err := db.Exec(gpgKeyAliasesTable); err != nil {
		return err
	}
	if _, err := db.Exec(userGPGKeysTable); err != nil {
		return err
	}
	if _, err := db.Exec(gpgSignaturesTable); err != nil {
		return err
	}
	if _, err := db.Exec(gpgReleasesTable); err != nil {
		return err
	}
	if _, err := db.Exec(auditLogsTable); err != nil {
		return err
	}

	columnMigrations := []struct {
		name  string
		query string
	}{
		{name: "sessions.login_method", query: "ALTER TABLE sessions ADD COLUMN login_method VARCHAR(64) NOT NULL DEFAULT 'password';"},
		{name: "fido_devices.user_present", query: "ALTER TABLE fido_devices ADD COLUMN user_present INT NOT NULL DEFAULT 0;"},
		{name: "fido_devices.user_verified", query: "ALTER TABLE fido_devices ADD COLUMN user_verified INT NOT NULL DEFAULT 0;"},
		{name: "fido_devices.backup_eligible", query: "ALTER TABLE fido_devices ADD COLUMN backup_eligible INT NOT NULL DEFAULT 0;"},
		{name: "fido_devices.backup_state", query: "ALTER TABLE fido_devices ADD COLUMN backup_state INT NOT NULL DEFAULT 0;"},
	}
	for _, migration := range columnMigrations {
		if err := execIgnoreDuplicateColumn(db, migration.query); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.name, err)
		}
	}

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
