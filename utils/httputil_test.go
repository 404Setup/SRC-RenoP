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
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

func TestReadAllLimitedOK(t *testing.T) {
	data, err := ReadAllLimited(strings.NewReader("hello"), 16)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q", data)
	}
}

func TestReadAllLimitedTooLarge(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 1<<20)
	data, err := ReadAllLimited(bytes.NewReader(payload), 64<<10)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got err=%v dataLen=%d", err, len(data))
	}
	if data != nil {
		t.Fatalf("expected nil data on overflow, got %d bytes", len(data))
	}
}

func TestDrainAndCloseDoesNotReadUnbounded(t *testing.T) {
	const size = 4 << 20
	payload := bytes.Repeat([]byte("z"), size)
	counted := &countReader{r: bytes.NewReader(payload)}
	DrainAndClose(counted)
	if counted.n > maxDrainForReuse {
		t.Fatalf("drained %d bytes, want <= %d", counted.n, maxDrainForReuse)
	}
}

func TestDiscardHTTPBodySkipsLargeContentLength(t *testing.T) {
	const size = 4 << 20
	payload := bytes.Repeat([]byte("z"), size)
	counted := &countReader{r: bytes.NewReader(payload)}
	DiscardHTTPBody(counted, int64(size))
	if counted.n != 0 {
		t.Fatalf("large CL must not be read, read %d", counted.n)
	}
	if !counted.closed {
		t.Fatal("expected Close")
	}
}

func TestDiscardHTTPBodyDrainsSmallContentLength(t *testing.T) {
	payload := []byte("hello-small-body")
	counted := &countReader{r: bytes.NewReader(payload)}
	DiscardHTTPBody(counted, int64(len(payload)))
	if counted.n != int64(len(payload)) {
		t.Fatalf("drained %d, want %d", counted.n, len(payload))
	}
}

func TestNewFastHTTPBodyBuffered(t *testing.T) {
	resp := fasthttp.AcquireResponse()
	resp.SetBodyString("hello-body")
	body := NewFastHTTPBody(resp)
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello-body" {
		t.Fatalf("got %q", data)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFastHTTPContentLength(t *testing.T) {
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	resp.Header.SetContentLength(42)
	if got := FastHTTPContentLength(resp); got != 42 {
		t.Fatalf("got %d", got)
	}
	resp.Header.SetContentLength(-1)
	if got := FastHTTPContentLength(resp); got != -1 {
		t.Fatalf("chunked got %d", got)
	}
}

func TestAbortAfterLargeContentLengthDoesNotSpikeHeap(t *testing.T) {
	const size = 40 << 20
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		_, _ = conn.Read(buf)
		hdr := "HTTP/1.1 200 OK\r\nContent-Length: " + strconv.Itoa(size) + "\r\nConnection: close\r\n\r\n"
		_, _ = conn.Write([]byte(hdr))
		_, _ = conn.Write(bytes.Repeat([]byte("A"), 64*1024))
		time.Sleep(2 * time.Second)
	}()

	client := &fasthttp.Client{
		Name:                "leak-repro",
		ReadTimeout:         500 * time.Millisecond,
		WriteTimeout:        500 * time.Millisecond,
		MaxConnsPerHost:     2,
		StreamResponseBody:  true,
		MaxResponseBodySize: 0,
	}
	defer client.CloseIdleConnections()

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	req.SetRequestURI("http://" + ln.Addr().String() + "/")
	req.Header.SetMethod(fasthttp.MethodGet)
	_ = client.DoTimeout(req, resp, time.Second)
	fasthttp.ReleaseRequest(req)
	AbortFastHTTPResponse(resp)

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	const maxGrowth = 8 << 20
	var growth uint64
	if after.HeapAlloc > before.HeapAlloc {
		growth = after.HeapAlloc - before.HeapAlloc
	}
	if growth > maxGrowth {
		t.Fatalf("HeapAlloc grew by %d after aborting large-CL client failure (limit %d)", growth, maxGrowth)
	}
}

func TestStreamClientWithPreBufferDoesNotPreallocFullCL(t *testing.T) {
	const size = 40 << 20
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(size))
		w.WriteHeader(http.StatusOK)
		chunk := bytes.Repeat([]byte("B"), 64*1024)
		for left := size; left > 0; {
			n := min(len(chunk), left)
			_, _ = w.Write(chunk[:n])
			left -= n
		}
	}))
	t.Cleanup(ts.Close)

	client := &fasthttp.Client{
		Name:                "stream-prebuffer",
		ReadTimeout:         5 * time.Second,
		WriteTimeout:        5 * time.Second,
		MaxConnsPerHost:     2,
		StreamResponseBody:  true,
		MaxResponseBodySize: FastHTTPStreamPreBuffer,
	}
	defer client.CloseIdleConnections()

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	req.SetRequestURI(ts.URL)
	req.Header.SetMethod(fasthttp.MethodGet)
	err := client.DoTimeout(req, resp, 5*time.Second)
	fasthttp.ReleaseRequest(req)
	if err != nil {
		AbortFastHTTPResponse(resp)
		t.Fatalf("DoTimeout: %v", err)
	}
	AbortFastHTTPResponse(resp)

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	const maxGrowth = 8 << 20
	var growth uint64
	if after.HeapAlloc > before.HeapAlloc {
		growth = after.HeapAlloc - before.HeapAlloc
	}
	if growth > maxGrowth {
		t.Fatalf("HeapAlloc grew by %d with Stream+prebuffer on large CL (limit %d)", growth, maxGrowth)
	}
}

func TestConfigureFastHTTPStreamClient(t *testing.T) {
	c := &fasthttp.Client{}
	ConfigureFastHTTPStreamClient(c)
	if !c.StreamResponseBody {
		t.Fatal("expected StreamResponseBody")
	}
	if c.MaxResponseBodySize != FastHTTPStreamPreBuffer {
		t.Fatalf("MaxResponseBodySize=%d, want %d", c.MaxResponseBodySize, FastHTTPStreamPreBuffer)
	}
}

type countReader struct {
	r      io.Reader
	n      int64
	closed bool
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func (c *countReader) Close() error {
	c.closed = true
	if closer, ok := c.r.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
