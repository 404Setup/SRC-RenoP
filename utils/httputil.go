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
	"sync"

	"github.com/valyala/fasthttp"
)

// ErrResponseTooLarge is returned when an HTTP response body exceeds the
// caller-imposed size limit (defense against unbounded reads / memory spikes).
var ErrResponseTooLarge = errors.New("HTTP response body exceeds size limit")

// maxDrainForReuse matches net/http's internal body-slurp budget for keep-alive
// reuse. We only discard this much on error paths so a multi-megabyte error
// page or misdirected binary is never pulled fully into process memory.
const maxDrainForReuse = 256 << 10

// FastHTTPBodyPoolLimit is the max capacity a fasthttp request/response body
// buffer may keep when returned to the global pool. Larger buffers are dropped
// for GC instead of permanently inflating process Alloc after failed clients.
const FastHTTPBodyPoolLimit = 256 << 10

// FastHTTPStreamPreBuffer is the recommended MaxResponseBodySize when
// StreamResponseBody is true. fasthttp only truly streams fixed Content-Length
// bodies larger than this threshold; with Max=0 it still pre-allocates the full
// Content-Length before any bytes are read (the client-failure leak).
const FastHTTPStreamPreBuffer = 64 << 10

func init() {
	fasthttp.SetBodySizePoolLimit(FastHTTPBodyPoolLimit, FastHTTPBodyPoolLimit)
}

// DrainAndClose discards a bounded prefix of body then closes it.
// Always call this (or Close after a full intentional read) on every non-nil
// body. Unbounded Close-only on a huge unread body can still leave socket/TLS
// buffers and prevents clean connection reuse.
func DrainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxDrainForReuse))
	_ = body.Close()
}

// DiscardHTTPBody closes an HTTP response body without pulling a large payload
// into the process.
func DiscardHTTPBody(body io.ReadCloser, contentLength int64) {
	if body == nil {
		return
	}
	if contentLength < 0 || contentLength > maxDrainForReuse {
		_ = body.Close()
		return
	}
	DrainAndClose(body)
}

// ReadAllLimited reads r up to max bytes. If more data is available it returns
// ErrResponseTooLarge without keeping the excess (only max+1 bytes are buffered).
func ReadAllLimited(r io.Reader, max int64) ([]byte, error) {
	if max < 0 {
		max = 0
	}
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, ErrResponseTooLarge
	}
	return data, nil
}

// FastHTTPBody owns a fasthttp.Response and exposes its body as io.ReadCloser.
// Close always closes any body stream and releases the response to the pool.
//
// Callers must not ReleaseResponse themselves after wrapping.
type FastHTTPBody struct {
	resp *fasthttp.Response
	r    io.Reader
	once sync.Once
	err  error
}

// NewFastHTTPBody wraps resp. If the body is streamed, reads come from the
// stream; otherwise a private copy of the buffered body is used so the response
// can be released on Close without invalidating the data.
func NewFastHTTPBody(resp *fasthttp.Response) *FastHTTPBody {
	if resp == nil {
		return &FastHTTPBody{r: bytes.NewReader(nil)}
	}
	if stream := resp.BodyStream(); stream != nil {
		return &FastHTTPBody{resp: resp, r: stream}
	}
	data := append([]byte(nil), resp.Body()...)
	dropFastHTTPResponseBody(resp)
	fasthttp.ReleaseResponse(resp)
	return &FastHTTPBody{r: bytes.NewReader(data)}
}

func (b *FastHTTPBody) Read(p []byte) (int, error) {
	if b.r == nil {
		return 0, io.EOF
	}
	return b.r.Read(p)
}

func (b *FastHTTPBody) Close() error {
	b.once.Do(func() {
		if b.resp != nil {
			b.err = b.resp.CloseBodyStream()
			dropFastHTTPResponseBody(b.resp)
			fasthttp.ReleaseResponse(b.resp)
			b.resp = nil
		}
		b.r = nil
	})
	return b.err
}

// AbortFastHTTPResponse closes any body stream without draining and releases
// the response. Prefer this on error / non-success paths when the body is unwanted.
func AbortFastHTTPResponse(resp *fasthttp.Response) {
	if resp == nil {
		return
	}
	_ = resp.CloseBodyStream()
	dropFastHTTPResponseBody(resp)
	fasthttp.ReleaseResponse(resp)
}

// dropFastHTTPResponseBody discards the response body buffer so it is never
// put back into responseBodyPool. Safe when body is nil or already empty.
func dropFastHTTPResponseBody(resp *fasthttp.Response) {
	if resp == nil {
		return
	}
	resp.ReleaseBody(0)
}

// FastHTTPContentLength returns Content-Length as int64, or -1 when unknown
// (chunked / identity / missing).
func FastHTTPContentLength(resp *fasthttp.Response) int64 {
	if resp == nil {
		return -1
	}
	cl := resp.Header.ContentLength()
	if cl < 0 {
		return -1
	}
	return int64(cl)
}

// ConfigureFastHTTPStreamClient sets the safe streaming pair for outbound
// clients that may receive large bodies (mirrors, downloads). Without a
// positive MaxResponseBodySize, StreamResponseBody still fully buffers fixed
// Content-Length responses.
func ConfigureFastHTTPStreamClient(c *fasthttp.Client) {
	if c == nil {
		return
	}
	if c.MaxResponseBodySize <= 0 {
		c.MaxResponseBodySize = FastHTTPStreamPreBuffer
	}
	c.StreamResponseBody = true
}
