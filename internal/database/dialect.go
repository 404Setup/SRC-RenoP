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

func initMavenTables(db *sql.DB) error {
	tables := [...]string{
		`CREATE TABLE IF NOT EXISTS maven_domains (
			repository VARCHAR(64) NOT NULL,
			domain VARCHAR(253) NOT NULL,
			verification_type VARCHAR(16) NOT NULL,
			verification_host VARCHAR(253) NOT NULL,
			verification_code VARCHAR(128) NOT NULL,
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
			publisher VARCHAR(255) NOT NULL,
			latest_version VARCHAR(255) NOT NULL,
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
	{Name: "docker_images.pull_count", Query: "ALTER TABLE docker_images ADD COLUMN pull_count BIGINT NOT NULL DEFAULT 0;"},
	{Name: "docker_images.private", Query: "ALTER TABLE docker_images ADD COLUMN private INT NOT NULL DEFAULT 0;"},
	{Name: "docker_images.push_enabled", Query: "ALTER TABLE docker_images ADD COLUMN push_enabled INT NOT NULL DEFAULT 1;"},
	{Name: "docker_tags.publisher", Query: "ALTER TABLE docker_tags ADD COLUMN publisher VARCHAR(255) NOT NULL DEFAULT '';"},
	{Name: "docker_manifests.publisher", Query: "ALTER TABLE docker_manifests ADD COLUMN publisher VARCHAR(255) NOT NULL DEFAULT '';"},
	{Name: "user_profiles.user_id", Query: "ALTER TABLE user_profiles ADD COLUMN user_id VARCHAR(36) NULL;"},
	{Name: "cargo_members.user_id", Query: "ALTER TABLE cargo_members ADD COLUMN user_id VARCHAR(36) NULL;"},
	{Name: "docker_members.user_id", Query: "ALTER TABLE docker_members ADD COLUMN user_id VARCHAR(36) NULL;"},
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
