/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package token

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func TestAutoRegisterAdminRejectsNilOperationChannel(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		done <- AutoRegisterAdmin(core.NewAppState(), nil)
	}()

	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("AutoRegisterAdmin blocked on a nil operation channel")
	}
}

func TestTokenConsumerPropagatesPersistenceErrors(t *testing.T) {
	db := newTestDB(t)
	state := core.NewAppState()
	state.Inner.DB = db
	tok := &core.AccessToken{Name: "persisted", Description: "original"}
	require.NoError(t, db.SaveToken(tok))
	state.Inner.TokensCount.Store(1)
	require.NoError(t, db.Close())

	opChan := make(chan TokenOp, 2)
	go StartTokenConsumer(state, opChan)

	err := UpdateTokenSync(opChan, "persisted", func(token *core.AccessToken) {
		token.Description = "changed"
	})
	require.Error(t, err)

	errChan := make(chan error, 1)
	opChan <- TokenOp{Type: OpTokenDelete, Name: "persisted", ErrChan: errChan}
	require.Error(t, <-errChan)
	assert.Equal(t, uint64(1), state.Inner.TokensCount.Load())
}

func TestDeletingTokenRevokesSessionsBeforeUsernameReuse(t *testing.T) {
	db := newTestDB(t)
	state := core.NewAppState()
	state.Inner.DB = db
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", Permissions: []string{"base"}}))
	state.Inner.TokensCount.Store(1)

	session := &core.Session{PublicId: "old-public", Username: "alice"}
	session.LastActive.Store(time.Now().UnixMilli())
	const sessionToken = "old-alice-session"
	require.NoError(t, state.SaveSession(session, sessionToken))

	opChan := make(chan TokenOp, 2)
	go StartTokenConsumer(state, opChan)
	deleteErr := make(chan error, 1)
	opChan <- TokenOp{Type: OpTokenDelete, Name: "alice", ErrChan: deleteErr}
	require.NoError(t, <-deleteErr)
	deletedSession, err := db.GetSession(sessionToken)
	require.NoError(t, err)
	require.Nil(t, deletedSession)
	require.Nil(t, state.GetSession(sessionToken))

	// Recreating the username must not make the old bearer session valid again.
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", Permissions: []string{"base"}}))

	app := fiber.New()
	app.Use(coreAuthMiddlewareForTest(state))
	app.Get("/protected", func(c fiber.Ctx) error {
		if c.Locals("user").(*config.User).Username == "guest" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(fiber.HeaderAuthorization, "Session "+sessionToken)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.NotEqual(t, fiber.StatusOK, resp.StatusCode)
}

func coreAuthMiddlewareForTest(state *core.AppState) fiber.Handler {
	return func(c fiber.Ctx) error {
		// Keep this test package independent of the auth package's route setup.
		if session := state.GetSession(strings.TrimPrefix(c.Get(fiber.HeaderAuthorization), "Session ")); session != nil {
			if token := state.GetTokenByName(session.Username); token != nil {
				c.Locals("user", &config.User{Username: token.Name, Roles: []string{"base"}})
				return c.Next()
			}
		}
		c.Locals("user", &config.User{Username: "guest"})
		return c.Next()
	}
}

func TestTokenConsumerRejectsInvalidOperationPayloads(t *testing.T) {
	db := newTestDB(t)
	state := core.NewAppState()
	state.Inner.DB = db
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "source"}))

	opChan := make(chan TokenOp, 3)
	go StartTokenConsumer(state, opChan)

	renameErr := make(chan error, 1)
	opChan <- TokenOp{Type: OpTokenRename, Name: "source", NewName: "target", ErrChan: renameErr}
	require.Error(t, <-renameErr)

	updateErr := make(chan error, 1)
	opChan <- TokenOp{Type: OpTokenUpdate, Name: "source", ErrChan: updateErr}
	require.Error(t, <-updateErr)

	require.NoError(t, UpdateTokenSync(opChan, "source", func(token *core.AccessToken) {
		token.Description = "consumer still running"
	}))
	updated, err := db.GetTokenByName("source")
	require.NoError(t, err)
	require.Equal(t, "consumer still running", updated.Description)
}

