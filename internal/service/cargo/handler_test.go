/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/auth"
	"renop/internal/service/index"
)

type memoryStore struct {
	mu    sync.Mutex
	files map[string][]byte
}

type memoryStagedFile struct {
	store  *memoryStore
	target string
	buffer bytes.Buffer
	closed bool
}

type cargoPublishResult struct {
	status int
	err    error
}

func newMemoryStore() *memoryStore {
	return &memoryStore{files: make(map[string][]byte)}
}

func (s *memoryStore) Open(path string) (io.ReadCloser, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.files[path]
	if !ok {
		return nil, false, nil
	}
	return io.NopCloser(bytes.NewReader(bytes.Clone(data))), true, nil
}

func (s *memoryStore) Exists(path string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.files[path]
	return ok, nil
}

func (s *memoryStore) Stage(path string) (StagedFile, error) {
	return &memoryStagedFile{store: s, target: path}, nil
}

func (s *memoryStore) Delete(_ *core.AppState, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, path)
	return nil
}

func (staged *memoryStagedFile) Write(data []byte) (int, error) {
	if staged.closed {
		return 0, io.ErrClosedPipe
	}
	return staged.buffer.Write(data)
}

func (staged *memoryStagedFile) Close() error {
	staged.closed = true
	return nil
}

func (staged *memoryStagedFile) Open() (io.ReadCloser, error) {
	staged.closed = true
	return io.NopCloser(bytes.NewReader(staged.buffer.Bytes())), nil
}

func (staged *memoryStagedFile) Size() (int64, error) {
	return int64(staged.buffer.Len()), nil
}

func (staged *memoryStagedFile) Commit(_ *core.AppState) error {
	staged.store.mu.Lock()
	defer staged.store.mu.Unlock()
	staged.store.files[staged.target] = bytes.Clone(staged.buffer.Bytes())
	return nil
}

func (staged *memoryStagedFile) Discard() error {
	staged.buffer.Reset()
	return nil
}

func cargoTestApp(t *testing.T, handler Handler, state *core.AppState, repo *config.Repository, storagePath string, users ...*config.User) *fiber.App {
	t.Helper()
	app := fiber.New()
	if len(users) > 0 && users[0] != nil {
		app.Use(func(c fiber.Ctx) error {
			c.Locals("user", users[0])
			return c.Next()
		})
	}
	app.All("/:repo_name/*", func(c fiber.Ctx) error {
		path := c.Params("*")
		handled, err := handler.Handle(c, state, repo, storagePath, path)
		if handled {
			return err
		}
		return c.SendStatus(fiber.StatusNotFound)
	})
	return app
}

func makePublishBody(t *testing.T, metadata PublishMetadata, crate []byte) []byte {
	t.Helper()
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 4+len(metadataJSON)+4+len(crate))
	binary.LittleEndian.PutUint32(body[:4], uint32(len(metadataJSON)))
	copy(body[4:], metadataJSON)
	crateLengthOffset := 4 + len(metadataJSON)
	binary.LittleEndian.PutUint32(body[crateLengthOffset:crateLengthOffset+4], uint32(len(crate)))
	copy(body[crateLengthOffset+4:], crate)
	return body
}

func TestHandlerServesSparseConfig(t *testing.T) {
	store := newMemoryStore()
	repo := &config.Repository{Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PRIVATE"}
	app := cargoTestApp(t, Handler{Store: store}, core.NewAppState(), repo, t.TempDir())

	req := httptest.NewRequest("GET", "http://registry.example/cargo/config.json", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var payload RegistryConfig
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.DownloadURL != "http://registry.example/cargo/api/v1/crates" ||
		payload.APIURL != "http://registry.example/cargo" || !payload.AuthNeeded {
		t.Fatalf("unexpected config: %+v", payload)
	}
}

func TestPublicProtocolTrustsForwardedHTTPSOnlyFromConfiguredProxy(t *testing.T) {
	state := core.NewAppState()
	cfg := &config.Config{}
	cfg.Server.TrustedProxies = []string{"0.0.0.0/0", "::/0"}
	cfg.Server.ParseTrustedProxies()
	state.Inner.Config.Store(cfg)
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString(publicProtocol(c, state))
	})

	req := httptest.NewRequest("GET", "http://registry.example/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set(fiber.HeaderXForwardedProto, "https")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "https" {
		t.Fatalf("protocol = %q, want https", body)
	}
}

