//go:build linux

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

import "testing"

func TestParseProcStatm(t *testing.T) {
	const pageSize = 4096
	rss, vss, ok := parseProcStatm([]byte("1000 200 50 10 0 100 0\n"), pageSize)
	if !ok {
		t.Fatal("expected valid statm sample")
	}
	if want := uint64(200 * pageSize); rss != want {
		t.Fatalf("rss: got %d want %d", rss, want)
	}
	if want := uint64(1000 * pageSize); vss != want {
		t.Fatalf("vss: got %d want %d", vss, want)
	}
}

func TestParseProcStatmSanitizesReservedAddressSpace(t *testing.T) {
	const pageSize = 4096
	rss, vss, ok := parseProcStatm([]byte("1048576 10240 0 0 0 0 0\n"), pageSize)
	if !ok {
		t.Fatal("expected valid statm sample")
	}
	if rss != 40<<20 {
		t.Fatalf("rss: got %d want %d", rss, uint64(40<<20))
	}
	if vss != rss {
		t.Fatalf("reserved VSS should fall back to RSS: rss=%d vss=%d", rss, vss)
	}
}

func TestParseProcStatmRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		pageSize uint64
	}{
		{name: "empty"},
		{name: "missing rss", data: "100\n", pageSize: 4096},
		{name: "invalid vms", data: "x 100\n", pageSize: 4096},
		{name: "invalid rss", data: "100 x\n", pageSize: 4096},
		{name: "zero page size", data: "100 50\n"},
		{name: "zero sample", data: "0 0\n", pageSize: 4096},
		{name: "number overflow", data: "18446744073709551616 1\n", pageSize: 1},
		{name: "byte overflow", data: "18446744073709551615 1\n", pageSize: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, ok := parseProcStatm([]byte(tt.data), tt.pageSize); ok {
				t.Fatal("expected invalid statm sample")
			}
		})
	}

	if _, _, ok := parseProcStatm([]byte("1 18446744073709551615\n"), 2); ok {
		t.Fatal("expected RSS byte overflow")
	}
}

func BenchmarkParseProcStatm(b *testing.B) {
	data := []byte("1048576 10240 2048 512 0 4096 0\n")
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = parseProcStatm(data, 4096)
	}
}
