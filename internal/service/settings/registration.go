/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"errors"
	"github.com/gofiber/fiber/v3"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/utils"
)

func getRegistrationSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(state.Inner.Config.Load().Registration)
}

func putRegistrationSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	var request config.RegistrationConfig
	if err := utils.ReadJSONLimited(c, &request, 4096); err != nil {
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) {
			return err
		}
		return cacheSettingsError(c, 400, "registration_settings_invalid")
	}
	if request.Validate() != nil {
		return cacheSettingsError(c, 400, "registration_settings_invalid")
	}
	state.Inner.ConfigWriteLock.Lock()
	defer state.Inner.ConfigWriteLock.Unlock()
	next := state.Inner.Config.Load().DeepCopy()
	next.Registration = request
	if err := persistConfigSnapshot(next); err != nil {
		return cacheSettingsError(c, 500, "registration_settings_save_failed")
	}
	state.Inner.Config.Store(next)
	username, operator, method, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{Username: username, Operator: operator, AuthMethod: method,
		SessionID: sessionID, IP: ip, Action: audit.ActionSettingsUpdate, Details: "Updated registration settings"})
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(request)
}
