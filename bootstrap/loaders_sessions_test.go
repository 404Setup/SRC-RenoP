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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"renop/core"
	"renop/pb"
)

func TestLoadSessionsProtobuf(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.bin")

	store := pb.FromSessionDbDtos([]core.SessionDbDto{
		{
			PublicId:     "pub-1",
			SessionToken: "tok-1",
			Username:     "admin",
			Ip:           "127.0.0.1",
			UserAgent:    "test-agent",
			CreatedAt:    100,
			LastActive:   200,
		},
	})
	bin, err := proto.Marshal(store)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, bin, 0644))

	sessions := LoadSessions(path)
	require.Len(t, sessions, 1)
	assert.Equal(t, "pub-1", sessions[0].PublicId)
	assert.Equal(t, "tok-1", sessions[0].SessionToken)
	assert.Equal(t, "admin", sessions[0].Username)
	assert.Equal(t, int64(100), sessions[0].CreatedAt)
	assert.Equal(t, int64(200), sessions[0].LastActive)
}

func TestLoadSessionsMissing(t *testing.T) {
	sessions := LoadSessions(filepath.Join(t.TempDir(), "missing.pb"))
	assert.Empty(t, sessions)
}

func TestInitializeDatabaseSessions(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	t.Setenv("RENOP_CONFIG", filepath.Join(dir, "nonexistent.yaml"))
	t.Setenv("RENOP_REPOSITORIES", filepath.Join(dir, "repos.yaml"))
	t.Setenv("RENOP_TOKENS", filepath.Join(dir, "tokens.yaml"))
	t.Setenv("RENOP_SESSIONS", filepath.Join(dir, "sessions.bin"))
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
	state.SaveSession(sess, token)

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
	revoked := state.RevokeSession(token)
	assert.True(t, revoked)
	assert.Nil(t, state.GetSession(token))
	_ = dbPath
}
