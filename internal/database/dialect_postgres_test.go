/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"renop/internal/database"
)

func TestPostgresDialect(t *testing.T) {
	d := database.NewDialect("postgres")
	assert.Equal(t, "postgres", d.Name())

	dPgx := database.NewDialect("pgx")
	assert.Equal(t, "postgres", dPgx.Name())

	dPostgresql := database.NewDialect("postgresql")
	assert.Equal(t, "postgres", dPostgresql.Name())

	assert.Contains(t, d.UpsertTokenQuery(), "ON CONFLICT(name) DO UPDATE SET")
	assert.Contains(t, d.UpsertSessionQuery(), "ON CONFLICT(session_token) DO UPDATE SET")
	assert.Contains(t, d.UpsertGPGPublicKeyQuery(), "ON CONFLICT(fingerprint) DO UPDATE SET")
	assert.Contains(t, d.UpsertGPGSignatureQuery(), "ON CONFLICT(artifact_key) DO UPDATE SET")
}

func TestRebindPostgres(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "No placeholders",
			input:    "SELECT * FROM tokens",
			expected: "SELECT * FROM tokens",
		},
		{
			name:     "Single placeholder",
			input:    "SELECT * FROM tokens WHERE name = ?",
			expected: "SELECT * FROM tokens WHERE name = $1",
		},
		{
			name:     "Multiple placeholders",
			input:    "SELECT * FROM tokens WHERE name = ? AND type = ? AND type_value = ?",
			expected: "SELECT * FROM tokens WHERE name = $1 AND type = $2 AND type_value = $3",
		},
		{
			name:     "Placeholder in single quote string literal ignored",
			input:    "SELECT * FROM tokens WHERE name = 'user?1' AND id = ?",
			expected: "SELECT * FROM tokens WHERE name = 'user?1' AND id = $1",
		},
		{
			name:     "Escaped single quotes in string literal",
			input:    "SELECT * FROM tokens WHERE name = 'O''Reilly?1' AND id = ?",
			expected: "SELECT * FROM tokens WHERE name = 'O''Reilly?1' AND id = $1",
		},
		{
			name:     "Placeholder in double quote identifier ignored",
			input:    `SELECT "col?1" FROM tokens WHERE id = ?`,
			expected: `SELECT "col?1" FROM tokens WHERE id = $1`,
		},
		{
			name:     "Placeholder in line comment ignored",
			input:    "SELECT * FROM tokens WHERE id = ? -- comment with ?\nAND name = ?",
			expected: "SELECT * FROM tokens WHERE id = $1 -- comment with ?\nAND name = $2",
		},
		{
			name:     "Placeholder in block comment ignored",
			input:    "SELECT * FROM tokens WHERE id = ? /* comment with ? */ AND name = ?",
			expected: "SELECT * FROM tokens WHERE id = $1 /* comment with ? */ AND name = $2",
		},
		{
			name:     "Complex IN clause",
			input:    "SELECT * FROM cargo_versions WHERE repository = ? AND normalized_name IN (?, ?, ?)",
			expected: "SELECT * FROM cargo_versions WHERE repository = $1 AND normalized_name IN ($2, $3, $4)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := database.RebindPostgres(tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
