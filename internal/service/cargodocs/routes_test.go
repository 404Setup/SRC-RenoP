/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargodocs

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zip"

	"github.com/gofiber/fiber/v3"
	"github.com/klauspost/compress/gzip"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/testutil"
)

func writeTestTarGzDoc(t *testing.T, entries map[string]string) string {
	t.Helper()
	file, err := os.CreateTemp(testutil.TempDir(t), "cargodoc-*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	gw := gzip.NewWriter(file)
	tw := tar.NewWriter(gw)

	for name, content := range entries {
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
	return file.Name()
}

func writeTestZipDoc(t *testing.T, entries map[string]string) string {
	t.Helper()
	file, err := os.CreateTemp(testutil.TempDir(t), "cargodoc-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	for name, content := range entries {
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
	return file.Name()
}

func withIsolatedCargodocConfig(t *testing.T, mutate func(cfg *config.Config)) *config.Config {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.CargodocExtractPath = testutil.TempDir(t)
	cfg.EnableCargodocPreview = true
	cfg.MaxCargodocSizeMb = 32
	if mutate != nil {
		mutate(cfg)
	}
	InitCargodocs(cfg)
	t.Cleanup(func() {
		currentConfig.Store(nil)
	})
	return cfg
}

func TestExtractCargodocRejectsUnsafeEntry(t *testing.T) {
	withIsolatedCargodocConfig(t, nil)
	archivePath := writeTestTarGzDoc(t, map[string]string{
		"../escape.txt": "must not be written",
		"index.html":    "<html></html>",
	})
	cacheDir := filepath.Join(testutil.TempDir(t), "cache")
	if err := extractCargodocArchive(archivePath, cacheDir, "test_crate"); err == nil {
		t.Fatal("expected unsafe archive entry to be rejected")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(cacheDir), "escape.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unsafe entry escaped extraction directory: %v", err)
	}
}

func TestExtractCargodocAutoGeneratesRootIndex(t *testing.T) {
	withIsolatedCargodocConfig(t, nil)
	archivePath := writeTestTarGzDoc(t, map[string]string{
		"my_crate/index.html": "<html><body>Rustdoc My Crate</body></html>",
		"my_crate/all.html":   "<html><body>All items</body></html>",
	})
	cacheDir := filepath.Join(testutil.TempDir(t), "cache")
	if err := extractCargodocArchive(archivePath, cacheDir, "my-crate"); err != nil {
		t.Fatalf("unexpected extraction error: %v", err)
	}
	rootIndex := filepath.Join(cacheDir, "index.html")
	content, err := os.ReadFile(rootIndex)
	if err != nil {
		t.Fatalf("failed to read generated root index.html: %v", err)
	}
	if !bytes.Contains(content, []byte("my_crate/index.html")) {
		t.Fatalf("root index.html does not redirect to module: %s", string(content))
	}
}

func TestExtractZipCargodoc(t *testing.T) {
	withIsolatedCargodocConfig(t, nil)
	archivePath := writeTestZipDoc(t, map[string]string{
		"index.html":      "<html><body>Zip Docs</body></html>",
		"search-index.js": "var R = [];",
	})
	cacheDir := filepath.Join(testutil.TempDir(t), "cache")
	if err := extractCargodocArchive(archivePath, cacheDir, "zip-crate"); err != nil {
		t.Fatalf("unexpected extraction error for zip doc: %v", err)
	}
	if !hasExtractedCargodoc(cacheDir) {
		t.Fatal("expected cacheDir to have extracted cargodoc")
	}
}

func TestCargodocHTMLInsertionScansOnlyBoundedTail(t *testing.T) {
	path := filepath.Join(testutil.TempDir(t), "large.html")
	content := strings.Repeat("x", cargodocHTMLTailScanSize*2) + "</body>" + strings.Repeat("y", 64)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	offset, found, err := cargodocHTMLInsertionOffset(file, int64(len(content)))
	_ = file.Close()
	if err != nil || !found || offset != int64(strings.LastIndex(content, "</body>")) {
		t.Fatalf("tail insertion offset = %d, found = %t, err = %v", offset, found, err)
	}

	content = "</body>" + strings.Repeat("z", cargodocHTMLTailScanSize+1)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err = os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, found, err = cargodocHTMLInsertionOffset(file, int64(len(content)))
	_ = file.Close()
	if err != nil || found {
		t.Fatalf("out-of-window closing tag found = %t, err = %v", found, err)
	}
}

func TestHandleCargodocPageAndServeRaw(t *testing.T) {
	tempStorage := testutil.TempDir(t)
	cfg := withIsolatedCargodocConfig(t, func(c *config.Config) {
		c.StoragePath = tempStorage
		c.Maven.Repositories = map[string]*config.Repository{
			"cargo-repo": {
				Name:       "cargo-repo",
				Visibility: "PUBLIC",
				Format:     "cargo",
			},
		}
	})

	crateDocDir := filepath.Join(tempStorage, "cargo-repo", "crates", "demo-crate")
	if err := os.MkdirAll(crateDocDir, 0755); err != nil {
		t.Fatal(err)
	}

	docArchive := writeTestTarGzDoc(t, map[string]string{
		"index.html":     "<html><body><h1>Demo Crate Docs</h1></body></html>",
		"stylesheet.css": "body { color: red; }",
		"search.js":      "console.log('search');",
	})

	destDocPath := filepath.Join(crateDocDir, "demo-crate-0.1.0-docs.tar.gz")
	data, err := os.ReadFile(docArchive)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destDocPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(tempStorage, "test.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	state.Inner.DB = db

	app := fiber.New()
	SetupCargodocRoutes(app, state)

	req := httptest.NewRequest(http.MethodGet, "/cargodoc/cargo-repo/demo-crate/0.1.0", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "demo-crate") || !strings.Contains(string(body), "v0.1.0") {
		t.Fatalf("expected body to contain crate name and version, got: %s", string(body))
	}

	reqRaw := httptest.NewRequest(http.MethodGet, "/cargodoc/cargo-repo/demo-crate/0.1.0/raw/index.html", nil)
	respRaw, err := app.Test(reqRaw)
	if err != nil {
		t.Fatal(err)
	}
	if respRaw.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for raw index.html, got %d", respRaw.StatusCode)
	}
	csp := respRaw.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "sandbox") || !strings.Contains(csp, "allow-scripts") || !strings.Contains(csp, "allow-popups") {
		t.Fatalf("expected CSP to contain sandbox, allow-scripts, and allow-popups, got: %s", csp)
	}
	rawBody, _ := io.ReadAll(respRaw.Body)
	if !strings.Contains(string(rawBody), "Demo Crate Docs") {
		t.Fatalf("expected raw body to contain doc title, got: %s", string(rawBody))
	}
	if !strings.Contains(string(rawBody), "target='_blank'") {
		t.Fatalf("expected raw body to contain external link handler script, got: %s", string(rawBody))
	}

	reqCSS := httptest.NewRequest(http.MethodGet, "/cargodoc/cargo-repo/demo-crate/0.1.0/raw/stylesheet.css", nil)
	respCSS, err := app.Test(reqCSS)
	if err != nil {
		t.Fatal(err)
	}
	if respCSS.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for stylesheet.css, got %d", respCSS.StatusCode)
	}

	reqMissing := httptest.NewRequest(http.MethodGet, "/cargodoc/cargo-repo/missing-crate/1.0.0", nil)
	respMissing, err := app.Test(reqMissing)
	if err != nil {
		t.Fatal(err)
	}
	if respMissing.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404 for missing crate, got %d", respMissing.StatusCode)
	}
}

