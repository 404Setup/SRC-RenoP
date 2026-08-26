/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

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
	"golang.org/x/crypto/bcrypt"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/token"
)

func accountSecurityRequest(t *testing.T, app *fiber.App, method, path string, body any,
	sessionToken string) *http.Response {
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

func TestRecoveryCodeHashingUsesOneWayHighEntropyVerifiers(t *testing.T) {
	codes, hashes, err := generateRecoveryCodeSet()
	require.NoError(t, err)
	require.Len(t, codes, core.RecoveryCodeCount)
	require.Len(t, hashes, core.RecoveryCodeCount)
	selectors := make(map[string]struct{}, core.RecoveryCodeCount)
	for index, code := range codes {
		parsed := parseRecoveryCode(code)
		assert.True(t, parsed.valid)
		if index == 0 {
			assert.True(t, verifyRecoveryCode(parsed.value, hashes[index].PasswordHash))
		}
		assert.NotContains(t, hashes[index].PasswordHash, code)
		assert.NotEqual(t, code, hashes[index].SelectorHash)
		selectors[hashes[index].SelectorHash] = struct{}{}
	}
	assert.Len(t, selectors, core.RecoveryCodeCount)
	wrong := parseRecoveryCode("RNP-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA")
	assert.False(t, verifyRecoveryCode(wrong.value, hashes[0].PasswordHash))
}

func TestPrivateEmailPasswordPolicyAndRecoveryRoutes(t *testing.T) {
	cfg := config.DefaultConfig()
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "account-routes.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state.Inner.DB = db
	initialHash, err := bcrypt.GenerateFromPassword([]byte("initial-password"), bcrypt.DefaultCost)
	require.NoError(t, err)
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "alice", EncryptedSecret: string(initialHash),
		CreatedAt: time.Now().UTC().Format(time.RFC3339), Permissions: []string{"base"},
		Tokens: []string{"alice-api-token"},
	}))
	state.Inner.TokensCount.Store(1)
	require.NoError(t, db.SaveFidoDevice(&core.FidoDevice{
		ID: "passkey", Username: "alice", Name: "Passkey", CredentialID: []byte("credential"),
		PublicKey: []byte("public"), CreatedAt: time.Now().UnixMilli(),
	}))
	session := &core.Session{
		PublicID: "account-session", Username: "alice", CreatedAt: time.Now().UnixMilli(),
		LoginMethod: "password",
	}
	session.LastActive.Store(time.Now().UnixMilli())
	const sessionToken = "account-session-secret"
	require.NoError(t, state.SaveSession(session, sessionToken))

	operations := make(chan token.TokenOp, 8)
	go token.StartTokenConsumer(state, operations)
	t.Cleanup(func() { close(operations) })
	app := fiber.New()
	app.Use(AuthMiddleware(state))
	SetupAuthRoutes(app, state, operations)

	response := accountSecurityRequest(t, app, http.MethodPut, "/auth/profile/email",
		map[string]any{"email": "Alice@Example.COM"}, sessionToken)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	emailToken, err := db.GetTokenByEmail("alice@example.com")
	require.NoError(t, err)
	require.NotNil(t, emailToken)
	assert.Equal(t, "alice", emailToken.Name)
	publicResponse, err := app.Test(httptest.NewRequest(http.MethodGet, "/users/alice/profile", nil))
	require.NoError(t, err)
	publicBody := new(bytes.Buffer)
	_, err = publicBody.ReadFrom(publicResponse.Body)
	require.NoError(t, err)
	require.NoError(t, publicResponse.Body.Close())
	assert.NotContains(t, publicBody.String(), "alice@example.com")
	assert.NotContains(t, publicBody.String(), "\"email\"")

	response = accountSecurityRequest(t, app, http.MethodPut, "/auth/profile/password-login",
		map[string]any{"enabled": false}, sessionToken)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	authenticated, err := AuthenticateUser(state, &core.LoginRequest{
		Name: "alice@example.com", Secret: "initial-password",
	}, operations)
	require.NoError(t, err)
	assert.Nil(t, authenticated)
	accessToken := state.GetTokenByName("alice")
	require.NotNil(t, accessToken)
	assert.False(t, verifyTokenSecret(state, accessToken, "initial-password"))
	assert.True(t, verifyTokenSecret(state, accessToken, "alice-api-token"))
	require.ErrorIs(t, db.DeleteFidoDevice("alice", "passkey"), core.ErrLastLoginMethod)

	response = accountSecurityRequest(t, app, http.MethodPost, "/auth/profile/recovery-codes", nil, sessionToken)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var generated struct {
		Codes []string `json:"codes"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&generated))
	require.NoError(t, response.Body.Close())
	require.Len(t, generated.Codes, core.RecoveryCodeCount)

	recoveryBody := map[string]any{
		"identifier":   "alice@example.com",
		"codes":        generated.Codes[:core.RecoveryCodesRequired],
		"new_password": "recovered-password",
	}
	response = accountSecurityRequest(t, app, http.MethodPost, "/auth/recovery/password", recoveryBody, "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	assert.Nil(t, state.GetSession(sessionToken))
	security, err := db.GetAccountSecurity("alice")
	require.NoError(t, err)
	assert.True(t, security.PasswordLoginEnabled)
	assert.Equal(t, core.RecoveryCodeCount-core.RecoveryCodesRequired, security.RecoveryCodesRemaining)
	authenticated, err = AuthenticateUser(state, &core.LoginRequest{
		Name: "alice@example.com", Secret: "recovered-password",
	}, operations)
	require.NoError(t, err)
	require.NotNil(t, authenticated)
	assert.Equal(t, "alice", authenticated.Username)

	response = accountSecurityRequest(t, app, http.MethodPost, "/auth/recovery/password", recoveryBody, "")
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.NoError(t, response.Body.Close())
}
