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
	"strings"
	"testing"
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
