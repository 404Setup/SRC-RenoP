/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
)

const maxConfiguredSuperTeamLimit = 1000

type superTeamConfigRequest struct {
	CreateLimit int `json:"create_limit"`
	JoinLimit   int `json:"join_limit"`
}

func getSuperTeamSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(state.Inner.Config.Load().SuperTeams)
}

func putSuperTeamSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	if len(c.Body()) > 2048 {
		return c.SendStatus(fiber.StatusRequestEntityTooLarge)
	}
	var request superTeamConfigRequest
	if err := c.Bind().Body(&request); err != nil || request.CreateLimit < 1 || request.JoinLimit < 1 ||
		request.CreateLimit > maxConfiguredSuperTeamLimit || request.JoinLimit > maxConfiguredSuperTeamLimit {
		return c.Status(fiber.StatusBadRequest).SendString("Global team limits must be between 1 and 1000")
	}

	state.Inner.ConfigWriteLock.Lock()
	current := state.Inner.Config.Load()
	next := current.DeepCopy()
	next.SuperTeams = config.SuperTeamConfig{CreateLimit: request.CreateLimit, JoinLimit: request.JoinLimit}
	err := persistConfigSnapshot(next)
	if err == nil {
		state.Inner.Config.Store(next)
	}
	state.Inner.ConfigWriteLock.Unlock()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save global team limits")
	}

	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionSettingsUpdate,
		Details: "Updated global team limits", AuthMethod: authMethod, SessionID: sessionID, IP: ip,
		CreatedAt: time.Now().UnixMilli(),
	})
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(next.SuperTeams)
}
