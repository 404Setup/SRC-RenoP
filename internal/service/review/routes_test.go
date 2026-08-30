/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package review

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

func setupReviewApp(t *testing.T) (*fiber.App, *core.AppState, *config.User, *string) {
	t.Helper()
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "reviews.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, username := range []string{"alice", "bob", "charlie", "dana"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{
			Name: username, CreatedAt: time.Now().Format(time.RFC3339),
		}))
	}
	now := time.Now().UnixMilli()
	require.NoError(t, db.CreateSuperTeam(&core.SuperTeam{
		Prefix: "platform", Name: "Platform", CreatedAt: now,
	}, "alice", 5, 10))
	require.NoError(t, db.ForceAddSuperTeamMembers("platform", "alice", []string{"bob"},
		core.SuperTeamRoleManage, 5, 10, now+1))
	require.NoError(t, db.ForceAddSuperTeamMembers("platform", "alice", []string{"charlie"},
		core.SuperTeamRoleRead, 5, 10, now+2))
	_, err = db.CreateDockerImage("containers", "personal", "charlie", false, now+3)
	require.NoError(t, err)
	state := core.NewAppState()
	state.Inner.DB = db
	current := &config.User{Username: "charlie", Roles: []string{"base"}}
	credentialKind := "session"
	app := fiber.New(fiber.Config{JSONEncoder: json.Marshal, JSONDecoder: json.Unmarshal})
	t.Cleanup(func() { require.NoError(t, app.Shutdown()) })
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", current)
		c.Locals("auth_credential_kind", credentialKind)
		return c.Next()
	})
	SetupRoutes(app.Group("/api"), state)
	return app, state, current, &credentialKind
}

func reviewRequest(t *testing.T, app *fiber.App, method, path string, body any) *http.Response {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}
	response, err := app.Test(request)
	require.NoError(t, err)
	return response
}

func TestReviewRoutesCreateListAndSingleDecision(t *testing.T) {
	app, state, current, _ := setupReviewApp(t)
	response := reviewRequest(t, app, http.MethodPost, "/api/reviews/super-team-transfers",
		core.SuperTeamTransferRequest{
			ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
			ResourceKey: "personal", TargetTeamPrefix: "platform",
		})
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var task core.ReviewTask
	require.NoError(t, json.NewDecoder(response.Body).Decode(&task))
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "charlie", task.RequestedBy)
	assert.Equal(t, "platform", task.ReviewTeamPrefix)

	*current = config.User{Username: "bob", Roles: []string{"base"}}
	response = reviewRequest(t, app, http.MethodGet,
		"/api/reviews?view=reviewer&status=pending&types=docker_image&limit=10&offset=0", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var page struct {
		Tasks []*core.ReviewTask `json:"tasks"`
		Total int                `json:"total"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&page))
	require.NoError(t, response.Body.Close())
	require.Len(t, page.Tasks, 1)
	assert.Equal(t, 1, page.Total)

	*current = config.User{Username: "dana", Roles: []string{"base"}}
	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+task.ID+"/decision",
		decisionRequest{Decision: core.ReviewStatusApproved})
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	assert.Equal(t, "review_permission", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())

	*current = config.User{Username: "bob", Roles: []string{"base"}}
	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+task.ID+"/decision",
		decisionRequest{Decision: core.ReviewStatusApproved})
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	image, err := state.GetDB().GetDockerImage("containers", "personal")
	require.NoError(t, err)
	assert.Equal(t, "platform", image.SuperTeamPrefix)

	*current = config.User{Username: "alice", Roles: []string{"base"}}
	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+task.ID+"/decision",
		decisionRequest{Decision: core.ReviewStatusRejected, Reason: "Already handled"})
	require.Equal(t, http.StatusConflict, response.StatusCode)
	assert.Equal(t, "review_decided", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())
}

func TestReviewRoutesRejectInvalidFiltersAndEmptyRejectionReasons(t *testing.T) {
	app, _, current, _ := setupReviewApp(t)
	response := reviewRequest(t, app, http.MethodGet,
		"/api/reviews?view=reviewer&status=unknown&limit=10&offset=0", nil)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.Equal(t, "invalid_request", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())

	response = reviewRequest(t, app, http.MethodGet,
		"/api/reviews?view=reviewer&types=unknown&limit=10&offset=0", nil)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())

	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/super-team-transfers",
		core.SuperTeamTransferRequest{
			ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
			ResourceKey: "personal", TargetTeamPrefix: "platform",
		})
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var task core.ReviewTask
	require.NoError(t, json.NewDecoder(response.Body).Decode(&task))
	require.NoError(t, response.Body.Close())

	*current = config.User{Username: "bob", Roles: []string{"base"}}
	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+task.ID+"/decision",
		decisionRequest{Decision: core.ReviewStatusRejected})
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.Equal(t, "invalid_request", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())
}

func TestReviewRoutesRequireBrowserSession(t *testing.T) {
	app, _, _, credentialKind := setupReviewApp(t)
	for _, kind := range []string{"api_token", "password"} {
		*credentialKind = kind
		response := reviewRequest(t, app, http.MethodGet,
			"/api/reviews?view=requested&status=pending&limit=10&offset=0", nil)
		require.Equal(t, http.StatusForbidden, response.StatusCode, kind)
		assert.Equal(t, "review_permission", response.Header.Get("X-Renop-Error-Code"), kind)
		require.NoError(t, response.Body.Close())
	}
}

func TestSystemAdministratorListsAndDecidesTeamReview(t *testing.T) {
	app, state, current, _ := setupReviewApp(t)
	response := reviewRequest(t, app, http.MethodPost, "/api/reviews/super-team-transfers",
		core.SuperTeamTransferRequest{
			ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
			ResourceKey: "personal", TargetTeamPrefix: "platform",
		})
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var task core.ReviewTask
	require.NoError(t, json.NewDecoder(response.Body).Decode(&task))
	require.NoError(t, response.Body.Close())
	now := time.Now().UnixMilli()
	_, err := state.GetDB().CreateDockerImage("containers", "reject-review", "charlie", false, now)
	require.NoError(t, err)
	rejectTask, err := state.GetDB().CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
		ResourceKey: "reject-review", TargetTeamPrefix: "platform",
	}, "charlie", false, now+1)
	require.NoError(t, err)
	require.NoError(t, state.GetDB().SaveToken(&core.AccessToken{
		Name: "dana", CreatedAt: time.Now().Format(time.RFC3339), Permissions: []string{"base", "manager"},
	}))
	*current = config.User{Username: "dana", Roles: []string{"manager"}}

	response = reviewRequest(t, app, http.MethodGet,
		"/api/reviews?view=reviewer&status=pending&limit=10&offset=0", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var page struct {
		Tasks []*core.ReviewTask `json:"tasks"`
		Total int                `json:"total"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&page))
	require.NoError(t, response.Body.Close())
	require.Equal(t, 2, page.Total)
	require.Len(t, page.Tasks, 2)

	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+task.ID+"/decision",
		decisionRequest{Decision: core.ReviewStatusApproved})
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+rejectTask.ID+"/decision",
		decisionRequest{Decision: core.ReviewStatusRejected, Reason: "Not approved"})
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
}
