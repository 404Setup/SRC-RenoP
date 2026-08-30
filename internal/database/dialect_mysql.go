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
	"strings"
)

type MySQLDialect struct{}

func initMySQLDownloadStatisticsIndexes(db *sql.DB) error {
	migrations := [...]SchemaMigration{
		{Name: "idx_download_statistics_user", Query: "CREATE INDEX idx_download_statistics_user ON download_statistics(user_id, repository);"},
		{Name: "idx_download_statistics_repository", Query: "CREATE INDEX idx_download_statistics_repository ON download_statistics(repository, format);"},
		{Name: "idx_download_statistics_namespace", Query: "CREATE INDEX idx_download_statistics_namespace ON download_statistics(repository, namespace);"},
		{Name: "idx_download_statistics_package", Query: "CREATE INDEX idx_download_statistics_package ON download_statistics(repository, package_name);"},
	}
	for _, migration := range migrations {
		if _, err := db.Exec(migration.Query); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate key name") {
				continue
			}
			return fmt.Errorf("failed to apply migration %s: %w", migration.Name, err)
		}
	}
	return nil
}

func initMySQLNPMIndexes(db *sql.DB) error {
	migrations := [...]SchemaMigration{
		{Name: "idx_npm_packages_search", Query: "CREATE INDEX idx_npm_packages_search ON npm_packages(repository, archived, package_name);"},
		{Name: "idx_npm_versions_package", Query: "CREATE INDEX idx_npm_versions_package ON npm_versions(repository, package_name, created_at DESC);"},
		{Name: "idx_npm_tags_package", Query: "CREATE INDEX idx_npm_tags_package ON npm_dist_tags(repository, package_name);"},
		{Name: "idx_npm_members_user", Query: "CREATE INDEX idx_npm_members_user ON npm_members(username, repository);"},
		{Name: "idx_npm_invitations_recipient", Query: "CREATE INDEX idx_npm_invitations_recipient ON npm_invitations(recipient, created_at);"},
	}
	for _, migration := range migrations {
		if _, err := db.Exec(migration.Query); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate key name") {
				continue
			}
			return fmt.Errorf("failed to apply migration %s: %w", migration.Name, err)
		}
	}
	return nil
}

