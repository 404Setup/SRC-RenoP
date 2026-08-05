/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storeTestSession(state *AppState, secret, publicID, username string) {
	s := &Session{
		PublicId:  publicID,
		Username:  username,
		Ip:        "203.0.113.10",
		UserAgent: "TestAgent/1.0",
		CreatedAt: 1_000,
	}
	s.LastActive.Store(2_000)
	state.Inner.Sessions.Store(secret, s)
}

func TestListUserSessionsAndExpiresAt(t *testing.T) {
	state := NewAppState()
	storeTestSession(state, "secret-a", "pub-a", "alice")
	storeTestSession(state, "secret-b", "pub-b", "alice")
	storeTestSession(state, "secret-c", "pub-c", "bob")

	list := state.ListUserSessions("alice", "secret-a")
	require.Len(t, list, 2)

	var current, other SessionDto
	for _, s := range list {
		if s.PublicId == "pub-a" {
			current = s
		} else {
			other = s
		}
	}
	assert.True(t, current.Current)
	assert.False(t, other.Current)
	assert.Equal(t, int64(2_000+SessionIdleTimeoutMillis), current.ExpiresAt)
	assert.Equal(t, "203.0.113.10", current.Ip)
	assert.Equal(t, "TestAgent/1.0", current.UserAgent)
	assert.Empty(t, state.ListUserSessions("nobody", ""))
}

func TestRevokeUserSessionByPublicID(t *testing.T) {
	state := NewAppState()
	storeTestSession(state, "secret-a", "pub-a", "alice")
	storeTestSession(state, "secret-b", "pub-b", "alice")

	revoked, wasCurrent := state.RevokeUserSessionByPublicID("alice", "pub-a", "secret-a")
	assert.True(t, revoked)
	assert.True(t, wasCurrent)
	assert.Len(t, state.ListUserSessions("alice", "secret-a"), 1)

	revoked, wasCurrent = state.RevokeUserSessionByPublicID("bob", "pub-b", "")
	assert.False(t, revoked)
	assert.False(t, wasCurrent)
	assert.Len(t, state.ListUserSessions("alice", ""), 1)
}

func TestRevokeOtherUserSessions(t *testing.T) {
	state := NewAppState()
	storeTestSession(state, "keep", "pub-keep", "alice")
	storeTestSession(state, "drop-1", "pub-1", "alice")
	storeTestSession(state, "drop-2", "pub-2", "alice")
	storeTestSession(state, "other", "pub-other", "bob")

	n := state.RevokeOtherUserSessions("alice", "keep")
	assert.Equal(t, 2, n)
	list := state.ListUserSessions("alice", "keep")
	require.Len(t, list, 1)
	assert.Equal(t, "pub-keep", list[0].PublicId)
	assert.Len(t, state.ListUserSessions("bob", ""), 1)

	assert.Equal(t, 1, state.RevokeAllUserSessions("bob"))
	assert.Empty(t, state.ListUserSessions("bob", ""))
}
