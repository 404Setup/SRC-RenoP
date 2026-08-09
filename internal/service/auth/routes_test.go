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
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/proto"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/token"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func TestPostAuthLogin(t *testing.T) {
	dbFile := t.TempDir() + "/auth_routes_test.db"
	dbCfg := config.DatabaseConfig{
		Driver:       "sqlite3",
		Dsn:          dbFile,
		MaxOpenConns: 5,
		MaxIdleConns: 5,
	}
	db, err := database.InitDB(dbCfg)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	state := core.NewAppState()
	state.Inner.DB = db
	cfg := config.DefaultConfig()
	state.Inner.Config.Store(cfg)

	const secret = "test-admin-password"
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.MinCost)
	assert.NoError(t, err)
	admin := &core.AccessToken{
		Name:            "admin",
		EncryptedSecret: string(hash),
		Permissions:     []string{"admin"},
	}
	require.NoError(t, db.SaveToken(admin))
	state.Inner.TokensCount.Store(1)

	opChan := make(chan token.TokenOp, 100)
	go token.StartTokenConsumer(state, opChan)

	app := fiber.New()
	app.Post("/login", func(c fiber.Ctx) error {
		return PostAuthLogin(c, state, opChan)
	})

	bodyBytes, err := proto.Marshal(&pb.LoginRequest{
		Name:   "admin",
		Secret: secret,
	})
	assert.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", protohttp.ContentType)

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	cookies := resp.Cookies()
	if assert.Len(t, cookies, 1) {
		assert.Equal(t, sessionCookieName, cookies[0].Name)
		assert.True(t, cookies[0].HttpOnly)
		assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
		assert.Equal(t, int(core.SessionIdleTimeoutMillis/1000), cookies[0].MaxAge)
		assert.NotEmpty(t, cookies[0].Value)
		_, ok := state.Inner.Sessions.Load(cookies[0].Value)
		assert.True(t, ok)
	}

	raw, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	_ = resp.Body.Close()
	var details pb.SessionDetails
	assert.NoError(t, proto.Unmarshal(raw, &details))
	assert.Empty(t, details.GetSessionToken(), "login must not return raw session secret in body")
}

func TestCreateSessionDetailsWriteUser(t *testing.T) {
	token := &core.AccessToken{
		Name:        "writer",
		Permissions: []string{"canupdate:releases", "canview:private"},
	}
	user := buildSynthUser(token)
	assert.True(t, user.CheckUpdatePermission("releases"))
	assert.False(t, user.CheckUpdatePermission("snapshots"))
	assert.False(t, user.IsManager())

	details := CreateSessionDetails(user, "sess-1")
	assert.Equal(t, "writer", details.AccessToken.Name)

	roleIDs := make([]string, 0, len(details.Permissions))
	for _, p := range details.Permissions {
		roleIDs = append(roleIDs, p.Identifier)
	}
	assert.Contains(t, roleIDs, "canupdate:releases")
	assert.Contains(t, roleIDs, "canview:private")
	assert.NotContains(t, roleIDs, "access-token:manager")

	var hasWriteReleases, hasReadPrivate bool
	for _, r := range details.Routes {
		if r.Permission.Identifier == "route:write" && r.Path == "releases" {
			hasWriteReleases = true
		}
		if r.Permission.Identifier == "route:read" && r.Path == "private" {
			hasReadPrivate = true
		}
	}
	assert.True(t, hasWriteReleases, "session should expose route:write for canupdate:releases")
	assert.True(t, hasReadPrivate, "session should expose route:read for canview:private")
}

func TestCreateSessionDetailsManagerHasFullWriteRoute(t *testing.T) {
	token := &core.AccessToken{
		Name:        "admin",
		Permissions: []string{"admin"},
	}
	user := buildSynthUser(token)
	assert.True(t, user.IsManager())
	assert.True(t, user.CheckUpdatePermission("any-repo"))

	details := CreateSessionDetails(user, "")
	roleIDs := make([]string, 0, len(details.Permissions))
	for _, p := range details.Permissions {
		roleIDs = append(roleIDs, p.Identifier)
	}
	assert.Contains(t, roleIDs, "access-token:manager")

	hasFullWrite := false
	for _, r := range details.Routes {
		if r.Permission.Identifier == "route:write" && r.Path == "*" {
			hasFullWrite = true
			break
		}
	}
	assert.True(t, hasFullWrite, "manager session should expose route:write for *")
}

func TestBuildSynthUserPermissionMapping(t *testing.T) {
	user := buildSynthUser(&core.AccessToken{
		Name:        "u",
		Permissions: []string{"canupdate:*", "canview:releases", "m"},
	})
	assert.True(t, user.IsManager())
	assert.True(t, user.CheckUpdatePermission("anything"))
	assert.True(t, user.CheckReadPermission("releases", "x", "PRIVATE", false))
}
