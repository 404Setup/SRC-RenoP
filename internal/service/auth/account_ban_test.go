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

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/proto"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	serviceToken "renop/internal/service/token"
	"renop/internal/testutil"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func TestAccountBanRevokesEveryCredentialAndCanBeLifted(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Maven.Repositories = map[string]*config.Repository{
		"files": {Name: "files", Format: config.RepositoryFormatFiles, Visibility: "PUBLIC"},
	}
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "account-ban-auth.db"), MaxOpenConns: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state.Inner.DB = db
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("alice-password"), bcrypt.DefaultCost)
	require.NoError(t, err)
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "admin", Permissions: []string{"admin"}, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}))
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "alice", EncryptedSecret: string(passwordHash), Permissions: []string{"base"},
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}))
	state.Inner.TokensCount.Store(2)
	apiSecret, err := core.GenerateAPITokenSecret()
	require.NoError(t, err)
	apiToken := &core.APIToken{
		ID: uuid.NewString(), Name: "Automation", Scopes: []string{core.APITokenScopeRepositoryRead},
		CreatedAt: time.Now().UnixMilli(),
	}
	require.NoError(t, db.CreateAPIToken("alice", apiToken, core.HashAPITokenSecret(apiSecret)))

	newSession := func(username, token string) {
		t.Helper()
		session := &core.Session{PublicID: uuid.NewString(), Username: username, CreatedAt: time.Now().UnixMilli()}
		session.LastActive.Store(time.Now().UnixMilli())
		require.NoError(t, state.SaveSession(session, token))
	}
	const adminSession = "admin-ban-session"
	const aliceSession = "alice-ban-session"
	newSession("admin", adminSession)
	newSession("alice", aliceSession)

	operations := make(chan serviceToken.TokenOp, 1)
	app := fiber.New()
	app.Use(AuthMiddleware(state))
	SetupAuthRoutes(app.Group("/api"), state, operations)
	serviceToken.SetupTokenRoutes(app.Group("/api"), state, operations)
	app.Get("/files/artifact", func(c fiber.Ctx) error { return c.SendString(GetUser(c).Username) })

	requestAPI := func() *http.Response {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/files/artifact", nil)
		request.Header.Set(fiber.HeaderAuthorization, "Bearer "+apiSecret)
		response, requestErr := app.Test(request)
		require.NoError(t, requestErr)
		return response
	}
	response := requestAPI()
	assert.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = apiTokenJSONRequest(t, app, http.MethodGet, "/api/auth/me", aliceSession, nil)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())

	response = apiTokenJSONRequest(t, app, http.MethodPut, "/api/tokens/alice/ban", adminSession,
		map[string]any{"reason": "Repeated abuse", "expires_at": nil})
	assert.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	assert.Nil(t, state.GetSession(aliceSession))
	response = requestAPI()
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = apiTokenJSONRequest(t, app, http.MethodGet, "/api/auth/me", aliceSession, nil)
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.NoError(t, response.Body.Close())
	authenticated, err := AuthenticateUser(state, &core.LoginRequest{
		Name: "alice", Secret: "alice-password",
	}, operations)
	assert.Nil(t, authenticated)
	require.ErrorIs(t, err, core.ErrAccountBanned)
	loginBody, err := proto.Marshal(&pb.LoginRequest{Name: "alice", Secret: "alice-password"})
	require.NoError(t, err)
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginRequest.Header.Set(fiber.HeaderContentType, protohttp.ContentType)
	response, err = app.Test(loginRequest)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	assert.Equal(t, "ACCOUNT_BANNED", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())

	response = apiTokenJSONRequest(t, app, http.MethodGet, "/api/tokens", adminSession, nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "no-store", response.Header.Get(fiber.HeaderCacheControl))
	encoded, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	var list pb.AccessTokenList
	require.NoError(t, proto.Unmarshal(encoded, &list))
	foundBan := false
	for _, account := range list.Tokens {
		if account.GetName() == "alice" {
			foundBan = account.GetBan() != nil && account.GetBan().GetReason() == "Repeated abuse"
		}
	}
	assert.True(t, foundBan)

	response = apiTokenJSONRequest(t, app, http.MethodPut, "/api/tokens/admin/ban", adminSession,
		map[string]any{"reason": "Self ban", "expires_at": nil})
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	assert.Equal(t, "ACCOUNT_BAN_SELF", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())

	response = apiTokenJSONRequest(t, app, http.MethodDelete, "/api/tokens/alice/ban", adminSession, nil)
	assert.Equal(t, http.StatusNoContent, response.StatusCode)
	require.NoError(t, response.Body.Close())
	unbannedAccount, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	require.Nil(t, unbannedAccount.Ban)
	response = requestAPI()
	assert.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	unbannedAccount, err = db.GetTokenByName("alice")
	require.NoError(t, err)
	require.Nil(t, unbannedAccount.Ban)
	authenticated, err = AuthenticateUser(state, &core.LoginRequest{
		Name: "alice", Secret: "alice-password",
	}, operations)
	require.NoError(t, err)
	require.NotNil(t, authenticated)
	assert.Nil(t, state.GetSession(aliceSession), "lifting a ban must not restore revoked sessions")
}
