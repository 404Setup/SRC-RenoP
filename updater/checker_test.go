/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package updater

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func largeBodyHandler(size int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(size))
		w.WriteHeader(http.StatusOK)
		chunk := bytes.Repeat([]byte("A"), 64*1024)
		left := size
		for left > 0 {
			n := min(len(chunk), left)
			_, _ = w.Write(chunk[:n])
			left -= n
		}
	})
}

func TestDoGitHubJSONRejectsOversizedContentLength(t *testing.T) {
	const size = 40 << 20
	ts := httptest.NewServer(largeBodyHandler(size))
	t.Cleanup(ts.Close)

	var dst map[string]any
	_, err := doGitHubJSON(context.Background(), ts.URL, &dst)
	if err == nil {
		t.Fatal("expected error for oversized Content-Length")
	}
	if !strings.Contains(err.Error(), "too large") && !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size-limit error, got: %v", err)
	}
}

func TestDoGitHubJSONRejectsOversizedChunkedBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		chunk := bytes.Repeat([]byte("x"), 64*1024)
		for i := 0; i < (maxGitHubAPIBody/len(chunk))+4; i++ {
			_, _ = w.Write(chunk)
		}
	}))
	t.Cleanup(ts.Close)

	var dst map[string]any
	_, err := doGitHubJSON(context.Background(), ts.URL, &dst)
	if err == nil {
		t.Fatal("expected error for oversized body")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected exceeds error, got: %v", err)
	}
}

func TestDoGitHubJSONOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"tag_name":"v1.2.3","name":"rel"}`)
	}))
	t.Cleanup(ts.Close)

	var rel GithubReleaseResponse
	status, err := doGitHubJSON(context.Background(), ts.URL, &rel)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if status != 200 {
		t.Fatalf("status=%d", status)
	}
	if rel.TagName != "v1.2.3" {
		t.Fatalf("tag=%q", rel.TagName)
	}
}

func TestDoGitHubJSONNonOKAbortsLargeBody(t *testing.T) {
	const size = 8 << 20
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(size))
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(bytes.Repeat([]byte("e"), size))
	}))
	t.Cleanup(ts.Close)

	var dst map[string]any
	status, err := doGitHubJSON(context.Background(), ts.URL, &dst)
	if err == nil {
		t.Fatal("expected error")
	}
	if status != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", status)
	}
}

func TestDoGitHubJSONNonOKSmallBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"not found"}`)
	}))
	t.Cleanup(ts.Close)

	var dst map[string]any
	status, err := doGitHubJSON(context.Background(), ts.URL, &dst)
	if err == nil {
		t.Fatal("expected error")
	}
	if status != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", status)
	}
}

func TestDoGitHubJSONOversizedDoesNotSpikeHeap(t *testing.T) {
	const size = 40 << 20
	ts := httptest.NewServer(largeBodyHandler(size))
	t.Cleanup(ts.Close)

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	var dst map[string]any
	_, err := doGitHubJSON(context.Background(), ts.URL, &dst)
	if err == nil {
		t.Fatal("expected oversized error")
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	const maxGrowth = 8 << 20
	var growth uint64
	if after.HeapAlloc > before.HeapAlloc {
		growth = after.HeapAlloc - before.HeapAlloc
	}
	if growth > maxGrowth {
		t.Fatalf("HeapAlloc grew by %d bytes after rejecting 40MiB body (limit %d)", growth, maxGrowth)
	}
}

func TestDoGitHubJSONClientFailureDoesNotSpikeHeap(t *testing.T) {
	const size = 40 << 20
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(size))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte("x"), 64*1024))
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, err := hj.Hijack()
			if err == nil {
				_ = conn.Close()
			}
		}
	}))
	t.Cleanup(ts.Close)

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	var dst map[string]any
	_, _ = doGitHubJSON(context.Background(), ts.URL, &dst)

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	const maxGrowth = 8 << 20
	var growth uint64
	if after.HeapAlloc > before.HeapAlloc {
		growth = after.HeapAlloc - before.HeapAlloc
	}
	if growth > maxGrowth {
		t.Fatalf("HeapAlloc grew by %d after client failure on large CL (limit %d)", growth, maxGrowth)
	}
}

func TestClipStringDoesNotRetainFullBacking(t *testing.T) {
	big := strings.Repeat("n", 4<<20)
	clipped := clipString(big, 64)
	if len(clipped) != 64 {
		t.Fatalf("len=%d", len(clipped))
	}
	big = ""
	runtime.GC()
	if clipped != strings.Repeat("n", 64) {
		t.Fatal("clip content mismatch")
	}
}
