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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"google.golang.org/protobuf/proto"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/index"
	"renop/internal/service/javadocs"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
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
	app, appState := setupSettingsTestApp(t, cfg)

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

	updatedCfg := appState.Inner.Config.Load()
	if updatedCfg.Updater.Channel != "nightly" || updatedCfg.Updater.Mode != "auto_check" {
		t.Fatalf("expected updater config to be updated, got channel=%s mode=%s", updatedCfg.Updater.Channel, updatedCfg.Updater.Mode)
	}
}

func TestGPGDomainSettings(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	app, appState := setupSettingsTestApp(t, cfg)

	var got pb.ServerConfig
	respGet := protoGET(t, app, "/domain/server", &got)
	if respGet.StatusCode != http.StatusOK {
		t.Fatalf("expected GET 200, got %d", respGet.StatusCode)
	}
	if got.Gpg == nil || len(got.Gpg.KeyServers) != 3 {
		t.Fatalf("expected three default GPG key servers, got %v", got.Gpg)
	}

	keyServers := []string{"https://keys.example.test", "https://backup.example.test"}
	got.Gpg = &pb.GpgConfig{KeyServers: keyServers}
	respPut := protoPUT(t, app, "/domain/server", &got)
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}
	if actual := appState.Inner.Config.Load().Server.GPG.KeyServers; !slices.Equal(actual, keyServers) {
		t.Fatalf("expected key servers %v, got %v", keyServers, actual)
	}

	got.Gpg = &pb.GpgConfig{KeyServers: []string{"http://insecure.example.test"}}
	respInvalid := protoPUT(t, app, "/domain/server", &got)
	if respInvalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid key server PUT 400, got %d", respInvalid.StatusCode)
	}
	if actual := appState.Inner.Config.Load().Server.GPG.KeyServers; !slices.Equal(actual, keyServers) {
		t.Fatalf("invalid update changed key servers to %v", actual)
	}

	var removed pb.GpgConfig
	if resp := protoGET(t, app, "/domain/gpg", &removed); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected removed GPG domain GET 404, got %d", resp.StatusCode)
	}
}

func TestProxyDomainSettings(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	app, appState := setupSettingsTestApp(t, cfg)

	var initial pb.ProxyConfig
	respGet := protoGET(t, app, "/domain/proxy", &initial)
	if respGet.StatusCode != http.StatusOK {
		t.Fatalf("expected GET 200, got %d", respGet.StatusCode)
	}
	if initial.Selected != "" || len(initial.Proxies) != 0 {
		t.Fatalf("expected direct routing without configured proxies, got selected %q with %d proxies", initial.Selected, len(initial.Proxies))
	}

	update := &pb.ProxyConfig{
		Selected: " PRIMARY ",
		Proxies: []*pb.OutboundProxy{
			{Name: "Primary", Url: "HTTP://proxy.example:8080/", Username: "alice", Password: "secret"},
			{Name: "fallback", Url: "socks5://127.0.0.1:1080"},
		},
	}
	respPut := protoPUT(t, app, "/domain/proxy", update)
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}
	actual := appState.Inner.Config.Load().Proxy
	if actual.Selected != "Primary" || len(actual.Proxies) != 2 {
		t.Fatalf("unexpected normalized proxy config: %+v", actual)
	}
	if actual.Proxies[0].URL != "http://proxy.example:8080" || actual.Proxies[0].Password != "secret" {
		t.Fatalf("expected normalized proxy and preserved credentials, got %+v", actual.Proxies[0])
	}

	var roundTrip pb.ProxyConfig
	respRoundTrip := protoGET(t, app, "/domain/proxy", &roundTrip)
	if respRoundTrip.StatusCode != http.StatusOK {
		t.Fatalf("expected second GET 200, got %d", respRoundTrip.StatusCode)
	}
	if roundTrip.Selected != "Primary" || len(roundTrip.Proxies) != 2 || roundTrip.Proxies[0].Password != "secret" {
		t.Fatalf("unexpected proxy round trip: selected %q with %d proxies", roundTrip.Selected, len(roundTrip.Proxies))
	}

	invalidUpdates := []*pb.ProxyConfig{
		{
			Proxies: []*pb.OutboundProxy{
				{Name: "duplicate", Url: "http://proxy.example"},
				{Name: "DUPLICATE", Url: "http://backup.example"},
			},
		},
		{Proxies: []*pb.OutboundProxy{{Name: "bad", Url: "file:///tmp/socket"}}},
		{Selected: "missing", Proxies: []*pb.OutboundProxy{{Name: "primary", Url: "http://proxy.example"}}},
	}
	for _, invalid := range invalidUpdates {
		respInvalid := protoPUT(t, app, "/domain/proxy", invalid)
		if respInvalid.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected invalid proxy PUT 400, got %d", respInvalid.StatusCode)
		}
		unchanged := appState.Inner.Config.Load().Proxy
		if unchanged.Selected != "Primary" || len(unchanged.Proxies) != 2 {
			t.Fatalf("invalid update changed proxy settings: %+v", unchanged)
		}
	}
}

