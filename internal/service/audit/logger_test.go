/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package audit

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
)

func TestExtractAuthDetailsDoesNotExposeSessionSecret(t *testing.T) {
	const secret = "live-session-secret-value"
	var sessionID, authMethod string
	state := core.NewAppState()
	state.Inner.Config.Store(config.DefaultConfig())

	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		c.Locals("current_session_id", secret)
		_, _, authMethod, sessionID, _ = ExtractAuthDetails(c, state)
		return c.SendStatus(fiber.StatusNoContent)
	})
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	want := core.SafeAuditSessionID(secret)
	if sessionID != want {
		t.Fatalf("session ID = %q, want %q", sessionID, want)
	}
	if strings.Contains(authMethod, secret) {
		t.Fatal("auth method contains the session secret")
	}
	if !strings.HasPrefix(authMethod, "Web (SessionID: sha256:") {
		t.Fatalf("auth method = %q", authMethod)
	}
}

func TestExtractAuthDetailsUsesRuntimeIPConfig(t *testing.T) {
	const clientIP = "203.0.113.50"
	state := core.NewAppState()
	cfg := config.DefaultConfig()
	cfg.Server.CdnIpHeader = "CF-Connecting-IP"
	cfg.Server.TrustedProxies = []string{"0.0.0.0"}
	cfg.Server.ParseTrustedProxies()
	state.Inner.Config.Store(cfg)

	var got string
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		_, _, _, _, got = ExtractAuthDetails(c, state)
		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequest(fiber.MethodGet, "/", nil)
	req.Header.Set("CF-Connecting-IP", clientIP)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if got != clientIP {
		t.Fatalf("audit IP = %q, want runtime-configured client IP %q", got, clientIP)
	}
}
