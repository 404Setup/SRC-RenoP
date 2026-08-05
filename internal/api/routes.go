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
	"os"
	"sync"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
)

var (
	privacyPolicy     []byte
	privacyPolicyOnce sync.Once
)

func getCachedPolicy() []byte {
	privacyPolicyOnce.Do(func() {
		data, err := os.ReadFile("privacy-policy.txt")
		if err == nil {
			privacyPolicy = data
		}
	})
	return privacyPolicy
}

func SetupApiRoutes(router fiber.Router, state *core.AppState) {
	router.Get("/maven/details", func(c fiber.Ctx) error { return GetDetailsAllRepos(c, state) })
	router.Get("/maven/details/", func(c fiber.Ctx) error { return GetDetailsAllRepos(c, state) })
	router.Get("/maven/details/:repo_name", func(c fiber.Ctx) error { return GetDetailsRoot(c, state) })
	router.Get("/maven/details/:repo_name/*", func(c fiber.Ctx) error { return GetDetails(c, state) })
	router.Get("/maven/repo-details/:repo_name", func(c fiber.Ctx) error { return GetRepoDetails(c, state) })

	router.Get("/maven/versions/:repo_name/*", func(c fiber.Ctx) error { return FindVersions(c, state) })
	router.Get("/maven/latest/version/:repo_name/*", func(c fiber.Ctx) error { return LatestVersion(c, state) })
	router.Get("/maven/latest/details/:repo_name/*", func(c fiber.Ctx) error { return LatestDetails(c, state) })
	router.Get("/maven/latest/file/:repo_name/*", func(c fiber.Ctx) error { return LatestFile(c, state) })

	router.Get("/badge/latest/:repo_name/*", func(c fiber.Ctx) error { return LatestBadge(c, state) })
	router.Post("/maven/generate/pom/:repo_name/*", func(c fiber.Ctx) error { return GeneratePom(c, state) })

	router.Head("/privacy-policy", func(c fiber.Ctx) error {
		if getCachedPolicy() != nil {
			return c.SendStatus(fiber.StatusOK)
		}
		return c.SendStatus(fiber.StatusNotFound)
	})

	router.Get("/privacy-policy", func(c fiber.Ctx) error {
		policy := getCachedPolicy()
		if policy != nil {
			c.Set(fiber.HeaderContentType, "text/plain; charset=utf-8")
			c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
			return c.Status(fiber.StatusOK).Send(policy)
		}
		return c.SendStatus(fiber.StatusNotFound)
	})
}
