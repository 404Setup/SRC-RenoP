/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package frontend

import (
	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
)

func SetupFrontendRoutes(app fiber.Router, state *core.AppState) {
	app.Get("/", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/index.html", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/user/:username", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/user/:username/edit", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/user/:username/maven", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/user/:username/cargo", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/user/:username/docker", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/user/:username/npm", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/domain/:domain", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/account/reviews", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/account/teams", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/account/teams/:prefix", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/account/maven-domains", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/account/maven-domains/:domain", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/assets/*", ServeAsset)
	app.Get("/js/*", ServeJs)
	app.Get("/css/*", ServeCSS)
	app.Get("/svg/*", ServeSvg)
	app.Get("/api/status/hash", GetHash)
}

func GetHash(c fiber.Ctx) error {
	h := GetAssetsHash()
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSONCharsetUTF8)
	return c.SendString(`"` + h + `"`)
}
