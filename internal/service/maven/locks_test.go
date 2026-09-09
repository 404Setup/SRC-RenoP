/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
	"renop/internal/service/storage"
	"renop/internal/utils"
)

func TestMavenVersionDeletionRejectsCoordinatePathAliases(t *testing.T) {
	for _, coordinate := range []struct{ group, artifact, version string }{
		{"com.example", "demo", "."}, {"com.example", "demo", ".."},
		{"com.example", "demo", "2.0."}, {"com.example", "demo.", "2.0"},
		{"com.example", "demo", "../demo/2.0"}, {"com.example", "demo", `..\demo\2.0`},
		{"com.example/other/..", "demo", "2.0"}, {"com.example", "demo/../demo", "2.0"},
	} {
		t.Run(coordinate.group+":"+coordinate.artifact+":"+coordinate.version, func(t *testing.T) {
			state, _ := newMavenRouteState(t)
			cfg, db := state.Inner.Config.Load(), state.GetDB()
			root := filepath.Join(cfg.StoragePath, "releases")
			file := filepath.Join(root, "com/example/demo/2.0/demo-2.0.jar")
			target := filepath.Join(root, strings.ReplaceAll(coordinate.group, ".", "/"), coordinate.artifact, coordinate.version)
			require.True(t, utils.IsSubPath(cfg.StoragePath, target))
			require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
			require.NoError(t, os.WriteFile(file, []byte("locked"), 0600))
			state.Inner.FileIndex.EnsureParentDirs(file)
			state.Inner.FileIndex.InsertFile(file, index.FileInfo{Size: 6, ModTime: time.Now().UnixNano()})
			require.NoError(t, db.SetResourceLock(&core.ResourceLock{ResourceLockTarget: mavenLockTarget("releases", "com.example", "demo", "2.0"),
				Source: core.ResourceLockSystem, Mode: core.ResourceLockRead, Reason: "trojan", LockedAt: time.Now().UnixMilli()}, "", ""))
			err := storage.RemoveMavenVersion(state, "releases", coordinate.group, coordinate.artifact, coordinate.version)
			_, statErr := os.Stat(file)
			require.ErrorIs(t, err, core.ErrMavenVersionNotFound, "Locked file preserved: %t", statErr == nil)
			require.NoError(t, statErr)
		})
	}
}

