/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package message

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func messageTestState(t *testing.T) (*core.AppState, *database.DB) {
	t.Helper()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "messages.db")})
	require.NoError(t, err)
	state := core.NewAppState()
	state.Inner.DB = db
	return state, db
}

func saveMessageTestToken(t *testing.T, db *database.DB, name string) {
	t.Helper()
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: name, Identifier: core.AccessTokenIdentifier{Type: core.Persistent, Value: 1},
		Tokens: []string{name + "-secret"}, Permissions: []string{"base"},
	}))
}

func messageTestApp(state *core.AppState, user *config.User) *fiber.App {
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", user)
		return c.Next()
	})
	SetupRoutes(app.Group("/api"), state)
	return app
}

func TestManagerSendsTargetedNotification(t *testing.T) {
	state, db := messageTestState(t)
	defer db.Close()
	saveMessageTestToken(t, db, "alice")
	app := messageTestApp(state, &config.User{Username: "admin", Roles: []string{"manager"}})
	reqMsg := &pb.SendNotificationRequest{
		Recipients: []string{"alice"},
		Title:      "Maintenance",
		Body:       "Tonight",
		Severity:   "warning",
	}
	reqData, err := proto.Marshal(reqMsg)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/messages/admin", bytes.NewReader(reqData))
	request.Header.Set(fiber.HeaderContentType, protohttp.ContentType)
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, fiber.StatusCreated, response.StatusCode)
	require.Equal(t, protohttp.ContentType, response.Header.Get(fiber.HeaderContentType))

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var respMsg pb.SendNotificationResponse
	require.NoError(t, proto.Unmarshal(body, &respMsg))
	require.True(t, respMsg.Ok)
	require.EqualValues(t, 1, respMsg.Sent)

	messages, err := db.ListMessages("alice", 10, 0, "", 1)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "Maintenance", messages[0].Title)
	require.Equal(t, "warning", messages[0].Severity)
}

func TestUserCannotReadAnotherUsersMessage(t *testing.T) {
	state, db := messageTestState(t)
	defer db.Close()
	require.NoError(t, Deliver(state, &core.UserMessage{
		Recipient: "bob", Sender: "admin", Kind: "announcement", Severity: "info", Title: "Private", Body: "Only Bob",
	}))
	app := messageTestApp(state, &config.User{Username: "alice", Roles: []string{"base"}})
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/messages", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, fiber.StatusOK, response.StatusCode)
	require.Equal(t, protohttp.ContentType, response.Header.Get(fiber.HeaderContentType))

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var payload pb.UserMessageList
	require.NoError(t, proto.Unmarshal(body, &payload))
	require.Empty(t, payload.Messages)
}

func TestNonManagerCannotSendNotification(t *testing.T) {
	state, db := messageTestState(t)
	defer db.Close()
	app := messageTestApp(state, &config.User{Username: "alice", Roles: []string{"base"}})
	reqMsg := &pb.SendNotificationRequest{All: true, Title: "No", Body: "No"}
	reqData, err := proto.Marshal(reqMsg)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/messages/admin", bytes.NewReader(reqData))
	request.Header.Set(fiber.HeaderContentType, protohttp.ContentType)
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, fiber.StatusForbidden, response.StatusCode)
}

func TestManagerSearchesNotificationRecipientsByPrefix(t *testing.T) {
	state, db := messageTestState(t)
	defer db.Close()
	saveMessageTestToken(t, db, "alice")
	saveMessageTestToken(t, db, "bob")
	saveMessageTestToken(t, db, "bobby")
	app := messageTestApp(state, &config.User{Username: "admin", Roles: []string{"manager"}})
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/messages/admin/users?q=bo", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, fiber.StatusOK, response.StatusCode)
	require.Equal(t, protohttp.ContentType, response.Header.Get(fiber.HeaderContentType))

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var payload pb.UserSearchResponse
	require.NoError(t, proto.Unmarshal(body, &payload))
	require.Equal(t, []string{"bob", "bobby"}, payload.Users)
}

