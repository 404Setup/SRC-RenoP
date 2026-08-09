/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
)

const (
	corsAllowMethods = "GET,HEAD,POST,PUT,PATCH,DELETE,OPTIONS"
	corsAllowHeaders = "Accept,Authorization,Content-Type,Origin,X-Requested-With"
	corsMaxAge       = "86400"
)

func isCorsMethodAllowed(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case fiber.MethodGet, fiber.MethodHead, fiber.MethodPost, fiber.MethodPut,
		fiber.MethodPatch, fiber.MethodDelete, fiber.MethodOptions:
		return true
	default:
		return false
	}
}

func areCorsHeadersAllowed(headers string) bool {
	for header := range strings.SplitSeq(headers, ",") {
		switch strings.ToLower(strings.TrimSpace(header)) {
		case "", "accept", "authorization", "content-type", "origin", "x-requested-with":
		default:
			return false
		}
	}
	return true
}

// CorsMiddleware enforces browser CORS using server.domains ∪ server.cors_origins.
// Credentials are always allowed when an Origin matches so session cookies work
// for approved cross-origin UIs. Disallowed origins receive no CORS headers.
func CorsMiddleware(state *core.AppState) fiber.Handler {
	return func(c fiber.Ctx) error {
		origin := strings.TrimSpace(c.Get(fiber.HeaderOrigin))
		if origin == "" {
			return c.Next()
		}

		cfg := state.Inner.Config.Load()
		if cfg == nil || !cfg.Server.IsOriginAllowed(origin) {
			if c.Method() == fiber.MethodOptions {
				return c.SendStatus(fiber.StatusForbidden)
			}
			return c.Next()
		}

		c.Set(fiber.HeaderAccessControlAllowOrigin, origin)
		c.Set(fiber.HeaderAccessControlAllowCredentials, "true")
		c.Vary(fiber.HeaderOrigin)

		if c.Method() == fiber.MethodOptions {
			reqHeaders := strings.TrimSpace(c.Get(fiber.HeaderAccessControlRequestHeaders))
			reqMethod := strings.TrimSpace(c.Get(fiber.HeaderAccessControlRequestMethod))
			if (reqMethod != "" && !isCorsMethodAllowed(reqMethod)) ||
				(reqHeaders != "" && !areCorsHeadersAllowed(reqHeaders)) {
				return c.SendStatus(fiber.StatusForbidden)
			}
			c.Set(fiber.HeaderAccessControlAllowHeaders, corsAllowHeaders)
			c.Set(fiber.HeaderAccessControlAllowMethods, corsAllowMethods)
			c.Set(fiber.HeaderAccessControlMaxAge, corsMaxAge)
			return c.SendStatus(fiber.StatusNoContent)
		}

		c.Set(fiber.HeaderAccessControlAllowHeaders, corsAllowHeaders)
		c.Set(fiber.HeaderAccessControlAllowMethods, corsAllowMethods)
		return c.Next()
	}
}
