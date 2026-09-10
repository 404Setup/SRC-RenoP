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
	"errors"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/utils"
)

func getLegalSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return fiber.ErrForbidden
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(state.Inner.Config.Load().Legal)
}

func putLegalSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return fiber.ErrForbidden
	}
	if !c.Is("json") {
		return fiber.ErrUnsupportedMediaType
	}
	var request config.LegalConfig
	if err := utils.ReadJSONLimited(c, &request, 10<<20); err != nil {
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) {
			return err
		}
		return cacheSettingsError(c, 400, "legal_settings_invalid")
	}
	if err := request.Normalize(); err != nil {
		return cacheSettingsError(c, 400, "legal_settings_invalid")
	}
	state.Inner.ConfigWriteLock.Lock()
	defer state.Inner.ConfigWriteLock.Unlock()
	next := state.Inner.Config.Load().DeepCopy()
	next.Legal = request.DeepCopy()
	if err := persistConfigSnapshot(next); err != nil {
		return cacheSettingsError(c, 500, "legal_settings_save_failed")
	}
	state.Inner.Config.Store(next)
	username, operator, method, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{Username: username, Operator: operator, AuthMethod: method,
		SessionID: sessionID, IP: ip, Action: audit.ActionSettingsUpdate, Details: "Updated legal documents and cookie notice"})
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(next.Legal)
}
