/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package docker

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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
	"renop/internal/service/index"
	"renop/internal/service/statistics"
	"renop/internal/testutil"
)

type memoryDockerStore struct {
	blobs     map[string][]byte
	manifests map[string][]byte
	staging   map[string]*bytes.Buffer
}

func newMemoryDockerStore() *memoryDockerStore {
	return &memoryDockerStore{
		blobs:     make(map[string][]byte),
		manifests: make(map[string][]byte),
		staging:   make(map[string]*bytes.Buffer),
	}
}

type memoryStagedBlob struct {
	store *memoryDockerStore
	repo  string
	buf   *bytes.Buffer
	uuid  string
}

func (m *memoryStagedBlob) Write(p []byte) (n int, err error) {
	return m.buf.Write(p)
}

func (m *memoryStagedBlob) WriteAt(p []byte, off int64) (n int, err error) {
	if int(off) > m.buf.Len() {
		pad := make([]byte, int(off)-m.buf.Len())
		m.buf.Write(pad)
	}
	b := m.buf.Bytes()
	if int(off)+len(p) <= len(b) {
		copy(b[off:], p)
		return len(p), nil
	}
	copy(b[off:], p[:len(b)-int(off)])
	m.buf.Write(p[len(b)-int(off):])
	return len(p), nil
}

func (m *memoryStagedBlob) Size() (int64, error) {
	return int64(m.buf.Len()), nil
}

func (m *memoryStagedBlob) Close() error {
	return nil
}

func (m *memoryStagedBlob) Discard() error {
	m.buf.Reset()
	if m.store != nil {
		delete(m.store.staging, m.repo+"/"+m.uuid)
	}
	return nil
}

func (m *memoryStagedBlob) Digest() (string, error) {
	return CalculateDigest(m.buf.Bytes()), nil
}

func (s *memoryDockerStore) OpenBlob(repository, digest string) (io.ReadCloser, int64, bool, error) {
	data, ok := s.blobs[repository+"/"+digest]
	if !ok {
		return nil, 0, false, nil
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), true, nil
}

func (s *memoryDockerStore) BlobExists(repository, digest string) (bool, int64, error) {
	data, ok := s.blobs[repository+"/"+digest]
	if !ok {
		return false, 0, nil
	}
	return true, int64(len(data)), nil
}

func (s *memoryDockerStore) BlobFilePath(repository, digest string) (string, bool) {
	return "", false
}

func (s *memoryDockerStore) StageBlob(repository, uploadUUID string) (StagedBlob, error) {
	buf := new(bytes.Buffer)
	s.staging[repository+"/"+uploadUUID] = buf
	return &memoryStagedBlob{store: s, repo: repository, buf: buf, uuid: uploadUUID}, nil
}

func (s *memoryDockerStore) GetStagedBlob(repository, uploadUUID string) (StagedBlob, error) {
	buf, ok := s.staging[repository+"/"+uploadUUID]
	if !ok {
		return nil, core.ErrDockerBlobUploadNotFound
	}
	return &memoryStagedBlob{store: s, repo: repository, buf: buf, uuid: uploadUUID}, nil
}

func (s *memoryDockerStore) CommitBlob(state *core.AppState, repository, uploadUUID, digest string) (int64, error) {
	buf, ok := s.staging[repository+"/"+uploadUUID]
	if !ok {
		return 0, core.ErrDockerBlobUploadNotFound
	}
	data := buf.Bytes()
	s.blobs[repository+"/"+digest] = data
	delete(s.staging, repository+"/"+uploadUUID)
	return int64(len(data)), nil
}

func (s *memoryDockerStore) DeleteBlob(state *core.AppState, repository, digest string) error {
	delete(s.blobs, repository+"/"+digest)
	return nil
}

func (s *memoryDockerStore) OpenManifest(repository, imageName, digest string) ([]byte, bool, error) {
	data, ok := s.manifests[repository+"/"+imageName+"/"+digest]
	return data, ok, nil
}

func (s *memoryDockerStore) PutManifest(state *core.AppState, repository, imageName, digest string, data []byte) error {
	s.manifests[repository+"/"+imageName+"/"+digest] = data
	return nil
}

func (s *memoryDockerStore) DeleteManifest(state *core.AppState, repository, imageName, digest string) error {
	delete(s.manifests, repository+"/"+imageName+"/"+digest)
	return nil
}

func setupTestDockerApp(t *testing.T) (*fiber.App, *core.AppState, Store) {
	t.Helper()
	dir := testutil.TempDir(t)

	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite",
		Dsn:    filepath.Join(dir, "docker_test.db"),
	})
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	tokenObj := &core.AccessToken{
		Name:        "admin",
		Permissions: []string{"admin"},
	}
	if err := db.SaveToken(tokenObj); err != nil {
		t.Fatalf("save administrator: %v", err)
	}
	if err := db.CreateAPIToken("admin", &core.APIToken{
		ID: "00000000-0000-4000-8000-000000000001", Name: "Docker test token",
		Scopes: []string{
			core.APITokenScopeRepositoryRead, core.APITokenScopeRepositoryPublish,
			core.APITokenScopeRepositoryDelete, core.APITokenScopePackageManage,
		},
		CreatedAt: time.Now().UnixMilli(),
	}, core.HashAPITokenSecret("admin-secret-token")); err != nil {
		t.Fatalf("create administrator API token: %v", err)
	}

	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()
	state.Inner.DockerSecret = []byte("test-secret-1234567890-test-secret")
	state.Inner.DB = db

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: 8080,
		},
		StoragePath: dir,
		Maven: config.MavenSettings{
			Repositories: map[string]*config.Repository{
				"docker-local": {
					Name:       "docker-local",
					Format:     config.RepositoryFormatDocker,
					Visibility: "PUBLIC",
				},
				"docker-private": {
					Name:       "docker-private",
					Format:     config.RepositoryFormatDocker,
					Visibility: "PRIVATE",
				},
			},
		},
	}
	state.Inner.Config.Store(cfg)

	store := newMemoryDockerStore()

	app := fiber.New(fiber.Config{
		JSONEncoder: json.Marshal,
		JSONDecoder: json.Unmarshal,
	})

	app.Use(auth.AuthMiddleware(state))
	SetupDockerRoutes(app, state, store)

	return app, state, store
}