func newTestDB(t *testing.T) *database.DB {
	t.Helper()
	dbFile := t.TempDir() + "/test_tokens.db"
	cfg := config.DatabaseConfig{
		Driver:       "sqlite3",
		Dsn:          dbFile,
		MaxOpenConns: 5,
		MaxIdleConns: 5,
	}
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUpsertToken(t *testing.T) {
	db := newTestDB(t)

	state := core.NewAppState()
	state.Inner.DB = db
	opChan := make(chan TokenOp, 10)
	go StartTokenConsumer(state, opChan)

	app := fiber.New()

	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", &config.User{
			Username: "admin",
			Roles:    []string{"admin"},
		})
		return c.Next()
	})

	SetupTokenRoutes(app, state, opChan)

	initialNickname := "Initial Nickname"
	payload := core.CreateAccessTokenRequest{
		Permissions: []string{"base"},
		Nickname:    &initialNickname,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/tokens/instan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	val := state.GetTokenByName("instan")
	assert.NotNil(t, val)
	assert.Equal(t, "instan", val.Name)
	profile, err := db.GetUserProfile("instan")
	require.NoError(t, err)
	assert.Equal(t, initialNickname, profile.Nickname)
	stableUserID := profile.UserID
	now := time.Now().UnixMilli()
	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{
		Repository: "cargo", Name: "admin-rename", NormalizedName: "admin-rename", CreatedAt: now, UpdatedAt: now,
	}, &core.CargoVersion{
		Repository: "cargo", Package: "admin-rename", Version: "1.0.0", Publisher: "instan", CreatedAt: now,
	}, "instan"))

	updatedNickname := "Updated Nickname"
	payload2 := core.CreateAccessTokenRequest{
		Permissions: []string{"base", "showing"},
		Nickname:    &updatedNickname,
	}
	body2, _ := json.Marshal(payload2)
	req2 := httptest.NewRequest(http.MethodPut, "/tokens/instan", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := app.Test(req2)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	allTokens := state.GetAllTokens()
	assert.Equal(t, 1, len(allTokens))
	profile, err = db.GetUserProfile("instan")
	require.NoError(t, err)
	assert.Equal(t, updatedNickname, profile.Nickname)

	newName := "instan2"
	payload3 := core.CreateAccessTokenRequest{
		NewName:     &newName,
		Permissions: []string{"base", "showing"},
	}
	body3, _ := json.Marshal(payload3)
	req3 := httptest.NewRequest(http.MethodPut, "/tokens/instan", bytes.NewReader(body3))
	req3.Header.Set("Content-Type", "application/json")

	resp3, err := app.Test(req3)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp3.StatusCode)

	allTokens = state.GetAllTokens()
	assert.Equal(t, 1, len(allTokens))
	assert.Nil(t, state.GetTokenByName("instan"))
	assert.NotNil(t, state.GetTokenByName("instan2"))
	renamedProfile, err := db.GetUserProfile("instan2")
	require.NoError(t, err)
	assert.Equal(t, stableUserID, renamedProfile.UserID)
	packageDetails, err := db.GetCargoPackageDetails("cargo", "admin-rename", "instan2")
	require.NoError(t, err)
	assert.Equal(t, core.CargoPermissionOwner, packageDetails.Package.PermissionLevel)

	payloadCreateDup := core.CreateAccessTokenRequest{
		Permissions: []string{"base"},
		IsCreate:    true,
	}
	bodyDup, _ := json.Marshal(payloadCreateDup)
	reqDup := httptest.NewRequest(http.MethodPut, "/tokens/instan2", bytes.NewReader(bodyDup))
	reqDup.Header.Set("Content-Type", "application/json")

	respDup, err := app.Test(reqDup)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, respDup.StatusCode)

	renameUser := func(oldName, newName string) int {
		t.Helper()
		payload := core.CreateAccessTokenRequest{NewName: &newName, Permissions: []string{"base"}}
		body, marshalErr := json.Marshal(payload)
		require.NoError(t, marshalErr)
		request := httptest.NewRequest(http.MethodPut, "/tokens/"+oldName, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response, requestErr := app.Test(request)
		require.NoError(t, requestErr)
		defer response.Body.Close()
		return response.StatusCode
	}
	assert.Equal(t, http.StatusOK, renameUser("instan2", "instan3"))
	assert.Equal(t, http.StatusTooManyRequests, renameUser("instan3", "instan4"))
	assert.NotNil(t, state.GetTokenByName("instan3"))
	assert.Nil(t, state.GetTokenByName("instan4"))
}

func TestFindAllTokensWithDB(t *testing.T) {
	db := newTestDB(t)

	state := core.NewAppState()
	state.Inner.DB = db

	tok1 := &core.AccessToken{
		Identifier:      core.AccessTokenIdentifier{Type: core.Persistent, Value: 1},
		Name:            "user1",
		EncryptedSecret: "sec1",
		CreatedAt:       "2026-08-01T00:00:00Z",
		Description:     "User 1",
		Permissions:     []string{"base"},
	}
	tok2 := &core.AccessToken{
		Identifier:      core.AccessTokenIdentifier{Type: core.Persistent, Value: 2},
		Name:            "user2",
		EncryptedSecret: "sec2",
		CreatedAt:       "2026-08-01T00:00:00Z",
		Description:     "User 2",
		Permissions:     []string{"admin"},
	}
	assert.NoError(t, db.SaveToken(tok1))
	assert.NoError(t, db.SaveToken(tok2))

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", &config.User{
			Username: "admin",
			Roles:    []string{"admin"},
		})
		return c.Next()
	})

	opChan := make(chan TokenOp, 10)
	SetupTokenRoutes(app, state, opChan)

	req := httptest.NewRequest(http.MethodGet, "/tokens", nil)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	tokens := state.GetAllTokens()
	assert.Len(t, tokens, 2)
}
