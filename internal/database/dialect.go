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

func initPackageDeprecationTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS package_deprecations (
		id VARCHAR(64) PRIMARY KEY,
		format VARCHAR(16) NOT NULL,
		repository VARCHAR(255) NOT NULL,
		package_key TEXT NOT NULL,
		deprecated_at BIGINT NOT NULL
	);`)
	return err
}

func initDockerImageBlobTables(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS docker_image_blobs (
		repository VARCHAR(64) NOT NULL,
		image_name VARCHAR(255) NOT NULL,
		manifest_digest VARCHAR(128) NOT NULL,
		blob_digest VARCHAR(128) NOT NULL,
		PRIMARY KEY (repository, image_name, manifest_digest, blob_digest)
	);`)
	return err
}

func initGitHubIdentityTables(db *sql.DB) error {
	tables := [...]string{
		`CREATE TABLE IF NOT EXISTS github_identities (
			github_user_id BIGINT PRIMARY KEY,
			user_id VARCHAR(36) NOT NULL UNIQUE,
			github_login VARCHAR(39) NOT NULL,
			authorized_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS github_principals (
			user_id VARCHAR(36) NOT NULL,
			principal_type VARCHAR(16) NOT NULL,
			github_principal_id BIGINT NOT NULL,
			github_login VARCHAR(39) NOT NULL,
			authorized_at BIGINT NOT NULL,
			PRIMARY KEY (user_id, principal_type, github_principal_id)
		);`,
	}
	for _, table := range tables {
		if _, err := db.Exec(table); err != nil {
			return err
		}
	}
	return nil
}

func initAccountSecurityTables(db *sql.DB) error {
	tables := [...]string{
		`CREATE TABLE IF NOT EXISTS user_account_security (
			user_id VARCHAR(36) PRIMARY KEY,
			email VARCHAR(254) NULL UNIQUE,
			password_login_enabled INT NOT NULL DEFAULT 1,
			updated_at BIGINT NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS user_recovery_codes (
			user_id VARCHAR(36) NOT NULL,
			selector_hash CHAR(64) NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			created_at BIGINT NOT NULL,
			used_at BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (user_id, selector_hash)
		);`,
		`CREATE TABLE IF NOT EXISTS user_api_tokens (
			id CHAR(36) PRIMARY KEY,
			user_id VARCHAR(36) NOT NULL,
			name VARCHAR(80) NOT NULL,
			secret_hash CHAR(64) NOT NULL UNIQUE,
			scopes_json TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			expires_at BIGINT NULL,
			disabled INT NOT NULL DEFAULT 0,
			UNIQUE (user_id, name)
		);`,
	}
	for _, table := range tables {
		if _, err := db.Exec(table); err != nil {
			return err
		}
	}
	return nil
}

