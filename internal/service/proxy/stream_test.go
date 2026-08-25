/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package proxy

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
	"renop/internal/utils"
)

func TestCreateProxyStreamSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello proxy stream"))
	}))
	defer ts.Close()

	res, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	tempDir := t.TempDir()
	localFilePath := filepath.Join(tempDir, "test.txt")

	inFlightMgr := core.NewInFlightManager()
	dl, _ := inFlightMgr.LockPath("test")
	permit := make(chan struct{}, 1)
	permit <- struct{}{}

	stream := CreateProxyStream(res.Body, -1, localFilePath, inFlightMgr, "test", dl, permit, func() {}, nil, nil, maxProxyArtifactSize, nil)

	var buf bytes.Buffer
	_, err = io.Copy(&buf, stream)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	stream.Close()
	if err := stream.Close(); err != nil {
		t.Fatalf("Second close failed: %v", err)
	}

	if buf.String() != "hello proxy stream" {
		t.Fatalf("Expected 'hello proxy stream', got '%s'", buf.String())
	}

	savedContent, err := os.ReadFile(localFilePath)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	if string(savedContent) != "hello proxy stream" {
		t.Fatalf("Expected saved file 'hello proxy stream', got '%s'", string(savedContent))
	}
}

func TestCreateProxyStreamError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("webserver doesn't support hijacking")
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		bufrw.WriteString("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n")
		bufrw.Flush()

		time.Sleep(50 * time.Millisecond)

		bufrw.WriteString("5\r\nhe")
		bufrw.Flush()

		time.Sleep(50 * time.Millisecond)
	}))
	defer ts.Close()

	res, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	tempDir := t.TempDir()
	localFilePath := filepath.Join(tempDir, "fail.txt")

	inFlightMgr := core.NewInFlightManager()
	dl, _ := inFlightMgr.LockPath("fail")
	permit := make(chan struct{}, 1)
	permit <- struct{}{}

	stream := CreateProxyStream(res.Body, -1, localFilePath, inFlightMgr, "fail", dl, permit, func() {}, nil, nil, maxProxyArtifactSize, nil)

	var buf bytes.Buffer
	_, err = io.Copy(&buf, stream)

	if err == nil {
		t.Fatalf("Stream should have errored out")
	}

	stream.Close()

	if _, err := os.Stat(localFilePath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Final file should not exist on error")
	}
}

func TestCreateProxyStreamReportsCacheFileCreationError(t *testing.T) {
	blockedParent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedParent, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	stream := CreateProxyStream(
		io.NopCloser(strings.NewReader("artifact")),
		int64(len("artifact")),
		filepath.Join(blockedParent, "artifact.jar"),
		nil,
		"",
		nil,
		nil,
		nil,
		nil,
		nil,
		maxProxyArtifactSize,
		nil,
	)
	if _, err := io.Copy(io.Discard, stream); err != nil {
		t.Fatalf("upstream delivery should continue when only the cache write fails: %v", err)
	}
	if err := stream.Close(); err == nil {
		t.Fatal("cache file creation error was swallowed")
	}
}

func TestCreateProxyStreamRejectsOversizedUnknownResponse(t *testing.T) {
	localFilePath := filepath.Join(t.TempDir(), "oversized.jar")
	stream := CreateProxyStream(
		io.NopCloser(strings.NewReader("123456")),
		-1,
		localFilePath,
		nil,
		"",
		nil,
		nil,
		nil,
		nil,
		nil,
		5,
		nil,
	)

	data, err := io.ReadAll(stream)
	if !errors.Is(err, utils.ErrResponseTooLarge) {
		t.Fatalf("read error = %v, want %v", err, utils.ErrResponseTooLarge)
	}
	if string(data) != "12345" {
		t.Fatalf("data = %q, want bounded prefix", data)
	}
	_ = stream.Close()
	if _, err := os.Stat(localFilePath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("oversized response must not be cached, stat error = %v", err)
	}
}

func TestCreateProxyStreamStopsCachingWhenDiskIsLow(t *testing.T) {
	localFilePath := filepath.Join(t.TempDir(), "disk-full.jar")
	stream := CreateProxyStream(
		io.NopCloser(strings.NewReader("artifact")),
		int64(len("artifact")),
		localFilePath,
		nil,
		"",
		nil,
		nil,
		nil,
		nil,
		nil,
		maxProxyArtifactSize,
		func(uint64) bool { return false },
	)

	data, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("upstream delivery failed: %v", err)
	}
	if string(data) != "artifact" {
		t.Fatalf("data = %q", data)
	}
	if err := stream.Close(); !errors.Is(err, errInsufficientProxyDiskSpace) {
		t.Fatalf("close error = %v, want %v", err, errInsufficientProxyDiskSpace)
	}
	if _, err := os.Stat(localFilePath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("disk-constrained response must not be cached, stat error = %v", err)
	}
}

func TestProxyArtifactInvalidPath(t *testing.T) {
	state := core.NewAppState()
	repo := &config.Repository{
		Name:       "test-repo",
		Visibility: "public",
		Mirrors:    []config.Mirror{},
	}

	pathStr := "com/example/lib.jar"
	dl1, _ := state.Inner.InFlightDownloads.LockPath(pathStr)

	_, err := ProxyArtifact(state, repo, "com/example/lib.jar?param=value", "/tmp", pathStr, dl1)
	if !errors.Is(err, fiber.ErrBadRequest) {
		t.Fatalf("Expected BadRequest, got %v", err)
	}

	dl2, _ := state.Inner.InFlightDownloads.LockPath(pathStr)
	_, err = ProxyArtifact(state, repo, "com/example/lib.jar#fragment", "/tmp", pathStr, dl2)
	if !errors.Is(err, fiber.ErrBadRequest) {
		t.Fatalf("Expected BadRequest, got %v", err)
	}

	dl3, _ := state.Inner.InFlightDownloads.LockPath(pathStr)
	_, err = ProxyArtifact(state, repo, "com/example/lib.jar%20", "/tmp", pathStr, dl3)
	if !errors.Is(err, fiber.ErrBadRequest) {
		t.Fatalf("Expected BadRequest, got %v", err)
	}
}