func TestNonManagerCannotSearchNotificationRecipients(t *testing.T) {
	state, db := messageTestState(t)
	defer db.Close()
	app := messageTestApp(state, &config.User{Username: "alice", Roles: []string{"base"}})
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/messages/admin/users?q=a", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, fiber.StatusForbidden, response.StatusCode)
}

func TestMessageListCursorReturnsTheNextPage(t *testing.T) {
	state, db := messageTestState(t)
	defer db.Close()
	for createdAt := int64(1); createdAt <= 3; createdAt++ {
		require.NoError(t, Deliver(state, &core.UserMessage{
			Recipient: "alice", Kind: "announcement", Severity: "info",
			Title: "Page", Body: "Cursor", CreatedAt: createdAt,
		}))
	}
	app := messageTestApp(state, &config.User{Username: "alice", Roles: []string{"base"}})
	firstResponse, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/messages?limit=2", nil))
	require.NoError(t, err)
	defer firstResponse.Body.Close()
	require.Equal(t, protohttp.ContentType, firstResponse.Header.Get(fiber.HeaderContentType))

	firstBody, err := io.ReadAll(firstResponse.Body)
	require.NoError(t, err)
	var firstPage pb.UserMessageList
	require.NoError(t, proto.Unmarshal(firstBody, &firstPage))
	require.Len(t, firstPage.Messages, 2)
	require.NotEmpty(t, firstPage.NextCursor)

	secondResponse, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/messages?limit=2&cursor="+firstPage.NextCursor, nil))
	require.NoError(t, err)
	defer secondResponse.Body.Close()
	require.Equal(t, protohttp.ContentType, secondResponse.Header.Get(fiber.HeaderContentType))

	secondBody, err := io.ReadAll(secondResponse.Body)
	require.NoError(t, err)
	var secondPage pb.UserMessageList
	require.NoError(t, proto.Unmarshal(secondBody, &secondPage))
	require.Len(t, secondPage.Messages, 1)
	require.Empty(t, secondPage.NextCursor)
	require.NotEqual(t, firstPage.Messages[1].Id, secondPage.Messages[0].Id)
}

func TestUserClearsOnlyOwnDismissibleMessages(t *testing.T) {
	state, db := messageTestState(t)
	defer db.Close()
	require.NoError(t, DeliverBatch(state, []*core.UserMessage{
		{Recipient: "alice", Kind: "announcement", Severity: "info", Title: "One", Body: "Body"},
		{Recipient: "alice", Kind: "invite", Severity: "info", Title: "Completed", Body: "Body", ActionKind: "cargo_invite", ActionStatus: core.MessageActionAccepted},
		{Recipient: "alice", Kind: "invite", Severity: "info", Title: "Pending", Body: "Body", ActionKind: "cargo_invite", ActionStatus: core.MessageActionPending},
		{Recipient: "bob", Kind: "announcement", Severity: "info", Title: "Other user", Body: "Body"},
	}))

	app := messageTestApp(state, &config.User{Username: "alice", Roles: []string{"base"}})
	response, err := app.Test(httptest.NewRequest(http.MethodDelete, "/api/messages", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, fiber.StatusOK, response.StatusCode)
	require.Equal(t, protohttp.ContentType, response.Header.Get(fiber.HeaderContentType))

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var result pb.ClearMessagesResponse
	require.NoError(t, proto.Unmarshal(body, &result))
	require.True(t, result.Ok)
	require.EqualValues(t, 2, result.Deleted)

	aliceMessages, err := db.ListMessages("alice", 10, 0, "", 1)
	require.NoError(t, err)
	require.Len(t, aliceMessages, 1)
	require.Equal(t, core.MessageActionPending, aliceMessages[0].ActionStatus)
	bobMessages, err := db.ListMessages("bob", 10, 0, "", 1)
	require.NoError(t, err)
	require.Len(t, bobMessages, 1)
}
