/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package storage provides repository file, S3, checksum, and publication services.
package storage

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/service/cargo"
	"renop/internal/service/npm"
	"renop/internal/service/statistics"
	"renop/internal/utils"
)

// HTMLFallback serves the SPA shell for browser GETs that miss an artifact.
// Wired from main to avoid a storage → frontend import cycle.
var HTMLFallback func(c fiber.Ctx, state *core.AppState) error

func mavenMutationError(c fiber.Ctx, err error) error {
	if errors.Is(err, core.ErrPackageDeprecated) {
		c.Set("X-Renop-Error-Code", "package_deprecated")
		return c.Status(fiber.StatusConflict).SendString("Maven artifact is permanently deprecated and read-only")
	}
	if errors.Is(err, core.ErrMavenDomainUnverified) {
		return c.Status(fiber.StatusConflict).SendString("Maven domain must be verified before publication")
	}
	if errors.Is(err, core.ErrDatabaseUnavailable) {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Maven domain authorization is unavailable")
	}
	return c.Status(fiber.StatusForbidden).SendString("Maven domain permission denied")
}

func SetupRoutes(app fiber.Router, state *core.AppState) {
	handler := func(c fiber.Ctx) error {
		return HandleRepository(c, state)
	}
	app.All("/:repo_name/*", handler)
	app.All("/:repo_name", handler)
}

func HandleRepository(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	if !utils.IsValidRepositoryName(repoName) {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	path := c.Params("*")
	if path == "" {
		path = "/"
	}

	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists {
		if c.Method() == fiber.MethodGet {
			accept := c.Get(fiber.HeaderAccept)
			if strings.Contains(accept, "text/html") {
				return serveHTMLFallback(c, state)
			}
		}
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	user := auth.GetUser(c)
	isCargo := repo.NormalizedFormat() == config.RepositoryFormatCargo
	isDocker := repo.NormalizedFormat() == config.RepositoryFormatDocker
	isMaven := repo.NormalizedFormat() == config.RepositoryFormatMaven
	isNPM := repo.NormalizedFormat() == config.RepositoryFormatNPM
	isRead := c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead

	if isNPM {
		if handled, err := npmHandler.Handle(c, state, repo, cfg.StoragePath, path); handled {
			return err
		}
		if !isRead {
			return c.Status(fiber.StatusMethodNotAllowed).SendString("npm repositories must be modified through npm registry endpoints")
		}
		var valid bool
		path, valid = npm.NormalizeRegistryPath(path)
		if !valid {
			return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
		}
	}

	sanitized, ok := utils.SanitizePath(path)
	if !ok {
		if handled, err := TryHTMLFallback(state, c); handled {
			return err
		}
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}

	localFilePath := filepath.Join(cfg.StoragePath, repoName, sanitized)
	if !utils.IsSubPath(cfg.StoragePath, localFilePath) {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	pathStr := localFilePath
	isDirOnDisk, _, isIndexed, isNotFound := state.Inner.FileIndex.GetPathState(pathStr)
	isConcreteArtifact := isIndexed && !isDirOnDisk

	if isRead {
		isRoot := strings.HasSuffix(path, "/") || path == "" || isDirOnDisk
		canRead, err := cargo.CanReadRepository(state, user, repo, sanitized, isRoot)
		if isMaven && MavenReadAuthorizer != nil {
			canRead, err = MavenReadAuthorizer(state, user, repo, sanitized, isRoot)
		} else if isNPM {
			canRead, err = npm.CanReadRepository(state, user, repo, sanitized, isRoot)
		}
		if err != nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("Repository metadata is unavailable")
		}
		if !canRead {
			if isCargo && sanitized == "config.json" &&
				strings.EqualFold(repo.Visibility, "PRIVATE") && user.Username == "guest" {
				return cargo.SendAuthChallenge(c)
			}
			if !isConcreteArtifact {
				if handled, fallbackErr := TryHTMLFallback(state, c); handled {
					return fallbackErr
				}
			}
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		if isDirOnDisk {
			if handled, fallbackErr := TryHTMLFallback(state, c); handled {
				return fallbackErr
			}
		}
	} else if isMaven {
		if MavenMutationAuthorizer == nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("Maven domain authorization is unavailable")
		}
		requiredLevel := core.MavenPermissionPublish
		if c.Method() == fiber.MethodDelete {
			requiredLevel = core.MavenPermissionVersion
		}
		if err := MavenMutationAuthorizer(state, user, repo, sanitized, requiredLevel); err != nil {
			return mavenMutationError(c, err)
		}
	} else if !isCargo && !isDocker && !isNPM && !user.CheckUpdatePermission(repoName) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	path = sanitized
	if isDocker {
		if !isConcreteArtifact {
			if handled, err := TryHTMLFallback(state, c); handled {
				return err
			}
		}
		if isRead {
			return c.Status(fiber.StatusOK).SendString("Docker repository must be accessed via Docker client or /v2/ API")
		}
		return c.Status(fiber.StatusMethodNotAllowed).SendString("Docker repositories must be modified through the Docker registry API")
	}

	if isCargo {
		if handled, err := cargoHandler.Handle(c, state, repo, cfg.StoragePath, path); handled {
			return err
		}
		if !isRead {
			return c.Status(fiber.StatusMethodNotAllowed).SendString("Cargo repositories must be modified through the Cargo registry API")
		}
	}

	if !isIndexed && isNotFound && c.Method() != fiber.MethodPut && c.Method() != fiber.MethodPost {
		if handled, err := TryHTMLFallback(state, c); handled {
			return err
		}
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	if !isIndexed && len(repo.Mirrors) == 0 && c.Method() != fiber.MethodPut && c.Method() != fiber.MethodPost {
		if handled, err := TryHTMLFallback(state, c); handled {
			return err
		}
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	switch c.Method() {
	case fiber.MethodGet:
		err := HandleGet(c, state, repo, cfg.StoragePath)
		if err == nil {
			statistics.RecordRepositoryDownload(c, state, repo, path)
		}
		return err
	case fiber.MethodHead:
		return HandleHead(c, state, repo, cfg.StoragePath)
	case fiber.MethodPut, fiber.MethodPost:
		return HandlePut(c, state, repo, localFilePath)
	case fiber.MethodDelete:
		return HandleDelete(c, state, repo, path, localFilePath)
	default:
		return c.Status(fiber.StatusMethodNotAllowed).SendString("Method not allowed")
	}
}

func TryHTMLFallback(state *core.AppState, c fiber.Ctx) (bool, error) {
	if HTMLFallback == nil {
		return false, nil
	}
	if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
		return false, nil
	}
	if !strings.Contains(c.Get(fiber.HeaderAccept), "text/html") {
		return false, nil
	}
	return true, HTMLFallback(c, state)
}

func serveHTMLFallback(c fiber.Ctx, state *core.AppState) error {
	if HTMLFallback != nil {
		return HTMLFallback(c, state)
	}
	return c.Status(fiber.StatusNotFound).SendString("Not found")
}
