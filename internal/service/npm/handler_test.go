/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/index"
)

type memoryStore struct {
	mu    sync.Mutex
	files map[string][]byte
}

type memoryStagedFile struct {
	store     *memoryStore
	target    string
	buffer    bytes.Buffer
	committed bool
}

func newMemoryStore() *memoryStore {
	return &memoryStore{files: make(map[string][]byte)}
}

func (store *memoryStore) Open(path string) (io.ReadCloser, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	data, exists := store.files[path]
	if !exists {
		return nil, false, nil
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), data...))), true, nil
}

func (store *memoryStore) Exists(path string) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	_, exists := store.files[path]
	return exists, nil
}

func (store *memoryStore) Stage(path string) (StagedFile, error) {
	return &memoryStagedFile{store: store, target: path}, nil
}

func (store *memoryStore) Delete(_ *core.AppState, path string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.files, path)
	return nil
}

func (staged *memoryStagedFile) Write(data []byte) (int, error) {
	return staged.buffer.Write(data)
}

func (staged *memoryStagedFile) Close() error {
	return nil
}

func (staged *memoryStagedFile) Open() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(staged.buffer.Bytes())), nil
}

func (staged *memoryStagedFile) Size() (int64, error) {
	return int64(staged.buffer.Len()), nil
}

func (staged *memoryStagedFile) Commit(_ *core.AppState) error {
	staged.store.mu.Lock()
	defer staged.store.mu.Unlock()
	staged.store.files[staged.target] = append([]byte(nil), staged.buffer.Bytes()...)
	staged.committed = true
	return nil
}

func (staged *memoryStagedFile) Discard() error {
	if !staged.committed {
		staged.buffer.Reset()
	}
	return nil
}

func npmTestTarball(t *testing.T, packageName, version string) []byte {
	t.Helper()
	manifest, err := json.Marshal(map[string]any{
		"name": packageName, "version": version, "description": "Test package",
	})
	require.NoError(t, err)
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{
		Name: "package/package.json", Mode: 0o644, Size: int64(len(manifest)), Typeflag: tar.TypeReg,
	}))
	_, err = tarWriter.Write(manifest)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	return buffer.Bytes()
}

func setupNPMTestApp(t *testing.T) (*fiber.App, *core.AppState, *memoryStore) {
	t.Helper()
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "npm-handler.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", Permissions: []string{"base", "canupdate:npm"}}))
	state := core.NewAppState()
	state.Inner.DB = db
	state.Inner.FileIndex = index.NewFileIndexCustom(true)
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	cfg.Maven.Repositories = map[string]*config.Repository{
		"npm": {Name: "npm", Format: config.RepositoryFormatNPM, Visibility: "PUBLIC", Mirrors: []config.Mirror{}},
	}
	state.Inner.Config.Store(cfg)
	_, err = db.CreateNPMPackage("npm", "demo", "alice", false, time.Now().UnixMilli())
	require.NoError(t, err)
	store := newMemoryStore()
	handler := Handler{Store: store}
	app := fiber.New(fiber.Config{StreamRequestBody: true, JSONEncoder: json.Marshal, JSONDecoder: json.Unmarshal})
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", &config.User{Username: "alice", Roles: []string{"base", "canupdate:npm"}})
		return c.Next()
	})
	app.All("/:repo/*", func(c fiber.Ctx) error {
		handled, err := handler.Handle(c, state, cfg.Maven.Repositories["npm"], cfg.StoragePath, c.Params("*"))
		if handled {
			return err
		}
		decoded, ok := decodeRegistryPath(c.Params("*"))
		if !ok {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		reader, found, openErr := store.Open(filepath.Join(cfg.StoragePath, "npm", filepath.FromSlash(decoded)))
		if openErr != nil || !found {
			return c.SendStatus(fiber.StatusNotFound)
		}
		return c.SendStream(reader)
	})
	return app, state, store
}

