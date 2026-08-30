/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database

import (
	"context"
	"fmt"
	"strings"

	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

const (
	clickHouseTransactionJournal = "_renop_transaction_journal"
	clickHouseTransactionKeys    = "_renop_transaction_keys"
)

type clickHouseTableSchema struct {
	name       string
	keyColumns []string
	columns    []string
}

func clickHouseIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

func clickHouseKeyExpression(columns []string) string {
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted := "`" + strings.ReplaceAll(column, "`", "``") + "`"
		value := "ifNull(toString(" + quoted + "), '')"
		parts = append(parts, "if(isNull("+quoted+"), 'N', concat('S', toString(length("+value+")), ':', "+value+"))")
	}
	return "concat(" + strings.Join(parts, ", ") + ")"
}

func (schema clickHouseTableSchema) createQuery() string {
	return schema.createQueryForName(schema.name)
}

func (schema clickHouseTableSchema) createQueryForName(name string) string {
	columns := append([]string(nil), schema.columns...)
	columns = append(columns, "`_renop_key` String MATERIALIZED "+clickHouseKeyExpression(schema.keyColumns))
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` (\n%s\n) ENGINE = EmbeddedRocksDB PRIMARY KEY _renop_key SETTINGS optimize_for_bulk_insert = 0",
		strings.ReplaceAll(name, "`", "``"), "\t"+strings.Join(columns, ",\n\t"))
}

func (schema clickHouseTableSchema) dataColumns() ([]string, error) {
	columns := make([]string, 0, len(schema.columns))
	for _, definition := range schema.columns {
		start := strings.IndexByte(definition, '`')
		if start < 0 {
			return nil, fmt.Errorf("ClickHouse column definition for %s is invalid", schema.name)
		}
		end := strings.IndexByte(definition[start+1:], '`')
		if end < 0 {
			return nil, fmt.Errorf("ClickHouse column definition for %s is invalid", schema.name)
		}
		columns = append(columns, definition[start+1:start+1+end])
	}
	return columns, nil
}

