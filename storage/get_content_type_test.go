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
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"

	"renop/config"
	"renop/core"
	"renop/index"
	"renop/utils"
)

func TestBinaryArtifactContentTypeOnFastPath(t *testing.T) {
	storagePath := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	InitS3(&cfg)

	state := core.NewAppState()
	state.Inner.Config.Store(&cfg)
	state.Inner.FileIndex = index.NewFileIndex()
	state.Inner.FileCache = core.NewFileByteCache(16 << 20)
	repo := cfg.Maven.Repositories["releases"]
	repo.AllowRedeployment = true

	app := fiber.New(fiber.Config{StreamRequestBody: true, UnescapePath: false})
	app.Put("/:repo_name/*", func(c fiber.Ctx) error {
		path, ok := utils.SanitizePath(c.Params("*"))
		if !ok {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		return HandlePut(c, state, repo, filepath.Join(storagePath, c.Params("repo_name"), path))
	})
	app.Get("/:repo_name/*", func(c fiber.Ctx) error {
		return HandleGet(c, state, repo, storagePath)
	})

	want := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0xff, 0x10, 0x80}
	const artifactURL = "/releases/assets/logo.PNG"
	putResp, err := app.Test(httptest.NewRequest(http.MethodPut, artifactURL, bytes.NewReader(want)))
	if err != nil {
		t.Fatal(err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != fiber.StatusCreated {
		t.Fatalf("PUT returned %d, want %d", putResp.StatusCode, fiber.StatusCreated)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, artifactURL, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	got, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if getResp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET returned %d, want %d", getResp.StatusCode, fiber.StatusOK)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("GET body = %v, want %v", got, want)
	}
	if contentType := getResp.Header.Get("Content-Type"); contentType != "image/png" {
		t.Fatalf("GET Content-Type = %q, want %q", contentType, "image/png")
	}

	updated := []byte{0x89, 'P', 'N', 'G', 0x01, 0x02, 0x03}
	artifactPath := filepath.Join(storagePath, "releases", "assets", "logo.PNG")
	if err := os.WriteFile(artifactPath, updated, 0644); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	state.Inner.FileIndex.InsertFile(artifactPath, index.FileInfo{Size: stat.Size(), ModTime: stat.ModTime().UnixNano()})

	updatedResp, err := app.Test(httptest.NewRequest(http.MethodGet, artifactURL, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer updatedResp.Body.Close()
	updatedBody, err := io.ReadAll(updatedResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(updatedBody, updated) {
		t.Fatalf("stale cached body = %v, want %v", updatedBody, updated)
	}
}
