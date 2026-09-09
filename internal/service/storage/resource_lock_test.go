/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/index"
)

func TestCargoLocksPrecedeCacheConditionalsAndMirrorRefresh(t *testing.T) {
	directory := storageTestTempDir(t)
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(directory, "lock.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		_, _ = w.Write([]byte("replacement"))
	}))
	t.Cleanup(upstream.Close)
	repo := &config.Repository{Name: "cargo", Format: "cargo", Visibility: "PUBLIC",
		Mirrors: []config.Mirror{{URL: upstream.URL, CacheTTLSecs: 1}}}
	state := core.NewAppState()
	state.Inner.DB = db
	state.Inner.FileIndex = index.NewFileIndex()
	cfg := &config.Config{StoragePath: directory}
	cfg.Maven.Repositories = map[string]*config.Repository{"cargo": repo}
	state.Inner.Config.Store(cfg)
	path := filepath.Join(directory, "cargo", "api", "v1", "crates", "demo", "2.0.0-RC", "download")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("trusted"), 0600))
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))
	state.Inner.FileIndex.InsertFile(filepath.ToSlash(path), index.FileInfo{Size: 7, ModTime: old.UnixNano()})
	lock := &core.ResourceLock{ResourceLockTarget: core.ResourceLockTarget{Format: "cargo", Repository: "cargo",
		Name: "demo", Version: "2.0.0-RC"}, Source: core.ResourceLockSystem, Mode: core.ResourceLockWrite,
		Reason: "hold", LockedAt: time.Now().UnixMilli()}
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	app := fiber.New()
	app.All("/:repo_name/*", func(c fiber.Ctx) error {
		c.Locals("user", &config.User{Username: "admin", Roles: []string{"admin"}})
		if c.Method() == "HEAD" {
			return HandleHead(c, state, repo, directory)
		}
		return HandleGet(c, state, repo, directory)
	})
	get := func(method, path, etag string) (int, []byte, string) {
		t.Helper()
		req := httptest.NewRequest(method, "/cargo/"+path, nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
			req.Header.Set("Range", "bytes=0-2")
		}
		response, err := app.Test(req)
		require.NoError(t, err)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return response.StatusCode, body, response.Header.Get("ETag")
	}
	status, body, etag := get("GET", "api/v1/crates/demo/2.0.0-RC/download", "")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "trusted", string(body))
	require.Equal(t, int64(0), upstreamCalls.Load())
	lock.Mode = core.ResourceLockRead
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	for _, method := range []string{"GET", "HEAD"} {
		status, body, _ = get(method, "api/v1/crates/demo/2.0.0-RC/download", etag)
		require.Equal(t, http.StatusNotFound, status)
		require.NotContains(t, string(body), "trusted")
		status, _, _ = get(method, "API/V1/CRATES/DEMO/2.0.0-RC/DOWNLOAD", etag)
		require.Equal(t, http.StatusNotFound, status)
	}
	if runtime.GOOS == "windows" {
		status, _, _ = get("GET", "api/v1/crates/demo/2.0.0-rc/download", etag)
		require.Equal(t, http.StatusNotFound, status)
		visible, err := db.CargoMetadataVisibility("cargo", "", false, []core.ResourceLockTarget{{Name: "demo", Version: "2.0.0-rc"}})
		require.NoError(t, err)
		require.Equal(t, []bool{false}, visible)
	}
	lock.Mode = core.ResourceLockWrite
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	require.NoError(t, os.Remove(path))
	state.Inner.FileIndex.RemoveFile(filepath.ToSlash(path))
	status, _, _ = get("GET", "api/v1/crates/demo/2.0.0-RC/download", "")
	require.Equal(t, http.StatusNotFound, status)
	require.Equal(t, int64(0), upstreamCalls.Load())
}
