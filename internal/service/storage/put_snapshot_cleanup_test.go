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
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
	"renop/internal/service/proxy"
	"renop/internal/utils"
)

func removeAllWithRetry(path string) {
	var last error
	for i := range 40 {
		last = os.RemoveAll(path)
		if last == nil {
			return
		}
		d := min(time.Duration(5*(i+1))*time.Millisecond, 100*time.Millisecond)
		time.Sleep(d)
	}
	_ = last
}

func setupSnapshotPutApp(t *testing.T) (*fiber.App, *core.AppState, string, *config.Repository) {
	t.Helper()
	storagePath := storageTestTempDir(t)
	if runtime.GOOS == "windows" {
		t.Cleanup(func() { removeAllWithRetry(storagePath) })
	}
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	InitS3(cfg)

	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()
	repo := cfg.Maven.Repositories["snapshots"]
	if repo == nil {
		repo = &config.Repository{
			Name:              "snapshots",
			Visibility:        "PUBLIC",
			AllowRedeployment: true,
		}
		cfg.Maven.Repositories["snapshots"] = repo
	}
	repo.AllowRedeployment = true

	app := fiber.New(fiber.Config{StreamRequestBody: true, UnescapePath: false})
	app.Put("/:repo_name/*", func(c fiber.Ctx) error {
		path, ok := utils.SanitizePath(c.Params("*"))
		if !ok {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		localPath := filepath.Join(storagePath, c.Params("repo_name"), path)
		return HandlePut(c, state, repo, localPath)
	})
	return app, state, storagePath, repo
}

func putBytes(t *testing.T, app *fiber.App, url string, body []byte, headers map[string]string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func mustWriteIndexed(t *testing.T, state *core.AppState, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	state.Inner.FileIndex.EnsureParentDirs(path)
	state.Inner.FileIndex.InsertFile(path, index.FileInfo{Size: int64(len(content)), ModTime: 1})
}

func TestSnapshotOverwriteCleansCompanions(t *testing.T) {
	app, state, storagePath, _ := setupSnapshotPutApp(t)

	versionDir := filepath.Join(storagePath, "snapshots", "com", "example", "demo", "1.0-SNAPSHOT")
	jarPath := filepath.Join(versionDir, "demo-1.0-SNAPSHOT.jar")
	mustWriteIndexed(t, state, jarPath, []byte("old-jar"))
	for _, ext := range []string{".md5", ".sha1", ".sha256", ".sha512", ".asc"} {
		mustWriteIndexed(t, state, jarPath+ext, []byte("stale-"+ext))
	}

	code := putBytes(t, app, "/snapshots/com/example/demo/1.0-SNAPSHOT/demo-1.0-SNAPSHOT.jar",
		[]byte("new-jar-bytes"), map[string]string{"X-Generate-Checksums": "true"})
	if code != fiber.StatusCreated {
		t.Fatalf("PUT status = %d, want %d", code, fiber.StatusCreated)
	}

	got, err := os.ReadFile(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-jar-bytes" {
		t.Fatalf("jar content = %q, want new-jar-bytes", got)
	}

	for _, ext := range []string{".md5", ".sha1", ".sha256", ".sha512"} {
		p := jarPath + ext
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("expected regenerated %s: %v", ext, err)
		}
		if bytes.HasPrefix(data, []byte("stale-")) {
			t.Fatalf("%s still holds stale content: %q", ext, data)
		}
		if !state.Inner.FileIndex.HasFile(p) {
			t.Fatalf("index missing regenerated %s", ext)
		}
	}
	if _, err := os.Stat(jarPath + ".asc"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected old .asc to be removed, stat err=%v", err)
	}
	if state.Inner.FileIndex.HasFile(jarPath + ".asc") {
		t.Fatal("index still lists old .asc")
	}
}

func TestUniqueSnapshotUploadPurgesOlderBuild(t *testing.T) {
	app, state, storagePath, _ := setupSnapshotPutApp(t)

	versionDir := filepath.Join(storagePath, "snapshots", "com", "example", "demo", "1.0-SNAPSHOT")
	oldBase := "demo-1.0-20240101.120000-1"
	newBase := "demo-1.0-20240202.130000-2"

	oldJar := filepath.Join(versionDir, oldBase+".jar")
	oldSources := filepath.Join(versionDir, oldBase+"-sources.jar")
	mustWriteIndexed(t, state, oldJar, []byte("old-unique-jar"))
	mustWriteIndexed(t, state, oldJar+".md5", []byte("old-md5"))
	mustWriteIndexed(t, state, oldJar+".sha1", []byte("old-sha1"))
	mustWriteIndexed(t, state, oldJar+".asc", []byte("old-asc"))
	mustWriteIndexed(t, state, oldSources, []byte("old-sources"))
	mustWriteIndexed(t, state, oldSources+".sha256", []byte("old-sources-sha"))
	nonUnique := filepath.Join(versionDir, "demo-1.0-SNAPSHOT.jar")
	mustWriteIndexed(t, state, nonUnique, []byte("non-unique-latest"))
	meta := filepath.Join(versionDir, "maven-metadata.xml")
	mustWriteIndexed(t, state, meta, []byte("<metadata/>"))

	newJarURL := "/snapshots/com/example/demo/1.0-SNAPSHOT/" + newBase + ".jar"
	code := putBytes(t, app, newJarURL, []byte("new-unique-jar"), nil)
	if code != fiber.StatusCreated {
		t.Fatalf("PUT status = %d, want %d", code, fiber.StatusCreated)
	}

	for _, p := range []string{
		oldJar, oldJar + ".md5", oldJar + ".sha1", oldJar + ".asc",
		oldSources, oldSources + ".sha256",
	} {
		if _, err := os.Stat(p); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("expected old file removed: %s (err=%v)", p, err)
		}
		if state.Inner.FileIndex.HasFile(p) {
			t.Fatalf("index still has old file: %s", p)
		}
	}

	newJar := filepath.Join(versionDir, newBase+".jar")
	if _, err := os.Stat(newJar); err != nil {
		t.Fatalf("new jar missing: %v", err)
	}
	if _, err := os.Stat(nonUnique); err != nil {
		t.Fatalf("non-unique SNAPSHOT jar should be kept: %v", err)
	}
	if _, err := os.Stat(meta); err != nil {
		t.Fatalf("maven-metadata.xml should be kept: %v", err)
	}
}

