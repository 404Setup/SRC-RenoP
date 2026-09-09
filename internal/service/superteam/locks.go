/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package superteam

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/repositorygate"
	"renop/internal/utils"
)

func setResourceLock(c fiber.Ctx, state *core.AppState) error {
	user, session := auth.GetUser(c), auth.CurrentSessionToken(c)
	if user == nil || !user.CheckModeratePermission("") || auth.CurrentCredentialKind(c) != "session" ||
		session == "" || c.Cookies("renop_session") != session {
		return apiError(c, core.ErrResourceLockPermission)
	}
	var request struct {
		Mode   string `json:"mode"`
		Reason string `json:"reason"`
	}
	if utils.ReadJSONLimited(c, &request, 4096) != nil {
		return apiError(c, core.ErrResourceLockInvalid)
	}
	prefix, valid := core.NormalizeSuperTeamPrefix(c.Params("prefix"))
	if !valid {
		return apiError(c, core.ErrResourceLockInvalid)
	}
	// A team can own resources in every repository, including both sides of a pending transfer.
	release := repositorygate.AcquireAllMigrations()
	defer release()
	target := core.ResourceLockTarget{Format: "superteam", Name: prefix}
	var err error
	action := audit.ActionResourceLock
	if c.Method() == fiber.MethodDelete {
		err = state.GetDB().DeleteResourceLock(target, core.ResourceLockManual, user.Username, session)
		action = audit.ActionResourceUnlock
	} else {
		err = state.GetDB().SetResourceLock(&core.ResourceLock{ResourceLockTarget: target,
			Mode: request.Mode, Reason: request.Reason, Source: core.ResourceLockManual,
			LockedAt: time.Now().UnixMilli()}, user.Username, session)
	}
	if err != nil {
		return apiError(c, err)
	}
	auditAction(c, state, action, "Global team: "+prefix+", reason: "+request.Reason)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.SendStatus(fiber.StatusNoContent)
}