func TestHandlerPublishesCrateAndRejectsDuplicate(t *testing.T) {
	storagePath := t.TempDir()
	store := newMemoryStore()
	repo := &config.Repository{Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC"}
	state := core.NewAppState()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "cargo.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state.Inner.DB = db
	user := &config.User{Username: "publisher", Roles: []string{"canupdate:cargo"}}
	app := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, user)
	crate := makeCrateArchive(t, map[string]string{
		"demo-1.2.3/Cargo.toml":     "[package]\nname = \"demo\"\nversion = \"1.2.3\"\nreadme = \"docs/README.md\"\n",
		"demo-1.2.3/docs/README.md": "# Demo\n\nPublished Cargo **README**.\n",
	})
	body := makePublishBody(t, PublishMetadata{
		Name: "demo", Version: "1.2.3", Deps: []PublishDependency{}, Features: map[string][]string{},
	}, crate)

	publish := func() (int, []byte) {
		req := httptest.NewRequest("PUT", "http://registry.example/cargo/api/v1/crates/new", bytes.NewReader(body))
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode, responseBody
	}
	status, responseBody := publish()
	if status != fiber.StatusOK {
		t.Fatalf("first publish status = %d", status)
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(responseBody, &response); err != nil {
		t.Fatalf("decode publish response: %v", err)
	}
	if _, found := response["warnings"]; !found {
		t.Fatalf("publish response missing warnings: %s", responseBody)
	}
	if _, found := response["errors"]; found {
		t.Fatalf("successful publish response must not contain errors: %s", responseBody)
	}
	cratePath := filepath.Join(storagePath, "cargo", "api", "v1", "crates", "demo", "1.2.3", "download")
	indexFile := filepath.Join(storagePath, "cargo", "de", "mo", "demo")
	crateFound, _ := store.Exists(cratePath)
	indexFound, _ := store.Exists(indexFile)
	if !crateFound || !indexFound {
		t.Fatalf("published files missing: crate=%v index=%v", crateFound, indexFound)
	}
	indexReader, _, _ := store.Open(indexFile)
	indexData, _ := io.ReadAll(indexReader)
	_ = indexReader.Close()
	if !bytes.Contains(indexData, []byte(`"vers":"1.2.3"`)) || !bytes.Contains(indexData, []byte(`"cksum":`)) {
		t.Fatalf("invalid sparse index entry: %s", indexData)
	}
	details, err := db.GetCargoPackageDetails("cargo", "demo", "publisher")
	if err != nil || details == nil || details.Package == nil || details.Package.Readme != "# Demo\n\nPublished Cargo **README**." {
		t.Fatalf("published Cargo README was not persisted: %#v, %v", details, err)
	}
	status, _ = publish()
	if status != fiber.StatusConflict {
		t.Fatalf("duplicate publish status = %d, want %d", status, fiber.StatusConflict)
	}
}

func TestCargoPublicationReviewDefersSparseIndexAndCatalog(t *testing.T) {
	storagePath := t.TempDir()
	store := newMemoryStore()
	repo := &config.Repository{
		Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC",
		PublicationReview: config.PublicationReviewEveryVersion,
	}
	state := core.NewAppState()
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{"cargo": repo}
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndexCustom(true)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "cargo-review.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state.Inner.DB = db
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "publisher", Permissions: []string{"canupdate:cargo"}}))
	user := &config.User{Username: "publisher", Roles: []string{"canupdate:cargo"}}
	app := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, user)
	publish := func(version string) *http.Response {
		crate := makeCrateArchive(t, map[string]string{
			"demo-" + version + "/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"" + version + "\"\n",
		})
		body := makePublishBody(t, PublishMetadata{
			Name: "demo", Version: version, Deps: []PublishDependency{}, Features: map[string][]string{},
		}, crate)
		response, requestErr := app.Test(httptest.NewRequest(
			http.MethodPut, "http://registry.example/cargo/api/v1/crates/new", bytes.NewReader(body)))
		require.NoError(t, requestErr)
		return response
	}
	response := publish("1.0.0")
	require.Equal(t, http.StatusAccepted, response.StatusCode)
	reviewID := response.Header.Get("X-RenoP-Review-ID")
	require.NotEmpty(t, reviewID)
	require.NoError(t, response.Body.Close())
	indexFile := filepath.Join(storagePath, "cargo", "de", "mo", "demo")
	indexExists, err := store.Exists(indexFile)
	require.NoError(t, err)
	assert.False(t, indexExists)
	pkg, err := db.GetCargoPackage("cargo", "demo")
	require.NoError(t, err)
	assert.Nil(t, pkg)
	requesterResponse, err := app.Test(httptest.NewRequest(
		http.MethodGet, "http://registry.example/cargo/api/v1/crates/demo", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, requesterResponse.StatusCode)
	var pendingDetails packageInfoResponse
	require.NoError(t, json.NewDecoder(requesterResponse.Body).Decode(&pendingDetails))
	require.NoError(t, requesterResponse.Body.Close())
	require.Len(t, pendingDetails.Versions, 1)
	assert.Equal(t, core.ReviewStatusPending, pendingDetails.Versions[0].ReviewStatus)
	assert.Equal(t, core.CargoPermissionOwner, pendingDetails.Package.PermissionLevel)
	guestApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath)
	guestResponse, err := guestApp.Test(httptest.NewRequest(
		http.MethodGet, "http://registry.example/cargo/api/v1/crates/demo", nil))
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, guestResponse.StatusCode)
	require.NoError(t, guestResponse.Body.Close())
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "other", Permissions: []string{"canupdate:cargo"}}))
	other := &config.User{Username: "other", Roles: []string{"canupdate:cargo"}}
	otherApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, other)
	otherArchive := makeCrateArchive(t, map[string]string{
		"demo-1.1.0/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"1.1.0\"\n",
	})
	otherBody := makePublishBody(t, PublishMetadata{
		Name: "demo", Version: "1.1.0", Deps: []PublishDependency{}, Features: map[string][]string{},
	}, otherArchive)
	otherResponse, err := otherApp.Test(httptest.NewRequest(
		http.MethodPut, "http://registry.example/cargo/api/v1/crates/new", bytes.NewReader(otherBody)))
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, otherResponse.StatusCode)
	require.NoError(t, otherResponse.Body.Close())

	task, err := db.GetReviewTask(reviewID)
	require.NoError(t, err)
	rollback, err := ApprovePublicationReview(state, task, store)
	require.NoError(t, err)
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "moderator", Permissions: []string{"base", "canmoderate:cargo"},
	}))
	_, err = db.DecideReviewTask(reviewID, "moderator", core.ReviewStatusApproved, "",
		task.UpdatedAt+core.PublicationReviewSettleMillis+1)
	if err != nil {
		require.NoError(t, rollback())
	}
	require.NoError(t, err)
	indexExists, err = store.Exists(indexFile)
	require.NoError(t, err)
	assert.True(t, indexExists)
	details, err := db.GetCargoPackageDetails("cargo", "demo", "publisher")
	require.NoError(t, err)
	require.Len(t, details.Versions, 1)
	assert.Equal(t, "1.0.0", details.Versions[0].Version)

	repo.PublicationReview = config.PublicationReviewNewPackages
	response = publish("2.0.0")
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Empty(t, response.Header.Get("X-RenoP-Review-ID"))
	require.NoError(t, response.Body.Close())
	details, err = db.GetCargoPackageDetails("cargo", "demo", "publisher")
	require.NoError(t, err)
	require.Len(t, details.Versions, 2)
}

