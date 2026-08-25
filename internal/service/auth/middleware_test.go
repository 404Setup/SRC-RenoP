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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/token"
)

func newTestAuthDB(t *testing.T) *database.DB {
	t.Helper()
	dbFile := t.TempDir() + "/auth_test.db"
	cfg := config.DatabaseConfig{
		Driver:       "sqlite3",
		Dsn:          dbFile,
		MaxOpenConns: 5,
		MaxIdleConns: 5,
	}
	db, err := database.InitDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func storeTestToken(t *testing.T, db *database.DB, state *core.AppState, tok *core.AccessToken) {
	t.Helper()
	require.NoError(t, db.SaveToken(tok))
}

func TestValidateAndRenewSessionPersistsLastActive(t *testing.T) {
	db := newTestAuthDB(t)
	state := core.NewAppState()
	state.Inner.DB = db

	const sessionToken = "renewed-session"
	staleLastActive := time.Now().UnixMilli() - core.SessionRenewalIntervalMillis - 1
	session := &core.Session{PublicID: "renewed-public", Username: "admin"}
	session.LastActive.Store(staleLastActive)
	require.NoError(t, state.SaveSession(session, sessionToken))

	require.Equal(t, "admin", ValidateAndRenewSession(state, sessionToken))
	updatedLastActive := session.LastActive.Load()
	require.Greater(t, updatedLastActive, staleLastActive)

	var persistedLastActive int64
	require.NoError(t, db.SQLDB.QueryRow(
		`SELECT last_active FROM sessions WHERE session_token = ?`, sessionToken,
	).Scan(&persistedLastActive))
	assert.Equal(t, updatedLastActive, persistedLastActive)
}

func TestExtractAuthHeader_WithOtherCookie(t *testing.T) {
	app := fiber.New()
	state := core.NewAppState()

	session := &core.Session{
		Username: "admin",
	}
	session.LastActive.Store(time.Now().UnixMilli())
	state.Inner.Sessions.Store("valid_session", session)

	app.Get("/test", func(c fiber.Ctx) error {
		header := extractAuthHeader(c, state)
		return c.SendString(header)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Cookie", "renop_session=valid_session")
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	buf := make([]byte, 100)
	n, _ := resp.Body.Read(buf)
	assert.Equal(t, "Session valid_session", string(buf[:n]))

	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Cookie", "renop_session=invalid_session")
	resp, err = app.Test(req)
	assert.NoError(t, err)
	buf = make([]byte, 100)
	n, _ = resp.Body.Read(buf)
	assert.Equal(t, "", string(buf[:n]))

	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Cookie", "foo=bar")
	resp, err = app.Test(req)
	assert.NoError(t, err)
	buf = make([]byte, 100)
	n, _ = resp.Body.Read(buf)
	assert.Equal(t, "", string(buf[:n]))

	req = httptest.NewRequest("GET", "/test?token=valid_session", nil)
	req.Header.Set("Cookie", "renop_session=invalid_session")
	resp, err = app.Test(req)
	assert.NoError(t, err)
	buf = make([]byte, 100)
	n, _ = resp.Body.Read(buf)
	assert.Equal(t, "Session valid_session", string(buf[:n]))

	app.Post("/test", func(c fiber.Ctx) error {
		return c.SendString(extractAuthHeader(c, state))
	})
	req = httptest.NewRequest("POST", "/test?token=valid_session", nil)
	resp, err = app.Test(req)
	assert.NoError(t, err)
	buf = make([]byte, 100)
	n, _ = resp.Body.Read(buf)
	assert.Equal(t, "", string(buf[:n]))
}

func TestAuthMiddlewareAcceptsOpaqueRegistryToken(t *testing.T) {
	db := newTestAuthDB(t)
	state := core.NewAppState()
	state.Inner.DB = db
	state.Inner.Config.Store(&config.Config{Maven: config.MavenSettings{Repositories: map[string]*config.Repository{
		"cargo": {Name: "cargo", Format: config.RepositoryFormatCargo},
	}}})
	storeTestToken(t, db, state, &core.AccessToken{
		Name:        "cargo-publisher",
		Tokens:      []string{"opaque-cargo-secret"},
		Permissions: []string{"canupdate:cargo"},
	})

	app := fiber.New()
	app.Use(AuthMiddleware(state))
	app.Get("/cargo/config.json", func(c fiber.Ctx) error {
		return c.SendString(GetUser(c).Username)
	})
	app.Get("/ordinary", func(c fiber.Ctx) error {
		return c.SendString(GetUser(c).Username)
	})
	req := httptest.NewRequest(http.MethodGet, "/cargo/config.json", nil)
	req.Header.Set(fiber.HeaderAuthorization, "opaque-cargo-secret")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body := make([]byte, 64)
	n, err := resp.Body.Read(body)
	if err != nil && err.Error() != "EOF" {
		require.NoError(t, err)
	}
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, "cargo-publisher", string(body[:n]))

	// Priming Cargo authentication must not make its opaque authorization
	// value valid on unrelated endpoints through the shared auth cache.
	ordinaryRequest := httptest.NewRequest(http.MethodGet, "/ordinary", nil)
	ordinaryRequest.Header.Set(fiber.HeaderAuthorization, "opaque-cargo-secret")
	ordinaryResponse, err := app.Test(ordinaryRequest)
	require.NoError(t, err)
	defer ordinaryResponse.Body.Close()
	assert.Equal(t, fiber.StatusUnauthorized, ordinaryResponse.StatusCode)

	invalidRequest := httptest.NewRequest(http.MethodGet, "/cargo/config.json", nil)
	invalidRequest.Header.Set(fiber.HeaderAuthorization, "invalid-cargo-secret")
	invalidResponse, err := app.Test(invalidRequest)
	require.NoError(t, err)
	defer invalidResponse.Body.Close()
	assert.Equal(t, fiber.StatusForbidden, invalidResponse.StatusCode)
	assert.Contains(t, invalidResponse.Header.Get(fiber.HeaderContentType), fiber.MIMEApplicationJSON)
}

func TestPostAuthLogout(t *testing.T) {
	db := newTestAuthDB(t)
	app := fiber.New()
	state := core.NewAppState()
	state.Inner.DB = db

	storeTestToken(t, db, state, &core.AccessToken{
		Name:        "admin",
		Permissions: []string{"admin"},
	})

	sessionToken := "test-session-token"
	session := &core.Session{
		PublicID: "test-public-id",
		Username: "admin",
	}
	session.LastActive.Store(time.Now().UnixMilli())
	state.SaveSession(session, sessionToken)

	app.Use(AuthMiddleware(state))
	app.Post("/logout", func(c fiber.Ctx) error {
		return PostAuthLogout(c, state)
	})
	app.Post("/api/auth/logout", func(c fiber.Ctx) error {
		return PostAuthLogout(c, state)
	})

	req := httptest.NewRequest("POST", "/logout", nil)
	req.Header.Set("Cookie", "renop_session=test-session-token")
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 204, resp.StatusCode)

	_, ok := state.Inner.Sessions.Load(sessionToken)
	assert.False(t, ok)
	// Verify session is also removed from DB
	dbSess, _ := db.GetSession(sessionToken)
	assert.Nil(t, dbSess)
}

func TestSessionPersistenceErrorsDoNotMutateMemory(t *testing.T) {
	db := newTestAuthDB(t)
	state := core.NewAppState()
	state.Inner.DB = db
	session := &core.Session{PublicID: "failed", Username: "admin"}
	session.LastActive.Store(time.Now().UnixMilli())

	require.NoError(t, db.Close())
	require.Error(t, state.SaveSession(session, "failed-save"))
	_, ok := state.Inner.Sessions.Load("failed-save")
	assert.False(t, ok)

	state.Inner.Sessions.Store("failed-revoke", session)
	revoked, err := state.RevokeSession("failed-revoke")
	require.Error(t, err)
	assert.False(t, revoked)
	_, ok = state.Inner.Sessions.Load("failed-revoke")
	assert.True(t, ok)
}

func TestPostAuthLogoutRevokesCookieEvenWithoutUser(t *testing.T) {
	app := fiber.New()
	state := core.NewAppState()

	sessionToken := "orphan-session"
	session := &core.Session{
		PublicID: "orphan-public",
		Username: "deleted-user",
	}
	session.LastActive.Store(time.Now().UnixMilli())
	state.Inner.Sessions.Store(sessionToken, session)

	app.Use(AuthMiddleware(state))
	app.Post("/api/auth/logout", func(c fiber.Ctx) error {
		return PostAuthLogout(c, state)
	})

	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	req.Header.Set("Cookie", "renop_session="+sessionToken)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 204, resp.StatusCode)

	_, ok := state.Inner.Sessions.Load(sessionToken)
	assert.False(t, ok, "logout must revoke session even when user no longer exists")
}