func TestDomainSettingsRejectsOversizedControlPlaneBody(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	app, _ := setupSettingsTestApp(t, cfg)

	resp := protoPUT(t, app, "/domain/frontend", &pb.FrontendConfig{
		Title: strings.Repeat("a", protohttp.MaxRequestBodySize),
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected PUT 413 for oversized body, got %d", resp.StatusCode)
	}
}

func TestFullDomainUpdateReplacesUpdater(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	cfg.Updater.Channel = "nightly"
	cfg.Updater.Mode = "manual"
	app, appState := setupSettingsTestApp(t, cfg)

	respPut := protoPUT(t, app, "/domain/updater", &pb.UpdaterConfig{
		Channel: "release",
		Mode:    "auto_install",
	})
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	updatedCfg := appState.Inner.Config.Load()
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
				{Name: "central", URL: "https://repo.maven.apache.org/maven2/"},
			},
		},
	}
	app, appState := setupSettingsTestApp(t, cfg)

	respPut := protoPUT(t, app, "/maven/repositories/releases", &pb.Repository{
		Name:                "releases",
		Visibility:          "PRIVATE",
		AllowRedeployment:   true,
		RequireGpgSignature: true,
		Mirrors: []*pb.Mirror{
			{Name: "central", Url: "https://repo.maven.apache.org/maven2/"},
		},
	})
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	updatedCfg := appState.Inner.Config.Load()
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
	if !repo.RequireGPGSignature {
		t.Fatal("expected RequireGPGSignature to be enabled")
	}
	if len(repo.Mirrors) != 1 || repo.Mirrors[0].Name != "central" {
		t.Fatalf("expected existing mirrors to be preserved")
	}
}

