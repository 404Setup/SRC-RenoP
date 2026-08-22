/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
)

func (h Handler) search(c fiber.Ctx, state *core.AppState, repo *config.Repository) error {
	query := strings.TrimSpace(c.Query("q"))
	if len(query) > 128 {
		return errorResponse(c, fiber.StatusBadRequest, "Cargo search query is too long")
	}
	perPage, err := strconv.Atoi(c.Query("per_page", "10"))
	if err != nil || perPage < 1 || perPage > 100 {
		return errorResponse(c, fiber.StatusBadRequest, "Cargo search page size must be between 1 and 100")
	}
	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || page < 1 || page > 10000 {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo search page")
	}
	db := state.GetDB()
	if db == nil {
		return cargoError(c, core.ErrDatabaseUnavailable)
	}
	packages, total, err := db.SearchCargoPackages(repo.Name, query, perPage, (page-1)*perPage)
	if err != nil {
		return cargoError(c, err)
	}
	results := make([]searchCrate, 0, len(packages))
	for _, pkg := range packages {
		if pkg == nil {
			continue
		}
		results = append(results, searchCrate{
			Name: pkg.Name, MaxVersion: pkg.MaxVersion, Description: pkg.Description,
		})
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(searchResponse{Crates: results, Meta: searchMeta{Total: total}})
}

func (h Handler) listManagedPackages(c fiber.Ctx, state *core.AppState, repo *config.Repository) error {
	user, err := authenticatedUser(c)
	if err != nil {
		return cargoError(c, err)
	}
	db := state.GetDB()
	if db == nil {
		return cargoError(c, core.ErrDatabaseUnavailable)
	}
	packages, err := db.ListCargoPackages(repo.Name, user.Username, user.IsManager())
	if err != nil {
		return cargoError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(packageListResponse{Packages: packages, Admin: user.IsManager()})
}

func (h Handler) packageInfo(c fiber.Ctx, state *core.AppState, repo *config.Repository, crateName string) error {
	user := auth.GetUser(c)
	username := ""
	administrator := false
	if user != nil && user.Username != "" && !strings.EqualFold(user.Username, "guest") {
		username = user.Username
		administrator = user.IsManager()
	}
	details, err := packageDetails(state, repo.Name, crateName, username)
	if err != nil {
		return cargoError(c, err)
	}
	if !administrator && details.Package.PermissionLevel == 0 {
		// Package metadata and versions are registry-readable. Team membership is
		// reserved for collaborators and administrators.
		details.Members = []*core.CargoMember{}
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(packageInfoResponse{CargoPackageDetails: details, Admin: administrator})
}
