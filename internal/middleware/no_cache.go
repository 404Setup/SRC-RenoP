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
)

const apiNoCacheControl = "no-store, no-cache, must-revalidate, private, max-age=0"

func isAPIPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/")
}

func setAPINoCacheHeaders(c fiber.Ctx) {
	c.Set(fiber.HeaderCacheControl, apiNoCacheControl)
	c.Set(fiber.HeaderPragma, "no-cache")
	c.Set(fiber.HeaderExpires, "0")
}

// APINoCacheMiddleware prevents clients and intermediaries from retaining API
// responses. It sets headers before downstream middleware for early failures,
// then reapplies them after handlers so route-specific headers cannot weaken it.
func APINoCacheMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		if !isAPIPath(c.Path()) {
			return c.Next()
		}

		setAPINoCacheHeaders(c)
		defer setAPINoCacheHeaders(c)
		return c.Next()
	}
}