func initDownloadStatisticsTables(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS download_statistics (
		id CHAR(64) PRIMARY KEY,
		user_id VARCHAR(36) NOT NULL,
		username VARCHAR(255) NOT NULL,
		repository VARCHAR(64) NOT NULL,
		format VARCHAR(32) NOT NULL,
		namespace VARCHAR(253) NOT NULL,
		package_name VARCHAR(512) NOT NULL,
		version VARCHAR(255) NOT NULL,
		download_count BIGINT NOT NULL DEFAULT 0,
		download_bytes BIGINT NOT NULL DEFAULT 0,
		updated_at BIGINT NOT NULL
	);`)
	return err
}

func initSuperTeamTables(db *sql.DB) error {
	tables := [...]string{
		`CREATE TABLE IF NOT EXISTS super_teams (
			prefix VARCHAR(64) PRIMARY KEY,
			name VARCHAR(320) NOT NULL,
			description VARCHAR(2048) NOT NULL,
			website_url VARCHAR(2048) NOT NULL DEFAULT '',
			github_url VARCHAR(2048) NOT NULL DEFAULT '',
			discord_url VARCHAR(2048) NOT NULL DEFAULT '',
			custom_link_name VARCHAR(160) NOT NULL DEFAULT '',
			custom_link_url VARCHAR(2048) NOT NULL DEFAULT '',
			created_by VARCHAR(36) NOT NULL,
			created_by_name VARCHAR(255) NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS super_team_members (
			team_prefix VARCHAR(64) NOT NULL,
			user_id VARCHAR(36) NOT NULL,
			role_level INT NOT NULL,
			public_visible INT NOT NULL DEFAULT 1,
			added_at BIGINT NOT NULL,
			PRIMARY KEY (team_prefix, user_id)
		);`,
		`CREATE TABLE IF NOT EXISTS super_team_invitations (
			id CHAR(36) PRIMARY KEY,
			team_prefix VARCHAR(64) NOT NULL,
			inviter_id VARCHAR(36) NOT NULL,
			recipient_id VARCHAR(36) NOT NULL,
			role_level INT NOT NULL,
			created_at BIGINT NOT NULL,
			expires_at BIGINT NOT NULL,
			UNIQUE (team_prefix, recipient_id)
		);`,
		`CREATE TABLE IF NOT EXISTS user_super_team_limits (
			user_id VARCHAR(36) PRIMARY KEY,
			create_limit INT NOT NULL DEFAULT -1,
			join_limit INT NOT NULL DEFAULT -1,
			updated_at BIGINT NOT NULL
		);`,
	}
	for _, table := range tables {
		if _, err := db.Exec(table); err != nil {
			return err
		}
	}
	return nil
}

func initPublicationQuotaTables(db *sql.DB) error {
	tables := [...]string{
		`CREATE TABLE IF NOT EXISTS publication_quota_overrides (
			owner_type VARCHAR(16) NOT NULL,
			owner_key VARCHAR(64) NOT NULL,
			file_limit BIGINT NOT NULL DEFAULT -1,
			byte_limit BIGINT NOT NULL DEFAULT -1,
			publication_limit BIGINT NOT NULL DEFAULT -1,
			quota_period VARCHAR(8) NOT NULL DEFAULT '',
			unlimited INT NOT NULL DEFAULT -1,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (owner_type, owner_key)
		);`,
		`CREATE TABLE IF NOT EXISTS publication_quota_usage (
			owner_type VARCHAR(16) NOT NULL,
			owner_key VARCHAR(64) NOT NULL,
			period_start BIGINT NOT NULL,
			files_used BIGINT NOT NULL DEFAULT 0,
			bytes_used BIGINT NOT NULL DEFAULT 0,
			publications_used BIGINT NOT NULL DEFAULT 0,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (owner_type, owner_key, period_start)
		);`,
		`CREATE TABLE IF NOT EXISTS publication_quota_reservations (
			id CHAR(36) PRIMARY KEY,
			owner_type VARCHAR(16) NOT NULL,
			owner_key VARCHAR(64) NOT NULL,
			period_start BIGINT NOT NULL,
			files_reserved BIGINT NOT NULL,
			bytes_reserved BIGINT NOT NULL,
			publications_reserved BIGINT NOT NULL,
			expires_at BIGINT NOT NULL,
			created_at BIGINT NOT NULL
		);`,
	}
	for _, table := range tables {
		if _, err := db.Exec(table); err != nil {
			return err
		}
	}
	return nil
}

func initNPMTables(db *sql.DB) error {
	tables := [...]string{
		`CREATE TABLE IF NOT EXISTS npm_packages (
			repository VARCHAR(64) NOT NULL,
			package_name VARCHAR(214) NOT NULL,
			description TEXT NOT NULL,
			publisher VARCHAR(255) NOT NULL,
			latest_version VARCHAR(128) NOT NULL,
			super_team_prefix VARCHAR(64) NOT NULL DEFAULT '',
			private INT NOT NULL DEFAULT 0,
			archived INT NOT NULL DEFAULT 0,
			mirrored INT NOT NULL DEFAULT 0,
			publish_enabled INT NOT NULL DEFAULT 1,
			revision BIGINT NOT NULL DEFAULT 1,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (repository, package_name)
		);`,
		`CREATE TABLE IF NOT EXISTS npm_versions (
			repository VARCHAR(64) NOT NULL,
			package_name VARCHAR(214) NOT NULL,
			version VARCHAR(128) NOT NULL,
			manifest_json TEXT NOT NULL,
			publisher VARCHAR(255) NOT NULL,
			tarball_path VARCHAR(1024) NOT NULL,
			shasum CHAR(40) NOT NULL,
			integrity VARCHAR(255) NOT NULL,
			size BIGINT NOT NULL DEFAULT 0,
			deprecated TEXT NOT NULL,
			unpublished INT NOT NULL DEFAULT 0,
			mirrored INT NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			PRIMARY KEY (repository, package_name, version)
		);`,
		`CREATE TABLE IF NOT EXISTS npm_dist_tags (
			repository VARCHAR(64) NOT NULL,
			package_name VARCHAR(214) NOT NULL,
			tag VARCHAR(128) NOT NULL,
			version VARCHAR(128) NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (repository, package_name, tag)
		);`,
		`CREATE TABLE IF NOT EXISTS npm_members (
			repository VARCHAR(64) NOT NULL,
			package_name VARCHAR(214) NOT NULL,
			username VARCHAR(255) NOT NULL,
			user_id VARCHAR(36) NULL,
			permission_level INT NOT NULL,
			added_at BIGINT NOT NULL,
			PRIMARY KEY (repository, package_name, username),
			UNIQUE (repository, package_name, user_id)
		);`,
		`CREATE TABLE IF NOT EXISTS npm_invitations (
			id CHAR(36) PRIMARY KEY,
			repository VARCHAR(64) NOT NULL,
			package_name VARCHAR(214) NOT NULL,
			inviter VARCHAR(255) NOT NULL,
			recipient VARCHAR(255) NOT NULL,
			permission_level INT NOT NULL,
			created_at BIGINT NOT NULL,
			UNIQUE (repository, package_name, recipient)
		);`,
	}
	for _, table := range tables {
		if _, err := db.Exec(table); err != nil {
			return err
		}
	}
	return nil
}

func initMavenTables(db *sql.DB, readmeColumnDefinition string) error {
	if readmeColumnDefinition == "" {
		readmeColumnDefinition = "TEXT NOT NULL"
	}
	tables := [...]string{
		`CREATE TABLE IF NOT EXISTS maven_domains (
			repository VARCHAR(64) NOT NULL,
			domain VARCHAR(253) NOT NULL,
			verification_type VARCHAR(16) NOT NULL,
			verification_host VARCHAR(253) NOT NULL,
			verification_code VARCHAR(128) NOT NULL,
			super_team_prefix VARCHAR(64) NOT NULL DEFAULT '',
			verified INT NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			verified_at BIGINT NOT NULL DEFAULT 0,
			last_check_at BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (repository, domain)
		);`,
		`CREATE TABLE IF NOT EXISTS maven_domain_members (
			repository VARCHAR(64) NOT NULL,
			domain VARCHAR(253) NOT NULL,
			username VARCHAR(255) NOT NULL,
			user_id VARCHAR(36) NULL,
			permission_level INT NOT NULL,
			added_at BIGINT NOT NULL,
			PRIMARY KEY (repository, domain, username),
			UNIQUE (repository, domain, user_id)
		);`,
		`CREATE TABLE IF NOT EXISTS maven_domain_invitations (
			id CHAR(36) PRIMARY KEY,
			repository VARCHAR(64) NOT NULL,
			domain VARCHAR(253) NOT NULL,
			inviter VARCHAR(255) NOT NULL,
			recipient VARCHAR(255) NOT NULL,
			permission_level INT NOT NULL,
			created_at BIGINT NOT NULL,
			UNIQUE (repository, domain, recipient)
		);`,
		`CREATE TABLE IF NOT EXISTS maven_artifacts (
			repository VARCHAR(64) NOT NULL,
			domain VARCHAR(253) NOT NULL,
			group_id VARCHAR(253) NOT NULL,
			artifact_id VARCHAR(255) NOT NULL,
			description TEXT NOT NULL,
			readme ` + readmeColumnDefinition + `,
			publisher VARCHAR(255) NOT NULL,
			latest_version VARCHAR(255) NOT NULL,
			super_team_prefix VARCHAR(64) NOT NULL DEFAULT '',
			mirrored INT NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			PRIMARY KEY (repository, group_id, artifact_id)
		);`,
		`CREATE TABLE IF NOT EXISTS maven_versions (
			repository VARCHAR(64) NOT NULL,
			group_id VARCHAR(253) NOT NULL,
			artifact_id VARCHAR(255) NOT NULL,
			version VARCHAR(255) NOT NULL,
			publisher VARCHAR(255) NOT NULL,
			size BIGINT NOT NULL DEFAULT 0,
			mirrored INT NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL,
			PRIMARY KEY (repository, group_id, artifact_id, version)
		);`,
		`CREATE TABLE IF NOT EXISTS maven_repository_upgrades (
			repository VARCHAR(64) PRIMARY KEY,
			completed_at BIGINT NOT NULL
		);`,
	}
	for _, table := range tables {
		if _, err := db.Exec(table); err != nil {
			return err
		}
	}
	return nil
}

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
	Name       string
	Query      string
	MySQLQuery string
}

var sharedIndexMigrations = []SchemaMigration{
	{Name: "idx_sessions_username", Query: "CREATE INDEX IF NOT EXISTS idx_sessions_username ON sessions(username);"},
	{Name: "idx_sessions_last_active", Query: "CREATE INDEX IF NOT EXISTS idx_sessions_last_active ON sessions(last_active);"},
	{Name: "idx_sessions_user_public", Query: "CREATE INDEX IF NOT EXISTS idx_sessions_user_public ON sessions(username, public_id);"},
	{Name: "idx_tokens_expires_at", Query: "CREATE INDEX IF NOT EXISTS idx_tokens_expires_at ON tokens(expires_at) WHERE expires_at IS NOT NULL;"},
	{Name: "idx_fido_username", Query: "CREATE INDEX IF NOT EXISTS idx_fido_username ON fido_devices(username);"},
	{Name: "idx_fido_credential_id", Query: "CREATE INDEX IF NOT EXISTS idx_fido_credential_id ON fido_devices(credential_id);"},
	{Name: "idx_gpg_alias_fingerprint", Query: "CREATE INDEX IF NOT EXISTS idx_gpg_alias_fingerprint ON gpg_key_aliases(fingerprint);"},
	{Name: "idx_gpg_alias_identifier", Query: "CREATE INDEX IF NOT EXISTS idx_gpg_alias_identifier ON gpg_key_aliases(identifier);"},
	{Name: "idx_user_gpg_username", Query: "CREATE INDEX IF NOT EXISTS idx_user_gpg_username ON user_gpg_keys(username);"},
	{Name: "idx_gpg_signatures_repository", Query: "CREATE INDEX IF NOT EXISTS idx_gpg_signatures_repository ON gpg_signatures(repository);"},
	{Name: "idx_gpg_releases_user_time", Query: "CREATE INDEX IF NOT EXISTS idx_gpg_releases_user_time ON gpg_releases(uploader, created_at DESC);"},
	{Name: "idx_gpg_releases_queue", Query: "CREATE INDEX IF NOT EXISTS idx_gpg_releases_queue ON gpg_releases(status, created_at);"},
	{Name: "idx_gpg_releases_cleanup", Query: "CREATE INDEX IF NOT EXISTS idx_gpg_releases_cleanup ON gpg_releases(cleanup_pending, updated_at);"},
	{Name: "idx_audit_logs_user_time", Query: "CREATE INDEX IF NOT EXISTS idx_audit_logs_user_time ON audit_logs(username, created_at DESC);"},
	{Name: "idx_audit_logs_user_id", Query: "CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(username, id DESC);"},
	{Name: "idx_audit_logs_created_at", Query: "CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);"},
	{Name: "idx_user_messages_recipient_time", Query: "CREATE INDEX IF NOT EXISTS idx_user_messages_recipient_time ON user_messages(recipient, created_at DESC, id DESC);"},
	{Name: "idx_user_messages_unread", Query: "CREATE INDEX IF NOT EXISTS idx_user_messages_unread ON user_messages(recipient, read_at, expires_at);"},
	{Name: "idx_user_messages_action", Query: "CREATE INDEX IF NOT EXISTS idx_user_messages_action ON user_messages(action_kind, action_status, expires_at);"},
	{Name: "idx_cargo_packages_search", Query: "CREATE INDEX IF NOT EXISTS idx_cargo_packages_search ON cargo_packages(repository, archived, normalized_name);"},
	{Name: "idx_cargo_versions_package", Query: "CREATE INDEX IF NOT EXISTS idx_cargo_versions_package ON cargo_versions(repository, normalized_name, created_at DESC);"},
	{Name: "idx_cargo_members_user", Query: "CREATE INDEX IF NOT EXISTS idx_cargo_members_user ON cargo_members(username, repository);"},
	{Name: "idx_cargo_invitations_recipient", Query: "CREATE INDEX IF NOT EXISTS idx_cargo_invitations_recipient ON cargo_invitations(recipient, created_at);"},
	{Name: "idx_docker_tags_repo_img", Query: "CREATE INDEX IF NOT EXISTS idx_docker_tags_repo_img ON docker_tags(repository, image_name, updated_at DESC);"},
	{Name: "idx_docker_tags_digest", Query: "CREATE INDEX IF NOT EXISTS idx_docker_tags_digest ON docker_tags(repository, digest);"},
	{Name: "idx_docker_manifests_repo_img", Query: "CREATE INDEX IF NOT EXISTS idx_docker_manifests_repo_img ON docker_manifests(repository, image_name);"},
	{Name: "idx_docker_images_search", Query: "CREATE INDEX IF NOT EXISTS idx_docker_images_search ON docker_images(repository, image_name);"},
	{Name: "idx_docker_blobs_repo", Query: "CREATE INDEX IF NOT EXISTS idx_docker_blobs_repo ON docker_blobs(repository, digest);"},
	{Name: "idx_docker_members_user", Query: "CREATE INDEX IF NOT EXISTS idx_docker_members_user ON docker_members(username, repository);"},
	{Name: "idx_docker_invitations_recipient", Query: "CREATE INDEX IF NOT EXISTS idx_docker_invitations_recipient ON docker_invitations(recipient, created_at);"},
	{Name: "idx_npm_packages_search", Query: "CREATE INDEX IF NOT EXISTS idx_npm_packages_search ON npm_packages(repository, archived, package_name);"},
	{Name: "idx_npm_versions_package", Query: "CREATE INDEX IF NOT EXISTS idx_npm_versions_package ON npm_versions(repository, package_name, created_at DESC);"},
	{Name: "idx_npm_tags_package", Query: "CREATE INDEX IF NOT EXISTS idx_npm_tags_package ON npm_dist_tags(repository, package_name);"},
	{Name: "idx_npm_members_user", Query: "CREATE INDEX IF NOT EXISTS idx_npm_members_user ON npm_members(username, repository);"},
	{Name: "idx_npm_invitations_recipient", Query: "CREATE INDEX IF NOT EXISTS idx_npm_invitations_recipient ON npm_invitations(recipient, created_at);"},
	{Name: "idx_download_statistics_user", Query: "CREATE INDEX IF NOT EXISTS idx_download_statistics_user ON download_statistics(user_id, repository);"},
	{Name: "idx_download_statistics_repository", Query: "CREATE INDEX IF NOT EXISTS idx_download_statistics_repository ON download_statistics(repository, format);"},
	{Name: "idx_download_statistics_namespace", Query: "CREATE INDEX IF NOT EXISTS idx_download_statistics_namespace ON download_statistics(repository, namespace);"},
	{Name: "idx_download_statistics_package", Query: "CREATE INDEX IF NOT EXISTS idx_download_statistics_package ON download_statistics(repository, package_name);"},
	{Name: "idx_super_teams_creator", Query: "CREATE INDEX IF NOT EXISTS idx_super_teams_creator ON super_teams(created_by, prefix);"},
	{Name: "idx_super_team_members_user", Query: "CREATE INDEX IF NOT EXISTS idx_super_team_members_user ON super_team_members(user_id, team_prefix);"},
	{Name: "idx_super_team_invitations_recipient", Query: "CREATE INDEX IF NOT EXISTS idx_super_team_invitations_recipient ON super_team_invitations(recipient_id, expires_at);"},
	{Name: "idx_cargo_packages_super_team", Query: "CREATE INDEX IF NOT EXISTS idx_cargo_packages_super_team ON cargo_packages(super_team_prefix, repository);"},
	{Name: "idx_docker_images_super_team", Query: "CREATE INDEX IF NOT EXISTS idx_docker_images_super_team ON docker_images(super_team_prefix, repository);"},
	{Name: "idx_npm_packages_super_team", Query: "CREATE INDEX IF NOT EXISTS idx_npm_packages_super_team ON npm_packages(super_team_prefix, repository);"},
	{Name: "idx_maven_domains_super_team", Query: "CREATE INDEX IF NOT EXISTS idx_maven_domains_super_team ON maven_domains(super_team_prefix, domain);"},
	{Name: "idx_maven_artifacts_super_team", Query: "CREATE INDEX IF NOT EXISTS idx_maven_artifacts_super_team ON maven_artifacts(super_team_prefix, repository);"},
	{Name: "idx_review_tasks_team", Query: "CREATE INDEX IF NOT EXISTS idx_review_tasks_team ON review_tasks(review_team_prefix, status, kind, created_at);"},
	{Name: "idx_review_tasks_requester", Query: "CREATE INDEX IF NOT EXISTS idx_review_tasks_requester ON review_tasks(requested_by_id, status, created_at);"},
	{Name: "idx_review_task_files_task", Query: "CREATE INDEX IF NOT EXISTS idx_review_task_files_task ON review_task_files(task_id, added_at);"},
	{Name: "idx_publication_quota_reservation_owner", Query: "CREATE INDEX IF NOT EXISTS idx_publication_quota_reservation_owner ON publication_quota_reservations(owner_type, owner_key, period_start, expires_at);"},
	{Name: "idx_publication_quota_reservation_expiry", Query: "CREATE INDEX IF NOT EXISTS idx_publication_quota_reservation_expiry ON publication_quota_reservations(expires_at);"},
	{Name: "idx_publication_quota_usage_window", Query: "CREATE INDEX IF NOT EXISTS idx_publication_quota_usage_window ON publication_quota_usage(period_start);"},
}

func applySharedIndexMigrations(db *sql.DB) error {
	for _, migration := range sharedIndexMigrations {
		if _, err := db.Exec(migration.Query); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Name, err)
		}
	}
	return nil
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
	{Name: "cargo_packages.readme", Query: "ALTER TABLE cargo_packages ADD COLUMN readme TEXT;", MySQLQuery: "ALTER TABLE cargo_packages ADD COLUMN readme MEDIUMTEXT;"},
	{Name: "cargo_packages.mirrored", Query: "ALTER TABLE cargo_packages ADD COLUMN mirrored INT NOT NULL DEFAULT 0;"},
	{Name: "cargo_versions.size", Query: "ALTER TABLE cargo_versions ADD COLUMN size BIGINT NOT NULL DEFAULT 0;"},
	{Name: "cargo_versions.checksum", Query: "ALTER TABLE cargo_versions ADD COLUMN checksum VARCHAR(64) NOT NULL DEFAULT '';"},
	{Name: "cargo_versions.rust_version", Query: "ALTER TABLE cargo_versions ADD COLUMN rust_version VARCHAR(64) NOT NULL DEFAULT '';"},
	{Name: "cargo_versions.license", Query: "ALTER TABLE cargo_versions ADD COLUMN license VARCHAR(255) NOT NULL DEFAULT '';"},
	{Name: "cargo_versions.repository_url", Query: "ALTER TABLE cargo_versions ADD COLUMN repository_url VARCHAR(1024) NOT NULL DEFAULT '';"},
	{Name: "cargo_versions.homepage", Query: "ALTER TABLE cargo_versions ADD COLUMN homepage VARCHAR(1024) NOT NULL DEFAULT '';"},
	{Name: "cargo_versions.documentation", Query: "ALTER TABLE cargo_versions ADD COLUMN documentation VARCHAR(1024) NOT NULL DEFAULT '';"},
	{Name: "cargo_versions.mirrored", Query: "ALTER TABLE cargo_versions ADD COLUMN mirrored INT NOT NULL DEFAULT 0;"},
	{Name: "maven_artifacts.mirrored", Query: "ALTER TABLE maven_artifacts ADD COLUMN mirrored INT NOT NULL DEFAULT 0;"},
	{Name: "maven_artifacts.readme", Query: "ALTER TABLE maven_artifacts ADD COLUMN readme TEXT;", MySQLQuery: "ALTER TABLE maven_artifacts ADD COLUMN readme MEDIUMTEXT;"},
	{Name: "maven_versions.mirrored", Query: "ALTER TABLE maven_versions ADD COLUMN mirrored INT NOT NULL DEFAULT 0;"},
	{Name: "docker_images.publisher", Query: "ALTER TABLE docker_images ADD COLUMN publisher VARCHAR(255) NOT NULL DEFAULT '';"},
	{Name: "docker_images.pull_count", Query: "ALTER TABLE docker_images ADD COLUMN pull_count BIGINT NOT NULL DEFAULT 0;"},
	{Name: "docker_images.private", Query: "ALTER TABLE docker_images ADD COLUMN private INT NOT NULL DEFAULT 0;"},
	{Name: "docker_images.push_enabled", Query: "ALTER TABLE docker_images ADD COLUMN push_enabled INT NOT NULL DEFAULT 1;"},
	{Name: "docker_tags.publisher", Query: "ALTER TABLE docker_tags ADD COLUMN publisher VARCHAR(255) NOT NULL DEFAULT '';"},
	{Name: "docker_manifests.publisher", Query: "ALTER TABLE docker_manifests ADD COLUMN publisher VARCHAR(255) NOT NULL DEFAULT '';"},
	{Name: "cargo_packages.super_team_prefix", Query: "ALTER TABLE cargo_packages ADD COLUMN super_team_prefix VARCHAR(64) NOT NULL DEFAULT '';"},
	{Name: "docker_images.super_team_prefix", Query: "ALTER TABLE docker_images ADD COLUMN super_team_prefix VARCHAR(64) NOT NULL DEFAULT '';"},
	{Name: "npm_packages.super_team_prefix", Query: "ALTER TABLE npm_packages ADD COLUMN super_team_prefix VARCHAR(64) NOT NULL DEFAULT '';"},
	{Name: "maven_domains.super_team_prefix", Query: "ALTER TABLE maven_domains ADD COLUMN super_team_prefix VARCHAR(64) NOT NULL DEFAULT '';"},
	{Name: "maven_artifacts.super_team_prefix", Query: "ALTER TABLE maven_artifacts ADD COLUMN super_team_prefix VARCHAR(64) NOT NULL DEFAULT '';"},
	{Name: "user_profiles.website_url", Query: "ALTER TABLE user_profiles ADD COLUMN website_url VARCHAR(2048) NOT NULL DEFAULT '';"},
	{Name: "user_profiles.github_url", Query: "ALTER TABLE user_profiles ADD COLUMN github_url VARCHAR(2048) NOT NULL DEFAULT '';"},
	{Name: "user_profiles.discord_url", Query: "ALTER TABLE user_profiles ADD COLUMN discord_url VARCHAR(2048) NOT NULL DEFAULT '';"},
	{Name: "user_profiles.custom_link_name", Query: "ALTER TABLE user_profiles ADD COLUMN custom_link_name VARCHAR(160) NOT NULL DEFAULT '';"},
	{Name: "user_profiles.custom_link_url", Query: "ALTER TABLE user_profiles ADD COLUMN custom_link_url VARCHAR(2048) NOT NULL DEFAULT '';"},
	{Name: "super_teams.website_url", Query: "ALTER TABLE super_teams ADD COLUMN website_url VARCHAR(2048) NOT NULL DEFAULT '';"},
	{Name: "super_teams.github_url", Query: "ALTER TABLE super_teams ADD COLUMN github_url VARCHAR(2048) NOT NULL DEFAULT '';"},
	{Name: "super_teams.discord_url", Query: "ALTER TABLE super_teams ADD COLUMN discord_url VARCHAR(2048) NOT NULL DEFAULT '';"},
	{Name: "super_teams.custom_link_name", Query: "ALTER TABLE super_teams ADD COLUMN custom_link_name VARCHAR(160) NOT NULL DEFAULT '';"},
	{Name: "super_teams.custom_link_url", Query: "ALTER TABLE super_teams ADD COLUMN custom_link_url VARCHAR(2048) NOT NULL DEFAULT '';"},
	{Name: "super_team_members.public_visible", Query: "ALTER TABLE super_team_members ADD COLUMN public_visible INT NOT NULL DEFAULT 1;"},
	{Name: "user_api_tokens.disabled", Query: "ALTER TABLE user_api_tokens ADD COLUMN disabled INT NOT NULL DEFAULT 0;"},
	{Name: "tokens.ban_reason", Query: "ALTER TABLE tokens ADD COLUMN ban_reason VARCHAR(2048) NOT NULL DEFAULT '';"},
	{Name: "tokens.banned_at", Query: "ALTER TABLE tokens ADD COLUMN banned_at BIGINT NOT NULL DEFAULT 0;"},
	{Name: "tokens.banned_until", Query: "ALTER TABLE tokens ADD COLUMN banned_until BIGINT NULL;"},
	{Name: "user_profiles.user_id", Query: "ALTER TABLE user_profiles ADD COLUMN user_id VARCHAR(36) NULL;"},
	{Name: "cargo_members.user_id", Query: "ALTER TABLE cargo_members ADD COLUMN user_id VARCHAR(36) NULL;"},
	{Name: "docker_members.user_id", Query: "ALTER TABLE docker_members ADD COLUMN user_id VARCHAR(36) NULL;"},
}

func NewDialect(driver string) Dialect {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "clickhouse", "ch":
		return &ClickHouseDialect{}
	case "mysql":
		return &MySQLDialect{}
	case "postgres", "postgresql", "pgx", "pg":
		return &PostgresDialect{}
	default:
		return &SQLiteDialect{}
	}
}
