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
	"strconv"
	"strings"
)

type PostgresDialect struct{}

func (d *PostgresDialect) Name() string {
	return "postgres"
}

func (d *PostgresDialect) Rebind(query string) string {
	return RebindPostgres(query)
}

// RebindPostgres transforms '?' placeholders in a SQL query into positional '$1, $2, ...' placeholders,
// safely ignoring '?' characters inside single-quoted string literals, double-quoted identifiers,
// and SQL comments.
func RebindPostgres(query string) string {
	if strings.IndexByte(query, '?') == -1 {
		return query
	}

	var b strings.Builder
	b.Grow(len(query) + 16)

	paramIdx := 1
	inSingleQuote := false
	inDoubleQuote := false
	inLineComment := false
	inBlockComment := false

	for i := 0; i < len(query); i++ {
		c := query[i]

		if inLineComment {
			b.WriteByte(c)
			if c == '\n' {
				inLineComment = false
			}
			continue
		}

		if inBlockComment {
			b.WriteByte(c)
			if c == '*' && i+1 < len(query) && query[i+1] == '/' {
				b.WriteByte('/')
				i++
				inBlockComment = false
			}
			continue
		}

		if inSingleQuote {
			b.WriteByte(c)
			if c == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					b.WriteByte('\'')
					i++
				} else {
					inSingleQuote = false
				}
			}
			continue
		}

		if inDoubleQuote {
			b.WriteByte(c)
			if c == '"' {
				if i+1 < len(query) && query[i+1] == '"' {
					b.WriteByte('"')
					i++
				} else {
					inDoubleQuote = false
				}
			}
			continue
		}

		if c == '\'' {
			inSingleQuote = true
			b.WriteByte(c)
			continue
		}

		if c == '"' {
			inDoubleQuote = true
			b.WriteByte(c)
			continue
		}

		if c == '-' && i+1 < len(query) && query[i+1] == '-' {
			inLineComment = true
			b.WriteByte('-')
			b.WriteByte('-')
			i++
			continue
		}

		if c == '/' && i+1 < len(query) && query[i+1] == '*' {
			inBlockComment = true
			b.WriteByte('/')
			b.WriteByte('*')
			i++
			continue
		}

		if c == '?' {
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(paramIdx))
			paramIdx++
			continue
		}

		b.WriteByte(c)
	}

	return b.String()
}

