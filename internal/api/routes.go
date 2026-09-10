/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package api implements format-aware general application endpoints.
package api

import (
	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/legal"
	"renop/internal/service/repositorygate"
)

// SetupAPIRoutes registers the general application API endpoints.
func SetupAPIRoutes(router fiber.Router, state *core.AppState) {
	dockerRoute := func(handler func(fiber.Ctx, *core.AppState) error) fiber.Handler {
		return withDockerAPIErrorCode(func(c fiber.Ctx) error {
			if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
				release := repositorygate.AcquireMutation(c.Params("repo_name"))
				defer release()
			}
			return handler(c, state)
		})
	}
	router.Get("/maven/details", func(c fiber.Ctx) error { return GetDetailsAllRepos(c, state) })
	router.Get("/maven/details/", func(c fiber.Ctx) error { return GetDetailsAllRepos(c, state) })
	router.Get("/maven/details/:repo_name", func(c fiber.Ctx) error { return GetDetailsRoot(c, state) })
	router.Get("/maven/details/:repo_name/*", func(c fiber.Ctx) error { return GetDetails(c, state) })
	router.Get("/maven/repo-details/:repo_name", func(c fiber.Ctx) error { return GetRepoDetails(c, state) })
	router.Get("/maven/signatures/:repo_name/*", func(c fiber.Ctx) error { return GetGPGSignature(c, state) })
	router.Get("/repositories/details", func(c fiber.Ctx) error { return GetDetailsAllRepos(c, state) })
	router.Get("/repositories/details/", func(c fiber.Ctx) error { return GetDetailsAllRepos(c, state) })
	router.Get("/repositories/details/:repo_name", func(c fiber.Ctx) error { return GetDetailsRoot(c, state) })
	router.Get("/repositories/details/:repo_name/*", func(c fiber.Ctx) error { return GetDetails(c, state) })
	router.Get("/repositories/repo-details/:repo_name", func(c fiber.Ctx) error { return GetRepoDetails(c, state) })
	router.Get("/repositories/search/:repo_name", func(c fiber.Ctx) error { return SearchRepository(c, state) })

	router.Get("/docker/repositories/:repo_name/images", withDockerAPIErrorCode(func(c fiber.Ctx) error {
		if c.Query("image") != "" {
			return GetDockerImageDetailsAPI(c, state)
		}
		return ListDockerImagesAPI(c, state)
	}))
	router.Post("/docker/repositories/:repo_name/images", dockerRoute(CreateDockerImageAPI))
	for _, method := range []string{fiber.MethodPut, fiber.MethodDelete} {
		router.Add([]string{method}, "/docker/repositories/:repo_name/locks", withDockerAPIErrorCode(func(c fiber.Ctx) error {
			return setDockerResourceLockAPI(c, state)
		}))
	}
	router.Get("/docker/repositories/:repo_name/images/*", dockerRoute(GetDockerImageDetailsAPI))
	router.Put("/docker/repositories/:repo_name/images", dockerRoute(UpdateDockerImageDescriptionAPI))
	router.Put("/docker/repositories/:repo_name/images/deprecate", dockerRoute(DeprecateDockerImageAPI))
	router.Put("/docker/repositories/:repo_name/images/*", dockerRoute(UpdateDockerImageDescriptionAPI))
	router.Get("/docker/repositories/:repo_name/manifests", dockerRoute(GetDockerManifestAPI))
	router.Get("/docker/repositories/:repo_name/manifests/*", dockerRoute(GetDockerManifestAPI))
	router.Delete("/docker/repositories/:repo_name/images", dockerRoute(DeleteDockerImageAPI))
	router.Delete("/docker/repositories/:repo_name/images/*", dockerRoute(DeleteDockerImageAPI))
	router.Delete("/docker/repositories/:repo_name/tags", dockerRoute(DeleteDockerTagAPI))
	router.Delete("/docker/repositories/:repo_name/tags/*", dockerRoute(DeleteDockerTagAPI))
	router.Get("/docker/repositories/:repo_name/owners", dockerRoute(ListDockerOwnersAPI))
	router.Post("/docker/repositories/:repo_name/owners", dockerRoute(InviteDockerOwnersAPI))
	router.Put("/docker/repositories/:repo_name/owners/:username", dockerRoute(SetDockerOwnerLevelAPI))
	router.Delete("/docker/repositories/:repo_name/owners/:username", dockerRoute(RemoveDockerOwnerAPI))
	router.Get("/docker/repositories/:repo_name/users/search", dockerRoute(SearchDockerUsersAPI))
	router.Post("/docker/repositories/:repo_name/invitations/:id/:decision", dockerRoute(RespondDockerInvitationAPI))

	router.Get("/maven/versions/:repo_name/*", func(c fiber.Ctx) error { return FindVersions(c, state) })
	router.Get("/maven/latest/version/:repo_name/*", func(c fiber.Ctx) error { return LatestVersion(c, state) })
	router.Get("/maven/latest/details/:repo_name/*", func(c fiber.Ctx) error { return LatestDetails(c, state) })
	router.Get("/maven/latest/file/:repo_name/*", func(c fiber.Ctx) error { return LatestFile(c, state) })

	router.Get("/badge/latest/:repo_name/*", func(c fiber.Ctx) error { return LatestBadge(c, state) })
	router.Post("/maven/generate/pom/:repo_name/*", func(c fiber.Ctx) error { return GeneratePom(c, state) })

	legal.SetupRoutes(router, state)
}
