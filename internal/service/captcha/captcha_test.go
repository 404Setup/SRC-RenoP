/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package captcha

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProviderValidationContracts(t *testing.T) {
	for _, provider := range []string{"recaptcha_v2", "recaptcha_invisible", "recaptcha_v3", "turnstile", "hcaptcha", "friendlycaptcha"} {
		t.Run(provider, func(t *testing.T) {
			cfg := config.DefaultCaptchaConfig()
			cfg.Provider, cfg.SiteKey, cfg.SecretKey = provider, "site", "private-secret"
			require.NoError(t, cfg.Normalize())
			payload := `{"success":true,"hostname":"example.test","action":"renop_password_login","score":0.8,"data":{"challenge":{"origin":"https://example.test"}}}`
			status := 200
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, "POST", r.Method)
				require.Empty(t, r.URL.RawQuery)
				require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
				require.NoError(t, r.ParseForm())
				require.Equal(t, "solved", r.Form.Get("response"))
				if provider == "friendlycaptcha" {
					require.Equal(t, "global.frcapi.com", r.URL.Host)
					require.Equal(t, "private-secret", r.Header.Get("X-API-Key"))
					require.Empty(t, r.Form.Get("secret"))
				} else {
					require.Equal(t, "private-secret", r.Form.Get("secret"))
				}
				if provider == "friendlycaptcha" || provider == "hcaptcha" {
					require.Equal(t, "site", r.Form.Get("sitekey"))
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(payload))}, nil
			})}
			verify := func() error {
				return verifyToken(context.Background(), client, cfg, []string{"example.test"}, config.CaptchaPasswordLogin, "solved", "192.0.2.1")
			}
			require.NoError(t, verify())
			payload = `{"success":false}`
			require.ErrorIs(t, verify(), errInvalid)
			payload = `{"success":true,"hostname":"evil.test","action":"other","score":0.8,"data":{"challenge":{"origin":"https://evil.test"}}}`
			if provider == "hcaptcha" {
				require.NoError(t, verify())
			} else {
				require.ErrorIs(t, verify(), errInvalid)
			}
			if provider == "recaptcha_v3" || provider == "turnstile" {
				payload = `{"success":true,"hostname":"example.test","action":"other","score":0.8}`
				require.ErrorIs(t, verify(), errInvalid)
			}
			if provider == "recaptcha_v3" {
				for _, score := range []string{"0.1", "null", "1.1"} {
					payload = `{"success":true,"hostname":"example.test","action":"renop_password_login","score":` + score + `}`
					require.ErrorIs(t, verify(), errInvalid)
				}
			}
			payload = "not JSON"
			require.ErrorIs(t, verify(), errUnavailable)
			payload = strings.Repeat("x", (64<<10)+1)
			require.ErrorIs(t, verify(), errUnavailable)
			status = 503
			require.ErrorIs(t, verify(), errUnavailable)
		})
	}
}

func TestBrowserApprovalsAreBoundSingleUseAndAutomationExempt(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Domains = []string{"example.test"}
	cfg.Captcha.Provider, cfg.Captcha.SiteKey, cfg.Captcha.SecretKey = "turnstile", "site", "private-secret"
	cfg.Captcha.Scopes = config.CaptchaScopes{ManualMail: true, Registration: true}
	require.NoError(t, cfg.Captcha.Normalize())
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	client := &http.Client{}
	client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"hostname":"example.test","action":"renop_manual_mail"}`))}, nil
	})
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("auth_credential_kind", c.Get("X-Test-Kind", "session"))
		return c.Next()
	})
	SetupRoutes(app.Group("/api"), state)
	app.Post("/verify", func(c fiber.Ctx) error { return verifyBrowser(c, state, client) })
	mutations := 0
	app.Post("/protected", func(c fiber.Ctx) error {
		if err := Require(c, state, config.CaptchaManualMail, config.CaptchaRegistration); err != nil {
			return err
		}
		mutations++
		return c.SendStatus(204)
	})
	meta, err := app.Test(httptest.NewRequest("GET", "/api/captcha", nil))
	require.NoError(t, err)
	data, err := io.ReadAll(meta.Body)
	require.NoError(t, err)
	require.NotContains(t, string(data), "private-secret")
	require.Len(t, meta.Cookies(), 1)
	nonce := meta.Cookies()[0]
	require.True(t, nonce.HttpOnly)
	require.NoError(t, meta.Body.Close())
	request := func(path, body, proof, kind string, cookie *http.Cookie) *http.Response {
		t.Helper()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(ProofHeader, proof)
		if kind != "" {
			req.Header.Set("X-Test-Kind", kind)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		response, err := app.Test(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = response.Body.Close() })
		return response
	}
	denied := request("/protected", "", "", "", nonce)
	require.Equal(t, 428, denied.StatusCode)
	require.Equal(t, config.CaptchaManualMail, denied.Header.Get(ScopeHeader))
	require.Zero(t, mutations)
	approval := func() string {
		response := request("/verify", `{"scope":"manual_mail","provider":"turnstile","site_key":"site","response":"solved"}`, "", "", nonce)
		require.Equal(t, 200, response.StatusCode)
		var value map[string]any
		require.NoError(t, json.NewDecoder(response.Body).Decode(&value))
		return value["proof"].(string)
	}
	proof := approval()
	require.Equal(t, 204, request("/protected", "", proof, "", nonce).StatusCode)
	require.Equal(t, 400, request("/protected", "", proof, "", nonce).StatusCode)
	require.Equal(t, 400, request("/protected", "", approval(), "", nil).StatusCode)
	proof = approval()
	changed := cfg.DeepCopy()
	changed.Captcha.SecretKey = "rotated-secret"
	require.NoError(t, changed.Captcha.Normalize())
	state.Inner.Config.Store(changed)
	require.Equal(t, 400, request("/protected", "", proof, "", nonce).StatusCode)
	for _, kind := range []string{"api_token", "password"} {
		require.Equal(t, 204, request("/protected", "", "", kind, nil).StatusCode)
	}
	require.Equal(t, 3, mutations)
}
