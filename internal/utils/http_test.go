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
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"

	"renop/internal/config"
)

func TestReadRequestBodyLimitedPreservesBoundedStream(t *testing.T) {
	payload := []byte(`{"name":"renop"}`)
	fastCtx := &fasthttp.RequestCtx{}
	fastCtx.Request.SetBodyStream(bytes.NewReader(payload), -1)
	app := fiber.New()
	ctx := app.AcquireCtx(fastCtx)
	defer app.ReleaseCtx(ctx)

	body, err := ReadRequestBodyLimited(ctx, int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, payload) || !bytes.Equal(ctx.Request().Body(), payload) {
		t.Fatal("bounded stream was not preserved for decoder fallback")
	}
}

type parseRangeTestCase struct {
	rangeStr  string
	fileSize  uint64
	wantStart uint64
	wantEnd   uint64
	wantOk    bool
}

type normalizeForwardedIPTestCase struct {
	in   string
	want string
}

func TestParseRange(t *testing.T) {
	tests := []parseRangeTestCase{
		{rangeStr: "bytes=0-499", fileSize: 1000, wantStart: 0, wantEnd: 499, wantOk: true},
		{rangeStr: "bytes=500-999", fileSize: 1000, wantStart: 500, wantEnd: 999, wantOk: true},
		{rangeStr: "bytes=-500", fileSize: 1000, wantStart: 500, wantEnd: 999, wantOk: true},
		{rangeStr: "bytes=500-", fileSize: 1000, wantStart: 500, wantEnd: 999, wantOk: true},
		{rangeStr: "bytes=0-0", fileSize: 1000, wantStart: 0, wantEnd: 0, wantOk: true},
		{rangeStr: "bytes=-1", fileSize: 1000, wantStart: 999, wantEnd: 999, wantOk: true},
		{rangeStr: "bytes=500-1500", fileSize: 1000, wantStart: 500, wantEnd: 999, wantOk: true},
		{rangeStr: "bytes=1500-", fileSize: 1000, wantStart: 0, wantEnd: 0, wantOk: false},
		{rangeStr: "bytes=500-400", fileSize: 1000, wantStart: 0, wantEnd: 0, wantOk: false},
		{rangeStr: "items=0-500", fileSize: 1000, wantStart: 0, wantEnd: 0, wantOk: false},
		{rangeStr: "bytes=a-b", fileSize: 1000, wantStart: 0, wantEnd: 0, wantOk: false},
		{rangeStr: "bytes=0-", fileSize: 0, wantStart: 0, wantEnd: 0, wantOk: false},
		{rangeStr: "bytes=-1", fileSize: 0, wantStart: 0, wantEnd: 0, wantOk: false},
	}

	for _, tt := range tests {
		start, end, ok := ParseRange(tt.rangeStr, tt.fileSize)
		if start != tt.wantStart || end != tt.wantEnd || ok != tt.wantOk {
			t.Errorf("ParseRange(%q, %d) = %d, %d, %v; want %d, %d, %v",
				tt.rangeStr, tt.fileSize, start, end, ok, tt.wantStart, tt.wantEnd, tt.wantOk)
		}
	}
}

func TestExtractIPDoesNotTrustForwardedHeaderWithoutProxyAllowlist(t *testing.T) {
	app := fiber.New()
	server := config.DefaultServerConfig()
	server.CdnIPHeader = "X-Forwarded-For"
	server.TrustedProxies = nil
	server.ParseTrustedProxies()
	app.Get("/", func(c fiber.Ctx) error {
		if got := ExtractIP(c, &server); got != c.IP() {
			t.Fatalf("ExtractIP trusted an untrusted forwarded header: got %q, peer %q", got, c.IP())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

func TestExtractIPUsesCdnHeaderFromTrustedProxy(t *testing.T) {
	app := fiber.New()
	server := config.DefaultServerConfig()
	server.CdnIPHeader = "CF-Connecting-IP"
	server.TrustedProxies = []string{"0.0.0.0"}
	server.ParseTrustedProxies()

	var got string
	app.Get("/", func(c fiber.Ctx) error {
		got = ExtractIP(c, &server)
		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("CF-Connecting-IP", "203.0.113.50")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if got != "203.0.113.50" {
		t.Fatalf("ExtractIP = %q, want client IP from CF-Connecting-IP", got)
	}
}

func TestExtractIPXForwardedForRightToLeftSkipsTrustedHops(t *testing.T) {
	app := fiber.New()
	server := config.DefaultServerConfig()
	server.CdnIPHeader = "X-Forwarded-For"
	server.TrustedProxies = []string{"0.0.0.0", "10.0.0.1"}
	server.ParseTrustedProxies()

	var got string
	app.Get("/", func(c fiber.Ctx) error {
		got = ExtractIP(c, &server)
		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.1, 203.0.113.50, 10.0.0.1")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if got != "203.0.113.50" {
		t.Fatalf("ExtractIP = %q, want rightmost non-trusted hop", got)
	}
}

func TestExtractIPIgnoresInvalidHeaderValues(t *testing.T) {
	app := fiber.New()
	server := config.DefaultServerConfig()
	server.CdnIPHeader = "CF-Connecting-IP"
	server.TrustedProxies = []string{"0.0.0.0"}
	server.ParseTrustedProxies()

	var got, peer string
	app.Get("/", func(c fiber.Ctx) error {
		peer = c.IP()
		got = ExtractIP(c, &server)
		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("CF-Connecting-IP", "not-an-ip")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if got != peer {
		t.Fatalf("ExtractIP = %q, want peer %q on invalid header", got, peer)
	}
}

func TestNormalizeForwardedIP(t *testing.T) {
	tests := []normalizeForwardedIPTestCase{
		{in: "  203.0.113.1  ", want: "203.0.113.1"},
		{in: `"203.0.113.1"`, want: "203.0.113.1"},
		{in: "203.0.113.1:8080", want: "203.0.113.1"},
		{in: "[2001:db8::1]", want: "2001:db8::1"},
		{in: "[2001:db8::1]:443", want: "2001:db8::1"},
		{in: "", want: ""},
	}
	for _, tt := range tests {
		if got := normalizeForwardedIP(tt.in); got != tt.want {
			t.Errorf("normalizeForwardedIP(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
