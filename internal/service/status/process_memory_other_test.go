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
	"runtime"
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

	rollup := []byte("Rss:                 48000 kB\nPss:                 45000 kB\nPrivate_Clean:        1200 kB\nPrivate_Dirty:       40000 kB\nAnonymous:           41000 kB\nSwap:                   64 kB\n")
	priv := parseProcKBField(rollup, "Private_Dirty") + parseProcKBField(rollup, "Private_Clean") + parseProcKBField(rollup, "Swap")
	if priv != (40000+1200+64)*1024 {
		t.Fatalf("private: got %d", priv)
	}
	vmSize := parseProcKBField(status, "VmSize")
	if priv >= vmSize {
		t.Fatalf("expected private commit %d << VmSize %d", priv, vmSize)
	}
}

func TestSanitizeVirtualSize(t *testing.T) {
	rss := uint64(40 << 20)
	goVMS := uint64(1280 << 20) // ~1.25 GiB — classic Go reserved VA
	if got := sanitizeVirtualSize(rss, goVMS); got != 0 {
		t.Fatalf("inflated VMS: got %d want 0 (drop)", got)
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

func TestPrivateOrRSS(t *testing.T) {
	rss := uint64(40 << 20)
	if got := privateOrRSS(rss, rss-1024); got != rss {
		t.Fatalf("private < rss: got %d want rss", got)
	}
	if got := privateOrRSS(rss, rss+10<<20); got != rss+10<<20 {
		t.Fatalf("private > rss: got %d", got)
	}
}

func TestGoRuntimeRetainedBytesNonZero(t *testing.T) {
	if got := goRuntimeRetainedBytes(); got == 0 {
		t.Fatal("expected non-zero Go retained bytes")
	}
}

func TestProcessMemoryBytesNotInflatedVA(t *testing.T) {
	rss, vss := processMemoryBytes()
	if rss == 0 && vss == 0 {
		t.Fatal("processMemoryBytes returned zeros")
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	t.Logf("RSS=%.2f MiB VSS=%.2f MiB HeapInuse=%.2f MiB Sys-Released=%.2f MiB",
		float64(rss)/(1024*1024), float64(vss)/(1024*1024),
		float64(m.HeapInuse)/(1024*1024),
		float64(goRuntimeRetainedBytes())/(1024*1024))
	// Must not be multi‑GiB reserved VA.
	if vss > 512<<20 && vss > rss*3 && vss > goRuntimeRetainedBytes()*2 {
		t.Fatalf("VSS looks like inflated virtual address size: RSS=%d VSS=%d", rss, vss)
	}
	if vss < rss {
		t.Fatalf("VSS (%d) < RSS (%d)", vss, rss)
	}
	// Live Go heap is a lower bound for process RSS in any non-trivial process
	// that has run GC; allow small test binaries where HeapInuse may be tiny.
	if m.HeapInuse > 0 && rss > 0 && rss*4 < m.HeapInuse {
		t.Fatalf("RSS (%d) absurdly below HeapInuse (%d) — wrong field?", rss, m.HeapInuse)
	}
}

func TestProcessMemoryRSSIsNotGoSys(t *testing.T) {
	// Success-path RSS must come from VmRSS / gopsutil RSS, not from mixing
	// MemStats.Sys into the pair (that made both series track Sys and look
	// nothing like pprof HeapInuse).
	rss, vss := processMemoryBytes()
	if rss == 0 {
		t.Fatal("expected non-zero RSS")
	}
	// VSS is private-commit floored at RSS — equal is OK for pure-Go processes.
	if vss < rss {
		t.Fatalf("vss %d < rss %d", vss, rss)
	}
}
