/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package frontend embeds and serves the RenoP single-page application.
package frontend

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/utils"
)

const frontendIndexCacheControl = "no-store, no-cache, must-revalidate, max-age=0"

func setFrontendIndexCacheHeaders(c fiber.Ctx) {
	c.Set(fiber.HeaderCacheControl, frontendIndexCacheControl)
	c.Set(fiber.HeaderPragma, "no-cache")
	c.Set(fiber.HeaderExpires, "0")
}

func ServeIndex(c fiber.Ctx, state *core.AppState) error {
	html := GenerateIndexHTML(state)

	hasher := sha256.New()
	hasher.Write(html)
	etag := `W/"` + hex.EncodeToString(hasher.Sum(nil))[:16] + `"`

	c.Set(fiber.HeaderContentType, "text/html; charset=utf-8")
	c.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src * data: blob:;")
	setFrontendIndexCacheHeaders(c)
	c.Set(fiber.HeaderETag, etag)

	if clientETag := c.Get(fiber.HeaderIfNoneMatch); clientETag != "" && clientETag == etag {
		return c.SendStatus(fiber.StatusNotModified)
	}

	return c.Send(html)
}

func ServeAsset(c fiber.Ctx) error {
	path := c.Params("*")
	sanitized, ok := utils.SanitizePath(path)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad path")
	}
	return ServeEmbeddedFile(c, "assets/"+sanitized)
}

func ServeSvg(c fiber.Ctx) error {
	path := c.Params("*")
	sanitized, ok := utils.SanitizePath(path)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad path")
	}
	return ServeEmbeddedFile(c, "svg/"+sanitized)
}

func ServeJs(c fiber.Ctx) error {
	path := c.Params("*")
	sanitized, ok := utils.SanitizePath(path)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad path")
	}
	return ServeEmbeddedFile(c, "js/"+sanitized)
}

// ServeCSS serves an embedded stylesheet with immutable-cache headers.
func ServeCSS(c fiber.Ctx) error {
	path := c.Params("*")
	sanitized, ok := utils.SanitizePath(path)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad path")
	}
	return ServeEmbeddedFile(c, "css/"+sanitized)
}
