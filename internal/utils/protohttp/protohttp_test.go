/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package protohttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
	"google.golang.org/protobuf/proto"

	"renop/pkg/pb"
)

func TestReadDecodesBoundedStreamingBody(t *testing.T) {
	body, err := proto.Marshal(&pb.LoginRequest{Name: "admin", Secret: "secret"})
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New(fiber.Config{StreamRequestBody: true})
	app.Post("/", func(c fiber.Ctx) error {
		var req pb.LoginRequest
		if err := Read(c, &req); err != nil {
			return err
		}
		if req.GetName() != "admin" || req.GetSecret() != "secret" {
			t.Fatalf("unexpected request: %v", &req)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusNoContent)
	}
}

func TestReadRejectsOversizedBody(t *testing.T) {
	body := bytes.Repeat([]byte{'x'}, MaxRequestBodySize+1)
	fastCtx := &fasthttp.RequestCtx{}
	fastCtx.Request.SetBodyStream(bytes.NewReader(body), -1)
	app := fiber.New()
	ctx := app.AcquireCtx(fastCtx)
	defer app.ReleaseCtx(ctx)

	var req pb.LoginRequest
	if err := Read(ctx, &req); err != fiber.ErrRequestEntityTooLarge {
		t.Fatalf("Read error = %v, want %v", err, fiber.ErrRequestEntityTooLarge)
	}
}
