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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/testutil"
)

func TestAccountBanLifecycle(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "account-ban.db"), MaxOpenConns: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", Permissions: []string{"base"}}))
	session := &core.Session{PublicID: "alice-session", Username: "alice", CreatedAt: time.Now().UnixMilli()}
	session.LastActive.Store(time.Now().UnixMilli())
	require.NoError(t, db.SaveSession(session, "alice-session-secret"))

	now := time.Now().UnixMilli()
	expiresAt := now + int64((24*time.Hour)/time.Millisecond)
	require.NoError(t, db.SetAccountBan("alice", &core.AccountBan{
		Reason: "Repeated abuse", CreatedAt: now, ExpiresAt: &expiresAt,
	}))
	account, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	require.NotNil(t, account.Ban)
	assert.True(t, account.Ban.IsActive(now))
	assert.Equal(t, "Repeated abuse", account.Ban.Reason)
	storedSession, err := db.GetSession("alice-session-secret")
	require.NoError(t, err)
	assert.Nil(t, storedSession)
	accounts, err := db.GetAllTokens()
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.NotNil(t, accounts[0].Ban)

	require.NoError(t, db.UpdateToken("alice", func(token *core.AccessToken) {
		token.Description = "Ban must survive unrelated account updates"
	}))
	account, err = db.GetTokenByName("alice")
	require.NoError(t, err)
	require.NotNil(t, account.Ban)
	require.NoError(t, db.RenameToken("alice", "alice2", account))
	account, err = db.GetTokenByName("alice2")
	require.NoError(t, err)
	require.NotNil(t, account.Ban)
	names, err := db.SearchTokenNames("ali", 8, now)
	require.NoError(t, err)
	assert.Empty(t, names)

	expiredAt := now - 1
	require.NoError(t, db.SetAccountBan("alice2", &core.AccountBan{
		Reason: "Expired suspension", CreatedAt: now - 2, ExpiresAt: &expiredAt,
	}))
	names, err = db.SearchTokenNames("ali", 8, now)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice2"}, names)
	require.NoError(t, db.SetAccountBan("alice2", nil))
	account, err = db.GetTokenByName("alice2")
	require.NoError(t, err)
	assert.Nil(t, account.Ban)

	require.ErrorIs(t, db.SetAccountBan("alice2", &core.AccountBan{
		Reason: "invalid\nreason", CreatedAt: now,
	}), core.ErrAccountBanInvalid)
	require.ErrorIs(t, db.SetAccountBan("missing", nil), core.ErrUserProfileNotFound)
}

func TestAccountBanRejectsProtectedRolesAndPromotion(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "protected-bans.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	now := time.Now().UnixMilli()
	ban := &core.AccountBan{Reason: "Abuse", CreatedAt: now}
	for _, role := range []string{"admin", "manager", "m", "access-token:manager", "canmoderate:*", "canmoderate:releases", " CANMODERATE:releases "} {
		t.Run(role, func(t *testing.T) {
			require.NoError(t, db.SaveToken(&core.AccessToken{Name: "staff", Permissions: []string{role}}))
			require.ErrorIs(t, db.SetAccountBan("staff", ban), core.ErrAccountBanProtected)
			account, err := db.GetTokenByName("staff")
			require.NoError(t, err)
			require.Nil(t, account.Ban)
			require.NoError(t, db.UpdateToken("staff", func(token *core.AccessToken) { token.Permissions = []string{"base"} }))
			require.NoError(t, db.SetAccountBan("staff", ban))
			require.ErrorIs(t, db.UpdateToken("staff", func(token *core.AccessToken) { token.Permissions = []string{role} }), core.ErrAccountBanProtected)
			account, err = db.GetTokenByName("staff")
			require.NoError(t, err)
			require.Equal(t, []string{"base"}, account.Permissions)
			require.True(t, account.Ban.IsActive(now))
			require.NoError(t, db.SetAccountBan("staff", nil))
			require.NoError(t, db.UpdateToken("staff", func(token *core.AccessToken) { token.Permissions = []string{role} }))
		})
	}
}
