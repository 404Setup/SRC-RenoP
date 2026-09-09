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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/index"
	"renop/internal/service/storage"
	"renop/internal/testutil"
)

func TestMavenLockedMetadataCacheAndLatestFileRespectViewer(t *testing.T) {
	state := core.NewAppState()
	initMavenReadTestDB(t, state)
	cfg := config.DefaultConfig()
	cfg.StoragePath = testutil.TempDir(t)
	cfg.Maven.Repositories = map[string]*config.Repository{"releases": {Name: "releases", Format: "maven", Visibility: "PUBLIC"}}
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()
	storage.InitS3(cfg)
	db, now := state.GetDB(), time.Now().UnixMilli()
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "owner"}))
	require.NoError(t, db.CreateMavenDomain(&core.MavenDomain{Domain: "com.example", VerificationType: "dns",
		VerificationHost: "example.com", VerificationCode: "proof", CreatedAt: now}, "owner"))
	require.NoError(t, db.MarkMavenDomainVerified("com.example", "proof", now, nil))
	root := filepath.Join(cfg.StoragePath, "releases/com/example/demo")
	for _, version := range []string{"1.0", "2.0"} {
		require.NoError(t, db.RecordMavenPublication(&core.MavenArtifact{Repository: "releases", Domain: "com.example",
			GroupID: "com.example", ArtifactID: "demo", CreatedAt: now}, &core.MavenVersion{Version: version, Size: 3, CreatedAt: now}))
		file := filepath.Join(root, version, "demo-"+version+".jar")
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
		require.NoError(t, os.WriteFile(file, []byte(version), 0600))
		state.Inner.FileIndex.InsertFile(file, index.FileInfo{Size: 3, ModTime: time.Now().UnixNano()})
	}
	metadataPath := filepath.Join(root, "maven-metadata.xml")
	require.NoError(t, os.WriteFile(metadataPath, []byte(`<metadata><versioning><latest>2.0</latest><versions><version>1.0</version><version>2.0</version></versions></versioning></metadata>`), 0600))
	admin := &config.User{Username: "admin", Roles: []string{"admin"}}
	guest := &config.User{Username: "guest"}
	cached, err := FindMetadata(state, admin, "releases", "com/example/demo")
	require.NoError(t, err)
	require.Len(t, cached.Versioning.Versions.Version, 2)
	lock := &core.ResourceLock{ResourceLockTarget: core.ResourceLockTarget{Format: "maven", Repository: "releases",
		Name: "com.example:demo", Version: "2.0"}, Source: core.ResourceLockSystem, Mode: core.ResourceLockRead, Reason: "trojan", LockedAt: now}
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	filtered, err := FindMetadata(state, guest, "releases", "com/example/demo")
	require.NoError(t, err)
	require.Equal(t, []string{"1.0"}, filtered.Versioning.Versions.Version)
	require.Equal(t, "1.0", *filtered.Versioning.Latest)
	require.Len(t, cached.Versioning.Versions.Version, 2)
	_, err = FindMetadata(state, guest, "releases", "com/example/demo/2.0")
	require.ErrorIs(t, err, fiber.ErrNotFound)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		user := guest
		if c.Get("X-Test-Admin") == "true" {
			user = admin
		}
		if c.Get("X-Test-Owner") == "true" {
			user = &config.User{Username: "owner"}
		}
		c.Locals("user", user)
		return c.Next()
	})
	app.Get("/latest/:repo_name/*", func(c fiber.Ctx) error { return LatestFile(c, state) })
	app.Post("/pom/:repo_name/*", func(c fiber.Ctx) error { return GeneratePom(c, state) })
	for _, staff := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodGet, "/latest/releases/com/example/demo", nil)
		if staff {
			req.Header.Set("X-Test-Admin", "true")
			req.Header.Set("Range", "bytes=0-1")
		}
		response, err := app.Test(req)
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		if staff {
			require.Equal(t, http.StatusNotFound, response.StatusCode)
		} else {
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.Equal(t, "1.0", string(body))
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/pom/releases/com/example/demo/2.0", strings.NewReader(`{"group_id":"com.example","artifact_id":"demo","version":"2.0"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Owner", "true")
	response, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusLocked, response.StatusCode)
	_, err = os.Stat(filepath.Join(root, "2.0/demo-2.0.pom"))
	require.ErrorIs(t, err, os.ErrNotExist)
	req = httptest.NewRequest(http.MethodPost, "/pom/releases/com/example/demo/3.0", strings.NewReader(`{"group_id":"com.example","artifact_id":"demo","version":"3.0"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Owner", "true")
	response, err = app.Test(req)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusLocked, response.StatusCode)
	require.False(t, state.Inner.FileIndex.HasDir(filepath.Join(root, "3.0")))
	_, err = os.Stat(filepath.Join(root, "3.0"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func initMavenReadTestDB(t *testing.T, state *core.AppState) {
	t.Helper()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "maven-read.db")})
	if err != nil {
		t.Fatal(err)
	}
	state.Inner.DB = db
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
}

func TestFindMetadataFetchesAndCachesMirrorMetadata(t *testing.T) {
	storagePath := t.TempDir()
	var hits atomic.Int64
	metadata := []byte(`<metadata><groupId>com.example</groupId><artifactId>demo</artifactId><versioning><versions><version>1.0.0</version></versions></versioning></metadata>`)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/com/example/demo/maven-metadata.xml" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(metadata)
	}))
	t.Cleanup(upstream.Close)

	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"mirror": {
			Name:       "mirror",
			Visibility: "PUBLIC",
			Mirrors:    []config.Mirror{{URL: upstream.URL, TimeoutSecs: 5}},
		},
	}
	storage.InitS3(cfg)
	state := core.NewAppState()
	initMavenReadTestDB(t, state)
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()

	got, err := FindMetadata(state, nil, "mirror", "com/example/demo")
	if err != nil {
		t.Fatalf("FindMetadata: %v", err)
	}
	if got.ArtifactID == nil || *got.ArtifactID != "demo" {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream requests = %d, want 1", hits.Load())
	}
	path := filepath.Join(storagePath, "mirror", "com", "example", "demo", "maven-metadata.xml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("metadata was not persisted: %v", err)
	}

	if _, err := FindMetadata(state, nil, "mirror", "com/example/demo"); err != nil {
		t.Fatalf("cached FindMetadata: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("cached lookup made another upstream request: %d", hits.Load())
	}
}

func TestFindMetadataFallsBackToMavenParentMetadata(t *testing.T) {
	storagePath := t.TempDir()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/com/example/demo/maven-metadata.xml" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<metadata><artifactId>demo</artifactId><versioning><versions><version>1.0.0-SNAPSHOT</version></versions></versioning></metadata>`))
	}))
	t.Cleanup(upstream.Close)

	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"mirror": {Name: "mirror", Visibility: "PUBLIC", Mirrors: []config.Mirror{{URL: upstream.URL, TimeoutSecs: 5}}},
	}
	storage.InitS3(cfg)
	state := core.NewAppState()
	initMavenReadTestDB(t, state)
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()

	metadata, err := FindMetadata(state, nil, "mirror", "com/example/demo/1.0.0-SNAPSHOT")
	if err != nil {
		t.Fatalf("FindMetadata version path: %v", err)
	}
	if metadata.ArtifactID == nil || *metadata.ArtifactID != "demo" {
		t.Fatalf("unexpected fallback metadata: %+v", metadata)
	}
}