func TestRepositoryUpdateNormalizesS3KeyPrefix(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	app, appState := setupSettingsTestApp(t, cfg)

	resp := protoPUT(t, app, "/maven/repositories/releases", &pb.Repository{
		Name:       "releases",
		Visibility: "PUBLIC",
		S3: &pb.S3Config{
			KeyPrefix: " /renop/production/ ",
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", resp.StatusCode)
	}

	got := appState.Inner.Config.Load().Maven.Repositories["releases"].S3.KeyPrefix
	if got != "renop/production" {
		t.Fatalf("key prefix = %q, want %q", got, "renop/production")
	}

	var repositories pb.MavenRepositoriesResponse
	resp = protoGET(t, app, "/maven/repositories", &repositories)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected GET 200, got %d", resp.StatusCode)
	}
	returned := repositories.Repositories["releases"]
	if returned == nil || returned.S3 == nil || returned.S3.KeyPrefix != "renop/production" {
		t.Fatalf("key prefix was not returned through the API: %#v", returned)
	}
}

func TestRepositoryUpdateRejectsInvalidS3KeyPrefix(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	app, appState := setupSettingsTestApp(t, cfg)

	resp := protoPUT(t, app, "/maven/repositories/releases", &pb.Repository{
		Name:       "releases",
		Visibility: "PUBLIC",
		S3: &pb.S3Config{
			KeyPrefix: "renop/../production",
		},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected PUT 400, got %d", resp.StatusCode)
	}
	if got := appState.Inner.Config.Load().Maven.Repositories["releases"].S3; got != nil {
		t.Fatalf("invalid S3 config was persisted: %#v", got)
	}
}

func TestFullFrontendUpdate(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	cfg.Frontend.Title = "Custom Server Title"
	cfg.Frontend.OrganizationWebsite = "https://custom.org"
	cfg.Frontend.ID = "custom-id"
	app, appState := setupSettingsTestApp(t, cfg)

	respPut := protoPUT(t, app, "/domain/frontend", &pb.FrontendConfig{
		Id:                   "custom-id",
		Title:                "Custom Server Title",
		Description:          "New Description",
		OrganizationWebsite:  "https://custom.org",
		OrganizationLogo:     cfg.Frontend.OrganizationLogo,
		BackgroundUrl:        "",
		IcpLicense:           cfg.Frontend.IcpLicense,
		PublicSecurityFiling: "京公网安备11000000000001号",
		LegalNoticeUrl:       "https://custom.org/legal",
	})
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	updatedCfg := appState.Inner.Config.Load()
	if updatedCfg.Frontend.Title != "Custom Server Title" {
		t.Fatalf("expected Title to remain 'Custom Server Title', got %s", updatedCfg.Frontend.Title)
	}
	if updatedCfg.Frontend.OrganizationWebsite != "https://custom.org" {
		t.Fatalf("expected OrganizationWebsite to remain 'https://custom.org', got %s", updatedCfg.Frontend.OrganizationWebsite)
	}
	if updatedCfg.Frontend.Description != "New Description" {
		t.Fatalf("expected Description to be updated to 'New Description', got %s", updatedCfg.Frontend.Description)
	}
	if updatedCfg.Frontend.LegalNoticeURL != "https://custom.org/legal" {
		t.Fatalf("expected LegalNoticeUrl to be persisted, got %s", updatedCfg.Frontend.LegalNoticeURL)
	}
	if updatedCfg.Frontend.PublicSecurityFiling != "京公网安备11000000000001号" {
		t.Fatalf("expected PublicSecurityFiling to be persisted, got %s", updatedCfg.Frontend.PublicSecurityFiling)
	}
}

func TestFrontendUpdateRejectsUnsafeLegalNoticeURL(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	app, _ := setupSettingsTestApp(t, cfg)

	msg := pb.FromFrontendConfig(cfg.Frontend)
	msg.LegalNoticeUrl = "javascript:alert(1)"
	resp := protoPUT(t, app, "/domain/frontend", msg)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected PUT 400, got %d", resp.StatusCode)
	}
}

func TestFullServerUpdate(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	cfg.Server.Port = 8080
	cfg.Server.Domains = []string{"myrepo.custom.com"}
	cfg.Server.FileCacheSizeMb = 100
	app, appState := setupSettingsTestApp(t, cfg)

	respPut := protoPUT(t, app, "/domain/server", &pb.ServerConfig{
		Host:              cfg.Server.Host,
		Port:              8080,
		SslEnabled:        cfg.Server.SslEnabled,
		SslCertPath:       cfg.Server.SslCertPath,
		SslKeyPath:        cfg.Server.SslKeyPath,
		Domains:           []string{"myrepo.custom.com", "cdn.myrepo.custom.com"},
		EnableCompression: cfg.Server.EnableCompression,
		FileCacheSizeMb:   256,
		MaxActiveRequests: cfg.Server.MaxActiveRequests,
		TrustedProxies:    append([]string(nil), cfg.Server.TrustedProxies...),
		CdnIpHeader:       cfg.Server.CdnIPHeader,
		CorsOrigins:       []string{"*.myrepo.custom.com", "https://partner.example.com"},
	})
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	updatedCfg := appState.Inner.Config.Load()
	if updatedCfg.Server.Port != 8080 {
		t.Fatalf("expected Port to remain 8080, got %d", updatedCfg.Server.Port)
	}
	if len(updatedCfg.Server.Domains) != 2 || updatedCfg.Server.Domains[0] != "myrepo.custom.com" {
		t.Fatalf("expected Domains to be updated, got %v", updatedCfg.Server.Domains)
	}
	if len(updatedCfg.Server.CorsOrigins) != 2 {
		t.Fatalf("expected CorsOrigins to be updated, got %v", updatedCfg.Server.CorsOrigins)
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
					URL:          "https://repo.maven.apache.org/maven2/",
					CacheTTLSecs: 3600,
					TimeoutSecs:  15,
				},
			},
		},
	}
	app, appState := setupSettingsTestApp(t, cfg)

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

	updatedCfg := appState.Inner.Config.Load()
	repo := updatedCfg.Maven.Repositories["releases"]
	if repo == nil {
		t.Fatalf("expected repo 'releases' to exist")
	}
	if len(repo.Mirrors) != 1 {
		t.Fatalf("expected 1 mirror")
	}
	m := repo.Mirrors[0]
	if m.CacheTTLSecs != 3600 {
		t.Fatalf("expected CacheTtlSecs to remain 3600, got %d", m.CacheTTLSecs)
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
	app, appState := setupSettingsTestApp(t, cfg)

	msg := &pb.FrontendConfig{
		Id:                   cfg.Frontend.ID,
		Title:                "UniqueTitleForZeroCopyCheck",
		Description:          cfg.Frontend.Description,
		OrganizationWebsite:  cfg.Frontend.OrganizationWebsite,
		OrganizationLogo:     cfg.Frontend.OrganizationLogo,
		BackgroundUrl:        cfg.Frontend.BackgroundURL,
		IcpLicense:           cfg.Frontend.IcpLicense,
		PublicSecurityFiling: cfg.Frontend.PublicSecurityFiling,
		LegalNoticeUrl:       cfg.Frontend.LegalNoticeURL,
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

	updatedCfg := appState.Inner.Config.Load()
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

	updatedCfg2 := appState.Inner.Config.Load()
	r := updatedCfg2.Maven.Repositories["releases"]
	if r == nil {
		t.Fatalf("expected repo 'releases'")
	}
	if r.Mirrors[0].Name != "custom-mirror" {
		t.Fatalf("Zero-copy memory leak in repository mirror name! Got %q", r.Mirrors[0].Name)
	}
	if r.Mirrors[0].URL != "https://mirror.example.com/repo" {
		t.Fatalf("Zero-copy memory leak in repository mirror url! Got %q", r.Mirrors[0].URL)
	}
	if r.Mirrors[0].Authorization.Login != "secretuser" {
		t.Fatalf("Zero-copy memory leak in repository mirror auth login! Got %q", r.Mirrors[0].Authorization.Login)
	}
}

func TestStorageDomainUpdate(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = tempDir
	app, appState := setupSettingsTestApp(t, cfg)

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

	updatedCfg := appState.Inner.Config.Load()
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
	fullCfg := appState.Inner.Config.Load()
	if filepath.Clean(fullCfg.StoragePath) != filepath.Clean(filepath.Join(tempDir, "partial")) {
		t.Fatalf("expected StoragePath updated, got %s", fullCfg.StoragePath)
	}
	if fullCfg.MaxJavadocSizeMb != 100 {
		t.Fatalf("expected MaxJavadocSizeMb 100, got %d", fullCfg.MaxJavadocSizeMb)
	}
}

func TestGetDomainsProtobuf(t *testing.T) {
	cfg := config.DefaultConfig()
	app, _ := setupSettingsTestApp(t, cfg)

	var got pb.SettingsDomainsResponse
	resp := protoGET(t, app, "/domains", &got)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected GET 200, got %d", resp.StatusCode)
	}
	if len(got.Domains) != 6 || !slices.Contains(got.Domains, "proxy") || slices.Contains(got.Domains, "gpg") {
		t.Fatalf("expected 6 domains including proxy and excluding gpg, got %v", got.Domains)
	}
}

