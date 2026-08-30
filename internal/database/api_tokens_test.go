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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
)

func apiTokenDigest(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}

func TestAPITokenLifecycleAndLegacyMigration(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "api-tokens.db")
	openDatabase := func() *DB {
		db, err := InitDB(config.DatabaseConfig{
			Driver: "sqlite", Dsn: databasePath, MaxOpenConns: 2, MaxIdleConns: 1,
		})
		require.NoError(t, err)
		return db
	}

	db := openDatabase()
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "alice", Tokens: []string{"legacy-upload-secret"},
		CreatedAt: time.Now().UTC().Format(time.RFC3339), Permissions: []string{"base"},
	}))
	require.NoError(t, db.Close())
	db = openDatabase()
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	account, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Empty(t, account.Tokens)
	legacyCredential, err := db.GetAPITokenByHash(apiTokenDigest("legacy-upload-secret"), "alice")
	require.NoError(t, err)
	require.NotNil(t, legacyCredential)
	assert.Equal(t, "alice", legacyCredential.Account.Name)
	assert.ElementsMatch(t, legacyAPITokenScopes, legacyCredential.Token.Scopes)

	now := time.Now().UnixMilli()
	expiresAt := now + int64((24*time.Hour)/time.Millisecond)
	created := &core.APIToken{
		ID: uuid.NewString(), Name: "Automation", Scopes: []string{"repository:read", "repository:publish"},
		Targets: map[string][]string{"repository:publish": {"files"}}, CreatedAt: now, ExpiresAt: &expiresAt,
	}
	const secret = "rnp_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"
	require.NoError(t, db.CreateAPIToken("alice", created, apiTokenDigest(secret)))
	duplicate := &core.APIToken{
		ID: uuid.NewString(), Name: "automation", Scopes: []string{"repository:read"}, CreatedAt: now + 1,
	}
	require.ErrorIs(t, db.CreateAPIToken("alice", duplicate, apiTokenDigest(secret+"x")),
		core.ErrAPITokenNameExists)

	tokens, err := db.ListAPITokens("alice")
	require.NoError(t, err)
	require.Len(t, tokens, 2)
	credential, err := db.GetAPITokenByHash(apiTokenDigest(secret), "")
	require.NoError(t, err)
	require.NotNil(t, credential)
	assert.Equal(t, created.ID, credential.Token.ID)
	assert.Equal(t, expiresAt, *credential.Token.ExpiresAt)
	assert.Equal(t, []string{"files"}, credential.Token.Targets[core.APITokenScopeRepositoryPublish])
	wrongOwner, err := db.GetAPITokenByHash(apiTokenDigest(secret), "bobby")
	require.NoError(t, err)
	assert.Nil(t, wrongOwner)

	count, err := db.CountAPITokens("alice")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	counts, err := db.CountAPITokensByUsername()
	require.NoError(t, err)
	assert.Equal(t, 2, counts["alice"])
	var storedHash, storedAuthorization, legacyJSON string
	require.NoError(t, db.QueryRow(`SELECT secret_hash, scopes_json FROM user_api_tokens WHERE id = ?`, created.ID).
		Scan(&storedHash, &storedAuthorization))
	require.NoError(t, db.QueryRow(`SELECT tokens_json FROM tokens WHERE name = ?`, "alice").Scan(&legacyJSON))
	assert.Equal(t, apiTokenDigest(secret), storedHash)
	assert.NotContains(t, storedHash, secret)
	assert.Contains(t, storedAuthorization, `"targets"`)
	assert.Equal(t, "[]", legacyJSON)

	require.NoError(t, db.DeleteAPIToken("alice", created.ID))
	require.ErrorIs(t, db.DeleteAPIToken("alice", created.ID), core.ErrAPITokenNotFound)
	credential, err = db.GetAPITokenByHash(apiTokenDigest(secret), "alice")
	require.NoError(t, err)
	assert.Nil(t, credential)
}

func TestAPITokenLimitIsEnforced(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "api-token-limit.db"), MaxOpenConns: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", Permissions: []string{"base"}}))

	now := time.Now().UnixMilli()
	for index := range core.MaxAPITokensPerUser {
		name := fmt.Sprintf("Automation %02d", index)
		require.NoError(t, db.CreateAPIToken("alice", &core.APIToken{
			ID: uuid.NewString(), Name: name, Scopes: []string{core.APITokenScopeRepositoryRead},
			CreatedAt: now + int64(index),
		}, apiTokenDigest("limit-secret-"+name)))
	}
	err = db.CreateAPIToken("alice", &core.APIToken{
		ID: uuid.NewString(), Name: "One too many", Scopes: []string{core.APITokenScopeRepositoryRead},
		CreatedAt: now + core.MaxAPITokensPerUser,
	}, apiTokenDigest("limit-secret-overflow"))
	require.ErrorIs(t, err, core.ErrAPITokenLimit)
}
