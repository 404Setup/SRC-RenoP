/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package javadocs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zip"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
)

func writeTestJavadocJar(t *testing.T, entries map[string]string) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "javadoc-*.jar")
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(file.Name())
}

func TestExtractJavadocRejectsUnsafeEntry(t *testing.T) {
	withIsolatedJavadocConfig(t, nil)
	jarPath := writeTestJavadocJar(t, map[string]string{
		"../escape.txt": "must not be written",
		"index.html":    "<html></html>",
	})
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := extractJavadoc(jarPath, cacheDir); err == nil {
		t.Fatal("expected unsafe archive entry to be rejected")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(cacheDir), "escape.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unsafe entry escaped extraction directory: %v", err)
	}
}

func TestExtractJavadocRequiresIndex(t *testing.T) {
	withIsolatedJavadocConfig(t, nil)
	jarPath := writeTestJavadocJar(t, map[string]string{"raw/doc.html": "docs"})
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := extractJavadoc(jarPath, cacheDir); err == nil {
		t.Fatal("expected archive without index.html to fail")
	}
	if _, err := os.Stat(cacheDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("failed extraction left a cache directory: %v", err)
	}
}

func TestHandleJavadocPageAndServeRaw(t *testing.T) {
	tempStorage := t.TempDir()
	withIsolatedJavadocConfig(t, func(cfg *config.Config) {
		cfg.StoragePath = tempStorage
	})

	repoDir := filepath.Join(tempStorage, "releases", "com", "example", "demo", "1.0.0")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	jarContent := map[string]string{
		"index.html":     "<html><body><h1>Javadoc Title</h1></body></html>",
		"stylesheet.css": "body { color: red; }",
		"script.js":      "console.log('test');",
	}

	file, err := os.Create(filepath.Join(repoDir, "demo-1.0.0-javadoc.jar"))
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	for name, content := range jarContent {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	_ = zw.Close()
	_ = file.Close()

	cfg := getActiveConfig()
	cfg.Maven = config.MavenSettings{
		Repositories: map[string]*config.Repository{
			"releases": {
				Name:       "releases",
				Visibility: "PUBLIC",
			},
		},
	}

	appState := core.NewAppState()
	appState.Inner.Config.Store(cfg)
	app := fiber.New()
	SetupJavadocRoutes(app, appState)

	req, _ := http.NewRequest("GET", "/javadoc/releases/com/example/demo/1.0.0/demo-1.0.0-javadoc.jar", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "window.location.pathname") {
		t.Fatalf("expected template to use window.location.pathname, got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, `sandbox="allow-scripts"`) {
		t.Fatalf("expected iframe to retain script support inside an opaque origin")
	}
	for _, unsafeCapability := range []string{"allow-same-origin", "allow-forms", "allow-popups"} {
		if strings.Contains(bodyStr, unsafeCapability) {
			t.Fatalf("iframe must not grant %s to uploaded documentation", unsafeCapability)
		}
	}

	reqRaw, _ := http.NewRequest("GET", "/javadoc/releases/com/example/demo/1.0.0/demo-1.0.0-javadoc.jar/raw/index.html", nil)
	respRaw, err := app.Test(reqRaw)
	if err != nil {
		t.Fatal(err)
	}
	if respRaw.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for raw index.html, got %d", respRaw.StatusCode)
	}
	if respRaw.Header.Get("Content-Disposition") != "inline" {
		t.Fatalf("expected Content-Disposition inline, got %s", respRaw.Header.Get("Content-Disposition"))
	}
	if csp := respRaw.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox allow-scripts") || strings.Contains(csp, "allow-same-origin") {
		t.Fatalf("expected raw documentation to run in a script-only sandbox, got: %s", csp)
	}
	if got := respRaw.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}

	reqCSS, _ := http.NewRequest("GET", "/javadoc/releases/com/example/demo/1.0.0/demo-1.0.0-javadoc.jar/raw/stylesheet.css", nil)
	respCSS, err := app.Test(reqCSS)
	if err != nil {
		t.Fatal(err)
	}
	if respCSS.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for stylesheet.css, got %d", respCSS.StatusCode)
	}
	if respCSS.Header.Get("Content-Disposition") != "inline" {
		t.Fatalf("expected Content-Disposition inline for sub-resources, got %s", respCSS.Header.Get("Content-Disposition"))
	}
	if cc := respCSS.Header.Get("Cache-Control"); !strings.Contains(cc, "max-age=") {
		t.Fatalf("expected Cache-Control max-age for raw assets, got %q", cc)
	}
}