func TestNPMPackageCreationRequiresMatchingGlobalTeamScope(t *testing.T) {
	_, state, store := setupNPMTestApp(t)
	db := state.GetDB()
	now := time.Now().UnixMilli()
	require.NoError(t, db.CreateSuperTeam(&core.SuperTeam{
		Prefix: "platform", Name: "Platform", CreatedAt: now,
	}, "alice", 5, 10))
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "bob", CreatedAt: time.Now().Format(time.RFC3339),
		Permissions: []string{"base", "canupdate:npm"},
	}))
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "carol", CreatedAt: time.Now().Format(time.RFC3339),
		Permissions: []string{"base", "canupdate:npm"},
	}))
	require.NoError(t, db.ForceAddSuperTeamMembers("platform", "alice", []string{"bob"},
		core.SuperTeamRoleWrite, 5, 10, now+1))
	require.NoError(t, db.ForceAddSuperTeamMembers("platform", "alice", []string{"carol"},
		core.SuperTeamRoleRead, 5, 10, now+2))
	app := fiber.New(fiber.Config{JSONEncoder: json.Marshal, JSONDecoder: json.Unmarshal})
	current := &config.User{Username: "alice", Roles: []string{"base", "canupdate:npm"}}
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", current)
		return c.Next()
	})
	SetupRoutes(app.Group("/api"), state, store)
	create := func(body string) *http.Response {
		request := httptest.NewRequest(http.MethodPost, "/api/npm/repositories/npm/packages", bytes.NewBufferString(body))
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		response, err := app.Test(request)
		require.NoError(t, err)
		return response
	}
	response := create(`{"name":"@platform/tool","private":true}`)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.Equal(t, "super_team_required", response.Header.Get(npmAPIErrorCodeHeader))
	require.NoError(t, response.Body.Close())
	response = create(`{"name":"@platform/tool","super_team_prefix":"other","private":true}`)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.Equal(t, "super_team_mismatch", response.Header.Get(npmAPIErrorCodeHeader))
	require.NoError(t, response.Body.Close())
	response = create(`{"name":"@platform/tool","super_team_prefix":"platform","private":true}`)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var pkg core.NPMPackage
	require.NoError(t, json.NewDecoder(response.Body).Decode(&pkg))
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "platform", pkg.SuperTeamPrefix)
	response = create(`{"name":"standalone"}`)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	require.NoError(t, response.Body.Close())
	current = &config.User{Username: "bob", Roles: []string{"base", "canupdate:npm"}}
	response = create(`{"name":"@platform/reviewed","super_team_prefix":"platform","private":true}`)
	require.Equal(t, http.StatusAccepted, response.StatusCode)
	var queued struct {
		ReviewID string `json:"review_id"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&queued))
	require.NoError(t, response.Body.Close())
	require.NotEmpty(t, queued.ReviewID)
	task, err := db.GetReviewTask(queued.ReviewID)
	require.NoError(t, err)
	assert.Equal(t, "platform", task.ReviewTeamPrefix)
	assert.Equal(t, "platform", task.TargetTeamPrefix)
	pendingPackage, err := db.GetNPMPackage("npm", "@platform/reviewed")
	require.NoError(t, err)
	assert.Nil(t, pendingPackage)
	current = &config.User{Username: "carol", Roles: []string{"base", "canupdate:npm"}}
	response = create(`{"name":"@platform/denied","super_team_prefix":"platform"}`)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.Equal(t, "super_team_permission", response.Header.Get(npmAPIErrorCodeHeader))
	require.NoError(t, response.Body.Close())
}

func TestNPMPackageCreationPolicyRequiresReviewBeforeReservation(t *testing.T) {
	registryApp, state, store := setupNPMTestApp(t)
	repo := state.Inner.Config.Load().Maven.Repositories["npm"]
	repo.PublicationReview = config.PublicationReviewNewPackages
	app := fiber.New(fiber.Config{JSONEncoder: json.Marshal, JSONDecoder: json.Unmarshal})
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", &config.User{Username: "alice", Roles: []string{"base", "canupdate:npm"}})
		return c.Next()
	})
	SetupRoutes(app.Group("/api"), state, store)
	request := httptest.NewRequest(http.MethodPost, "/api/npm/repositories/npm/packages",
		bytes.NewBufferString(`{"name":"reviewed-package"}`))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, response.StatusCode)
	var queued struct {
		Pending  bool   `json:"pending"`
		ReviewID string `json:"review_id"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&queued))
	require.NoError(t, response.Body.Close())
	assert.True(t, queued.Pending)
	require.NotEmpty(t, queued.ReviewID)
	pkg, err := state.GetDB().GetNPMPackage("npm", "reviewed-package")
	require.NoError(t, err)
	assert.Nil(t, pkg)

	require.NoError(t, state.GetDB().SaveToken(&core.AccessToken{
		Name: "moderator", Permissions: []string{"base", "canmoderate:npm"},
	}))
	task, err := state.GetDB().GetReviewTask(queued.ReviewID)
	require.NoError(t, err)
	decided, err := ApprovePackageCreationReview(
		state, task, "moderator", task.UpdatedAt+core.PublicationReviewSettleMillis+1)
	require.NoError(t, err)
	assert.Equal(t, core.ReviewStatusApproved, decided.Status)
	pkg, err = state.GetDB().GetNPMPackage("npm", "reviewed-package")
	require.NoError(t, err)
	require.NotNil(t, pkg)
	assert.Equal(t, "alice", pkg.Publisher)
	tarball := npmTestTarball(t, "reviewed-package", "1.0.0")
	document := map[string]any{
		"_id": "reviewed-package", "name": "reviewed-package",
		"dist-tags": map[string]string{"latest": "1.0.0"},
		"versions": map[string]any{"1.0.0": map[string]any{
			"name": "reviewed-package", "version": "1.0.0",
		}},
		"_attachments": map[string]any{"reviewed-package-1.0.0.tgz": map[string]any{
			"length": len(tarball), "data": base64.StdEncoding.EncodeToString(tarball),
		}},
	}
	body, err := json.Marshal(document)
	require.NoError(t, err)
	request = httptest.NewRequest(http.MethodPut,
		"http://registry.example/npm/reviewed-package", bytes.NewReader(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err = registryApp.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, response.StatusCode)
	assert.Empty(t, response.Header.Get("X-RenoP-Review-ID"))
	require.NoError(t, response.Body.Close())
}