func TestPostAuthLogoutRevokesAuthorizationSessionHeader(t *testing.T) {
	db := newTestAuthDB(t)
	app := fiber.New()
	state := core.NewAppState()
	state.Inner.DB = db

	storeTestToken(t, db, state, &core.AccessToken{
		Name:        "admin",
		Permissions: []string{"admin"},
	})

	sessionToken := "header-session"
	session := &core.Session{PublicID: "hdr", Username: "admin"}
	session.LastActive.Store(time.Now().UnixMilli())
	state.SaveSession(session, sessionToken)

	app.Use(AuthMiddleware(state))
	app.Post("/api/auth/logout", func(c fiber.Ctx) error {
		return PostAuthLogout(c, state)
	})

	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	req.Header.Set("Authorization", "Session "+sessionToken)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 204, resp.StatusCode)

	_, ok := state.Inner.Sessions.Load(sessionToken)
	assert.False(t, ok)
}

func TestRevokedSessionCannotAuthenticate(t *testing.T) {
	db := newTestAuthDB(t)
	state := core.NewAppState()
	state.Inner.DB = db

	storeTestToken(t, db, state, &core.AccessToken{
		Name:        "admin",
		Permissions: []string{"admin"},
	})
	session := &core.Session{Username: "admin"}
	session.LastActive.Store(time.Now().UnixMilli())
	state.SaveSession(session, "live-token")

	app := fiber.New()
	app.Use(AuthMiddleware(state))
	app.Post("/api/auth/logout", func(c fiber.Ctx) error {
		return PostAuthLogout(c, state)
	})
	app.Get("/api/auth/me", func(c fiber.Ctx) error {
		return GetAuthMe(c)
	})

	logoutReq := httptest.NewRequest("POST", "/api/auth/logout", nil)
	logoutReq.Header.Set("Cookie", "renop_session=live-token")
	logoutResp, err := app.Test(logoutReq)
	assert.NoError(t, err)
	assert.Equal(t, 204, logoutResp.StatusCode)
	_ = logoutResp.Body.Close()

	meReq := httptest.NewRequest("GET", "/api/auth/me", nil)
	meReq.Header.Set("Cookie", "renop_session=live-token")
	meResp, err := app.Test(meReq)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, meResp.StatusCode)
	_ = meResp.Body.Close()
}

