/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package api

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/service/gpg"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func GetGPGSignature(c fiber.Ctx, state *core.AppState) error {
	repository := c.Params("repo_name")
	artifactPath := strings.TrimPrefix(c.Params("*"), "/")
	if !gpg.IsProtectedArtifact(artifactPath) {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	user := auth.GetUser(c)
	pathParam := artifactPath
	if _, err := ResolveAndCheckPath(state, user, repository, &pathParam); err != nil {
		if errors.Is(err, fiber.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	signature, err := db.GetGPGSignature(gpg.ArtifactKey(repository, artifactPath))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load GPG signature")
	}
	if signature == nil {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}
	return protohttp.Write(c, pb.FromGPGSignature(signature))
}
