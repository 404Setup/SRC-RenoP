/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
)

func TestGeneratePomFilenameAppending(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "renop-test-storage-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &config.Config{
		StoragePath: tempDir,
	}
	cfg.Maven.Repositories = map[string]*config.Repository{
		"test-repo": {
			Name:              "test-repo",
			Visibility:        "public",
			AllowRedeployment: true,
		},
	}

	mockUser := &config.User{
		Username:         "admin",
		Roles:            []string{"admin"},
		WritePermissions: []string{"test-repo"},
	}

	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()

	app := fiber.New()
	app.Post("/maven/generate/pom/:repo_name/*", func(c fiber.Ctx) error {
		c.Locals("user", mockUser)
		return GeneratePom(c, state)
	})

	payload := PomDetails{
		GroupId:    "com.example",
		ArtifactId: "test-artifact",
		Version:    "1.0.0",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/maven/generate/pom/test-repo/com/example/test-artifact/1.0.0/test-artifact-1.0.0.pom", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("test request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}

	expectedFilePath := filepath.Join(tempDir, "test-repo", "com", "example", "test-artifact", "1.0.0", "test-artifact-1.0.0.pom")
	if _, err := os.Stat(expectedFilePath); err != nil {
		t.Errorf("expected POM file to exist at %s, but got error: %v", expectedFilePath, err)
	}

	original, err := os.ReadFile(expectedFilePath)
	if err != nil {
		t.Fatalf("failed to read generated POM: %v", err)
	}
	cfg.Maven.Repositories["test-repo"].AllowRedeployment = false
	redeployPayload := PomDetails{
		GroupId:    "com.replaced",
		ArtifactId: "test-artifact",
		Version:    "1.0.0",
	}
	redeployBody, _ := json.Marshal(redeployPayload)
	redeployReq := httptest.NewRequest("POST", "/maven/generate/pom/test-repo/com/example/test-artifact/1.0.0/test-artifact-1.0.0.pom", bytes.NewReader(redeployBody))
	redeployReq.Header.Set("Content-Type", "application/json")
	redeployResp, err := app.Test(redeployReq)
	if err != nil {
		t.Fatalf("redeployment request failed: %v", err)
	}
	if redeployResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", redeployResp.StatusCode)
	}
	after, err := os.ReadFile(expectedFilePath)
	if err != nil {
		t.Fatalf("failed to read POM after rejected redeployment: %v", err)
	}
	if !bytes.Equal(after, original) {
		t.Fatal("rejected redeployment changed the existing POM")
	}
	cfg.Maven.Repositories["test-repo"].AllowRedeployment = true

	folderPath := filepath.Join(tempDir, "test-repo", "com", "example", "test-artifact", "2.0.0")
	if err := os.MkdirAll(folderPath, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	dummyFile := filepath.Join(folderPath, "dummy.txt")
	_ = os.WriteFile(dummyFile, []byte("dummy"), 0644)

	payload2 := PomDetails{
		GroupId:    "com.example",
		ArtifactId: "test-artifact",
		Version:    "2.0.0",
	}
	body2, _ := json.Marshal(payload2)

	req2 := httptest.NewRequest("POST", "/maven/generate/pom/test-repo/com/example/test-artifact/2.0.0", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("test request 2 failed: %v", err)
	}
	if resp2.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201, got %d", resp2.StatusCode)
	}

	expectedFilePath2 := filepath.Join(tempDir, "test-repo", "com", "example", "test-artifact", "2.0.0", "test-artifact-2.0.0.pom")
	if _, err := os.Stat(expectedFilePath2); err != nil {
		t.Errorf("expected appended POM file to exist at %s, but got error: %v", expectedFilePath2, err)
	}
}
