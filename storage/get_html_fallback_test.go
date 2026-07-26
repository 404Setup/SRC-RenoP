/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"

	"renop/config"
	"renop/core"
)

func TestTryHTMLFallback(t *testing.T) {
	state := core.NewAppState()
	cfg := &config.Config{
		Frontend: config.FrontendConfig{
			Title: "Test RenoP",
		},
	}
	state.Inner.Config.Store(cfg)

	prev := HTMLFallback
	HTMLFallback = func(c fiber.Ctx, _ *core.AppState) error {
		return c.Status(fiber.StatusOK).SendString("<html>ok</html>")
	}
	t.Cleanup(func() { HTMLFallback = prev })

	app := fiber.New()

	app.Get("/test-html-fallback/*", func(c fiber.Ctx) error {
		if TryHTMLFallback(state, c) {
			return nil
		}
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	})

	reqHTML, _ := http.NewRequest("GET", "/test-html-fallback/missing.jar", nil)
	reqHTML.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9")
	respHTML, err := app.Test(reqHTML)
	if err != nil {
		t.Fatal(err)
	}
	if respHTML.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 (HTML page), got %d", respHTML.StatusCode)
	}

	reqNonHTML, _ := http.NewRequest("GET", "/test-html-fallback/missing.jar", nil)
	reqNonHTML.Header.Set("Accept", "application/xml")
	respNonHTML, err := app.Test(reqNonHTML)
	if err != nil {
		t.Fatal(err)
	}
	if respNonHTML.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404 for non-HTML client, got %d", respNonHTML.StatusCode)
	}
}
