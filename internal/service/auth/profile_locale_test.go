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
	"fmt"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/testutil"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

func TestAccountLocaleRequiresItsBrowserSession(t *testing.T) {
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "locale-routes.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state := core.NewAppState()
	state.Inner.DB = db
	state.Inner.Config.Store(config.DefaultConfig())
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", EncryptedSecret: "password", Permissions: []string{"base"}, Tokens: []string{"locale-api-token"}}))
	profile, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	payload := func(language string) string {
		return fmt.Sprintf(`{"user_id":%q,"locale":%q}`, profile.UserID, language)
	}
	result := func(language string) string {
		return fmt.Sprintf(`{"user_id":%q,"username":"alice","locale":%q}`, profile.UserID, language)
	}
	now := time.Now().UnixMilli()
	session := &core.Session{PublicID: "locale", Username: "alice", CreatedAt: now, LoginMethod: "password"}
	session.LastActive.Store(now)
	require.NoError(t, state.SaveSession(session, "locale-session"))
	app := fiber.New()
	app.Use(AuthMiddleware(state))
	SetupAuthRoutes(app.Group("/api"), state, nil)
	request := func(method, path, body, cookie, authorization string, status int) string {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookie)
		req.Header.Set("Authorization", authorization)
		response, err := app.Test(req)
		require.NoError(t, err)
		data, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, status, response.StatusCode, string(data))
		if status == 200 && strings.HasSuffix(path, "/locale") {
			require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
		}
		return string(data)
	}
	const endpoint = "/api/auth/profile/locale"
	const cookie = sessionCookieName + "=locale-session"
	request("GET", endpoint, "", "", "", 401)
	request("PUT", endpoint, payload("fr-FR"), "", "Session locale-session", 401)
	request("PUT", endpoint, payload("fr-FR"), "", "Bearer locale-api-token", 403)
	require.JSONEq(t, result(""), request("GET", endpoint, "", cookie, "", 200))
	request("PUT", endpoint, payload("invalid/value"), cookie, "", 400)
	request("PUT", endpoint, `{"user_id":"another-account","locale":"fr-FR"}`, cookie, "", 403)
	require.JSONEq(t, result("ja-JP"), request("PUT", endpoint, payload("ja-JP"), cookie, "", 200))
	require.JSONEq(t, result("ja-JP"), request("GET", endpoint, "", cookie, "", 200))
	public := request("GET", "/api/users/alice/profile", "", "", "", 200)
	require.NotContains(t, public, "locale")
	_, err = state.RevokeSession("locale-session")
	require.NoError(t, err)
	request("PUT", endpoint, payload("fr-FR"), cookie, "", 401)
}
