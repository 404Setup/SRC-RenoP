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
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/utils"
)

type captchaSettingsRequest struct {
	config.CaptchaConfig
	ClearSecret      bool `json:"clear_secret"`
	SecretConfigured bool `json:"secret_configured"`
}

func captchaSettings(value config.CaptchaConfig) fiber.Map {
	return fiber.Map{"provider": value.Provider, "site_key": value.SiteKey, "secret_key": "",
		"secret_configured": value.SecretKey != "", "min_score": value.MinScore,
		"scopes": value.Scopes, "friendly_region": value.FriendlyRegion}
}

func getCaptchaSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return fiber.ErrForbidden
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(captchaSettings(state.Inner.Config.Load().Captcha))
}

func putCaptchaSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return fiber.ErrForbidden
	}
	if !c.Is("json") {
		return fiber.ErrUnsupportedMediaType
	}
	request := captchaSettingsRequest{CaptchaConfig: config.DefaultCaptchaConfig()}
	if err := utils.ReadJSONLimited(c, &request, 16<<10); err != nil {
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) {
			return err
		}
		return cacheSettingsError(c, 400, "captcha_settings_invalid")
	}
	state.Inner.ConfigWriteLock.Lock()
	defer state.Inner.ConfigWriteLock.Unlock()
	current := state.Inner.Config.Load()
	nextCaptcha := request.CaptchaConfig
	nextCaptcha.Provider, nextCaptcha.SiteKey = strings.TrimSpace(nextCaptcha.Provider), strings.TrimSpace(nextCaptcha.SiteKey)
	nextCaptcha.SecretKey = strings.TrimSpace(nextCaptcha.SecretKey)
	if request.ClearSecret {
		nextCaptcha.SecretKey = ""
	} else if nextCaptcha.SecretKey == "" &&
		nextCaptcha.Provider == current.Captcha.Provider && nextCaptcha.SiteKey == current.Captcha.SiteKey {
		nextCaptcha.SecretKey = current.Captcha.SecretKey
	}
	if nextCaptcha.Normalize() != nil {
		return cacheSettingsError(c, 400, "captcha_settings_invalid")
	}
	next := current.DeepCopy()
	next.Captcha = nextCaptcha.DeepCopy()
	if persistConfigSnapshot(next) != nil {
		return cacheSettingsError(c, 500, "captcha_settings_save_failed")
	}
	state.Inner.Config.Store(next)
	username, operator, method, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{Username: username, Operator: operator, AuthMethod: method,
		SessionID: sessionID, IP: ip, Action: audit.ActionSettingsUpdate, Details: "Updated CAPTCHA settings"})
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(captchaSettings(next.Captcha))
}
