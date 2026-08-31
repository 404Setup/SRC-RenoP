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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/index"
	"renop/internal/service/statistics"
	"renop/internal/testutil"
)

func npmStorageTestTarball(t *testing.T, packageName, version string) []byte {
	t.Helper()
	manifest, err := json.Marshal(map[string]string{"name": packageName, "version": version})
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: "package/package.json", Mode: 0o644, Size: int64(len(manifest)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(manifest); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

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
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "npm-mirror.db"), MaxOpenConns: 1, MaxIdleConns: 1,
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

func TestNPMScopedPublishUsesDecodedPathThroughStorageRouter(t *testing.T) {
	storagePath := storageTestTempDir(t)
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"npm": {Name: "npm", Format: config.RepositoryFormatNPM, Visibility: "PUBLIC", Mirrors: []config.Mirror{}},
	}
	InitS3(cfg)
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()
	state.Inner.FileCache = core.NewFileByteCache(1 << 20)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "npm-scoped-publish.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state.Inner.DB = db
	if err := db.SaveToken(&core.AccessToken{Name: "alice", Permissions: []string{"base", "canupdate:npm"}}); err != nil {
		t.Fatal(err)
	}
	const packageName = "@team/scoped-demo"
	const version = "1.2.3"
	if _, err := db.CreateNPMPackage("npm", packageName, "alice", false, 1); err != nil {
		t.Fatal(err)
	}
	tarball := npmStorageTestTarball(t, packageName, version)
	document := map[string]any{
		"_id": packageName, "name": packageName, "description": "Scoped package", "access": "public",
		"dist-tags": map[string]string{"latest": version},
		"versions":  map[string]any{version: map[string]any{"name": packageName, "version": version}},
		"_attachments": map[string]any{packageName + "-" + version + ".tgz": map[string]any{
			"content_type": "application/octet-stream", "length": len(tarball),
			"data": base64.StdEncoding.EncodeToString(tarball),
		}},
	}
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New(fiber.Config{UnescapePath: false, StreamRequestBody: true})
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", &config.User{Username: "alice", Roles: []string{"base", "canupdate:npm"}})
		return c.Next()
	})
	app.All("/:repo_name/*", func(c fiber.Ctx) error { return HandleRepository(c, state) })
	request := httptest.NewRequest(http.MethodPut,
		"http://registry.example/npm/@team%2fscoped-demo", bytes.NewReader(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	responseBody, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusCreated {
		t.Fatalf("scoped npm storage publish = %d %q: %v", response.StatusCode, responseBody, readErr)
	}
	details, err := db.GetNPMPackageDetails("npm", packageName, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if details == nil || len(details.Versions) != 1 || details.Versions[0].Version != version {
		t.Fatalf("scoped npm version was not recorded: %#v", details)
	}
}
