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

	"renop/internal/core"
)

func TestExtractAuthDetailsDoesNotExposeSessionSecret(t *testing.T) {
	const secret = "live-session-secret-value"
	var sessionID, authMethod string

	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		c.Locals("current_session_id", secret)
		_, _, authMethod, sessionID, _ = ExtractAuthDetails(c)
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
