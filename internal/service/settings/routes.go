/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package settings validates and applies administrator configuration changes.
package settings

import (
	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/service/token"
)

func SetupSettingsRoutes(router fiber.Router, state *core.AppState) {
	router.Get("/github-oauth", func(c fiber.Ctx) error { return getGitHubOAuthSettings(c, state) })
	router.Put("/github-oauth", func(c fiber.Ctx) error { return putGitHubOAuthSettings(c, state) })
	router.Post("/index/rebuild", func(c fiber.Ctx) error { return RebuildIndex(c, state) })
	router.Get("/domains", func(c fiber.Ctx) error { return GetDomains(c) })
	router.Get("/domain/:name", func(c fiber.Ctx) error { return GetDomainSettings(c, state) })
	router.Put("/domain/:name", func(c fiber.Ctx) error { return UpdateDomainSettings(c, state) })
	router.Get("/maven/repositories", func(c fiber.Ctx) error { return GetMavenRepositories(c, state) })
	router.Put("/maven/repositories/:name", func(c fiber.Ctx) error { return PutMavenRepository(c, state) })
	router.Post("/maven/repositories/:name/migrate/:target", func(c fiber.Ctx) error { return MigrateRepositoryEngine(c, state) })
	router.Delete("/maven/repositories/:name", func(c fiber.Ctx) error { return DeleteMavenRepository(c, state) })
	// Generic aliases are used by the multi-format UI. The Maven-prefixed
	// routes remain available for older clients.
	router.Get("/repositories", func(c fiber.Ctx) error { return GetMavenRepositories(c, state) })
	router.Get("/repositories/download-statistics", func(c fiber.Ctx) error {
		return getRepositoryDownloadStatistics(c, state)
	})
	router.Put("/repositories/:name/download-statistics", func(c fiber.Ctx) error {
		return putRepositoryDownloadStatistics(c, state)
	})
	router.Delete("/repositories/:name/download-statistics", func(c fiber.Ctx) error {
		return resetRepositoryDownloadStatistics(c, state)
	})
	router.Put("/repositories/:name", func(c fiber.Ctx) error { return PutMavenRepository(c, state) })
	router.Post("/repositories/:name/migrate/:target", func(c fiber.Ctx) error { return MigrateRepositoryEngine(c, state) })
	router.Delete("/repositories/:name", func(c fiber.Ctx) error { return DeleteMavenRepository(c, state) })
}

func isManager(c fiber.Ctx) bool {
	return token.RequireManager(auth.GetUser(c))
}
