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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/index"
	"renop/internal/service/statistics"
	"renop/internal/testutil"
)

func insertDownloadFixture(t *testing.T, state *core.AppState, storagePath, relativePath string, contents []byte) {
	t.Helper()
	path := filepath.Join(storagePath, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	state.Inner.FileIndex.EnsureParentDirs(path)
	state.Inner.FileIndex.InsertFile(path, index.FileInfo{Size: int64(len(contents)), ModTime: time.Now().UnixNano()})
}

func TestMavenAndFilesDownloadsRecordOnlyPrimaryTransfers(t *testing.T) {
	storagePath := storageTestTempDir(t)
	enabled := true
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"releases": {Name: "releases", Format: config.RepositoryFormatMaven, Visibility: "PUBLIC"},
		"files": {
			Name: "files", Format: config.RepositoryFormatFiles, Visibility: "PUBLIC", DownloadStatistics: &enabled,
		},
	}
	InitS3(cfg)
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndexCustom(true)
	state.Inner.FileCache = core.NewFileByteCache(1 << 20)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "storage-statistics.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state.Inner.DB = db

	jar := []byte("primary Maven artifact")
	archive := []byte("unstructured release archive")
	insertDownloadFixture(t, state, storagePath, "releases/com/example/demo/1.0/demo-1.0.jar", jar)
	insertDownloadFixture(t, state, storagePath, "releases/com/example/demo/1.0/demo-1.0.jar.sha256", []byte("checksum"))
	insertDownloadFixture(t, state, storagePath, "releases/com/example/demo/1.0/demo-1.0-javadoc.jar", []byte("docs"))
	insertDownloadFixture(t, state, storagePath, "files/releases/app.zip", archive)
	insertDownloadFixture(t, state, storagePath, "files/releases/app.zip.sha256", []byte("checksum"))

	app := fiber.New(fiber.Config{UnescapePath: false})
	SetupRoutes(app, state)
	requests := []struct {
		method    string
		path      string
		byteRange string
	}{
		{method: http.MethodGet, path: "/releases/com/example/demo/1.0/demo-1.0.jar"},
		{method: http.MethodGet, path: "/releases/com/example/demo/1.0/demo-1.0.jar.sha256"},
		{method: http.MethodGet, path: "/releases/com/example/demo/1.0/demo-1.0-javadoc.jar"},
		{method: http.MethodGet, path: "/releases/com/example/demo/1.0/demo-1.0.jar", byteRange: "bytes=2-4"},
		{method: http.MethodGet, path: "/files/releases/app.zip"},
		{method: http.MethodHead, path: "/files/releases/app.zip"},
		{method: http.MethodGet, path: "/files/releases/app.zip.sha256"},
	}
	for _, request := range requests {
		httpRequest := httptest.NewRequest(request.method, "http://repo.example"+request.path, nil)
		if request.byteRange != "" {
			httpRequest.Header.Set(fiber.HeaderRange, request.byteRange)
		}
		response, requestErr := app.Test(httpRequest)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
			t.Fatalf("%s %s returned %d", request.method, request.path, response.StatusCode)
		}
	}

	if err := statistics.GetCounter(state).Flush(); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT repository, namespace, package_name, version, download_count, download_bytes
		FROM download_statistics ORDER BY repository`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type recordedDownload struct {
		repository, namespace, packageName, version string
		count, bytes                                int64
	}
	records := make([]recordedDownload, 0, 2)
	for rows.Next() {
		var record recordedDownload
		if err := rows.Scan(&record.repository, &record.namespace, &record.packageName, &record.version,
			&record.count, &record.bytes); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("recorded downloads = %#v", records)
	}
	if records[0].repository != "files" || records[0].packageName != "releases/app.zip" ||
		records[0].count != 1 || records[0].bytes != int64(len(archive)) {
		t.Fatalf("files download statistics = %#v", records[0])
	}
	if records[1].repository != "releases" || records[1].namespace != "com.example" ||
		records[1].packageName != "com.example:demo" || records[1].version != "1.0" ||
		records[1].count != 1 || records[1].bytes != int64(len(jar)) {
		t.Fatalf("Maven download statistics = %#v", records[1])
	}
}
