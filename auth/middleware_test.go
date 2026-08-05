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
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/config"
	"renop/core"
	"renop/database"
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
		PublicId: "test-public-id",
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

func TestPostAuthLogoutRevokesCookieEvenWithoutUser(t *testing.T) {
	app := fiber.New()
	state := core.NewAppState()

	sessionToken := "orphan-session"
	session := &core.Session{
		PublicId: "orphan-public",
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
	session := &core.Session{PublicId: "hdr", Username: "admin"}
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
