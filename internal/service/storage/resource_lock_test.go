/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/index"
	"renop/internal/service/proxy"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

func TestPackageLocksPrecedeCacheConditionalsAndMirrorRefresh(t *testing.T) {
	for _, tc := range []struct{ format, path string }{
		{"cargo", "api/v1/crates/demo/2.0.0-RC/download"},
		{"npm", "demo/-/demo-2.0.0-RC.tgz"},
	} {
		t.Run(tc.format, func(t *testing.T) {
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
			repo := &config.Repository{Name: tc.format, Format: tc.format, Visibility: "PUBLIC",
				Mirrors: []config.Mirror{{URL: upstream.URL, CacheTTLSecs: 1}}}
			state := core.NewAppState()
			state.Inner.DB = db
			state.Inner.FileIndex = index.NewFileIndex()
			state.Inner.FileCache = core.NewFileByteCache(1 << 20)
			cfg := &config.Config{StoragePath: directory}
			cfg.Maven.Repositories = map[string]*config.Repository{tc.format: repo}
			state.Inner.Config.Store(cfg)
			path := filepath.Join(directory, tc.format, filepath.FromSlash(tc.path))
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
			require.NoError(t, os.WriteFile(path, []byte("trusted"), 0600))
			old := time.Now().Add(-time.Hour)
			require.NoError(t, os.Chtimes(path, old, old))
			state.Inner.FileIndex.InsertFile(filepath.ToSlash(path), index.FileInfo{Size: 7, ModTime: old.UnixNano()})
			lock := &core.ResourceLock{Format: tc.format, Repository: tc.format,
				Name: "demo", Version: "2.0.0-RC", Source: core.ResourceLockSystem, Mode: core.ResourceLockWrite,
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
				req := httptest.NewRequest(method, "/"+tc.format+"/"+path, nil)
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
			status, body, etag := get("GET", tc.path, "")
			require.Equal(t, http.StatusOK, status)
			require.Equal(t, "trusted", string(body))
			require.Equal(t, int64(0), upstreamCalls.Load())
			lock.Mode = core.ResourceLockRead
			require.NoError(t, db.SetResourceLock(lock, "", ""))
			for _, method := range []string{"GET", "HEAD"} {
				status, body, _ = get(method, tc.path, etag)
				require.Equal(t, http.StatusNotFound, status)
				require.NotContains(t, string(body), "trusted")
				status, _, _ = get(method, strings.ToUpper(tc.path), etag)
				require.Equal(t, http.StatusNotFound, status)
			}
			if runtime.GOOS == "windows" {
				status, _, _ = get("GET", strings.Replace(tc.path, "RC", "rc", 1), etag)
				require.Equal(t, http.StatusNotFound, status)
				visible, err := db.ResourceMetadataVisibility(tc.format, tc.format, "", false, []core.ResourceLockTarget{{Name: "demo", Version: "2.0.0-rc"}})
				require.NoError(t, err)
				require.Equal(t, []bool{false}, visible)
			}
			lock.Mode = core.ResourceLockWrite
			require.NoError(t, db.SetResourceLock(lock, "", ""))
			require.NoError(t, os.Remove(path))
			state.Inner.FileIndex.RemoveFile(filepath.ToSlash(path))
			status, _, _ = get("GET", tc.path, "")
			require.Equal(t, http.StatusNotFound, status)
			require.Equal(t, int64(0), upstreamCalls.Load())
			dl := state.Inner.InFlightDownloads.AcquirePath(filepath.ToSlash(path))
			reader, err := proxy.ProxyArtifact(state, repo, tc.path, directory, filepath.ToSlash(path), dl)
			require.ErrorIs(t, err, core.ErrResourceLocked)
			require.Nil(t, reader)
			require.Equal(t, int64(0), upstreamCalls.Load())
		})
	}
}
