/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"bytes"
	"encoding/binary"
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

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
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
		"demo-1.2.3/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"1.2.3\"\n",
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
	status, _ = publish()
	if status != fiber.StatusConflict {
		t.Fatalf("duplicate publish status = %d, want %d", status, fiber.StatusConflict)
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

	type publishResult struct {
		status int
		err    error
	}
	start := make(chan struct{})
	results := make(chan publishResult, len(names))
	for _, body := range bodies {
		body := body
		go func() {
			<-start
			response, err := app.Test(httptest.NewRequest(
				http.MethodPut, "http://registry.example/cargo/api/v1/crates/new", bytes.NewReader(body),
			))
			if err != nil {
				results <- publishResult{err: err}
				return
			}
			defer response.Body.Close()
			results <- publishResult{status: response.StatusCode}
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
	repo := &config.Repository{Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC"}
	admin := &config.User{Username: "admin", Roles: []string{"manager", "canupdate:cargo"}}
	app := cargoTestApp(t, Handler{Store: newMemoryStore()}, state, repo, t.TempDir(), admin)
	crate := makeCrateArchive(t, map[string]string{
		"demo-1.0.0/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"1.0.0\"\n",
	})
	body := makePublishBody(t, PublishMetadata{Name: "demo", Version: "1.0.0", Deps: []PublishDependency{}, Features: map[string][]string{}}, crate)
	request := httptest.NewRequest("PUT", "http://registry.example/cargo/api/v1/crates/new", bytes.NewReader(body))
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusForbidden {
		t.Fatalf("administrator publish status = %d, want 403", response.StatusCode)
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

	owner := &config.User{Username: "owner", Roles: []string{"canupdate:cargo"}}
	ownerApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath, owner)
	crate := makeCrateArchive(t, map[string]string{
		"demo-1.0.0/Cargo.toml": "[package]\nname = \"demo\"\nversion = \"1.0.0\"\n",
	})
	body := makePublishBody(t, PublishMetadata{
		Name: "demo", Version: "1.0.0", Description: "Public package", Deps: []PublishDependency{}, Features: map[string][]string{},
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

	guestApp := cargoTestApp(t, Handler{Store: store}, state, repo, storagePath)
	response, err := guestApp.Test(httptest.NewRequest(
		http.MethodGet, "http://registry.example/cargo/api/v1/crates/demo", nil,
	))
	if err != nil {
		t.Fatal(err)
	}
	var payload packageInfoResponse
	if decodeErr := json.NewDecoder(response.Body).Decode(&payload); decodeErr != nil {
		_ = response.Body.Close()
		t.Fatal(decodeErr)
	}
	_ = response.Body.Close()
	if response.StatusCode != fiber.StatusOK || payload.Package == nil || len(payload.Versions) != 1 {
		t.Fatalf("guest package info = status %d, payload %+v", response.StatusCode, payload)
	}
	if payload.Admin || payload.Package.PermissionLevel != 0 || len(payload.Members) != 0 {
		t.Fatalf("guest package info exposed management data: %+v", payload)
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
	if inviteResponse.StatusCode != fiber.StatusForbidden {
		t.Fatalf("administrator team mutation status = %d, want 403", inviteResponse.StatusCode)
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
