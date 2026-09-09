/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package api

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/docker"
	"renop/internal/service/repositorygate"
	"renop/internal/utils"
)

func setDockerResourceLockAPI(c fiber.Ctx, state *core.AppState) error {
	repository := c.Params("repo_name")
	image, valid := docker.NormalizeImageName(c.Query("image"))
	if !valid || !utils.IsValidRepositoryName(repository) {
		return dockerAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid Docker resource")
	}
	user, session := auth.GetUser(c), auth.CurrentSessionToken(c)
	if user == nil || !user.CheckModeratePermission(repository) || auth.CurrentCredentialKind(c) != "session" || session == "" || c.Cookies("renop_session") != session {
		return dockerAPIError(c, fiber.StatusForbidden, "permission_denied", "A moderator browser session is required")
	}
	var request struct {
		Version string `json:"version"`
		Mode    string `json:"mode"`
		Reason  string `json:"reason"`
	}
	if utils.ReadJSONLimited(c, &request, 4096) != nil {
		return dockerAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid resource lock")
	}
	if request.Version != "" {
		if len(request.Version) != 71 || !strings.HasPrefix(request.Version, "sha256:") || strings.ToLower(request.Version) != request.Version {
			return dockerAPIError(c, fiber.StatusBadRequest, "invalid_request", "Lock an immutable manifest digest")
		}
		if _, err := hex.DecodeString(request.Version[7:]); err != nil {
			return dockerAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid manifest digest")
		}
	}
	release := repositorygate.AcquireMigration(repository)
	defer release()
	repo := state.Inner.Config.Load().Maven.Repositories[repository]
	if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return dockerAPIError(c, fiber.StatusNotFound, "repository_not_found", "Docker repository was not found")
	}
	db := state.GetDB()
	if db == nil {
		return dockerImageMutationError(c, core.ErrDatabaseUnavailable)
	}
	pkg, err := db.GetDockerImage(repository, image)
	if err != nil {
		return dockerImageMutationError(c, err)
	}
	if pkg == nil {
		return dockerAPIError(c, fiber.StatusNotFound, "image_not_found", "Docker image was not found")
	}
	if request.Version != "" {
		manifest, err := db.GetDockerManifest(repository, image, request.Version)
		if err != nil {
			return dockerImageMutationError(c, err)
		}
		if manifest == nil {
			return dockerAPIError(c, fiber.StatusNotFound, "manifest_not_found", "Manifest was not found")
		}
	}
	target := core.ResourceLockTarget{Format: "docker", Repository: repository, Name: image, Version: request.Version}
	action := audit.ActionResourceLock
	if c.Method() == fiber.MethodDelete {
		action = audit.ActionResourceUnlock
		err = db.DeleteResourceLock(target, core.ResourceLockManual, user.Username, session)
	} else {
		err = db.SetResourceLock(&core.ResourceLock{ResourceLockTarget: target, Source: core.ResourceLockManual,
			Mode: request.Mode, Reason: request.Reason, LockedAt: time.Now().UnixMilli()}, user.Username, session)
	}
	if errors.Is(err, core.ErrResourceLockPermission) {
		return dockerAPIError(c, fiber.StatusForbidden, "permission_denied", "Moderator permission is required")
	}
	if errors.Is(err, core.ErrResourceLockInvalid) || errors.Is(err, core.ErrDockerManifestInvalid) {
		return dockerAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid resource lock or manifest references")
	}
	if err != nil {
		return dockerImageMutationError(c, err)
	}
	logDockerAudit(c, state, action, "Repository: "+repository+", image: "+image+", digest: "+request.Version+", reason: "+request.Reason)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"ok": true})
}
