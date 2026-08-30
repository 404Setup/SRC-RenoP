/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package updater

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func TestSystemUpdateNotificationsTargetManagersAndDedupe(t *testing.T) {
	state := core.NewAppState()
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "updater-notifications.db"), MaxOpenConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state.Inner.DB = db
	expiredAt := time.Now().Add(-time.Hour).UnixMilli()
	for _, token := range []*core.AccessToken{
		{Name: "admin", Permissions: []string{"manager"}, CreatedAt: time.Now().Format(time.RFC3339)},
		{Name: "base", Permissions: []string{"base"}, CreatedAt: time.Now().Format(time.RFC3339)},
		{Name: "expired", Permissions: []string{"admin"}, ExpiresAt: &expiredAt, CreatedAt: time.Now().Format(time.RFC3339)},
	} {
		require.NoError(t, db.SaveToken(token))
	}
	result := &CheckResult{LatestVersion: "v2.0.0", CurrentVersion: "v1.0.0", Channel: "release", IsRelease: true}
	require.NoError(t, deliverUpdateNotificationToManagers(state, updateNoticeAvailable, result))
	require.NoError(t, deliverUpdateNotificationToManagers(state, updateNoticeAvailable, result))

	now := time.Now().Add(time.Minute).UnixMilli()
	messages, err := db.ListMessages("admin", 10, 0, "", now)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "system_update", messages[0].Kind)
	require.Equal(t, "warning", messages[0].Severity)
	var payload updateNotificationPayload
	require.NoError(t, json.Unmarshal(messages[0].Payload, &payload))
	require.Equal(t, updateNoticeAvailable, payload.Event)
	require.Equal(t, "v2.0.0", payload.Version)
	require.True(t, payload.RequiresAction)

	baseMessages, err := db.ListMessages("base", 10, 0, "", now)
	require.NoError(t, err)
	require.Empty(t, baseMessages)
	expiredMessages, err := db.ListMessages("expired", 10, 0, "", now)
	require.NoError(t, err)
	require.Empty(t, expiredMessages)
}
