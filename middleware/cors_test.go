/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"

	"renop/config"
	"renop/core"
)

func setupCorsApp(t *testing.T, sc config.ServerConfig) *fiber.App {
	t.Helper()
	state := core.NewAppState()
	cfg := config.DefaultConfig()
	cfg.Server = sc
	state.Inner.Config.Store(&cfg)

	app := fiber.New()
	app.Use(CorsMiddleware(state))
	app.Get("/api/ping", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})
	return app
}

func TestCorsAllowsConfiguredDomain(t *testing.T) {
	sc := config.DefaultServerConfig()
	sc.Domains = []string{"mvnc.pkg.one"}
	app := setupCorsApp(t, sc)

	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.Header.Set("Origin", "https://mvnc.pkg.one")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://mvnc.pkg.one" {
		t.Fatalf("ACAO = %q", got)
	}
	if resp.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("expected credentials true")
	}
}

func TestCorsBlocksUnknownOrigin(t *testing.T) {
	sc := config.DefaultServerConfig()
	sc.Domains = []string{"mvnc.pkg.one"}
	app := setupCorsApp(t, sc)

	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.Header.Set("Origin", "https://evil.example")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("disallowed origin must not receive ACAO")
	}
}

func TestCorsPreflightWildcard(t *testing.T) {
	sc := config.DefaultServerConfig()
	sc.Domains = []string{"mvnc.pkg.one"}
	sc.CorsOrigins = []string{"*.pkg.one"}
	app := setupCorsApp(t, sc)

	req := httptest.NewRequest(http.MethodOptions, "/api/ping", nil)
	req.Header.Set("Origin", "https://cdn.pkg.one")
	req.Header.Set("Access-Control-Request-Method", "PUT")
	req.Header.Set("Access-Control-Request-Headers", "content-type,authorization")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://cdn.pkg.one" {
		t.Fatalf("ACAO = %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); got != "PUT" {
		t.Fatalf("methods = %q", got)
	}
}

func TestCorsPreflightForbidden(t *testing.T) {
	sc := config.DefaultServerConfig()
	sc.CorsOrigins = []string{"https://allowed.example"}
	app := setupCorsApp(t, sc)

	req := httptest.NewRequest(http.MethodOptions, "/api/ping", nil)
	req.Header.Set("Origin", "https://denied.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestCorsDomainsStillAllowedWhenCorsOriginsSet(t *testing.T) {
	sc := config.DefaultServerConfig()
	sc.Domains = []string{"mvnc.pkg.one"}
	sc.CorsOrigins = []string{"https://partner.example.com"}
	app := setupCorsApp(t, sc)

	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req.Header.Set("Origin", "https://mvnc.pkg.one")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://mvnc.pkg.one" {
		t.Fatalf("domain origin ACAO = %q, want https://mvnc.pkg.one", got)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	req2.Header.Set("Origin", "https://partner.example.com")
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if got := resp2.Header.Get("Access-Control-Allow-Origin"); got != "https://partner.example.com" {
		t.Fatalf("partner ACAO = %q", got)
	}
}
