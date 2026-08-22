/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
)

func newCargoRouteTestApp(t *testing.T, visibility string) *fiber.App {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	cfg.Maven.Repositories["cargo"] = &config.Repository{
		Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: visibility,
		Mirrors: []config.Mirror{},
	}
	InitS3(cfg)
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()
	state.Inner.FileCache = core.NewFileByteCache(1 << 20)
	app := fiber.New(fiber.Config{UnescapePath: false})
	app.All("/:repo_name/*", func(c fiber.Ctx) error { return HandleRepository(c, state) })
	return app
}

func TestPrivateCargoConfigChallengesAnonymousClient(t *testing.T) {
	app := newCargoRouteTestApp(t, "PRIVATE")
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "http://registry.example/cargo/config.json", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
	if challenge := resp.Header.Get(fiber.HeaderWWWAuthenticate); challenge != "Cargo" {
		t.Fatalf("WWW-Authenticate = %q, want Cargo", challenge)
	}
}

func TestHiddenCargoConfigDoesNotRequireAuthentication(t *testing.T) {
	app := newCargoRouteTestApp(t, "HIDDEN")
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "http://registry.example/cargo/config.json", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}
