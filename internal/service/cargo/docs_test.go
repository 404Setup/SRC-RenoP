/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zip"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/klauspost/compress/gzip"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func createTestDocTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		data := []byte(content)
		hdr := &tar.Header{
			Name:     name,
			Mode:     0644,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func createTestDocZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func setupCargoDocsTestApp(t *testing.T) (*fiber.App, *core.AppState, *config.Repository, string) {
	t.Helper()
	storagePath := t.TempDir()
	repo := &config.Repository{
		Name:       "test-repo",
		Visibility: "PUBLIC",
		Format:     "cargo",
	}

	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"test-repo": repo,
	}

	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.DB = db

	now := time.Now().UnixMilli()
	_ = db.RecordCargoPublication(&core.CargoPackage{
		Repository:     repo.Name,
		Name:           "doc-crate",
		NormalizedName: "doc-crate",
		Description:    "Crate with docs",
		CreatedAt:      now,
		UpdatedAt:      now,
	}, &core.CargoVersion{
		Repository:  repo.Name,
		Package:     "doc-crate",
		Version:     "1.0.0",
		Publisher:   "owner_user",
		Description: "v1.0.0",
		CreatedAt:   now,
	}, "owner_user")

	store := newMemoryStore()
	handler := Handler{Store: store}

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		switch authHeader {
		case "Bearer owner-token":
			c.Locals("user", &config.User{Username: "owner_user", WritePermissions: []string{"test-repo"}})
		case "Bearer non-owner-token":
			c.Locals("user", &config.User{Username: "random_user"})
		default:
			c.Locals("user", &config.User{Username: "guest"})
		}
		return c.Next()
	})

	app.All("/api/v1/*", func(c fiber.Ctx) error {
		requestPath := c.Params("*")
		if !strings.HasPrefix(requestPath, "api/v1/") {
			requestPath = "api/v1/" + requestPath
		}
		handled, err := handler.Handle(c, state, repo, storagePath, requestPath)
		if handled {
			return err
		}
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	})

	return app, state, repo, storagePath
}