func TestNPMRegistryPublishInstallTagsAndDeprecation(t *testing.T) {
	app, state, store := setupNPMTestApp(t)
	tarball := npmTestTarball(t, "demo", "1.0.0")
	document := map[string]any{
		"_id": "demo", "name": "demo", "description": "Test package",
		"dist-tags": map[string]string{"latest": "1.0.0"},
		"versions": map[string]any{"1.0.0": map[string]any{
			"name": "demo", "version": "1.0.0", "description": "Test package",
		}},
		"_attachments": map[string]any{"demo-1.0.0.tgz": map[string]any{
			"content_type": "application/octet-stream", "length": len(tarball),
			"data": base64.StdEncoding.EncodeToString(tarball),
		}},
	}
	body, err := json.Marshal(document)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPut, "http://registry.example/npm/demo", bytes.NewReader(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	require.NoError(t, response.Body.Close())

	request = httptest.NewRequest(http.MethodGet, "http://registry.example/npm/demo", nil)
	request.Header.Set(fiber.HeaderAccept, abbreviatedMetadataType+"; q=1.0, application/json; q=0.8")
	response, err = app.Test(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Contains(t, response.Header.Get(fiber.HeaderContentType), abbreviatedMetadataType)
	var abbreviated map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&abbreviated))
	require.NoError(t, response.Body.Close())
	versions := abbreviated["versions"].(map[string]any)
	version := versions["1.0.0"].(map[string]any)
	dist := version["dist"].(map[string]any)
	assert.Equal(t, "http://registry.example/npm/demo/-/demo-1.0.0.tgz", dist["tarball"])
	assert.NotEmpty(t, dist["integrity"])
	assert.NotEmpty(t, dist["shasum"])

	request = httptest.NewRequest(http.MethodGet, "http://registry.example/npm/demo/-/demo-1.0.0.tgz", nil)
	response, err = app.Test(request)
	require.NoError(t, err)
	downloaded, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, tarball, downloaded)

	request = httptest.NewRequest(http.MethodPut,
		"http://registry.example/npm/-/package/demo/dist-tags/stable", bytes.NewReader([]byte(`"1.0.0"`)))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err = app.Test(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	require.NoError(t, response.Body.Close())

	request = httptest.NewRequest(http.MethodGet, "http://registry.example/npm/demo", nil)
	response, err = app.Test(request)
	require.NoError(t, err)
	var full map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&full))
	require.NoError(t, response.Body.Close())
	fullVersions := full["versions"].(map[string]any)
	fullVersion := fullVersions["1.0.0"].(map[string]any)
	fullVersion["deprecated"] = "Use 2.x"
	updateBody, err := json.Marshal(full)
	require.NoError(t, err)
	request = httptest.NewRequest(http.MethodPut, "http://registry.example/npm/demo", bytes.NewReader(updateBody))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err = app.Test(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	require.NoError(t, response.Body.Close())

	details, err := state.GetDB().GetNPMPackageDetails("npm", "demo", "alice")
	require.NoError(t, err)
	require.Len(t, details.Versions, 1)
	assert.Equal(t, "Use 2.x", details.Versions[0].Deprecated)
	assert.Equal(t, "1.0.0", details.DistTags["stable"])

	request = httptest.NewRequest(http.MethodPut, "http://registry.example/npm/demo", bytes.NewReader(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, response.StatusCode)
	require.NoError(t, response.Body.Close())

	request = httptest.NewRequest(http.MethodGet, "http://registry.example/npm/-/whoami", nil)
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	store.mu.Lock()
	assert.Len(t, store.files, 1)
	store.mu.Unlock()
}

func TestNPMPublishEnforcesPublicationQuotaBeforeStorageCommit(t *testing.T) {
	app, state, store := setupNPMTestApp(t)
	next := state.Inner.Config.Load().DeepCopy()
	next.PublicationQuota.FileLimit = 0
	state.Inner.Config.Store(next)
	tarball := npmTestTarball(t, "demo", "1.0.0")
	document := map[string]any{
		"_id": "demo", "name": "demo", "dist-tags": map[string]string{"latest": "1.0.0"},
		"versions": map[string]any{"1.0.0": map[string]any{"name": "demo", "version": "1.0.0"}},
		"_attachments": map[string]any{"demo-1.0.0.tgz": map[string]any{
			"length": len(tarball), "data": base64.StdEncoding.EncodeToString(tarball),
		}},
	}
	body, err := json.Marshal(document)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPut, "http://registry.example/npm/demo", bytes.NewReader(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, fiber.StatusTooManyRequests, response.StatusCode)
	assert.Equal(t, "publication_file_quota", response.Header.Get("X-Renop-Error-Code"))
	store.mu.Lock()
	assert.Empty(t, store.files)
	store.mu.Unlock()
}

func TestNPMPublicationReviewDefersVersionsAndNewPackagePolicy(t *testing.T) {
	app, state, _ := setupNPMTestApp(t)
	repo := state.Inner.Config.Load().Maven.Repositories["npm"]
	repo.PublicationReview = config.PublicationReviewEveryVersion
	publishVersion := func(version string) *http.Response {
		tarball := npmTestTarball(t, "demo", version)
		document := map[string]any{
			"_id": "demo", "name": "demo", "description": "Reviewed package",
			"dist-tags": map[string]string{"latest": version},
			"versions": map[string]any{version: map[string]any{
				"name": "demo", "version": version, "description": "Reviewed package",
			}},
			"_attachments": map[string]any{"demo-" + version + ".tgz": map[string]any{
				"content_type": "application/octet-stream", "length": len(tarball),
				"data": base64.StdEncoding.EncodeToString(tarball),
			}},
		}
		body, err := json.Marshal(document)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPut, "http://registry.example/npm/demo", bytes.NewReader(body))
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		response, err := app.Test(request)
		require.NoError(t, err)
		return response
	}
	response := publishVersion("1.0.0")
	require.Equal(t, http.StatusAccepted, response.StatusCode)
	reviewID := response.Header.Get("X-RenoP-Review-ID")
	require.NotEmpty(t, reviewID)
	require.NoError(t, response.Body.Close())
	details, err := state.GetDB().GetNPMPackageDetails("npm", "demo", "alice")
	require.NoError(t, err)
	require.Empty(t, details.Versions)
	require.NoError(t, AddPendingPublicationVersions(state, details))
	require.Len(t, details.Versions, 1)
	assert.Equal(t, core.ReviewStatusPending, details.Versions[0].ReviewStatus)
	assert.Equal(t, reviewID, details.Versions[0].ReviewID)

	task, err := state.GetDB().GetReviewTask(reviewID)
	require.NoError(t, err)
	previous, err := state.GetDB().GetNPMPackage("npm", "demo")
	require.NoError(t, err)
	require.NoError(t, ApprovePublicationReview(state, task))
	require.NoError(t, state.GetDB().SaveToken(&core.AccessToken{
		Name: "moderator", CreatedAt: time.Now().Format(time.RFC3339),
		Permissions: []string{"base", "canmoderate:npm"},
	}))
	_, err = state.GetDB().DecideReviewTask(reviewID, "moderator", core.ReviewStatusApproved, "",
		task.UpdatedAt+core.PublicationReviewSettleMillis+1)
	if err != nil {
		require.NoError(t, RemoveApprovedPublicationMetadata(state, task, previous, map[string]string{}))
	}
	require.NoError(t, err)
	details, err = state.GetDB().GetNPMPackageDetails("npm", "demo", "alice")
	require.NoError(t, err)
	require.Len(t, details.Versions, 1)
	assert.Equal(t, "1.0.0", details.Versions[0].Version)

	repo.PublicationReview = config.PublicationReviewNewPackages
	response = publishVersion("2.0.0")
	require.Equal(t, http.StatusCreated, response.StatusCode)
	require.Empty(t, response.Header.Get("X-RenoP-Review-ID"))
	require.NoError(t, response.Body.Close())
	details, err = state.GetDB().GetNPMPackageDetails("npm", "demo", "alice")
	require.NoError(t, err)
	require.Len(t, details.Versions, 2)
}

