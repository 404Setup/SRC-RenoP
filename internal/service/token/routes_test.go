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
	"testing"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

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

	payload := core.CreateAccessTokenRequest{
		Permissions: []string{"base"},
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

	payload2 := core.CreateAccessTokenRequest{
		Permissions: []string{"base", "showing"},
	}
	body2, _ := json.Marshal(payload2)
	req2 := httptest.NewRequest(http.MethodPut, "/tokens/instan", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := app.Test(req2)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	allTokens := state.GetAllTokens()
	assert.Equal(t, 1, len(allTokens))

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