func TestNewCargoPackageRequiresCreateScope(t *testing.T) {
	storagePath := t.TempDir()
	store := newMemoryStore()
	repo := &config.Repository{Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC"}
	state := core.NewAppState()
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{"cargo": repo}
	state.Inner.Config.Store(cfg)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "cargo-api-token.db"), MaxOpenConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state.Inner.DB = db
	if err := db.SaveToken(&core.AccessToken{
		Name: "publisher", Permissions: []string{"canupdate:cargo"},
	}); err != nil {
		t.Fatal(err)
	}
	createCredential := func(name string, scopes []string) string {
		secret, generateErr := core.GenerateAPITokenSecret()
		if generateErr != nil {
			t.Fatal(generateErr)
		}
		if createErr := db.CreateAPIToken("publisher", &core.APIToken{
			ID: uuid.NewString(), Name: name, Scopes: scopes, CreatedAt: time.Now().UnixMilli(),
		}, core.HashAPITokenSecret(secret)); createErr != nil {
			t.Fatal(createErr)
		}
		return secret
	}
	publishOnly := createCredential("Publish existing crates", []string{core.APITokenScopeRepositoryPublish})
	publishAndCreate := createCredential("Create crates", []string{
		core.APITokenScopeRepositoryPublish, core.APITokenScopePackageCreate,
	})

	handler := Handler{Store: store}
	app := fiber.New()
	app.Use(auth.AuthMiddleware(state))
	app.All("/:repo_name/*", func(c fiber.Ctx) error {
		handled, handlerErr := handler.Handle(c, state, repo, storagePath, c.Params("*"))
		if handled {
			return handlerErr
		}
		return c.SendStatus(fiber.StatusNotFound)
	})
	crate := makeCrateArchive(t, map[string]string{
		"scoped-1.0.0/Cargo.toml": "[package]\nname = \"scoped\"\nversion = \"1.0.0\"\n",
	})
	body := makePublishBody(t, PublishMetadata{
		Name: "scoped", Version: "1.0.0", Deps: []PublishDependency{}, Features: map[string][]string{},
	}, crate)
	request := func(secret string) int {
		req := httptest.NewRequest(http.MethodPut, "/cargo/api/v1/crates/new", bytes.NewReader(body))
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+secret)
		resp, requestErr := app.Test(req)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if status := request(publishOnly); status != fiber.StatusForbidden {
		t.Fatalf("publish-only token created a package: status = %d", status)
	}
	if status := request(publishAndCreate); status != fiber.StatusOK {
		t.Fatalf("publish/create token status = %d, want 200", status)
	}
}

