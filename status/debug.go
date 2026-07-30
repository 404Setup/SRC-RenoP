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
	"bytes"
	"fmt"
	"runtime"
	"runtime/pprof"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/auth"
	"renop/token"
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

func writePprof(c fiber.Ctx, name, filename string, debug int) error {
	p := pprof.Lookup(name)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "profile not found: "+name)
	}
	var buf bytes.Buffer
	if err := p.WriteTo(&buf, debug); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Cache-Control", "no-store")
	c.Set("X-Renop-Profile", name)
	c.Set("X-Renop-Profile-Time", strconv.FormatInt(time.Now().Unix(), 10))
	return c.Send(buf.Bytes())
}
