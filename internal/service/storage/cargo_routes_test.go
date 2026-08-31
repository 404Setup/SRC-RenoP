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
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/index"
	"renop/internal/service/statistics"
	"renop/internal/testutil"
)

func newCargoRouteTestApp(t *testing.T, visibility string) *fiber.App {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.StoragePath = storageTestTempDir(t)
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

func TestCargoMirrorDownloadRecordsPackageProvenance(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/crates/serde/1.0.0/download" {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte("mirrored crate"))
	}))
	t.Cleanup(upstream.Close)

	storagePath := storageTestTempDir(t)
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories["cargo"] = &config.Repository{
		Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC",
		Mirrors: []config.Mirror{{Name: "upstream", URL: upstream.URL, TimeoutSecs: 5}},
	}
	InitS3(cfg)
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()
	state.Inner.FileCache = core.NewFileByteCache(1 << 20)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "cargo-mirror.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state.Inner.DB = db
	app := fiber.New(fiber.Config{UnescapePath: false})
	app.All("/:repo_name/*", func(c fiber.Ctx) error { return HandleRepository(c, state) })

	response, err := app.Test(httptest.NewRequest(http.MethodGet,
		"http://registry.example/cargo/api/v1/crates/serde/1.0.0/download", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "mirrored crate" {
		t.Fatalf("mirror response = %d %q", response.StatusCode, body)
	}
	details, err := db.GetCargoPackageDetails("cargo", "serde", "guest")
	if err != nil {
		t.Fatal(err)
	}
	if details == nil || details.Package == nil || !details.Package.Mirrored || len(details.Versions) != 1 || !details.Versions[0].Mirrored {
		t.Fatalf("mirrored Cargo provenance was not cataloged: %#v", details)
	}
	if err := statistics.GetCounter(state).Flush(); err != nil {
		t.Fatal(err)
	}
	var packageName, version string
	var count, bytes int64
	if err := db.QueryRow(`SELECT package_name, version, download_count, download_bytes
		FROM download_statistics WHERE repository = ?`, "cargo").Scan(&packageName, &version, &count, &bytes); err != nil {
		t.Fatal(err)
	}
	if packageName != "serde" || version != "1.0.0" || count != 1 || bytes != int64(len("mirrored crate")) {
		t.Fatalf("download statistics = %s %s %d %d", packageName, version, count, bytes)
	}
}
