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
	"os"
	"testing"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"

	"renop/config"
	"renop/core"
)

func TestUpsertToken(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "tokens_test_*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	os.Setenv("RENOP_TOKENS", tmpFile.Name())

	state := core.NewAppState()
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

	val, ok := state.Inner.TokenRepository.Load("instan")
	assert.True(t, ok)
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

	count := 0
	state.Inner.TokenRepository.Range(func(key string, value *core.AccessToken) bool {
		count++
		return true
	})
	assert.Equal(t, 1, count)

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

	count = 0
	var foundInstan, foundInstan2 bool
	state.Inner.TokenRepository.Range(func(key string, value *core.AccessToken) bool {
		count++
		if key == "instan" {
			foundInstan = true
		}
		if key == "instan2" {
			foundInstan2 = true
		}
		return true
	})
	assert.Equal(t, 1, count)
	assert.False(t, foundInstan)
	assert.True(t, foundInstan2)

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