func TestNewCargoPackageRequiresAvailableUpstreamName(t *testing.T) {
	tests := []struct {
		name          string
		upstreamFound bool
		probeErr      error
		wantStatus    int
	}{
		{name: "upstream conflict", upstreamFound: true, wantStatus: fiber.StatusConflict},
		{name: "probe unavailable", probeErr: errors.New("upstream unavailable"), wantStatus: fiber.StatusServiceUnavailable},
		{name: "upstream name available", wantStatus: fiber.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			storagePath := t.TempDir()
			store := newMemoryStore()
			repo := &config.Repository{
				Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC",
				Mirrors: []config.Mirror{{URL: "https://upstream.example"}},
			}
			state := core.NewAppState()
			db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "cargo.db")})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			state.Inner.DB = db
			handler := Handler{
				Store: store,
				UpstreamIndexExists: func(_ context.Context, _ *core.AppState, _ *config.Repository, path string) (bool, error) {
					if path != "de/mo/demo" {
						t.Fatalf("upstream index path = %q", path)
					}
					return tc.upstreamFound, tc.probeErr
				},
			}
			app := cargoTestApp(t, handler, state, repo, storagePath,
				&config.User{Username: "publisher", Roles: []string{"canupdate:cargo"}})
			crate := makeCrateArchive(t, map[string]string{
				"demo-1.0.0/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"1.0.0\"\n",
			})
			body := makePublishBody(t, PublishMetadata{
				Name: "demo", Version: "1.0.0", Deps: []PublishDependency{}, Features: map[string][]string{},
			}, crate)
			response, err := app.Test(httptest.NewRequest(
				http.MethodPut, "http://registry.example/cargo/api/v1/crates/new", bytes.NewReader(body),
			))
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode != tc.wantStatus {
				t.Fatalf("publish status = %d, want %d", response.StatusCode, tc.wantStatus)
			}
			if tc.wantStatus != fiber.StatusOK {
				store.mu.Lock()
				storedFiles := len(store.files)
				store.mu.Unlock()
				if storedFiles != 0 {
					t.Fatalf("rejected upstream name stored %d files", storedFiles)
				}
			}
		})
	}
}

