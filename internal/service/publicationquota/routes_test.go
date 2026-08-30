/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package publicationquota

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func setupPublicationQuotaApp(t *testing.T) (*fiber.App, *core.AppState) {
	t.Helper()
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "publication-quota.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, username := range []string{"alice", "bob", "admin"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: username, CreatedAt: time.Now().Format(time.RFC3339)}))
	}
	now := time.Now().UnixMilli()
	require.NoError(t, db.CreateSuperTeam(&core.SuperTeam{Prefix: "platform", Name: "Platform", CreatedAt: now},
		"alice", 5, 20))
	state := core.NewAppState()
	state.Inner.DB = db
	state.Inner.Config.Store(config.DefaultConfig())
	app := fiber.New(fiber.Config{JSONEncoder: json.Marshal, JSONDecoder: json.Unmarshal})
	app.Use(func(c fiber.Ctx) error {
		username := c.Get("X-Test-User")
		roles := []string{"base"}
		if username == "admin" {
			roles = append(roles, "manager")
		}
		if username == "" {
			username = "guest"
		}
		c.Locals("user", &config.User{Username: username, Roles: roles})
		return c.Next()
	})
	SetupRoutes(app.Group("/api"), state)
	return app, state
}

func quotaRequest(t *testing.T, app *fiber.App, method, path, username string, body any) *http.Response {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		require.NoError(t, err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	if body != nil {
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}
	if username != "" {
		request.Header.Set("X-Test-User", username)
	}
	response, err := app.Test(request)
	require.NoError(t, err)
	return response
}

func decodeQuotaStatus(t *testing.T, response *http.Response) core.PublicationQuotaStatus {
	t.Helper()
	defer response.Body.Close()
	var status core.PublicationQuotaStatus
	require.NoError(t, json.NewDecoder(response.Body).Decode(&status))
	return status
}

func TestPublicationQuotaRoutesEnforceVisibilityAndAdministratorOverrides(t *testing.T) {
	app, state := setupPublicationQuotaApp(t)
	response := quotaRequest(t, app, http.MethodGet, "/api/publication-quota", "", nil)
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
	response.Body.Close()

	response = quotaRequest(t, app, http.MethodGet, "/api/publication-quota", "alice", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	status := decodeQuotaStatus(t, response)
	assert.True(t, status.Inherited)
	assert.EqualValues(t, config.DefaultPublicationFileLimit, status.FileLimit)

	response = quotaRequest(t, app, http.MethodGet, "/api/publication-quota/users/bob", "alice", nil)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	response.Body.Close()
	response = quotaRequest(t, app, http.MethodGet, "/api/publication-quota/super-teams/platform", "alice", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	response.Body.Close()
	response = quotaRequest(t, app, http.MethodGet, "/api/publication-quota/super-teams/platform", "bob", nil)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	response.Body.Close()

	response = quotaRequest(t, app, http.MethodPut, "/api/publication-quota/users/bob", "alice", map[string]any{
		"file_limit": 10,
	})
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	response.Body.Close()
	response = quotaRequest(t, app, http.MethodPut, "/api/publication-quota/users/bob", "admin", map[string]any{
		"file_limit": 10, "byte_limit": 2048, "publication_limit": 3, "period": "week", "unlimited": true,
	})
	require.Equal(t, http.StatusOK, response.StatusCode)
	status = decodeQuotaStatus(t, response)
	assert.False(t, status.Inherited)
	assert.True(t, status.Unlimited)
	assert.EqualValues(t, 10, status.FileLimit)
	assert.Equal(t, "week", status.Period)

	response = quotaRequest(t, app, http.MethodPut, "/api/publication-quota/users/bob", "admin", map[string]any{})
	require.Equal(t, http.StatusOK, response.StatusCode)
	status = decodeQuotaStatus(t, response)
	assert.True(t, status.Inherited)
	assert.False(t, status.Unlimited)

	response = quotaRequest(t, app, http.MethodPut, "/api/publication-quota/super-teams/platform", "admin", map[string]any{
		"file_limit": 0, "byte_limit": 0, "publication_limit": 0, "period": "day", "unlimited": false,
	})
	require.Equal(t, http.StatusOK, response.StatusCode)
	status = decodeQuotaStatus(t, response)
	assert.Zero(t, status.FileLimit)
	assert.Equal(t, "day", status.Period)

	response = quotaRequest(t, app, http.MethodPut, "/api/publication-quota/users/bob", "admin", map[string]any{
		"period": "year",
	})
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	response.Body.Close()
	response = quotaRequest(t, app, http.MethodGet, "/api/publication-quota/users/missing", "admin", nil)
	assert.Equal(t, http.StatusNotFound, response.StatusCode)
	response.Body.Close()
	_, err := state.GetDB().GetUserProfile("missing")
	require.ErrorIs(t, err, core.ErrUserProfileNotFound)
}