func TestUniqueSnapshotBuildNumberFromMirrorPurgesOlderBuild(t *testing.T) {
	_, state, storagePath, repo := setupSnapshotPutApp(t)
	versionDir := filepath.Join(storagePath, "snapshots", "com", "example", "demo", "1.0.0-SNAPSHOT")
	oldPath := filepath.Join(versionDir, "demo-1.0.0-1.jar")
	mustWriteIndexed(t, state, oldPath, []byte("old"))
	mustWriteIndexed(t, state, oldPath+".sha1", []byte("old-sha"))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "demo-1.0.0-2.jar") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("new"))
	}))
	t.Cleanup(upstream.Close)
	repo.Mirrors = []config.Mirror{{URL: upstream.URL, TimeoutSecs: 5}}

	path := "com/example/demo/1.0.0-SNAPSHOT/demo-1.0.0-2.jar"
	localPath := filepath.ToSlash(filepath.Join(storagePath, repo.Name, path))
	dl, _ := state.Inner.InFlightDownloads.LockPath(localPath)
	stream, err := proxy.ProxyArtifact(state, repo, path, storagePath, localPath, dl)
	if err != nil {
		t.Fatalf("ProxyArtifact: %v", err)
	}
	if _, err := io.Copy(io.Discard, stream); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(oldPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("old numeric snapshot still exists, err=%v", err)
	}
	if state.Inner.FileIndex.HasFile(oldPath) || state.Inner.FileIndex.HasFile(oldPath+".sha1") {
		t.Fatal("old numeric snapshot remains in the file index")
	}
}

func TestSnapshotMetadataPurgesBuildsNotAdvertisedUpstream(t *testing.T) {
	_, state, storagePath, _ := setupSnapshotPutApp(t)
	versionDir := filepath.Join(storagePath, "snapshots", "com", "example", "demo", "1.0.0-SNAPSHOT")
	oldPath := filepath.Join(versionDir, "demo-1.0.0-1.jar")
	keepPath := filepath.Join(versionDir, "demo-1.0.0-2.jar")
	mustWriteIndexed(t, state, oldPath, []byte("old"))
	mustWriteIndexed(t, state, oldPath+".asc", []byte("old-signature"))
	mustWriteIndexed(t, state, keepPath, []byte("keep"))
	mustWriteIndexed(t, state, keepPath+".sha1", []byte("keep-sha"))
	metadataPath := filepath.Join(versionDir, "maven-metadata.xml")
	metadata := []byte(`<metadata><artifactId>demo</artifactId><version>1.0.0-SNAPSHOT</version><versioning><snapshotVersions><snapshotVersion><extension>jar</extension><value>1.0.0-2</value></snapshotVersion></snapshotVersions></versioning></metadata>`)
	mustWriteIndexed(t, state, metadataPath, metadata)

	if err := cleanupSnapshotArtifactsFromMetadata(state, metadataPath); err != nil {
		t.Fatalf("cleanupSnapshotArtifactsFromMetadata: %v", err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("metadata-stale build still exists, err=%v", err)
	}
	if _, err := os.Stat(oldPath + ".asc"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("metadata-stale signature still exists, err=%v", err)
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("advertised build was removed: %v", err)
	}
}

type uniqueSnapshotBaseNameTestCase struct {
	name    string
	ok      bool
	prefix  string
	unique  string
	classif string
	ext     string
}

func TestParseUniqueSnapshotBaseName(t *testing.T) {
	cases := []uniqueSnapshotBaseNameTestCase{
		{name: "demo-1.0-20240101.120000-1.jar", ok: true, prefix: "demo-1.0", unique: "20240101.120000-1", classif: "", ext: ".jar"},
		{name: "demo-1.0-20240101.120000-1-sources.jar", ok: true, prefix: "demo-1.0", unique: "20240101.120000-1", classif: "-sources", ext: ".jar"},
		{name: "demo-1.0.0-2.jar", ok: true, prefix: "demo-1.0.0", unique: "2", classif: "", ext: ".jar"},
		{name: "demo-1.0.0-2-sources.jar", ok: true, prefix: "demo-1.0.0", unique: "2", classif: "-sources", ext: ".jar"},
		{name: "demo-1.0-SNAPSHOT.jar", ok: false, prefix: "", unique: "", classif: "", ext: ""},
		{name: "maven-metadata.xml", ok: false, prefix: "", unique: "", classif: "", ext: ""},
	}
	for _, tc := range cases {
		parts, ok := parseUniqueSnapshotBaseName(tc.name)
		if ok != tc.ok {
			t.Fatalf("%s: ok=%v want %v", tc.name, ok, tc.ok)
		}
		if !ok {
			continue
		}
		if parts.prefix != tc.prefix || parts.uniqueVer != tc.unique || parts.classifier != tc.classif || parts.primaryExt != tc.ext {
			t.Fatalf("%s: got %+v", tc.name, parts)
		}
	}
}