func initMySQLSuperTeamIndexes(db *sql.DB) error {
	migrations := [...]SchemaMigration{
		{Name: "idx_super_teams_creator", Query: "CREATE INDEX idx_super_teams_creator ON super_teams(created_by, prefix);"},
		{Name: "idx_super_team_members_user", Query: "CREATE INDEX idx_super_team_members_user ON super_team_members(user_id, team_prefix);"},
		{Name: "idx_super_team_invitations_recipient", Query: "CREATE INDEX idx_super_team_invitations_recipient ON super_team_invitations(recipient_id, expires_at);"},
		{Name: "idx_cargo_packages_super_team", Query: "CREATE INDEX idx_cargo_packages_super_team ON cargo_packages(super_team_prefix, repository);"},
		{Name: "idx_docker_images_super_team", Query: "CREATE INDEX idx_docker_images_super_team ON docker_images(super_team_prefix, repository);"},
		{Name: "idx_npm_packages_super_team", Query: "CREATE INDEX idx_npm_packages_super_team ON npm_packages(super_team_prefix, repository);"},
		{Name: "idx_maven_domains_super_team", Query: "CREATE INDEX idx_maven_domains_super_team ON maven_domains(super_team_prefix, domain);"},
		{Name: "idx_maven_artifacts_super_team", Query: "CREATE INDEX idx_maven_artifacts_super_team ON maven_artifacts(super_team_prefix, repository);"},
		{Name: "idx_review_tasks_team", Query: "CREATE INDEX idx_review_tasks_team ON review_tasks(review_team_prefix, status, kind, created_at);"},
		{Name: "idx_review_tasks_requester", Query: "CREATE INDEX idx_review_tasks_requester ON review_tasks(requested_by_id, status, created_at);"},
		{Name: "idx_review_task_files_task", Query: "CREATE INDEX idx_review_task_files_task ON review_task_files(task_id, added_at);"},
		{Name: "idx_publication_quota_reservation_owner", Query: "CREATE INDEX idx_publication_quota_reservation_owner ON publication_quota_reservations(owner_type, owner_key, period_start, expires_at);"},
		{Name: "idx_publication_quota_reservation_expiry", Query: "CREATE INDEX idx_publication_quota_reservation_expiry ON publication_quota_reservations(expires_at);"},
		{Name: "idx_publication_quota_usage_window", Query: "CREATE INDEX idx_publication_quota_usage_window ON publication_quota_usage(period_start);"},
	}
	for _, migration := range migrations {
		if _, err := db.Exec(migration.Query); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate key name") {
				continue
			}
			return fmt.Errorf("failed to apply migration %s: %w", migration.Name, err)
		}
	}
	return nil
}

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

	userProfilesTable := `
	CREATE TABLE IF NOT EXISTS user_profiles (
		user_id VARCHAR(36) PRIMARY KEY,
		username VARCHAR(255) NOT NULL UNIQUE,
		nickname VARCHAR(144) NOT NULL DEFAULT '',
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

	userMessagesTable := `
	CREATE TABLE IF NOT EXISTS user_messages (
		id CHAR(36) PRIMARY KEY,
		recipient VARCHAR(255) NOT NULL,
		sender VARCHAR(255) NOT NULL,
		kind VARCHAR(64) NOT NULL,
		severity VARCHAR(16) NOT NULL,
		title VARCHAR(240) NOT NULL,
		body TEXT NOT NULL,
		payload_json TEXT NOT NULL,
		action_kind VARCHAR(64) NOT NULL DEFAULT '',
		action_status VARCHAR(16) NOT NULL DEFAULT '',
		created_at BIGINT NOT NULL,
		read_at BIGINT NOT NULL DEFAULT 0,
		acted_at BIGINT NOT NULL DEFAULT 0,
		expires_at BIGINT NOT NULL DEFAULT 0,
		dedupe_key VARCHAR(255) NULL,
		UNIQUE KEY uq_user_messages_dedupe (recipient, dedupe_key),
		INDEX idx_user_messages_recipient_time (recipient, created_at DESC, id DESC),
		INDEX idx_user_messages_unread (recipient, read_at, expires_at),
		INDEX idx_user_messages_action (action_kind, action_status, expires_at)
	);`

	cargoPackagesTable := `
	CREATE TABLE IF NOT EXISTS cargo_packages (
		repository VARCHAR(64) NOT NULL,
		normalized_name VARCHAR(64) NOT NULL,
		package_name VARCHAR(64) NOT NULL,
		description TEXT NOT NULL,
		readme MEDIUMTEXT NOT NULL,
		repository_url VARCHAR(1024) NOT NULL DEFAULT '',
		homepage VARCHAR(1024) NOT NULL DEFAULT '',
		documentation VARCHAR(1024) NOT NULL DEFAULT '',
		super_team_prefix VARCHAR(64) NOT NULL DEFAULT '',
		archived TINYINT(1) NOT NULL DEFAULT 0,
		admin_archived TINYINT(1) NOT NULL DEFAULT 0,
		mirrored TINYINT(1) NOT NULL DEFAULT 0,
		created_at BIGINT NOT NULL,
		updated_at BIGINT NOT NULL,
		PRIMARY KEY (repository, normalized_name),
		INDEX idx_cargo_packages_search (repository, archived, normalized_name)
	);`

	cargoVersionsTable := `
	CREATE TABLE IF NOT EXISTS cargo_versions (
		repository VARCHAR(64) NOT NULL,
		normalized_name VARCHAR(64) NOT NULL,
		version VARCHAR(128) NOT NULL,
		description TEXT NOT NULL,
		publisher VARCHAR(255) NOT NULL,
		size BIGINT NOT NULL DEFAULT 0,
		checksum VARCHAR(64) NOT NULL DEFAULT '',
		rust_version VARCHAR(64) NOT NULL DEFAULT '',
		license VARCHAR(255) NOT NULL DEFAULT '',
		repository_url VARCHAR(1024) NOT NULL DEFAULT '',
		homepage VARCHAR(1024) NOT NULL DEFAULT '',
		documentation VARCHAR(1024) NOT NULL DEFAULT '',
		yanked TINYINT(1) NOT NULL DEFAULT 0,
		admin_yanked TINYINT(1) NOT NULL DEFAULT 0,
		archive_yanked TINYINT(1) NOT NULL DEFAULT 0,
		mirrored TINYINT(1) NOT NULL DEFAULT 0,
		created_at BIGINT NOT NULL,
		PRIMARY KEY (repository, normalized_name, version),
		INDEX idx_cargo_versions_package (repository, normalized_name, created_at)
	);`

	cargoMembersTable := `
	CREATE TABLE IF NOT EXISTS cargo_members (
		repository VARCHAR(64) NOT NULL,
		normalized_name VARCHAR(64) NOT NULL,
		username VARCHAR(255) NOT NULL,
		user_id VARCHAR(36) NULL,
		permission_level INT NOT NULL,
		added_at BIGINT NOT NULL,
		PRIMARY KEY (repository, normalized_name, username),
		INDEX idx_cargo_members_user (username, repository)
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
		UNIQUE KEY uq_cargo_invitation (repository, normalized_name, recipient),
		INDEX idx_cargo_invitations_recipient (recipient, created_at)
	);`

	dockerImagesTable := `
	CREATE TABLE IF NOT EXISTS docker_images (
		repository VARCHAR(64) NOT NULL,
		image_name VARCHAR(255) NOT NULL,
		description TEXT NOT NULL,
		publisher VARCHAR(255) NOT NULL DEFAULT '',
		pull_count BIGINT NOT NULL DEFAULT 0,
		super_team_prefix VARCHAR(64) NOT NULL DEFAULT '',
		private INT NOT NULL DEFAULT 0,
		push_enabled INT NOT NULL DEFAULT 1,
		created_at BIGINT NOT NULL,
		updated_at BIGINT NOT NULL,
		PRIMARY KEY (repository, image_name),
		INDEX idx_docker_images_search (repository, image_name)
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
		PRIMARY KEY (repository, image_name, tag),
		INDEX idx_docker_tags_repo_img (repository, image_name, updated_at DESC),
		INDEX idx_docker_tags_digest (repository, digest)
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
		raw_json MEDIUMTEXT NOT NULL,
		created_at BIGINT NOT NULL,
		PRIMARY KEY (repository, image_name, digest),
		INDEX idx_docker_manifests_repo_img (repository, image_name)
	);`

	dockerBlobsTable := `
	CREATE TABLE IF NOT EXISTS docker_blobs (
		repository VARCHAR(64) NOT NULL,
		digest VARCHAR(128) NOT NULL,
		size BIGINT NOT NULL DEFAULT 0,
		created_at BIGINT NOT NULL,
		PRIMARY KEY (repository, digest),
		INDEX idx_docker_blobs_repo (repository, digest)
	);`

	dockerMembersTable := `
	CREATE TABLE IF NOT EXISTS docker_members (
		repository VARCHAR(64) NOT NULL,
		image_name VARCHAR(255) NOT NULL,
		username VARCHAR(255) NOT NULL,
		user_id VARCHAR(36) NULL,
		permission_level INT NOT NULL,
		added_at BIGINT NOT NULL,
		PRIMARY KEY (repository, image_name, username),
		INDEX idx_docker_members_user (username, repository)
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
		UNIQUE KEY uq_docker_invitation (repository, image_name, recipient),
		INDEX idx_docker_invitations_recipient (recipient, created_at)
	);`

	if _, err := db.Exec(tokensTable); err != nil {
		return err
	}
	if _, err := db.Exec(userProfilesTable); err != nil {
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
	if _, err := db.Exec(userMessagesTable); err != nil {
		return err
	}
	if _, err := db.Exec(cargoPackagesTable); err != nil {
		return err
	}
	if _, err := db.Exec(cargoVersionsTable); err != nil {
		return err
	}
	if _, err := db.Exec(cargoMembersTable); err != nil {
		return err
	}
	if _, err := db.Exec(cargoInvitationsTable); err != nil {
		return err
	}
	if _, err := db.Exec(dockerImagesTable); err != nil {
		return err
	}
	if _, err := db.Exec(dockerTagsTable); err != nil {
		return err
	}
	if _, err := db.Exec(dockerManifestsTable); err != nil {
		return err
	}
	if _, err := db.Exec(dockerBlobsTable); err != nil {
		return err
	}
	if _, err := db.Exec(dockerMembersTable); err != nil {
		return err
	}
	if _, err := db.Exec(dockerInvitationsTable); err != nil {
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
	if err := initMavenTables(db, "MEDIUMTEXT NOT NULL"); err != nil {
		return err
	}
	if err := initNPMTables(db); err != nil {
		return err
	}
	if err := initMySQLNPMIndexes(db); err != nil {
		return err
	}
	if err := initDownloadStatisticsTables(db); err != nil {
		return err
	}
	if err := initMySQLDownloadStatisticsIndexes(db); err != nil {
		return err
	}
	if err := initSuperTeamTables(db); err != nil {
		return err
	}
	if err := initPublicationQuotaTables(db); err != nil {
		return err
	}
	if err := initReviewTables(db, true); err != nil {
		return err
	}

	for _, migration := range sharedColumnMigrations {
		query := migration.Query
		if migration.MySQLQuery != "" {
			query = migration.MySQLQuery
		}
		if err := execIgnoreDuplicateColumn(db, query); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Name, err)
		}
	}
	if err := initMySQLSuperTeamIndexes(db); err != nil {
		return err
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

func (d *MySQLDialect) UpsertGPGPublicKeyQuery() string {
	return `INSERT INTO gpg_public_keys (fingerprint, key_id, primary_identity, public_key, key_created_at, key_expires_at, fetched_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE key_id=VALUES(key_id), primary_identity=VALUES(primary_identity),
	public_key=VALUES(public_key), key_created_at=VALUES(key_created_at), key_expires_at=VALUES(key_expires_at), fetched_at=VALUES(fetched_at)`
}

func (d *MySQLDialect) UpsertGPGSignatureQuery() string {
	return `INSERT INTO gpg_signatures (artifact_key, repository, artifact_path, fingerprint, key_id, primary_identity, uploader, signature_created_at, verified_at, hash_algorithm, public_key_algorithm)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE repository=VALUES(repository), artifact_path=VALUES(artifact_path), fingerprint=VALUES(fingerprint),
	key_id=VALUES(key_id), primary_identity=VALUES(primary_identity), uploader=VALUES(uploader),
	signature_created_at=VALUES(signature_created_at), verified_at=VALUES(verified_at),
	hash_algorithm=VALUES(hash_algorithm), public_key_algorithm=VALUES(public_key_algorithm)`
}

func (d *MySQLDialect) Rebind(query string) string {
	return query
}
