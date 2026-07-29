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
	"errors"
	"io"
	"net/http"
)

// ErrResponseTooLarge is returned when an HTTP response body exceeds the
// caller-imposed size limit (defense against unbounded reads / memory spikes).
var ErrResponseTooLarge = errors.New("HTTP response body exceeds size limit")

// maxDrainForReuse matches net/http's internal body-slurp budget for keep-alive
// reuse. We discard at most this much on early close so multi-megabyte payloads
// are never pulled fully into process memory while allowing connection reuse.
const maxDrainForReuse = 256 << 10

// DrainAndClose discards up to 256 KiB of body then closes it.
// Always call this (or Close after a full intentional read) on every non-nil
// body to enable net/http connection reuse without unbounded memory consumption.
func DrainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxDrainForReuse))
	_ = body.Close()
}

// DiscardHTTPBody closes an HTTP response body without pulling large payloads
// into process memory.
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

// CloseHTTPResponse closes a net/http Response body cleanly.
func CloseHTTPResponse(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	DrainAndClose(resp.Body)
	resp.Body = http.NoBody
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