func createTestDockerImage(t *testing.T, state *core.AppState, repository, image string, private bool) {
	t.Helper()
	_, err := state.GetDB().CreateDockerImage(repository, image, "admin", private, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("create Docker image %s/%s: %v", repository, image, err)
	}
}

func TestDockerRegistryFullLifecycle(t *testing.T) {
	app, state, _ := setupTestDockerApp(t)
	createTestDockerImage(t, state, "docker-local", "my-app", false)

	req := httptest.NewRequest(http.MethodGet, "/v2/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET /v2/ failed: %v", err)
	}
	if resp.Header.Get(DockerHeaderVersion) != DockerVersionValue {
		t.Fatalf("missing %s header", DockerHeaderVersion)
	}

	req = httptest.NewRequest(http.MethodGet, "/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/my-app:pull,push", nil)
	req.SetBasicAuth("admin", "admin-secret-token")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("GET /v2/token failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /v2/token, got %d", resp.StatusCode)
	}
	var tokenResp TokenResponse
	_ = json.NewDecoder(resp.Body).Decode(&tokenResp)
	if tokenResp.Token == "" {
		t.Fatal("empty token received")
	}

	req = httptest.NewRequest(http.MethodPost, "/v2/docker-local/my-app/blobs/uploads/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("POST blobs/uploads failed: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", resp.StatusCode)
	}
	uploadUUID := resp.Header.Get(DockerUploadUUID)
	if uploadUUID == "" {
		t.Fatal("missing upload UUID")
	}

	layerData := []byte("hello-docker-layer-data-1234567890")
	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/v2/docker-local/my-app/blobs/uploads/%s", uploadUUID), bytes.NewReader(layerData))
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("PATCH blob upload chunk failed: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", resp.StatusCode)
	}

	expectedBlobDigest := CalculateDigest(layerData)
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/v2/docker-local/my-app/blobs/uploads/%s?digest=%s", uploadUUID, expectedBlobDigest), nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("PUT blob upload commit failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}
	if resp.Header.Get(DockerDigestHeader) != expectedBlobDigest {
		t.Fatalf("expected digest %s, got %s", expectedBlobDigest, resp.Header.Get(DockerDigestHeader))
	}

	req = httptest.NewRequest(http.MethodHead, fmt.Sprintf("/v2/docker-local/my-app/blobs/%s", expectedBlobDigest), nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("HEAD blob failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for HEAD blob, got %d", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v2/docker-local/my-app/blobs/%s", expectedBlobDigest), nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("GET blob failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for GET blob, got %d", resp.StatusCode)
	}
	downloadedBlob, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(downloadedBlob, layerData) {
		t.Fatalf("downloaded blob does not match uploaded: got %s", string(downloadedBlob))
	}

	configData := []byte(`{"architecture":"amd64","os":"linux","rootfs":{"type":"layers"}}`)
	configDigest := CalculateDigest(configData)
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v2/docker-local/my-app/blobs/uploads/?digest=%s", configDigest), bytes.NewReader(configData))
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("Monolithic POST blob failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for monolithic upload, got %d", resp.StatusCode)
	}

	manifestPayload := fmt.Sprintf(`{
		"schemaVersion": 2,
		"mediaType": "%s",
		"config": {
			"mediaType": "%s",
			"size": %d,
			"digest": "%s"
		},
		"layers": [
			{
				"mediaType": "%s",
				"size": %d,
				"digest": "%s"
			}
		]
	}`, MediaTypeDockerManifest2, MediaTypeDockerConfig, len(configData), configDigest, MediaTypeDockerLayer, len(layerData), expectedBlobDigest)

	manifestBytes := []byte(manifestPayload)
	expectedManifestDigest := CalculateDigest(manifestBytes)

	req = httptest.NewRequest(http.MethodPut, "/v2/docker-local/my-app/manifests/v1.0.0", bytes.NewReader(manifestBytes))
	req.Header.Set("Content-Type", MediaTypeDockerManifest2)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("PUT manifest failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for PUT manifest, got %d", resp.StatusCode)
	}
	if resp.Header.Get(DockerDigestHeader) != expectedManifestDigest {
		t.Fatalf("expected manifest digest %s, got %s", expectedManifestDigest, resp.Header.Get(DockerDigestHeader))
	}

	req = httptest.NewRequest(http.MethodPut, "/v2/docker-local/my-app/manifests/latest", bytes.NewReader(manifestBytes))
	req.Header.Set("Content-Type", MediaTypeDockerManifest2)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("PUT manifest tag latest failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodGet, "/v2/docker-local/my-app/manifests/v1.0.0", nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("GET manifest by tag failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for GET manifest, got %d", resp.StatusCode)
	}
	if resp.Header.Get(DockerDigestHeader) != expectedManifestDigest {
		t.Fatalf("expected digest %s, got %s", expectedManifestDigest, resp.Header.Get(DockerDigestHeader))
	}

	req = httptest.NewRequest(http.MethodHead, fmt.Sprintf("/v2/docker-local/my-app/manifests/%s", expectedManifestDigest), nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("HEAD manifest by digest failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for HEAD manifest, got %d", resp.StatusCode)
	}
	if err := statistics.GetCounter(state).Flush(); err != nil {
		t.Fatalf("flush Docker download statistics: %v", err)
	}
	statisticsPage, err := state.GetDB().QueryDownloadStatistics(core.DownloadStatisticsQuery{
		Repository: "docker-local", GroupBy: "version", Limit: 10,
	})
	if err != nil {
		t.Fatalf("query Docker download statistics: %v", err)
	}
	if len(statisticsPage.Records) != 1 {
		t.Fatalf("Docker download statistics records = %#v", statisticsPage.Records)
	}
	record := statisticsPage.Records[0]
	if statisticsPage.Count != 1 || record.Package != "my-app" || record.Version != "v1.0.0" ||
		record.Bytes != int64(len(manifestBytes)) {
		t.Fatalf("Docker download statistics: count=%d package=%q version=%q bytes=%d, want bytes=%d",
			statisticsPage.Count, record.Package, record.Version, record.Bytes, len(manifestBytes))
	}

	req = httptest.NewRequest(http.MethodGet, "/v2/docker-local/my-app/tags/list", nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("GET tags list failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for tags list, got %d", resp.StatusCode)
	}
	var tagList TagList
	_ = json.NewDecoder(resp.Body).Decode(&tagList)
	if len(tagList.Tags) != 2 {
		t.Fatalf("expected 2 tags (v1.0.0, latest), got: %v", tagList.Tags)
	}

	req = httptest.NewRequest(http.MethodGet, "/v2/_catalog", nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("GET _catalog failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for _catalog, got %d", resp.StatusCode)
	}
	var catalog CatalogList
	_ = json.NewDecoder(resp.Body).Decode(&catalog)
	if len(catalog.Repositories) != 1 || catalog.Repositories[0] != "docker-local/my-app" {
		t.Fatalf("unexpected catalog list: %v", catalog.Repositories)
	}

	req = httptest.NewRequest(http.MethodDelete, "/v2/docker-local/my-app/manifests/v1.0.0", nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("DELETE tag failed: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted for DELETE tag, got %d", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/v2/docker-local/my-app/blobs/%s", expectedBlobDigest), nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.Token)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("DELETE blob failed: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted for DELETE blob, got %d", resp.StatusCode)
	}
}

