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
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"google.golang.org/protobuf/proto"

	"renop/config"
	"renop/pb"
	"renop/utils/protohttp"
)

func TestDebugModeLatch(t *testing.T) {
	InitDebugMode(false)
	if DebugModeActive() {
		t.Fatal("expected inactive")
	}
	InitDebugMode(true)
	if !DebugModeActive() {
		t.Fatal("expected active")
	}
	InitDebugMode(false)
}

func TestDumpHeapRequiresDebugMode(t *testing.T) {
	InitDebugMode(false)
	t.Cleanup(func() { InitDebugMode(false) })

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		// Manager user so we hit the debug-mode gate, not auth.
		c.Locals("user", &config.User{Username: "admin", Roles: []string{"admin"}})
		return c.Next()
	})
	SetupDebugRoutes(app.Group("/api"))

	req := httptest.NewRequest("GET", "/api/debug/memory/heap", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status=%d want 403", resp.StatusCode)
	}
}

func TestDumpHeapWhenDebugActive(t *testing.T) {
	InitDebugMode(true)
	t.Cleanup(func() { InitDebugMode(false) })

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", &config.User{Username: "admin", Roles: []string{"admin"}})
		return c.Next()
	})
	SetupDebugRoutes(app.Group("/api"))

	req := httptest.NewRequest("GET", "/api/debug/memory/heap?gc=0", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 32 {
		t.Fatalf("heap profile too small: %d bytes", len(body))
	}
	cd := resp.Header.Get("Content-Disposition")
	if cd == "" || !contains(cd, "renop-heap.pprof") {
		t.Fatalf("Content-Disposition=%q", cd)
	}
}

func TestDumpRuntimeMemoryWhenDebugActive(t *testing.T) {
	InitDebugMode(true)
	t.Cleanup(func() { InitDebugMode(false) })

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", &config.User{Username: "admin", Roles: []string{"admin"}})
		return c.Next()
	})
	SetupDebugRoutes(app.Group("/api"))

	req := httptest.NewRequest("GET", "/api/debug/memory/runtime", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != protohttp.ContentType {
		t.Fatalf("Content-Type=%q want %q", ct, protohttp.ContentType)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var brk pb.RuntimeMemoryBreakdown
	if err := proto.Unmarshal(body, &brk); err != nil {
		t.Fatalf("unmarshal protobuf: %v", err)
	}
	if brk.GetHeapInuse() == 0 && brk.GetSys() == 0 {
		t.Fatal("expected non-zero runtime memory fields")
	}
	if brk.GetNote() == "" {
		t.Fatal("expected note field")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && (func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})()))
}