func clickHouseSchemas() []clickHouseTableSchema {
	return []clickHouseTableSchema{
		{name: "tokens", keyColumns: []string{"name"}, columns: []string{
			"`name` String", "`type` String", "`type_value` Int64", "`encrypted_secret` String",
			"`password_hash` String", "`tokens_json` String", "`created_at` String", "`description` String",
			"`expires_at` Nullable(Int64)", "`permissions_json` String",
		}},
		{name: "user_profiles", keyColumns: []string{"user_id"}, columns: []string{
			"`user_id` String", "`username` String", "`nickname` String DEFAULT ''",
			"`rename_window_started_at` Int64 DEFAULT 0", "`rename_count` Int64 DEFAULT 0", "`updated_at` Int64 DEFAULT 0",
		}},
		{name: "sessions", keyColumns: []string{"session_token"}, columns: []string{
			"`session_token` String", "`public_id` String", "`username` String", "`ip` String", "`user_agent` String",
			"`created_at` Int64", "`last_active` Int64", "`login_method` String DEFAULT 'password'",
		}},
		{name: "fido_devices", keyColumns: []string{"id"}, columns: []string{
			"`id` String", "`username` String", "`name` String", "`credential_id` String", "`public_key` String",
			"`attestation_type` String", "`aaguid` String", "`sign_count` Int64", "`created_at` Int64",
			"`user_present` Int64 DEFAULT 0", "`user_verified` Int64 DEFAULT 0",
			"`backup_eligible` Int64 DEFAULT 0", "`backup_state` Int64 DEFAULT 0",
		}},
		{name: "gpg_public_keys", keyColumns: []string{"fingerprint"}, columns: []string{
			"`fingerprint` String", "`key_id` String", "`primary_identity` String", "`public_key` String",
			"`key_created_at` Int64", "`key_expires_at` Int64 DEFAULT 0", "`fetched_at` Int64",
		}},
		{name: "gpg_key_aliases", keyColumns: []string{"identifier", "fingerprint"}, columns: []string{
			"`identifier` String", "`fingerprint` String",
		}},
		{name: "user_gpg_keys", keyColumns: []string{"username", "fingerprint"}, columns: []string{
			"`username` String", "`fingerprint` String", "`requested_id` String", "`added_at` Int64",
		}},
		{name: "gpg_signatures", keyColumns: []string{"artifact_key"}, columns: []string{
			"`artifact_key` String", "`repository` String", "`artifact_path` String", "`fingerprint` String",
			"`key_id` String", "`primary_identity` String", "`uploader` String", "`signature_created_at` Int64",
			"`verified_at` Int64", "`hash_algorithm` String", "`public_key_algorithm` String",
		}},
		{name: "gpg_releases", keyColumns: []string{"id"}, columns: []string{
			"`id` String", "`active_key` Nullable(String)", "`repository` String", "`artifact_path` String",
			"`uploader` String", "`status` String", "`failure_reason` String DEFAULT ''",
			"`require_signature` Int64 DEFAULT 0", "`artifact_staging_path` String DEFAULT ''",
			"`signature_staging_path` String DEFAULT ''", "`artifact_size` Int64 DEFAULT 0",
			"`artifact_mod_time` Int64 DEFAULT 0", "`signature_size` Int64 DEFAULT 0",
			"`signature_mod_time` Int64 DEFAULT 0", "`artifact_existed` Int64 DEFAULT 0",
			"`signature_existed` Int64 DEFAULT 0", "`artifact_generate_checksums` Int64 DEFAULT 0",
			"`signature_generate_checksums` Int64 DEFAULT 0", "`artifact_md5` String DEFAULT ''",
			"`artifact_sha1` String DEFAULT ''", "`artifact_sha256` String DEFAULT ''", "`artifact_sha512` String DEFAULT ''",
			"`signature_md5` String DEFAULT ''", "`signature_sha1` String DEFAULT ''",
			"`signature_sha256` String DEFAULT ''", "`signature_sha512` String DEFAULT ''",
			"`publish_started` Int64 DEFAULT 0", "`created_at` Int64", "`updated_at` Int64",
			"`completed_at` Int64 DEFAULT 0", "`cleanup_pending` Int64 DEFAULT 0",
		}},
		{name: "audit_logs", keyColumns: []string{"id"}, columns: []string{
			"`id` UInt64 DEFAULT toUnixTimestamp64Nano(now64(9))", "`username` String", "`operator` String",
			"`action` String", "`details` String", "`auth_method` String", "`session_id` String DEFAULT ''",
			"`ip` String", "`created_at` Int64",
		}},
		{name: "user_messages", keyColumns: []string{"id"}, columns: []string{
			"`id` String", "`recipient` String", "`sender` String", "`kind` String", "`severity` String",
			"`title` String", "`body` String", "`payload_json` String DEFAULT '{}'", "`action_kind` String DEFAULT ''",
			"`action_status` String DEFAULT ''", "`created_at` Int64", "`read_at` Int64 DEFAULT 0",
			"`acted_at` Int64 DEFAULT 0", "`expires_at` Int64 DEFAULT 0", "`dedupe_key` Nullable(String)",
		}},
		{name: "cargo_packages", keyColumns: []string{"repository", "normalized_name"}, columns: []string{
			"`repository` String", "`normalized_name` String", "`package_name` String", "`description` String DEFAULT ''",
			"`readme` String DEFAULT ''", "`repository_url` String DEFAULT ''", "`homepage` String DEFAULT ''",
			"`documentation` String DEFAULT ''", "`super_team_prefix` String DEFAULT ''",
			"`archived` Int64 DEFAULT 0", "`admin_archived` Int64 DEFAULT 0",
			"`mirrored` Int64 DEFAULT 0", "`created_at` Int64", "`updated_at` Int64",
		}},
		{name: "cargo_versions", keyColumns: []string{"repository", "normalized_name", "version"}, columns: []string{
			"`repository` String", "`normalized_name` String", "`version` String", "`description` String DEFAULT ''",
			"`publisher` String", "`size` Int64 DEFAULT 0", "`checksum` String DEFAULT ''", "`rust_version` String DEFAULT ''",
			"`license` String DEFAULT ''", "`repository_url` String DEFAULT ''", "`homepage` String DEFAULT ''",
			"`documentation` String DEFAULT ''", "`yanked` Int64 DEFAULT 0", "`admin_yanked` Int64 DEFAULT 0",
			"`archive_yanked` Int64 DEFAULT 0", "`mirrored` Int64 DEFAULT 0", "`created_at` Int64",
		}},
		{name: "cargo_members", keyColumns: []string{"repository", "normalized_name", "user_id"}, columns: []string{
			"`repository` String", "`normalized_name` String", "`username` String", "`user_id` Nullable(String)",
			"`permission_level` Int64", "`added_at` Int64",
		}},
		{name: "cargo_invitations", keyColumns: []string{"id"}, columns: []string{
			"`id` String", "`repository` String", "`normalized_name` String", "`package_name` String",
			"`inviter` String", "`recipient` String", "`permission_level` Int64", "`created_at` Int64",
		}},
		{name: "docker_images", keyColumns: []string{"repository", "image_name"}, columns: []string{
			"`repository` String", "`image_name` String", "`description` String DEFAULT ''", "`publisher` String DEFAULT ''",
			"`pull_count` Int64 DEFAULT 0", "`super_team_prefix` String DEFAULT ''",
			"`private` Int64 DEFAULT 0", "`push_enabled` Int64 DEFAULT 1",
			"`created_at` Int64", "`updated_at` Int64",
		}},
		{name: "docker_tags", keyColumns: []string{"repository", "image_name", "tag"}, columns: []string{
			"`repository` String", "`image_name` String", "`tag` String", "`digest` String", "`media_type` String DEFAULT ''",
			"`size` Int64 DEFAULT 0", "`config_digest` String DEFAULT ''", "`publisher` String DEFAULT ''",
			"`created_at` Int64", "`updated_at` Int64",
		}},
		{name: "docker_manifests", keyColumns: []string{"repository", "image_name", "digest"}, columns: []string{
			"`repository` String", "`image_name` String", "`digest` String", "`media_type` String DEFAULT ''",
			"`size` Int64 DEFAULT 0", "`config_digest` String DEFAULT ''", "`publisher` String DEFAULT ''",
			"`raw_json` String", "`created_at` Int64",
		}},
		{name: "docker_blobs", keyColumns: []string{"repository", "digest"}, columns: []string{
			"`repository` String", "`digest` String", "`size` Int64 DEFAULT 0", "`created_at` Int64",
		}},
		{name: "docker_members", keyColumns: []string{"repository", "image_name", "user_id"}, columns: []string{
			"`repository` String", "`image_name` String", "`username` String", "`user_id` Nullable(String)",
			"`permission_level` Int64", "`added_at` Int64",
		}},
		{name: "docker_invitations", keyColumns: []string{"id"}, columns: []string{
			"`id` String", "`repository` String", "`image_name` String", "`inviter` String", "`recipient` String",
			"`permission_level` Int64", "`created_at` Int64",
		}},
		{name: "docker_image_blobs", keyColumns: []string{"repository", "image_name", "manifest_digest", "blob_digest"}, columns: []string{
			"`repository` String", "`image_name` String", "`manifest_digest` String", "`blob_digest` String",
		}},
		{name: "github_identities", keyColumns: []string{"github_user_id"}, columns: []string{
			"`github_user_id` Int64", "`user_id` String", "`github_login` String", "`authorized_at` Int64",
		}},
		{name: "github_principals", keyColumns: []string{"user_id", "principal_type", "github_principal_id"}, columns: []string{
			"`user_id` String", "`principal_type` String", "`github_principal_id` Int64", "`github_login` String", "`authorized_at` Int64",
		}},
		{name: "user_account_security", keyColumns: []string{"user_id"}, columns: []string{
			"`user_id` String", "`email` Nullable(String)", "`password_login_enabled` Int64 DEFAULT 1", "`updated_at` Int64 DEFAULT 0",
		}},
		{name: "user_recovery_codes", keyColumns: []string{"user_id", "selector_hash"}, columns: []string{
			"`user_id` String", "`selector_hash` String", "`password_hash` String", "`created_at` Int64", "`used_at` Int64 DEFAULT 0",
		}},
		{name: "user_api_tokens", keyColumns: []string{"id"}, columns: []string{
			"`id` String", "`user_id` String", "`name` String", "`secret_hash` String", "`scopes_json` String",
			"`created_at` Int64", "`expires_at` Nullable(Int64)",
		}},
		{name: "download_statistics", keyColumns: []string{"id"}, columns: []string{
			"`id` String", "`user_id` String", "`username` String", "`repository` String", "`format` String",
			"`namespace` String", "`package_name` String", "`version` String", "`download_count` Int64 DEFAULT 0",
			"`download_bytes` Int64 DEFAULT 0", "`updated_at` Int64",
		}},
		{name: "super_teams", keyColumns: []string{"prefix"}, columns: []string{
			"`prefix` String", "`name` String", "`description` String", "`created_by` String", "`created_by_name` String",
			"`created_at` Int64", "`updated_at` Int64",
		}},
		{name: "super_team_members", keyColumns: []string{"team_prefix", "user_id"}, columns: []string{
			"`team_prefix` String", "`user_id` String", "`role_level` Int64", "`added_at` Int64",
		}},
		{name: "super_team_invitations", keyColumns: []string{"id"}, columns: []string{
			"`id` String", "`team_prefix` String", "`inviter_id` String", "`recipient_id` String",
			"`role_level` Int64", "`created_at` Int64", "`expires_at` Int64",
		}},
		{name: "user_super_team_limits", keyColumns: []string{"user_id"}, columns: []string{
			"`user_id` String", "`create_limit` Int64 DEFAULT -1", "`join_limit` Int64 DEFAULT -1", "`updated_at` Int64",
		}},
		{name: "review_tasks", keyColumns: []string{"id"}, columns: []string{
			"`id` String", "`kind` String", "`resource_type` String", "`repository` String DEFAULT ''",
			"`resource_key` String", "`resource_name` String", "`source_team_prefix` String DEFAULT ''",
			"`target_team_prefix` String DEFAULT ''", "`review_team_prefix` String", "`requested_by_id` String",
			"`requested_by_name` String", "`status` String", "`decision_reason` String DEFAULT ''",
			"`decided_by_id` String DEFAULT ''", "`decided_by_name` String DEFAULT ''", "`created_at` Int64",
			"`decided_at` Int64 DEFAULT 0", "`active_key` Nullable(String)",
		}},
		{name: "npm_packages", keyColumns: []string{"repository", "package_name"}, columns: []string{
			"`repository` String", "`package_name` String", "`description` String", "`publisher` String",
			"`latest_version` String", "`super_team_prefix` String DEFAULT ''", "`private` Int64 DEFAULT 0",
			"`archived` Int64 DEFAULT 0", "`mirrored` Int64 DEFAULT 0",
			"`publish_enabled` Int64 DEFAULT 1", "`revision` Int64 DEFAULT 1", "`created_at` Int64", "`updated_at` Int64",
		}},
		{name: "npm_versions", keyColumns: []string{"repository", "package_name", "version"}, columns: []string{
			"`repository` String", "`package_name` String", "`version` String", "`manifest_json` String", "`publisher` String",
			"`tarball_path` String", "`shasum` String", "`integrity` String", "`size` Int64 DEFAULT 0",
			"`deprecated` String", "`unpublished` Int64 DEFAULT 0", "`mirrored` Int64 DEFAULT 0", "`created_at` Int64",
		}},
		{name: "npm_dist_tags", keyColumns: []string{"repository", "package_name", "tag"}, columns: []string{
			"`repository` String", "`package_name` String", "`tag` String", "`version` String", "`updated_at` Int64",
		}},
		{name: "npm_members", keyColumns: []string{"repository", "package_name", "user_id"}, columns: []string{
			"`repository` String", "`package_name` String", "`username` String", "`user_id` Nullable(String)",
			"`permission_level` Int64", "`added_at` Int64",
		}},
		{name: "npm_invitations", keyColumns: []string{"id"}, columns: []string{
			"`id` String", "`repository` String", "`package_name` String", "`inviter` String", "`recipient` String",
			"`permission_level` Int64", "`created_at` Int64",
		}},
		{name: "maven_domains", keyColumns: []string{"repository", "domain"}, columns: []string{
			"`repository` String", "`domain` String", "`verification_type` String", "`verification_host` String",
			"`verification_code` String", "`super_team_prefix` String DEFAULT ''", "`verified` Int64 DEFAULT 0", "`created_at` Int64",
			"`verified_at` Int64 DEFAULT 0", "`last_check_at` Int64 DEFAULT 0",
		}},
		{name: "maven_domain_members", keyColumns: []string{"repository", "domain", "user_id"}, columns: []string{
			"`repository` String", "`domain` String", "`username` String", "`user_id` Nullable(String)",
			"`permission_level` Int64", "`added_at` Int64",
		}},
		{name: "maven_domain_invitations", keyColumns: []string{"id"}, columns: []string{
			"`id` String", "`repository` String", "`domain` String", "`inviter` String", "`recipient` String",
			"`permission_level` Int64", "`created_at` Int64",
		}},
		{name: "maven_artifacts", keyColumns: []string{"repository", "group_id", "artifact_id"}, columns: []string{
			"`repository` String", "`domain` String", "`group_id` String", "`artifact_id` String", "`description` String",
			"`readme` String", "`publisher` String", "`latest_version` String", "`super_team_prefix` String DEFAULT ''",
			"`mirrored` Int64 DEFAULT 0",
			"`created_at` Int64", "`updated_at` Int64",
		}},
		{name: "maven_versions", keyColumns: []string{"repository", "group_id", "artifact_id", "version"}, columns: []string{
			"`repository` String", "`group_id` String", "`artifact_id` String", "`version` String", "`publisher` String",
			"`size` Int64 DEFAULT 0", "`mirrored` Int64 DEFAULT 0", "`created_at` Int64",
		}},
		{name: "maven_repository_upgrades", keyColumns: []string{"repository"}, columns: []string{
			"`repository` String", "`completed_at` Int64",
		}},
		{name: clickHouseTransactionJournal, keyColumns: []string{"transaction_id", "table_name"}, columns: []string{
			"`transaction_id` String", "`table_name` String", "`snapshot_name` String", "`created_at` Int64",
		}},
		{name: clickHouseTransactionKeys, keyColumns: []string{"transaction_id", "table_name", "row_key"}, columns: []string{
			"`transaction_id` String", "`table_name` String", "`row_key` String", "`created_at` Int64",
		}},
	}
}