func (d *PostgresDialect) InitTables(db *sql.DB) error {
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
		ban_reason VARCHAR(2048) NOT NULL DEFAULT '',
		banned_at BIGINT NOT NULL DEFAULT 0,
		banned_until BIGINT NULL
	);`

	userProfilesTable := `
	CREATE TABLE IF NOT EXISTS user_profiles (
		user_id VARCHAR(36) PRIMARY KEY,
		username VARCHAR(255) NOT NULL UNIQUE,
		nickname VARCHAR(144) NOT NULL DEFAULT '',
		website_url VARCHAR(2048) NOT NULL DEFAULT '',
		github_url VARCHAR(2048) NOT NULL DEFAULT '',
		discord_url VARCHAR(2048) NOT NULL DEFAULT '',
		custom_link_name VARCHAR(160) NOT NULL DEFAULT '',
		custom_link_url VARCHAR(2048) NOT NULL DEFAULT '',
		rename_window_started_at BIGINT NOT NULL DEFAULT 0,
		rename_count INT NOT NULL DEFAULT 0,
		updated_at BIGINT NOT NULL DEFAULT 0
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
		login_method VARCHAR(64) NOT NULL DEFAULT 'password'
	);`

	fidoTable := `
	CREATE TABLE IF NOT EXISTS fido_devices (
		id VARCHAR(255) PRIMARY KEY,
		username VARCHAR(255) NOT NULL,
		name VARCHAR(255) NOT NULL,
		credential_id BYTEA NOT NULL,
		public_key BYTEA NOT NULL,
		attestation_type VARCHAR(64) NOT NULL,
		aaguid BYTEA NOT NULL,
		sign_count INT NOT NULL,
		created_at BIGINT NOT NULL,
		user_present INT NOT NULL DEFAULT 0,
		user_verified INT NOT NULL DEFAULT 0,
		backup_eligible INT NOT NULL DEFAULT 0,
		backup_state INT NOT NULL DEFAULT 0
	);`

	gpgPublicKeysTable := `
	CREATE TABLE IF NOT EXISTS gpg_public_keys (
		fingerprint VARCHAR(64) PRIMARY KEY,
		key_id VARCHAR(16) NOT NULL,
		primary_identity TEXT NOT NULL,
		public_key BYTEA NOT NULL,
		key_created_at BIGINT NOT NULL,
		key_expires_at BIGINT NOT NULL DEFAULT 0,
		fetched_at BIGINT NOT NULL
	);`

	gpgKeyAliasesTable := `
	CREATE TABLE IF NOT EXISTS gpg_key_aliases (
		identifier VARCHAR(64) NOT NULL,
		fingerprint VARCHAR(64) NOT NULL,
		PRIMARY KEY (identifier, fingerprint),
		FOREIGN KEY (fingerprint) REFERENCES gpg_public_keys(fingerprint) ON DELETE CASCADE
	);`

	userGPGKeysTable := `
	CREATE TABLE IF NOT EXISTS user_gpg_keys (
		username VARCHAR(255) NOT NULL,
		fingerprint VARCHAR(64) NOT NULL,
		requested_id VARCHAR(64) NOT NULL,
		added_at BIGINT NOT NULL,
		PRIMARY KEY (username, fingerprint),
		FOREIGN KEY (fingerprint) REFERENCES gpg_public_keys(fingerprint) ON DELETE CASCADE
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
		public_key_algorithm VARCHAR(32) NOT NULL
	);`

	gpgReleasesTable := `
	CREATE TABLE IF NOT EXISTS gpg_releases (
		id CHAR(36) PRIMARY KEY,
		active_key CHAR(64) NULL UNIQUE,
		repository VARCHAR(255) NOT NULL,
		artifact_path TEXT NOT NULL,
		uploader VARCHAR(255) NOT NULL,
		status VARCHAR(16) NOT NULL,
		failure_reason TEXT NOT NULL DEFAULT '',
		require_signature INT NOT NULL DEFAULT 0,
		artifact_staging_path TEXT NOT NULL DEFAULT '',
		signature_staging_path TEXT NOT NULL DEFAULT '',
		artifact_size BIGINT NOT NULL DEFAULT 0,
		artifact_mod_time BIGINT NOT NULL DEFAULT 0,
		signature_size BIGINT NOT NULL DEFAULT 0,
		signature_mod_time BIGINT NOT NULL DEFAULT 0,
		artifact_existed INT NOT NULL DEFAULT 0,
		signature_existed INT NOT NULL DEFAULT 0,
		artifact_generate_checksums INT NOT NULL DEFAULT 0,
		signature_generate_checksums INT NOT NULL DEFAULT 0,
		artifact_md5 VARCHAR(64) NOT NULL DEFAULT '',
		artifact_sha1 VARCHAR(64) NOT NULL DEFAULT '',
		artifact_sha256 VARCHAR(128) NOT NULL DEFAULT '',
		artifact_sha512 VARCHAR(128) NOT NULL DEFAULT '',
		signature_md5 VARCHAR(64) NOT NULL DEFAULT '',
		signature_sha1 VARCHAR(64) NOT NULL DEFAULT '',
		signature_sha256 VARCHAR(128) NOT NULL DEFAULT '',
		signature_sha512 VARCHAR(128) NOT NULL DEFAULT '',
		publish_started INT NOT NULL DEFAULT 0,
		created_at BIGINT NOT NULL,
		updated_at BIGINT NOT NULL,
		completed_at BIGINT NOT NULL DEFAULT 0,
		cleanup_pending INT NOT NULL DEFAULT 0
	);`

	auditLogsTable := `
	CREATE TABLE IF NOT EXISTS audit_logs (
		id BIGSERIAL PRIMARY KEY,
		username VARCHAR(255) NOT NULL,
		operator VARCHAR(255) NOT NULL,
		action VARCHAR(64) NOT NULL,
		details TEXT NOT NULL,
		auth_method VARCHAR(64) NOT NULL,
		session_id VARCHAR(255) NOT NULL DEFAULT '',
		ip VARCHAR(255) NOT NULL,
		created_at BIGINT NOT NULL
	);`

	userMessagesTable := `
	CREATE TABLE IF NOT EXISTS user_messages (
		id CHAR(36) PRIMARY KEY,
		recipient VARCHAR(255) NOT NULL,
		sender VARCHAR(255) NOT NULL,
		kind VARCHAR(64) NOT NULL,
		severity VARCHAR(16) NOT NULL,
		title VARCHAR(240) NOT NULL,
		body TEXT NOT NULL,
		payload_json TEXT NOT NULL DEFAULT '{}',
		action_kind VARCHAR(64) NOT NULL DEFAULT '',
		action_status VARCHAR(16) NOT NULL DEFAULT '',
		created_at BIGINT NOT NULL,
		read_at BIGINT NOT NULL DEFAULT 0,
		acted_at BIGINT NOT NULL DEFAULT 0,
		expires_at BIGINT NOT NULL DEFAULT 0,
		dedupe_key VARCHAR(255) NULL,
		UNIQUE (recipient, dedupe_key)
	);`

	cargoPackagesTable := `
	CREATE TABLE IF NOT EXISTS cargo_packages (
		repository VARCHAR(64) NOT NULL,
		normalized_name VARCHAR(64) NOT NULL,
		package_name VARCHAR(64) NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		readme TEXT NOT NULL DEFAULT '',
		repository_url VARCHAR(1024) NOT NULL DEFAULT '',
		homepage VARCHAR(1024) NOT NULL DEFAULT '',
		documentation VARCHAR(1024) NOT NULL DEFAULT '',
		super_team_prefix VARCHAR(64) NOT NULL DEFAULT '',
		archived INT NOT NULL DEFAULT 0,
		admin_archived INT NOT NULL DEFAULT 0,
		mirrored INT NOT NULL DEFAULT 0,
		created_at BIGINT NOT NULL,
		updated_at BIGINT NOT NULL,
		PRIMARY KEY (repository, normalized_name)
	);`

	cargoVersionsTable := `
	CREATE TABLE IF NOT EXISTS cargo_versions (
		repository VARCHAR(64) NOT NULL,
		normalized_name VARCHAR(64) NOT NULL,
		version VARCHAR(128) NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		publisher VARCHAR(255) NOT NULL,
		size BIGINT NOT NULL DEFAULT 0,
		checksum VARCHAR(64) NOT NULL DEFAULT '',
		rust_version VARCHAR(64) NOT NULL DEFAULT '',
		license VARCHAR(255) NOT NULL DEFAULT '',
		repository_url VARCHAR(1024) NOT NULL DEFAULT '',
		homepage VARCHAR(1024) NOT NULL DEFAULT '',
		documentation VARCHAR(1024) NOT NULL DEFAULT '',
		yanked INT NOT NULL DEFAULT 0,
		admin_yanked INT NOT NULL DEFAULT 0,
		archive_yanked INT NOT NULL DEFAULT 0,
		mirrored INT NOT NULL DEFAULT 0,
		created_at BIGINT NOT NULL,
		PRIMARY KEY (repository, normalized_name, version)
	);`

	cargoMembersTable := `
	CREATE TABLE IF NOT EXISTS cargo_members (
		repository VARCHAR(64) NOT NULL,
		normalized_name VARCHAR(64) NOT NULL,
		username VARCHAR(255) NOT NULL,
		user_id VARCHAR(36) NULL,
		permission_level INT NOT NULL,
		added_at BIGINT NOT NULL,
		PRIMARY KEY (repository, normalized_name, username)
	);`

	cargoInvitationsTable := `
	CREATE TABLE IF NOT EXISTS cargo_invitations (
		id CHAR(36) PRIMARY KEY,
		repository VARCHAR(64) NOT NULL,
		normalized_name VARCHAR(64) NOT NULL,
		package_name VARCHAR(64) NOT NULL,
		inviter VARCHAR(255) NOT NULL,
		recipient VARCHAR(255) NOT NULL,
		permission_level INT NOT NULL,
		created_at BIGINT NOT NULL,
		UNIQUE (repository, normalized_name, recipient)
	);`

	dockerImagesTable := `
	CREATE TABLE IF NOT EXISTS docker_images (
		repository VARCHAR(64) NOT NULL,
		image_name VARCHAR(255) NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		publisher VARCHAR(255) NOT NULL DEFAULT '',
		pull_count BIGINT NOT NULL DEFAULT 0,
		super_team_prefix VARCHAR(64) NOT NULL DEFAULT '',
		private INT NOT NULL DEFAULT 0,
		push_enabled INT NOT NULL DEFAULT 1,
		created_at BIGINT NOT NULL,
		updated_at BIGINT NOT NULL,
		PRIMARY KEY (repository, image_name)
	);`

	dockerTagsTable := `
	CREATE TABLE IF NOT EXISTS docker_tags (
		repository VARCHAR(64) NOT NULL,
		image_name VARCHAR(255) NOT NULL,
		tag VARCHAR(128) NOT NULL,
		digest VARCHAR(128) NOT NULL,
		media_type VARCHAR(128) NOT NULL DEFAULT '',
		size BIGINT NOT NULL DEFAULT 0,
		config_digest VARCHAR(128) NOT NULL DEFAULT '',
		publisher VARCHAR(255) NOT NULL DEFAULT '',
		created_at BIGINT NOT NULL,
		updated_at BIGINT NOT NULL,
		PRIMARY KEY (repository, image_name, tag)
	);`

	dockerManifestsTable := `
	CREATE TABLE IF NOT EXISTS docker_manifests (
		repository VARCHAR(64) NOT NULL,
		image_name VARCHAR(255) NOT NULL,
		digest VARCHAR(128) NOT NULL,
		media_type VARCHAR(128) NOT NULL DEFAULT '',
		size BIGINT NOT NULL DEFAULT 0,
		config_digest VARCHAR(128) NOT NULL DEFAULT '',
		publisher VARCHAR(255) NOT NULL DEFAULT '',
		raw_json TEXT NOT NULL,
		created_at BIGINT NOT NULL,
		PRIMARY KEY (repository, image_name, digest)
	);`

	dockerBlobsTable := `
	CREATE TABLE IF NOT EXISTS docker_blobs (
		repository VARCHAR(64) NOT NULL,
		digest VARCHAR(128) NOT NULL,
		size BIGINT NOT NULL DEFAULT 0,
		created_at BIGINT NOT NULL,
		PRIMARY KEY (repository, digest)
	);`

	dockerMembersTable := `
	CREATE TABLE IF NOT EXISTS docker_members (
		repository VARCHAR(64) NOT NULL,
		image_name VARCHAR(255) NOT NULL,
		username VARCHAR(255) NOT NULL,
		user_id VARCHAR(36) NULL,
		permission_level INT NOT NULL,
		added_at BIGINT NOT NULL,
		PRIMARY KEY (repository, image_name, username)
	);`

	dockerInvitationsTable := `
	CREATE TABLE IF NOT EXISTS docker_invitations (
		id CHAR(36) PRIMARY KEY,
		repository VARCHAR(64) NOT NULL,
		image_name VARCHAR(255) NOT NULL,
		inviter VARCHAR(255) NOT NULL,
		recipient VARCHAR(255) NOT NULL,
		permission_level INT NOT NULL,
		created_at BIGINT NOT NULL,
		UNIQUE (repository, image_name, recipient)
	);`

	tables := []string{
		tokensTable,
		userProfilesTable,
		sessionsTable,
		fidoTable,
		gpgPublicKeysTable,
		gpgKeyAliasesTable,
		userGPGKeysTable,
		gpgSignaturesTable,
		gpgReleasesTable,
		auditLogsTable,
		userMessagesTable,
		cargoPackagesTable,
		cargoVersionsTable,
		cargoMembersTable,
		cargoInvitationsTable,
		dockerImagesTable,
		dockerTagsTable,
		dockerManifestsTable,
		dockerBlobsTable,
		dockerMembersTable,
		dockerInvitationsTable,
	}

	for _, tbl := range tables {
		if _, err := db.Exec(tbl); err != nil {
			return err
		}
	}
	if err := initUserAvatarTable(db, "BYTEA"); err != nil {
		return err
	}
	if err := initDockerImageBlobTables(db); err != nil {
		return err
	}
	if err := initGitHubIdentityTables(db); err != nil {
		return err
	}
	if err := initAccountSecurityTables(db); err != nil {
		return err
	}
	if err := initMavenTables(db, "TEXT NOT NULL"); err != nil {
		return err
	}
	if err := initNPMTables(db); err != nil {
		return err
	}
	if err := initDownloadStatisticsTables(db); err != nil {
		return err
	}
	if err := initSuperTeamTables(db); err != nil {
		return err
	}
	if err := initPublicationQuotaTables(db); err != nil {
		return err
	}
	if err := initReviewTables(db, false); err != nil {
		return err
	}

	for _, migration := range sharedColumnMigrations {
		if err := execIgnoreDuplicateColumn(db, migration.Query); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Name, err)
		}
	}

	if err := applySharedIndexMigrations(db); err != nil {
		return err
	}
	return nil
}

func (d *PostgresDialect) UpsertTokenQuery() string {
	return `INSERT INTO tokens (name, type, type_value, encrypted_secret, password_hash, tokens_json, created_at, description, expires_at, permissions_json, ban_reason, banned_at, banned_until)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(name) DO UPDATE SET
	type=excluded.type, type_value=excluded.type_value, encrypted_secret=excluded.encrypted_secret, password_hash=excluded.password_hash,
	tokens_json=excluded.tokens_json, created_at=excluded.created_at, description=excluded.description, expires_at=excluded.expires_at,
	permissions_json=excluded.permissions_json, ban_reason=excluded.ban_reason, banned_at=excluded.banned_at, banned_until=excluded.banned_until`
}

func (d *PostgresDialect) UpsertSessionQuery() string {
	return `INSERT INTO sessions (session_token, public_id, username, ip, user_agent, created_at, last_active, login_method)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(session_token) DO UPDATE SET
	public_id=excluded.public_id, username=excluded.username, ip=excluded.ip, user_agent=excluded.user_agent, created_at=excluded.created_at, last_active=excluded.last_active, login_method=excluded.login_method`
}

func (d *PostgresDialect) UpsertGPGPublicKeyQuery() string {
	return `INSERT INTO gpg_public_keys (fingerprint, key_id, primary_identity, public_key, key_created_at, key_expires_at, fetched_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(fingerprint) DO UPDATE SET
	key_id=excluded.key_id, primary_identity=excluded.primary_identity,
	public_key=excluded.public_key, key_created_at=excluded.key_created_at, key_expires_at=excluded.key_expires_at, fetched_at=excluded.fetched_at`
}

func (d *PostgresDialect) UpsertGPGSignatureQuery() string {
	return `INSERT INTO gpg_signatures (artifact_key, repository, artifact_path, fingerprint, key_id, primary_identity, uploader, signature_created_at, verified_at, hash_algorithm, public_key_algorithm)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(artifact_key) DO UPDATE SET
	repository=excluded.repository, artifact_path=excluded.artifact_path,
	fingerprint=excluded.fingerprint, key_id=excluded.key_id, primary_identity=excluded.primary_identity,
	uploader=excluded.uploader, signature_created_at=excluded.signature_created_at, verified_at=excluded.verified_at,
	hash_algorithm=excluded.hash_algorithm, public_key_algorithm=excluded.public_key_algorithm`
}
