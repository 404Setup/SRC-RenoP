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
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
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

	stream := CreateProxyStream(res.Body, -1, localFilePath, inFlightMgr, "test", dl, permit, func() {}, nil, nil)

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

	stream := CreateProxyStream(res.Body, -1, localFilePath, inFlightMgr, "fail", dl, permit, func() {}, nil, nil)

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