func clickHouseTableExists(ctx context.Context, conn chdriver.Conn, database, table string) (bool, error) {
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT count() > 0 FROM system.tables WHERE database = ? AND name = ?`,
		database, table).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func clickHouseTableColumns(ctx context.Context, conn chdriver.Conn, database, table string) ([]string, error) {
	rows, err := conn.Query(ctx, `SELECT name FROM system.columns WHERE database = ? AND table = ? ORDER BY position`,
		database, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make([]string, 0)
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, err
		}
		if column != "_renop_key" {
			columns = append(columns, column)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func clickHouseSchemaComplete(current, expected []string) bool {
	available := make(map[string]struct{}, len(current))
	for _, column := range current {
		available[column] = struct{}{}
	}
	for _, column := range expected {
		if _, exists := available[column]; !exists {
			return false
		}
	}
	return true
}

func recoverClickHouseSchemaMigration(ctx context.Context, conn chdriver.Conn, database string,
	schema clickHouseTableSchema, expected []string,
) error {
	temporary := "_renop_schema_" + schema.name + "_new"
	backup := "_renop_schema_" + schema.name + "_backup"
	sourceExists, err := clickHouseTableExists(ctx, conn, database, schema.name)
	if err != nil {
		return err
	}
	backupExists, err := clickHouseTableExists(ctx, conn, database, backup)
	if err != nil {
		return err
	}
	if !sourceExists && backupExists {
		if err := conn.Exec(ctx, "RENAME TABLE "+clickHouseIdentifier(backup)+" TO "+clickHouseIdentifier(schema.name)); err != nil {
			return fmt.Errorf("restore interrupted ClickHouse schema migration for %s: %w", schema.name, err)
		}
		sourceExists = true
		backupExists = false
	}
	temporaryExists, err := clickHouseTableExists(ctx, conn, database, temporary)
	if err != nil {
		return err
	}
	if temporaryExists {
		if err := conn.Exec(ctx, "DROP TABLE "+clickHouseIdentifier(temporary)); err != nil {
			return fmt.Errorf("discard interrupted ClickHouse schema copy for %s: %w", schema.name, err)
		}
	}
	if sourceExists && backupExists {
		columns, columnErr := clickHouseTableColumns(ctx, conn, database, schema.name)
		if columnErr != nil {
			return columnErr
		}
		if !clickHouseSchemaComplete(columns, expected) {
			return fmt.Errorf("ClickHouse schema migration for %s has two incomplete source tables", schema.name)
		}
		if err := conn.Exec(ctx, "DROP TABLE "+clickHouseIdentifier(backup)); err != nil {
			return fmt.Errorf("remove completed ClickHouse schema backup for %s: %w", schema.name, err)
		}
	}
	return nil
}

func migrateClickHouseTableSchema(ctx context.Context, conn chdriver.Conn, database string,
	schema clickHouseTableSchema, expected []string,
) error {
	current, err := clickHouseTableColumns(ctx, conn, database, schema.name)
	if err != nil {
		return err
	}
	if clickHouseSchemaComplete(current, expected) {
		return nil
	}
	expectedSet := make(map[string]struct{}, len(expected))
	for _, column := range expected {
		expectedSet[column] = struct{}{}
	}
	for _, column := range current {
		if _, exists := expectedSet[column]; !exists {
			return fmt.Errorf("ClickHouse table %s contains unsupported column %s", schema.name, column)
		}
	}
	temporary := "_renop_schema_" + schema.name + "_new"
	backup := "_renop_schema_" + schema.name + "_backup"
	if err := conn.Exec(ctx, schema.createQueryForName(temporary)); err != nil {
		return fmt.Errorf("create ClickHouse schema copy for %s: %w", schema.name, err)
	}
	quotedColumns := make([]string, len(current))
	for index, column := range current {
		quotedColumns[index] = clickHouseIdentifier(column)
	}
	if len(quotedColumns) > 0 {
		columnList := strings.Join(quotedColumns, ", ")
		if err := conn.Exec(ctx, "INSERT INTO "+clickHouseIdentifier(temporary)+" ("+columnList+") SELECT "+
			columnList+" FROM "+clickHouseIdentifier(schema.name)); err != nil {
			return fmt.Errorf("copy ClickHouse rows for %s: %w", schema.name, err)
		}
	}
	var sourceCount, copiedCount uint64
	if err := conn.QueryRow(ctx, "SELECT count() FROM "+clickHouseIdentifier(schema.name)).Scan(&sourceCount); err != nil {
		return fmt.Errorf("count ClickHouse source rows for %s: %w", schema.name, err)
	}
	if err := conn.QueryRow(ctx, "SELECT count() FROM "+clickHouseIdentifier(temporary)).Scan(&copiedCount); err != nil {
		return fmt.Errorf("count ClickHouse copied rows for %s: %w", schema.name, err)
	}
	if sourceCount != copiedCount {
		return fmt.Errorf("ClickHouse schema copy for %s is incomplete: copied %d of %d rows",
			schema.name, copiedCount, sourceCount)
	}
	if err := conn.Exec(ctx, "RENAME TABLE "+clickHouseIdentifier(schema.name)+" TO "+clickHouseIdentifier(backup)+", "+
		clickHouseIdentifier(temporary)+" TO "+clickHouseIdentifier(schema.name)); err != nil {
		return fmt.Errorf("activate ClickHouse schema migration for %s: %w", schema.name, err)
	}
	if err := conn.Exec(ctx, "DROP TABLE "+clickHouseIdentifier(backup)); err != nil {
		return fmt.Errorf("remove ClickHouse schema backup for %s: %w", schema.name, err)
	}
	return nil
}

func initializeClickHouseSchema(ctx context.Context, conn chdriver.Conn, database string) error {
	for _, schema := range clickHouseSchemas() {
		expected, err := schema.dataColumns()
		if err != nil {
			return err
		}
		if err := recoverClickHouseSchemaMigration(ctx, conn, database, schema, expected); err != nil {
			return fmt.Errorf("recover ClickHouse table %s: %w", schema.name, err)
		}
		if err := conn.Exec(ctx, schema.createQuery()); err != nil {
			return fmt.Errorf("create ClickHouse table %s: %w", schema.name, err)
		}
		if err := migrateClickHouseTableSchema(ctx, conn, database, schema, expected); err != nil {
			return fmt.Errorf("migrate ClickHouse table %s: %w", schema.name, err)
		}
	}
	return nil
}
