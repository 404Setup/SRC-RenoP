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
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/config"
	"renop/core"
	"renop/database"
)

func TestDialects(t *testing.T) {
	sqliteDialect := database.NewDialect("sqlite3")
	assert.Equal(t, "sqlite3", sqliteDialect.Name())
	assert.Contains(t, sqliteDialect.UpsertTokenQuery(), "ON CONFLICT")

	mysqlDialect := database.NewDialect("mysql")
	assert.Equal(t, "mysql", mysqlDialect.Name())
	assert.Contains(t, mysqlDialect.UpsertTokenQuery(), "ON DUPLICATE KEY UPDATE")
}

func TestInitDB_SQLite(t *testing.T) {
	dbFile := "test_renop.db"
	defer os.Remove(dbFile)

	cfg := config.DatabaseConfig{
		Enabled:      true,
		Driver:       "sqlite3",
		Dsn:          dbFile,
		MaxOpenConns: 5,
		MaxIdleConns: 5,
	}

	db, err := database.InitDB(cfg)
	require.NoError(t, err)
	require.NotNil(t, db)
	defer db.Close()

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
		assert.Equal(t, []string{"token1", "token2"}, fetched.Tokens)

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

	t.Run("Session Operations with TTL Cache", func(t *testing.T) {
		now := time.Now().UnixMilli()
		sess1 := &core.Session{
			PublicId:  "pub1",
			Username:  "admin",
			Ip:        "127.0.0.1",
			UserAgent: "Mozilla/5.0",
			CreatedAt: now,
		}
		sess1.LastActive.Store(now)

		err := db.SaveSession(sess1, "token_abc")
		assert.NoError(t, err)

		fetchedSess, err := db.GetSession("token_abc")
		assert.NoError(t, err)
		require.NotNil(t, fetchedSess)
		assert.Equal(t, "admin", fetchedSess.Username)

		userSessions, err := db.ListUserSessions("admin", "token_abc")
		assert.NoError(t, err)
		assert.Len(t, userSessions, 1)
		assert.True(t, userSessions[0].Current)

		err = db.DeleteSession("token_abc")
		assert.NoError(t, err)

		fetchedDeletedSess, err := db.GetSession("token_abc")
		assert.NoError(t, err)
		assert.Nil(t, fetchedDeletedSess)

		// Test DeleteUserSessionByPublicID and DeleteOtherUserSessions
		sess2 := &core.Session{PublicId: "pub2", Username: "user1", Ip: "127.0.0.1", CreatedAt: now}
		sess2.LastActive.Store(now)
		sess3 := &core.Session{PublicId: "pub3", Username: "user1", Ip: "127.0.0.1", CreatedAt: now}
		sess3.LastActive.Store(now)

		require.NoError(t, db.SaveSession(sess2, "token_pub2"))
		require.NoError(t, db.SaveSession(sess3, "token_pub3"))

		tok, revoked, wasCurrent, err := db.DeleteUserSessionByPublicID("user1", "pub2", "token_pub2")
		assert.NoError(t, err)
		assert.Equal(t, "token_pub2", tok)
		assert.True(t, revoked)
		assert.True(t, wasCurrent)

		deletedTokens, err := db.DeleteOtherUserSessions("user1", "token_pub3")
		assert.NoError(t, err)
		assert.Empty(t, deletedTokens)

		deletedTokens, err = db.DeleteOtherUserSessions("user1", "")
		assert.NoError(t, err)
		assert.Equal(t, []string{"token_pub3"}, deletedTokens)
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
}