func TestDockerManifestEnforcesPublicationQuotaBeforeStorageCommit(t *testing.T) {
	app, state, store := setupTestDockerApp(t)
	createTestDockerImage(t, state, "docker-local", "quota-app", false)
	next := state.Inner.Config.Load().DeepCopy()
	next.PublicationQuota = config.PublicationQuotaConfig{
		FileLimit: 1, ByteLimit: 1 << 20, PublicationLimit: 10, Period: core.PublicationQuotaPeriodMonth,
	}
	state.Inner.Config.Store(next)
	tokenRequest := httptest.NewRequest(http.MethodGet,
		"/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/quota-app:pull,push", nil)
	tokenRequest.SetBasicAuth("admin", "admin-secret-token")
	tokenResponse, err := app.Test(tokenRequest)
	if err != nil {
		t.Fatal(err)
	}
	var token TokenResponse
	if err := json.NewDecoder(tokenResponse.Body).Decode(&token); err != nil {
		t.Fatal(err)
	}
	_ = tokenResponse.Body.Close()

	manifest := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","size":2,"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"layers":[]}`)
	request := httptest.NewRequest(http.MethodPut, "/v2/docker-local/quota-app/manifests/latest", bytes.NewReader(manifest))
	request.Header.Set(fiber.HeaderContentType, MediaTypeOCIManifest1)
	request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token.Token)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("manifest quota status = %d, want %d", response.StatusCode, fiber.StatusTooManyRequests)
	}
	if code := response.Header.Get("X-Renop-Error-Code"); code != "publication_file_quota" {
		t.Fatalf("manifest quota code = %q", code)
	}
	if memoryStore, ok := store.(*memoryDockerStore); !ok || len(memoryStore.manifests) != 0 {
		t.Fatal("quota-rejected manifest reached storage")
	}
}

func TestDockerPublicationReviewDefersManifestAndTags(t *testing.T) {
	app, state, store := setupTestDockerApp(t)
	createTestDockerImage(t, state, "docker-local", "reviewed-app", false)
	repo := state.Inner.Config.Load().Maven.Repositories["docker-local"]
	repo.PublicationReview = config.PublicationReviewEveryVersion

	tokenRequest := httptest.NewRequest(http.MethodGet,
		"/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/reviewed-app:pull,push", nil)
	tokenRequest.SetBasicAuth("admin", "admin-secret-token")
	tokenResponse, err := app.Test(tokenRequest)
	if err != nil {
		t.Fatalf("request Docker token: %v", err)
	}
	var token TokenResponse
	if err := json.NewDecoder(tokenResponse.Body).Decode(&token); err != nil {
		t.Fatalf("decode Docker token: %v", err)
	}
	_ = tokenResponse.Body.Close()

	publish := func(reference, marker string) *http.Response {
		t.Helper()
		manifest := []byte(fmt.Sprintf(`{
			"schemaVersion":2,
			"mediaType":%q,
			"config":{},
			"layers":[],
			"annotations":{"test.marker":%q}
		}`, MediaTypeDockerManifest2, marker))
		request := httptest.NewRequest(http.MethodPut,
			"/v2/docker-local/reviewed-app/manifests/"+reference, bytes.NewReader(manifest))
		request.Header.Set(fiber.HeaderContentType, MediaTypeDockerManifest2)
		request.Header.Set(fiber.HeaderAuthorization, "Bearer "+token.Token)
		response, requestErr := app.Test(request)
		if requestErr != nil {
			t.Fatalf("publish Docker manifest: %v", requestErr)
		}
		return response
	}

	response := publish("1.0.0", "pending")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("reviewed manifest status = %d, want %d", response.StatusCode, http.StatusAccepted)
	}
	reviewID := response.Header.Get("X-RenoP-Review-ID")
	manifestDigest := response.Header.Get(DockerDigestHeader)
	_ = response.Body.Close()
	if reviewID == "" || manifestDigest == "" {
		t.Fatalf("review response headers: review=%q digest=%q", reviewID, manifestDigest)
	}
	if _, found, err := store.OpenManifest("docker-local", "reviewed-app", manifestDigest); err != nil || found {
		t.Fatalf("pending manifest storage visibility: found=%t err=%v", found, err)
	}
	tag, err := state.GetDB().GetDockerTag("docker-local", "reviewed-app", "1.0.0")
	if err != nil || tag != nil {
		t.Fatalf("pending Docker tag was public: %#v, %v", tag, err)
	}
	details, err := state.GetDB().GetDockerImageDetails("docker-local", "reviewed-app", "admin")
	if err != nil {
		t.Fatalf("load reviewed image: %v", err)
	}
	if err := AddPendingPublicationTags(state, details); err != nil {
		t.Fatalf("add pending Docker tags: %v", err)
	}
	if len(details.Tags) != 1 || details.Tags[0].ReviewStatus != core.ReviewStatusPending {
		t.Fatalf("pending Docker tags = %#v", details.Tags)
	}

	reviewTask, err := state.GetDB().GetReviewTask(reviewID)
	if err != nil {
		t.Fatalf("load Docker review: %v", err)
	}
	if err := state.GetDB().SaveToken(&core.AccessToken{
		Name: "moderator", Permissions: []string{"base", "canmoderate:docker-local"},
	}); err != nil {
		t.Fatalf("save Docker moderator: %v", err)
	}
	decided, err := ApprovePublicationReview(state, reviewTask, store, "moderator",
		reviewTask.UpdatedAt+core.PublicationReviewSettleMillis+1)
	if err != nil {
		t.Fatalf("approve Docker publication: %v", err)
	}
	if decided.Status != core.ReviewStatusApproved {
		t.Fatalf("Docker review status = %q", decided.Status)
	}
	if _, found, err := store.OpenManifest("docker-local", "reviewed-app", manifestDigest); err != nil || !found {
		t.Fatalf("approved manifest storage visibility: found=%t err=%v", found, err)
	}
	tag, err = state.GetDB().GetDockerTag("docker-local", "reviewed-app", "1.0.0")
	if err != nil || tag == nil || tag.Digest != manifestDigest {
		t.Fatalf("approved Docker tag = %#v, %v", tag, err)
	}

	repo.PublicationReview = config.PublicationReviewNewPackages
	response = publish("2.0.0", "visible")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("existing image manifest status = %d, want %d", response.StatusCode, http.StatusCreated)
	}
	if response.Header.Get("X-RenoP-Review-ID") != "" {
		t.Fatal("existing image unexpectedly entered new-package review")
	}
	_ = response.Body.Close()

	repo.PublicationReview = config.PublicationReviewEveryVersion
	missingDigest := "sha256:" + strings.Repeat("a", 64)
	missingManifestJSON := []byte(fmt.Sprintf(`{
		"schemaVersion":2,
		"mediaType":%q,
		"config":{"digest":%q},
		"layers":[]
	}`, MediaTypeDockerManifest2, missingDigest))
	missingManifest, err := ParseManifest(missingManifestJSON, MediaTypeDockerManifest2)
	if err != nil {
		t.Fatalf("parse missing-blob manifest: %v", err)
	}
	missingReview, err := QueuePublicationReview(
		state, repo, "reviewed-app", "3.0.0", missingManifest, "admin", true, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("queue missing-blob manifest: %v", err)
	}
	missingTask, err := state.GetDB().GetReviewTask(missingReview.TaskID)
	if err != nil {
		t.Fatalf("load missing-blob review: %v", err)
	}
	_, err = ApprovePublicationReview(state, missingTask, store, "moderator",
		missingTask.UpdatedAt+core.PublicationReviewSettleMillis+1)
	if !errors.Is(err, core.ErrReviewResourceConflict) {
		t.Fatalf("missing-blob approval error = %v", err)
	}
	missingTask, err = state.GetDB().GetReviewTask(missingReview.TaskID)
	if err != nil || missingTask.Status != core.ReviewStatusPending {
		t.Fatalf("missing-blob review state = %#v, %v", missingTask, err)
	}
	if _, found, err := store.OpenManifest(
		"docker-local", "reviewed-app", missingManifest.Digest); err != nil || found {
		t.Fatalf("missing-blob manifest visibility: found=%t err=%v", found, err)
	}
}

func TestDockerRegistrySecurityAndErrors(t *testing.T) {
	app, state, _ := setupTestDockerApp(t)
	createTestDockerImage(t, state, "docker-local", "test", false)
	createTestDockerImage(t, state, "docker-local", "private-app", true)
	createTestDockerImage(t, state, "docker-local", "mount-target", false)
	createTestDockerImage(t, state, "docker-private", "secret-app", true)
	if err := state.GetDB().SaveToken(&core.AccessToken{
		Name: "bob", Tokens: []string{"bob-secret-token"}, Permissions: []string{"base"},
	}); err != nil {
		t.Fatalf("save Bob token: %v", err)
	}
	missingTokenReq := httptest.NewRequest(http.MethodGet,
		"/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/not-created:pull,push", nil)
	missingTokenReq.SetBasicAuth("admin", "admin-secret-token")
	missingTokenResp, err := app.Test(missingTokenReq)
	if err != nil {
		t.Fatalf("request missing-image token: %v", err)
	}
	var missingToken TokenResponse
	if err := json.NewDecoder(missingTokenResp.Body).Decode(&missingToken); err != nil {
		t.Fatalf("decode missing-image token: %v", err)
	}
	missingUploadReq := httptest.NewRequest(http.MethodPost,
		"/v2/docker-local/not-created/blobs/uploads/", nil)
	missingUploadReq.Header.Set("Authorization", "Bearer "+missingToken.Token)
	missingUploadResp, err := app.Test(missingUploadReq)
	if err != nil {
		t.Fatalf("attempt missing-image upload: %v", err)
	}
	if missingUploadResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected missing-image push rejected with 403, got %d", missingUploadResp.StatusCode)
	}
	missingImage, err := state.GetDB().GetDockerImage("docker-local", "not-created")
	if err != nil || missingImage != nil {
		t.Fatalf("missing-image push created metadata: image=%+v err=%v", missingImage, err)
	}
	privateTokenReq := httptest.NewRequest(http.MethodGet,
		"/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/private-app:pull,push", nil)
	privateTokenReq.SetBasicAuth("admin", "admin-secret-token")
	privateTokenResp, err := app.Test(privateTokenReq)
	if err != nil {
		t.Fatalf("request private-image owner token: %v", err)
	}
	var privateToken TokenResponse
	if err := json.NewDecoder(privateTokenResp.Body).Decode(&privateToken); err != nil {
		t.Fatalf("decode private-image owner token: %v", err)
	}
	privateBlob := []byte("private-image-layer")
	privateDigest := CalculateDigest(privateBlob)
	privateUploadReq := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/v2/docker-local/private-app/blobs/uploads/?digest=%s", privateDigest), bytes.NewReader(privateBlob))
	privateUploadReq.Header.Set("Authorization", "Bearer "+privateToken.Token)
	privateUploadResp, err := app.Test(privateUploadReq)
	if err != nil || privateUploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("upload private-image blob: status=%d err=%v", privateUploadResp.StatusCode, err)
	}

	publicTokenReq := httptest.NewRequest(http.MethodGet,
		"/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/test:pull", nil)
	publicTokenReq.SetBasicAuth("bob", "bob-secret-token")
	publicTokenResp, err := app.Test(publicTokenReq)
	if err != nil {
		t.Fatalf("request public-image reader token: %v", err)
	}
	var publicToken TokenResponse
	if err := json.NewDecoder(publicTokenResp.Body).Decode(&publicToken); err != nil {
		t.Fatalf("decode public-image reader token: %v", err)
	}
	pivotReq := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/v2/docker-local/test/blobs/%s", privateDigest), nil)
	pivotReq.Header.Set("Authorization", "Bearer "+publicToken.Token)
	pivotResp, err := app.Test(pivotReq)
	if err != nil || pivotResp.StatusCode != http.StatusNotFound {
		t.Fatalf("private blob was readable through public image: status=%d err=%v", pivotResp.StatusCode, err)
	}
	mountPivotReq := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/v2/docker-local/mount-target/blobs/uploads/?mount=%s&from=docker-local/test", privateDigest), nil)
	mountPivotReq.SetBasicAuth("admin", "admin-secret-token")
	mountPivotResp, err := app.Test(mountPivotReq)
	if err != nil || mountPivotResp.StatusCode != http.StatusAccepted {
		t.Fatalf("unreferenced private blob mount did not fall back to upload: status=%d err=%v", mountPivotResp.StatusCode, err)
	}
	targetReferencesPrivateBlob, err := state.GetDB().DockerImageReferencesBlob("docker-local", "mount-target", privateDigest)
	if err != nil || targetReferencesPrivateBlob {
		t.Fatalf("unreferenced private blob was linked to public target: referenced=%t err=%v", targetReferencesPrivateBlob, err)
	}

	if err := state.GetDB().ForceAddDockerMembers("docker-local", "private-app", "admin", []string{"bob"}, core.DockerPermissionRead); err != nil {
		t.Fatalf("grant private-image L0 access: %v", err)
	}
	privateReaderTokenReq := httptest.NewRequest(http.MethodGet,
		"/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/private-app:pull", nil)
	privateReaderTokenReq.SetBasicAuth("bob", "bob-secret-token")
	privateReaderTokenResp, err := app.Test(privateReaderTokenReq)
	if err != nil {
		t.Fatalf("request private-image L0 token: %v", err)
	}
	var privateReaderToken TokenResponse
	if err := json.NewDecoder(privateReaderTokenResp.Body).Decode(&privateReaderToken); err != nil {
		t.Fatalf("decode private-image L0 token: %v", err)
	}
	privateReadReq := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/v2/docker-local/private-app/blobs/%s", privateDigest), nil)
	privateReadReq.Header.Set("Authorization", "Bearer "+privateReaderToken.Token)
	privateReadResp, err := app.Test(privateReadReq)
	if err != nil || privateReadResp.StatusCode != http.StatusOK {
		t.Fatalf("private-image L0 member could not read blob: status=%d err=%v", privateReadResp.StatusCode, err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v2/docker-private/secret-app/manifests/latest", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET private repo manifest failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for private repo, got %d", resp.StatusCode)
	}
	wwwAuth := resp.Header.Get("Www-Authenticate")
	if wwwAuth == "" || !bytes.Contains([]byte(wwwAuth), []byte("Bearer realm=")) {
		t.Fatalf("missing or invalid WWW-Authenticate challenge: %s", wwwAuth)
	}

	tokenReq := httptest.NewRequest(http.MethodGet, "/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/test:pull,push", nil)
	tokenReq.SetBasicAuth("admin", "admin-secret-token")
	tokenResp, _ := app.Test(tokenReq)
	var tok TokenResponse
	_ = json.NewDecoder(tokenResp.Body).Decode(&tok)

	uploadReq := httptest.NewRequest(http.MethodPost, "/v2/docker-local/test/blobs/uploads/", nil)
	uploadReq.Header.Set("Authorization", "Bearer "+tok.Token)
	uploadResp, _ := app.Test(uploadReq)
	uploadUUID := uploadResp.Header.Get(DockerUploadUUID)

	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/v2/docker-local/test/blobs/uploads/%s", uploadUUID), bytes.NewReader([]byte("sample-data")))
	patchReq.Header.Set("Authorization", "Bearer "+tok.Token)
	_, _ = app.Test(patchReq)

	putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/v2/docker-local/test/blobs/uploads/%s?digest=sha256:wrongdigest00000000000000000000000000000000000000000000000000000", uploadUUID), nil)
	putReq.Header.Set("Authorization", "Bearer "+tok.Token)
	putResp, err := app.Test(putReq)
	if err != nil {
		t.Fatalf("PUT wrong digest failed: %v", err)
	}
	if putResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for mismatched digest, got %d", putResp.StatusCode)
	}

	nonExistentReq := httptest.NewRequest(http.MethodGet, "/v2/docker-local/test/blobs/sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", nil)
	nonExistentReq.Header.Set("Authorization", "Bearer "+tok.Token)
	nonExistentResp, err := app.Test(nonExistentReq)
	if err != nil {
		t.Fatalf("GET non-existent blob failed: %v", err)
	}
	if nonExistentResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", nonExistentResp.StatusCode)
	}
}

func TestDockerMonolithicBlobUpload(t *testing.T) {
	app, state, _ := setupTestDockerApp(t)
	createTestDockerImage(t, state, "docker-local", "monolithic-app", false)

	tokenReq := httptest.NewRequest(http.MethodGet, "/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/monolithic-app:pull,push", nil)
	tokenReq.SetBasicAuth("admin", "admin-secret-token")
	tokenResp, _ := app.Test(tokenReq)
	var tok TokenResponse
	_ = json.NewDecoder(tokenResp.Body).Decode(&tok)

	blobPayload := []byte("monolithic-single-shot-upload-data")
	digest := CalculateDigest(blobPayload)

	postURL := fmt.Sprintf("/v2/docker-local/monolithic-app/blobs/uploads/?digest=%s", digest)
	postReq := httptest.NewRequest(http.MethodPost, postURL, bytes.NewReader(blobPayload))
	postReq.Header.Set("Authorization", "Bearer "+tok.Token)
	postReq.Header.Set("Content-Type", "application/octet-stream")

	postResp, err := app.Test(postReq)
	if err != nil {
		t.Fatalf("Monolithic POST blob upload failed: %v", err)
	}
	if postResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for monolithic upload, got %d", postResp.StatusCode)
	}

	location := postResp.Header.Get("Location")
	if !strings.Contains(location, digest) {
		t.Fatalf("expected Location header containing %s, got %s", digest, location)
	}

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v2/docker-local/monolithic-app/blobs/%s", digest), nil)
	getReq.Header.Set("Authorization", "Bearer "+tok.Token)
	getResp, err := app.Test(getReq)
	if err != nil || getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for retrieved monolithic blob, got %d (err: %v)", getResp.StatusCode, err)
	}
	body, _ := io.ReadAll(getResp.Body)
	if string(body) != string(blobPayload) {
		t.Fatalf("blob content mismatch: got %q, expected %q", string(body), string(blobPayload))
	}
}

func TestDockerChunkedUploadRangeAndDiscard(t *testing.T) {
	app, state, _ := setupTestDockerApp(t)
	createTestDockerImage(t, state, "docker-local", "chunked-app", false)

	tokenReq := httptest.NewRequest(http.MethodGet, "/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/chunked-app:pull,push", nil)
	tokenReq.SetBasicAuth("admin", "admin-secret-token")
	tokenResp, _ := app.Test(tokenReq)
	var tok TokenResponse
	_ = json.NewDecoder(tokenResp.Body).Decode(&tok)

	startReq := httptest.NewRequest(http.MethodPost, "/v2/docker-local/chunked-app/blobs/uploads/", nil)
	startReq.Header.Set("Authorization", "Bearer "+tok.Token)
	startResp, _ := app.Test(startReq)
	uploadUUID := startResp.Header.Get(DockerUploadUUID)

	statusReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v2/docker-local/chunked-app/blobs/uploads/%s", uploadUUID), nil)
	statusReq.Header.Set("Authorization", "Bearer "+tok.Token)
	statusResp, _ := app.Test(statusReq)
	if statusResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for upload status, got %d", statusResp.StatusCode)
	}

	chunk1 := []byte("chunk-one-")
	patchReq := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/v2/docker-local/chunked-app/blobs/uploads/%s", uploadUUID), bytes.NewReader(chunk1))
	patchReq.Header.Set("Authorization", "Bearer "+tok.Token)
	patchReq.Header.Set("Content-Range", fmt.Sprintf("0-%d", len(chunk1)-1))
	patchResp, _ := app.Test(patchReq)
	if patchResp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted on PATCH, got %d", patchResp.StatusCode)
	}

	statusReq2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v2/docker-local/chunked-app/blobs/uploads/%s", uploadUUID), nil)
	statusReq2.Header.Set("Authorization", "Bearer "+tok.Token)
	statusResp2, _ := app.Test(statusReq2)
	rangeHdr := statusResp2.Header.Get("Range")
	expectedRange := fmt.Sprintf("0-%d", len(chunk1))
	if rangeHdr != expectedRange {
		t.Fatalf("expected Range %s, got %s", expectedRange, rangeHdr)
	}

	// 5. Discard upload session
	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/v2/docker-local/chunked-app/blobs/uploads/%s", uploadUUID), nil)
	delReq.Header.Set("Authorization", "Bearer "+tok.Token)
	delResp, err := app.Test(delReq)
	if err != nil || delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 No Content on DELETE upload session, got %d (err: %v)", delResp.StatusCode, err)
	}

	// 6. Query status after discard -> should be 404
	statusReq3 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v2/docker-local/chunked-app/blobs/uploads/%s", uploadUUID), nil)
	statusReq3.Header.Set("Authorization", "Bearer "+tok.Token)
	statusResp3, _ := app.Test(statusReq3)
	if statusResp3.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found after upload discard, got %d", statusResp3.StatusCode)
	}
}

func TestDockerCrossRepositoryBlobMount(t *testing.T) {
	app, state, _ := setupTestDockerApp(t)
	createTestDockerImage(t, state, "docker-local", "source-app", false)
	createTestDockerImage(t, state, "docker-local", "dest-app", false)

	// Auth for source and dest
	tokenReq := httptest.NewRequest(http.MethodGet, "/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/source-app:pull,push&scope=repository:docker-local/dest-app:pull,push", nil)
	tokenReq.SetBasicAuth("admin", "admin-secret-token")
	tokenResp, _ := app.Test(tokenReq)
	var tok TokenResponse
	_ = json.NewDecoder(tokenResp.Body).Decode(&tok)

	// Push blob to source-app
	sharedPayload := []byte("shared-layer-across-multiple-repos")
	sharedDigest := CalculateDigest(sharedPayload)

	uploadReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v2/docker-local/source-app/blobs/uploads/?digest=%s", sharedDigest), bytes.NewReader(sharedPayload))
	uploadReq.Header.Set("Authorization", "Bearer "+tok.Token)
	uploadResp, _ := app.Test(uploadReq)
	if uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for source upload, got %d", uploadResp.StatusCode)
	}

	// Mount existing blob into dest-app
	mountURL := fmt.Sprintf("/v2/docker-local/dest-app/blobs/uploads/?mount=%s&from=docker-local/source-app", sharedDigest)
	mountReq := httptest.NewRequest(http.MethodPost, mountURL, nil)
	mountReq.Header.Set("Authorization", "Bearer "+tok.Token)
	mountResp, err := app.Test(mountReq)
	if err != nil {
		t.Fatalf("Mount request failed: %v", err)
	}
	if mountResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created on successful blob mount, got %d", mountResp.StatusCode)
	}

	// Try mounting non-existent blob -> should fallback to 202 Accepted upload session
	badMountURL := "/v2/docker-local/dest-app/blobs/uploads/?mount=sha256:0000000000000000000000000000000000000000000000000000000000000000&from=docker-local/source-app"
	badMountReq := httptest.NewRequest(http.MethodPost, badMountURL, nil)
	badMountReq.Header.Set("Authorization", "Bearer "+tok.Token)
	badMountResp, err := app.Test(badMountReq)
	if err != nil {
		t.Fatalf("Failed mount fallback request: %v", err)
	}
	if badMountResp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted fallback on missing blob mount, got %d", badMountResp.StatusCode)
	}
}

func TestDockerHeadRequestsAndDigests(t *testing.T) {
	app, state, _ := setupTestDockerApp(t)
	createTestDockerImage(t, state, "docker-local", "head-app", false)

	tokenReq := httptest.NewRequest(http.MethodGet, "/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/head-app:pull,push", nil)
	tokenReq.SetBasicAuth("admin", "admin-secret-token")
	tokenResp, _ := app.Test(tokenReq)
	var tok TokenResponse
	_ = json.NewDecoder(tokenResp.Body).Decode(&tok)

	blobPayload := []byte("head-request-blob-content")
	blobDigest := CalculateDigest(blobPayload)

	upReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v2/docker-local/head-app/blobs/uploads/?digest=%s", blobDigest), bytes.NewReader(blobPayload))
	upReq.Header.Set("Authorization", "Bearer "+tok.Token)
	_, _ = app.Test(upReq)

	manifestJSON := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		"config": {
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size": %d,
			"digest": "%s"
		},
		"layers": [
			{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size": %d,
				"digest": "%s"
			}
		]
	}`, len(blobPayload), blobDigest, len(blobPayload), blobDigest))
	manifestDigest := CalculateDigest(manifestJSON)

	putManifestReq := httptest.NewRequest(http.MethodPut, "/v2/docker-local/head-app/manifests/v1.0.0", bytes.NewReader(manifestJSON))
	putManifestReq.Header.Set("Authorization", "Bearer "+tok.Token)
	putManifestReq.Header.Set("Content-Type", MediaTypeDockerManifest2)
	_, _ = app.Test(putManifestReq)

	headBlobReq := httptest.NewRequest(http.MethodHead, fmt.Sprintf("/v2/docker-local/head-app/blobs/%s", blobDigest), nil)
	headBlobReq.Header.Set("Authorization", "Bearer "+tok.Token)
	headBlobResp, err := app.Test(headBlobReq)
	if err != nil || headBlobResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on HEAD blob, got %d (err: %v)", headBlobResp.StatusCode, err)
	}
	if headBlobResp.Header.Get(DockerDigestHeader) != blobDigest {
		t.Fatalf("expected digest %s, got %s", blobDigest, headBlobResp.Header.Get(DockerDigestHeader))
	}
	blobBody, _ := io.ReadAll(headBlobResp.Body)
	if len(blobBody) != 0 {
		t.Fatal("HEAD response body must be empty")
	}

	headBadBlobReq := httptest.NewRequest(http.MethodHead, "/v2/docker-local/head-app/blobs/sha256:0000000000000000000000000000000000000000000000000000000000000000", nil)
	headBadBlobReq.Header.Set("Authorization", "Bearer "+tok.Token)
	headBadBlobResp, _ := app.Test(headBadBlobReq)
	if headBadBlobResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found on HEAD non-existent blob, got %d", headBadBlobResp.StatusCode)
	}

	headManifestReq := httptest.NewRequest(http.MethodHead, "/v2/docker-local/head-app/manifests/v1.0.0", nil)
	headManifestReq.Header.Set("Authorization", "Bearer "+tok.Token)
	headManifestResp, err := app.Test(headManifestReq)
	if err != nil || headManifestResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on HEAD manifest, got %d (err: %v)", headManifestResp.StatusCode, err)
	}
	if headManifestResp.Header.Get(DockerDigestHeader) != manifestDigest {
		t.Fatalf("expected digest %s, got %s", manifestDigest, headManifestResp.Header.Get(DockerDigestHeader))
	}
	manifestBody, _ := io.ReadAll(headManifestResp.Body)
	if len(manifestBody) != 0 {
		t.Fatal("HEAD response body must be empty")
	}

	headByDigestReq := httptest.NewRequest(http.MethodHead, fmt.Sprintf("/v2/docker-local/head-app/manifests/%s", manifestDigest), nil)
	headByDigestReq.Header.Set("Authorization", "Bearer "+tok.Token)
	headByDigestResp, err := app.Test(headByDigestReq)
	if err != nil || headByDigestResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on HEAD manifest by digest, got %d (err: %v)", headByDigestResp.StatusCode, err)
	}

	headBadManifestReq := httptest.NewRequest(http.MethodHead, "/v2/docker-local/head-app/manifests/v9.9.9", nil)
	headBadManifestReq.Header.Set("Authorization", "Bearer "+tok.Token)
	headBadManifestResp, _ := app.Test(headBadManifestReq)
	if headBadManifestResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found on HEAD non-existent manifest, got %d", headBadManifestResp.StatusCode)
	}
}

