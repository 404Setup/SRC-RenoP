/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core_test

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"testing"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/shirou/gopsutil/v3/process"

	"renop/internal/core"
)

const (
	cmpHardMaxMB   = 16
	cmpKeyCount    = 2000
	cmpValueSize   = 4 * 1024
	cmpFillEntries = 3000
)

type memSnap struct {
	HeapAlloc uint64
	HeapInuse uint64
	Sys       uint64
	RSS       uint64
	VSS       uint64
}

func snapMem() memSnap {
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(20 * time.Millisecond)
	runtime.GC()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s := memSnap{
		HeapAlloc: m.HeapAlloc,
		HeapInuse: m.HeapInuse,
		Sys:       m.Sys,
	}
	if p, err := process.NewProcess(int32(os.Getpid())); err == nil {
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			s.RSS = mi.RSS
			s.VSS = mi.VMS
		}
	}
	return s
}

func delta(a, b memSnap) memSnap {
	return memSnap{
		HeapAlloc: b.HeapAlloc - a.HeapAlloc,
		HeapInuse: b.HeapInuse - a.HeapInuse,
		Sys:       b.Sys - a.Sys,
		RSS:       b.RSS - a.RSS,
		VSS:       b.VSS - a.VSS,
	}
}

func fmtMB(b uint64) string {
	return fmt.Sprintf("%.2f MiB", float64(b)/1024/1024)
}

func fmtDeltaMB(b uint64) string {
	const signBit = uint64(1) << 63
	if b > signBit {
		neg := int64(b)
		return fmt.Sprintf("%.2f MiB", float64(neg)/1024/1024)
	}
	if b > 1<<40 {
		return "~0 (noise)"
	}
	return fmtMB(b)
}

func newBigcacheLikeProd(hardMaxMB int) (*bigcache.BigCache, error) {
	cfg := bigcache.DefaultConfig(time.Hour)
	cfg.Shards = 4
	cfg.MaxEntriesInWindow = 32
	cfg.MaxEntrySize = 1024
	cfg.HardMaxCacheSize = hardMaxMB
	cfg.CleanWindow = 5 * time.Minute
	cfg.Verbose = false
	return bigcache.New(context.Background(), cfg)
}

func newBigcacheLegacy(hardMaxMB int) (*bigcache.BigCache, error) {
	cfg := bigcache.DefaultConfig(time.Hour)
	cfg.Shards = 8
	cfg.MaxEntriesInWindow = 256
	cfg.MaxEntrySize = 32 * 1024
	cfg.HardMaxCacheSize = hardMaxMB
	cfg.CleanWindow = 5 * time.Minute
	cfg.Verbose = false
	return bigcache.New(context.Background(), cfg)
}

func payload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

func keys(n int) []string {
	out := make([]string, n)
	for i := range n {
		out[i] = "storage/releases/com/example/artifact/maven-metadata-" + strconv.Itoa(i) + ".xml"
	}
	return out
}

func report(t *testing.T, name string, empty, filled memSnap, dEmpty, dFilled memSnap) {
	t.Helper()
	t.Logf("\n=== %s ===", name)
	t.Logf("  empty:  HeapAlloc=%s HeapInuse=%s Sys=%s RSS=%s VSS=%s",
		fmtMB(empty.HeapAlloc), fmtMB(empty.HeapInuse), fmtMB(empty.Sys), fmtMB(empty.RSS), fmtMB(empty.VSS))
	t.Logf("  +empty: HeapAlloc=%s HeapInuse=%s Sys=%s RSS=%s VSS=%s",
		fmtDeltaMB(dEmpty.HeapAlloc), fmtDeltaMB(dEmpty.HeapInuse), fmtDeltaMB(dEmpty.Sys),
		fmtDeltaMB(dEmpty.RSS), fmtDeltaMB(dEmpty.VSS))
	t.Logf("  filled: HeapAlloc=%s HeapInuse=%s Sys=%s RSS=%s VSS=%s",
		fmtMB(filled.HeapAlloc), fmtMB(filled.HeapInuse), fmtMB(filled.Sys), fmtMB(filled.RSS), fmtMB(filled.VSS))
	t.Logf("  +fill:  HeapAlloc=%s HeapInuse=%s Sys=%s RSS=%s VSS=%s",
		fmtDeltaMB(dFilled.HeapAlloc), fmtDeltaMB(dFilled.HeapInuse), fmtDeltaMB(dFilled.Sys),
		fmtDeltaMB(dFilled.RSS), fmtDeltaMB(dFilled.VSS))
}

