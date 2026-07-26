/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"google.golang.org/protobuf/proto"

	"renop/config"
	"renop/core"
	"renop/index"
	"renop/javadocs"
	"renop/pb"
	"renop/utils/protohttp"
)

func setupSettingsTestApp(t *testing.T, cfg *config.Config) (*fiber.App, *core.AppState) {
	t.Helper()
	t.Setenv("RENOP_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	t.Setenv("RENOP_REPOSITORIES", filepath.Join(t.TempDir(), "repositories.yaml"))

	appState := core.NewAppState()
	appState.Inner.FileIndex = index.NewFileIndex()
	appState.Inner.Config.Store(cfg)

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", &config.User{
			Username: "admin",
			Roles:    []string{"admin"},
		})
		return c.Next()
	})
	SetupSettingsRoutes(app, appState)
	return app, appState
}

func protoBody(t *testing.T, m proto.Message) *bytes.Buffer {
	t.Helper()
	data, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewBuffer(data)
}

func protoPUT(t *testing.T, app *fiber.App, path string, m proto.Message) *http.Response {
	t.Helper()
	req := httptest.NewRequest("PUT", path, protoBody(t, m))
	req.Header.Set("Content-Type", protohttp.ContentType)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func protoPOST(t *testing.T, app *fiber.App, path string, m proto.Message) *http.Response {
	t.Helper()
	req := httptest.NewRequest("POST", path, protoBody(t, m))
	req.Header.Set("Content-Type", protohttp.ContentType)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func protoGET(t *testing.T, app *fiber.App, path string, into proto.Message) *http.Response {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Accept", protohttp.ContentType)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusOK && into != nil {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := proto.Unmarshal(body, into); err != nil {
			t.Fatalf("decode protobuf response: %v", err)
		}
	}
	return resp
}