func TestMavenLocksSeparateMetadataFromBytesAndRejectFrozenWrites(t *testing.T) {
	state, _ := newMavenRouteState(t)
	db, cfg := state.GetDB(), state.Inner.Config.Load()
	now := time.Now().UnixMilli()
	users := map[string]*config.User{
		"guest": {Username: "guest"}, "alice": {Username: "alice"}, "bob": {Username: "bob"},
		"admin":  {Username: "admin", Roles: []string{"admin"}},
		"staff":  {Username: "staff", Roles: []string{"canmoderate:releases"}},
		"writer": {Username: "writer", Roles: []string{"canupdate:releases"}},
		"global": {Username: "global", Roles: []string{"canmoderate:*"}},
		"scoped": {Username: "global", Roles: []string{"canview:releases"}},
	}
	for name, user := range users {
		if name == "guest" || name == "scoped" {
			continue
		}
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: name, Permissions: user.Roles}))
		session := &core.Session{PublicID: name, Username: name, CreatedAt: now}
		session.LastActive.Store(now)
		require.NoError(t, db.SaveSession(session, name+"-session"))
	}
	team := &core.SuperTeam{Prefix: "domain-team", Name: "Domain", CreatedAt: now}
	require.NoError(t, db.CreateSuperTeam(team, "alice", 5, 10))
	domain := &core.MavenDomain{Domain: "com.example", SuperTeamPrefix: team.Prefix, VerificationType: core.MavenVerificationDNS,
		VerificationHost: "example.com", VerificationCode: "renop-verification=locks", CreatedAt: now}
	require.NoError(t, db.CreateMavenDomain(domain, "alice"))
	require.NoError(t, db.MarkMavenDomainVerified(domain.Domain, domain.VerificationCode, now))
	require.NoError(t, db.ForceAddMavenMembers(domain.Domain, "alice", []string{"bob"}, 0))
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		name := c.Get("X-Test-User", "guest")
		c.Locals("user", users[name])
		c.Locals("current_session_id", name+"-session")
		c.Locals("auth_credential_kind", "session")
		return c.Next()
	})
	SetupRoutes(app.Group("/api"), state)
	storage.SetupRoutes(app, state)
	request := func(method, url, user, body string, cookie bool) (int, []byte) {
		t.Helper()
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("X-Test-User", user)
		req.Header.Set("Content-Type", "application/json")
		if cookie {
			req.AddCookie(&http.Cookie{Name: "renop_session", Value: user + "-session"})
		}
		res, err := app.Test(req)
		require.NoError(t, err)
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		return res.StatusCode, data
	}
	for _, version := range []string{"1.0-SNAPSHOT", "2.0"} {
		status, body := request(http.MethodPut, "/releases/com/example/demo/"+version+"/demo-"+version+".jar", "alice", "artifact", true)
		require.Equal(t, http.StatusCreated, status, string(body))
	}
	metadata := `<metadata><groupId>com.example</groupId><artifactId>demo</artifactId><versioning><latest>2.0</latest><release>2.0</release><versions><version>1.0-SNAPSHOT</version><version>2.0</version></versions></versioning></metadata>`
	metadataPath := filepath.Join(cfg.StoragePath, "releases/com/example/demo/maven-metadata.xml")
	require.NoError(t, os.WriteFile(metadataPath, []byte(metadata), 0600))
	state.Inner.FileIndex.InsertFile(metadataPath, index.FileInfo{Size: int64(len(metadata)), ModTime: time.Now().UnixNano()})
	const lockURL = "/api/maven/repositories/releases/package/locks?group=com.example&artifact=demo"
	const detailURL = "/api/maven/repositories/releases/package?group=com.example&artifact=demo"
	for _, user := range []string{"alice", "bob", "writer", "guest"} {
		status, _ := request(http.MethodPut, lockURL, user, `{"version":"2.0","mode":"read","reason":"trojan"}`, true)
		require.Equal(t, http.StatusForbidden, status)
	}
	status, _ := request(http.MethodPut, lockURL, "staff", `{"version":"2.0","mode":"read","reason":"trojan"}`, false)
	require.Equal(t, http.StatusForbidden, status)
	status, body := request(http.MethodPut, lockURL, "staff", `{"version":"2.0","mode":"read","reason":"trojan"}`, true)
	require.Equal(t, http.StatusOK, status, string(body))
	for _, user := range []string{"guest", "writer", "alice", "bob", "staff", "admin"} {
		status, body = request(http.MethodGet, detailURL, user, "", true)
		require.Equal(t, http.StatusOK, status, string(body))
		var details core.MavenArtifactDetails
		require.NoError(t, json.Unmarshal(body, &details))
		inspect := user != "guest" && user != "writer"
		if inspect {
			require.Len(t, details.Versions, 2)
		} else {
			require.Len(t, details.Versions, 1)
			require.Equal(t, "1.0-SNAPSHOT", details.Artifact.LatestVersion)
		}
		status, body = request(http.MethodGet, "/releases/com/example/demo/maven-metadata.xml", user, "", true)
		require.Equal(t, http.StatusOK, status, string(body))
		var projected config.Metadata
		require.NoError(t, xml.Unmarshal(body, &projected))
		if inspect {
			require.Len(t, projected.Versioning.Versions.Version, 2)
		} else {
			require.Equal(t, []string{"1.0-SNAPSHOT"}, projected.Versioning.Versions.Version)
			require.Equal(t, "1.0-SNAPSHOT", *projected.Versioning.Latest)
			require.Nil(t, projected.Versioning.Release)
		}
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			status, _ = request(method, "/releases/com/example/demo/2.0/demo-2.0.jar", user, "", true)
			require.Equal(t, http.StatusNotFound, status)
		}
	}
	require.ErrorIs(t, EnsurePathMutable(state, cfg.Maven.Repositories["releases"], "com/example/demo/2.0/other.txt"), core.ErrResourceLocked)
	for _, url := range []string{"/releases/com/example/demo/2.0/demo-2.0.jar.sha256", "/releases/com/example/demo/maven-metadata.xml"} {
		status, body = request(http.MethodPut, url, "alice", "blocked", true)
		require.Equal(t, http.StatusLocked, status, string(body))
	}
	status, body = request(http.MethodDelete, "/api/maven/repositories/releases/versions?group=com.example&artifact=demo&version=2.0", "alice", "", true)
	require.Equal(t, http.StatusLocked, status, string(body))
	status, body = request(http.MethodPut, lockURL, "staff", `{"mode":"read","reason":"abuse"}`, true)
	require.Equal(t, http.StatusOK, status, string(body))
	for _, user := range []string{"guest", "writer"} {
		status, _ = request(http.MethodGet, detailURL, user, "", true)
		require.Equal(t, http.StatusNotFound, status)
		status, _ = request(http.MethodGet, "/releases/com/example/demo/maven-metadata.xml", user, "", true)
		require.Equal(t, http.StatusNotFound, status)
	}
	status, body = request(http.MethodDelete, lockURL, "staff", `{}`, true)
	require.Equal(t, http.StatusOK, status, string(body))
	status, body = request(http.MethodDelete, lockURL, "staff", `{"version":"2.0"}`, true)
	require.Equal(t, http.StatusOK, status, string(body))
	status, _ = request(http.MethodGet, "/releases/com/example/demo/2.0/demo-2.0.jar", "guest", "", false)
	require.Equal(t, http.StatusOK, status)
	t.Run("team locks cover domain metadata, uncatalogued files, and independent subdomains", func(t *testing.T) {
		nested := &core.MavenDomain{Domain: "com.example.independent", VerificationType: core.MavenVerificationDNS,
			VerificationHost: "independent.example.com", VerificationCode: "nested", CreatedAt: now}
		require.NoError(t, db.CreateMavenDomain(nested, "writer"))
		require.NoError(t, db.MarkMavenDomainVerified(nested.Domain, nested.VerificationCode, now))
		for relative, data := range map[string]string{
			"com/example/maven-metadata.xml":                  `<metadata><plugins><plugin><prefix>demo</prefix><artifactId>demo</artifactId></plugin></plugins></metadata>`,
			"com/example/uncatalogued/file.bin":               "private bytes",
			"com/example/independent/other/1.0/other-1.0.jar": "independent bytes",
		} {
			file := filepath.Join(cfg.StoragePath, "releases", filepath.FromSlash(relative))
			require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
			require.NoError(t, os.WriteFile(file, []byte(data), 0600))
			state.Inner.FileIndex.EnsureParentDirs(file)
			state.Inner.FileIndex.InsertFile(file, index.FileInfo{Size: int64(len(data)), ModTime: time.Now().UnixNano()})
		}
		lock := &core.ResourceLock{ResourceLockTarget: core.ResourceLockTarget{Format: "superteam", Name: team.Prefix},
			Mode: core.ResourceLockRead, Source: core.ResourceLockSystem, Reason: "hold", LockedAt: now}
		require.NoError(t, db.SetResourceLock(lock, "", ""))
		for _, viewer := range []string{"guest", "writer", "alice", "bob", "staff", "admin"} {
			inspect := viewer != "guest" && viewer != "writer"
			want := http.StatusNotFound
			if inspect {
				want = http.StatusOK
			}
			for _, path := range []string{"com/example/maven-metadata.xml", "com/example/demo/maven-metadata.xml"} {
				status, body := request(http.MethodGet, "/releases/"+path, viewer, "", true)
				require.Equal(t, want, status, viewer+": "+string(body))
			}
			status, _ := request(http.MethodGet, "/releases/com/example/uncatalogued/file.bin", viewer, "", true)
			require.Equal(t, http.StatusNotFound, status, viewer)
			status, body := request(http.MethodGet, "/releases/com/example/independent/other/1.0/other-1.0.jar", viewer, "", true)
			require.Equal(t, http.StatusOK, status, viewer+": "+string(body))
			paths := []string{"com/example", "com/example/demo", "com/example/uncatalogued/file.bin", "com/example/independent/other/1.0"}
			visible, err := VisibleMetadataPaths(state, users[viewer], "releases", paths)
			require.NoError(t, err)
			require.Equal(t, []bool{inspect, inspect, inspect, true}, visible, viewer)
			filter, err := MetadataPathFilter(state, users[viewer], "releases")
			require.NoError(t, err)
			for i, path := range paths {
				require.Equal(t, visible[i], filter(path), viewer+": "+path)
			}
		}
		status, body := request(http.MethodGet, "/api/maven/domains/com.example", "alice", "", true)
		require.Equal(t, http.StatusOK, status, string(body))
		var details core.MavenDomainDetails
		require.NoError(t, json.Unmarshal(body, &details))
		require.Len(t, details.Domain.Locks, 1)
		status, _ = request(http.MethodGet, "/api/maven/domains/com.example", "guest", "", false)
		require.Equal(t, http.StatusNotFound, status)
		status, body = request(http.MethodPut, "/releases/com/example/new/1.0/new-1.0.jar", "alice", "blocked", true)
		require.Equal(t, http.StatusLocked, status, string(body))
		status, body = request(http.MethodPost, "/api/maven/domains/com.example/close", "admin", "", true)
		require.Equal(t, http.StatusLocked, status, string(body))
		require.ErrorIs(t, db.ReserveMavenVerificationAttempt(domain.Domain, "alice", false, now+1, now), core.ErrResourceLocked)
		require.ErrorIs(t, EnsurePathMutable(state, cfg.Maven.Repositories["releases"], "com/example/uncatalogued/new.bin"), core.ErrResourceLocked)
		require.NoError(t, db.DeleteResourceLock(lock.ResourceLockTarget, core.ResourceLockSystem, "", ""))
		status, _ = request(http.MethodGet, "/releases/com/example/uncatalogued/file.bin", "guest", "", false)
		require.Equal(t, http.StatusOK, status)
	})
	t.Run("manual domain locks require live global authority and preserve system restrictions", func(t *testing.T) {
		const url = "/api/maven/domains/com.example/locks"
		const body = `{"mode":"read","reason":"quality"}`
		for _, viewer := range []string{"guest", "alice", "bob", "writer", "staff", "scoped"} {
			status, _ := request(http.MethodPut, url, viewer, body, true)
			require.Equal(t, http.StatusForbidden, status, viewer)
		}
		status, _ := request(http.MethodPut, url, "global", body, false)
		require.Equal(t, http.StatusForbidden, status)
		status, data := request(http.MethodPut, url, "global", body, true)
		require.Equal(t, http.StatusNoContent, status, string(data))
		for _, viewer := range []string{"alice", "bob", "global", "admin", "guest", "scoped"} {
			status, data := request(http.MethodGet, "/api/maven/domains/com.example", viewer, "", true)
			if viewer == "guest" || viewer == "scoped" {
				require.Equal(t, http.StatusNotFound, status, viewer)
				continue
			}
			require.Equal(t, http.StatusOK, status, string(data))
			var details core.MavenDomainDetails
			require.NoError(t, json.Unmarshal(data, &details))
			require.Len(t, details.Domain.Locks, 1)
			require.Equal(t, viewer == "global" || viewer == "admin", details.Moderator)
			if viewer == "alice" {
				require.Equal(t, core.MavenPermissionOwner, details.Domain.PermissionLevel)
				require.Len(t, details.Members, 2)
			}
			status, _ = request(http.MethodGet, "/releases/com/example/uncatalogued/file.bin", viewer, "", true)
			require.Equal(t, http.StatusNotFound, status, viewer)
		}
		locks, err := db.GetResourceLocks(mavenLockTarget("releases", "com.example", "demo", "2.0"), false)
		require.NoError(t, err)
		require.Len(t, locks, 1)
		require.True(t, locks[0].Inherited)
		status, _ = request(http.MethodPut, "/releases/com/example/new/1.0/new-1.0.jar", "alice", "blocked", true)
		require.Equal(t, http.StatusLocked, status)
		target := core.ResourceLockTarget{Format: "maven-domain", Name: domain.Domain}
		require.NoError(t, db.SetResourceLock(&core.ResourceLock{ResourceLockTarget: target, Source: core.ResourceLockSystem,
			Mode: core.ResourceLockWrite, Reason: "hold", LockedAt: now}, "", ""))
		status, _ = request(http.MethodDelete, url, "global", `{}`, true)
		require.Equal(t, http.StatusNoContent, status)
		locks, err = db.GetResourceLocks(target, false)
		require.NoError(t, err)
		require.Len(t, locks, 1)
		require.Equal(t, core.ResourceLockSystem, locks[0].Source)
		require.ErrorIs(t, db.EnsureResourceMutable(target, false), core.ErrResourceLocked)
		status, _ = request(http.MethodGet, "/releases/com/example/uncatalogued/file.bin", "guest", "", false)
		require.Equal(t, http.StatusOK, status)
		status, _ = request(http.MethodPut, url, "global", body, true)
		require.Equal(t, http.StatusNoContent, status)
		require.NoError(t, db.DeleteResourceLock(target, core.ResourceLockSystem, "", ""))
		require.ErrorIs(t, db.EnsureResourceMutable(target, false), core.ErrResourceLocked)
		require.NoError(t, db.DeleteSession("global-session"))
		status, _ = request(http.MethodDelete, url, "global", `{}`, true)
		require.Equal(t, http.StatusForbidden, status)
		status, _ = request(http.MethodDelete, url, "admin", `{}`, true)
		require.Equal(t, http.StatusNoContent, status)
		require.NoError(t, db.EnsureResourceMutable(target, false))
	})
}
