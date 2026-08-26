/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/token"
)

func TestGitHubOAuthCreatesAccountAndSingleUseSession(t *testing.T) {
	providerServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/token":
			require.NoError(t, request.ParseForm())
			assert.Equal(t, "test-code", request.Form.Get("code"))
			_ = json.NewEncoder(writer).Encode(map[string]string{
				"access_token": "provider-token",
				"token_type":   "bearer",
				"scope":        "read:user,read:org",
			})
		case "/api/user":
			assert.Equal(t, "Bearer provider-token", request.Header.Get("Authorization"))
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"id": int64(42), "login": "Octo-Cat", "name": "Octo Cat",
			})
		case "/api/user/orgs":
			_ = json.NewEncoder(writer).Encode([]map[string]any{{
				"id": int64(84), "login": "Example-Org",
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(providerServer.Close)

	cfg := config.DefaultConfig()
	cfg.Server.GitHubOAuth = config.GitHubOAuthConfig{
		Enabled: true, ClientID: "client-id", ClientSecret: "client-secret",
		CallbackURL: "https://repo.example/api/auth/github/callback",
	}
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "github-auth.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state.Inner.DB = db
	operations := make(chan token.TokenOp, 8)
	go token.StartTokenConsumer(state, operations)
	t.Cleanup(func() { close(operations) })

	app := fiber.New()
	app.Use(AuthMiddleware(state))
	setupGitHubRoutesWithProvider(app.Group("/auth"), state, operations, githubOAuthProvider{
		AuthorizeURL: "https://github.example/authorize",
		TokenURL:     providerServer.URL + "/token",
		APIURL:       providerServer.URL + "/api",
	})
	startResponse, err := app.Test(httptest.NewRequest(http.MethodGet,
		"/auth/github/start?return_to=%2Fpackages", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, startResponse.StatusCode)
	location, err := url.Parse(startResponse.Header.Get("Location"))
	require.NoError(t, err)
	stateValue := location.Query().Get("state")
	require.NotEmpty(t, stateValue)
	assert.Equal(t, "read:user read:org", location.Query().Get("scope"))
	require.NoError(t, startResponse.Body.Close())

	callbackResponse, err := app.Test(httptest.NewRequest(http.MethodGet,
		"/auth/github/callback?state="+url.QueryEscape(stateValue)+"&code=test-code", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, callbackResponse.StatusCode)
	assert.Equal(t, "/packages?github_oauth=success", callbackResponse.Header.Get("Location"))
	sessionCookie := callbackResponse.Cookies()[0]
	assert.Equal(t, "renop_session", sessionCookie.Name)
	require.NoError(t, callbackResponse.Body.Close())

	identity, err := db.GetGitHubIdentityByProviderID(42)
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, "octo-cat", identity.GitHubLogin)
	assert.Equal(t, "octo_cat", identity.Username)
	assert.Equal(t, 2, identity.PrincipalCount)
	profile, err := db.GetUserProfile(identity.Username)
	require.NoError(t, err)
	assert.Equal(t, "Octo Cat", profile.Nickname)
	authorized, err := db.HasRecentGitHubPrincipal(identity.Username, "example-org", 0)
	require.NoError(t, err)
	assert.True(t, authorized)
	sessions, err := db.ListUserSessions(identity.Username, "")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "github", sessions[0].LoginMethod)

	replayResponse, err := app.Test(httptest.NewRequest(http.MethodGet,
		"/auth/github/callback?state="+url.QueryEscape(stateValue)+"&code=test-code", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, replayResponse.StatusCode)
	assert.Equal(t, "/?github_oauth=state_invalid", replayResponse.Header.Get("Location"))
	require.NoError(t, replayResponse.Body.Close())

	disconnectRequest := httptest.NewRequest(http.MethodDelete, "/auth/profile/github", nil)
	disconnectRequest.AddCookie(sessionCookie)
	disconnectResponse, err := app.Test(disconnectRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, disconnectResponse.StatusCode)
	assert.Equal(t, "GITHUB_LAST_LOGIN_METHOD", disconnectResponse.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, disconnectResponse.Body.Close())
	require.NoError(t, token.UpdateTokenSync(operations, identity.Username, func(accessToken *core.AccessToken) {
		accessToken.EncryptedSecret = "configured-password-hash"
	}))
	disconnectRequest = httptest.NewRequest(http.MethodDelete, "/auth/profile/github", nil)
	disconnectRequest.AddCookie(sessionCookie)
	disconnectResponse, err = app.Test(disconnectRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, disconnectResponse.StatusCode)
	require.NoError(t, disconnectResponse.Body.Close())
}

func TestGitHubOAuthScopeAndReturnPathValidation(t *testing.T) {
	assert.True(t, githubScopesAuthorized("read:user, read:org"))
	assert.True(t, githubScopesAuthorized("user admin:org"))
	assert.False(t, githubScopesAuthorized("read:user"))
	assert.False(t, githubScopesAuthorized("read:org"))
	assert.Equal(t, "/", safeOAuthReturnTo("https://evil.example/"))
	assert.Equal(t, "/", safeOAuthReturnTo("//evil.example/"))
	assert.Equal(t, "/", safeOAuthReturnTo("/api/auth/github/callback"))
	assert.Equal(t, "/user/alice/edit", safeOAuthReturnTo("/user/alice/edit?ignored=yes"))
	assert.Equal(t, "github_user", githubUsernameBase(strings.Repeat("-", 50)))
}