func withIsolatedJavadocConfig(t *testing.T, mutate func(*config.Config)) {
	t.Helper()
	prev := currentConfig.Load()
	t.Cleanup(func() {
		currentConfig.Store(prev)
	})
	cfg := &config.Config{
		EnableJavadocPreview: true,
		JavadocExtractPath:   t.TempDir(),
		MaxJavadocSizeMb:     48,
	}
	if mutate != nil {
		mutate(cfg)
	}
	InitJavadocs(cfg)
}

func TestExtractJavadocManyEntries(t *testing.T) {
	withIsolatedJavadocConfig(t, nil)

	entries := map[string]string{"index.html": "<html></html>"}
	for i := range 200 {
		entries[fmt.Sprintf("pkg/file-%03d.html", i)] = "<html>doc</html>"
	}

	jarPath := writeTestJavadocJar(t, entries)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := extractJavadoc(jarPath, cacheDir); err != nil {
		t.Fatalf("extractJavadoc failed: %v", err)
	}
	if !hasExtractedJavadoc(cacheDir) {
		t.Fatal("expected extracted index.html")
	}
	for i := range 200 {
		p := filepath.Join(cacheDir, "pkg", fmt.Sprintf("file-%03d.html", i))
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing extracted entry %s: %v", p, err)
		}
	}
}

func TestClearAllJavadocCachesSkipsInFlightExtract(t *testing.T) {
	withIsolatedJavadocConfig(t, nil)
	cfg := getActiveConfig()
	extractPath := getJavadocExtractPath(cfg)

	inFlight, err := os.MkdirTemp(extractPath, "renop-javadoc-extract-*")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(inFlight, "index.html")
	if err := os.WriteFile(marker, []byte("<html></html>"), 0644); err != nil {
		t.Fatal(err)
	}

	finished := filepath.Join(extractPath, "renop-javadoc-test-finished")
	if err := os.MkdirAll(finished, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(finished, "index.html"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ClearAllJavadocCaches(); err != nil {
		t.Fatalf("ClearAllJavadocCaches: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("in-flight extract workspace was cleared: %v", err)
	}
	if _, err := os.Stat(finished); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("finished cache dir should have been cleared, still exists: %v", err)
	}
}

func TestServeRawJavadocRejectsSymlink(t *testing.T) {
	tempStorage := t.TempDir()
	withIsolatedJavadocConfig(t, func(cfg *config.Config) {
		cfg.StoragePath = tempStorage
		cfg.Maven = config.MavenSettings{
			Repositories: map[string]*config.Repository{
				"releases": {Name: "releases", Visibility: "PUBLIC"},
			},
		}
	})

	repoDir := filepath.Join(tempStorage, "releases", "com", "example", "demo", "1.0.0")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	jarPath := writeTestJavadocJar(t, map[string]string{
		"index.html": "<html>ok</html>",
	})
	destJar := filepath.Join(repoDir, "demo-1.0.0-javadoc.jar")
	data, err := os.ReadFile(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destJar, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := getActiveConfig()
	appState := core.NewAppState()
	appState.Inner.Config.Store(cfg)

	cacheDir, err := EnsureJavadocExtractedBlocking(destJar)
	if err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(cacheDir, "escape.html")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	app := fiber.New()
	SetupJavadocRoutes(app, appState)
	req, _ := http.NewRequest("GET", "/javadoc/releases/com/example/demo/1.0.0/demo-1.0.0-javadoc.jar/raw/escape.html", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected symlink raw path to be rejected, got %d body=%s", resp.StatusCode, body)
	}
}

func TestClearAllJavadocCaches(t *testing.T) {
	withIsolatedJavadocConfig(t, nil)
	extractPath := getJavadocExtractPath(getActiveConfig())
	cacheDir1 := filepath.Join(extractPath, "renop-javadoc-test1")
	cacheDir2 := filepath.Join(extractPath, "renop-javadoc-test2")

	if err := os.MkdirAll(cacheDir1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDir2, 0755); err != nil {
		t.Fatal(err)
	}

	if err := ClearAllJavadocCaches(); err != nil {
		t.Fatalf("ClearAllJavadocCaches failed: %v", err)
	}

	if _, err := os.Stat(cacheDir1); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("cacheDir1 still exists after ClearAllJavadocCaches")
	}
	if _, err := os.Stat(cacheDir2); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("cacheDir2 still exists after ClearAllJavadocCaches")
	}
}
