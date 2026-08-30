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

// ClickHouseDialect keeps RenoP's portable placeholders while translating
// row updates to the mutation syntax supported by EmbeddedRocksDB tables.
type ClickHouseDialect struct{}

func (d *ClickHouseDialect) Name() string {
	return "clickhouse"
}

func (d *ClickHouseDialect) InitTables(_ *sql.DB) error {
	return nil
}

func (d *ClickHouseDialect) UpsertTokenQuery() string {
	return `/* renop:upsert */ INSERT INTO tokens
		(name, type, type_value, encrypted_secret, password_hash, tokens_json, created_at, description, expires_at, permissions_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
}

func (d *ClickHouseDialect) UpsertSessionQuery() string {
	return `/* renop:upsert */ INSERT INTO sessions
		(session_token, public_id, username, ip, user_agent, created_at, last_active, login_method)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
}

func (d *ClickHouseDialect) UpsertGPGPublicKeyQuery() string {
	return `/* renop:upsert */ INSERT INTO gpg_public_keys
		(fingerprint, key_id, primary_identity, public_key, key_created_at, key_expires_at, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
}

func (d *ClickHouseDialect) UpsertGPGSignatureQuery() string {
	return `/* renop:upsert */ INSERT INTO gpg_signatures
		(artifact_key, repository, artifact_path, fingerprint, key_id, primary_identity, uploader,
		signature_created_at, verified_at, hash_algorithm, public_key_algorithm)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
}

func (d *ClickHouseDialect) Rebind(query string) string {
	trimmed := strings.TrimSpace(query)
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(upper, "UPDATE ") {
		remainder := strings.TrimSpace(trimmed[len("UPDATE "):])
		if separator := strings.IndexAny(remainder, " \t\r\n"); separator > 0 {
			table := remainder[:separator]
			assignment := strings.TrimSpace(remainder[separator:])
			if strings.HasPrefix(strings.ToUpper(assignment), "SET ") {
				return "ALTER TABLE " + table + " UPDATE " + strings.TrimSpace(assignment[len("SET "):])
			}
		}
	}
	return query
}
