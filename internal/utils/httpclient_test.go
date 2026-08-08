/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package utils

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultTransportSettings(t *testing.T) {
	tr := DefaultTransport
	if tr.ForceAttemptHTTP2 {
		t.Fatal("HTTP/2 should be disabled for artifact proxy stability")
	}
	if !tr.DisableCompression {
		t.Fatal("automatic response decompression adds avoidable per-request buffers")
	}
	if tr.DisableKeepAlives {
		t.Fatal("keep-alives must remain enabled to avoid repeated TLS allocation spikes")
	}
	if tr.MaxIdleConns != 16 || tr.MaxIdleConnsPerHost != 4 || tr.MaxConnsPerHost != 64 {
		t.Fatalf("unexpected connection bounds: idle=%d per_host=%d max_per_host=%d",
			tr.MaxIdleConns, tr.MaxIdleConnsPerHost, tr.MaxConnsPerHost)
	}
	if tr.MaxResponseHeaderBytes != 256<<10 {
		t.Fatalf("MaxResponseHeaderBytes=%d", tr.MaxResponseHeaderBytes)
	}
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("minimum TLS version=%x", tr.TLSClientConfig.MinVersion)
	}
}

func TestCloseHTTPResponseAllowsReuse(t *testing.T) {
	var hits atomic.Int32
	var connections atomic.Int32
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Length", "2")
		_, _ = w.Write([]byte("ok"))
	}))
	ts.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	ts.Start()
	t.Cleanup(ts.Close)

	client := OutboundClient(5 * time.Second)

	for range 3 {
		req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "ok" {
			t.Fatalf("body %q", body)
		}
		CloseHTTPResponse(resp)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits=%d", hits.Load())
	}
	if connections.Load() != 1 {
		t.Fatalf("connections=%d; sequential requests should reuse one connection", connections.Load())
	}
}

func TestSmallGETsSteadyStateHeap(t *testing.T) {
	payload := []byte(`{"v":1}`)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	t.Cleanup(ts.Close)

	client := OutboundClient(5 * time.Second)

	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	CloseHTTPResponse(resp)

	runtime.GC()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	for range 30 {
		resp, err := client.Get(ts.URL)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != len(payload) {
			t.Fatalf("len=%d", len(data))
		}
		CloseHTTPResponse(resp)
	}

	runtime.GC()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	var growth uint64
	if after.HeapAlloc > before.HeapAlloc {
		growth = after.HeapAlloc - before.HeapAlloc
	}
	const maxGrowth = 512 << 10
	if growth > maxGrowth {
		t.Fatalf("HeapAlloc grew by %d after 30 small GETs (limit %d)", growth, maxGrowth)
	}
}

func TestDiagnoseOutboundHTTPSMemory(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "5")
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(ts.Close)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	client := ts.Client()
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	runtime.GC()
	runtime.ReadMemStats(&after)

	t.Logf("Before: HeapAlloc=%d Sys=%d HeapSys=%d StackSys=%d MCacheSys=%d MSpanSys=%d OtherSys=%d",
		before.HeapAlloc, before.Sys, before.HeapSys, before.StackSys, before.MCacheSys, before.MSpanSys, before.OtherSys)
	t.Logf("After:  HeapAlloc=%d Sys=%d HeapSys=%d StackSys=%d MCacheSys=%d MSpanSys=%d OtherSys=%d",
		after.HeapAlloc, after.Sys, after.HeapSys, after.StackSys, after.MCacheSys, after.MSpanSys, after.OtherSys)
	t.Logf("Delta:  HeapAlloc=%d Sys=%d",
		int64(after.HeapAlloc)-int64(before.HeapAlloc), int64(after.Sys)-int64(before.Sys))
}
