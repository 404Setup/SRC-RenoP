/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package docker implements OCI and Docker Registry v2 services.
package docker

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/repositorygate"
)

// SetupDockerRoutes registers all Docker/OCI registry v2 endpoints on the Fiber router.
func SetupDockerRoutes(app fiber.Router, state *core.AppState, store Store) {
	h := &Handler{Store: store}
	mutateImage := func(c fiber.Ctx, name string, handler func() error) error {
		repository, image := ParseRepositoryAndImage(name)
		if repository == "" || image == "" {
			return handler()
		}
		release := repositorygate.AcquireMutation(repository)
		defer release()
		if state == nil || state.GetDB() == nil {
			return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported,
				"package state is unavailable", nil)
		}
		if err := state.GetDB().EnsurePackageMutable(config.RepositoryFormatDocker, repository, image); err != nil {
			if errors.Is(err, core.ErrPackageDeprecated) {
				return RespondError(c, fiber.StatusConflict, ErrCodeDenied,
					"image is permanently deprecated and pull-only", nil)
			}
			return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported,
				"failed to inspect package state", nil)
		}
		return handler()
	}

	baseHandler := func(c fiber.Ctx) error {
		return h.HandleBase(c)
	}

	app.Get("/v2", baseHandler)
	app.Get("/v2/", baseHandler)
	app.Head("/v2", baseHandler)
	app.Head("/v2/", baseHandler)

	tokenHandler := func(c fiber.Ctx) error {
		return HandleTokenAuth(c, state)
	}
	app.Get("/v2/token", tokenHandler)
	app.Get("/v2/auth", tokenHandler)

	catalogHandler := func(c fiber.Ctx) error {
		return h.HandleCatalog(c, state)
	}
	app.Get("/v2/_catalog", catalogHandler)

	// Catch-all dispatcher for /v2/*
	dispatcher := func(c fiber.Ctx) error {
		path := strings.TrimPrefix(c.Path(), "/v2/")
		path = strings.Trim(path, "/")
		if path == "" {
			return h.HandleBase(c)
		}
		if path == "_catalog" {
			return h.HandleCatalog(c, state)
		}
		if path == "token" || path == "auth" {
			return HandleTokenAuth(c, state)
		}

		// 1. Check for /tags/list
		if before, ok := strings.CutSuffix(path, "/tags/list"); ok {
			name := before
			c.Locals("name", name)
			return h.HandleTagsList(c, state)
		}

		// 2. Check for /manifests/<reference>
		if idx := strings.Index(path, "/manifests/"); idx != -1 {
			name := path[:idx]
			reference := path[idx+len("/manifests/"):]
			c.Locals("name", name)
			c.Locals("reference", reference)
			switch c.Method() {
			case fiber.MethodGet, fiber.MethodHead:
				return h.HandleGetManifest(c, state)
			case fiber.MethodPut:
				return mutateImage(c, name, func() error { return h.HandlePutManifest(c, state) })
			case fiber.MethodDelete:
				return mutateImage(c, name, func() error { return h.HandleDeleteManifest(c, state) })
			default:
				return RespondError(c, fiber.StatusMethodNotAllowed, ErrCodeUnsupported, "method not allowed", nil)
			}
		}

		// 3. Check for /blobs/uploads/ or /blobs/uploads/<uuid>
		if idx := strings.Index(path, "/blobs/uploads"); idx != -1 {
			name := path[:idx]
			rest := strings.TrimPrefix(path[idx+len("/blobs/uploads"):], "/")
			c.Locals("name", name)
			if rest == "" {
				if c.Method() == fiber.MethodPost {
					return mutateImage(c, name, func() error { return h.HandlePostUpload(c, state) })
				}
				return RespondError(c, fiber.StatusMethodNotAllowed, ErrCodeUnsupported, "method not allowed", nil)
			}

			c.Locals("uuid", rest)
			switch c.Method() {
			case fiber.MethodPatch:
				return mutateImage(c, name, func() error { return h.HandlePatchUpload(c, state) })
			case fiber.MethodPut:
				return mutateImage(c, name, func() error { return h.HandlePutUpload(c, state) })
			case fiber.MethodGet:
				return mutateImage(c, name, func() error { return h.HandleGetUploadStatus(c, state) })
			case fiber.MethodDelete:
				return mutateImage(c, name, func() error { return h.HandleDeleteUpload(c, state) })
			default:
				return RespondError(c, fiber.StatusMethodNotAllowed, ErrCodeUnsupported, "method not allowed", nil)
			}
		}

		// 4. Check for /blobs/<digest>
		if idx := strings.Index(path, "/blobs/"); idx != -1 {
			name := path[:idx]
			digest := path[idx+len("/blobs/"):]
			c.Locals("name", name)
			c.Locals("digest", digest)
			switch c.Method() {
			case fiber.MethodGet, fiber.MethodHead:
				return h.HandleGetBlob(c, state)
			case fiber.MethodDelete:
				return mutateImage(c, name, func() error { return h.HandleDeleteBlob(c, state) })
			default:
				return RespondError(c, fiber.StatusMethodNotAllowed, ErrCodeUnsupported, "method not allowed", nil)
			}
		}

		return RespondError(c, fiber.StatusNotFound, ErrCodeNameUnknown, "endpoint not found", nil)
	}

	app.All("/v2/*", dispatcher)
}