func TestProxyArtifactRejectsOversizedDeclaredResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(maxProxyArtifactSize+1, 10))
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()
	repo := &config.Repository{Name: "releases", Mirrors: []config.Mirror{{URL: upstream.URL, TimeoutSecs: 5}}}
	storagePath := t.TempDir()
	path := "com/example/oversized.jar"
	pathStr := filepath.ToSlash(filepath.Join(storagePath, repo.Name, path))
	dl, _ := state.Inner.InFlightDownloads.LockPath(pathStr)

	stream, err := ProxyArtifact(state, repo, path, storagePath, pathStr, dl)
	if stream != nil {
		_ = stream.Close()
		t.Fatal("oversized response returned a stream")
	}
	if !errors.Is(err, fiber.ErrRequestEntityTooLarge) {
		t.Fatalf("error = %v, want %v", err, fiber.ErrRequestEntityTooLarge)
	}
	if _, loaded := state.Inner.InFlightDownloads.LockPath(pathStr); loaded {
		t.Fatal("oversized response left the path locked")
	}
}

func TestProxyMetadataRejectsOversizedDeclaredResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(maxProxyMetadataSize+1, 10))
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()
	repo := &config.Repository{Name: "snapshots", Mirrors: []config.Mirror{{URL: upstream.URL, TimeoutSecs: 5}}}
	storagePath := t.TempDir()
	path := "com/example/demo/1.0-SNAPSHOT/maven-metadata.xml"
	pathStr := filepath.ToSlash(filepath.Join(storagePath, repo.Name, path))
	dl, _ := state.Inner.InFlightDownloads.LockPath(pathStr)

	stream, err := ProxyArtifact(state, repo, path, storagePath, pathStr, dl)
	if stream != nil {
		_ = stream.Close()
		t.Fatal("oversized metadata response returned a stream")
	}
	if !errors.Is(err, fiber.ErrRequestEntityTooLarge) {
		t.Fatalf("error = %v, want %v", err, fiber.ErrRequestEntityTooLarge)
	}
}

func TestProxyArtifactUsesMirrorTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer upstream.Close()

	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()
	repo := &config.Repository{Name: "releases", Mirrors: []config.Mirror{{URL: upstream.URL, TimeoutSecs: 1}}}
	storagePath := t.TempDir()
	path := "com/example/timeout.jar"
	pathStr := filepath.ToSlash(filepath.Join(storagePath, repo.Name, path))
	dl, _ := state.Inner.InFlightDownloads.LockPath(pathStr)

	started := time.Now()
	_, _ = ProxyArtifact(state, repo, path, storagePath, pathStr, dl)
	if elapsed := time.Since(started); elapsed >= 1500*time.Millisecond {
		t.Fatalf("proxy ignored mirror timeout: elapsed %s", elapsed)
	}
}

func TestEscapeArtifactPathUnicode(t *testing.T) {
	got := escapeArtifactPath("中文/依赖 🚀.jar")
	want := "%E4%B8%AD%E6%96%87/%E4%BE%9D%E8%B5%96%20%F0%9F%9A%80.jar"
	if got != want {
		t.Fatalf("escapeArtifactPath() = %q, want %q", got, want)
	}
}

func TestEscapeArtifactPathPreservesPlusInVersion(t *testing.T) {
	got := escapeArtifactPath("com/example/mod/1.2.0+26.1/mod-1.2.0+26.1.jar")
	want := "com/example/mod/1.2.0+26.1/mod-1.2.0+26.1.jar"
	if got != want {
		t.Fatalf("escapeArtifactPath() = %q, want %q", got, want)
	}

	got = escapeArtifactPath("group/art with space/1.0.0+build.1/a b.jar")
	want = "group/art%20with%20space/1.0.0+build.1/a%20b.jar"
	if got != want {
		t.Fatalf("escapeArtifactPath() = %q, want %q", got, want)
	}
}

func TestProxyArtifactDoesNotPadOrCacheTruncatedSmallResponse(t *testing.T) {
	const declaredSize = 32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "32")
		_, _ = w.Write([]byte("short"))
	}))
	defer upstream.Close()

	tempDir := t.TempDir()
	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()
	repo := &config.Repository{
		Name: "releases",
		Mirrors: []config.Mirror{{
			URL: upstream.URL,
		}},
	}
	path := "com/example/demo.jar"
	localPath := filepath.Join(tempDir, repo.Name, filepath.FromSlash(path))
	pathStr := filepath.ToSlash(localPath)
	dl, loaded := state.Inner.InFlightDownloads.LockPath(pathStr)
	if loaded {
		t.Fatal("unexpected in-flight download")
	}

	stream, err := ProxyArtifact(state, repo, path, tempDir, pathStr, dl)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(stream)
	if readErr == nil {
		t.Fatal("expected truncated upstream response to return an error")
	}
	if closeErr := stream.Close(); closeErr != nil && !errors.Is(closeErr, readErr) {
		t.Fatalf("close error = %v, read error = %v", closeErr, readErr)
	}
	if string(data) != "short" {
		t.Fatalf("response = %q, want only bytes received from upstream", data)
	}
	if len(data) == declaredSize {
		t.Fatal("truncated response was padded to the declared content length")
	}
	if _, statErr := os.Stat(localPath); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("truncated response must not be cached, stat error = %v", statErr)
	}
}
