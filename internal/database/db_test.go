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
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func TestDialects(t *testing.T) {
	sqliteDialect := database.NewDialect("sqlite3")
	assert.Equal(t, "sqlite3", sqliteDialect.Name())
	assert.Contains(t, sqliteDialect.UpsertTokenQuery(), "ON CONFLICT")
	assert.Contains(t, sqliteDialect.UpsertGPGPublicKeyQuery(), "ON CONFLICT")
	assert.Contains(t, sqliteDialect.UpsertGPGSignatureQuery(), "ON CONFLICT")

	mysqlDialect := database.NewDialect("mysql")
	assert.Equal(t, "mysql", mysqlDialect.Name())
	assert.Contains(t, mysqlDialect.UpsertTokenQuery(), "ON DUPLICATE KEY UPDATE")
	assert.Contains(t, mysqlDialect.UpsertGPGPublicKeyQuery(), "ON DUPLICATE KEY UPDATE")
	assert.Contains(t, mysqlDialect.UpsertGPGSignatureQuery(), "ON DUPLICATE KEY UPDATE")

	pgDialect := database.NewDialect("postgres")
	assert.Equal(t, "postgres", pgDialect.Name())
	assert.Contains(t, pgDialect.UpsertTokenQuery(), "ON CONFLICT")
	assert.Contains(t, pgDialect.UpsertGPGPublicKeyQuery(), "ON CONFLICT")
	assert.Contains(t, pgDialect.UpsertGPGSignatureQuery(), "ON CONFLICT")
}

func TestSQLiteMigrationsFailOnInvalidSchema(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec("CREATE TABLE sessions (invalid_column TEXT)")
	require.NoError(t, err)

	err = database.NewDialect("sqlite").InitTables(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "idx_sessions_username")
}

