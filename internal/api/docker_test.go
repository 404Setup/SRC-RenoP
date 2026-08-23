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
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/auth"
	"renop/internal/service/docker"
	"renop/internal/service/index"
)

func setupTestAPIDockerApp(t *testing.T) (*fiber.App, *core.AppState) {
	t.Helper()
	dir := t.TempDir()

	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite",
		Dsn:    filepath.Join(dir, "docker_api_test.db"),
	})
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	adminToken := &core.AccessToken{
		Name:        "admin",
		Tokens:      []string{"admin-test-token"},
		Permissions: []string{"admin"},
	}
	_ = db.SaveToken(adminToken)

	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()
	state.Inner.DockerSecret = []byte("api-test-docker-secret-32-bytes")
	state.Inner.DB = db

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
		},
		StoragePath: dir,
		Maven: config.MavenSettings{
			Repositories: map[string]*config.Repository{
				"docker-pub": {
					Name:       "docker-pub",
					Format:     config.RepositoryFormatDocker,
					Visibility: "PUBLIC",
				},
				"docker-priv": {
					Name:       "docker-priv",
					Format:     config.RepositoryFormatDocker,
					Visibility: "PRIVATE",
				},
				"maven-repo": {
					Name:       "maven-repo",
					Format:     config.RepositoryFormatMaven,
					Visibility: "PUBLIC",
				},
			},
		},
	}
	state.Inner.Config.Store(cfg)

	app := fiber.New()
	app.Use(auth.AuthMiddleware(state))

	app.Get("/api/docker/repositories/:repo_name/images", func(c fiber.Ctx) error {
		if c.Query("image") != "" {
			return GetDockerImageDetailsAPI(c, state)
		}
		return ListDockerImagesAPI(c, state)
	})
	app.Get("/api/docker/repositories/:repo_name/images/*", func(c fiber.Ctx) error {
		return GetDockerImageDetailsAPI(c, state)
	})
	app.Get("/api/docker/repositories/:repo_name/manifests", func(c fiber.Ctx) error {
		return GetDockerManifestAPI(c, state)
	})
	app.Get("/api/docker/repositories/:repo_name/manifests/*", func(c fiber.Ctx) error {
		return GetDockerManifestAPI(c, state)
	})
	app.Put("/api/docker/repositories/:repo_name/images", func(c fiber.Ctx) error {
		return UpdateDockerImageDescriptionAPI(c, state)
	})
	app.Put("/api/docker/repositories/:repo_name/images/*", func(c fiber.Ctx) error {
		return UpdateDockerImageDescriptionAPI(c, state)
	})
	app.Delete("/api/docker/repositories/:repo_name/images", func(c fiber.Ctx) error {
		return DeleteDockerImageAPI(c, state)
	})
	app.Delete("/api/docker/repositories/:repo_name/images/*", func(c fiber.Ctx) error {
		return DeleteDockerImageAPI(c, state)
	})
	app.Delete("/api/docker/repositories/:repo_name/tags", func(c fiber.Ctx) error {
		return DeleteDockerTagAPI(c, state)
	})
	app.Delete("/api/docker/repositories/:repo_name/tags/*", func(c fiber.Ctx) error {
		return DeleteDockerTagAPI(c, state)
	})

	return app, state
}

