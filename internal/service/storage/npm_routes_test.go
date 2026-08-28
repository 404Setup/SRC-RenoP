/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
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
)

func TestNPMMirrorTarballRecordsProvenanceAndDownloadStatistics(t *testing.T) {
	config.ClearRepoCacheConfigs()
	t.Cleanup(config.ClearRepoCacheConfigs)
	tarball := []byte("mirrored npm tarball")
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/mirror-demo":
			writer.Header().Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
			if _, err := writer.Write([]byte(`{
				"name":"mirror-demo",
				"description":"Mirrored package",
				"dist-tags":{"latest":"1.2.3"},
				"time":{"created":"2026-08-01T00:00:00Z","modified":"2026-08-02T00:00:00Z","1.2.3":"2026-08-02T00:00:00Z"},
				"versions":{"1.2.3":{"name":"mirror-demo","version":"1.2.3","dist":{
					"tarball":"https://upstream.example/mirror-demo/-/mirror-demo-1.2.3.tgz",
					"shasum":"0123456789012345678901234567890123456789",
					"integrity":"sha512-ZGVtbw=="}}}
			}`)); err != nil {
				t.Errorf("write npm mirror metadata: %v", err)
			}
		case "/mirror-demo/-/mirror-demo-1.2.3.tgz":
			writer.Header().Set(fiber.HeaderContentType, "application/octet-stream")
			if _, err := writer.Write(tarball); err != nil {
				t.Errorf("write npm mirror tarball: %v", err)
			}
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(upstream.Close)

	storagePath := storageTestTempDir(t)
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"npm": {
			Name: "npm", Format: config.RepositoryFormatNPM, Visibility: "PUBLIC",
			Mirrors: []config.Mirror{{Name: "upstream", URL: upstream.URL, Persist: true, TimeoutSecs: 5}},
		},
	}
	InitS3(cfg)
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()
	state.Inner.FileCache = core.NewFileByteCache(1 << 20)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "npm-mirror.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state.Inner.DB = db
	app := fiber.New(fiber.Config{UnescapePath: false})
	app.All("/:repo_name/*", func(c fiber.Ctx) error { return HandleRepository(c, state) })

	metadataResponse, err := app.Test(httptest.NewRequest(http.MethodGet,
		"http://registry.example/npm/mirror-demo", nil))
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.ReadAll(metadataResponse.Body)
	_ = metadataResponse.Body.Close()
	if readErr != nil || metadataResponse.StatusCode != http.StatusOK {
		t.Fatalf("npm mirror metadata response = %d: %v", metadataResponse.StatusCode, readErr)
	}

	tarballResponse, err := app.Test(httptest.NewRequest(http.MethodGet,
		"http://registry.example/npm/mirror-demo/-/mirror-demo-1.2.3.tgz", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(tarballResponse.Body)
	_ = tarballResponse.Body.Close()
	if readErr != nil || tarballResponse.StatusCode != http.StatusOK || string(body) != string(tarball) {
		t.Fatalf("npm mirror tarball response = %d %q: %v", tarballResponse.StatusCode, body, readErr)
	}
	details, err := db.GetNPMPackageDetails("npm", "mirror-demo", "guest")
	if err != nil {
		t.Fatal(err)
	}
	if details == nil || details.Package == nil || !details.Package.Mirrored || len(details.Versions) != 1 ||
		!details.Versions[0].Mirrored || details.Versions[0].Size != int64(len(tarball)) {
		t.Fatalf("mirrored npm provenance was not cataloged: %#v", details)
	}
	if err := statistics.GetCounter(state).Flush(); err != nil {
		t.Fatal(err)
	}
	var format, packageName, version string
	var count, bytes int64
	if err := db.QueryRow(`SELECT format, package_name, version, download_count, download_bytes
		FROM download_statistics WHERE repository = ?`, "npm").Scan(
		&format, &packageName, &version, &count, &bytes); err != nil {
		t.Fatal(err)
	}
	if format != config.RepositoryFormatNPM || packageName != "mirror-demo" || version != "1.2.3" ||
		count != 1 || bytes != int64(len(tarball)) {
		t.Fatalf("npm download statistics = %s %s %s %d %d", format, packageName, version, count, bytes)
	}
}
