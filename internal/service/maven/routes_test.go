/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	"renop/internal/service/storage"
)

func newMavenRouteState(t *testing.T) (*core.AppState, *config.User) {
	t.Helper()
	storagePath := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"releases":  {Name: "releases", Format: config.RepositoryFormatMaven, Visibility: "PUBLIC"},
		"snapshots": {Name: "snapshots", Format: config.RepositoryFormatMavenClassic, Visibility: "PUBLIC"},
		"third":     {Name: "third", Format: config.RepositoryFormatMaven, Visibility: "PUBLIC"},
		"private":   {Name: "private", Format: config.RepositoryFormatMaven, Visibility: "PRIVATE"},
		"files": {
			Name: "files", Format: config.RepositoryFormatFiles, Visibility: "PUBLIC", AllowRedeployment: true,
		},
	}
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "maven-routes.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state.Inner.DB = db
	for _, username := range []string{"alice", "bob", "admin"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: username, CreatedAt: "2026-08-25T00:00:00Z"}))
	}
	return state, &config.User{Username: "alice", Roles: []string{"base"}}
}

func mavenRequest(t *testing.T, app *fiber.App, method, path, body string) *http.Response {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}
	response, err := app.Test(request)
	require.NoError(t, err)
	return response
}

func TestMavenDomainForceVerificationAndCrossRepositoryReuse(t *testing.T) {
	state, currentUser := newMavenRouteState(t)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", currentUser)
		return c.Next()
	})
	SetupRoutes(app.Group("/api"), state)
	storage.SetupRoutes(app, state)
	response := mavenRequest(t, app, http.MethodPost, "/api/maven/domains", `{"domain":"com.example"}`)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var created core.MavenDomain
	require.NoError(t, json.NewDecoder(response.Body).Decode(&created))
	require.NoError(t, response.Body.Close())
	assert.False(t, created.Verified)

	response = mavenRequest(t, app, http.MethodPut, "/releases/com/example/demo/1.0/demo-1.0.pom", "pom")
	require.Equal(t, http.StatusConflict, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = mavenRequest(t, app, http.MethodPost,
		"/api/maven/domains/com.example/verify/force", "")
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())

	currentUser = &config.User{Username: "admin", Roles: []string{"manager"}}
	response = mavenRequest(t, app, http.MethodPost,
		"/api/maven/domains/com.example/verify/force", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = mavenRequest(t, app, http.MethodPost, "/api/maven/domains/com.example/members",
		`{"users":[],"level":0}`)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = mavenRequest(t, app, http.MethodPost, "/api/maven/domains/com.example/members",
		`{"users":["missing-user"],"level":0}`)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())

	currentUser = &config.User{Username: "alice", Roles: []string{"base"}}
	response = mavenRequest(t, app, http.MethodPost, "/api/maven/domains/com.example/members",
		`{"users":["missing-user"],"level":0}`)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.Equal(t, "MAVEN_USER_NOT_FOUND", response.Header.Get("X-RenoP-Error-Code"))
	require.NoError(t, response.Body.Close())
	response = mavenRequest(t, app, http.MethodPost,
		"/api/maven/domains/com.example/members",
		`{"users":["bob","admin"],"level":0}`)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	require.NoError(t, response.Body.Close())
	messages, err := state.GetDB().ListMessages("bob", 10, 0, "", time.Now().Add(time.Minute).UnixMilli())
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "maven_domain_invite", messages[0].ActionKind)
	currentUser = &config.User{Username: "bob", Roles: []string{"base"}}
	response = mavenRequest(t, app, http.MethodPost,
		"/api/maven/invitations/"+messages[0].ID+"/accept", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	domainDetails, err := state.GetDB().GetMavenDomainDetails("com.example", "bob")
	require.NoError(t, err)
	assert.True(t, domainDetails.Domain.Member)
	assert.Equal(t, core.MavenPermissionRead, domainDetails.Domain.PermissionLevel)
	privateRepo := state.Inner.Config.Load().Maven.Repositories["releases"].DeepCopy()
	privateRepo.Visibility = "PRIVATE"
	allowed, err := CanReadRepository(state, currentUser, privateRepo, "", true)
	require.NoError(t, err)
	assert.True(t, allowed)
	allowed, err = CanReadRepository(state, &config.User{Username: "guest"}, privateRepo, "", true)
	require.NoError(t, err)
	assert.False(t, allowed)

	currentUser = &config.User{Username: "alice", Roles: []string{"base"}}
	response = mavenRequest(t, app, http.MethodPut, "/releases/com/example/readme.txt", "not a Maven artifact")
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = mavenRequest(t, app, http.MethodPut, "/releases/com/example/demo/1.0/demo-1.0.pom", "pom")
	require.Equal(t, http.StatusCreated, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = mavenRequest(t, app, http.MethodGet,
		"/api/maven/repositories/releases/packages?domain=com.example&limit=50&offset=0", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	var releaseCatalog struct {
		Artifacts []*core.MavenArtifact `json:"artifacts"`
		Total     int                   `json:"total"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&releaseCatalog))
	require.NoError(t, response.Body.Close())
	assert.Equal(t, 1, releaseCatalog.Total)
	require.Len(t, releaseCatalog.Artifacts, 1)
	assert.Equal(t, "com.example:demo", releaseCatalog.Artifacts[0].GroupID+":"+releaseCatalog.Artifacts[0].ArtifactID)
	response = mavenRequest(t, app, http.MethodGet, "/api/maven/repositories/releases/domains", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	var releaseDomains struct {
		Domains []*core.MavenDomain `json:"domains"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&releaseDomains))
	require.NoError(t, response.Body.Close())
	require.Len(t, releaseDomains.Domains, 1)
	assert.Equal(t, "com.example", releaseDomains.Domains[0].Domain)
	assert.Equal(t, 1, releaseDomains.Domains[0].ArtifactCount)

	response = mavenRequest(t, app, http.MethodGet, "/api/maven/domains/com.example", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	var reused core.MavenDomainDetails
	require.NoError(t, json.NewDecoder(response.Body).Decode(&reused))
	require.NoError(t, response.Body.Close())
	require.NotNil(t, reused.Domain)
	assert.True(t, reused.Domain.Verified)
	assert.Equal(t, created.VerificationCode, reused.Domain.VerificationCode)
	response = mavenRequest(t, app, http.MethodPut, "/snapshots/com/example/free-form.txt", "blocked")
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = mavenRequest(t, app, http.MethodPut, "/snapshots/com/example/demo/2.0/demo-2.0.jar", "artifact")
	require.Equal(t, http.StatusCreated, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = mavenRequest(t, app, http.MethodGet,
		"/api/maven/repositories/snapshots/packages?domain=com.example&limit=50&offset=0", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	var snapshotCatalog struct {
		Artifacts []*core.MavenArtifact `json:"artifacts"`
		Total     int                   `json:"total"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&snapshotCatalog))
	require.NoError(t, response.Body.Close())
	assert.Equal(t, 1, snapshotCatalog.Total)
	require.Len(t, snapshotCatalog.Artifacts, 1)
	response = mavenRequest(t, app, http.MethodGet, "/api/maven/repositories/snapshots/domains", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	var snapshotDomains struct {
		Domains []*core.MavenDomain `json:"domains"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&snapshotDomains))
	require.NoError(t, response.Body.Close())
	require.Len(t, snapshotDomains.Domains, 1)
	assert.Equal(t, 1, snapshotDomains.Domains[0].ArtifactCount)

	response = mavenRequest(t, app, http.MethodGet, "/api/maven/domains/com.example", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	var crossRepositoryDetails core.MavenDomainDetails
	require.NoError(t, json.NewDecoder(response.Body).Decode(&crossRepositoryDetails))
	require.NoError(t, response.Body.Close())
	require.NotNil(t, crossRepositoryDetails.Domain)
	assert.Equal(t, 2, crossRepositoryDetails.Domain.ArtifactCount)

	currentUser = &config.User{Username: "bob", Roles: []string{"base"}}
	response = mavenRequest(t, app, http.MethodPost, "/api/maven/domains", `{"domain":"org.third"}`)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var independent core.MavenDomain
	require.NoError(t, json.NewDecoder(response.Body).Decode(&independent))
	require.NoError(t, response.Body.Close())
	assert.False(t, independent.Verified)
}

func TestFileRepositoryAllowsReplacementWithoutMavenHelpers(t *testing.T) {
	state, _ := newMavenRouteState(t)
	currentUser := &config.User{Username: "admin", Roles: []string{"manager"}}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", currentUser)
		return c.Next()
	})
	SetupRoutes(app.Group("/api"), state)
	storage.SetupRoutes(app, state)

	for _, content := range []string{"first", "replacement"} {
		request := httptest.NewRequest(http.MethodPut, "/files/arbitrary/path/readme.txt", strings.NewReader(content))
		request.Header.Set("X-Generate-Checksums", "true")
		request.Header.Set("X-RenoP-GPG-Signature-Expected", "true")
		response, err := app.Test(request)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	signatureRequest := httptest.NewRequest(http.MethodPut, "/files/signatures/example.jar.asc", strings.NewReader("plain file"))
	signatureResponse, err := app.Test(signatureRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, signatureResponse.StatusCode)
	require.NoError(t, signatureResponse.Body.Close())
	path := filepath.Join(state.Inner.Config.Load().StoragePath, "files", "arbitrary", "path", "readme.txt")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "replacement", string(content))
	stored, indexed := state.Inner.FileIndex.GetFileInfo(path)
	require.True(t, indexed)
	assert.Equal(t, int64(len(content)), stored.Size)
	assert.False(t, state.Inner.FileIndex.HasFile(path+".sha1"))
	assert.False(t, state.Inner.FileIndex.HasFile(path+".sha256"))
}

func TestModernMavenAndFileRepositoriesResolveMirrors(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/com/example/demo/1.0/demo-1.0.pom":
			writer.Header().Set(fiber.HeaderContentType, "application/xml")
			_, _ = writer.Write([]byte("<project/>"))
		case "/downloads/app.zip":
			writer.Header().Set(fiber.HeaderContentType, "application/zip")
			_, _ = writer.Write([]byte("archive"))
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(upstream.Close)

	state, currentUser := newMavenRouteState(t)
	cfg := state.Inner.Config.Load().DeepCopy()
	cfg.Maven.Repositories["releases"].Mirrors = []config.Mirror{{
		Name: "upstream", URL: upstream.URL, TimeoutSecs: 5,
	}}
	cfg.Maven.Repositories["files"].Mirrors = []config.Mirror{{
		Name: "upstream", URL: upstream.URL, TimeoutSecs: 5,
	}}
	state.Inner.Config.Store(cfg)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", currentUser)
		return c.Next()
	})
	SetupRoutes(app.Group("/api"), state)
	storage.SetupRoutes(app, state)

	response := mavenRequest(t, app, http.MethodGet, "/releases/com/example/demo/1.0/demo-1.0.pom", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "<project/>", string(body))
	response = mavenRequest(t, app, http.MethodGet, "/files/downloads/app.zip", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	body, err = io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "archive", string(body))
}
