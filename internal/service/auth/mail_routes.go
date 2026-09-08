/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/mailqueue"
)

func getMailJobStatus(c fiber.Ctx, state *core.AppState) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	if len(c.Params("id")) > 64 || len(c.Get("X-Renop-Mail-Ticket")) > 128 {
		return c.SendStatus(404)
	}
	cfg := state.Inner.Config.Load().Mail
	if cfg.EncryptionKey == "" {
		return c.SendStatus(404)
	}
	job, err := state.GetDB().GetMailJob(c.Params("id"), cfg.EncryptionKey)
	if err != nil {
		c.Set("X-Renop-Error-Code", "mail_status_unavailable")
		return c.SendStatus(503)
	}
	if job == nil {
		return c.SendStatus(404)
	}
	userID := ""
	if user := GetUser(c); user != nil && user.Username != "guest" {
		profile, err := state.GetDB().GetUserProfile(user.Username)
		if err != nil {
			c.Set("X-Renop-Error-Code", "mail_status_unavailable")
			return c.SendStatus(503)
		}
		if profile != nil {
			userID = profile.UserID
		}
	}
	if !mailqueue.CanRead(job, userID, c.Get("X-Renop-Mail-Ticket")) {
		return c.SendStatus(404)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(mailqueue.View(job))
}