func TestNPMTarballValidationRejectsUnsafeAndMismatchedArchives(t *testing.T) {
	_, valid := NormalizePackageName("@team/package")
	assert.True(t, valid)
	_, ok := NormalizePackageName("../escape")
	assert.False(t, ok)
	_, _, ok = ClassifyTarballPath("@team/package/-/package-1.2.3.tgz")
	assert.True(t, ok)

	app, _, _ := setupNPMTestApp(t)
	tarball := npmTestTarball(t, "other", "1.0.0")
	document := map[string]any{
		"_id": "demo", "name": "demo", "dist-tags": map[string]string{"latest": "1.0.0"},
		"versions": map[string]any{"1.0.0": map[string]any{"name": "demo", "version": "1.0.0"}},
		"_attachments": map[string]any{"demo-1.0.0.tgz": map[string]any{
			"length": len(tarball), "data": base64.StdEncoding.EncodeToString(tarball),
		}},
	}
	body, err := json.Marshal(document)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPut, "http://registry.example/npm/demo", bytes.NewReader(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())
}

func TestNPMRegistryPublishesEncodedScopedPackage(t *testing.T) {
	app, state, _ := setupNPMTestApp(t)
	const packageName = "@r3/renop-test-core"
	const version = "1.2.3"
	_, err := state.GetDB().CreateNPMPackage("npm", packageName, "alice", false, time.Now().UnixMilli())
	require.NoError(t, err)
	tarball := npmTestTarball(t, packageName, version)
	document := map[string]any{
		"_id": packageName, "name": packageName, "description": "Scoped package", "access": "public",
		"dist-tags": map[string]string{"server-e2e": version},
		"versions": map[string]any{version: map[string]any{
			"_id": packageName + "@" + version, "name": packageName, "version": version,
			"description": "Scoped package", "_nodeVersion": "22.0.0", "_npmVersion": "11.0.0",
			"dist": map[string]any{
				"tarball": "http://registry.example/npm/@r3/renop-test-core/-/@r3/renop-test-core-1.2.3.tgz",
			},
		}},
		"_attachments": map[string]any{packageName + "-" + version + ".tgz": map[string]any{
			"content_type": "application/octet-stream",
			"length":       len(tarball),
			"data":         base64.StdEncoding.EncodeToString(tarball),
		}},
	}
	body, err := json.Marshal(document)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPut,
		"http://registry.example/npm/@r3%2frenop-test-core", bytes.NewReader(body))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request)
	require.NoError(t, err)
	responseBody, readErr := io.ReadAll(response.Body)
	require.NoError(t, readErr)
	require.NoError(t, response.Body.Close())
	require.Equalf(t, http.StatusCreated, response.StatusCode, "scoped npm publish failed: %s", responseBody)
}

