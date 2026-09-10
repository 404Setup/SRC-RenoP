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
	"google.golang.org/protobuf/proto"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	serviceToken "renop/internal/service/token"
	"renop/internal/testutil"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func TestSelfServiceAccountRetirementRevokesLoginAndSupportsEarlyRetentionCleanup(t *testing.T) {
	state := core.NewAppState()
	state.Inner.Config.Store(config.DefaultConfig())
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "account-retirement-routes.db"), MaxOpenConns: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state.Inner.DB = db
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("alice-password"), bcrypt.MinCost)
	require.NoError(t, err)
	for _, account := range []*core.AccessToken{
		{Name: "admin", EncryptedSecret: "admin", Permissions: []string{"manager"}},
		{Name: "alice", EncryptedSecret: string(passwordHash), Permissions: []string{"base"}},
	} {
		account.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		require.NoError(t, db.SaveToken(account))
	}
	state.Inner.TokensCount.Store(2)
	_, err = db.UpdateAccountEmail("alice", "alice@example.com", time.Now().UnixMilli())
	require.NoError(t, err)
	require.NoError(t, db.SaveAuditLog(&core.AuditLogEntry{
		Username: "alice", Operator: "alice", Action: "LOGIN", CreatedAt: time.Now().UnixMilli(),
	}))
	newSession := func(username, secret string) {
		t.Helper()
		session := &core.Session{PublicID: secret, Username: username, CreatedAt: time.Now().UnixMilli()}
		session.LastActive.Store(time.Now().UnixMilli())
		require.NoError(t, state.SaveSession(session, secret))
	}
	const aliceSession = "alice-retirement-session"
	const adminSession = "admin-retirement-session"
	newSession("alice", aliceSession)
	newSession("admin", adminSession)
	operations := make(chan serviceToken.TokenOp, 8)
	go serviceToken.StartTokenConsumer(state, operations)
	t.Cleanup(func() { close(operations) })
	app := fiber.New()
	app.Use(AuthMiddleware(state))
	SetupAuthRoutes(app.Group("/api"), state, operations)
	serviceToken.SetupTokenRoutes(app.Group("/api"), state, operations)

	response := accountSecurityRequest(t, app, http.MethodGet,
		"/api/auth/profile/retirement", nil, aliceSession)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var plan core.AccountRetirementPlan
	require.NoError(t, json.NewDecoder(response.Body).Decode(&plan))
	require.NoError(t, response.Body.Close())
	assert.True(t, plan.Eligible)
	response = accountSecurityRequest(t, app, http.MethodDelete,
		"/api/auth/profile/retirement", map[string]any{"confirmation": "wrong"}, aliceSession)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.Equal(t, "ACCOUNT_RETIREMENT_CONFIRMATION", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())
	response = accountSecurityRequest(t, app, http.MethodDelete,
		"/api/auth/profile/retirement", map[string]any{"confirmation": "alice"}, aliceSession)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	require.NoError(t, response.Body.Close())
	assert.Nil(t, state.GetSession(aliceSession))

	account, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Positive(t, account.DeletedAt)
	assert.Equal(t, uint64(1), state.Inner.TokensCount.Load())
	authenticated, err := AuthenticateUser(state, &core.LoginRequest{
		Name: "alice", Secret: "alice-password",
	}, operations)
	assert.Nil(t, authenticated)
	require.ErrorIs(t, err, core.ErrAccountDeleted)
	loginBody, err := proto.Marshal(&pb.LoginRequest{Name: "alice", Secret: "alice-password"})
	require.NoError(t, err)
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginRequest.Header.Set("X-Renop-Legal-Revision", state.Inner.Config.Load().Legal.Revision())
	loginRequest.Header.Set(fiber.HeaderContentType, protohttp.ContentType)
	response, err = app.Test(loginRequest)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, response.StatusCode)
	assert.Equal(t, "ACCOUNT_DELETED", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())
	response, err = app.Test(httptest.NewRequest(http.MethodGet, "/api/users/alice/profile", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var retiredProfile userProfileResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&retiredProfile))
	require.NoError(t, response.Body.Close())
	assert.Positive(t, retiredProfile.DeletedAt)

	response = accountSecurityRequest(t, app, http.MethodGet,
		"/api/tokens/alice/retention", nil, adminSession)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var status core.AccountRetirementStatus
	require.NoError(t, json.NewDecoder(response.Body).Decode(&status))
	require.NoError(t, response.Body.Close())
	assert.Positive(t, status.EmailReleaseAt)
	assert.Positive(t, status.AuditPurgeAt)
	response = accountSecurityRequest(t, app, http.MethodDelete,
		"/api/tokens/alice/retention/email", nil, adminSession)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = accountSecurityRequest(t, app, http.MethodDelete,
		"/api/tokens/alice/retention/audit", nil, adminSession)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	require.NoError(t, response.Body.Close())
	storedStatus, err := db.GetAccountRetirementStatus("alice")
	require.NoError(t, err)
	assert.Positive(t, storedStatus.EmailReleasedAt)
	assert.Positive(t, storedStatus.AuditPurgedAt)
}
