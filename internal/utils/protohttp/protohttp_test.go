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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
	"google.golang.org/protobuf/encoding/protojson"
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

func TestJSONAndProtobufNegotiation(t *testing.T) {
	want := &pb.SessionDto{PublicId: "public-session", CreatedAt: 1 << 54}
	binary, err := proto.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	jsonBody := []byte(`{"public_id":"public-session","created_at":"18014398509481984"}`)
	cases := []struct {
		name, contentType, accept, responseType string
		body                                    []byte
	}{
		{"legacy", "", "", ContentType, binary},
		{"json default", "application/json; charset=utf-8", "", ContentType, jsonBody},
		{"json round trip", "application/json; charset=utf-8", "application/json", fiber.MIMEApplicationJSON, jsonBody},
		{"json response", ContentType, "application/json", fiber.MIMEApplicationJSON, binary},
		{"binary response", "application/json", ContentType, ContentType, jsonBody},
		{"binary alias", "application/protobuf", "application/protobuf", "application/protobuf", binary},
		{"quality", ContentType, "application/x-protobuf;q=0.2, application/json;q=0.8", fiber.MIMEApplicationJSON, binary},
		{"json excluded", ContentType, "application/json;q=0, */*;q=1", ContentType, binary},
		{"unknown accept retains default", ContentType, "text/html", ContentType, binary},
	}
	app := fiber.New(fiber.Config{StreamRequestBody: true})
	app.Post("/", func(c fiber.Ctx) error {
		var request pb.SessionDto
		if err := Read(c, &request); err != nil {
			return err
		}
		c.Set(fiber.HeaderVary, "Origin")
		return WriteStatus(c, fiber.StatusCreated, &request)
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(tc.body))
			if tc.contentType != "" {
				request.Header.Set(fiber.HeaderContentType, tc.contentType)
			}
			request.Header.Set(fiber.HeaderAccept, tc.accept)
			response, err := app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != fiber.StatusCreated || response.Header.Get(fiber.HeaderContentType) != tc.responseType {
				t.Fatalf("response = %d %s %s", response.StatusCode, response.Header.Get(fiber.HeaderContentType), body)
			}
			for _, header := range []string{"Origin", fiber.HeaderAccept} {
				if !strings.Contains(response.Header.Get(fiber.HeaderVary), header) {
					t.Errorf("Vary does not contain %s", header)
				}
			}
			var got pb.SessionDto
			if tc.responseType == fiber.MIMEApplicationJSON {
				err = protojson.Unmarshal(body, &got)
				if !bytes.Contains(body, []byte(`"public_id"`)) || !bytes.Contains(body, []byte(`"18014398509481984"`)) {
					t.Errorf("JSON did not preserve field names and integer precision: %s", body)
				}
			} else {
				err = proto.Unmarshal(body, &got)
			}
			if err != nil || !proto.Equal(want, &got) {
				t.Fatalf("decoded response differs: %v, %v", &got, err)
			}
		})
	}
}

func TestJSONRejectsInvalidAndOversizedRequests(t *testing.T) {
	app := fiber.New()
	for _, body := range []string{
		`{"name":"admin","unknown":true}`, `{"name":"admin","name":"other"}`,
		`{"name":true}`, `null`, `{"name":"admin"} {}`, `{"name":"` + strings.Repeat("x", MaxRequestBodySize) + `"}`,
	} {
		fastCtx := &fasthttp.RequestCtx{}
		fastCtx.Request.Header.SetContentType(fiber.MIMEApplicationJSON)
		fastCtx.Request.SetBodyStream(strings.NewReader(body), -1)
		ctx := app.AcquireCtx(fastCtx)
		var request pb.LoginRequest
		err := Read(ctx, &request)
		app.ReleaseCtx(ctx)
		if err == nil {
			t.Fatal("invalid JSON request was accepted")
		}
		if len(body) > MaxRequestBodySize && err != fiber.ErrRequestEntityTooLarge {
			t.Fatalf("oversized JSON error = %v", err)
		}
	}
}
