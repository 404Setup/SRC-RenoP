/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

package message

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
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
	body, err := json.Marshal(notificationRequest{Recipients: []string{"alice"}, Title: "Maintenance", Body: "Tonight", Severity: "warning"})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/messages/admin", bytes.NewReader(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, fiber.StatusCreated, response.StatusCode)

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
	var payload listResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
	require.Empty(t, payload.Messages)
}

func TestNonManagerCannotSendNotification(t *testing.T) {
	state, db := messageTestState(t)
	defer db.Close()
	app := messageTestApp(state, &config.User{Username: "alice", Roles: []string{"base"}})
	request := httptest.NewRequest(http.MethodPost, "/api/messages/admin", bytes.NewBufferString(`{"all":true,"title":"No","body":"No"}`))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
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
	var payload userSearchResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
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
	var firstPage listResponse
	require.NoError(t, json.NewDecoder(firstResponse.Body).Decode(&firstPage))
	require.Len(t, firstPage.Messages, 2)
	require.NotEmpty(t, firstPage.NextCursor)

	secondResponse, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/messages?limit=2&cursor="+firstPage.NextCursor, nil))
	require.NoError(t, err)
	defer secondResponse.Body.Close()
	var secondPage listResponse
	require.NoError(t, json.NewDecoder(secondResponse.Body).Decode(&secondPage))
	require.Len(t, secondPage.Messages, 1)
	require.Empty(t, secondPage.NextCursor)
	require.NotEqual(t, firstPage.Messages[1].ID, secondPage.Messages[0].ID)
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
	var result struct {
		Deleted int64 `json:"deleted"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	require.EqualValues(t, 2, result.Deleted)

	aliceMessages, err := db.ListMessages("alice", 10, 0, "", 1)
	require.NoError(t, err)
	require.Len(t, aliceMessages, 1)
	require.Equal(t, core.MessageActionPending, aliceMessages[0].ActionStatus)
	bobMessages, err := db.ListMessages("bob", 10, 0, "", 1)
	require.NoError(t, err)
	require.Len(t, bobMessages, 1)
}