func TestCleanupAndClearAllCargodocCaches(t *testing.T) {
	withIsolatedCargodocConfig(t, nil)
	archivePath := writeTestZipDoc(t, map[string]string{
		"index.html": "<html><body>Cached Doc</body></html>",
	})

	cfg := getActiveConfig()
	extractPath := getCargodocExtractPath(cfg)
	hash := cargodocHashKey("repo", "crate", "1.0.0")
	cacheDir := filepath.Join(extractPath, "renop-cargodoc-"+hash)

	if err := extractCargodocArchive(archivePath, cacheDir, "crate"); err != nil {
		t.Fatal(err)
	}
	if !hasExtractedCargodoc(cacheDir) {
		t.Fatal("cacheDir should exist")
	}

	CleanupCargodoc("repo", "crate", "1.0.0")
	if hasExtractedCargodoc(cacheDir) {
		t.Fatal("cacheDir should have been cleaned up")
	}

	cacheDir1 := filepath.Join(extractPath, "renop-cargodoc-1111")
	cacheDir2 := filepath.Join(extractPath, "renop-cargodoc-2222")
	_ = extractCargodocArchive(archivePath, cacheDir1, "crate1")
	_ = extractCargodocArchive(archivePath, cacheDir2, "crate2")

	if err := ClearAllCargodocCaches(); err != nil {
		t.Fatalf("ClearAllCargodocCaches failed: %v", err)
	}
	if hasExtractedCargodoc(cacheDir1) || hasExtractedCargodoc(cacheDir2) {
		t.Fatal("all caches should have been cleared")
	}
}
