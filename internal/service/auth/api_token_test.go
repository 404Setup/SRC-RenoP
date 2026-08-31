/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/token"
	"renop/internal/testutil"
)

func apiTokenJSONRequest(t *testing.T, app *fiber.App, method, path, sessionToken string, body any) *http.Response {
	t.Helper()
	var payload bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&payload).Encode(body))
	}
	request := httptest.NewRequest(method, path, &payload)
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	if sessionToken != "" {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	}
	response, err := app.Test(request)
	require.NoError(t, err)
	return response
}

func TestFineGrainedAPITokenRoutesAndAuthorizationBoundaries(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Maven.Repositories = map[string]*config.Repository{
		"files": {Name: "files", Format: config.RepositoryFormatFiles, Visibility: "PUBLIC"},
		"other": {Name: "other", Format: config.RepositoryFormatFiles, Visibility: "PUBLIC"},
	}
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "fine-grained-api-tokens.db"),
		MaxOpenConns: 2, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state.Inner.DB = db
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("account-password"), bcrypt.DefaultCost)
	require.NoError(t, err)
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "alice", EncryptedSecret: string(passwordHash), Permissions: []string{"base"},
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}))
	state.Inner.TokensCount.Store(1)
	const sessionToken = "api-token-browser-session"
	session := &core.Session{PublicID: "api-token-session", Username: "alice", CreatedAt: time.Now().UnixMilli()}
	session.LastActive.Store(time.Now().UnixMilli())
	require.NoError(t, state.SaveSession(session, sessionToken))

	operations := make(chan token.TokenOp, 4)
	go token.StartTokenConsumer(state, operations)
	t.Cleanup(func() { close(operations) })
	app := fiber.New()
	app.Use(AuthMiddleware(state))
	SetupAuthRoutes(app.Group("/api"), state, operations)
	app.Get("/files/artifact", func(c fiber.Ctx) error { return c.SendString(GetUser(c).Username) })
	app.Get("/other/artifact", func(c fiber.Ctx) error { return c.SendString(GetUser(c).Username) })
	app.Post("/files/artifact", func(c fiber.Ctx) error { return c.SendString(GetUser(c).Username) })
	app.Get("/automation", func(c fiber.Ctx) error { return c.SendString(GetUser(c).Username) })

	response := apiTokenJSONRequest(t, app, http.MethodGet, "/api/auth/profile/api-tokens/scopes", sessionToken, nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var scopeResult struct {
		Scopes      []string          `json:"scopes"`
		TargetKinds map[string]string `json:"target_kinds"`
		TargetLimit int               `json:"target_limit"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&scopeResult))
	require.NoError(t, response.Body.Close())
	assert.Contains(t, scopeResult.Scopes, core.APITokenScopeRepositoryRead)
	assert.Contains(t, scopeResult.Scopes, core.APITokenScopeTeamManage)
	assert.Contains(t, scopeResult.Scopes, core.APITokenScopeDomainVerify)
	assert.NotContains(t, scopeResult.Scopes, core.APITokenScopePackageManage)
	assert.NotContains(t, scopeResult.Scopes, core.APITokenScopeDomainManage)
	assert.NotContains(t, scopeResult.Scopes, core.APITokenScopeAdminSettings)
	assert.Equal(t, "repository", scopeResult.TargetKinds[core.APITokenScopeRepositoryRead])
	assert.Equal(t, "team", scopeResult.TargetKinds[core.APITokenScopeTeamManage])
	assert.Equal(t, core.MaxAPITokenTargets, scopeResult.TargetLimit)

	expiresAt := time.Now().Add(24 * time.Hour).UnixMilli()
	response = apiTokenJSONRequest(t, app, http.MethodPost, "/api/auth/profile/api-tokens", sessionToken, map[string]any{
		"name": "Read automation", "scopes": []string{core.APITokenScopeRepositoryRead},
		"targets": map[string][]string{core.APITokenScopeRepositoryRead: {"files"}}, "expires_at": expiresAt,
	})
	require.Equal(t, http.StatusCreated, response.StatusCode)
	assert.Equal(t, "no-store", response.Header.Get(fiber.HeaderCacheControl))
	var created struct {
		Token  core.APIToken `json:"token"`
		Secret string        `json:"secret"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&created))
	require.NoError(t, response.Body.Close())
	assert.True(t, len(created.Secret) == len("rnp_pat_")+43)
	assert.True(t, bytes.HasPrefix([]byte(created.Secret), []byte("rnp_pat_")))
	assert.Equal(t, []string{core.APITokenScopeRepositoryRead}, created.Token.Scopes)
	assert.Equal(t, []string{"files"}, created.Token.Targets[core.APITokenScopeRepositoryRead])
	request := httptest.NewRequest(http.MethodGet, "/api/auth/profile/api-tokens", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+created.Secret)
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())

	request = httptest.NewRequest(http.MethodGet, "/automation", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+created.Secret)
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())
	request = httptest.NewRequest(http.MethodGet, "/other/artifact", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+created.Secret)
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	assert.Equal(t, core.APITokenScopeRepositoryRead, response.Header.Get("X-Renop-Required-Scope"))
	require.NoError(t, response.Body.Close())
	request = httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+created.Secret)
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	assert.Equal(t, core.APITokenScopeAccountRead, response.Header.Get("X-Renop-Required-Scope"))
	require.NoError(t, response.Body.Close())

	request = httptest.NewRequest(http.MethodGet, "/automation?token="+created.Secret, nil)
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	queryBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "guest", string(queryBody), "query-string credentials must be ignored")

	request = httptest.NewRequest(http.MethodPost, "/files/artifact", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+created.Secret)
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	assert.Equal(t, core.APITokenScopeRepositoryPublish, response.Header.Get("X-Renop-Required-Scope"))
	require.NoError(t, response.Body.Close())

	request = httptest.NewRequest(http.MethodGet, "/automation", nil)
	request.SetBasicAuth("alice", created.Secret)
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())
	request = httptest.NewRequest(http.MethodGet, "/files/artifact", nil)
	request.SetBasicAuth("alice", created.Secret)
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())

	request = httptest.NewRequest(http.MethodGet, "/automation", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Session "+sessionToken)
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.NoError(t, response.Body.Close())
	request = httptest.NewRequest(http.MethodGet, "/automation", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())

	response = apiTokenJSONRequest(t, app, http.MethodPost, "/api/auth/profile/api-tokens", sessionToken, map[string]any{
		"name": "Escalation", "scopes": []string{core.APITokenScopeAdminSettings},
	})
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = apiTokenJSONRequest(t, app, http.MethodPost, "/api/auth/profile/api-tokens", sessionToken, map[string]any{
		"name": core.LegacyAPITokenNamePrefix + "spoof", "scopes": []string{core.APITokenScopeRepositoryRead},
	})
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())

	response = apiTokenJSONRequest(t, app, http.MethodGet, "/api/auth/profile/api-tokens", sessionToken, nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var listed struct {
		Tokens []core.APIToken `json:"tokens"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&listed))
	require.NoError(t, response.Body.Close())
	require.Len(t, listed.Tokens, 1)
	assert.NotContains(t, string(mustJSONMarshal(t, listed)), created.Secret)

	response = apiTokenJSONRequest(t, app, http.MethodDelete,
		"/api/auth/profile/api-tokens/"+created.Token.ID, sessionToken, nil)
	assert.Equal(t, http.StatusNoContent, response.StatusCode)
	require.NoError(t, response.Body.Close())
	request = httptest.NewRequest(http.MethodGet, "/automation", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+created.Secret)
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.NoError(t, response.Body.Close())
}

func mustJSONMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

func TestExpiredAPITokenIsRejected(t *testing.T) {
	state := core.NewAppState()
	state.Inner.Config.Store(config.DefaultConfig())
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "expired-api-token.db"), MaxOpenConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state.Inner.DB = db
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", Permissions: []string{"base"}}))
	secret, err := core.GenerateAPITokenSecret()
	require.NoError(t, err)
	createdAt := time.Now().Add(-2 * time.Hour).UnixMilli()
	expiresAt := time.Now().Add(-time.Hour).UnixMilli()
	require.NoError(t, db.CreateAPIToken("alice", &core.APIToken{
		ID: uuid.NewString(), Name: "Expired", Scopes: []string{core.APITokenScopeRepositoryRead},
		CreatedAt: createdAt, ExpiresAt: &expiresAt,
	}, core.HashAPITokenSecret(secret)))
	app := fiber.New()
	app.Use(AuthMiddleware(state))
	app.Get("/automation", func(c fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
	request := httptest.NewRequest(http.MethodGet, "/automation", nil)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+secret)
	response, err := app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())
}

func TestAPITokenScopesRemainCappedByCurrentAccountPermissions(t *testing.T) {
	state := core.NewAppState()
	state.Inner.Config.Store(config.DefaultConfig())
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "api-token-account-cap.db"), MaxOpenConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state.Inner.DB = db
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "admin", Permissions: []string{"admin"}}))
	secret, err := core.GenerateAPITokenSecret()
	require.NoError(t, err)
	require.NoError(t, db.CreateAPIToken("admin", &core.APIToken{
		ID: uuid.NewString(), Name: "Settings automation",
		Scopes: []string{core.APITokenScopeAdminSettings}, CreatedAt: time.Now().UnixMilli(),
	}, core.HashAPITokenSecret(secret)))
	app := fiber.New()
	app.Use(AuthMiddleware(state))
	app.Get("/api/settings/test", func(c fiber.Ctx) error { return c.SendString(GetUser(c).Username) })
	request := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/settings/test", nil)
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+secret)
		response, requestErr := app.Test(req)
		require.NoError(t, requestErr)
		defer response.Body.Close()
		return response.StatusCode
	}
	require.Equal(t, http.StatusOK, request())
	require.NoError(t, db.UpdateToken("admin", func(account *core.AccessToken) {
		account.Permissions = []string{"base"}
	}))
	state.InvalidateAccountAuthCache(false, "admin")
	assert.Equal(t, http.StatusForbidden, request())
}

func TestRequiredAPITokenScopeMatchesEndpointCapability(t *testing.T) {
	state := core.NewAppState()
	cfg := config.DefaultConfig()
	cfg.Maven.Repositories = map[string]*config.Repository{
		"cargo": {Name: "cargo", Format: config.RepositoryFormatCargo},
		"npm":   {Name: "npm", Format: config.RepositoryFormatNPM},
	}
	state.Inner.Config.Store(cfg)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		return c.SendString(requiredAPITokenScope(c, state).Scope)
	})

	tests := []struct {
		method string
		path   string
		scope  string
	}{
		{http.MethodGet, "/api/auth/me", core.APITokenScopeAccountRead},
		{http.MethodGet, "/api/auth/users/alice/audit-logs", core.APITokenScopeAdminAudit},
		{http.MethodGet, "/api/docker/repositories/releases/images", core.APITokenScopeRepositoryRead},
		{http.MethodPost, "/api/docker/repositories/releases/images", core.APITokenScopePackageCreate},
		{http.MethodDelete, "/api/docker/repositories/releases/images/demo", core.APITokenScopeRepositoryDelete},
		{http.MethodGet, "/api/docker/repositories/releases/owners", core.APITokenScopeTeamManage},
		{http.MethodDelete, "/api/docker/repositories/releases/owners/alice", core.APITokenScopeTeamManage},
		{http.MethodGet, "/api/npm/repositories/npm/packages", core.APITokenScopeRepositoryRead},
		{http.MethodPost, "/api/npm/repositories/npm/packages", core.APITokenScopePackageCreate},
		{http.MethodPut, "/api/npm/repositories/npm/packages?package=demo", core.APITokenScopePackageMetadata},
		{http.MethodDelete, "/api/npm/repositories/npm/packages?package=demo", core.APITokenScopePackageLifecycle},
		{http.MethodDelete, "/api/npm/repositories/npm/versions?package=demo&version=1.0.0", core.APITokenScopePackageLifecycle},
		{http.MethodGet, "/api/npm/repositories/npm/owners?package=demo", core.APITokenScopeTeamManage},
		{http.MethodGet, "/api/maven/details/releases/demo.jar", core.APITokenScopeRepositoryRead},
		{http.MethodGet, "/api/maven/signatures/releases/demo.jar", core.APITokenScopeRepositoryRead},
		{http.MethodGet, "/api/maven/repositories/releases/packages", core.APITokenScopeRepositoryRead},
		{http.MethodPut, "/api/maven/repositories/releases/package", core.APITokenScopePackageMetadata},
		{http.MethodDelete, "/api/maven/repositories/releases/versions", core.APITokenScopeRepositoryDelete},
		{http.MethodGet, "/api/maven/repositories/releases/domains/example.com", core.APITokenScopeDomainRead},
		{http.MethodPost, "/api/maven/domains", core.APITokenScopeDomainCreate},
		{http.MethodPost, "/api/maven/domains/example.com/verify", core.APITokenScopeDomainVerify},
		{http.MethodDelete, "/api/maven/domains/example.com", core.APITokenScopeDomainDelete},
		{http.MethodPut, "/api/maven/domains/example.com/members/alice", core.APITokenScopeTeamManage},
		{http.MethodGet, "/api/statistics/users/alice", core.APITokenScopeStatisticsRead},
		{http.MethodGet, "/api/statistics/system/repositories", core.APITokenScopeAdminStatistics},
		{http.MethodGet, "/api/super-teams", core.APITokenScopeTeamManage},
		{http.MethodGet, "/api/super-teams/eligible", core.APITokenScopeTeamManage},
		{http.MethodPost, "/api/super-teams", core.APITokenScopeTeamManage},
		{http.MethodGet, "/api/super-teams/platform", core.APITokenScopeTeamManage},
		{http.MethodGet, "/api/super-teams/limits", core.APITokenScopeAccountRead},
		{http.MethodPut, "/api/super-teams/users/alice/limits", core.APITokenScopeAdminUsers},
		{http.MethodGet, "/api/publication-quota", core.APITokenScopeAccountRead},
		{http.MethodGet, "/api/publication-quota/users/alice", core.APITokenScopeAccountRead},
		{http.MethodPut, "/api/publication-quota/users/alice", core.APITokenScopeAdminUsers},
		{http.MethodGet, "/api/publication-quota/super-teams/platform", core.APITokenScopeTeamManage},
		{http.MethodPut, "/api/publication-quota/super-teams/platform", core.APITokenScopeAdminSettings},
		{http.MethodPut, "/api/settings/service", core.APITokenScopeAdminSettings},
		{http.MethodPut, "/api/settings/repositories/releases", core.APITokenScopeAdminRepositories},
		{http.MethodPost, "/api/settings/index/rebuild", core.APITokenScopeAdminRepositories},
		{http.MethodPut, "/cargo/api/v1/crates/new", core.APITokenScopeRepositoryPublish},
		{http.MethodGet, "/cargo/api/v1/crates/demo/owners", core.APITokenScopeTeamManage},
		{http.MethodDelete, "/cargo/api/v1/crates/demo/owners/alice", core.APITokenScopeTeamManage},
		{http.MethodDelete, "/cargo/api/v1/crates/demo/1.0.0/yank", core.APITokenScopePackageLifecycle},
		{http.MethodPut, "/cargo/api/v1/crates/demo/1.0.0/unyank", core.APITokenScopePackageLifecycle},
		{http.MethodGet, "/npm/demo", core.APITokenScopeRepositoryRead},
		{http.MethodPut, "/npm/demo", core.APITokenScopeRepositoryPublish},
	}

	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			response, err := app.Test(httptest.NewRequest(test.method, test.path, nil))
			require.NoError(t, err)
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			assert.Equal(t, test.scope, string(body))
		})
	}
	assert.True(t, requireAPITokenScope(core.APITokenScopeTeamManage,
		core.APITokenScopePackageManage).allows([]string{core.APITokenScopePackageManage}, nil))
	assert.True(t, requireAPITokenScope(core.APITokenScopeDomainVerify,
		core.APITokenScopeDomainManage).allows([]string{core.APITokenScopeDomainManage}, nil))
	assert.False(t, requireAPITokenScope(core.APITokenScopeTeamManage,
		core.APITokenScopePackageManage).allows([]string{core.APITokenScopePackageMetadata}, nil))
	assert.False(t, requireAPITokenScope(core.APITokenScopeDomainVerify,
		core.APITokenScopeDomainManage).allows([]string{core.APITokenScopeDomainRead}, nil))
	restricted := map[string][]string{core.APITokenScopeRepositoryPublish: {"releases"}}
	assert.True(t, requireAPITokenTarget(core.APITokenScopeRepositoryPublish, "releases").
		allows([]string{core.APITokenScopeRepositoryPublish}, restricted))
	assert.False(t, requireAPITokenTarget(core.APITokenScopeRepositoryPublish, "snapshots").
		allows([]string{core.APITokenScopeRepositoryPublish}, restricted))
	assert.True(t, requireAPITokenTarget(core.APITokenScopeRepositoryPublish, "snapshots").
		allows([]string{core.APITokenScopeRepositoryPublish}, nil))
	globalTarget, valid := normalizeTeamTarget("global/Platform_Team")
	assert.True(t, valid)
	assert.Equal(t, "global/platform_team", globalTarget)
}