func TestAuthorizeRequestProtectsAllRestrictedPrefixes(t *testing.T) {
	app := fiber.New()
	guard := func(c fiber.Ctx) error { return authorizeRequest(c, GuestUser) }
	app.Get("/api/settings/config", guard)
	app.Get("/api/tokens", guard)
	app.Get("/api/statistics", guard)
	app.Get("/api/status/instance", guard)

	for _, path := range []string{"/api/settings/config", "/api/tokens", "/api/statistics", "/api/status/instance"} {
		resp, err := app.Test(httptest.NewRequest("GET", path, nil))
		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode, path)
		_ = resp.Body.Close()
	}
}

func TestSessionRevocationIsObservedImmediately(t *testing.T) {
	db := newTestAuthDB(t)
	state := core.NewAppState()
	state.Inner.DB = db

	storeTestToken(t, db, state, &core.AccessToken{
		Name:        "admin",
		Permissions: []string{"admin"},
	})
	session := &core.Session{Username: "admin"}
	session.LastActive.Store(time.Now().UnixMilli())
	state.SaveSession(session, "revocable")

	app := fiber.New()
	app.Use(AuthMiddleware(state))
	app.Get("/protected", func(c fiber.Ctx) error {
		if GetUser(c).Username == "guest" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		return c.SendStatus(fiber.StatusOK)
	})

	request := func() int {
		req := httptest.NewRequest("GET", "/protected", nil)
		req.Header.Set("Authorization", "Session revocable")
		resp, err := app.Test(req)
		assert.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}

	assert.Equal(t, fiber.StatusOK, request())
	state.RevokeSession("revocable")
	assert.Equal(t, fiber.StatusUnauthorized, request())
}

