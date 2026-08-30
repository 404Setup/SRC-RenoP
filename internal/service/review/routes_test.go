/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package review

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	cargoservice "renop/internal/service/cargo"
	"renop/internal/service/index"
	npmservice "renop/internal/service/npm"
)

func setupReviewApp(t *testing.T) (*fiber.App, *core.AppState, *config.User, *string) {
	t.Helper()
	storagePath := t.TempDir()
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "reviews.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, username := range []string{"alice", "bob", "charlie", "dana"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{
			Name: username, CreatedAt: time.Now().Format(time.RFC3339),
		}))
	}
	now := time.Now().UnixMilli()
	require.NoError(t, db.CreateSuperTeam(&core.SuperTeam{
		Prefix: "platform", Name: "Platform", CreatedAt: now,
	}, "alice", 5, 10))
	require.NoError(t, db.ForceAddSuperTeamMembers("platform", "alice", []string{"bob"},
		core.SuperTeamRoleManage, 5, 10, now+1))
	require.NoError(t, db.ForceAddSuperTeamMembers("platform", "alice", []string{"charlie"},
		core.SuperTeamRoleRead, 5, 10, now+2))
	_, err = db.CreateDockerImage("containers", "personal", "charlie", false, now+3)
	require.NoError(t, err)
	state := core.NewAppState()
	state.Inner.DB = db
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"releases": {Name: "releases", Format: config.RepositoryFormatMaven, Visibility: "PUBLIC"},
		"npm":      {Name: "npm", Format: config.RepositoryFormatNPM, Visibility: "PUBLIC"},
		"cargo":    {Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC"},
	}
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndexCustom(true)
	current := &config.User{Username: "charlie", Roles: []string{"base"}}
	credentialKind := "session"
	app := fiber.New(fiber.Config{JSONEncoder: json.Marshal, JSONDecoder: json.Unmarshal})
	t.Cleanup(func() { require.NoError(t, app.Shutdown()) })
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", current)
		c.Locals("auth_credential_kind", credentialKind)
		return c.Next()
	})
	SetupRoutes(app.Group("/api"), state)
	return app, state, current, &credentialKind
}

func reviewRequest(t *testing.T, app *fiber.App, method, path string, body any) *http.Response {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}
	response, err := app.Test(request)
	require.NoError(t, err)
	return response
}

