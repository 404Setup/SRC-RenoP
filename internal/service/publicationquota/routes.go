/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package publicationquota

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
)

const (
	maxQuotaFileLimit        = int64(10_000_000)
	maxQuotaByteLimit        = int64(1 << 50)
	maxQuotaPublicationLimit = int64(1_000_000)
)

func quotaAPIError(c fiber.Ctx, err error) error {
	status, code := fiber.StatusInternalServerError, "database_unavailable"
	switch {
	case errors.Is(err, fiber.ErrUnauthorized):
		status, code = fiber.StatusUnauthorized, "authentication_required"
	case errors.Is(err, core.ErrUserProfileNotFound), errors.Is(err, core.ErrSuperTeamNotFound):
		status, code = fiber.StatusNotFound, "owner_not_found"
	case errors.Is(err, core.ErrSuperTeamPermissionDenied):
		status, code = fiber.StatusForbidden, "permission_denied"
	case errors.Is(err, core.ErrPublicationQuotaInvalid), errors.Is(err, fiber.ErrBadRequest):
		status, code = fiber.StatusBadRequest, "invalid_quota"
	case errors.Is(err, core.ErrDatabaseUnavailable):
		status, code = fiber.StatusServiceUnavailable, "database_unavailable"
	}
	c.Set("X-Renop-Error-Code", code)
	return c.Status(status).JSON(fiber.Map{"error": code})
}

func authenticatedQuotaUser(c fiber.Ctx) (*config.User, error) {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return nil, fiber.ErrUnauthorized
	}
	return user, nil
}

func validateQuotaOverride(override core.PublicationQuotaOverride) bool {
	if override.FileLimit != nil && (*override.FileLimit < 0 || *override.FileLimit > maxQuotaFileLimit) {
		return false
	}
	if override.ByteLimit != nil && (*override.ByteLimit < 0 || *override.ByteLimit > maxQuotaByteLimit) {
		return false
	}
	if override.PublicationLimit != nil &&
		(*override.PublicationLimit < 0 || *override.PublicationLimit > maxQuotaPublicationLimit) {
		return false
	}
	return override.Period == nil || core.ValidPublicationQuotaPeriod(strings.ToLower(strings.TrimSpace(*override.Period)))
}

func quotaSubjectFromRoute(c fiber.Ctx, ownerType string) core.PublicationQuotaSubject {
	key := c.Params("username")
	if ownerType == core.PublicationQuotaOwnerSuperTeam {
		key = c.Params("prefix")
	}
	return core.PublicationQuotaSubject{OwnerType: ownerType, OwnerKey: strings.ToLower(strings.TrimSpace(key))}
}

func getOwnQuota(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticatedQuotaUser(c)
	if err != nil {
		return quotaAPIError(c, err)
	}
	status, err := Status(state, Subject(user.Username, ""))
	if err != nil {
		return quotaAPIError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(status)
}

func getOwnerQuota(c fiber.Ctx, state *core.AppState, ownerType string) error {
	user, err := authenticatedQuotaUser(c)
	if err != nil {
		return quotaAPIError(c, err)
	}
	if state == nil || state.GetDB() == nil {
		return quotaAPIError(c, core.ErrDatabaseUnavailable)
	}
	subject := quotaSubjectFromRoute(c, ownerType)
	if ownerType == core.PublicationQuotaOwnerUser {
		if !user.IsManager() && !strings.EqualFold(user.Username, subject.OwnerKey) {
			return quotaAPIError(c, core.ErrSuperTeamPermissionDenied)
		}
	} else if !user.IsManager() {
		if _, err := state.GetDB().GetSuperTeamDetails(subject.OwnerKey, user.Username, false); err != nil {
			return quotaAPIError(c, core.ErrSuperTeamPermissionDenied)
		}
	}
	status, err := Status(state, subject)
	if err != nil {
		return quotaAPIError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(status)
}

func putOwnerQuota(c fiber.Ctx, state *core.AppState, ownerType string) error {
	user, err := authenticatedQuotaUser(c)
	if err != nil {
		return quotaAPIError(c, err)
	}
	if state == nil || state.GetDB() == nil {
		return quotaAPIError(c, core.ErrDatabaseUnavailable)
	}
	if !user.IsManager() {
		return quotaAPIError(c, core.ErrSuperTeamPermissionDenied)
	}
	if len(c.Body()) > 4096 {
		return c.SendStatus(fiber.StatusRequestEntityTooLarge)
	}
	var override core.PublicationQuotaOverride
	if err := c.Bind().Body(&override); err != nil || !validateQuotaOverride(override) {
		return quotaAPIError(c, core.ErrPublicationQuotaInvalid)
	}
	subject := quotaSubjectFromRoute(c, ownerType)
	if err := state.GetDB().SetPublicationQuotaOverride(subject, override, time.Now().UnixMilli()); err != nil {
		return quotaAPIError(c, err)
	}
	username, operator, method, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, AuthMethod: method, SessionID: sessionID, IP: ip,
		Action:    audit.ActionPublicationQuotaUpdate,
		Details:   "Owner type: " + ownerType + ", owner: " + subject.OwnerKey,
		CreatedAt: time.Now().UnixMilli(),
	})
	return getOwnerQuota(c, state, ownerType)
}

// SetupRoutes registers own, account, and global-team publication quota APIs.
func SetupRoutes(router fiber.Router, state *core.AppState) {
	base := router.Group("/publication-quota")
	base.Get("", func(c fiber.Ctx) error { return getOwnQuota(c, state) })
	base.Get("/users/:username", func(c fiber.Ctx) error {
		return getOwnerQuota(c, state, core.PublicationQuotaOwnerUser)
	})
	base.Put("/users/:username", func(c fiber.Ctx) error {
		return putOwnerQuota(c, state, core.PublicationQuotaOwnerUser)
	})
	base.Get("/super-teams/:prefix", func(c fiber.Ctx) error {
		return getOwnerQuota(c, state, core.PublicationQuotaOwnerSuperTeam)
	})
	base.Put("/super-teams/:prefix", func(c fiber.Ctx) error {
		return putOwnerQuota(c, state, core.PublicationQuotaOwnerSuperTeam)
	})
}
