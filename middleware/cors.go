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

	"renop/config"
	"renop/core"
)

const (
	corsAllowMethods = "GET,HEAD,POST,PUT,PATCH,DELETE,OPTIONS"
	corsAllowHeaders = "Accept,Authorization,Content-Type,Origin,X-Requested-With"
	corsMaxAge       = "86400"
)

// CorsMiddleware enforces browser CORS using server.domains / server.cors_origins.
// Credentials are always allowed when an Origin matches so session cookies work
// for approved cross-origin UIs. Disallowed origins receive no CORS headers.
func CorsMiddleware(state *core.AppState) fiber.Handler {
	return func(c fiber.Ctx) error {
		origin := strings.TrimSpace(c.Get("Origin"))
		if origin == "" {
			return c.Next()
		}

		cfg := state.Inner.Config.Load().(*config.Config)
		if !cfg.Server.IsOriginAllowed(origin) {
			if c.Method() == fiber.MethodOptions {
				return c.SendStatus(fiber.StatusForbidden)
			}
			return c.Next()
		}

		c.Set("Access-Control-Allow-Origin", origin)
		c.Set("Access-Control-Allow-Credentials", "true")
		c.Set("Vary", "Origin")

		if c.Method() == fiber.MethodOptions {
			reqHeaders := strings.TrimSpace(c.Get("Access-Control-Request-Headers"))
			if reqHeaders != "" {
				c.Set("Access-Control-Allow-Headers", reqHeaders)
			} else {
				c.Set("Access-Control-Allow-Headers", corsAllowHeaders)
			}
			reqMethod := strings.TrimSpace(c.Get("Access-Control-Request-Method"))
			if reqMethod != "" {
				c.Set("Access-Control-Allow-Methods", reqMethod)
			} else {
				c.Set("Access-Control-Allow-Methods", corsAllowMethods)
			}
			c.Set("Access-Control-Max-Age", corsMaxAge)
			return c.SendStatus(fiber.StatusNoContent)
		}

		c.Set("Access-Control-Allow-Headers", corsAllowHeaders)
		c.Set("Access-Control-Allow-Methods", corsAllowMethods)
		return c.Next()
	}
}