func TestReviewRoutesCreateListAndSingleDecision(t *testing.T) {
	app, state, current, _ := setupReviewApp(t)
	response := reviewRequest(t, app, http.MethodPost, "/api/reviews/super-team-transfers",
		core.SuperTeamTransferRequest{
			ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
			ResourceKey: "personal", TargetTeamPrefix: "platform",
		})
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var task core.ReviewTask
	require.NoError(t, json.NewDecoder(response.Body).Decode(&task))
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "charlie", task.RequestedBy)
	assert.Equal(t, "platform", task.ReviewTeamPrefix)

	*current = config.User{Username: "bob", Roles: []string{"base"}}
	response = reviewRequest(t, app, http.MethodGet,
		"/api/reviews?view=reviewer&status=pending&types=docker_image&limit=10&offset=0", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var page struct {
		Tasks []*core.ReviewTask `json:"tasks"`
		Total int                `json:"total"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&page))
	require.NoError(t, response.Body.Close())
	require.Len(t, page.Tasks, 1)
	assert.Equal(t, 1, page.Total)

	*current = config.User{Username: "dana", Roles: []string{"base"}}
	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+task.ID+"/decision",
		decisionRequest{Decision: core.ReviewStatusApproved})
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	assert.Equal(t, "review_permission", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())

	*current = config.User{Username: "bob", Roles: []string{"base"}}
	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+task.ID+"/decision",
		decisionRequest{Decision: core.ReviewStatusApproved})
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	image, err := state.GetDB().GetDockerImage("containers", "personal")
	require.NoError(t, err)
	assert.Equal(t, "platform", image.SuperTeamPrefix)

	*current = config.User{Username: "alice", Roles: []string{"base"}}
	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+task.ID+"/decision",
		decisionRequest{Decision: core.ReviewStatusRejected, Reason: "Already handled"})
	require.Equal(t, http.StatusConflict, response.StatusCode)
	assert.Equal(t, "review_decided", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())
}

func TestReviewRoutesRejectInvalidFiltersAndEmptyRejectionReasons(t *testing.T) {
	app, _, current, _ := setupReviewApp(t)
	response := reviewRequest(t, app, http.MethodGet,
		"/api/reviews?view=reviewer&status=unknown&limit=10&offset=0", nil)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.Equal(t, "invalid_request", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())

	response = reviewRequest(t, app, http.MethodGet,
		"/api/reviews?view=reviewer&types=unknown&limit=10&offset=0", nil)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())

	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/super-team-transfers",
		core.SuperTeamTransferRequest{
			ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
			ResourceKey: "personal", TargetTeamPrefix: "platform",
		})
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var task core.ReviewTask
	require.NoError(t, json.NewDecoder(response.Body).Decode(&task))
	require.NoError(t, response.Body.Close())

	*current = config.User{Username: "bob", Roles: []string{"base"}}
	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+task.ID+"/decision",
		decisionRequest{Decision: core.ReviewStatusRejected})
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.Equal(t, "invalid_request", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())
}

func TestReviewRoutesRequireBrowserSession(t *testing.T) {
	app, _, _, credentialKind := setupReviewApp(t)
	for _, kind := range []string{"api_token", "password"} {
		*credentialKind = kind
		response := reviewRequest(t, app, http.MethodGet,
			"/api/reviews?view=requested&status=pending&limit=10&offset=0", nil)
		require.Equal(t, http.StatusForbidden, response.StatusCode, kind)
		assert.Equal(t, "review_permission", response.Header.Get("X-Renop-Error-Code"), kind)
		require.NoError(t, response.Body.Close())
	}
}

func TestRepositoryModeratorListsDownloadsAndRejectsPublication(t *testing.T) {
	app, state, current, _ := setupReviewApp(t)
	require.NoError(t, state.GetDB().SaveToken(&core.AccessToken{
		Name: "bob", CreatedAt: time.Now().Format(time.RFC3339),
		Permissions: []string{"base", "canmoderate:releases"},
	}))
	now := time.Now().UnixMilli() - core.PublicationReviewSettleMillis - 1000
	relative := "com/example/demo/1.0.0/demo-1.0.0.jar"
	absolute := filepath.Join(state.Inner.Config.Load().StoragePath, "releases", filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(absolute), 0755))
	require.NoError(t, os.WriteFile(absolute, []byte("review artifact"), 0644))
	state.Inner.FileIndex.InsertFile(absolute, index.FileInfo{Size: 15, ModTime: time.Now().UnixNano()})
	result, err := state.GetDB().CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceMavenArtifact, Repository: "releases",
		ResourceKey: "com.example:demo", ResourceName: "com.example:demo", Version: "1.0.0",
		RequestedBy: "charlie", Policy: config.PublicationReviewEveryVersion, CreatedAt: now,
		Files: []*core.ReviewFile{{Path: relative, Size: 15, Critical: true, AddedAt: now}},
	})
	require.NoError(t, err)
	state.Inner.FileIndex.BlockFile(absolute)
	*current = config.User{Username: "bob", Roles: []string{"base", "canmoderate:releases"}}

	response := reviewRequest(t, app, http.MethodGet,
		"/api/reviews?view=reviewer&status=pending&types=maven_artifact&limit=10&offset=0", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var page struct {
		Tasks []*core.ReviewTask `json:"tasks"`
		Total int                `json:"total"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&page))
	require.NoError(t, response.Body.Close())
	require.Equal(t, 1, page.Total)
	require.Len(t, page.Tasks, 1)
	assert.Equal(t, result.TaskID, page.Tasks[0].ID)

	response = reviewRequest(t, app, http.MethodGet, "/api/reviews/"+result.TaskID+"/files", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var filesPayload struct {
		Files []*core.ReviewFile `json:"files"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&filesPayload))
	require.NoError(t, response.Body.Close())
	require.Len(t, filesPayload.Files, 1)
	response = reviewRequest(t, app, http.MethodGet,
		"/api/reviews/"+result.TaskID+"/files/"+filesPayload.Files[0].ID, nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "review artifact", string(body))

	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+result.TaskID+"/decision",
		decisionRequest{Decision: core.ReviewStatusRejected, ReasonCode: "custom"})
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())
	_, err = os.Stat(absolute)
	require.NoError(t, err)

	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+result.TaskID+"/decision",
		decisionRequest{Decision: core.ReviewStatusRejected, ReasonCode: "quality"})
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	_, err = os.Stat(absolute)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRepositoryModeratorApprovalPublishesMavenCatalogBeforeFiles(t *testing.T) {
	app, state, current, _ := setupReviewApp(t)
	require.NoError(t, state.GetDB().SaveToken(&core.AccessToken{
		Name: "bob", CreatedAt: time.Now().Format(time.RFC3339),
		Permissions: []string{"base", "canmoderate:releases"},
	}))
	now := time.Now().UnixMilli() - core.PublicationReviewSettleMillis - 1000
	require.NoError(t, state.GetDB().CreateMavenDomain(&core.MavenDomain{
		Domain: "org.example", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "example.org", VerificationCode: "review-code",
		Verified: true, CreatedAt: now, VerifiedAt: now,
	}, "alice"))
	relative := "org/example/demo/1.0.0/demo-1.0.0.jar"
	absolute := filepath.Join(state.Inner.Config.Load().StoragePath, "releases", filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(absolute), 0755))
	require.NoError(t, os.WriteFile(absolute, []byte("approved artifact"), 0644))
	result, err := state.GetDB().CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceMavenArtifact, Repository: "releases",
		ResourceKey: "org.example:demo", ResourceName: "org.example:demo", Version: "1.0.0",
		RequestedBy: "charlie", Policy: config.PublicationReviewEveryVersion, CreatedAt: now,
		Files: []*core.ReviewFile{{Path: relative, Size: 17, Critical: true, AddedAt: now}},
	})
	require.NoError(t, err)
	state.Inner.FileIndex.BlockFile(absolute)
	*current = config.User{Username: "bob", Roles: []string{"base", "canmoderate:releases"}}
	response := reviewRequest(t, app, http.MethodPost, "/api/reviews/"+result.TaskID+"/decision",
		decisionRequest{Decision: core.ReviewStatusApproved})
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	details, err := state.GetDB().GetMavenArtifactDetails("releases", "org.example", "demo")
	require.NoError(t, err)
	require.Len(t, details.Versions, 1)
	assert.Equal(t, "1.0.0", details.Versions[0].Version)
	assert.False(t, state.Inner.FileIndex.IsBlocked(absolute))
	assert.True(t, state.Inner.FileIndex.HasFile(absolute))
}

func TestRepositoryModeratorApprovalPublishesNPMVersionBeforeTarball(t *testing.T) {
	app, state, current, _ := setupReviewApp(t)
	require.NoError(t, state.GetDB().SaveToken(&core.AccessToken{
		Name: "bob", CreatedAt: time.Now().Format(time.RFC3339),
		Permissions: []string{"base", "canmoderate:npm"},
	}))
	now := time.Now().UnixMilli() - core.PublicationReviewSettleMillis - 1000
	_, err := state.GetDB().CreateNPMPackage("npm", "reviewed", "alice", false, now)
	require.NoError(t, err)
	repo := state.Inner.Config.Load().Maven.Repositories["npm"]
	repo.PublicationReview = config.PublicationReviewEveryVersion
	tarballPath := "reviewed/-/reviewed-1.0.0.tgz"
	absolute := filepath.Join(state.Inner.Config.Load().StoragePath, "npm", filepath.FromSlash(tarballPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(absolute), 0755))
	require.NoError(t, os.WriteFile(absolute, []byte("npm tarball"), 0644))
	result, err := npmservice.QueuePublicationReview(state, repo, &core.NPMPackage{
		Repository: "npm", Name: "reviewed", Description: "Reviewed npm package", UpdatedAt: now,
	}, &core.NPMVersion{
		Repository: "npm", Package: "reviewed", Version: "1.0.0",
		ManifestJSON: `{"name":"reviewed","version":"1.0.0"}`, Publisher: "alice",
		TarballPath: tarballPath, Size: 11, CreatedAt: now,
	}, map[string]string{"latest": "1.0.0"}, false)
	require.NoError(t, err)
	state.Inner.FileIndex.BlockFile(absolute)
	*current = config.User{Username: "bob", Roles: []string{"base", "canmoderate:npm"}}
	response := reviewRequest(t, app, http.MethodPost, "/api/reviews/"+result.TaskID+"/decision",
		decisionRequest{Decision: core.ReviewStatusApproved})
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	details, err := state.GetDB().GetNPMPackageDetails("npm", "reviewed", "alice")
	require.NoError(t, err)
	require.Len(t, details.Versions, 1)
	assert.Equal(t, "1.0.0", details.Versions[0].Version)
	assert.False(t, state.Inner.FileIndex.IsBlocked(absolute))
	assert.True(t, state.Inner.FileIndex.HasFile(absolute))
}

func TestRepositoryModeratorApprovalPublishesCargoIndexBeforeCrate(t *testing.T) {
	app, state, current, _ := setupReviewApp(t)
	require.NoError(t, state.GetDB().SaveToken(&core.AccessToken{
		Name: "bob", CreatedAt: time.Now().Format(time.RFC3339),
		Permissions: []string{"base", "canmoderate:cargo"},
	}))
	now := time.Now().UnixMilli() - core.PublicationReviewSettleMillis - 1000
	repo := state.Inner.Config.Load().Maven.Repositories["cargo"]
	repo.PublicationReview = config.PublicationReviewEveryVersion
	cratePath := "api/v1/crates/reviewed/1.0.0/download"
	absolute := filepath.Join(state.Inner.Config.Load().StoragePath, "cargo", filepath.FromSlash(cratePath))
	require.NoError(t, os.MkdirAll(filepath.Dir(absolute), 0755))
	require.NoError(t, os.WriteFile(absolute, []byte("Cargo archive"), 0644))
	state.Inner.FileIndex.InsertFile(absolute, index.FileInfo{Size: 13, ModTime: time.Now().UnixNano()})
	result, err := cargoservice.QueuePublicationReview(state, repo, &core.CargoPackage{
		Repository: "cargo", Name: "reviewed", NormalizedName: "reviewed",
		Description: "Reviewed Cargo package", CreatedAt: now, UpdatedAt: now,
	}, &core.CargoVersion{
		Repository: "cargo", Package: "reviewed", Version: "1.0.0", Publisher: "charlie",
		Checksum: "0123456789abcdef", Size: 13, CreatedAt: now,
	}, cargoservice.IndexEntry{
		Name: "reviewed", Version: "1.0.0", Checksum: "0123456789abcdef",
		Deps: []cargoservice.IndexDependency{}, Features: map[string][]string{},
	}, cratePath, false)
	require.NoError(t, err)
	state.Inner.FileIndex.BlockFile(absolute)
	*current = config.User{Username: "bob", Roles: []string{"base", "canmoderate:cargo"}}
	response := reviewRequest(t, app, http.MethodPost, "/api/reviews/"+result.TaskID+"/decision",
		decisionRequest{Decision: core.ReviewStatusApproved})
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	details, err := state.GetDB().GetCargoPackageDetails("cargo", "reviewed", "charlie")
	require.NoError(t, err)
	require.Len(t, details.Versions, 1)
	assert.Equal(t, "1.0.0", details.Versions[0].Version)
	indexPath := filepath.Join(state.Inner.Config.Load().StoragePath, "cargo", "re", "vi", "reviewed")
	indexContents, err := os.ReadFile(indexPath)
	require.NoError(t, err)
	assert.Contains(t, string(indexContents), `"vers":"1.0.0"`)
	assert.False(t, state.Inner.FileIndex.IsBlocked(absolute))
	assert.True(t, state.Inner.FileIndex.HasFile(absolute))
}

func TestSystemAdministratorListsAndDecidesTeamReview(t *testing.T) {
	app, state, current, _ := setupReviewApp(t)
	response := reviewRequest(t, app, http.MethodPost, "/api/reviews/super-team-transfers",
		core.SuperTeamTransferRequest{
			ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
			ResourceKey: "personal", TargetTeamPrefix: "platform",
		})
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var task core.ReviewTask
	require.NoError(t, json.NewDecoder(response.Body).Decode(&task))
	require.NoError(t, response.Body.Close())
	now := time.Now().UnixMilli()
	_, err := state.GetDB().CreateDockerImage("containers", "reject-review", "charlie", false, now)
	require.NoError(t, err)
	rejectTask, err := state.GetDB().CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
		ResourceKey: "reject-review", TargetTeamPrefix: "platform",
	}, "charlie", false, now+1)
	require.NoError(t, err)
	require.NoError(t, state.GetDB().SaveToken(&core.AccessToken{
		Name: "dana", CreatedAt: time.Now().Format(time.RFC3339), Permissions: []string{"base", "manager"},
	}))
	*current = config.User{Username: "dana", Roles: []string{"manager"}}

	response = reviewRequest(t, app, http.MethodGet,
		"/api/reviews?view=reviewer&status=pending&limit=10&offset=0", nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var page struct {
		Tasks []*core.ReviewTask `json:"tasks"`
		Total int                `json:"total"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&page))
	require.NoError(t, response.Body.Close())
	require.Equal(t, 2, page.Total)
	require.Len(t, page.Tasks, 2)

	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+task.ID+"/decision",
		decisionRequest{Decision: core.ReviewStatusApproved})
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = reviewRequest(t, app, http.MethodPost, "/api/reviews/"+rejectTask.ID+"/decision",
		decisionRequest{Decision: core.ReviewStatusRejected, Reason: "Not approved"})
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
}
