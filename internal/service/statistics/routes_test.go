/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package statistics

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/auth"
)

func statisticsTestToken(t *testing.T, db *database.DB, username string, scopes []string) string {
	t.Helper()
	secret, err := core.GenerateAPITokenSecret()
	require.NoError(t, err)
	require.NoError(t, db.CreateAPIToken(username, &core.APIToken{
		ID: uuid.NewString(), Name: "Statistics " + uuid.NewString(), Scopes: scopes, CreatedAt: time.Now().UnixMilli(),
	}, core.HashAPITokenSecret(secret)))
	return secret
}

func statisticsRequest(t *testing.T, app *fiber.App, path, secret string) *http.Response {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if secret != "" {
		request.Header.Set(fiber.HeaderAuthorization, "Bearer "+secret)
	}
	response, err := app.Test(request)
	require.NoError(t, err)
	return response
}

func TestStatisticsRoutesRequireScopedAPITokensAndStableUserBoundaries(t *testing.T) {
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "statistics-routes.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", Permissions: []string{"base"}}))
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "admin", Permissions: []string{"admin"}}))
	aliceSecret := statisticsTestToken(t, db, "alice", []string{core.APITokenScopeStatisticsRead})
	unscopedSecret := statisticsTestToken(t, db, "alice", []string{core.APITokenScopeRepositoryRead})
	adminSecret := statisticsTestToken(t, db, "admin", []string{
		core.APITokenScopeStatisticsRead, core.APITokenScopeAdminStatistics,
	})
	require.NoError(t, db.BatchIncrementDownloadStatistics([]*core.DownloadStatisticDelta{
		{Username: "alice", Repository: "releases", Format: "maven", Namespace: "com.example",
			Package: "com.example:demo", Version: "1.0", Count: 2, Bytes: 2048, UpdatedAt: time.Now().UnixMilli()},
		{Username: "admin", Repository: "containers", Format: "docker", Package: "team/app",
			Version: "latest", Count: 1, Bytes: 4096, UpdatedAt: time.Now().UnixMilli()},
	}))
	state := core.NewAppState()
	state.Inner.DB = db
	state.Inner.Config.Store(config.DefaultConfig())
	app := fiber.New()
	app.Use(auth.AuthMiddleware(state))
	SetupRoutes(app.Group("/api"), state)

	response := statisticsRequest(t, app, "/api/statistics", "")
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.NoError(t, response.Body.Close())
	sessionToken := strings.Repeat("s", 48)
	require.NoError(t, state.SaveSession(&core.Session{
		PublicID: "statistics-browser-session", Username: "alice", CreatedAt: time.Now().UnixMilli(),
	}, sessionToken))
	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/statistics", nil)
	sessionRequest.AddCookie(&http.Cookie{Name: "renop_session", Value: sessionToken})
	response, err = app.Test(sessionRequest)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = statisticsRequest(t, app, "/api/statistics", aliceSecret)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var own core.DownloadStatisticsPage
	require.NoError(t, json.NewDecoder(response.Body).Decode(&own))
	require.NoError(t, response.Body.Close())
	assert.Equal(t, int64(2), own.Count)
	require.Len(t, own.Records, 1)
	assert.Equal(t, "releases", own.Records[0].Repository)
	response = statisticsRequest(t, app, "/api/statistics", unscopedSecret)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = statisticsRequest(t, app, "/api/statistics?offset=1000001", aliceSecret)
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())

	response = statisticsRequest(t, app, "/api/statistics/system?group_by=user", aliceSecret)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = statisticsRequest(t, app, "/api/statistics/users/admin", aliceSecret)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())

	response = statisticsRequest(t, app, "/api/statistics/system?group_by=user", adminSecret)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var system core.DownloadStatisticsPage
	require.NoError(t, json.NewDecoder(response.Body).Decode(&system))
	require.NoError(t, response.Body.Close())
	assert.Equal(t, int64(3), system.Count)
	assert.Equal(t, 2, system.Total)
}
