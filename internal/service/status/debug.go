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
	"fmt"
	"io"
	"runtime"
	"runtime/pprof"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/service/auth"
	"renop/pkg/pb"
	"renop/internal/service/token"
	"renop/internal/utils/protohttp"
)

// debugModeActive is latched at process start from server.debug_mode.
// Toggling the setting only takes effect after a full restart.
var debugModeActive atomic.Bool

// InitDebugMode latches whether debug dump APIs are available for this process.
func InitDebugMode(enabled bool) {
	debugModeActive.Store(enabled)
}

// DebugModeActive reports whether this process was started with debug mode on.
func DebugModeActive() bool {
	return debugModeActive.Load()
}

// SetupDebugRoutes registers manager-only memory profile endpoints.
// When debug mode is inactive, handlers still respond (403/404) so clients can detect the state.
func SetupDebugRoutes(api fiber.Router) {
	dbg := api.Group("/debug")
	dbg.Get("/memory/heap", dumpHeapProfile)
	dbg.Get("/memory/allocs", dumpAllocsProfile)
	dbg.Get("/memory/goroutine", dumpGoroutineProfile)
	dbg.Get("/memory/runtime", dumpRuntimeMemory)
}

func requireDebugManager(c fiber.Ctx) error {
	if !token.RequireManager(auth.GetUser(c)) {
		user := auth.GetUser(c)
		if user == nil || user.Username == "guest" {
			return fiber.NewError(fiber.StatusUnauthorized, "Unauthorized")
		}
		return fiber.NewError(fiber.StatusForbidden, "Forbidden")
	}
	if !DebugModeActive() {
		return fiber.NewError(fiber.StatusForbidden, "Debug mode is not active (enable server.debug_mode and restart)")
	}
	return nil
}

func dumpHeapProfile(c fiber.Ctx) error {
	if err := requireDebugManager(c); err != nil {
		return err
	}
	if c.Query("gc", "1") != "0" {
		runtime.GC()
	}
	return writePprof(c, "heap", "renop-heap.pprof", 0)
}

func dumpAllocsProfile(c fiber.Ctx) error {
	if err := requireDebugManager(c); err != nil {
		return err
	}
	return writePprof(c, "allocs", "renop-allocs.pprof", 0)
}

func dumpGoroutineProfile(c fiber.Ctx) error {
	if err := requireDebugManager(c); err != nil {
		return err
	}
	return writePprof(c, "goroutine", "renop-goroutine.pprof", 0)
}

func dumpRuntimeMemory(c fiber.Ctx) error {
	if err := requireDebugManager(c); err != nil {
		return err
	}
	if c.Query("gc", "0") == "1" {
		runtime.GC()
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	rss, vss := processMemoryBytes()

	goFromOS := m.Sys
	if m.Sys > m.HeapReleased {
		goFromOS = m.Sys - m.HeapReleased
	}
	offHeapRuntime := uint64(0)
	if m.Sys > m.HeapInuse {
		offHeapRuntime = m.Sys - m.HeapInuse
	}

	c.Set(fiber.HeaderCacheControl, "no-store")
	return protohttp.Write(c, &pb.RuntimeMemoryBreakdown{
		Note: "heap/allocs pprof flames only cover heap objects (≈ heap_inuse). " +
			"stack_* and *_sys / other_sys have no allocation call stacks in standard pprof. " +
			"process_rss may exceed go_retained: binary text, cgo, mmap, and kernel accounting.",
		ProcessRss:             rss,
		ProcessVss:             vss,
		GoRetained:             goFromOS,
		HeapInuse:              m.HeapInuse,
		HeapAlloc:              m.HeapAlloc,
		HeapSys:                m.HeapSys,
		HeapIdle:               m.HeapIdle,
		HeapReleased:           m.HeapReleased,
		HeapObjects:            m.HeapObjects,
		StackInuse:             m.StackInuse,
		StackSys:               m.StackSys,
		MspanInuse:             m.MSpanInuse,
		MspanSys:               m.MSpanSys,
		McacheInuse:            m.MCacheInuse,
		McacheSys:              m.MCacheSys,
		BuckHashSys:            m.BuckHashSys,
		GcSys:                  m.GCSys,
		OtherSys:               m.OtherSys,
		Sys:                    m.Sys,
		OffHeapRuntimeEstimate: offHeapRuntime,
		RssMinusGoRetained:     int64(rss) - int64(goFromOS),
		NumGoroutine:           uint64(runtime.NumGoroutine()),
		NumCpu:                 int32(runtime.NumCPU()),
	})
}

func writePprof(c fiber.Ctx, name, filename string, debug int) error {
	p := pprof.Lookup(name)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "profile not found: "+name)
	}
	pr, pw := io.Pipe()
	go func() {
		err := p.WriteTo(pw, debug)
		_ = pw.CloseWithError(err)
	}()
	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Cache-Control", "no-store")
	c.Set("X-Renop-Profile", name)
	c.Set("X-Renop-Profile-Time", strconv.FormatInt(time.Now().Unix(), 10))
	return c.SendStream(pr)
}
