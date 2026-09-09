/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
)

func TestNPMLocksProtectPackumentsTagsAndTarballs(t *testing.T) {
	config.ClearRepoCacheConfigs()
	t.Cleanup(config.ClearRepoCacheConfigs)
	_, state, store := setupNPMTestApp(t)
	db, cfg := state.GetDB(), state.Inner.Config.Load()
	repo := cfg.Maven.Repositories["npm"]
	now := time.Now().UnixMilli()
	users := map[string]*config.User{
		"guest": {Username: "guest"}, "alice": {Username: "alice", Roles: []string{"canupdate:npm"}},
		"reader": {Username: "reader"}, "writer": {Username: "writer", Roles: []string{"canupdate:npm"}},
		"staff":   {Username: "staff", Roles: []string{"canmoderate:npm"}},
		"outside": {Username: "outside", Roles: []string{"canmoderate:other"}},
		"admin":   {Username: "admin", Roles: []string{"admin"}},
	}
	for name, user := range users {
		if name == "guest" {
			continue
		}
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: name, Permissions: user.Roles}))
		session := &core.Session{PublicID: name, Username: name, CreatedAt: now}
		session.LastActive.Store(now)
		require.NoError(t, db.SaveSession(session, name+"-session"))
	}
	require.NoError(t, db.ForceAddNPMMembers("npm", "demo", "alice", []string{"reader"}, 0))
	for _, version := range []string{"1.0.0", "2.0.0"} {
		path := canonicalTarballPath("demo", version)
		require.NoError(t, db.RecordNPMPublication(&core.NPMPackage{Repository: "npm", Name: "demo", UpdatedAt: now},
			&core.NPMVersion{Repository: "npm", Package: "demo", Version: version, TarballPath: path, CreatedAt: now,
				ManifestJSON: `{"name":"demo","version":"` + version + `"}`}, map[string]string{"latest": version}, "alice"))
		store.files[filepath.Join(cfg.StoragePath, "npm", filepath.FromSlash(path))] = []byte("tarball-" + version)
	}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		name := c.Get("X-Test-User", "guest")
		c.Locals("user", users[name])
		c.Locals("current_session_id", name+"-session")
		c.Locals("auth_credential_kind", c.Get("X-Test-Credential", "session"))
		return c.Next()
	})
	SetupRoutes(app.Group("/api"), state, store)
	handler := Handler{Store: store}
	app.All("/npm/*", func(c fiber.Ctx) error {
		if handled, err := handler.Handle(c, state, repo, cfg.StoragePath, c.Params("*")); handled {
			return err
		}
		path, _ := decodeRegistryPath(c.Params("*"))
		reader, exists, err := store.Open(filepath.Join(cfg.StoragePath, "npm", filepath.FromSlash(path)))
		if err != nil || !exists {
			return fiber.ErrNotFound
		}
		return c.SendStream(reader)
	})
	request := func(method, url, user, body string, headers map[string]string) (int, []byte, http.Header) {
		t.Helper()
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("X-Test-User", user)
		req.Header.Set("Content-Type", "application/json")
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		if headers["X-No-Cookie"] == "" {
			req.AddCookie(&http.Cookie{Name: "renop_session", Value: user + "-session"})
		}
		response, err := app.Test(req)
		require.NoError(t, err)
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return response.StatusCode, data, response.Header
	}
	status, _, headers := request("GET", "/npm/demo", "guest", "", nil)
	require.Equal(t, 200, status)
	etag := headers.Get("ETag")
	endpoint, body := "/api/npm/repositories/npm/locks?package=demo", `{"version":"2.0.0","mode":"read","reason":"trojan"}`
	for _, user := range []string{"alice", "outside"} {
		status, _, _ = request("PUT", endpoint, user, body, nil)
		require.Equal(t, 403, status)
	}
	for _, extra := range []map[string]string{{"X-No-Cookie": "1"}, {"X-Test-Credential": "api_token"}} {
		status, _, _ = request("PUT", endpoint, "staff", body, extra)
		require.Equal(t, 403, status)
	}
	status, _, _ = request("PUT", endpoint, "staff", body, nil)
	require.Equal(t, 200, status)
	for _, user := range []string{"guest", "writer", "reader", "alice", "staff", "admin"} {
		status, data, headers := request("GET", "/npm/demo", user, "", map[string]string{"If-None-Match": etag})
		require.Equal(t, 200, status)
		require.Contains(t, headers.Get("Cache-Control"), "no-store")
		var document map[string]any
		require.NoError(t, json.Unmarshal(data, &document))
		versions := document["versions"].(map[string]any)
		if user == "guest" || user == "writer" {
			require.Len(t, versions, 1)
			require.NotContains(t, versions, "2.0.0")
			require.Empty(t, document["dist-tags"])
		} else {
			require.Len(t, versions, 2)
		}
		for _, method := range []string{"GET", "HEAD"} {
			status, data, _ = request(method, "/npm/demo/-/demo-2.0.0.tgz", user, "", nil)
			require.Equal(t, 404, status)
			require.NotContains(t, string(data), "tarball-")
		}
	}
	status, data, _ := request("GET", "/npm/-/package/demo/dist-tags", "guest", "", nil)
	require.Equal(t, 200, status)
	require.JSONEq(t, `{}`, string(data))
	status, data, _ = request("GET", "/api/npm/repositories/npm/packages?package=demo", "guest", "", nil)
	require.Equal(t, 200, status)
	var details core.NPMPackageDetails
	require.NoError(t, json.Unmarshal(data, &details))
	require.Len(t, details.Versions, 1)
	require.Equal(t, "1.0.0", details.Package.LatestVersion)
	status, data, _ = request("GET", "/npm/demo/-/demo-1.0.0.tgz", "guest", "", nil)
	require.Equal(t, 200, status)
	require.Equal(t, "tarball-1.0.0", string(data))
	status, _, _ = request("PUT", endpoint, "staff", `{"mode":"read","reason":"abuse"}`, nil)
	require.Equal(t, 200, status)
	for _, user := range []string{"guest", "writer", "reader", "alice", "staff", "admin"} {
		status, _, _ = request("GET", "/npm/demo", user, "", nil)
		if user == "guest" || user == "writer" {
			require.Equal(t, 404, status)
		} else {
			require.Equal(t, 200, status)
		}
	}
	status, _, _ = request("GET", "/api/npm/repositories/npm/owners?package=demo", "writer", "", nil)
	require.Equal(t, 403, status)
	status, _, headers = request("PUT", "/api/npm/repositories/npm/packages?package=demo", "alice", `{"description":"changed"}`, nil)
	require.Equal(t, 423, status)
	require.Equal(t, "resource_locked", headers.Get(npmAPIErrorCodeHeader))
	status, _, _ = request("DELETE", endpoint, "staff", `{}`, nil)
	require.Equal(t, 200, status)
	status, _, _ = request("GET", "/npm/demo", "guest", "", nil)
	require.Equal(t, 200, status)
	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)
	repo.Mirrors = []config.Mirror{{URL: upstream.URL, CacheTTLSecs: 1}}
	mirrored := &core.NPMPackage{Repository: "npm", Name: "mirror-demo", Description: "original", Mirrored: true,
		CreatedAt: now - 10000, UpdatedAt: now - 10000}
	mirroredVersions := []*core.NPMVersion{{Version: "1.0.0", ManifestJSON: `{"name":"mirror-demo","version":"1.0.0"}`, CreatedAt: now - 10000}}
	require.NoError(t, db.RecordNPMMirrorPublication(mirrored, mirroredVersions, map[string]string{"latest": "1.0.0"}))
	require.NoError(t, db.SetResourceLock(&core.ResourceLock{ResourceLockTarget: npmLockTarget("npm", "mirror-demo", "1.0.0"),
		Source: core.ResourceLockSystem, Mode: core.ResourceLockWrite, Reason: "hold", LockedAt: now}, "", ""))
	status, data, _ = request("GET", "/npm/mirror-demo", "guest", "", nil)
	require.Equal(t, 200, status)
	require.Contains(t, string(data), "original")
	require.Zero(t, upstreamCalls.Load())
	mirrored.Description = "replacement"
	require.ErrorIs(t, db.RecordNPMMirrorPublication(mirrored, mirroredVersions, map[string]string{"latest": "1.0.0"}), core.ErrResourceLocked)
}