func TestConcurrentNormalizedCargoNamesCannotCreateSplitPackages(t *testing.T) {
	storagePath := t.TempDir()
	store := newMemoryStore()
	repo := &config.Repository{Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC"}
	state := core.NewAppState()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "cargo-name-race.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state.Inner.DB = db
	app := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath,
		&config.User{Username: "publisher", Roles: []string{"canupdate:cargo"}})

	names := []string{"demo-crate", "demo_crate"}
	bodies := make([][]byte, len(names))
	for i, name := range names {
		crate := makeCrateArchive(t, map[string]string{
			name + "-1.0.0/Cargo.toml": "[package]\nname = \"" + name + "\"\nversion = \"1.0.0\"\n",
		})
		bodies[i] = makePublishBody(t, PublishMetadata{
			Name: name, Version: "1.0.0", Deps: []PublishDependency{}, Features: map[string][]string{},
		}, crate)
	}

	start := make(chan struct{})
	results := make(chan cargoPublishResult, len(names))
	for _, body := range bodies {
		go func() {
			<-start
			response, err := app.Test(httptest.NewRequest(
				http.MethodPut, "http://registry.example/cargo/api/v1/crates/new", bytes.NewReader(body),
			))
			if err != nil {
				results <- cargoPublishResult{err: err}
				return
			}
			defer response.Body.Close()
			results <- cargoPublishResult{status: response.StatusCode}
		}()
	}
	close(start)
	statuses := make([]int, 0, len(names))
	for range names {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		statuses = append(statuses, result.status)
	}
	slices.Sort(statuses)
	if !slices.Equal(statuses, []int{fiber.StatusOK, fiber.StatusConflict}) {
		t.Fatalf("concurrent normalized-name publish statuses = %v", statuses)
	}

	packages, total, err := db.SearchCargoPackages("cargo", "demo", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(packages) != 1 {
		t.Fatalf("normalized-name race created %d package records", total)
	}
	store.mu.Lock()
	storedFiles := len(store.files)
	store.mu.Unlock()
	if storedFiles != 2 {
		t.Fatalf("normalized-name race left %d stored files, want one index and one crate", storedFiles)
	}
}

func TestCargoInvitationGrantsOnlyRequestedPackageLevel(t *testing.T) {
	storagePath := t.TempDir()
	store := newMemoryStore()
	repo := &config.Repository{Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC"}
	state := core.NewAppState()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "cargo-team.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state.Inner.DB = db
	if err := db.SaveToken(&core.AccessToken{Name: "bob", Identifier: core.AccessTokenIdentifier{Type: core.Persistent}}); err != nil {
		t.Fatal(err)
	}

	alice := &config.User{Username: "alice", Roles: []string{"canupdate:cargo"}}
	aliceApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, alice)
	publishVersion := func(app *fiber.App, version string) int {
		crate := makeCrateArchive(t, map[string]string{
			"demo-" + version + "/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"" + version + "\"\n",
		})
		body := makePublishBody(t, PublishMetadata{
			Name: "demo", Version: version, Deps: []PublishDependency{}, Features: map[string][]string{},
		}, crate)
		request := httptest.NewRequest("PUT", "http://registry.example/cargo/api/v1/crates/new", bytes.NewReader(body))
		response, err := app.Test(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}
	if status := publishVersion(aliceApp, "1.0.0"); status != fiber.StatusOK {
		t.Fatalf("initial publish status = %d", status)
	}

	inviteRequest := httptest.NewRequest("PUT", "http://registry.example/cargo/api/v1/crates/demo/owners", strings.NewReader(`{"users":["bob"],"level":1}`))
	inviteRequest.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	inviteResponse, err := aliceApp.Test(inviteRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = inviteResponse.Body.Close()
	if inviteResponse.StatusCode != fiber.StatusOK {
		t.Fatalf("invite status = %d", inviteResponse.StatusCode)
	}

	messages, err := db.ListMessages("bob", 10, 0, "", time.Now().UnixMilli())
	if err != nil || len(messages) != 1 {
		t.Fatalf("invitation messages = %d, err = %v", len(messages), err)
	}
	bob := &config.User{Username: "bob", Roles: []string{"base"}}
	bobApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, bob)
	acceptRequest := httptest.NewRequest("POST", "http://registry.example/cargo/api/v1/invitations/"+messages[0].ID+"/accept", nil)
	acceptResponse, err := bobApp.Test(acceptRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = acceptResponse.Body.Close()
	if acceptResponse.StatusCode != fiber.StatusOK {
		t.Fatalf("accept status = %d", acceptResponse.StatusCode)
	}
	if status := publishVersion(bobApp, "1.1.0"); status != fiber.StatusOK {
		t.Fatalf("L1 publish status = %d", status)
	}

	yankRequest := httptest.NewRequest("DELETE", "http://registry.example/cargo/api/v1/crates/demo/1.1.0/yank", nil)
	yankResponse, err := bobApp.Test(yankRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = yankResponse.Body.Close()
	if yankResponse.StatusCode != fiber.StatusForbidden {
		t.Fatalf("L1 yank status = %d, want 403", yankResponse.StatusCode)
	}
}

func TestCargoAdministratorCannotPublish(t *testing.T) {
	state := core.NewAppState()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "cargo-admin.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state.Inner.DB = db
	store := newMemoryStore()
	repo := &config.Repository{Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC"}
	storagePath := t.TempDir()
	admin := &config.User{Username: "admin", Roles: []string{"manager"}}
	adminApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, admin)

	crate := makeCrateArchive(t, map[string]string{
		"demo-1.0.0/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"1.0.0\"\n",
	})
	body := makePublishBody(t, PublishMetadata{
		Name: "demo", Version: "1.0.0", Deps: []PublishDependency{}, Features: map[string][]string{},
	}, crate)
	request := httptest.NewRequest("PUT", "http://registry.example/cargo/api/v1/crates/new", bytes.NewReader(body))
	response, err := adminApp.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != fiber.StatusForbidden {
		t.Fatalf("admin publish status = %d, want 403", response.StatusCode)
	}
}

func TestCargoPackageInfoIsPublicAndHidesTeamFromNonMembers(t *testing.T) {
	storagePath := t.TempDir()
	store := newMemoryStore()
	repo := &config.Repository{Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC"}
	state := core.NewAppState()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "cargo-public-info.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state.Inner.DB = db
	if err := db.SaveToken(&core.AccessToken{Name: "alice", Identifier: core.AccessTokenIdentifier{Type: core.Persistent}}); err != nil {
		t.Fatal(err)
	}

	alice := &config.User{Username: "alice", Roles: []string{"canupdate:cargo"}}
	aliceApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, alice)
	crate := makeCrateArchive(t, map[string]string{
		"demo-1.0.0/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"1.0.0\"\n",
	})
	body := makePublishBody(t, PublishMetadata{
		Name: "demo", Version: "1.0.0", Deps: []PublishDependency{}, Features: map[string][]string{},
	}, crate)
	publishResponse, err := aliceApp.Test(httptest.NewRequest(
		http.MethodPut, "http://registry.example/cargo/api/v1/crates/new", bytes.NewReader(body),
	))
	if err != nil {
		t.Fatal(err)
	}
	_ = publishResponse.Body.Close()
	if publishResponse.StatusCode != fiber.StatusOK {
		t.Fatalf("publish status = %d", publishResponse.StatusCode)
	}

	guestApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, &config.User{Username: "guest"})
	publicInfoResult, err := guestApp.Test(httptest.NewRequest(
		http.MethodGet, "http://registry.example/cargo/api/v1/crates/demo", nil,
	))
	if err != nil {
		t.Fatal(err)
	}
	var publicInfo packageInfoResponse
	if decodeErr := json.NewDecoder(publicInfoResult.Body).Decode(&publicInfo); decodeErr != nil {
		_ = publicInfoResult.Body.Close()
		t.Fatal(decodeErr)
	}
	_ = publicInfoResult.Body.Close()
	if publicInfoResult.StatusCode != fiber.StatusOK || publicInfo.Admin || publicInfo.Package == nil || len(publicInfo.Members) != 0 {
		t.Fatalf("public package info = status %d, payload %+v", publicInfoResult.StatusCode, publicInfo)
	}
	if publicInfo.Package.PermissionLevel != 0 {
		t.Fatalf("public package permission level = %d, want 0", publicInfo.Package.PermissionLevel)
	}
}

func TestCargoAdministratorLifecycleLocksCannotBeRestoredByOwner(t *testing.T) {
	storagePath := t.TempDir()
	store := newMemoryStore()
	repo := &config.Repository{Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC"}
	state := core.NewAppState()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "cargo-admin-locks.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state.Inner.DB = db
	if err := db.SaveToken(&core.AccessToken{Name: "owner", Identifier: core.AccessTokenIdentifier{Type: core.Persistent}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveToken(&core.AccessToken{Name: "other", Identifier: core.AccessTokenIdentifier{Type: core.Persistent}}); err != nil {
		t.Fatal(err)
	}

	owner := &config.User{Username: "owner", Roles: []string{"canupdate:cargo"}}
	ownerApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, owner)
	crate := makeCrateArchive(t, map[string]string{
		"demo-1.0.0/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"1.0.0\"\n",
	})
	body := makePublishBody(t, PublishMetadata{
		Name: "demo", Version: "1.0.0", Deps: []PublishDependency{}, Features: map[string][]string{},
	}, crate)
	publishResponse, err := ownerApp.Test(httptest.NewRequest(
		http.MethodPut, "http://registry.example/cargo/api/v1/crates/new", bytes.NewReader(body),
	))
	if err != nil {
		t.Fatal(err)
	}
	_ = publishResponse.Body.Close()
	if publishResponse.StatusCode != fiber.StatusOK {
		t.Fatalf("owner publish status = %d", publishResponse.StatusCode)
	}

	admin := &config.User{Username: "admin", Roles: []string{"manager"}}
	adminApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, admin)
	packageInfoResult, err := adminApp.Test(httptest.NewRequest(
		http.MethodGet, "http://registry.example/cargo/api/v1/crates/demo", nil,
	))
	if err != nil {
		t.Fatal(err)
	}
	var packageInfo packageInfoResponse
	if decodeErr := json.NewDecoder(packageInfoResult.Body).Decode(&packageInfo); decodeErr != nil {
		_ = packageInfoResult.Body.Close()
		t.Fatal(decodeErr)
	}
	_ = packageInfoResult.Body.Close()
	if packageInfoResult.StatusCode != fiber.StatusOK || !packageInfo.Admin || packageInfo.Package == nil {
		t.Fatalf("administrator package info = status %d, payload %+v", packageInfoResult.StatusCode, packageInfo)
	}
	invite := httptest.NewRequest(http.MethodPut, "http://registry.example/cargo/api/v1/crates/demo/owners", strings.NewReader(`{"users":["other"],"level":3}`))
	invite.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	inviteResponse, err := adminApp.Test(invite)
	if err != nil {
		t.Fatal(err)
	}
	_ = inviteResponse.Body.Close()
	if inviteResponse.StatusCode != fiber.StatusOK {
		t.Fatalf("administrator team mutation status = %d, want 200", inviteResponse.StatusCode)
	}

	requestStatus := func(app *fiber.App, method, target string) int {
		response, err := app.Test(httptest.NewRequest(method, "http://registry.example"+target, nil))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}
	if status := requestStatus(adminApp, http.MethodDelete, "/cargo/api/v1/crates/demo/1.0.0/yank"); status != fiber.StatusOK {
		t.Fatalf("administrator yank status = %d", status)
	}
	if status := requestStatus(ownerApp, http.MethodPut, "/cargo/api/v1/crates/demo/1.0.0/unyank"); status != fiber.StatusForbidden {
		t.Fatalf("owner restore of administrator-yanked version status = %d, want 403", status)
	}
	if status := requestStatus(adminApp, http.MethodPut, "/cargo/api/v1/crates/demo/1.0.0/unyank"); status != fiber.StatusOK {
		t.Fatalf("administrator version restore status = %d", status)
	}
	if status := requestStatus(adminApp, http.MethodPut, "/cargo/api/v1/crates/demo/archive"); status != fiber.StatusOK {
		t.Fatalf("administrator archive status = %d", status)
	}
	if status := requestStatus(ownerApp, http.MethodDelete, "/cargo/api/v1/crates/demo/archive"); status != fiber.StatusForbidden {
		t.Fatalf("owner restore of administrator-archived package status = %d, want 403", status)
	}
	if status := requestStatus(adminApp, http.MethodDelete, "/cargo/api/v1/crates/demo/archive"); status != fiber.StatusOK {
		t.Fatalf("administrator package restore status = %d", status)
	}
}

func TestCargoAdministratorWithL3CanManageTeam(t *testing.T) {
	storagePath := t.TempDir()
	store := newMemoryStore()
	repo := &config.Repository{Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC"}
	state := core.NewAppState()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "cargo-admin-l3.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state.Inner.DB = db
	if err := db.SaveToken(&core.AccessToken{Name: "alice", Identifier: core.AccessTokenIdentifier{Type: core.Persistent}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveToken(&core.AccessToken{Name: "bob", Identifier: core.AccessTokenIdentifier{Type: core.Persistent}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveToken(&core.AccessToken{Name: "admin", Identifier: core.AccessTokenIdentifier{Type: core.Persistent}}); err != nil {
		t.Fatal(err)
	}

	alice := &config.User{Username: "alice", Roles: []string{"canupdate:cargo"}}
	aliceApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, alice)
	crate := makeCrateArchive(t, map[string]string{
		"demo-1.0.0/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"1.0.0\"\n",
	})
	body := makePublishBody(t, PublishMetadata{
		Name: "demo", Version: "1.0.0", Deps: []PublishDependency{}, Features: map[string][]string{},
	}, crate)
	publishResponse, err := aliceApp.Test(httptest.NewRequest(
		http.MethodPut, "http://registry.example/cargo/api/v1/crates/new", bytes.NewReader(body),
	))
	if err != nil {
		t.Fatal(err)
	}
	_ = publishResponse.Body.Close()
	if publishResponse.StatusCode != fiber.StatusOK {
		t.Fatalf("alice publish status = %d", publishResponse.StatusCode)
	}

	admin := &config.User{Username: "admin", Roles: []string{"manager"}}
	inviteAdmin := httptest.NewRequest("PUT", "http://registry.example/cargo/api/v1/crates/demo/owners", strings.NewReader(`{"users":["admin"],"level":3}`))
	inviteAdmin.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	inviteAdminRes, err := aliceApp.Test(inviteAdmin)
	if err != nil {
		t.Fatal(err)
	}
	_ = inviteAdminRes.Body.Close()
	if inviteAdminRes.StatusCode != fiber.StatusOK {
		t.Fatalf("invite admin status = %d", inviteAdminRes.StatusCode)
	}

	messages, err := db.ListMessages("admin", 10, 0, "", time.Now().UnixMilli())
	if err != nil || len(messages) != 1 {
		t.Fatalf("admin messages count = %d, err = %v", len(messages), err)
	}
	adminApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, admin)
	acceptRes, err := adminApp.Test(httptest.NewRequest("POST", "http://registry.example/cargo/api/v1/invitations/"+messages[0].ID+"/accept", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = acceptRes.Body.Close()
	if acceptRes.StatusCode != fiber.StatusOK {
		t.Fatalf("admin accept status = %d", acceptRes.StatusCode)
	}

	inviteBob := httptest.NewRequest("PUT", "http://registry.example/cargo/api/v1/crates/demo/owners", strings.NewReader(`{"users":["bob"],"level":2}`))
	inviteBob.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	inviteBobRes, err := adminApp.Test(inviteBob)
	if err != nil {
		t.Fatal(err)
	}
	_ = inviteBobRes.Body.Close()
	if inviteBobRes.StatusCode != fiber.StatusOK {
		t.Fatalf("admin invite bob status = %d, want 200", inviteBobRes.StatusCode)
	}

	overwriteBob := httptest.NewRequest("PUT", "http://registry.example/cargo/api/v1/crates/demo/owners", strings.NewReader(`{"users":["bob"],"level":3}`))
	overwriteBob.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	overwriteBobRes, err := adminApp.Test(overwriteBob)
	if err != nil {
		t.Fatal(err)
	}
	_ = overwriteBobRes.Body.Close()
	if overwriteBobRes.StatusCode != fiber.StatusConflict {
		t.Fatalf("admin overwrite existing member status = %d, want 409", overwriteBobRes.StatusCode)
	}
	details, err := db.GetCargoPackageDetails("cargo", "demo", "alice")
	if err != nil {
		t.Fatal(err)
	}
	bobUserID := ""
	aliceUserID := ""
	for _, member := range details.Members {
		switch member.Username {
		case "bob":
			bobUserID = member.UserID
		case "alice":
			aliceUserID = member.UserID
		}
	}
	if bobUserID == "" || aliceUserID == "" {
		t.Fatalf("expected immutable Cargo member IDs, got %+v", details.Members)
	}

	bob := &config.User{Username: "bob", Roles: []string{"base"}}
	bobApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, bob)
	leaveBobRes, err := bobApp.Test(httptest.NewRequest(
		http.MethodDelete, "http://registry.example/cargo/api/v1/crates/demo/owners/"+bobUserID, nil,
	))
	if err != nil {
		t.Fatal(err)
	}
	_ = leaveBobRes.Body.Close()
	if leaveBobRes.StatusCode != fiber.StatusOK {
		t.Fatalf("ordinary member leave status = %d, want 200", leaveBobRes.StatusCode)
	}

	ownerLeaveRes, err := aliceApp.Test(httptest.NewRequest(
		http.MethodDelete, "http://registry.example/cargo/api/v1/crates/demo/owners/"+aliceUserID, nil,
	))
	if err != nil {
		t.Fatal(err)
	}
	_ = ownerLeaveRes.Body.Close()
	if ownerLeaveRes.StatusCode != fiber.StatusConflict {
		t.Fatalf("L4 owner leave status = %d, want 409", ownerLeaveRes.StatusCode)
	}
}
