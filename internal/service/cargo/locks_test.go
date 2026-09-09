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
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/testutil"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

func TestCargoResourceLocksCoverMetadataFilesAndMutations(t *testing.T) {
	directory := testutil.TempDir(t)
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(directory, "lock.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state := core.NewAppState()
	state.Inner.DB = db
	repo := &config.Repository{Name: "cargo", Format: "cargo", Visibility: "PUBLIC"}
	cfg := &config.Config{StoragePath: directory}
	cfg.Maven.Repositories = map[string]*config.Repository{"cargo": repo}
	state.Inner.Config.Store(cfg)
	now := time.Now().UnixMilli()
	users := map[string]*config.User{
		"guest": {Username: "guest"}, "owner": {Username: "owner"}, "reader": {Username: "reader"},
		"staff":   {Username: "staff", Roles: []string{"canmoderate:cargo"}},
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
	store := newMemoryStore()
	pkg := &core.CargoPackage{Repository: "cargo", Name: "demo", NormalizedName: "demo", CreatedAt: now, UpdatedAt: now}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		require.NoError(t, db.RecordCargoPublication(pkg, &core.CargoVersion{Repository: "cargo", Package: "demo",
			Version: version, Publisher: "owner", CreatedAt: now}, "owner"))
		entry, err := json.Marshal(IndexEntry{Name: "demo", Version: version})
		require.NoError(t, err)
		index := filepath.Join(directory, "cargo", "de", "mo", "demo")
		store.files[index] = append(store.files[index], append(entry, '\n')...)
		store.files[filepath.Join(directory, "cargo", "api", "v1", "crates", "demo", version, "download")] = []byte("crate")
	}
	reader, err := db.GetUserProfile("reader")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO cargo_members (repository, normalized_name, username, user_id, permission_level, added_at)
		VALUES (?, ?, ?, ?, ?, ?)`, "cargo", "demo", "reader", reader.UserID, 0, now)
	require.NoError(t, err)
	handler := Handler{Store: store}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		name := c.Get("X-Test-User", "guest")
		c.Locals("user", users[name])
		c.Locals("current_session_id", name+"-session")
		c.Locals("auth_credential_kind", c.Get("X-Test-Credential", "session"))
		return c.Next()
	})
	app.All("/:repo_name/*", func(c fiber.Ctx) error {
		path := c.Params("*")
		if handled, err := handler.Handle(c, state, repo, directory, path); handled {
			return err
		}
		if handled, err := handler.HandleReadLocks(c, state, repo, directory, path); handled {
			return err
		}
		reader, exists, err := store.Open(filepath.Join(directory, "cargo", filepath.FromSlash(path)))
		if err != nil || !exists {
			return fiber.ErrNotFound
		}
		return c.SendStream(reader)
	})
	request := func(user, method, path string, body any) (int, []byte) {
		t.Helper()
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(method, "/cargo/"+path, bytes.NewReader(encoded))
		req.Header.Set("X-Test-User", user)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "renop_session", Value: user + "-session"})
		response, err := app.Test(req)
		require.NoError(t, err)
		defer response.Body.Close()
		content, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return response.StatusCode, content
	}
	lock := map[string]string{"version": "2.0.0", "mode": "read", "reason": "trojan"}
	for _, user := range []string{"owner", "outside"} {
		status, _ := request(user, "PUT", "api/v1/crates/demo/locks", lock)
		require.Equal(t, http.StatusForbidden, status)
	}
	status, _ := request("staff", "PUT", "api/v1/crates/demo/locks", lock)
	require.Equal(t, http.StatusOK, status)
	for _, credential := range []string{"session", "api_token"} {
		req := httptest.NewRequest("DELETE", "/cargo/api/v1/crates/demo/locks", bytes.NewBufferString(`{"version":"2.0.0"}`))
		req.Header.Set("X-Test-User", "staff")
		req.Header.Set("X-Test-Credential", credential)
		if credential == "api_token" {
			req.AddCookie(&http.Cookie{Name: "renop_session", Value: "staff-session"})
		}
		response, err := app.Test(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	status, index := request("guest", "GET", "de/mo/demo", nil)
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, string(index), "1.0.0")
	require.NotContains(t, string(index), "2.0.0")
	for _, user := range []string{"guest", "owner", "reader", "staff", "outside", "admin"} {
		status, data := request(user, "GET", "api/v1/crates/demo", nil)
		require.Equal(t, http.StatusOK, status)
		if user == "guest" || user == "outside" {
			require.NotContains(t, string(data), "2.0.0")
		} else {
			require.Contains(t, string(data), "2.0.0")
			require.Contains(t, string(data), "trojan")
		}
		status, _ = request(user, "GET", "api/v1/crates/demo/2.0.0/download", nil)
		require.Equal(t, http.StatusNotFound, status)
	}
	status, _ = request("owner", "DELETE", "api/v1/crates/demo/2.0.0/yank", nil)
	require.Equal(t, http.StatusLocked, status)
	status, _ = request("admin", "DELETE", "api/v1/crates/demo", nil)
	require.Equal(t, http.StatusLocked, status)
	require.NoError(t, db.RecordCargoPublication(pkg, &core.CargoVersion{Repository: "cargo", Package: "demo",
		Version: "3.0.0", Publisher: "owner", CreatedAt: now}, "owner"))
	lock["version"] = ""
	status, _ = request("staff", "PUT", "api/v1/crates/demo/locks", lock)
	require.Equal(t, http.StatusOK, status)
	status, _ = request("guest", "GET", "api/v1/crates/demo", nil)
	require.Equal(t, http.StatusNotFound, status)
	status, _ = request("reader", "GET", "api/v1/crates/demo", nil)
	require.Equal(t, http.StatusOK, status)
	status, _ = request("admin", "DELETE", "api/v1/crates/demo/locks", lock)
	require.Equal(t, http.StatusOK, status)
	status, _ = request("guest", "GET", "api/v1/crates/demo/1.0.0/download", nil)
	require.Equal(t, http.StatusOK, status)
}
