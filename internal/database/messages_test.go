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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/testutil"
)

func TestUserMessageLifecycle(t *testing.T) {
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "messages.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	now := time.Now().UnixMilli()
	messages := []*core.UserMessage{
		{ID: "00000000-0000-4000-8000-000000000002", Recipient: "Alice", Sender: "admin", Kind: "announcement", Severity: "info", Title: "Second", Body: "Body", Payload: []byte("{}"), CreatedAt: now + 1},
		{ID: "00000000-0000-4000-8000-000000000001", Recipient: "Alice", Sender: "admin", Kind: "invite", Severity: "info", Title: "First", Body: "Body", Payload: []byte(`{"crate":"demo"}`), ActionKind: "cargo_invite", ActionStatus: core.MessageActionPending, CreatedAt: now},
		{ID: "00000000-0000-4000-8000-000000000003", Recipient: "bob", Sender: "admin", Kind: "announcement", Severity: "info", Title: "Other", Body: "Body", Payload: []byte("{}"), CreatedAt: now},
	}
	require.NoError(t, db.SaveMessages(messages))

	page, err := db.ListMessages("alice", 1, 0, "", now)
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Equal(t, "Second", page[0].Title)

	next, err := db.ListMessages("alice", 10, page[0].CreatedAt, page[0].ID, now)
	require.NoError(t, err)
	require.Len(t, next, 1)
	require.Equal(t, "First", next[0].Title)

	unread, err := db.CountUnreadMessages("alice", now)
	require.NoError(t, err)
	require.Equal(t, 2, unread)

	changed, err := db.MarkMessageRead(messages[0].ID, "alice", now+2)
	require.NoError(t, err)
	require.True(t, changed)

	transitioned, err := db.TransitionMessageAction(messages[1].ID, "alice", core.MessageActionPending, core.MessageActionAccepted, now+3)
	require.NoError(t, err)
	require.True(t, transitioned)

	deleted, err := db.DeleteUserMessage(messages[1].ID, "alice")
	require.NoError(t, err)
	require.True(t, deleted)

	unread, err = db.CountUnreadMessages("alice", now+4)
	require.NoError(t, err)
	require.Zero(t, unread)
}

func TestPendingActionMessageCannotBeDeleted(t *testing.T) {
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "pending.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	message := &core.UserMessage{
		ID: "00000000-0000-4000-8000-000000000004", Recipient: "alice", Sender: "admin",
		Kind: "invite", Severity: "info", Title: "Invite", Body: "Body", Payload: []byte("{}"),
		ActionKind: "cargo_invite", ActionStatus: core.MessageActionPending, CreatedAt: time.Now().UnixMilli(),
	}
	require.NoError(t, db.SaveMessages([]*core.UserMessage{message}))
	deleted, err := db.DeleteUserMessage(message.ID, "alice")
	require.NoError(t, err)
	require.False(t, deleted)
}

func TestMessageDedupeKeyIsIdempotent(t *testing.T) {
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "dedupe.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	now := time.Now().UnixMilli()
	first := &core.UserMessage{
		ID: "00000000-0000-4000-8000-000000000010", Recipient: "alice", Kind: "system_update",
		Severity: "info", Title: "Update", Body: "Available", Payload: []byte("{}"),
		CreatedAt: now, DedupeKey: "system-update:available:v1",
	}
	inserted, err := db.SaveMessageIfAbsent(first)
	require.NoError(t, err)
	require.True(t, inserted)
	duplicate := *first
	duplicate.ID = "00000000-0000-4000-8000-000000000011"
	inserted, err = db.SaveMessageIfAbsent(&duplicate)
	require.NoError(t, err)
	require.False(t, inserted)
	page, err := db.ListMessages("alice", 10, 0, "", now+1)
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Equal(t, first.ID, page[0].ID)
	deleted, err := db.DeleteMessagesByDedupeKey(first.DedupeKey)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	page, err = db.ListMessages("alice", 10, 0, "", now+2)
	require.NoError(t, err)
	require.Empty(t, page)
}