func TestDockerRESTAPIs(t *testing.T) {
	app, state := setupTestAPIDockerApp(t)
	db := state.GetDB()

	now := time.Now().Unix()
	_ = db.PutDockerManifest(&core.DockerManifest{
		Repository:   "docker-pub",
		ImageName:    "web/backend",
		Digest:       "sha256:1111222233334444555566667777888899990000111122223333444455556666",
		MediaType:    docker.MediaTypeDockerManifest2,
		Size:         2048,
		ConfigDigest: "sha256:config1111",
		CreatedAt:    now,
	}, "v1.0.0", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/docker/repositories/docker-pub/images", nil)
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("List images failed: %v (status: %d)", err, resp.StatusCode)
	}
	var listResp struct {
		Repository string                        `json:"repository"`
		Images     []*core.DockerRepositoryImage `json:"images"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&listResp)
	if len(listResp.Images) != 1 || listResp.Images[0].ImageName != "web/backend" {
		t.Fatalf("unexpected images list: %+v", listResp.Images)
	}
	if listResp.Images[0].TagCount != 1 || listResp.Images[0].LatestTag != "v1.0.0" {
		t.Fatalf("unexpected tag count or latest tag: %+v", listResp.Images[0])
	}

	privReq := httptest.NewRequest(http.MethodGet, "/api/docker/repositories/docker-priv/images", nil)
	privResp, _ := app.Test(privReq)
	if privResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for private repo list without auth, got %d", privResp.StatusCode)
	}

	privAuthReq := httptest.NewRequest(http.MethodGet, "/api/docker/repositories/docker-priv/images", nil)
	privAuthReq.Header.Set("Authorization", "Bearer admin-test-token")
	privAuthResp, _ := app.Test(privAuthReq)
	if privAuthResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for private repo list with admin token, got %d", privAuthResp.StatusCode)
	}

	badRepoReq := httptest.NewRequest(http.MethodGet, "/api/docker/repositories/maven-repo/images", nil)
	badRepoResp, _ := app.Test(badRepoReq)
	if badRepoResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for non-Docker repository format, got %d", badRepoResp.StatusCode)
	}

	detailsReq := httptest.NewRequest(http.MethodGet, "/api/docker/repositories/docker-pub/images/web/backend", nil)
	detailsResp, err := app.Test(detailsReq)
	if err != nil || detailsResp.StatusCode != http.StatusOK {
		t.Fatalf("Get image details failed: %v (status: %d)", err, detailsResp.StatusCode)
	}
	var details core.DockerImageDetails
	_ = json.NewDecoder(detailsResp.Body).Decode(&details)
	if details.Image == nil || details.Image.ImageName != "web/backend" || len(details.Tags) != 1 || details.Tags[0].Tag != "v1.0.0" {
		t.Fatalf("unexpected image details: %+v", details)
	}
	if details.Image.Publisher != "admin" {
		t.Fatalf("expected details image publisher to be 'admin', got '%s'", details.Image.Publisher)
	}

	detailsQueryReq := httptest.NewRequest(http.MethodGet, "/api/docker/repositories/docker-pub/images?image=web/backend", nil)
	detailsQueryResp, err := app.Test(detailsQueryReq)
	if err != nil || detailsQueryResp.StatusCode != http.StatusOK {
		t.Fatalf("Get image details via query param failed: %v (status: %d)", err, detailsQueryResp.StatusCode)
	}
	var detailsQuery core.DockerImageDetails
	_ = json.NewDecoder(detailsQueryResp.Body).Decode(&detailsQuery)
	if detailsQuery.Image == nil || len(detailsQuery.Tags) != 1 || detailsQuery.Tags[0].Tag != "v1.0.0" {
		t.Fatalf("unexpected image details from query param: %+v", detailsQuery)
	}

	manifestReq := httptest.NewRequest(http.MethodGet, "/api/docker/repositories/docker-pub/manifests/web/backend/v1.0.0", nil)
	manifestResp, err := app.Test(manifestReq)
	if err != nil || manifestResp.StatusCode != http.StatusOK {
		t.Fatalf("Get manifest failed: %v (status: %d)", err, manifestResp.StatusCode)
	}
	var manifestData map[string]any
	_ = json.NewDecoder(manifestResp.Body).Decode(&manifestData)
	if manifestData["image_name"] != "web/backend" || manifestData["digest"] != "sha256:1111222233334444555566667777888899990000111122223333444455556666" {
		t.Fatalf("unexpected manifest payload: %+v", manifestData)
	}
	if manifestData["publisher"] != "admin" {
		t.Fatalf("expected manifest publisher to be 'admin', got '%v'", manifestData["publisher"])
	}

	manifestQueryReq := httptest.NewRequest(http.MethodGet, "/api/docker/repositories/docker-pub/manifests?image=web/backend&ref=v1.0.0", nil)
	manifestQueryResp, err := app.Test(manifestQueryReq)
	if err != nil || manifestQueryResp.StatusCode != http.StatusOK {
		t.Fatalf("Get manifest with query params failed: %v (status: %d)", err, manifestQueryResp.StatusCode)
	}

	updateDescReq := httptest.NewRequest(http.MethodPut, "/api/docker/repositories/docker-pub/images/web/backend", strings.NewReader(`{"description":"# Web Backend Service\n\nRuns production workloads."}`))
	updateDescReq.Header.Set("Content-Type", "application/json")
	updateDescReq.Header.Set("Authorization", "Bearer admin-test-token")
	updateDescResp, err := app.Test(updateDescReq)
	if err != nil || updateDescResp.StatusCode != http.StatusOK {
		t.Fatalf("Update image description failed: %v (status: %d)", err, updateDescResp.StatusCode)
	}

	detailsWithDescReq := httptest.NewRequest(http.MethodGet, "/api/docker/repositories/docker-pub/images/web%2Fbackend", nil)
	detailsWithDescResp, err := app.Test(detailsWithDescReq)
	if err != nil || detailsWithDescResp.StatusCode != http.StatusOK {
		t.Fatalf("Get image details with encoded name failed: %v (status: %d)", err, detailsWithDescResp.StatusCode)
	}
	var detailsWithDesc core.DockerImageDetails
	_ = json.NewDecoder(detailsWithDescResp.Body).Decode(&detailsWithDesc)
	if detailsWithDesc.Image == nil || detailsWithDesc.Image.Description != "# Web Backend Service\n\nRuns production workloads." {
		t.Fatalf("expected updated description in details, got %+v", detailsWithDesc.Image)
	}

	delTagReq := httptest.NewRequest(http.MethodDelete, "/api/docker/repositories/docker-pub/tags/web/backend/v1.0.0", nil)
	delTagReq.Header.Set("Authorization", "Bearer admin-test-token")
	delTagResp, err := app.Test(delTagReq)
	if err != nil || delTagResp.StatusCode != http.StatusOK {
		t.Fatalf("Delete tag failed: %v (status: %d)", err, delTagResp.StatusCode)
	}

	tagsAfter, _ := db.ListDockerTags("docker-pub", "web/backend", "", 10)
	if len(tagsAfter) != 0 {
		t.Fatalf("expected 0 tags after delete, got %d", len(tagsAfter))
	}

	delImageReq := httptest.NewRequest(http.MethodDelete, "/api/docker/repositories/docker-pub/images/web/backend", nil)
	delImageReq.Header.Set("Authorization", "Bearer admin-test-token")
	delImageResp, err := app.Test(delImageReq)
	if err != nil || delImageResp.StatusCode != http.StatusOK {
		t.Fatalf("Delete image failed: %v (status: %d)", err, delImageResp.StatusCode)
	}

	detailsAfter, err := db.GetDockerImageDetails("docker-pub", "web/backend")
	if err != nil || detailsAfter != nil {
		t.Fatalf("expected nil details after image deletion, got %+v (err: %v)", detailsAfter, err)
	}

	delAgainReq := httptest.NewRequest(http.MethodDelete, "/api/docker/repositories/docker-pub/images/web/backend", nil)
	delAgainReq.Header.Set("Authorization", "Bearer admin-test-token")
	delAgainResp, _ := app.Test(delAgainReq)
	if delAgainResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when deleting non-existent image, got %d", delAgainResp.StatusCode)
	}
}

func TestDockerRESTAPIValidationErrors(t *testing.T) {
	app, _ := setupTestAPIDockerApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/docker/repositories/%2e%2e/images", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for invalid repo name, got %d", resp.StatusCode)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/docker/repositories/docker-pub/images/non-existent-image", nil)
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for non-existent image, got %d", resp2.StatusCode)
	}

	req3 := httptest.NewRequest(http.MethodDelete, "/api/docker/repositories/docker-pub/images/test-app", nil)
	resp3, _ := app.Test(req3)
	if resp3.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for unauthorized delete, got %d", resp3.StatusCode)
	}
}
