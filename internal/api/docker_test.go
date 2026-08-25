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
	"github.com/stretchr/testify/require"

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
	app.Post("/api/docker/repositories/:repo_name/images", func(c fiber.Ctx) error {
		return CreateDockerImageAPI(c, state)
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
	app.Get("/api/docker/repositories/:repo_name/owners", func(c fiber.Ctx) error {
		return ListDockerOwnersAPI(c, state)
	})
	app.Post("/api/docker/repositories/:repo_name/owners", func(c fiber.Ctx) error {
		return InviteDockerOwnersAPI(c, state)
	})
	app.Put("/api/docker/repositories/:repo_name/owners/:username", func(c fiber.Ctx) error {
		return SetDockerOwnerLevelAPI(c, state)
	})
	app.Delete("/api/docker/repositories/:repo_name/owners/:username", func(c fiber.Ctx) error {
		return RemoveDockerOwnerAPI(c, state)
	})
	app.Get("/api/docker/repositories/:repo_name/users/search", func(c fiber.Ctx) error {
		return SearchDockerUsersAPI(c, state)
	})
	app.Post("/api/docker/repositories/:repo_name/invitations/:id/:decision", func(c fiber.Ctx) error {
		return RespondDockerInvitationAPI(c, state)
	})

	return app, state
}

func TestCreateDockerImageRejectsLocalAndUpstreamNameConflicts(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/taken/tags/list":
			w.WriteHeader(http.StatusOK)
		case "/v2/available/tags/list":
			http.NotFound(w, r)
		case "/v2/broken/tags/list":
			w.WriteHeader(http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)

	app, state := setupTestAPIDockerApp(t)
	cfg := state.Inner.Config.Load().DeepCopy()
	cfg.Maven.Repositories["docker-pub"].Mirrors = []config.Mirror{{Url: upstream.URL, TimeoutSecs: 5}}
	state.Inner.Config.Store(cfg)

	create := func(image string) int {
		request := httptest.NewRequest(http.MethodPost, "/api/docker/repositories/docker-pub/images",
			strings.NewReader(`{"image":"`+image+`"}`))
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		request.SetBasicAuth("admin", "admin-test-token")
		response, err := app.Test(request)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		return response.StatusCode
	}

	require.Equal(t, http.StatusConflict, create("taken"))
	require.Equal(t, http.StatusCreated, create("AVAILABLE"))
	require.Equal(t, http.StatusConflict, create("available"))
	require.Equal(t, http.StatusServiceUnavailable, create("broken"))
}

func TestDockerRESTAPIs(t *testing.T) {
	app, state := setupTestAPIDockerApp(t)
	db := state.GetDB()

	bobToken := &core.AccessToken{
		Name:        "bob",
		Tokens:      []string{"bob-test-token"},
		Permissions: []string{"read"},
	}
	_ = db.SaveToken(bobToken)
	createReq := httptest.NewRequest(http.MethodPost, "/api/docker/repositories/docker-pub/images",
		strings.NewReader(`{"image":"private/empty","private":true}`))
	createReq.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	createReq.SetBasicAuth("admin", "admin-test-token")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	var createdImage core.DockerRepositoryImage
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&createdImage))
	require.NoError(t, createResp.Body.Close())
	require.True(t, createdImage.Private)
	duplicateReq := httptest.NewRequest(http.MethodPost, "/api/docker/repositories/docker-pub/images",
		strings.NewReader(`{"image":"private/empty","private":false}`))
	duplicateReq.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	duplicateReq.SetBasicAuth("admin", "admin-test-token")
	duplicateResp, err := app.Test(duplicateReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, duplicateResp.StatusCode)
	require.NoError(t, duplicateResp.Body.Close())
	unauthorizedCreateReq := httptest.NewRequest(http.MethodPost, "/api/docker/repositories/docker-pub/images",
		strings.NewReader(`{"image":"bob/app"}`))
	unauthorizedCreateReq.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	unauthorizedCreateReq.SetBasicAuth("bob", "bob-test-token")
	unauthorizedCreateResp, err := app.Test(unauthorizedCreateReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, unauthorizedCreateResp.StatusCode)
	require.NoError(t, unauthorizedCreateResp.Body.Close())

	grantReadReq := httptest.NewRequest(http.MethodPost,
		"/api/docker/repositories/docker-pub/owners?image=private/empty",
		strings.NewReader(`{"users":["bob"],"level":0}`))
	grantReadReq.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	grantReadReq.SetBasicAuth("admin", "admin-test-token")
	grantReadResp, err := app.Test(grantReadReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, grantReadResp.StatusCode)
	require.NoError(t, grantReadResp.Body.Close())

	privateDetailsReq := httptest.NewRequest(http.MethodGet,
		"/api/docker/repositories/docker-pub/images?image=private/empty", nil)
	privateDetailsReq.SetBasicAuth("bob", "bob-test-token")
	privateDetailsResp, err := app.Test(privateDetailsReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, privateDetailsResp.StatusCode)
	require.NoError(t, privateDetailsResp.Body.Close())

	now := time.Now().Unix()
	_, _ = db.CreateDockerImage("docker-pub", "web/backend", "admin", false, 1_700_000_000_000)
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

	inviteReq := httptest.NewRequest(http.MethodPost, "/api/docker/repositories/docker-pub/owners?image=web/backend", strings.NewReader(`{"users":["bob"],"level":2}`))
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteReq.Header.Set("Authorization", "Bearer admin-test-token")
	inviteResp, err := app.Test(inviteReq)
	if err != nil || inviteResp.StatusCode != http.StatusOK {
		t.Fatalf("Invite owner failed: %v (status: %d)", err, inviteResp.StatusCode)
	}

	searchUserReq := httptest.NewRequest(http.MethodGet, "/api/docker/repositories/docker-pub/users/search?q=bo", nil)
	searchUserReq.Header.Set("Authorization", "Bearer admin-test-token")
	searchUserResp, err := app.Test(searchUserReq)
	if err != nil || searchUserResp.StatusCode != http.StatusOK {
		t.Fatalf("Search users failed: %v (status: %d)", err, searchUserResp.StatusCode)
	}
	var searchUserResult struct {
		Users []string `json:"users"`
	}
	_ = json.NewDecoder(searchUserResp.Body).Decode(&searchUserResult)
	if len(searchUserResult.Users) != 1 || searchUserResult.Users[0] != "bob" {
		t.Fatalf("unexpected user search result: %+v", searchUserResult)
	}

	ownersReq := httptest.NewRequest(http.MethodGet, "/api/docker/repositories/docker-pub/owners?image=web/backend", nil)
	ownersResp, err := app.Test(ownersReq)
	if err != nil || ownersResp.StatusCode != http.StatusOK {
		t.Fatalf("List owners failed: %v (status: %d)", err, ownersResp.StatusCode)
	}
	var ownersResult struct {
		Users []*core.DockerMember `json:"users"`
	}
	_ = json.NewDecoder(ownersResp.Body).Decode(&ownersResult)
	if len(ownersResult.Users) != 2 {
		t.Fatalf("expected 2 members (admin + bob), got %d", len(ownersResult.Users))
	}
	bobUserID := ""
	adminUserID := ""
	for _, member := range ownersResult.Users {
		switch member.Username {
		case "bob":
			bobUserID = member.UserID
		case "admin":
			adminUserID = member.UserID
		}
	}
	if bobUserID == "" || adminUserID == "" {
		t.Fatalf("expected immutable member IDs, got %+v", ownersResult.Users)
	}

	overwriteReq := httptest.NewRequest(http.MethodPost, "/api/docker/repositories/docker-pub/owners?image=web/backend", strings.NewReader(`{"users":["bob"],"level":1}`))
	overwriteReq.Header.Set("Content-Type", "application/json")
	overwriteReq.Header.Set("Authorization", "Bearer admin-test-token")
	overwriteResp, err := app.Test(overwriteReq)
	if err != nil || overwriteResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected force overwrite of an existing member to return 409, got %d (err: %v)", overwriteResp.StatusCode, err)
	}

	ownerLeaveReq := httptest.NewRequest(http.MethodDelete, "/api/docker/repositories/docker-pub/owners/"+adminUserID+"?image=web/backend", nil)
	ownerLeaveReq.Header.Set("Authorization", "Bearer admin-test-token")
	ownerLeaveResp, err := app.Test(ownerLeaveReq)
	if err != nil || ownerLeaveResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected L4 owner self-removal to return 400, got %d (err: %v)", ownerLeaveResp.StatusCode, err)
	}

	setLvl3Req := httptest.NewRequest(http.MethodPut, "/api/docker/repositories/docker-pub/owners/"+bobUserID+"?image=web/backend", strings.NewReader(`{"level":3}`))
	setLvl3Req.Header.Set("Content-Type", "application/json")
	setLvl3Req.Header.Set("Authorization", "Bearer admin-test-token")
	setLvl3Resp, err := app.Test(setLvl3Req)
	if err != nil || setLvl3Resp.StatusCode != http.StatusOK {
		t.Fatalf("Set bob to L3 failed: %v (status: %d)", err, setLvl3Resp.StatusCode)
	}

	_ = db.SaveToken(&core.AccessToken{Name: "carol", Tokens: []string{"carol-test-token"}, Identifier: core.AccessTokenIdentifier{Type: core.Persistent}})
	inviteCarolReq := httptest.NewRequest(http.MethodPost, "/api/docker/repositories/docker-pub/owners?image=web/backend", strings.NewReader(`{"users":["carol"],"level":1}`))
	inviteCarolReq.Header.Set("Content-Type", "application/json")
	inviteCarolReq.Header.Set("Authorization", "Bearer bob-test-token")
	inviteCarolResp, err := app.Test(inviteCarolReq)
	if err != nil || inviteCarolResp.StatusCode != http.StatusOK {
		t.Fatalf("Bob invite carol failed: %v (status: %d)", err, inviteCarolResp.StatusCode)
	}

	messages, err := db.ListMessages("carol", 10, 0, "", time.Now().UnixMilli()+1000)
	if err != nil || len(messages) == 0 {
		t.Fatalf("expected invitation message for carol, got err: %v, msgs: %+v", err, messages)
	}
	invID := messages[0].ID

	acceptReq := httptest.NewRequest(http.MethodPost, "/api/docker/repositories/docker-pub/invitations/"+invID+"/accept", nil)
	acceptReq.Header.Set("Authorization", "Bearer carol-test-token")
	acceptResp, err := app.Test(acceptReq)
	if err != nil || acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("Accept invitation failed: %v (status: %d)", err, acceptResp.StatusCode)
	}

	setLevelReq := httptest.NewRequest(http.MethodPut, "/api/docker/repositories/docker-pub/owners/"+bobUserID+"?image=web/backend", strings.NewReader(`{"level":1}`))
	setLevelReq.Header.Set("Content-Type", "application/json")
	setLevelReq.Header.Set("Authorization", "Bearer admin-test-token")
	setLevelResp, err := app.Test(setLevelReq)
	if err != nil || setLevelResp.StatusCode != http.StatusOK {
		t.Fatalf("Set level failed: %v (status: %d)", err, setLevelResp.StatusCode)
	}

	removeReq := httptest.NewRequest(http.MethodDelete, "/api/docker/repositories/docker-pub/owners/"+bobUserID+"?image=web/backend", nil)
	removeReq.Header.Set("Authorization", "Bearer bob-test-token")
	removeResp, err := app.Test(removeReq)
	if err != nil || removeResp.StatusCode != http.StatusOK {
		t.Fatalf("Remove owner failed: %v (status: %d)", err, removeResp.StatusCode)
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

	invalidCreate := httptest.NewRequest(http.MethodPost, "/api/docker/repositories/docker-pub/images",
		strings.NewReader(`{"image":"invalid:tag"}`))
	invalidCreate.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	invalidCreate.SetBasicAuth("admin", "admin-test-token")
	invalidCreateResponse, err := app.Test(invalidCreate)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, invalidCreateResponse.StatusCode)
	require.NoError(t, invalidCreateResponse.Body.Close())

	largeCreate := httptest.NewRequest(http.MethodPost, "/api/docker/repositories/docker-pub/images",
		strings.NewReader(`{"image":"`+strings.Repeat("a", 2048)+`"}`))
	largeCreate.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	largeCreate.SetBasicAuth("admin", "admin-test-token")
	largeCreateResponse, err := app.Test(largeCreate)
	require.NoError(t, err)
	require.Equal(t, http.StatusRequestEntityTooLarge, largeCreateResponse.StatusCode)
	require.NoError(t, largeCreateResponse.Body.Close())
}