func TestGetAndUpdateDatabaseSettingsProtobuf(t *testing.T) {
	cfg := config.DefaultConfig()
	app, appState := setupSettingsTestApp(t, cfg)

	var got pb.ServerConfig
	respGet := protoGET(t, app, "/domain/server", &got)
	if respGet.StatusCode != http.StatusOK {
		t.Fatalf("expected GET 200, got %d", respGet.StatusCode)
	}
	if got.Database == nil || got.Database.Driver != "sqlite3" {
		t.Fatalf("expected embedded database driver sqlite3, got %v", got.Database)
	}

	updateServer := proto.Clone(&got).(*pb.ServerConfig)
	updateServer.Database = &pb.DatabaseConfig{
		Enabled:            true,
		Driver:             "sqlite3",
		Dsn:                "new_renop.db",
		MaxOpenConns:       50,
		MaxIdleConns:       10,
		ConnMaxLifetimeSec: 600,
	}
	respPut := protoPUT(t, app, "/domain/server", updateServer)
	if respPut.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200, got %d", respPut.StatusCode)
	}

	updatedCfg := appState.Inner.Config.Load()
	if updatedCfg.Database.Dsn != "new_renop.db" || updatedCfg.Database.MaxOpenConns != 50 {
		t.Fatalf("expected Dsn new_renop.db and MaxOpenConns 50, got Dsn=%s MaxOpenConns=%d", updatedCfg.Database.Dsn, updatedCfg.Database.MaxOpenConns)
	}

	updateServerPg := proto.Clone(&got).(*pb.ServerConfig)
	updateServerPg.Database = &pb.DatabaseConfig{
		Enabled:            true,
		Driver:             "postgres",
		Dsn:                "postgres://user:pass@localhost:5432/renop?sslmode=disable",
		MaxOpenConns:       20,
		MaxIdleConns:       5,
		ConnMaxLifetimeSec: 300,
	}
	respPutPg := protoPUT(t, app, "/domain/server", updateServerPg)
	if respPutPg.StatusCode != http.StatusOK {
		t.Fatalf("expected PUT 200 for postgres driver, got %d", respPutPg.StatusCode)
	}
	updatedCfgPg := appState.Inner.Config.Load()
	if updatedCfgPg.Database.Driver != "postgres" || updatedCfgPg.Database.Dsn != "postgres://user:pass@localhost:5432/renop?sslmode=disable" {
		t.Fatalf("expected postgres driver and DSN, got %v", updatedCfgPg.Database)
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
	app, appState := setupSettingsTestApp(t, cfg)

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

	updated := appState.Inner.Config.Load()
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

func TestStoragePathChangeRejectedWhileGPGPublicationPending(t *testing.T) {
	oldDir := t.TempDir()
	newDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = oldDir
	cfg.Database = config.DatabaseConfig{
		Driver:       "sqlite",
		Dsn:          filepath.Join(t.TempDir(), "settings-gpg.db"),
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	}
	db, err := database.InitDB(cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	app, state := setupSettingsTestApp(t, cfg)
	state.Inner.DB = db
	now := time.Now().UnixMilli()
	if err := db.SaveGPGRelease(&core.GPGRelease{
		ID:           "11111111-1111-4111-8111-111111111111",
		ActiveKey:    strings.Repeat("a", 64),
		Repository:   "releases",
		ArtifactPath: "org/example/demo/1.0/demo-1.0.jar",
		Uploader:     "alice",
		Status:       core.GPGReleaseQueued,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatal(err)
	}

	resp := protoPUT(t, app, "/domain/storage", &pb.StorageConfig{
		StoragePath:          filepath.ToSlash(newDir),
		EnableJavadocPreview: cfg.EnableJavadocPreview,
		JavadocExtractPath:   cfg.JavadocExtractPath,
		MaxJavadocSizeMb:     cfg.MaxJavadocSizeMb,
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected PUT 409, got %d", resp.StatusCode)
	}
	if filepath.Clean(state.Inner.Config.Load().StoragePath) != filepath.Clean(oldDir) {
		t.Fatal("rejected storage path update changed the active configuration")
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
	app, appState := setupSettingsTestApp(t, cfg)

	resp := protoPUT(t, app, "/domain/server", &pb.ServerConfig{
		Host:              cfg.Server.Host,
		Port:              0,
		SslEnabled:        cfg.Server.SslEnabled,
		EnableCompression: cfg.Server.EnableCompression,
		FileCacheSizeMb:   cfg.Server.FileCacheSizeMb,
		MaxActiveRequests: cfg.Server.MaxActiveRequests,
		TrustedProxies:    append([]string(nil), cfg.Server.TrustedProxies...),
		CdnIpHeader:       cfg.Server.CdnIPHeader,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected port 0 to be rejected with 400, got %d", resp.StatusCode)
	}

	live := appState.Inner.Config.Load()
	if live.Server.Port != cfg.Server.Port {
		t.Fatalf("expected port unchanged after failed PUT, got %d", live.Server.Port)
	}
}

func TestStorageDomainRejectsEmptyPath(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	app, _ := setupSettingsTestApp(t, cfg)

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
	app, appState := setupSettingsTestApp(t, cfg)

	repoName := "new-repo"
	repoDir := filepath.Join(tempDir, repoName)
	if _, err := os.Stat(repoDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected repo dir to not exist before create, err=%v", err)
	}

	respPut := protoPUT(t, app, "/maven/repositories/"+repoName, &pb.Repository{
		Name:       repoName,
		Format:     config.RepositoryFormatMaven,
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

func TestRepositoryCreationRequiresStableFormatAndSlug(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	cfg.Maven.Repositories = map[string]*config.Repository{
		"existing": {Name: "existing", Format: config.RepositoryFormatMaven, Visibility: "PUBLIC", Mirrors: []config.Mirror{}},
	}
	app, _ := setupSettingsTestApp(t, cfg)

	missingFormat := protoPUT(t, app, "/repositories/new-repo", &pb.Repository{
		Name: "new-repo", Visibility: "PUBLIC", Mirrors: []*pb.Mirror{},
	})
	if missingFormat.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected missing creation format rejected with 400, got %d", missingFormat.StatusCode)
	}

	invalidName := protoPUT(t, app, "/repositories/Bad_Name", &pb.Repository{
		Name: "Bad_Name", Format: config.RepositoryFormatMaven, Visibility: "PUBLIC", Mirrors: []*pb.Mirror{},
	})
	if invalidName.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid repository slug rejected with 400, got %d", invalidName.StatusCode)
	}

	digitName := protoPUT(t, app, "/repositories/cargo2", &pb.Repository{
		Name: "cargo2", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC", Mirrors: []*pb.Mirror{},
	})
	if digitName.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected numeric repository slug rejected with 400, got %d", digitName.StatusCode)
	}

	reservedName := protoPUT(t, app, "/repositories/javadoc", &pb.Repository{
		Name: "javadoc", Format: config.RepositoryFormatMaven, Visibility: "PUBLIC", Mirrors: []*pb.Mirror{},
	})
	if reservedName.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected reserved repository slug rejected with 400, got %d", reservedName.StatusCode)
	}

	formatChange := protoPUT(t, app, "/repositories/existing", &pb.Repository{
		Name: "existing", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC", Mirrors: []*pb.Mirror{},
	})
	if formatChange.StatusCode != http.StatusConflict {
		t.Fatalf("expected repository format change rejected with 409, got %d", formatChange.StatusCode)
	}

	caseCollision := protoPUT(t, app, "/repositories/EXISTING", &pb.Repository{
		Name: "EXISTING", Format: config.RepositoryFormatMaven, Visibility: "PUBLIC", Mirrors: []*pb.Mirror{},
	})
	if caseCollision.StatusCode != http.StatusConflict {
		t.Fatalf("expected case-insensitive repository collision rejected with 409, got %d", caseCollision.StatusCode)
	}
}

func TestMavenLayoutCanChangeAndFilePolicyIsForced(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	cfg.Maven.Repositories = map[string]*config.Repository{
		"existing": {
			Name: "existing", Format: config.RepositoryFormatMaven, Visibility: "PUBLIC", Mirrors: []config.Mirror{},
		},
	}
	app, state := setupSettingsTestApp(t, cfg)

	response := protoPUT(t, app, "/repositories/existing", &pb.Repository{
		Name: "existing", Format: config.RepositoryFormatMavenClassic, Visibility: "PUBLIC", Mirrors: []*pb.Mirror{},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected Maven layout change accepted, got %d", response.StatusCode)
	}
	if configured := state.Inner.Config.Load().Maven.Repositories["existing"].ConfiguredFormat(); configured != config.RepositoryFormatMavenClassic {
		t.Fatalf("Maven classic layout was not persisted: %q", configured)
	}
	response = protoPUT(t, app, "/repositories/existing", &pb.Repository{
		Name: "existing", Format: config.RepositoryFormatMaven, Visibility: "PUBLIC", Mirrors: []*pb.Mirror{},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected Maven modern layout restore accepted, got %d", response.StatusCode)
	}

	response = protoPUT(t, app, "/repositories/downloads", &pb.Repository{
		Name: "downloads", Format: config.RepositoryFormatFiles, Visibility: "PUBLIC", Mirrors: []*pb.Mirror{},
		AllowRedeployment: false, RequireGpgSignature: true,
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected file repository creation accepted, got %d", response.StatusCode)
	}
	files := state.Inner.Config.Load().Maven.Repositories["downloads"]
	if files == nil || !files.AllowRedeployment || files.RequireGPGSignature {
		t.Fatalf("file repository policy was not normalized: %#v", files)
	}
}

func TestCargoMirrorAllowsIndexURLWithoutArtifactTemplate(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.StoragePath = t.TempDir()
	cfg.Maven.Repositories = map[string]*config.Repository{}
	app, _ := setupSettingsTestApp(t, cfg)

	response := protoPUT(t, app, "/repositories/cargo", &pb.Repository{
		Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC",
		Mirrors: []*pb.Mirror{{Name: "crates-io", Url: "https://index.crates.io/"}},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected Cargo mirror without artifact template accepted, got %d", response.StatusCode)
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
	app, appState := setupSettingsTestApp(t, cfg)

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

	live := appState.Inner.Config.Load()
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
	app, appState := setupSettingsTestApp(t, cfg)

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
	live := appState.Inner.Config.Load()
	if live.Maven.Repositories["releases"].Visibility != "PRIVATE" {
		t.Fatalf("expected visibility normalized to PRIVATE, got %s", live.Maven.Repositories["releases"].Visibility)
	}
	m := live.Maven.Repositories["releases"].Mirrors[0]
	if m.CacheTTLSecs == 0 || m.TimeoutSecs == 0 {
		t.Fatalf("expected default ttl/timeout, got ttl=%d timeout=%d", m.CacheTTLSecs, m.TimeoutSecs)
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
	app, appState := setupSettingsTestApp(t, cfg)

	badDir := filepath.Join(t.TempDir(), "missing-parent", "nested")
	t.Setenv("RENOP_CONFIG", filepath.Join(badDir, "config.yaml"))

	before := appState.Inner.Config.Load().Updater.Channel

	resp := protoPUT(t, app, "/domain/updater", &pb.UpdaterConfig{
		Channel: "nightly",
		Mode:    "manual",
	})
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected write failure, got 200")
	}
	after := appState.Inner.Config.Load().Updater.Channel
	if after != before {
		t.Fatalf("in-memory config published despite disk failure: before=%s after=%s", before, after)
	}
}
