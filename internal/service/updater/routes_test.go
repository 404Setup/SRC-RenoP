/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package updater

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"google.golang.org/protobuf/proto"

	"renop/pkg/pb"
	"renop/internal/utils/protohttp"
)

func TestGetUpdaterStatusProtobuf(t *testing.T) {
	updateStateFields(func(s *UpdateState) {
		*s = UpdateState{Status: "idle"}
	})

	app := fiber.New()
	app.Get("/api/updater/status", func(c fiber.Ctx) error {
		return protohttp.Write(c, ToPbUpdateState(GetUpdateState()))
	})

	req := httptest.NewRequest("GET", "/api/updater/status", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != protohttp.ContentType {
		t.Fatalf("expected Content-Type %q, got %q", protohttp.ContentType, ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var got pb.UpdateState
	if err := proto.Unmarshal(body, &got); err != nil {
		t.Fatalf("failed to unmarshal protobuf response: %v", err)
	}
	if got.GetStatus() != "idle" {
		t.Fatalf("expected status 'idle', got %q", got.GetStatus())
	}
}

func TestToPbUpdateState(t *testing.T) {
	if ToPbUpdateState(nil) != nil {
		t.Fatalf("expected nil for nil input")
	}
	src := &UpdateState{
		Status:             "available",
		LatestVersion:      "1.2.3",
		DownloadUrl:        "https://example.com/pkg.zip",
		Progress:           42,
		ErrorMessage:       "",
		Size:               100,
		EstimatedDiskSpace: 300,
		ReleaseDate:        "2026-01-01",
		ReleaseNotes:       "notes",
		CommitSha:          "abcdef1",
		IsRelease:          true,
	}
	got := ToPbUpdateState(src)
	if got.GetStatus() != src.Status || got.GetLatestVersion() != src.LatestVersion ||
		got.GetDownloadUrl() != src.DownloadUrl || got.GetProgress() != int32(src.Progress) ||
		got.GetSize() != src.Size || got.GetEstimatedDiskSpace() != src.EstimatedDiskSpace ||
		got.GetReleaseDate() != src.ReleaseDate || got.GetReleaseNotes() != src.ReleaseNotes ||
		got.GetCommitSha() != src.CommitSha || got.GetIsRelease() != src.IsRelease {
		t.Fatalf("ToPbUpdateState mismatch: %+v vs source %+v", got, src)
	}
}
