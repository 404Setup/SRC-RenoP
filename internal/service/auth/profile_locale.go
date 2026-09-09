/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"renop/internal/core"
	"renop/internal/locale"
)

func accountLocale(c fiber.Ctx, state *core.AppState) error {
	setPrivateResponseHeaders(c)
	user, err := requireAccountSession(c)
	if err != nil {
		return accountSessionError(c, err)
	}
	session := c.Locals("current_session_id").(string)
	if c.Cookies(sessionCookieName) != session {
		return accountSessionError(c, fiber.ErrForbidden)
	}
	if c.Method() == fiber.MethodGet {
		profile, err := state.GetDB().GetUserProfile(user.Username)
		if err != nil {
			return c.SendStatus(fiber.StatusServiceUnavailable)
		}
		return c.JSON(fiber.Map{"user_id": profile.UserID, "username": profile.Username, "locale": locale.Match(profile.Locale)})
	}
	var request struct {
		UserID string `json:"user_id"`
		Locale string `json:"locale"`
	}
	if err := readMFARequest(c, &request); err != nil {
		return err
	}
	err = state.GetDB().SetUserLocale(user.Username, session, request.Locale, request.UserID)
	if errors.Is(err, core.ErrUserProfileNotFound) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	if errors.Is(err, locale.ErrUnsupported) {
		return passwordResetError(c, 400, "ACCOUNT_LOCALE_INVALID")
	}
	if errors.Is(err, core.ErrEmailCodeInvalid) || errors.Is(err, core.ErrAccountDeleted) {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	if err != nil {
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	return c.JSON(fiber.Map{"user_id": request.UserID, "username": user.Username, "locale": locale.Match(request.Locale)})
}
