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
	"github.com/gofiber/fiber/v3"

	"renop/internal/cache"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
)

type cacheSettingsResponse struct {
	cache.Config
	PasswordConfigured bool `json:"password_configured"`
	RestartRequired    bool `json:"restart_required"`
}

type cacheSettingsRequest struct {
	cache.Config
	ClearPassword bool `json:"clear_password"`
}

func cacheSettingsError(c fiber.Ctx, status int, code string) error {
	c.Set("X-Renop-Error-Code", code)
	return c.Status(status).JSON(fiber.Map{"error": code})
}

func cacheSettings(cfg cache.Config) cacheSettingsResponse {
	cfg.Normalize()
	configured := cfg.Password != ""
	cfg.Password = ""
	return cacheSettingsResponse{Config: cfg, PasswordConfigured: configured, RestartRequired: true}
}

func readCacheSettings(c fiber.Ctx, state *core.AppState) (config.CacheConfig, error) {
	if len(c.Body()) > 8192 {
		return cache.Config{}, fiber.ErrRequestEntityTooLarge
	}
	var request cacheSettingsRequest
	if err := c.Bind().Body(&request); err != nil {
		return cache.Config{}, fiber.ErrBadRequest
	}
	next := request.Config
	if next.Password == "" && !request.ClearPassword {
		next.Password = state.Inner.Config.Load().Cache.Password
	}
	next.Normalize()
	if err := next.Validate(); err != nil {
		return cache.Config{}, fiber.ErrBadRequest
	}
	return next, nil
}

func getCacheSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(cacheSettings(state.Inner.Config.Load().Cache))
}

func putCacheSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	state.Inner.ConfigWriteLock.Lock()
	defer state.Inner.ConfigWriteLock.Unlock()
	nextCache, err := readCacheSettings(c, state)
	if err != nil {
		if err == fiber.ErrBadRequest {
			return cacheSettingsError(c, 400, "cache_settings_invalid")
		}
		return err
	}
	next := state.Inner.Config.Load().DeepCopy()
	next.Cache = nextCache
	if err := persistConfigSnapshot(next); err != nil {
		return cacheSettingsError(c, 500, "cache_settings_save_failed")
	}
	state.Inner.Config.Store(next)
	username, operator, method, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{Username: username, Operator: operator, AuthMethod: method,
		SessionID: sessionID, IP: ip, Action: audit.ActionSettingsUpdate, Details: "Updated cache settings"})
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(cacheSettings(nextCache))
}

func testCacheSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	cfg, err := readCacheSettings(c, state)
	if err != nil {
		if err == fiber.ErrBadRequest {
			return cacheSettingsError(c, 400, "cache_settings_invalid")
		}
		return err
	}
	remote, err := cache.Open(cfg)
	if err != nil {
		return cacheSettingsError(c, 502, "cache_connection_failed")
	}
	if err := remote.Close(); err != nil {
		return cacheSettingsError(c, 502, "cache_connection_failed")
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"ok": true})
}
