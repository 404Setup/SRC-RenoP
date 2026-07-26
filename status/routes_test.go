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

	"renop/core"
	"renop/pb"
)

func TestGetInstanceStatusIncludesUpdateState(t *testing.T) {
	app := fiber.New()
	state := core.NewAppState()

	app.Get("/api/status/instance", func(c fiber.Ctx) error {
		return GetInstanceStatus(c, state)
	})

	req := httptest.NewRequest("GET", "/api/status/instance", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var statusResp pb.InstanceStatus
	if err := proto.Unmarshal(body, &statusResp); err != nil {
		t.Fatalf("failed to unmarshal protobuf response: %v", err)
	}

	if statusResp.GetUpdateState() == nil {
		t.Fatalf("expected update_state to be present in response, got nil")
	}
	if statusResp.GetUpdateState().GetStatus() != "idle" {
		t.Fatalf("expected default update state status 'idle', got '%s'", statusResp.GetUpdateState().GetStatus())
	}
}