func TestNPMDistTagValidationSeparatesTagsFromVersionRanges(t *testing.T) {
	for _, tag := range []string{"latest", "next", "release-2", "2beta"} {
		assert.Truef(t, validNPMTag(tag), "expected %q to be a valid npm dist-tag", tag)
	}
	for _, tag := range []string{"", "1", "1.2.3", "v2", "2.x", "^2.0.0", ">=2", "bad/tag"} {
		assert.Falsef(t, validNPMTag(tag), "expected %q to be rejected as an npm dist-tag", tag)
	}
}

func TestNPMBrowserRoutesYieldToTheSPAWithoutInterceptingRegistryClients(t *testing.T) {
	for _, requestPath := range []string{"", "packages", "packages/demo", "packages/%40team%2Fpackage"} {
		assert.Truef(t, npmBrowserRoute(fiber.MethodGet, "text/html,application/xhtml+xml", requestPath),
			"expected browser route %q to yield to the SPA", requestPath)
	}
	assert.False(t, npmBrowserRoute(fiber.MethodGet, fiber.MIMEApplicationJSON, "packages/demo"))
	assert.False(t, npmBrowserRoute(fiber.MethodPut, fiber.MIMETextHTML, "packages/demo"))
	assert.False(t, npmBrowserRoute(fiber.MethodGet, fiber.MIMETextHTML, "demo"))
}

