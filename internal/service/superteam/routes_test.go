/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package superteam

import (
	"bytes"
	"io"
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
	"renop/internal/testutil"
)

func setupSuperTeamApp(t *testing.T) (*fiber.App, *core.AppState) {
	t.Helper()
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "super-teams.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, username := range []string{"alice", "bob", "admin"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: username, CreatedAt: time.Now().Format(time.RFC3339)}))
	}
	state := core.NewAppState()
	state.Inner.DB = db
	cfg := config.DefaultConfig()
	cfg.SuperTeams = config.SuperTeamConfig{CreateLimit: 1, JoinLimit: 3}
	state.Inner.Config.Store(cfg)
	app := fiber.New(fiber.Config{JSONEncoder: json.Marshal, JSONDecoder: json.Unmarshal})
	t.Cleanup(func() { require.NoError(t, app.Shutdown()) })
	app.Use(func(c fiber.Ctx) error {
		username := c.Get("X-Test-User")
		roles := []string{"base"}
		if username == "admin" {
			roles = []string{"base", "manager"}
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

func superTeamRequest(t *testing.T, app *fiber.App, method, path, username string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, reader)
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

func decodeSuperTeamResponse(t *testing.T, response *http.Response, destination any) {
	t.Helper()
	defer response.Body.Close()
	require.NoError(t, json.NewDecoder(response.Body).Decode(destination))
}

func TestSuperTeamRoutesLifecycleInvitationAndVisibility(t *testing.T) {
	app, state := setupSuperTeamApp(t)
	response := superTeamRequest(t, app, http.MethodGet, "/api/super-teams", "", nil)
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
	response.Body.Close()

	response = superTeamRequest(t, app, http.MethodPost, "/api/super-teams", "alice", map[string]any{
		"prefix": "platform", "name": "Platform", "description": "Shared packages",
		"links": map[string]any{"website": "https://platform.example", "github": "https://github.com/platform"},
	})
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var created core.SuperTeam
	decodeSuperTeamResponse(t, response, &created)
	assert.Equal(t, "platform", created.Prefix)
	assert.Equal(t, "https://platform.example", created.Links.Website)
	assert.Equal(t, core.SuperTeamRoleOwner, created.RoleLevel)
	response = superTeamRequest(t, app, http.MethodGet, "/api/super-teams/platform", "", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var publicDetails core.SuperTeamDetails
	decodeSuperTeamResponse(t, response, &publicDetails)
	assert.Zero(t, publicDetails.Team.RoleLevel)
	assert.False(t, publicDetails.Administrator)
	assert.Equal(t, "https://github.com/platform", publicDetails.Team.Links.GitHub)
	require.Len(t, publicDetails.Members, 1)
	assert.Equal(t, "alice", publicDetails.Members[0].Username)
	response = superTeamRequest(t, app, http.MethodGet,
		"/api/super-teams/eligible?minimum_role=3&limit=10&offset=0", "alice", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var eligible struct {
		Teams []core.SuperTeam `json:"teams"`
		Total int              `json:"total"`
	}
	decodeSuperTeamResponse(t, response, &eligible)
	require.Len(t, eligible.Teams, 1)
	assert.Equal(t, 1, eligible.Total)
	response = superTeamRequest(t, app, http.MethodGet,
		"/api/super-teams/eligible?minimum_role=3&limit=10&offset=0", "bob", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	decodeSuperTeamResponse(t, response, &eligible)
	assert.Empty(t, eligible.Teams)
	assert.Zero(t, eligible.Total)

	response = superTeamRequest(t, app, http.MethodPost, "/api/super-teams", "alice", map[string]any{
		"prefix": "second", "name": "Second", "description": "",
	})
	assert.Equal(t, http.StatusConflict, response.StatusCode)
	assert.Equal(t, "create_limit", response.Header.Get("X-Renop-Error-Code"))
	response.Body.Close()

	response = superTeamRequest(t, app, http.MethodGet, "/api/super-teams", "bob", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var bobList struct {
		Teams []core.SuperTeam `json:"teams"`
		Total int              `json:"total"`
	}
	decodeSuperTeamResponse(t, response, &bobList)
	assert.Empty(t, bobList.Teams)
	assert.Zero(t, bobList.Total)

	response = superTeamRequest(t, app, http.MethodGet, "/api/super-teams", "admin", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var adminList struct {
		Teams []core.SuperTeam `json:"teams"`
		Total int              `json:"total"`
	}
	decodeSuperTeamResponse(t, response, &adminList)
	require.Len(t, adminList.Teams, 1)
	assert.Equal(t, 1, adminList.Total)

	response = superTeamRequest(t, app, http.MethodPost, "/api/super-teams/platform/members", "alice", map[string]any{
		"users": []string{"bob"}, "level": core.SuperTeamRoleWrite,
	})
	require.Equal(t, http.StatusCreated, response.StatusCode)
	response.Body.Close()
	messages, err := state.GetDB().ListMessages("bob", 10, 0, "", time.Now().UnixMilli())
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "super_team_invite", messages[0].ActionKind)

	response = superTeamRequest(t, app, http.MethodPost,
		"/api/super-teams/invitations/"+messages[0].ID+"/accept", "bob", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	response.Body.Close()
	response = superTeamRequest(t, app, http.MethodGet, "/api/super-teams/platform", "bob", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var details core.SuperTeamDetails
	decodeSuperTeamResponse(t, response, &details)
	assert.Equal(t, core.SuperTeamRoleWrite, details.Team.RoleLevel)
	require.Len(t, details.Members, 2)
	memberJSON, err := json.Marshal(details.Members[0])
	require.NoError(t, err)
	assert.NotContains(t, string(memberJSON), "user_id")

	response = superTeamRequest(t, app, http.MethodGet, "/api/super-teams/platform/users/search?q=a", "bob", nil)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	response.Body.Close()

	response = superTeamRequest(t, app, http.MethodPut, "/api/super-teams/platform/members/bob", "alice",
		map[string]any{"level": core.SuperTeamRoleOwner})
	require.Equal(t, http.StatusOK, response.StatusCode)
	response.Body.Close()
	response = superTeamRequest(t, app, http.MethodDelete, "/api/super-teams/platform/membership", "alice", nil)
	require.Equal(t, http.StatusConflict, response.StatusCode)
	assert.Equal(t, "owner_cannot_leave", response.Header.Get("X-Renop-Error-Code"))
	response.Body.Close()
	response = superTeamRequest(t, app, http.MethodPut, "/api/super-teams/platform/members/alice", "alice",
		map[string]any{"level": core.SuperTeamRoleManage})
	require.Equal(t, http.StatusOK, response.StatusCode)
	response.Body.Close()
	response = superTeamRequest(t, app, http.MethodDelete, "/api/super-teams/platform/members/alice", "alice", nil)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	response.Body.Close()
	response = superTeamRequest(t, app, http.MethodDelete, "/api/super-teams/platform/membership", "alice", nil)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	response.Body.Close()
}

func TestSuperTeamAdministratorLimitOverrides(t *testing.T) {
	app, _ := setupSuperTeamApp(t)
	response := superTeamRequest(t, app, http.MethodPut, "/api/super-teams/users/bob/limits", "alice", map[string]any{
		"create_limit": 0, "join_limit": 0,
	})
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	response.Body.Close()

	response = superTeamRequest(t, app, http.MethodPut, "/api/super-teams/users/bob/limits", "admin", map[string]any{
		"create_limit": 0, "join_limit": 1,
	})
	require.Equal(t, http.StatusOK, response.StatusCode)
	var status core.SuperTeamLimitStatus
	decodeSuperTeamResponse(t, response, &status)
	assert.Zero(t, status.CreateLimit)
	assert.Equal(t, 1, status.JoinLimit)
	assert.False(t, status.CreateLimitInherited)
	assert.False(t, status.JoinLimitInherited)

	response = superTeamRequest(t, app, http.MethodPut, "/api/super-teams/users/bob/limits", "admin", map[string]any{
		"create_limit": -1, "join_limit": -1,
	})
	require.Equal(t, http.StatusOK, response.StatusCode)
	decodeSuperTeamResponse(t, response, &status)
	assert.Equal(t, 1, status.CreateLimit)
	assert.Equal(t, 3, status.JoinLimit)
	assert.True(t, status.CreateLimitInherited)
	assert.True(t, status.JoinLimitInherited)
}