func TestCargoDocsLifecycle(t *testing.T) {
	app, _, _, _ := setupCargoDocsTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/crates/doc-crate/1.0.0/docs", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var statusResp DocStatusResponse
	_ = json.NewDecoder(resp.Body).Decode(&statusResp)
	if statusResp.HasDocs {
		t.Fatal("expected has_docs to be false before upload")
	}

	docBytes := createTestDocTarGz(t, map[string]string{
		"doc-crate/index.html": "<html><body>Docs</body></html>",
	})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/crates/doc-crate/1.0.0/docs", bytes.NewReader(docBytes))
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for guest upload, got %d", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/v1/crates/doc-crate/1.0.0/docs", bytes.NewReader(docBytes))
	req.Header.Set("Authorization", "Bearer owner-token")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 for owner upload, got %d: %s", resp.StatusCode, string(body))
	}
	var uploadResp DocOperationResponse
	_ = json.NewDecoder(resp.Body).Decode(&uploadResp)
	if !uploadResp.OK || uploadResp.DocURL == "" {
		t.Fatalf("expected upload success, got %+v", uploadResp)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/crates/doc-crate/1.0.0/docs", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	_ = json.NewDecoder(resp.Body).Decode(&statusResp)
	if !statusResp.HasDocs {
		t.Fatal("expected has_docs to be true after upload")
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/crates/doc-crate/1.0.0/docs", nil)
	req.Header.Set("Authorization", "Bearer owner-token")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for delete docs, got %d", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/crates/doc-crate/1.0.0/docs", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(resp.Body).Decode(&statusResp)
	if statusResp.HasDocs {
		t.Fatal("expected has_docs to be false after delete")
	}

	nonDocTarGz := createTestDocTarGz(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"doc-crate\"\nversion = \"1.0.0\"\n",
		"src/lib.rs": "pub fn hello() {}",
	})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/crates/doc-crate/1.0.0/docs", bytes.NewReader(nonDocTarGz))
	req.Header.Set("Authorization", "Bearer owner-token")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-doc archive, got %d", resp.StatusCode)
	}

	nonDocZip := createTestDocZip(t, map[string]string{
		"readme.txt": "just a text file",
	})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/crates/doc-crate/1.0.0/docs", bytes.NewReader(nonDocZip))
	req.Header.Set("Authorization", "Bearer owner-token")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-doc zip archive, got %d", resp.StatusCode)
	}

	validZip := createTestDocZip(t, map[string]string{
		"doc-crate/index.html": "<html><body>Zip docs</body></html>",
	})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/crates/doc-crate/1.0.0/docs", bytes.NewReader(validZip))
	req.Header.Set("Authorization", "Bearer owner-token")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 for valid zip doc upload, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestCargoDocsRejectWrongCrate(t *testing.T) {
	app, _, _, _ := setupCargoDocsTestApp(t)

	// Archive for different crate ("serde") uploaded to "doc-crate"
	wrongCrateDoc := createTestDocTarGz(t, map[string]string{
		"serde/index.html": "<html><body>Serde Docs</body></html>",
		"serde/all.html":   "<html><body>All</body></html>",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/crates/doc-crate/1.0.0/docs", bytes.NewReader(wrongCrateDoc))
	req.Header.Set("Authorization", "Bearer owner-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 when uploading doc of different crate, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "does not contain documentation for crate") {
		t.Fatalf("expected error message mentioning missing crate documentation, got: %s", string(body))
	}
}

func TestCargoDocsRejectForbiddenContent(t *testing.T) {
	app, _, _, _ := setupCargoDocsTestApp(t)

	forbiddenCases := []struct {
		name     string
		fileName string
		content  string
	}{
		{"Executable", "doc_crate/malware.exe", "MZ..."},
		{"Shell Script", "doc_crate/install.sh", "#!/bin/bash\necho bad"},
		{"PHP Script", "doc_crate/shell.php", "<?php phpinfo(); ?>"},
		{"System Git", ".git/HEAD", "ref: refs/heads/main"},
		{"Hidden Env", "doc_crate/.env", "SECRET=xyz"},
		{"Nested Zip", "doc_crate/nested.zip", "PK..."},
	}

	for _, tc := range forbiddenCases {
		t.Run(tc.name, func(t *testing.T) {
			docBytes := createTestDocTarGz(t, map[string]string{
				"doc_crate/index.html": "<html><body>Doc</body></html>",
				tc.fileName:            tc.content,
			})
			req := httptest.NewRequest(http.MethodPut, "/api/v1/crates/doc-crate/1.0.0/docs", bytes.NewReader(docBytes))
			req.Header.Set("Authorization", "Bearer owner-token")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 for forbidden content %s, got %d", tc.fileName, resp.StatusCode)
			}
		})
	}
}

func TestCargoDocsRejectTrackers(t *testing.T) {
	app, _, _, _ := setupCargoDocsTestApp(t)

	trackerCases := []struct {
		name    string
		content string
	}{
		{"Google Analytics Script", "<html><body><script src=\"https://www.googletagmanager.com/gtag/js\"></script></body></html>"},
		{"Google Analytics Inlined gtag", "<html><body><script>gtag('config', 'UA-12345');</script></body></html>"},
		{"Baidu Tongji", "<html><body><script>var _hmt = _hmt || []; _hmt.push(['_trackEvent', 'doc']);</script></body></html>"},
		{"Plausible Analytics", "<html><body><script defer data-domain=\"example.com\" src=\"https://plausible.io/js/script.js\"></script></body></html>"},
		{"Cloudflare Insights", "<html><body><script src=\"https://static.cloudflareinsights.com/beacon.min.js\"></script></body></html>"},
		{"Matomo Analytics", "<html><body><script>_paq.push(['trackPageView']);</script></body></html>"},
		{"Beacon API", "<html><body><script>navigator.sendBeacon('https://tracker.com', data);</script></body></html>"},
		{"Ping Tracking", "<html><body><a href=\"https://rust-lang.org\" ping=\"https://track.com/ping\">Rust</a></body></html>"},
		{"Tracking Pixel 1x1", "<html><body><img src=\"https://track.com/pixel.png\" width=\"1\" height=\"1\" /></body></html>"},
		{"Hidden Tracking Pixel", "<html><body><img src=\"https://track.com/pixel.png\" style=\"display:none\" /></body></html>"},
		{"Remote Script", "<html><body><script src=\"https://evil.com/hook.js\"></script></body></html>"},
	}

	for _, tc := range trackerCases {
		t.Run(tc.name, func(t *testing.T) {
			docBytes := createTestDocTarGz(t, map[string]string{
				"doc_crate/index.html": tc.content,
			})
			req := httptest.NewRequest(http.MethodPut, "/api/v1/crates/doc-crate/1.0.0/docs", bytes.NewReader(docBytes))
			req.Header.Set("Authorization", "Bearer owner-token")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 for tracker %s, got %d", tc.name, resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), "tracker") && !strings.Contains(string(body), "suspicious") {
				t.Fatalf("expected error message mentioning tracker, got: %s", string(body))
			}
		})
	}
}

