/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/repositorygate"
	"renop/internal/utils"
)

type publicationReviewRequest struct {
	Policy string `json:"policy"`
}

func getRepositoryPublicationReviews(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	cfg := state.Inner.Config.Load()
	result := make(map[string]string, len(cfg.Maven.Repositories))
	for name, repository := range cfg.Maven.Repositories {
		result[name] = repository.PublicationReviewPolicy()
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"repositories": result})
}

func putRepositoryPublicationReview(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	repository := strings.ToLower(strings.TrimSpace(c.Params("name")))
	if !utils.IsValidRepositoryName(repository) || len(c.Body()) > 1024 {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	var request publicationReviewRequest
	if err := c.Bind().Body(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	policy, valid := config.NormalizePublicationReviewPolicy(request.Policy)
	if !valid {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid publication review policy")
	}

	releaseMigration := repositorygate.AcquireMigration(repository)
	defer releaseMigration()
	state.Inner.ConfigWriteLock.Lock()
	oldConfig := state.Inner.Config.Load()
	existing := oldConfig.Maven.Repositories[repository]
	if existing == nil {
		state.Inner.ConfigWriteLock.Unlock()
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}
	if !existing.SupportsPublicationReview() {
		state.Inner.ConfigWriteLock.Unlock()
		return c.Status(fiber.StatusConflict).SendString("Publication review is not supported by this repository engine")
	}
	if state.GetDB() != nil {
		if pending, pendingErr := state.GetDB().HasPendingPublicationReviews(repository); pendingErr != nil {
			state.Inner.ConfigWriteLock.Unlock()
			return c.Status(fiber.StatusServiceUnavailable).SendString("Repository review state is unavailable")
		} else if pending && policy != existing.PublicationReviewPolicy() {
			state.Inner.ConfigWriteLock.Unlock()
			c.Set("X-Renop-Error-Code", "repository_pending_review")
			return c.Status(fiber.StatusConflict).SendString("Repository has pending publication reviews")
		}
	}
	nextConfig := oldConfig.DeepCopy()
	nextRepository := nextConfig.Maven.Repositories[repository]
	nextRepository.PublicationReview = policy
	if policy != config.PublicationReviewOff {
		nextRepository.AllowRedeployment = false
	}
	if err := saveRepositories(nextConfig); err != nil {
		state.Inner.ConfigWriteLock.Unlock()
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save repository settings")
	}
	state.Inner.Config.Store(nextConfig)
	state.Inner.ConfigWriteLock.Unlock()

	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionSettingsUpdate,
		Details: "Updated publication review for (" + repository + ")", AuthMethod: authMethod,
		SessionID: sessionID, IP: ip,
	})
	return c.JSON(fiber.Map{"repository": repository, "policy": policy})
}
