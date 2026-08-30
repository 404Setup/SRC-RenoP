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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClickHouseDialectUsesNativeMutationSyntax(t *testing.T) {
	dialect := &ClickHouseDialect{}
	assert.Equal(t, "clickhouse", dialect.Name())
	assert.Equal(t,
		"ALTER TABLE tokens UPDATE description = ? WHERE name = ?",
		dialect.Rebind("UPDATE tokens SET description = ? WHERE name = ?"),
	)
	assert.Contains(t, dialect.UpsertTokenQuery(), "renop:upsert")
	assert.NotContains(t, dialect.UpsertTokenQuery(), "ON CONFLICT")
}

func TestClickHouseSchemaHasUniqueMutableTableContracts(t *testing.T) {
	schemas := clickHouseSchemas()
	require.GreaterOrEqual(t, len(schemas), 40)
	seen := make(map[string]struct{}, len(schemas))
	for _, schema := range schemas {
		require.NotEmpty(t, schema.name)
		require.NotEmpty(t, schema.keyColumns, schema.name)
		_, duplicate := seen[schema.name]
		assert.Falsef(t, duplicate, "duplicate ClickHouse schema %s", schema.name)
		seen[schema.name] = struct{}{}
		query := schema.createQuery()
		assert.Contains(t, query, "ENGINE = EmbeddedRocksDB")
		assert.Contains(t, query, "MATERIALIZED")
	}
	assert.Contains(t, seen, clickHouseTransactionJournal)
	assert.Contains(t, seen, clickHouseTransactionKeys)
}

func TestClickHouseInsertKeyParserMatchesMaterializedEncoding(t *testing.T) {
	keys, err := clickHouseInsertKeys("cargo_versions", `INSERT INTO cargo_versions
		(repository, normalized_name, version, description, created_at) VALUES (?, ?, ?, '', ?)`,
		[]any{"cargo", "demo", "1.0.0", int64(1000)})
	require.NoError(t, err)
	require.Equal(t, []string{"S5:cargoS4:demoS5:1.0.0"}, keys)
	assert.Equal(t, "NS2:\\N", encodeClickHouseKey([]any{nil, `\N`}))
	where := findTopLevelSQLKeyword(`ALTER TABLE demo UPDATE value = if((SELECT count() FROM nested WHERE id = ?) > 0, ?, value)
		WHERE repository = ? AND name = ?`, "WHERE")
	require.Positive(t, where)
	assert.Equal(t, 2, countSQLPlaceholders(`value = if((SELECT count() FROM nested WHERE id = ?) > 0, ?, value)`))
}

func TestClickHousePortableValueConversion(t *testing.T) {
	var integer int
	require.NoError(t, assignClickHouseValue(&integer, uint64(42)))
	assert.Equal(t, 42, integer)
	var bytes []byte
	require.NoError(t, assignClickHouseValue(&bytes, "binary\x00value"))
	assert.Equal(t, []byte("binary\x00value"), bytes)
}