func TestDockerCatalogAndTagsPagination(t *testing.T) {
	app, state, _ := setupTestDockerApp(t)
	createTestDockerImage(t, state, "docker-local", "app-1", false)
	createTestDockerImage(t, state, "docker-local", "app-2", false)

	tokenReq := httptest.NewRequest(http.MethodGet, "/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/app-1:pull,push&scope=repository:docker-local/app-2:pull,push", nil)
	tokenReq.SetBasicAuth("admin", "admin-secret-token")
	tokenResp, _ := app.Test(tokenReq)
	var tok TokenResponse
	_ = json.NewDecoder(tokenResp.Body).Decode(&tok)

	pushManifest := func(img, tag string) {
		payload := []byte(fmt.Sprintf(`{"schemaVersion": 2, "mediaType": "application/vnd.docker.distribution.manifest.v2+", "tag": "%s"}`, tag))
		putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/v2/docker-local/%s/manifests/%s", img, tag), bytes.NewReader(payload))
		putReq.Header.Set("Authorization", "Bearer "+tok.Token)
		putReq.Header.Set("Content-Type", MediaTypeDockerManifest2)
		_, _ = app.Test(putReq)
	}

	pushManifest("app-1", "v1.0.0")
	pushManifest("app-1", "v2.0.0")
	pushManifest("app-1", "v3.0.0")
	pushManifest("app-2", "latest")

	catReq := httptest.NewRequest(http.MethodGet, "/v2/_catalog?n=1", nil)
	catReq.Header.Set("Authorization", "Bearer "+tok.Token)
	catResp, err := app.Test(catReq)
	if err != nil || catResp.StatusCode != http.StatusOK {
		t.Fatalf("Catalog request failed: %v (code: %d)", err, catResp.StatusCode)
	}
	linkHdr := catResp.Header.Get("Link")
	if !strings.Contains(linkHdr, `rel="next"`) {
		t.Fatalf("expected Link header with rel=next, got: %s", linkHdr)
	}

	var cat CatalogList
	_ = json.NewDecoder(catResp.Body).Decode(&cat)
	if len(cat.Repositories) != 1 {
		t.Fatalf("expected 1 repository in page, got %d", len(cat.Repositories))
	}

	tagsReq := httptest.NewRequest(http.MethodGet, "/v2/docker-local/app-1/tags/list?n=2", nil)
	tagsReq.Header.Set("Authorization", "Bearer "+tok.Token)
	tagsResp, err := app.Test(tagsReq)
	if err != nil || tagsResp.StatusCode != http.StatusOK {
		t.Fatalf("Tags list request failed: %v (code: %d)", err, tagsResp.StatusCode)
	}
	tagsLinkHdr := tagsResp.Header.Get("Link")
	if !strings.Contains(tagsLinkHdr, `rel="next"`) {
		t.Fatalf("expected Link header for tags pagination, got: %s", tagsLinkHdr)
	}

	var tagsRespObj TagList
	_ = json.NewDecoder(tagsResp.Body).Decode(&tagsRespObj)
	if len(tagsRespObj.Tags) != 2 {
		t.Fatalf("expected 2 tags in page, got %d", len(tagsRespObj.Tags))
	}
}

