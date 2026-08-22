/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
)

func (h Handler) serveConfig(c fiber.Ctx, state *core.AppState, repo *config.Repository) error {
	host := strings.TrimSpace(c.Host())
	if host == "" || strings.ContainsAny(host, "\r\n") {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	repoName := url.PathEscape(c.Params("repo_name"))
	base := publicProtocol(c, state) + "://" + host + "/" + repoName
	payload := RegistryConfig{
		DownloadURL: base + "/api/v1/crates",
		APIURL:      base,
		AuthNeeded:  repo != nil && strings.EqualFold(repo.Visibility, "PRIVATE"),
	}
	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	c.Set(fiber.HeaderCacheControl, "private, max-age=60")
	if c.Method() == fiber.MethodHead {
		return c.SendStatus(fiber.StatusOK)
	}
	return c.JSON(payload)
}

func publicProtocol(c fiber.Ctx, state *core.AppState) string {
	if c.Secure() {
		return "https"
	}
	if state != nil && state.Inner != nil {
		if cfg := state.Inner.Config.Load(); cfg != nil && cfg.Server.IsTrustedProxy(c.IP()) {
			if strings.EqualFold(strings.TrimSpace(c.Get(fiber.HeaderXForwardedProto)), "https") {
				return "https"
			}
		}
	}
	return "http"
}

// SendAuthChallenge returns the sparse-registry authentication response Cargo
// expects before retrying config.json with its credential provider.
func SendAuthChallenge(c fiber.Ctx) error {
	c.Set(fiber.HeaderWWWAuthenticate, "Cargo")
	return errorResponse(c, fiber.StatusUnauthorized, "Cargo registry authentication is required")
}
