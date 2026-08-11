/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package bootstrap

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/core"
)

func TestInitializeDatabaseSessions(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	t.Setenv("RENOP_CONFIG", filepath.Join(dir, "nonexistent.yaml"))
	t.Setenv("RENOP_REPOSITORIES", filepath.Join(dir, "repos.yaml"))
	t.Setenv("RENOP_INDEX", filepath.Join(dir, "index.json"))

	state, _ := Initialize()
	require.NotNil(t, state)

	// Save session
	now := time.Now().UnixMilli()
	sess := &core.Session{
		PublicId:  "pub-db-test",
		Username:  "dbuser",
		Ip:        "127.0.0.1",
		UserAgent: "TestUA",
		CreatedAt: now,
	}
	sess.LastActive.Store(now)

	token := "session-token-123"
	require.NoError(t, state.SaveSession(sess, token))

	// Verify GetSession returns the session
	fetched := state.GetSession(token)
	require.NotNil(t, fetched)
	assert.Equal(t, "dbuser", fetched.Username)
	assert.Equal(t, "pub-db-test", fetched.PublicId)

	// Verify ListUserSessions
	list := state.ListUserSessions("dbuser", token)
	require.Len(t, list, 1)
	assert.True(t, list[0].Current)

	// Revoke session
	revoked, err := state.RevokeSession(token)
	require.NoError(t, err)
	assert.True(t, revoked)
	assert.Nil(t, state.GetSession(token))
	_ = dbPath
}
