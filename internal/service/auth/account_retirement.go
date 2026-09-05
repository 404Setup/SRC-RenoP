/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/token"
	"renop/internal/utils"
)

type accountRetirementRequest struct {
	Confirmation string `json:"confirmation"`
}

func setupAccountRetirementRoutes(router fiber.Router, state *core.AppState, opChan chan<- token.TokenOp) {
	router.Get("/profile/retirement", func(c fiber.Ctx) error {
		user, err := requireAccountSession(c)
		if err != nil {
			return accountSessionError(c, err)
		}
		plan, err := state.GetDB().GetAccountRetirementPlan(user.Username)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to inspect account retirement")
		}
		c.Set(fiber.HeaderCacheControl, "no-store")
		return c.JSON(plan)
	})
	router.Delete("/profile/retirement", func(c fiber.Ctx) error {
		user, err := requireAccountSession(c)
		if err != nil {
			return accountSessionError(c, err)
		}
		var request accountRetirementRequest
		if err := utils.ReadJSONLimited(c, &request, 1024); err != nil ||
			strings.TrimSpace(request.Confirmation) != user.Username {
			c.Set("X-Renop-Error-Code", "ACCOUNT_RETIREMENT_CONFIRMATION")
			return c.Status(fiber.StatusBadRequest).SendString("Account retirement confirmation does not match")
		}
		username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
		retiredAt := time.Now().UnixMilli()
		if err := token.RetireAccountSync(state, opChan, user.Username, retiredAt); errors.Is(err, core.ErrAccountRetirementBusy) {
			plan, planErr := state.GetDB().GetAccountRetirementPlan(user.Username)
			if planErr != nil {
				return c.Status(fiber.StatusConflict).SendString("Account retirement prerequisites are not satisfied")
			}
			c.Set("X-Renop-Error-Code", "ACCOUNT_RETIREMENT_BLOCKED")
			return c.Status(fiber.StatusConflict).JSON(plan)
		} else if errors.Is(err, core.ErrAccountDeleted) {
			c.Set("X-Renop-Error-Code", "ACCOUNT_DELETED")
			return c.SendStatus(fiber.StatusConflict)
		} else if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to retire account")
		}
		audit.Log(state, &core.AuditLogEntry{
			Username: username, Operator: operator, Action: audit.ActionAccountRetire,
			Details: "Account retired by its owner", AuthMethod: authMethod, SessionID: sessionID, IP: ip,
			CreatedAt: retiredAt,
		})
		setSessionCookie(c, "", -1)
		return c.SendStatus(fiber.StatusNoContent)
	})
}
