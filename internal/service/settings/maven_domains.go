/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/utils"
)

func getMavenDomainSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(state.Inner.Config.Load().MavenDomains)
}

func putMavenDomainSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	var request config.MavenDomainConfig
	if utils.ReadJSONLimited(c, &request, 2048) != nil || request.Validate() != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid Maven domain release period")
	}
	state.Inner.ConfigWriteLock.Lock()
	next := state.Inner.Config.Load().DeepCopy()
	next.MavenDomains = request
	err := persistConfigSnapshot(next)
	if err == nil {
		state.Inner.Config.Store(next)
	}
	state.Inner.ConfigWriteLock.Unlock()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save Maven domain settings")
	}
	username, operator, method, session, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{Username: username, Operator: operator, AuthMethod: method, SessionID: session, IP: ip,
		Action: audit.ActionSettingsUpdate, Details: "Updated publishing domain reservation period", CreatedAt: time.Now().UnixMilli()})
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(next.MavenDomains)
}