func TestDockerMultiTagSameManifestLifecycle(t *testing.T) {
	app, state, _ := setupTestDockerApp(t)
	createTestDockerImage(t, state, "docker-local", "multi-tag-app", false)

	tokenReq := httptest.NewRequest(http.MethodGet, "/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/multi-tag-app:pull,push", nil)
	tokenReq.SetBasicAuth("admin", "admin-secret-token")
	tokenResp, _ := app.Test(tokenReq)
	var tok TokenResponse
	_ = json.NewDecoder(tokenResp.Body).Decode(&tok)

	manifestData := []byte(`{"schemaVersion": 2, "mediaType": "application/vnd.docker.distribution.manifest.v2+json", "unique": "test-data"}`)
	manifestDigest := CalculateDigest(manifestData)

	putReq1 := httptest.NewRequest(http.MethodPut, "/v2/docker-local/multi-tag-app/manifests/v1.0.0", bytes.NewReader(manifestData))
	putReq1.Header.Set("Authorization", "Bearer "+tok.Token)
	putReq1.Header.Set("Content-Type", MediaTypeDockerManifest2)
	_, _ = app.Test(putReq1)

	putReq2 := httptest.NewRequest(http.MethodPut, "/v2/docker-local/multi-tag-app/manifests/latest", bytes.NewReader(manifestData))
	putReq2.Header.Set("Authorization", "Bearer "+tok.Token)
	putReq2.Header.Set("Content-Type", MediaTypeDockerManifest2)
	_, _ = app.Test(putReq2)

	tags, err := state.GetDB().ListDockerTags("docker-local", "multi-tag-app", "", 10)
	if err != nil || len(tags) != 2 {
		t.Fatalf("expected 2 tags in DB, got %d (err: %v)", len(tags), err)
	}

	delTagReq := httptest.NewRequest(http.MethodDelete, "/v2/docker-local/multi-tag-app/manifests/v1.0.0", nil)
	delTagReq.Header.Set("Authorization", "Bearer "+tok.Token)
	delResp, _ := app.Test(delTagReq)
	if delResp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted on tag delete, got %d", delResp.StatusCode)
	}

	getLatestReq := httptest.NewRequest(http.MethodGet, "/v2/docker-local/multi-tag-app/manifests/latest", nil)
	getLatestReq.Header.Set("Authorization", "Bearer "+tok.Token)
	getLatestResp, _ := app.Test(getLatestReq)
	if getLatestResp.StatusCode != http.StatusOK {
		t.Fatalf("expected latest tag to still be accessible, got %d", getLatestResp.StatusCode)
	}

	delManifestReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/v2/docker-local/multi-tag-app/manifests/%s", manifestDigest), nil)
	delManifestReq.Header.Set("Authorization", "Bearer "+tok.Token)
	delManifestResp, _ := app.Test(delManifestReq)
	if delManifestResp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted on manifest delete by digest, got %d", delManifestResp.StatusCode)
	}

	getGoneReq := httptest.NewRequest(http.MethodGet, "/v2/docker-local/multi-tag-app/manifests/latest", nil)
	getGoneReq.Header.Set("Authorization", "Bearer "+tok.Token)
	getGoneResp, _ := app.Test(getGoneReq)
	if getGoneResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found after manifest deletion, got %d", getGoneResp.StatusCode)
	}
}

func TestReadManifestRequestRejectsKnownAndUnknownOversizedBodies(t *testing.T) {
	small := []byte(`{"schemaVersion":2}`)
	body, err := readManifestBody(bytes.NewReader(small), int64(len(small)))
	if err != nil || !bytes.Equal(body, small) {
		t.Fatalf("small manifest body = %q, err = %v", body, err)
	}
	oversized := bytes.Repeat([]byte{'x'}, MaxManifestSize+1)
	if _, err := readManifestBody(bytes.NewReader([]byte("ignored")), int64(len(oversized))); !errors.Is(err, ErrManifestTooLarge) {
		t.Fatalf("known oversized manifest error = %v", err)
	}
	if _, err := readManifestBody(bytes.NewReader(oversized), -1); !errors.Is(err, ErrManifestTooLarge) {
		t.Fatalf("streamed oversized manifest error = %v", err)
	}
}
