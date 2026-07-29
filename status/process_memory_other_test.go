//go:build !windows

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
	"testing"
)

func TestParseProcKBField(t *testing.T) {
	status := []byte("Name:\trenop\nVmRSS:\t   48256 kB\nVmSwap:\t       0 kB\nVmSize:\t 1282048 kB\n")
	if got := parseProcKBField(status, "VmRSS"); got != 48256*1024 {
		t.Fatalf("VmRSS: got %d want %d", got, 48256*1024)
	}
	if got := parseProcKBField(status, "VmSize"); got != 1282048*1024 {
		t.Fatalf("VmSize: got %d want %d", got, 1282048*1024)
	}
	if got := parseProcKBField(status, "VmSwap"); got != 0 {
		t.Fatalf("VmSwap: got %d want 0", got)
	}
	if got := parseProcKBField(status, "Missing"); got != 0 {
		t.Fatalf("Missing: got %d want 0", got)
	}

	rollup := []byte("Rss:                 48000 kB\nPss:                 45000 kB\nPrivate_Clean:        1200 kB\nPrivate_Dirty:       40000 kB\nSwap:                   64 kB\n")
	priv := parseProcKBField(rollup, "Private_Dirty") + parseProcKBField(rollup, "Private_Clean")
	sw := parseProcKBField(rollup, "Swap")
	if priv != (40000+1200)*1024 {
		t.Fatalf("private: got %d", priv)
	}
	if sw != 64*1024 {
		t.Fatalf("swap: got %d", sw)
	}
	vmSize := parseProcKBField(status, "VmSize")
	vss := priv + sw
	if vss >= vmSize {
		t.Fatalf("expected private commit VSS %d << VmSize %d", vss, vmSize)
	}
	if vss > 50*1024*1024 {
		t.Fatalf("sample VSS unexpectedly large: %d", vss)
	}
}

func TestSanitizeVirtualSize(t *testing.T) {
	rss := uint64(40 << 20)
	goVMS := uint64(1280 << 20)
	if got := sanitizeVirtualSize(rss, goVMS); got != rss {
		t.Fatalf("inflated VMS: got %d want RSS %d", got, rss)
	}
	modest := rss + 32<<20
	if got := sanitizeVirtualSize(rss, modest); got != modest {
		t.Fatalf("modest VMS: got %d want %d", got, modest)
	}
	if got := sanitizeVirtualSize(rss, rss/2); got != rss {
		t.Fatalf("VMS < RSS should clamp to RSS: got %d", got)
	}
	if got := sanitizeVirtualSize(0, goVMS); got != goVMS {
		t.Fatalf("rss=0 should keep VMS: got %d", got)
	}
}

func TestProcessMemoryBytesNotInflatedVA(t *testing.T) {
	rss, vss := processMemoryBytes()
	if rss == 0 && vss == 0 {
		t.Fatal("processMemoryBytes returned zeros")
	}
	t.Logf("RSS=%.2f MiB VSS=%.2f MiB", float64(rss)/(1024*1024), float64(vss)/(1024*1024))
	if vss > 512<<20 && vss > rss*3 {
		t.Fatalf("VSS looks like inflated virtual address size: RSS=%d VSS=%d", rss, vss)
	}
	if vss < rss {
		t.Fatalf("VSS (%d) < RSS (%d)", vss, rss)
	}
}
