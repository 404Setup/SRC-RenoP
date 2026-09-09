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
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/testutil"
)

func TestAccountIPBanLifecycle(t *testing.T) {
	cfg := config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "ip-bans.db")}
	db, err := InitDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	now := time.Now().UnixMilli()
	ban := &core.AccountBan{Reason: "Abuse", CreatedAt: now}
	for _, name := range []string{"alice", "bob", "neverlogged"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: name, Permissions: []string{"base"}}))
	}
	for _, name := range []string{"alice", "bob"} {
		session := &core.Session{PublicID: name + "-session", Username: name, IP: "::ffff:192.0.2.10", CreatedAt: now}
		session.LastActive.Store(now)
		require.NoError(t, db.SaveSession(session, name+"-session-secret"))
	}
	for _, entry := range []*core.AuditLogEntry{
		{Username: "alice", Operator: "alice", Action: "LOGIN", IP: "2001:0db8::1", CreatedAt: now},
		{Username: "alice", Operator: "alice", Action: "LOGIN", IP: "192.0.2.11", CreatedAt: now - int64(31*24*time.Hour/time.Millisecond)},
		{Username: "alice", Operator: "admin", Action: "LOGIN", IP: "192.0.2.12", CreatedAt: now},
		{Username: "alice", Operator: "alice", Action: "USER_BAN", IP: "192.0.2.13", CreatedAt: now},
		{Username: "alice", Operator: "alice", Action: "LOGIN", IP: "invalid", CreatedAt: now},
	} {
		require.NoError(t, db.SaveAuditLog(entry))
	}
	check := func(ip string, expected bool) {
		t.Helper()
		banned, err := db.IsIPBanned(ip)
		require.NoError(t, err)
		require.Equal(t, expected, banned, ip)
	}
	check("192.0.2.10", false)
	require.NoError(t, db.SetAccountBan("alice", ban, true))
	status, err := db.GetAccountBanStatus("alice")
	require.NoError(t, err)
	require.Equal(t, 2, status.IPCount)
	require.NotNil(t, status.Ban)
	for _, ip := range []string{"192.0.2.10", "::ffff:192.0.2.10", "2001:db8::1"} {
		check(ip, true)
	}
	for _, ip := range []string{"192.0.2.11", "192.0.2.12", "192.0.2.13", "invalid"} {
		check(ip, false)
	}
	session, err := db.GetSession("alice-session-secret")
	require.NoError(t, err)
	require.Nil(t, session)
	require.NoError(t, db.Close())
	db, err = InitDB(cfg)
	require.NoError(t, err)
	check("192.0.2.10", true)
	require.NoError(t, db.SetAccountBan("alice", ban))
	check("192.0.2.10", true)
	account, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	require.NoError(t, db.RenameToken("alice", "alice2", account))
	check("192.0.2.10", true)
	require.NoError(t, db.SetAccountBan("bob", ban, true))
	require.NoError(t, db.SetAccountBan("alice2", ban, false))
	check("2001:db8::1", false)
	check("192.0.2.10", true)
	status, err = db.GetAccountBanStatus("alice2")
	require.NoError(t, err)
	require.Zero(t, status.IPCount)
	require.NotNil(t, status.Ban, "disabling IP restrictions must not lift the account suspension")
	require.NoError(t, db.SetAccountBan("bob", nil))
	check("192.0.2.10", false)
	require.ErrorIs(t, db.SetAccountBan("neverlogged", ban, true), core.ErrAccountBanIPUnknown)
	status, err = db.GetAccountBanStatus("neverlogged")
	require.NoError(t, err)
	require.Nil(t, status.Ban, "an unavailable IP restriction rolls back the account suspension")
	require.NoError(t, db.SetAccountBan("alice2", ban, true))
	check("2001:db8::1", true)
	expires := time.Now().Add(time.Second).UnixMilli()
	temporary := ban.Clone()
	temporary.ExpiresAt = &expires
	require.NoError(t, db.SetAccountBan("alice2", temporary))
	check("2001:db8::1", true)
	time.Sleep(time.Until(time.UnixMilli(expires)) + time.Millisecond)
	check("2001:db8::1", false)
	require.NoError(t, db.SetAccountBan("alice2", ban, true))
	check("2001:db8::1", true)
	require.NoError(t, db.DeleteToken("alice2"))
	check("2001:db8::1", false)
}

func TestAccountIPBanBounds(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "bounded-ip-bans.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", Permissions: []string{"base"}}))
	now := time.Now().UnixMilli()
	for i := range 100 {
		require.NoError(t, db.SaveAuditLog(&core.AuditLogEntry{Username: "alice", Operator: "alice", Action: "LOGIN",
			IP: fmt.Sprintf("192.0.2.%d", i+1), CreatedAt: now - int64(i)}))
	}
	require.NoError(t, db.SetAccountBan("alice", &core.AccountBan{Reason: "Abuse", CreatedAt: now}, true))
	status, err := db.GetAccountBanStatus("alice")
	require.NoError(t, err)
	require.Equal(t, maxAccountBanIPs, status.IPCount)
	banned, err := db.IsIPBanned("192.0.2.1")
	require.NoError(t, err)
	require.True(t, banned)
	banned, err = db.IsIPBanned("192.0.2.100")
	require.NoError(t, err)
	require.False(t, banned)
}

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

func TestAccountBanRejectsNewSessionsEvenWithACurrentSnapshot(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "banned-session.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", Permissions: []string{"base"}}))
	now := time.Now().UnixMilli()
	require.NoError(t, db.SetAccountBan("alice", &core.AccountBan{Reason: "Abuse", CreatedAt: now}))
	state, err := db.GetMFAState("alice")
	require.NoError(t, err)
	session := &core.Session{PublicID: "late-session", Username: "alice", CreatedAt: now, AuthenticationSnapshot: state.Snapshot}
	session.LastActive.Store(now)
	require.ErrorIs(t, db.SaveSession(session, "late-session-secret"), core.ErrAccountBanned)
	stored, err := db.GetSession("late-session-secret")
	require.NoError(t, err)
	require.Nil(t, stored)
	require.NoError(t, db.SetAccountBan("alice", nil))
	require.NoError(t, db.UpdateToken("alice", func(account *core.AccessToken) { expired := now - 1; account.ExpiresAt = &expired }))
	state, err = db.GetMFAState("alice")
	require.NoError(t, err)
	session.AuthenticationSnapshot = state.Snapshot
	require.ErrorIs(t, db.SaveSession(session, "expired-session-secret"), core.ErrMFAInvalid)
}
