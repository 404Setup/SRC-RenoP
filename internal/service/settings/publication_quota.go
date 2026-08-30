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
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
)

func validGlobalPublicationQuota(request config.PublicationQuotaConfig) bool {
	return request.FileLimit >= 1 && request.FileLimit <= 10_000_000 &&
		request.ByteLimit >= 1 && request.ByteLimit <= 1<<50 &&
		request.PublicationLimit >= 1 && request.PublicationLimit <= 1_000_000 &&
		core.ValidPublicationQuotaPeriod(request.Period)
}

func getPublicationQuotaSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(state.Inner.Config.Load().PublicationQuota)
}

func putPublicationQuotaSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	if len(c.Body()) > 2048 {
		return c.SendStatus(fiber.StatusRequestEntityTooLarge)
	}
	var request config.PublicationQuotaConfig
	if err := c.Bind().Body(&request); err != nil || !validGlobalPublicationQuota(request) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid publication quota settings")
	}

	state.Inner.ConfigWriteLock.Lock()
	current := state.Inner.Config.Load()
	next := current.DeepCopy()
	next.PublicationQuota = request
	err := persistConfigSnapshot(next)
	if err == nil {
		state.Inner.Config.Store(next)
	}
	state.Inner.ConfigWriteLock.Unlock()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save publication quota settings")
	}

	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionSettingsUpdate,
		Details: "Updated global publication quotas", AuthMethod: authMethod, SessionID: sessionID, IP: ip,
		CreatedAt: time.Now().UnixMilli(),
	})
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(next.PublicationQuota)
}
