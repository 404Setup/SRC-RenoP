/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/repositorygate"
	"renop/internal/service/statistics"
	"renop/internal/utils"
)

type downloadStatisticsRequest struct {
	Enabled *bool `json:"enabled"`
}

func getRepositoryDownloadStatistics(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	cfg := state.Inner.Config.Load()
	result := make(map[string]bool, len(cfg.Maven.Repositories))
	for name, repository := range cfg.Maven.Repositories {
		result[name] = repository.DownloadStatisticsEnabled()
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"repositories": result})
}

func putRepositoryDownloadStatistics(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	repository := strings.ToLower(strings.TrimSpace(c.Params("name")))
	if !utils.IsValidRepositoryName(repository) || len(c.Body()) > 1024 {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	var request downloadStatisticsRequest
	if err := c.Bind().Body(&request); err != nil || request.Enabled == nil {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	releaseMigration := repositorygate.AcquireMigration(repository)
	defer releaseMigration()
	state.Inner.ConfigWriteLock.Lock()
	oldConfig := state.Inner.Config.Load()
	if oldConfig.Maven.Repositories[repository] == nil {
		state.Inner.ConfigWriteLock.Unlock()
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}
	nextConfig := oldConfig.DeepCopy()
	enabled := *request.Enabled
	nextConfig.Maven.Repositories[repository].DownloadStatistics = &enabled
	if err := saveRepositories(nextConfig); err != nil {
		state.Inner.ConfigWriteLock.Unlock()
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save repository settings")
	}
	state.Inner.Config.Store(nextConfig)
	state.Inner.ConfigWriteLock.Unlock()
	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionSettingsUpdate,
		Details: "Updated download statistics for (" + repository + ")", AuthMethod: authMethod,
		SessionID: sessionID, IP: ip,
	})
	return c.JSON(fiber.Map{"repository": repository, "enabled": enabled})
}

func resetRepositoryDownloadStatistics(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	repository := strings.ToLower(strings.TrimSpace(c.Params("name")))
	if !utils.IsValidRepositoryName(repository) {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	releaseMigration := repositorygate.AcquireMigration(repository)
	defer releaseMigration()
	if state.Inner.Config.Load().Maven.Repositories[repository] == nil {
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}
	if err := statistics.GetCounter(state).ResetRepository(repository); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to reset download statistics")
	}
	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionSettingsUpdate,
		Details: "Reset download statistics for (" + repository + ")", AuthMethod: authMethod,
		SessionID: sessionID, IP: ip,
	})
	return c.SendStatus(fiber.StatusNoContent)
}