func TestExpiredTokenIsNotAcceptedFromAuthCache(t *testing.T) {
	db := newTestAuthDB(t)
	state := core.NewAppState()
	state.Inner.DB = db

	expiresAt := time.Now().Add(40 * time.Millisecond).UnixMilli()
	storeTestToken(t, db, state, &core.AccessToken{
		Name:        "expiring",
		Tokens:      []string{"secret"},
		Permissions: []string{"canview:releases"},
		ExpiresAt:   &expiresAt,
	})

	app := fiber.New()
	app.Use(AuthMiddleware(state))
	app.Get("/protected", func(c fiber.Ctx) error {
		if GetUser(c).Username == "guest" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		return c.SendStatus(fiber.StatusOK)
	})

	request := func() int {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set(fiber.HeaderAuthorization, "Bearer expiring:secret")
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}

	require.Equal(t, fiber.StatusOK, request())
	time.Sleep(60 * time.Millisecond)
	assert.NotEqual(t, fiber.StatusOK, request(), "expired token must not remain authorized in the positive auth cache")
}

func TestExpiredTokenSessionIsRejected(t *testing.T) {
	db := newTestAuthDB(t)
	state := core.NewAppState()
	state.Inner.DB = db

	expiresAt := time.Now().Add(40 * time.Millisecond).UnixMilli()
	storeTestToken(t, db, state, &core.AccessToken{
		Name:        "session-expiring",
		Permissions: []string{"canview:releases"},
		ExpiresAt:   &expiresAt,
	})
	session := &core.Session{Username: "session-expiring"}
	session.LastActive.Store(time.Now().UnixMilli())
	require.NoError(t, state.SaveSession(session, "expiring-session"))

	app := fiber.New()
	app.Use(AuthMiddleware(state))
	app.Get("/protected", func(c fiber.Ctx) error {
		if GetUser(c).Username == "guest" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		return c.SendStatus(fiber.StatusOK)
	})

	request := func() int {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set(fiber.HeaderAuthorization, "Session expiring-session")
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}

	require.Equal(t, fiber.StatusOK, request())
	time.Sleep(60 * time.Millisecond)
	assert.NotEqual(t, fiber.StatusOK, request(), "expired account sessions must not remain authorized")
}

func TestDeleteTokenRejectsAuthenticatedAccount(t *testing.T) {
	db := newTestAuthDB(t)
	state := core.NewAppState()
	state.Inner.DB = db
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name:        "admin",
		Permissions: []string{"admin"},
	}))
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name:        "other-admin",
		Permissions: []string{"admin"},
	}))
	state.Inner.TokensCount.Store(2)

	const sessionToken = "self-delete-session"
	session := &core.Session{PublicID: "self-delete-public", Username: "admin"}
	session.LastActive.Store(time.Now().UnixMilli())
	require.NoError(t, state.SaveSession(session, sessionToken))

	opChan := make(chan token.TokenOp, 2)
	go token.StartTokenConsumer(state, opChan)
	app := fiber.New()
	app.Use(AuthMiddleware(state))
	token.SetupTokenRoutes(app.Group("/api"), state, opChan)

	selfDelete := httptest.NewRequest(http.MethodDelete, "/api/tokens/AdMiN", nil)
	selfDelete.Header.Set(fiber.HeaderAuthorization, "Session "+sessionToken)
	selfResponse, err := app.Test(selfDelete)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusForbidden, selfResponse.StatusCode)
	require.NoError(t, selfResponse.Body.Close())
	assert.NotNil(t, state.GetTokenByName("admin"))

	otherDelete := httptest.NewRequest(http.MethodDelete, "/api/tokens/other-admin", nil)
	otherDelete.Header.Set(fiber.HeaderAuthorization, "Session "+sessionToken)
	otherResponse, err := app.Test(otherDelete)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusNoContent, otherResponse.StatusCode)
	require.NoError(t, otherResponse.Body.Close())
	assert.Nil(t, state.GetTokenByName("other-admin"))
}
