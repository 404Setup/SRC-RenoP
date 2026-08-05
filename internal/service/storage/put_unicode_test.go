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

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
	"renop/internal/utils"
)

func TestUnicodeFilenameUploadAndDownload(t *testing.T) {
	storagePath := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	InitS3(&cfg)

	state := core.NewAppState()
	state.Inner.Config.Store(&cfg)
	state.Inner.FileIndex = index.NewFileIndex()
	repo := cfg.Maven.Repositories["releases"]
	repo.AllowRedeployment = true

	app := fiber.New(fiber.Config{StreamRequestBody: true, UnescapePath: false})
	var receivedPath string
	app.Put("/:repo_name/*", func(c fiber.Ctx) error {
		receivedPath = c.Params("*")
		path, ok := utils.SanitizePath(receivedPath)
		if !ok {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		localPath := filepath.Join(storagePath, c.Params("repo_name"), path)
		return HandlePut(c, state, repo, localPath)
	})
	app.Get("/:repo_name/*", func(c fiber.Ctx) error {
		return HandleGet(c, state, repo, storagePath)
	})
	app.Head("/:repo_name/*", func(c fiber.Ctx) error {
		return HandleHead(c, state, repo, storagePath)
	})

	want := []byte("unicode artifact content")
	const artifactURL = "/releases/%E4%B8%AD%E6%96%87/%E4%BE%9D%E8%B5%96%20%F0%9F%9A%80.jar"
	req := httptest.NewRequest(http.MethodPut, artifactURL, bytes.NewReader(want))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("PUT returned %d, want %d (path parameter %q)", resp.StatusCode, fiber.StatusCreated, receivedPath)
	}

	artifactPath := filepath.Join(storagePath, "releases", "中文", "依赖 🚀.jar")
	got, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("reading uploaded Unicode artifact: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("uploaded content = %q, want %q", got, want)
	}

	getResp, err := app.Test(httptest.NewRequest(http.MethodGet, artifactURL, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	got, err = io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if getResp.StatusCode != fiber.StatusOK || !bytes.Equal(got, want) {
		t.Fatalf("GET returned status %d and body %q (indexed=%v)", getResp.StatusCode, got, state.Inner.FileIndex.HasFile(artifactPath))
	}

	headResp, err := app.Test(httptest.NewRequest(http.MethodHead, artifactURL, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer headResp.Body.Close()
	if headResp.StatusCode != fiber.StatusOK || headResp.ContentLength != int64(len(want)) {
		t.Fatalf("HEAD returned status %d and content length %d", headResp.StatusCode, headResp.ContentLength)
	}
}
