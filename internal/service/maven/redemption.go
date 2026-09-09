/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/repositorygate"
)

func redeemDomain(c fiber.Ctx, state *core.AppState) error {
	user, session := auth.GetUser(c), auth.CurrentSessionToken(c)
	if user == nil || auth.CurrentCredentialKind(c) != "session" || session == "" || c.Cookies("renop_session") != session {
		return apiError(c, core.ErrMavenPermissionDenied)
	}
	domain, err := NormalizeDomain(c.Params("domain"))
	if err != nil {
		return apiError(c, fiber.ErrBadRequest)
	}
	now := time.Now()
	if err := state.GetDB().ReserveMavenRedemptionAttempt(domain, user.Username, session, now.UnixMilli(), now.Add(-verificationInterval).UnixMilli()); err != nil {
		return apiError(c, err)
	}
	details, err := state.GetDB().GetMavenDomainDetails(domain, user.Username)
	if err != nil {
		return apiError(c, err)
	}
	health, err := VerifyDomainProof(c.Context(), state.Inner.Config.Load(), details.Domain)
	if err != nil {
		if errors.Is(err, core.ErrMavenVerificationFailed) {
			return apiError(c, err)
		}
		return c.Status(fiber.StatusBadGateway).SendString("Maven verification provider is unavailable")
	}
	health.CheckedAt, health.NextCheckAt = time.Now().UnixMilli(), time.Now().Add(domainHealthInterval).UnixMilli()
	release := repositorygate.AcquireAllMigrations()
	err = state.GetDB().RedeemMavenDomain(details.Domain, health, user.Username, session)
	release()
	if err != nil {
		return apiError(c, err)
	}
	logAudit(c, state, audit.ActionResourceUnlock, "Redeemed publishing domain: "+domain)
	details, err = state.GetDB().GetMavenDomainDetails(domain, user.Username)
	if err != nil {
		return apiError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(details.Domain)
}