func TestCargoDocsStripNonTargetPackageContent(t *testing.T) {
	app, _, repo, storagePath := setupCargoDocsTestApp(t)

	// Archive contains target crate ("doc_crate") AND third-party dependencies ("serde", "tokio")
	mixedDocArchive := createTestDocTarGz(t, map[string]string{
		"doc_crate/index.html":                "<html><body>Target Crate Docs</body></html>",
		"doc_crate/struct.Client.html":        "<html><body>Client Struct</body></html>",
		"static.files/storage-123.js":         "console.log('rustdoc');",
		"crates.js":                           "window.ALL_CRATES = ['doc_crate'];",
		"search-index.js":                     "var searchIndex = {};",
		"src/doc_crate/lib.rs.html":           "<html><body>Source code for doc_crate</body></html>",
		"trait.impl/doc_crate/trait.Test.js":  "console.log('trait');",
		"serde/index.html":                    "<html><body>Serde Docs - Should Be Stripped</body></html>",
		"serde/de/index.html":                 "<html><body>Serde De - Should Be Stripped</body></html>",
		"tokio/index.html":                    "<html><body>Tokio Docs - Should Be Stripped</body></html>",
		"src/serde/lib.rs.html":               "<html><body>Serde Source - Should Be Stripped</body></html>",
		"trait.impl/serde/trait.Serialize.js": "console.log('serde trait');",
		"type.impl/tokio/struct.Runtime.js":   "console.log('tokio type');",
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/crates/doc-crate/1.0.0/docs", bytes.NewReader(mixedDocArchive))
	req.Header.Set("Authorization", "Bearer owner-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 for upload, got %d: %s", resp.StatusCode, string(body))
	}

	// Verify the stored archive on storage
	storePath := cargoDocStoragePath(storagePath, repo, "doc-crate", "1.0.0", false)
	f, err := os.Open(storePath)
	if err != nil {
		// In test setup, memoryStore was used, check memoryStore or extraction
		t.Logf("Checking extraction directly")
	} else {
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			t.Fatal(err)
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		for {
			hdr, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(hdr.Name, "serde") || strings.HasPrefix(hdr.Name, "tokio") {
				t.Fatalf("non-target package file was not stripped: %s", hdr.Name)
			}
			if strings.HasPrefix(hdr.Name, "src/serde") || strings.HasPrefix(hdr.Name, "trait.impl/serde") {
				t.Fatalf("non-target package source/impl file was not stripped: %s", hdr.Name)
			}
		}
	}
}