func TestNPMMirrorImportsPackumentOnceAndRewritesTarballOrigin(t *testing.T) {
	config.ClearRepoCacheConfigs()
	t.Cleanup(config.ClearRepoCacheConfigs)
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path != "/mirror-demo" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		if _, err := writer.Write([]byte(`{
			"name":"mirror-demo",
			"description":"Mirrored package",
			"dist-tags":{"latest":"1.2.3"},
			"time":{"created":"2026-08-01T00:00:00Z","modified":"2026-08-02T00:00:00Z","1.2.3":"2026-08-02T00:00:00Z"},
			"versions":{"1.2.3":{"name":"mirror-demo","version":"1.2.3","dist":{
				"tarball":"https://upstream.example/mirror-demo/-/mirror-demo-1.2.3.tgz",
				"shasum":"0123456789012345678901234567890123456789",
				"integrity":"sha512-ZGVtbw=="}}}
		}`)); err != nil {
			t.Errorf("write npm mirror response: %v", err)
		}
	}))
	t.Cleanup(upstream.Close)

	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "npm-mirror.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state := core.NewAppState()
	state.Inner.DB = db
	cfg := config.DefaultConfig()
	cfg.Maven.Repositories = map[string]*config.Repository{
		"npm": {
			Name: "npm", Format: config.RepositoryFormatNPM, Visibility: "PUBLIC",
			Mirrors: []config.Mirror{{Name: "upstream", URL: upstream.URL, Persist: true, TimeoutSecs: 5}},
		},
	}
	state.Inner.Config.Store(cfg)
	handler := Handler{Store: newMemoryStore()}
	app := fiber.New(fiber.Config{JSONEncoder: json.Marshal, JSONDecoder: json.Unmarshal})
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", &config.User{Username: "guest", Roles: []string{"base"}})
		return c.Next()
	})
	app.Get("/npm/*", func(c fiber.Ctx) error {
		handled, handleErr := handler.Handle(c, state, cfg.Maven.Repositories["npm"], cfg.StoragePath, c.Params("*"))
		if !handled {
			return c.SendStatus(fiber.StatusNotFound)
		}
		return handleErr
	})

	for range 2 {
		request := httptest.NewRequest(http.MethodGet, "http://registry.example/npm/mirror-demo", nil)
		request.Header.Set(fiber.HeaderAccept, abbreviatedMetadataType)
		response, requestErr := app.Test(request)
		require.NoError(t, requestErr)
		require.Equal(t, http.StatusOK, response.StatusCode)
		var document struct {
			Versions map[string]struct {
				Dist map[string]any `json:"dist"`
			} `json:"versions"`
		}
		require.NoError(t, json.NewDecoder(response.Body).Decode(&document))
		require.NoError(t, response.Body.Close())
		require.Equal(t, "http://registry.example/npm/mirror-demo/-/mirror-demo-1.2.3.tgz",
			document.Versions["1.2.3"].Dist["tarball"])
	}
	require.Equal(t, int32(1), requests.Load())
	pkg, err := db.GetNPMPackage("npm", "mirror-demo")
	require.NoError(t, err)
	require.True(t, pkg.Mirrored)
	require.False(t, pkg.PublishEnabled)
}