func TestRebuildIndexFullClearsJavadocCache(t *testing.T) {
	tempDir := t.TempDir()
	extractPath := t.TempDir()
	testCacheDir := filepath.Join(extractPath, "renop-javadoc-test-rebuild")
	if err := os.MkdirAll(testCacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		StoragePath:          tempDir,
		EnableJavadocPreview: true,
		JavadocExtractPath:   extractPath,
		MaxJavadocSizeMb:     48,
	}
	javadocs.InitJavadocs(cfg)
	app, _ := setupSettingsTestApp(t, cfg)

	resp := protoPOST(t, app, "/index/rebuild", &pb.RebuildIndexRequest{Mode: "full"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if _, err := os.Stat(testCacheDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected javadoc cache dir to be cleared after full rebuild")
	}
}

func TestUpdaterDomainSettings(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	app, appState := setupSettingsTestApp(t, &cfg)

	var got pb.UpdaterConfig
	respGet := protoGET(t, app, "/domain/updater", &got)
	if respGet.StatusCode != http.StatusOK {
		t.Fatalf("expected GET 200, got %d", respGet.StatusCode)
	}

	respPut := protoPUT(t, app, "/domain/updater", &pb.UpdaterConfig{
		Channel: "nightly",
		Mode:    "auto_check",
	})
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	updatedCfg := appState.Inner.Config.Load().(*config.Config)
	if updatedCfg.Updater.Channel != "nightly" || updatedCfg.Updater.Mode != "auto_check" {
		t.Fatalf("expected updater config to be updated, got channel=%s mode=%s", updatedCfg.Updater.Channel, updatedCfg.Updater.Mode)
	}
}

func TestFullDomainUpdateReplacesUpdater(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	cfg.Updater.Channel = "nightly"
	cfg.Updater.Mode = "manual"
	app, appState := setupSettingsTestApp(t, &cfg)

	respPut := protoPUT(t, app, "/domain/updater", &pb.UpdaterConfig{
		Channel: "release",
		Mode:    "auto_install",
	})
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	updatedCfg := appState.Inner.Config.Load().(*config.Config)
	if updatedCfg.Updater.Channel != "release" {
		t.Fatalf("expected channel 'release', got %s", updatedCfg.Updater.Channel)
	}
	if updatedCfg.Updater.Mode != "auto_install" {
		t.Fatalf("expected mode 'auto_install', got %s", updatedCfg.Updater.Mode)
	}
}

func TestFullRepoUpdate(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	cfg.Maven.Repositories = map[string]*config.Repository{
		"releases": {
			Name:              "releases",
			Visibility:        "PUBLIC",
			AllowRedeployment: true,
			Mirrors: []config.Mirror{
				{Name: "central", Url: "https://repo.maven.apache.org/maven2/"},
			},
		},
	}
	app, appState := setupSettingsTestApp(t, &cfg)

	respPut := protoPUT(t, app, "/maven/repositories/releases", &pb.Repository{
		Name:              "releases",
		Visibility:        "PRIVATE",
		AllowRedeployment: true,
		Mirrors: []*pb.Mirror{
			{Name: "central", Url: "https://repo.maven.apache.org/maven2/"},
		},
	})
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	updatedCfg := appState.Inner.Config.Load().(*config.Config)
	repo := updatedCfg.Maven.Repositories["releases"]
	if repo == nil {
		t.Fatalf("expected repo 'releases' to exist")
	}
	if repo.Visibility != "PRIVATE" {
		t.Fatalf("expected visibility 'PRIVATE', got %s", repo.Visibility)
	}
	if !repo.AllowRedeployment {
		t.Fatalf("expected AllowRedeployment to remain true")
	}
	if len(repo.Mirrors) != 1 || repo.Mirrors[0].Name != "central" {
		t.Fatalf("expected existing mirrors to be preserved")
	}
}

func TestFullFrontendUpdate(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	cfg.Frontend.Title = "Custom Server Title"
	cfg.Frontend.OrganizationWebsite = "https://custom.org"
	cfg.Frontend.Id = "custom-id"
	app, appState := setupSettingsTestApp(t, &cfg)

	respPut := protoPUT(t, app, "/domain/frontend", &pb.FrontendConfig{
		Id:                  "custom-id",
		Title:               "Custom Server Title",
		Description:         "New Description",
		OrganizationWebsite: "https://custom.org",
		OrganizationLogo:    cfg.Frontend.OrganizationLogo,
		BackgroundUrl:       "",
		IcpLicense:          cfg.Frontend.IcpLicense,
	})
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	updatedCfg := appState.Inner.Config.Load().(*config.Config)
	if updatedCfg.Frontend.Title != "Custom Server Title" {
		t.Fatalf("expected Title to remain 'Custom Server Title', got %s", updatedCfg.Frontend.Title)
	}
	if updatedCfg.Frontend.OrganizationWebsite != "https://custom.org" {
		t.Fatalf("expected OrganizationWebsite to remain 'https://custom.org', got %s", updatedCfg.Frontend.OrganizationWebsite)
	}
	if updatedCfg.Frontend.Description != "New Description" {
		t.Fatalf("expected Description to be updated to 'New Description', got %s", updatedCfg.Frontend.Description)
	}
}

func TestFullServerUpdate(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	cfg.Server.Port = 8080
	cfg.Server.Domain = "myrepo.custom.com"
	cfg.Server.FileCacheSizeMb = 100
	app, appState := setupSettingsTestApp(t, &cfg)

	respPut := protoPUT(t, app, "/domain/server", &pb.ServerConfig{
		Host:              cfg.Server.Host,
		Port:              8080,
		SslEnabled:        cfg.Server.SslEnabled,
		SslCertPath:       cfg.Server.SslCertPath,
		SslKeyPath:        cfg.Server.SslKeyPath,
		Domain:            "myrepo.custom.com",
		EnableCompression: cfg.Server.EnableCompression,
		FileCacheSizeMb:   256,
		MaxActiveRequests: cfg.Server.MaxActiveRequests,
		TrustedProxies:    append([]string(nil), cfg.Server.TrustedProxies...),
		CdnIpHeader:       cfg.Server.CdnIpHeader,
	})
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	updatedCfg := appState.Inner.Config.Load().(*config.Config)
	if updatedCfg.Server.Port != 8080 {
		t.Fatalf("expected Port to remain 8080, got %d", updatedCfg.Server.Port)
	}
	if updatedCfg.Server.Domain != "myrepo.custom.com" {
		t.Fatalf("expected Domain to remain 'myrepo.custom.com', got %s", updatedCfg.Server.Domain)
	}
	if updatedCfg.Server.FileCacheSizeMb != 256 {
		t.Fatalf("expected FileCacheSizeMb to be updated to 256, got %d", updatedCfg.Server.FileCacheSizeMb)
	}
}

func TestFullRepoMirrorUpdate(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	cfg.Maven.Repositories = map[string]*config.Repository{
		"releases": {
			Name:              "releases",
			Visibility:        "PUBLIC",
			AllowRedeployment: true,
			Mirrors: []config.Mirror{
				{
					Name:         "central",
					Url:          "https://repo.maven.apache.org/maven2/",
					CacheTtlSecs: 3600,
					TimeoutSecs:  15,
				},
			},
		},
	}
	app, appState := setupSettingsTestApp(t, &cfg)

	respPut := protoPUT(t, app, "/maven/repositories/releases", &pb.Repository{
		Name:              "releases",
		Visibility:        "PUBLIC",
		AllowRedeployment: true,
		Mirrors: []*pb.Mirror{
			{
				Name:           "central",
				Url:            "https://repo.maven.apache.org/maven2/",
				AllowArtifacts: []string{"org.apache"},
				CacheTtlSecs:   3600,
				TimeoutSecs:    15,
			},
		},
	})
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	updatedCfg := appState.Inner.Config.Load().(*config.Config)
	repo := updatedCfg.Maven.Repositories["releases"]
	if repo == nil {
		t.Fatalf("expected repo 'releases' to exist")
	}
	if len(repo.Mirrors) != 1 {
		t.Fatalf("expected 1 mirror")
	}
	m := repo.Mirrors[0]
	if m.CacheTtlSecs != 3600 {
		t.Fatalf("expected CacheTtlSecs to remain 3600, got %d", m.CacheTtlSecs)
	}
	if m.TimeoutSecs != 15 {
		t.Fatalf("expected TimeoutSecs to remain 15, got %d", m.TimeoutSecs)
	}
	if len(m.AllowArtifacts) != 1 || m.AllowArtifacts[0] != "org.apache" {
		t.Fatalf("expected AllowArtifacts to contain 'org.apache'")
	}
}

func TestZeroCopyMemorySafetyOnUpdate(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	app, appState := setupSettingsTestApp(t, &cfg)

	msg := &pb.FrontendConfig{
		Id:                  cfg.Frontend.Id,
		Title:               "UniqueTitleForZeroCopyCheck",
		Description:         cfg.Frontend.Description,
		OrganizationWebsite: cfg.Frontend.OrganizationWebsite,
		OrganizationLogo:    cfg.Frontend.OrganizationLogo,
		BackgroundUrl:       cfg.Frontend.BackgroundUrl,
		IcpLicense:          cfg.Frontend.IcpLicense,
	}
	bodyBytes, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	reqPut := httptest.NewRequest("PUT", "/domain/frontend", bytes.NewBuffer(bodyBytes))
	reqPut.Header.Set("Content-Type", protohttp.ContentType)
	respPut, err := app.Test(reqPut)
	if err != nil {
		t.Fatal(err)
	}
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	for i := range bodyBytes {
		bodyBytes[i] = 'X'
	}

	updatedCfg := appState.Inner.Config.Load().(*config.Config)
	if updatedCfg.Frontend.Title != "UniqueTitleForZeroCopyCheck" {
		t.Fatalf("Zero-copy memory leak detected! Title got corrupted: %q", updatedCfg.Frontend.Title)
	}

	repoMsg := &pb.Repository{
		Visibility: "PUBLIC",
		Mirrors: []*pb.Mirror{
			{
				Name: "custom-mirror",
				Url:  "https://mirror.example.com/repo",
				Authorization: &pb.MirrorCredentials{
					Login:    "secretuser",
					Password: "secretpass",
				},
			},
		},
	}
	repoPayload, err := proto.Marshal(repoMsg)
	if err != nil {
		t.Fatal(err)
	}
	reqRepo := httptest.NewRequest("PUT", "/maven/repositories/releases", bytes.NewBuffer(repoPayload))
	reqRepo.Header.Set("Content-Type", protohttp.ContentType)
	respRepo, err := app.Test(reqRepo)
	if err != nil {
		t.Fatal(err)
	}
	if respRepo.StatusCode != http.StatusOK {
		t.Fatalf("expected repo PUT 200, got %d", respRepo.StatusCode)
	}

	for i := range repoPayload {
		repoPayload[i] = 'Z'
	}

	updatedCfg2 := appState.Inner.Config.Load().(*config.Config)
	r := updatedCfg2.Maven.Repositories["releases"]
	if r == nil {
		t.Fatalf("expected repo 'releases'")
	}
	if r.Mirrors[0].Name != "custom-mirror" {
		t.Fatalf("Zero-copy memory leak in repository mirror name! Got %q", r.Mirrors[0].Name)
	}
	if r.Mirrors[0].Url != "https://mirror.example.com/repo" {
		t.Fatalf("Zero-copy memory leak in repository mirror url! Got %q", r.Mirrors[0].Url)
	}
	if r.Mirrors[0].Authorization.Login != "secretuser" {
		t.Fatalf("Zero-copy memory leak in repository mirror auth login! Got %q", r.Mirrors[0].Authorization.Login)
	}
}

func TestStorageDomainUpdate(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	app, appState := setupSettingsTestApp(t, &cfg)

	var got pb.StorageConfig
	respGet := protoGET(t, app, "/domain/storage", &got)
	if respGet.StatusCode != http.StatusOK {
		t.Fatalf("expected GET 200, got %d", respGet.StatusCode)
	}

	respPut := protoPUT(t, app, "/domain/storage", &pb.StorageConfig{
		StoragePath:          filepath.ToSlash(tempDir),
		EnableJavadocPreview: false,
		JavadocExtractPath:   "/tmp/extracted-javadocs",
		MaxJavadocSizeMb:     100,
	})
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	updatedCfg := appState.Inner.Config.Load().(*config.Config)
	if filepath.Clean(updatedCfg.StoragePath) != filepath.Clean(tempDir) {
		t.Fatalf("expected StoragePath to remain %s, got %s", tempDir, updatedCfg.StoragePath)
	}
	if updatedCfg.EnableJavadocPreview {
		t.Fatalf("expected EnableJavadocPreview to be updated to false, got %t", updatedCfg.EnableJavadocPreview)
	}
	if updatedCfg.JavadocExtractPath != "/tmp/extracted-javadocs" {
		t.Fatalf("expected JavadocExtractPath to be updated to '/tmp/extracted-javadocs', got %s", updatedCfg.JavadocExtractPath)
	}
	if updatedCfg.MaxJavadocSizeMb != 100 {
		t.Fatalf("expected MaxJavadocSizeMb to be updated to 100, got %d", updatedCfg.MaxJavadocSizeMb)
	}

	respPutInvalid := protoPUT(t, app, "/domain/storage", &pb.StorageConfig{
		StoragePath:          filepath.ToSlash(tempDir),
		EnableJavadocPreview: true,
		JavadocExtractPath:   "/tmp/extracted-javadocs",
		MaxJavadocSizeMb:     -5,
	})
	if respPutInvalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected PUT 400, got %d", respPutInvalid.StatusCode)
	}
	respPutFull := protoPUT(t, app, "/domain/storage", &pb.StorageConfig{
		StoragePath:          filepath.ToSlash(filepath.Join(tempDir, "partial")),
		EnableJavadocPreview: false,
		JavadocExtractPath:   "/tmp/partial-javadocs",
		MaxJavadocSizeMb:     100,
	})
	if respPutFull.StatusCode != http.StatusOK {
		t.Fatalf("expected full PUT 200, got %d", respPutFull.StatusCode)
	}
	fullCfg := appState.Inner.Config.Load().(*config.Config)
	if filepath.Clean(fullCfg.StoragePath) != filepath.Clean(filepath.Join(tempDir, "partial")) {
		t.Fatalf("expected StoragePath updated, got %s", fullCfg.StoragePath)
	}
	if fullCfg.MaxJavadocSizeMb != 100 {
		t.Fatalf("expected MaxJavadocSizeMb 100, got %d", fullCfg.MaxJavadocSizeMb)
	}
}

func TestGetDomainsProtobuf(t *testing.T) {
	cfg := config.DefaultConfig()
	app, _ := setupSettingsTestApp(t, &cfg)

	var got pb.SettingsDomainsResponse
	resp := protoGET(t, app, "/domains", &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected GET 200, got %d", resp.StatusCode)
	}
	if len(got.Domains) != 5 {
		t.Fatalf("expected 5 domains, got %v", got.Domains)
	}
}

func TestStoragePathChangeRebuildsIndex(t *testing.T) {
	oldDir := t.TempDir()
	newDir := t.TempDir()

	oldFile := filepath.Join(oldDir, "releases", "old-artifact.txt")
	if err := os.MkdirAll(filepath.Dir(oldFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldFile, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	newFile := filepath.Join(newDir, "releases", "new-artifact.txt")
	if err := os.MkdirAll(filepath.Dir(newFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newFile, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.StoragePath = oldDir
	cfg.Maven.Repositories = map[string]*config.Repository{
		"releases": {Name: "releases", Visibility: "PUBLIC", Mirrors: []config.Mirror{}},
	}
	app, appState := setupSettingsTestApp(t, &cfg)

	oldFileSlash := filepath.ToSlash(filepath.Clean(oldFile))
	oldDirSlash := filepath.ToSlash(filepath.Clean(oldDir))
	appState.Inner.FileIndex.InsertDir(oldDirSlash)
	appState.Inner.FileIndex.InsertDir(filepath.ToSlash(filepath.Join(oldDir, "releases")))
	appState.Inner.FileIndex.InsertFile(oldFileSlash, index.FileInfo{Size: 3, ModTime: 1})

	if !appState.Inner.FileIndex.HasFile(oldFileSlash) {
		t.Fatal("precondition: old file should be indexed")
	}

	resp := protoPUT(t, app, "/domain/storage", &pb.StorageConfig{
		StoragePath:          filepath.ToSlash(newDir),
		EnableJavadocPreview: cfg.EnableJavadocPreview,
		JavadocExtractPath:   cfg.JavadocExtractPath,
		MaxJavadocSizeMb:     cfg.MaxJavadocSizeMb,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", resp.StatusCode)
	}

	updated := appState.Inner.Config.Load().(*config.Config)
	if filepath.Clean(updated.StoragePath) != filepath.Clean(newDir) {
		t.Fatalf("expected storage path %s, got %s", newDir, updated.StoragePath)
	}

	newFileSlash := filepath.ToSlash(filepath.Clean(newFile))
	deadline := time.Now().Add(3 * time.Second)
	for {
		hasNew := appState.Inner.FileIndex.HasFile(newFileSlash)
		hasOld := appState.Inner.FileIndex.HasFile(oldFileSlash)
		if hasNew && !hasOld {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("index not rebuilt after storage path change: hasNew=%v hasOld=%v", hasNew, hasOld)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSameStoragePathNormalization(t *testing.T) {
	dir := t.TempDir()
	if !sameStoragePath(dir, dir) {
		t.Fatal("expected path equal to itself")
	}
	if !sameStoragePath(dir, filepath.Clean(dir+string(filepath.Separator))) {
		t.Fatal("expected trailing-separator path to match")
	}
	other := t.TempDir()
	if sameStoragePath(dir, other) {
		t.Fatal("expected different temp dirs to not match")
	}
}

func TestServerDomainRejectsInvalidPort(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	app, appState := setupSettingsTestApp(t, &cfg)

	resp := protoPUT(t, app, "/domain/server", &pb.ServerConfig{
		Host:              cfg.Server.Host,
		Port:              0,
		SslEnabled:        cfg.Server.SslEnabled,
		EnableCompression: cfg.Server.EnableCompression,
		FileCacheSizeMb:   cfg.Server.FileCacheSizeMb,
		MaxActiveRequests: cfg.Server.MaxActiveRequests,
		TrustedProxies:    append([]string(nil), cfg.Server.TrustedProxies...),
		CdnIpHeader:       cfg.Server.CdnIpHeader,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected port 0 to be rejected with 400, got %d", resp.StatusCode)
	}

	live := appState.Inner.Config.Load().(*config.Config)
	if live.Server.Port != cfg.Server.Port {
		t.Fatalf("expected port unchanged after failed PUT, got %d", live.Server.Port)
	}
}

func TestStorageDomainRejectsEmptyPath(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	app, _ := setupSettingsTestApp(t, &cfg)

	resp := protoPUT(t, app, "/domain/storage", &pb.StorageConfig{
		StoragePath:          "   ",
		EnableJavadocPreview: true,
		JavadocExtractPath:   "/tmp/j",
		MaxJavadocSizeMb:     10,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected empty storage path rejected with 400, got %d", resp.StatusCode)
	}
}

func TestPutMavenRepositoryCreatesStorageDir(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	cfg.Maven.Repositories = map[string]*config.Repository{}
	app, appState := setupSettingsTestApp(t, &cfg)

	repoName := "new-repo"
	repoDir := filepath.Join(tempDir, repoName)
	if _, err := os.Stat(repoDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected repo dir to not exist before create, err=%v", err)
	}

	respPut := protoPUT(t, app, "/maven/repositories/"+repoName, &pb.Repository{
		Name:       repoName,
		Visibility: "PUBLIC",
		Mirrors:    []*pb.Mirror{},
	})
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	if fi, err := os.Stat(repoDir); err != nil || !fi.IsDir() {
		t.Fatalf("expected repo storage dir to be created, err=%v", err)
	}

	pathNorm := filepath.ToSlash(filepath.Clean(repoDir))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if appState.Inner.FileIndex.HasDir(pathNorm) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !appState.Inner.FileIndex.HasDir(pathNorm) {
		t.Fatalf("expected repo dir to be registered in file index: %s", pathNorm)
	}
}

func TestDeleteMavenRepositoryRemovesStorageAndIndex(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	repoName := "to-delete"
	cfg.Maven.Repositories = map[string]*config.Repository{
		repoName: {Name: repoName, Visibility: "PUBLIC", Mirrors: []config.Mirror{}},
	}
	app, appState := setupSettingsTestApp(t, &cfg)

	repoDir := filepath.Join(tempDir, repoName)
	nested := filepath.Join(repoDir, "com", "example", "artifact")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(nested, "file.txt")
	if err := os.WriteFile(artifact, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	repoDirSlash := filepath.ToSlash(filepath.Clean(repoDir))
	nestedSlash := filepath.ToSlash(filepath.Clean(nested))
	artifactSlash := filepath.ToSlash(filepath.Clean(artifact))
	appState.Inner.FileIndex.InsertDir(repoDirSlash)
	appState.Inner.FileIndex.EnsureParentDirs(artifactSlash)
	appState.Inner.FileIndex.InsertFile(artifactSlash, index.FileInfo{Size: 5, ModTime: 1})
	appState.Inner.FileIndex.InsertNotFound(filepath.ToSlash(filepath.Join(nested, "missing.jar")), time.Now().Unix()+3600)

	reqDel := httptest.NewRequest("DELETE", "/maven/repositories/"+repoName, nil)
	respDel, err := app.Test(reqDel)
	if err != nil {
		t.Fatal(err)
	}
	if respDel.StatusCode != http.StatusOK {
		t.Fatalf("expected DELETE 200, got %d", respDel.StatusCode)
	}

	if _, err := os.Stat(repoDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected repo storage dir to be deleted, err=%v", err)
	}

	live := appState.Inner.Config.Load().(*config.Config)
	if _, ok := live.Maven.Repositories[repoName]; ok {
		t.Fatal("expected repository removed from config")
	}

	if appState.Inner.FileIndex.HasDir(repoDirSlash) {
		t.Fatal("expected repo dir removed from index")
	}
	if appState.Inner.FileIndex.HasDir(nestedSlash) {
		t.Fatal("expected nested dir removed from index")
	}
	if appState.Inner.FileIndex.HasFile(artifactSlash) {
		t.Fatal("expected artifact removed from index")
	}
	if appState.Inner.FileIndex.IsNotFound(filepath.ToSlash(filepath.Join(nested, "missing.jar"))) {
		t.Fatal("expected not-found entry under repo to be purged")
	}
}

func TestRepoVisibilityValidationAndDeleteNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	cfg.Maven.Repositories = map[string]*config.Repository{
		"releases": {Name: "releases", Visibility: "PUBLIC", Mirrors: []config.Mirror{}},
	}
	app, appState := setupSettingsTestApp(t, &cfg)

	respBad := protoPUT(t, app, "/maven/repositories/releases", &pb.Repository{
		Visibility: "OPEN",
		Mirrors:    []*pb.Mirror{},
	})
	if respBad.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid visibility 400, got %d", respBad.StatusCode)
	}

	respPut := protoPUT(t, app, "/maven/repositories/releases", &pb.Repository{
		Visibility: "private",
		Mirrors: []*pb.Mirror{
			{Name: "central", Url: "https://repo.maven.apache.org/maven2/"},
		},
	})
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}
	live := appState.Inner.Config.Load().(*config.Config)
	if live.Maven.Repositories["releases"].Visibility != "PRIVATE" {
		t.Fatalf("expected visibility normalized to PRIVATE, got %s", live.Maven.Repositories["releases"].Visibility)
	}
	m := live.Maven.Repositories["releases"].Mirrors[0]
	if m.CacheTtlSecs == 0 || m.TimeoutSecs == 0 {
		t.Fatalf("expected default ttl/timeout, got ttl=%d timeout=%d", m.CacheTtlSecs, m.TimeoutSecs)
	}

	reqDel := httptest.NewRequest("DELETE", "/maven/repositories/does-not-exist", nil)
	respDel, err := app.Test(reqDel)
	if err != nil {
		t.Fatal(err)
	}
	if respDel.StatusCode != http.StatusNotFound {
		t.Fatalf("expected DELETE missing repo 404, got %d", respDel.StatusCode)
	}
}

func TestDomainUpdateDoesNotPublishOnWriteFailure(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	app, appState := setupSettingsTestApp(t, &cfg)

	badDir := filepath.Join(t.TempDir(), "missing-parent", "nested")
	t.Setenv("RENOP_CONFIG", filepath.Join(badDir, "config.yaml"))

	before := appState.Inner.Config.Load().(*config.Config).Updater.Channel

	resp := protoPUT(t, app, "/domain/updater", &pb.UpdaterConfig{
		Channel: "nightly",
		Mode:    "manual",
	})
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected write failure, got 200")
	}
	after := appState.Inner.Config.Load().(*config.Config).Updater.Channel
	if after != before {
		t.Fatalf("in-memory config published despite disk failure: before=%s after=%s", before, after)
	}
}
