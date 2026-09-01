/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
)

func authenticatedUser(c fiber.Ctx) (*config.User, error) {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return nil, core.ErrCargoPermissionDenied
	}
	return user, nil
}

// CanReadRepository applies the normal repository visibility rules and lets
// Cargo package membership grant read access to the rest of a private Cargo
// registry, which is required to resolve package dependencies.
func CanReadRepository(state *core.AppState, user *config.User, repo *config.Repository, path string, isRoot bool) (bool, error) {
	if repo == nil {
		return false, nil
	}
	if strings.EqualFold(repo.Visibility, "PUBLIC") {
		return true, nil
	}
	if user != nil && user.CheckReadPermission(repo.Name, path, repo.Visibility, isRoot) {
		return true, nil
	}
	if repo.NormalizedFormat() != config.RepositoryFormatCargo || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return false, nil
	}
	if state == nil {
		return false, core.ErrDatabaseUnavailable
	}
	db := state.GetDB()
	if db == nil {
		return false, core.ErrDatabaseUnavailable
	}
	allowed, err := db.HasCargoMembership(repo.Name, user.Username)
	if err != nil {
		return false, errors.Join(core.ErrDatabaseUnavailable, err)
	}
	return allowed, nil
}

func packageDetails(state *core.AppState, repository, crateName, username string) (*core.CargoPackageDetails, error) {
	if err := validateCrateName(crateName); err != nil {
		return nil, err
	}
	db := state.GetDB()
	if db == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	details, err := db.GetCargoPackageDetails(repository, normalizeCrateName(crateName), username)
	if err != nil {
		return nil, err
	}
	if details == nil || details.Package == nil {
		return nil, core.ErrCargoPackageNotFound
	}
	deprecated, err := db.IsPackageDeprecated(config.RepositoryFormatCargo, repository,
		details.Package.NormalizedName)
	if err != nil {
		return nil, err
	}
	details.Package.Deprecated = deprecated
	return details, nil
}

func authorizePackageMutation(c fiber.Ctx, state *core.AppState, repository, crateName string, level int) (*config.User, *core.CargoPackageDetails, error) {
	user, err := authenticatedUser(c)
	if err != nil {
		return nil, nil, err
	}
	details, err := packageDetails(state, repository, crateName, user.Username)
	if err != nil {
		return nil, nil, err
	}
	if user.IsManager() {
		return user, details, nil
	}
	if details.Package.PermissionLevel < level {
		return nil, nil, core.ErrCargoPermissionDenied
	}
	return user, details, nil
}

func cargoError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, core.ErrCargoPermissionDenied):
		return errorResponse(c, fiber.StatusForbidden, "You do not have permission to perform this Cargo operation")
	case errors.Is(err, core.ErrCargoPackageNotFound):
		return errorResponse(c, fiber.StatusNotFound, "Crate was not found")
	case errors.Is(err, core.ErrCargoVersionNotFound):
		return errorResponse(c, fiber.StatusNotFound, "Crate version was not found")
	case errors.Is(err, core.ErrCargoVersionExists):
		return errorResponse(c, fiber.StatusConflict, errVersionExists.Error())
	case errors.Is(err, core.ErrCargoPackageArchived):
		return errorResponse(c, fiber.StatusConflict, "Crate is archived")
	case errors.Is(err, core.ErrPackageDeprecated):
		c.Set("X-Renop-Error-Code", "package_deprecated")
		return errorResponse(c, fiber.StatusConflict, "Crate is permanently deprecated and read-only")
	case errors.Is(err, core.ErrPackageDeprecationPending):
		return errorResponse(c, fiber.StatusConflict, "Resolve pending reviews before deprecating this crate")
	case errors.Is(err, core.ErrCargoAdminArchived):
		return errorResponse(c, fiber.StatusForbidden, "Only an administrator can restore this crate")
	case errors.Is(err, core.ErrCargoAdminYanked):
		return errorResponse(c, fiber.StatusForbidden, "Only an administrator can restore this crate version")
	case errors.Is(err, core.ErrCargoLastFullMember), errors.Is(err, core.ErrCargoOwnerCannotLeave):
		return errorResponse(c, fiber.StatusConflict, err.Error())
	case errors.Is(err, core.ErrCargoMemberExists), errors.Is(err, core.ErrCargoInvitationExists):
		return errorResponse(c, fiber.StatusConflict, err.Error())
	case errors.Is(err, core.ErrCargoInvitationInvalid):
		return errorResponse(c, fiber.StatusConflict, err.Error())
	case errors.Is(err, core.ErrDatabaseUnavailable):
		return errorResponse(c, fiber.StatusServiceUnavailable, "Cargo registry metadata is unavailable")
	default:
		return errorResponse(c, fiber.StatusInternalServerError, "Cargo registry operation failed")
	}
}
