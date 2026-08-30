/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package reviewnotify

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func setupNotificationState(t *testing.T) *core.AppState {
	t.Helper()
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "review-notifications.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for username, permissions := range map[string][]string{
		"requester": {"base", "canupdate:releases"},
		"moderator": {"base", "canmoderate:releases"},
		"manager":   {"base", "manager"},
		"unrelated": {"base"},
	} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: username, Permissions: permissions}))
	}
	state := core.NewAppState()
	state.Inner.DB = db
	return state
}

func messagesFor(t *testing.T, state *core.AppState, username string) []*core.UserMessage {
	t.Helper()
	messages, err := state.GetDB().ListMessages(username, 20, 0, "", time.Now().Add(time.Hour).UnixMilli())
	require.NoError(t, err)
	return messages
}

func TestPublicationReviewNotificationsDedupeClearAndHideReviewer(t *testing.T) {
	state := setupNotificationState(t)
	now := time.Now().UnixMilli() - core.PublicationReviewSettleMillis - 1000
	result, err := state.GetDB().CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceMavenArtifact, Repository: "releases",
		ResourceKey: "org.example:demo", ResourceName: "org.example:demo", Version: "1.0.0",
		RequestedBy: "requester", Policy: config.PublicationReviewEveryVersion, CreatedAt: now,
		Files: []*core.ReviewFile{{Path: "org/example/demo/1.0.0/demo-1.0.0.jar", Size: 64}},
	})
	require.NoError(t, err)
	require.NoError(t, NotifyPendingByID(state, result.TaskID))
	require.NoError(t, NotifyPendingByID(state, result.TaskID))
	for _, recipient := range []string{"moderator", "manager"} {
		messages := messagesFor(t, state, recipient)
		require.Len(t, messages, 1, recipient)
		assert.Equal(t, "review_pending", messages[0].Kind)
		assert.NotContains(t, string(messages[0].Payload), "manager")
	}
	assert.Empty(t, messagesFor(t, state, "requester"))
	assert.Empty(t, messagesFor(t, state, "unrelated"))

	task, err := state.GetDB().DecideReviewTask(result.TaskID, "moderator",
		core.ReviewStatusRejected, "preset:quality", now+core.PublicationReviewSettleMillis+1)
	require.NoError(t, err)
	require.NoError(t, NotifyDecision(state, task))
	assert.Empty(t, messagesFor(t, state, "moderator"))
	assert.Empty(t, messagesFor(t, state, "manager"))
	requesterMessages := messagesFor(t, state, "requester")
	require.Len(t, requesterMessages, 1)
	assert.Equal(t, "review_result", requesterMessages[0].Kind)
	assert.Contains(t, string(requesterMessages[0].Payload), `"status":"rejected"`)
	assert.Contains(t, string(requesterMessages[0].Payload), `"decision_reason":"preset:quality"`)
	assert.NotContains(t, strings.ToLower(string(requesterMessages[0].Payload)), "moderator")
}

func TestTeamReviewNotifiesT3AndSystemManager(t *testing.T) {
	state := setupNotificationState(t)
	db := state.GetDB()
	now := time.Now().UnixMilli()
	require.NoError(t, db.CreateSuperTeam(&core.SuperTeam{
		Prefix: "platform", Name: "Platform", CreatedAt: now,
	}, "manager", 10, 10))
	require.NoError(t, db.ForceAddSuperTeamMembers(
		"platform", "manager", []string{"moderator"}, core.SuperTeamRoleManage, 10, 10, now+1))
	require.NoError(t, db.ForceAddSuperTeamMembers(
		"platform", "manager", []string{"requester"}, core.SuperTeamRoleRead, 10, 10, now+2))
	_, err := db.DeleteUserMessages("moderator")
	require.NoError(t, err)
	_, err = db.DeleteUserMessages("requester")
	require.NoError(t, err)
	_, err = db.CreateDockerImage("containers", "personal", "requester", false, now+3)
	require.NoError(t, err)
	task, err := db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
		ResourceKey: "personal", TargetTeamPrefix: "platform",
	}, "requester", false, now+4)
	require.NoError(t, err)
	require.NoError(t, NotifyPending(state, task))
	assert.Len(t, messagesFor(t, state, "moderator"), 1)
	assert.Len(t, messagesFor(t, state, "manager"), 1)
	assert.Empty(t, messagesFor(t, state, "unrelated"))
}

func TestTeamPackageCreationNotificationMovesToRepositoryModerators(t *testing.T) {
	state := setupNotificationState(t)
	db := state.GetDB()
	now := time.Now().UnixMilli() - core.PublicationReviewSettleMillis - 100
	require.NoError(t, db.CreateSuperTeam(&core.SuperTeam{
		Prefix: "platform", Name: "Platform", CreatedAt: now,
	}, "manager", 10, 10))
	require.NoError(t, db.ForceAddSuperTeamMembers(
		"platform", "manager", []string{"requester"}, core.SuperTeamRoleWrite, 10, 10, now+1))
	require.NoError(t, db.ForceAddSuperTeamMembers(
		"platform", "manager", []string{"unrelated"}, core.SuperTeamRoleManage, 10, 10, now+2))
	for _, username := range []string{"requester", "unrelated", "moderator", "manager"} {
		_, err := db.DeleteUserMessages(username)
		require.NoError(t, err)
	}
	result, err := db.CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceNPMPackage, Repository: "releases",
		ResourceKey: "@platform/tool", ResourceName: "@platform/tool", Version: core.ReviewVersionPackageCreation,
		RequestedBy: "requester", Policy: config.PublicationReviewNewPackages,
		ReviewTeamPrefix: "platform", TargetTeamPrefix: "platform", CreatedAt: now + 3,
		Files: []*core.ReviewFile{{Path: "review-requests/npm/platform-tool.json", Size: 64}},
	})
	require.NoError(t, err)
	require.NoError(t, NotifyPendingByID(state, result.TaskID))
	assert.Len(t, messagesFor(t, state, "unrelated"), 1)
	assert.Len(t, messagesFor(t, state, "manager"), 1)
	assert.Empty(t, messagesFor(t, state, "moderator"))

	advanced, err := db.AdvancePackageCreationReview(
		result.TaskID, "unrelated", now+core.PublicationReviewSettleMillis+4)
	require.NoError(t, err)
	require.NoError(t, NotifyPendingTransition(state, advanced))
	assert.Empty(t, messagesFor(t, state, "unrelated"))
	assert.Len(t, messagesFor(t, state, "manager"), 1)
	assert.Len(t, messagesFor(t, state, "moderator"), 1)
}
