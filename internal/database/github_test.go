/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
)

func TestGitHubIdentityLifecycleAndPrincipalRefresh(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "github.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, username := range []string{"alice", "bob"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{
			Name: username, CreatedAt: time.Now().UTC().Format(time.RFC3339), Permissions: []string{"base"},
		}))
	}
	require.ErrorIs(t, db.CreateToken(&core.AccessToken{
		Name: "alice", Description: "must not replace", CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Permissions: []string{"base"},
	}, "", time.Now().UnixMilli()), core.ErrUsernameAlreadyExists)
	originalAlice, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	assert.NotEqual(t, "must not replace", originalAlice.Description)
	alice, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	bob, err := db.GetUserProfile("bob")
	require.NoError(t, err)
	now := time.Now().UnixMilli()
	require.NoError(t, db.StoreGitHubIdentity(alice.UserID, 101, "Alice-GH", []core.GitHubPrincipal{
		{Type: core.GitHubPrincipalUser, GitHubID: 101, Login: "Alice-GH"},
		{Type: core.GitHubPrincipalOrganization, GitHubID: 202, Login: "Example-Org"},
	}, now))

	identity, err := db.GetGitHubIdentity("alice")
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, int64(101), identity.GitHubUserID)
	assert.Equal(t, "alice-gh", identity.GitHubLogin)
	assert.Equal(t, 2, identity.PrincipalCount)
	byProvider, err := db.GetGitHubIdentityByProviderID(101)
	require.NoError(t, err)
	assert.Equal(t, "alice", byProvider.Username)
	authorized, err := db.HasRecentGitHubPrincipal("alice", "EXAMPLE-ORG", now-1)
	require.NoError(t, err)
	assert.True(t, authorized)
	authorized, err = db.HasRecentGitHubPrincipal("alice", "example-org", now+1)
	require.NoError(t, err)
	assert.False(t, authorized)

	require.ErrorIs(t, db.StoreGitHubIdentity(bob.UserID, 101, "alice-gh", []core.GitHubPrincipal{
		{Type: core.GitHubPrincipalUser, GitHubID: 101, Login: "alice-gh"},
	}, now), core.ErrGitHubIdentityLinked)
	require.ErrorIs(t, db.StoreGitHubIdentity(alice.UserID, 303, "other", []core.GitHubPrincipal{
		{Type: core.GitHubPrincipalUser, GitHubID: 303, Login: "other"},
	}, now), core.ErrGitHubIdentityLinked)

	later := now + 100
	require.NoError(t, db.StoreGitHubIdentity(alice.UserID, 101, "alice-renamed", []core.GitHubPrincipal{
		{Type: core.GitHubPrincipalUser, GitHubID: 101, Login: "alice-renamed"},
		{Type: core.GitHubPrincipalOrganization, GitHubID: 404, Login: "new-org"},
	}, later))
	authorized, err = db.HasRecentGitHubPrincipal("alice", "example-org", 0)
	require.NoError(t, err)
	assert.False(t, authorized)
	authorized, err = db.HasRecentGitHubPrincipal("alice", "new-org", later)
	require.NoError(t, err)
	assert.True(t, authorized)

	require.ErrorIs(t, db.DeleteGitHubIdentity("alice"), core.ErrLastLoginMethod)
	require.NoError(t, db.SetAccountPassword("alice", "configured-password-hash", later+1))
	require.NoError(t, db.DeleteGitHubIdentity("alice"))
	identity, err = db.GetGitHubIdentity("alice")
	require.NoError(t, err)
	assert.Nil(t, identity)
	assert.True(t, errors.Is(db.DeleteGitHubIdentity("alice"), core.ErrGitHubIdentityNotFound))
}
