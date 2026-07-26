/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"github.com/gofiber/fiber/v3"

	"renop/config"
	"renop/core"
	"renop/index"
	"renop/javadocs"
	"renop/pb"
	"renop/utils/protohttp"
)

func RebuildIndex(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	var payload pb.RebuildIndexRequest
	if err := protohttp.Read(c, &payload); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}

	cfg := state.Inner.Config.Load().(*config.Config)
	storagePath := cfg.StoragePath

	switch payload.Mode {
	case "full":
		index.RebuildIndexAsync(storagePath, state.Inner.FileIndex)
		_ = javadocs.ClearAllJavadocCaches()
		return c.Status(fiber.StatusOK).SendString("")
	case "diff":
		index.RebuildIndexDiff(storagePath, state.Inner.FileIndex)
		return c.Status(fiber.StatusOK).SendString("")
	default:
		return c.Status(fiber.StatusBadRequest).SendString("Invalid mode. Expected 'full' or 'diff'")
	}
}
