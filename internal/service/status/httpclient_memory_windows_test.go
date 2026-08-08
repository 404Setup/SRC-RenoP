//go:build windows

/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package status

import (
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"renop/internal/utils"
)

func TestRepeatedHTTPSRequestsHaveBoundedProcessMemory(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(ts.Close)

	testTransport := ts.Client().Transport.(*http.Transport)
	transport := utils.DefaultTransport.Clone()
	transport.TLSClientConfig = testTransport.TLSClientConfig.Clone()
	client := &http.Client{Transport: transport}
	t.Cleanup(client.CloseIdleConnections)

	runtime.GC()
	beforeRSS, beforePrivate := processMemoryBytes()
	for range 5 {
		resp, err := client.Get(ts.URL)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			_ = resp.Body.Close()
			t.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}
	client.CloseIdleConnections()
	runtime.GC()
	afterRSS, afterPrivate := processMemoryBytes()

	const maxGrowth = 32 << 20
	rssGrowth := positiveDelta(afterRSS, beforeRSS)
	privateGrowth := positiveDelta(afterPrivate, beforePrivate)
	t.Logf("private working set: %d -> %d (delta %d); private commit: %d -> %d (delta %d)",
		beforeRSS, afterRSS, rssGrowth, beforePrivate, afterPrivate, privateGrowth)
	if rssGrowth > maxGrowth {
		t.Fatalf("private working set grew by %d bytes (limit %d)", rssGrowth, maxGrowth)
	}
	if privateGrowth > maxGrowth {
		t.Fatalf("private commit grew by %d bytes (limit %d)", privateGrowth, maxGrowth)
	}
}

func positiveDelta(after, before uint64) uint64 {
	if after > before {
		return after - before
	}
	return 0
}