func TestInitDB_SQLite(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "test_renop.db")

	cfg := config.DatabaseConfig{
		Driver:       "sqlite3",
		Dsn:          dbFile,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	}

	db, err := database.InitDB(cfg)
	require.NoError(t, err)
	require.NotNil(t, db)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	t.Run("Token Operations with TTL Cache", func(t *testing.T) {
		exp := time.Now().Add(time.Hour).UnixMilli()
		tok := &core.AccessToken{
			Identifier: core.AccessTokenIdentifier{
				Type:  core.Persistent,
				Value: 1,
			},
			Name:            "Admin",
			EncryptedSecret: "$2a$10$hashedsecret",
			PasswordHash:    "hash",
			Tokens:          []string{"token1", "token2"},
			CreatedAt:       "2026-07-31T00:00:00Z",
			Description:     "Test token",
			ExpiresAt:       &exp,
			Permissions:     []string{"route:read", "route:write"},
		}

		err := db.SaveToken(tok)
		assert.NoError(t, err)

		fetched, err := db.GetTokenByName("admin")
		assert.NoError(t, err)
		require.NotNil(t, fetched)
		assert.Equal(t, "admin", fetched.Name)
		assert.Empty(t, fetched.Tokens, "API token secrets must not remain in account rows")

		fetchedBySecret, err := db.GetTokenBySecret("token1")
		assert.NoError(t, err)
		require.NotNil(t, fetchedBySecret)
		assert.Equal(t, "admin", fetchedBySecret.Name)

		count, err := db.CountTokens()
		assert.NoError(t, err)
		assert.Equal(t, uint64(1), count)

		err = db.RenameToken("admin", "newadmin", tok)
		assert.NoError(t, err)

		fetchedOld, err := db.GetTokenByName("admin")
		assert.NoError(t, err)
		assert.Nil(t, fetchedOld)

		fetchedNew, err := db.GetTokenByName("newadmin")
		assert.NoError(t, err)
		require.NotNil(t, fetchedNew)

		err = db.DeleteToken("newadmin")
		assert.NoError(t, err)

		fetchedDeleted, err := db.GetTokenByName("newadmin")
		assert.NoError(t, err)
		assert.Nil(t, fetchedDeleted)
	})

	t.Run("Token Rotation Invalidates Cached Secrets", func(t *testing.T) {
		tok := &core.AccessToken{
			Identifier: core.AccessTokenIdentifier{Type: core.Persistent, Value: 2},
			Name:       "rotating-user",
			Tokens:     []string{"old-secret"},
			CreatedAt:  "2026-07-31T00:00:00Z",
		}
		require.NoError(t, db.SaveToken(tok))

		cached, err := db.GetTokenBySecret("old-secret")
		require.NoError(t, err)
		require.NotNil(t, cached)

		updated := *tok
		updated.Tokens = []string{"new-secret"}
		require.NoError(t, db.SaveToken(&updated))

		oldToken, err := db.GetTokenBySecret("old-secret")
		require.NoError(t, err)
		assert.Nil(t, oldToken)
		newToken, err := db.GetTokenBySecret("new-secret")
		require.NoError(t, err)
		require.NotNil(t, newToken)
		assert.Equal(t, "rotating-user", newToken.Name)
	})

	t.Run("Malformed Token JSON Is Rejected", func(t *testing.T) {
		_, err := db.SQLDB.Exec(`INSERT INTO tokens
			(name, type, type_value, encrypted_secret, password_hash, tokens_json, created_at, description, expires_at, permissions_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"corrupt-json", "persistent", 99, "", "", "not-json", time.Now().UTC().Format(time.RFC3339), "", nil, `[]`)
		require.NoError(t, err)
		fetched, err := db.GetTokenByName("corrupt-json")
		assert.Nil(t, fetched)
		assert.ErrorContains(t, err, "decode token secrets")
	})

	t.Run("Session Operations with TTL Cache", func(t *testing.T) {
		now := time.Now().UnixMilli()
		sess1 := &core.Session{
			PublicID:    "pub1",
			Username:    "admin",
			IP:          "127.0.0.1",
			UserAgent:   "Mozilla/5.0",
			CreatedAt:   now,
			LoginMethod: "fido",
		}
		sess1.LastActive.Store(now)

		err := db.SaveSession(sess1, "token_abc")
		assert.NoError(t, err)

		fetchedSess, err := db.GetSession("token_abc")
		assert.NoError(t, err)
		require.NotNil(t, fetchedSess)
		assert.Equal(t, "admin", fetchedSess.Username)
		assert.Equal(t, "fido", fetchedSess.LoginMethod)

		userSessions, err := db.ListUserSessions("admin", "token_abc")
		assert.NoError(t, err)
		assert.Len(t, userSessions, 1)
		assert.True(t, userSessions[0].Current)
		assert.Equal(t, "fido", userSessions[0].LoginMethod)

		err = db.DeleteSession("token_abc")
		assert.NoError(t, err)

		fetchedDeletedSess, err := db.GetSession("token_abc")
		assert.NoError(t, err)
		assert.Nil(t, fetchedDeletedSess)

		// Test DeleteUserSessionByPublicID and DeleteOtherUserSessions
		sess2 := &core.Session{PublicID: "pub2", Username: "user1", IP: "127.0.0.1", CreatedAt: now, LoginMethod: "password"}
		sess2.LastActive.Store(now)
		sess3 := &core.Session{PublicID: "pub3", Username: "user1", IP: "127.0.0.1", CreatedAt: now, LoginMethod: "fido"}
		sess3.LastActive.Store(now)

		require.NoError(t, db.SaveSession(sess2, "token_pub2"))
		require.NoError(t, db.SaveSession(sess3, "token_pub3"))

		tok, revoked, wasCurrent, err := db.DeleteUserSessionByPublicID("user1", "pub2", "token_pub2")
		assert.NoError(t, err)
		assert.Equal(t, "token_pub2", tok)
		assert.True(t, revoked)
		assert.True(t, wasCurrent)

		// Test GetActiveSessions and UpdateSessionsUsername
		activeSesses, err := db.GetActiveSessions(now - 1000)
		assert.NoError(t, err)
		assert.Len(t, activeSesses, 1)
		assert.Equal(t, "fido", activeSesses[0].LoginMethod)

		require.NoError(t, db.UpdateSessionsUsername("user1", "user2"))
		user2Sessions, err := db.ListUserSessions("user2", "")
		assert.NoError(t, err)
		assert.Len(t, user2Sessions, 1)
		assert.Equal(t, "fido", user2Sessions[0].LoginMethod)

		deletedTokens, err := db.DeleteOtherUserSessions("user2", "token_pub3")
		assert.NoError(t, err)
		assert.Empty(t, deletedTokens)

		deletedTokens, err = db.DeleteOtherUserSessions("user2", "")
		assert.NoError(t, err)
		assert.Equal(t, []string{"token_pub3"}, deletedTokens)
		deletedSession, err := db.GetSession("token_pub3")
		require.NoError(t, err)
		assert.Nil(t, deletedSession)
	})

	t.Run("SQL Security Bounds & Validation", func(t *testing.T) {
		overLengthToken := string(make([]byte, 600))
		overLengthUsername := string(make([]byte, 300))

		sess, err := db.GetSession(overLengthToken)
		assert.NoError(t, err)
		assert.Nil(t, sess)

		err = db.SaveSession(&core.Session{PublicID: "p", Username: "u"}, overLengthToken)
		assert.NoError(t, err)

		sesses, err := db.ListUserSessions(overLengthUsername, "")
		assert.NoError(t, err)
		assert.Empty(t, sesses)
	})

	t.Run("SQL LIKE Wildcard Safety and Negative Caching", func(t *testing.T) {
		tok := &core.AccessToken{
			Identifier: core.AccessTokenIdentifier{
				Type:  core.Persistent,
				Value: 2,
			},
			Name:            "wildcard_user",
			EncryptedSecret: "secret",
			PasswordHash:    "hash",
			Tokens:          []string{"token_%_special", "token_\\_slash"},
			CreatedAt:       "2026-08-01T00:00:00Z",
			Description:     "Wildcard token test",
		}
		require.NoError(t, db.SaveToken(tok))

		found, err := db.GetTokenBySecret("token_%_special")
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, "wildcard_user", found.Name)

		notFound, err := db.GetTokenBySecret("token_X_special")
		require.NoError(t, err)
		assert.Nil(t, notFound)

		noSess, err := db.GetSession("non_existent_token_123")
		require.NoError(t, err)
		assert.Nil(t, noSess)
	})

	t.Run("FIDO Device Operations in Database", func(t *testing.T) {
		dev := &core.FidoDevice{
			ID:              "dev-db-1",
			Username:        "dbuser",
			Name:            "DB YubiKey",
			CredentialID:    []byte("cred-db-123"),
			PublicKey:       []byte("pub-db-456"),
			AttestationType: "none",
			AAGUID:          []byte("0000000000000000"),
			SignCount:       5,
			CreatedAt:       1700000000000,
		}

		require.NoError(t, db.SaveFidoDevice(dev))

		devs, err := db.ListFidoDevices("dbuser")
		require.NoError(t, err)
		require.Len(t, devs, 1)
		assert.Equal(t, "dev-db-1", devs[0].ID)
		assert.Equal(t, uint32(5), devs[0].SignCount)

		matched, err := db.GetFidoDeviceByCredentialID([]byte("cred-db-123"))
		require.NoError(t, err)
		require.NotNil(t, matched)
		assert.Equal(t, "dbuser", matched.Username)

		require.NoError(t, db.UpdateFidoSignCount([]byte("cred-db-123"), 10))

		updated, err := db.GetFidoDeviceByCredentialID([]byte("cred-db-123"))
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.Equal(t, uint32(10), updated.SignCount)

		require.NoError(t, db.DeleteFidoDevice("dbuser", "dev-db-1"))
		emptyDevs, err := db.ListFidoDevices("dbuser")
		require.NoError(t, err)
		assert.Empty(t, emptyDevs)
	})

	t.Run("Token Deletion Removes FIDO Devices", func(t *testing.T) {
		tok := &core.AccessToken{Name: "fido-owner", CreatedAt: "2026-07-31T00:00:00Z"}
		require.NoError(t, db.SaveToken(tok))
		require.NoError(t, db.SaveFidoDevice(&core.FidoDevice{
			ID:           "delete-with-token",
			Username:     "fido-owner",
			CredentialID: []byte("delete-credential"),
			PublicKey:    []byte("key"),
		}))

		require.NoError(t, db.DeleteToken("fido-owner"))
		devices, err := db.ListFidoDevices("fido-owner")
		require.NoError(t, err)
		assert.Empty(t, devices)
	})

	t.Run("Sanitization and Null Byte Injection Defense", func(t *testing.T) {
		sanitized := database.SanitizeInputString("user\x00name", 255)
		assert.Equal(t, "username", sanitized)

		sanitizedCtrl := database.SanitizeInputString("user\x07name\x1b\x7f", 255)
		assert.Equal(t, "username", sanitizedCtrl)

		err := db.SaveAuditLog(&core.AuditLogEntry{
			Username:   "user\x00name\x07",
			Operator:   "admin",
			Action:     "TEST",
			Details:    "test\x00details\x1b",
			AuthMethod: "password",
			SessionID:  "sess1",
			IP:         "127.0.0.1",
			CreatedAt:  time.Now().UnixMilli(),
		})
		assert.NoError(t, err)
	})
}