// TestFileCacheVsBigcacheMemory compares idle construction cost and post-fill heap/process memory.
// Run: go test ./core -run TestFileCacheVsBigcacheMemory -v -count=1
func TestFileCacheVsBigcacheMemory(t *testing.T) {
	ks := keys(cmpFillEntries)
	val := payload(cmpValueSize)

	type kind string
	const (
		kindOurs   kind = "FileByteCache"
		kindLean   kind = "bigcache-lean"
		kindLegacy kind = "bigcache-legacy"
	)

	run := func(name kind) {
		runtime.GC()
		debug.FreeOSMemory()
		time.Sleep(50 * time.Millisecond)

		base := snapMem()

		var hold any
		switch name {
		case kindOurs:
			c := core.NewFileByteCache(cmpHardMaxMB << 20)
			hold = c
			_ = hold
			empty := snapMem()
			for i := range cmpFillEntries {
				_ = c.Set(ks[i], val)
			}
			for i := range 100 {
				_, _ = c.Get(ks[i%cmpKeyCount])
			}
			filled := snapMem()
			report(t, string(name), empty, filled, delta(base, empty), delta(empty, filled))
			runtime.KeepAlive(c)

		case kindLean:
			c, err := newBigcacheLikeProd(cmpHardMaxMB)
			if err != nil {
				t.Fatal(err)
			}
			empty := snapMem()
			for i := range cmpFillEntries {
				_ = c.Set(ks[i], val)
			}
			for i := range 100 {
				_, _ = c.Get(ks[i%cmpKeyCount])
			}
			filled := snapMem()
			report(t, string(name), empty, filled, delta(base, empty), delta(empty, filled))
			_ = c.Close()
			runtime.KeepAlive(c)

		case kindLegacy:
			c, err := newBigcacheLegacy(cmpHardMaxMB)
			if err != nil {
				t.Fatal(err)
			}
			empty := snapMem()
			for i := range cmpFillEntries {
				_ = c.Set(ks[i], val)
			}
			for i := range 100 {
				_, _ = c.Get(ks[i%cmpKeyCount])
			}
			filled := snapMem()
			report(t, string(name), empty, filled, delta(base, empty), delta(empty, filled))
			_ = c.Close()
			runtime.KeepAlive(c)
		}
	}

	for _, k := range []kind{kindOurs, kindLean, kindLegacy} {
		run(k)
	}

	t.Logf("\nScenario: hardMax=%dMiB value=%dB fillSets=%d (eviction expected)",
		cmpHardMaxMB, cmpValueSize, cmpFillEntries)
	t.Logf("Notes: RSS/VSS deltas on Windows are noisy after FreeOSMemory; prefer HeapAlloc/HeapInuse for cache cost.")
	t.Logf("bigcache Get also copies on read; FileByteCache Get copies on read — both include that cost in fill+touch.")
}

func BenchmarkCache_Set(b *testing.B) {
	val := payload(cmpValueSize)
	ks := keys(cmpKeyCount)

	b.Run("FileByteCache", func(b *testing.B) {
		c := core.NewFileByteCache(cmpHardMaxMB << 20)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = c.Set(ks[i%cmpKeyCount], val)
		}
	})

	b.Run("bigcache-lean", func(b *testing.B) {
		c, err := newBigcacheLikeProd(cmpHardMaxMB)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { _ = c.Close() })
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = c.Set(ks[i%cmpKeyCount], val)
		}
	})

	b.Run("bigcache-legacy", func(b *testing.B) {
		c, err := newBigcacheLegacy(cmpHardMaxMB)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { _ = c.Close() })
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = c.Set(ks[i%cmpKeyCount], val)
		}
	})
}

func BenchmarkCache_Get(b *testing.B) {
	val := payload(cmpValueSize)
	ks := keys(cmpKeyCount)

	b.Run("FileByteCache", func(b *testing.B) {
		c := core.NewFileByteCache(cmpHardMaxMB << 20)
		for i := range cmpKeyCount {
			_ = c.Set(ks[i], val)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = c.Get(ks[i%cmpKeyCount])
		}
	})

	b.Run("bigcache-lean", func(b *testing.B) {
		c, err := newBigcacheLikeProd(cmpHardMaxMB)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { _ = c.Close() })
		for i := range cmpKeyCount {
			_ = c.Set(ks[i], val)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = c.Get(ks[i%cmpKeyCount])
		}
	})

	b.Run("bigcache-legacy", func(b *testing.B) {
		c, err := newBigcacheLegacy(cmpHardMaxMB)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { _ = c.Close() })
		for i := range cmpKeyCount {
			_ = c.Set(ks[i], val)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = c.Get(ks[i%cmpKeyCount])
		}
	})
}

func BenchmarkCache_GetParallel(b *testing.B) {
	val := payload(cmpValueSize)
	ks := keys(cmpKeyCount)

	b.Run("FileByteCache", func(b *testing.B) {
		c := core.NewFileByteCache(cmpHardMaxMB << 20)
		for i := range cmpKeyCount {
			_ = c.Set(ks[i], val)
		}
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				_, _ = c.Get(ks[i%cmpKeyCount])
				i++
			}
		})
	})

	b.Run("bigcache-lean", func(b *testing.B) {
		c, err := newBigcacheLikeProd(cmpHardMaxMB)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { _ = c.Close() })
		for i := range cmpKeyCount {
			_ = c.Set(ks[i], val)
		}
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				_, _ = c.Get(ks[i%cmpKeyCount])
				i++
			}
		})
	})
}

func BenchmarkCache_SetParallel(b *testing.B) {
	val := payload(cmpValueSize)
	ks := keys(cmpKeyCount)

	b.Run("FileByteCache", func(b *testing.B) {
		c := core.NewFileByteCache(cmpHardMaxMB << 20)
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				_ = c.Set(ks[i%cmpKeyCount], val)
				i++
			}
		})
	})

	b.Run("bigcache-lean", func(b *testing.B) {
		c, err := newBigcacheLikeProd(cmpHardMaxMB)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { _ = c.Close() })
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				_ = c.Set(ks[i%cmpKeyCount], val)
				i++
			}
		})
	})
}
