/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/klauspost/compress/zstd"
	brrr "github.com/molecule-man/go-brrr"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
)

func compressedNavigationFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	payload := []byte("RenoP compressed navigation fixture")

	var brotli bytes.Buffer
	brotliWriter, err := brrr.NewWriterOptions(&brotli, brrr.BestSpeed,
		brrr.WriterOptions{SizeHint: uint(len(payload))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := brotliWriter.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := brotliWriter.Close(); err != nil {
		t.Fatal(err)
	}

	var gzipData bytes.Buffer
	gzipWriter := gzip.NewWriter(&gzipData)
	if _, err := gzipWriter.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	zstdWriter, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
	if err != nil {
		t.Fatal(err)
	}
	zstdData := zstdWriter.EncodeAll(payload, nil)
	zstdWriter.Close()

	return map[string][]byte{
		"update.br":   brotli.Bytes(),
		"update.gz":   gzipData.Bytes(),
		"update.tgz":  gzipData.Bytes(),
		"update.zst":  zstdData,
		"update.zstd": zstdData,
		"update.zip":  append([]byte("PK\x03\x04"), payload...),
		"update.xz":   append([]byte("\xfd7zXZ\x00"), payload...),
		"update.bz2":  append([]byte("BZh"), payload...),
		"update.7z":   append([]byte("7z\xbc\xaf\x27\x1c"), payload...),
		"update.rar":  append([]byte("Rar!"), payload...),
		"update.lz4":  append([]byte("\x04\x22\x4d\x18"), payload...),
		"update.sz":   append([]byte("\xff\x06\x00\x00"), payload...),
		"update.cab":  append([]byte("MSCF"), payload...),
		"update.wim":  append([]byte("MSWIM"), payload...),
	}
}

func TestCompressedFilesBypassHTMLFallbackAndStreamAsAttachments(t *testing.T) {
	storagePath := storageTestTempDir(t)
	repositoryRoot := filepath.Join(storagePath, "files")
	if err := os.MkdirAll(filepath.Join(repositoryRoot, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"files": {
			Name: "files", Format: config.RepositoryFormatFiles, Visibility: "PUBLIC",
			Mirrors: []config.Mirror{},
		},
	}
	InitS3(cfg)
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndexCustom(true)
	state.Inner.FileIndex.InsertDir(filepath.ToSlash(repositoryRoot))
	state.Inner.FileIndex.InsertDir(filepath.ToSlash(filepath.Join(repositoryRoot, "folder")))

	fixtures := compressedNavigationFixtures(t)
	for name, content := range fixtures {
		path := filepath.Join(repositoryRoot, name)
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
		state.Inner.FileIndex.InsertFile(filepath.ToSlash(path), index.FileInfo{
			Size: int64(len(content)), ModTime: time.Now().UnixNano(),
		})
	}

	fallbackCalls := 0
	previousFallback := HTMLFallback
	HTMLFallback = func(c fiber.Ctx, _ *core.AppState) error {
		fallbackCalls++
		return c.Status(http.StatusOK).SendString("<html>spa</html>")
	}
	t.Cleanup(func() { HTMLFallback = previousFallback })
	app := fiber.New(fiber.Config{UnescapePath: false})
	SetupRoutes(app, state)

	contentTypes := map[string]string{
		".br": "application/x-brotli", ".gz": "application/gzip", ".tgz": "application/gzip",
		".zst": "application/zstd", ".zstd": "application/zstd", ".zip": "application/zip",
		".xz": "application/x-xz", ".bz2": "application/x-bzip2", ".7z": "application/x-7z-compressed",
		".rar": "application/vnd.rar", ".lz4": "application/x-lz4",
		".sz": "application/x-snappy-framed", ".cab": "application/vnd.ms-cab-compressed",
		".wim": "application/x-ms-wim",
	}
	for name, expectedBody := range fixtures {
		request := httptest.NewRequest(http.MethodGet, "http://repo.example/files/"+name, nil)
		request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		response, err := app.Test(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusOK || !bytes.Equal(body, expectedBody) {
			t.Fatalf("%s response = %d with %d bytes; want %d bytes", name,
				response.StatusCode, len(body), len(expectedBody))
		}
		if !strings.HasPrefix(response.Header.Get("Content-Disposition"), "attachment") {
			t.Fatalf("%s Content-Disposition = %q", name, response.Header.Get("Content-Disposition"))
		}
		if got := response.Header.Get("Content-Type"); got != contentTypes[strings.ToLower(filepath.Ext(name))] {
			t.Fatalf("%s Content-Type = %q", name, got)
		}
		if encoding := response.Header.Get("Content-Encoding"); encoding != "" {
			t.Fatalf("%s was mislabeled as HTTP content encoding %q", name, encoding)
		}
	}
	if fallbackCalls != 0 {
		t.Fatalf("compressed file navigation invoked the SPA fallback %d time(s)", fallbackCalls)
	}

	for _, path := range []string{"/files/folder/", "/files/missing.br"} {
		request := httptest.NewRequest(http.MethodGet, "http://repo.example"+path, nil)
		request.Header.Set("Accept", "text/html")
		response, err := app.Test(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || string(body) != "<html>spa</html>" {
			t.Fatalf("SPA route %s returned %d %q: %v", path, response.StatusCode, body, readErr)
		}
	}
	if fallbackCalls != 2 {
		t.Fatalf("directory/missing navigation invoked the SPA fallback %d time(s), want 2", fallbackCalls)
	}
}
