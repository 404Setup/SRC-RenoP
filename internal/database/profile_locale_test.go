/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/locale"
	"renop/internal/testutil"
)

func TestUserLocalePersistsWithoutChangingCredentials(t *testing.T) {
	cfg := config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "locale.db")}
	db, err := InitDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	now := time.Now().UnixMilli()
	account := &core.AccessToken{Name: "alice", EncryptedSecret: "password", Permissions: []string{"base"}}
	require.NoError(t, db.SaveToken(account))
	_, err = db.UpdateAccountEmail("alice", "alice@example.com", now)
	require.NoError(t, err)
	session := &core.Session{PublicID: "locale", Username: "alice", CreatedAt: now}
	session.LastActive.Store(now)
	require.NoError(t, db.SaveSession(session, "locale-session"))
	before, err := db.GetMFAState("alice")
	require.NoError(t, err)
	require.ErrorIs(t, db.SetUserLocale("alice", "locale-session", "invalid/value", before.UserID), locale.ErrUnsupported)
	require.ErrorIs(t, db.SetUserLocale("alice", "other-session", "fr-FR", before.UserID), core.ErrEmailCodeInvalid)
	require.ErrorIs(t, db.SetUserLocale("alice", "locale-session", "fr-FR", "another-account"), core.ErrUserProfileNotFound)
	require.NoError(t, db.SetUserLocale("alice", "locale-session", "fr", before.UserID))
	after, err := db.GetMFAState("alice")
	require.NoError(t, err)
	require.Equal(t, before.Snapshot, after.Snapshot)
	profile, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	require.Equal(t, "fr-FR", profile.Locale)
	public, err := json.Marshal(profile)
	require.NoError(t, err)
	require.NotContains(t, string(public), "locale")
	code, err := db.GetEmailLocale("ALICE@example.com")
	require.NoError(t, err)
	require.Equal(t, "fr-FR", code)
	require.NoError(t, db.RenameToken("alice", "alice_new", account))
	require.NoError(t, db.Close())
	db, err = InitDB(cfg)
	require.NoError(t, err)
	profile, err = db.GetUserProfile("alice_new")
	require.NoError(t, err)
	require.Equal(t, "fr-FR", profile.Locale)
	require.NoError(t, db.SetAccountBan("alice_new", &core.AccountBan{Reason: "test", CreatedAt: now}))
	require.ErrorIs(t, db.SetUserLocale("alice_new", "locale-session", "ja-JP", before.UserID), core.ErrEmailCodeInvalid)
	require.NoError(t, db.SetAccountBan("alice_new", nil))
	require.NoError(t, db.RetireAccount("alice_new", now+1))
	code, err = db.GetEmailLocale("alice@example.com")
	require.NoError(t, err)
	require.Empty(t, code)
}
