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
	"testing"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"

	"renop/config"
	"renop/core"
	"renop/token"
)

func TestFidoBeginEndpoints(t *testing.T) {
	state := core.NewAppState()
	cfg := config.DefaultConfig()
	state.Inner.Config.Store(&cfg)

	opChan := make(chan token.TokenOp, 100)
	go token.StartTokenConsumer(state, opChan)

	app := fiber.New()
	SetupAuthRoutes(app, state, opChan)

	t.Run("PostFidoLoginBegin", func(t *testing.T) {
		reqBody, _ := json.Marshal(map[string]string{"username": "admin"})
		req := httptest.NewRequest(http.MethodPost, "/auth/fido/login/begin", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var res map[string]any
		assert.NoError(t, json.NewDecoder(resp.Body).Decode(&res))
		assert.NotEmpty(t, res["session_id"])
		assert.NotNil(t, res["options"])
	})

	t.Run("GetProfileFidoDevices Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/auth/profile/fido", nil)
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("FidoDeviceStateOperations", func(t *testing.T) {
		dev := &core.FidoDevice{
			ID:           "dev-1",
			Username:     "user1",
			Name:         "My YubiKey",
			CredentialID: []byte("cred-123"),
			PublicKey:    []byte("pubkey-456"),
			CreatedAt:    1700000000000,
		}
		state.SaveFidoDevice(dev)

		devs := state.ListFidoDevices("user1")
		if assert.Len(t, devs, 1) {
			assert.Equal(t, "dev-1", devs[0].ID)
			assert.Equal(t, "My YubiKey", devs[0].Name)
		}

		matched := state.GetFidoDeviceByCredentialID([]byte("cred-123"))
		if assert.NotNil(t, matched) {
			assert.Equal(t, "user1", matched.Username)
		}

		deleted := state.DeleteFidoDevice("user1", "dev-1")
		assert.True(t, deleted)
		assert.Empty(t, state.ListFidoDevices("user1"))
	})
}

func TestGetWebAuthnEngine(t *testing.T) {
	state := core.NewAppState()
	cfg := config.DefaultConfig()
	cfg.Server.Domains = []string{"renop.example.com"}
	state.Inner.Config.Store(&cfg)

	testCases := []struct {
		name       string
		headers    map[string]string
		expectRpID string
	}{
		{
			name:       "Standard Localhost",
			headers:    map[string]string{"Host": "localhost:3000"},
			expectRpID: "localhost",
		},
		{
			name:       "Standard 127.0.0.1",
			headers:    map[string]string{"Host": "127.0.0.1:3000"},
			expectRpID: "127.0.0.1",
		},
		{
			name:       "IPv6 Host",
			headers:    map[string]string{"Host": "[::1]:8080"},
			expectRpID: "::1",
		},
		{
			name:       "0.0.0.0 Host Falls back to primary domain",
			headers:    map[string]string{"Host": "0.0.0.0:3000"},
			expectRpID: "renop.example.com",
		},
		{
			name:       "Origin Header Present",
			headers:    map[string]string{"Origin": "https://mvnc.pkg.one:8443", "Host": "127.0.0.1:3000"},
			expectRpID: "mvnc.pkg.one",
		},
		{
			name:       "X-Forwarded-Host Header",
			headers:    map[string]string{"X-Forwarded-Host": "proxy.example.com:443", "Host": "127.0.0.1:3000"},
			expectRpID: "proxy.example.com",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			var capturedRpID string
			app.Get("/test", func(c fiber.Ctx) error {
				w, err := getWebAuthnEngine(c, state)
				if err != nil {
					return c.Status(500).SendString(err.Error())
				}
				capturedRpID = w.Config.RPID
				return c.SendString("ok")
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			for k, v := range tc.headers {
				if k == "Host" {
					req.Host = v
				} else {
					req.Header.Set(k, v)
				}
			}
			resp, err := app.Test(req)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, tc.expectRpID, capturedRpID)
		})
	}
}
