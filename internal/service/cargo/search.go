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
	"strconv"
	"strings"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/service/cargodocs"
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
			Name: pkg.Name, MaxVersion: pkg.MaxVersion, Description: pkg.Description, Mirrored: pkg.Mirrored,
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

func (h Handler) packageInfo(c fiber.Ctx, state *core.AppState, repo *config.Repository, storagePath, crateName string) error {
	user := auth.GetUser(c)
	username := ""
	administrator := false
	if user != nil && user.Username != "" && !strings.EqualFold(user.Username, "guest") {
		username = user.Username
		administrator = user.IsManager()
	}
	details, err := packageDetails(state, repo.Name, crateName, username)
	if errors.Is(err, core.ErrCargoPackageNotFound) && username != "" {
		normalizedName, valid := NormalizeCrateName(crateName)
		if valid {
			details, err = PendingPublicationPackageDetails(state, repo.Name, normalizedName,
				username, administrator || user.CheckModeratePermission(repo.Name))
		}
	}
	if err != nil {
		return cargoError(c, err)
	}
	if !administrator && details.Package.PermissionLevel == 0 {
		// Package metadata and versions are registry-readable. Team membership is
		// reserved for collaborators and administrators.
		details.Members = []*core.CargoMember{}
	}
	if administrator || user != nil && (user.CheckModeratePermission(repo.Name) ||
		details.Package.PermissionLevel >= core.CargoPermissionPublish) {
		if err := AddPendingPublicationVersions(state, details); err != nil {
			return cargoError(c, err)
		}
	}

	h.enrichPackageVersionsFromIndex(state, repo, storagePath, details)

	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(packageInfoResponse{CargoPackageDetails: details, Admin: administrator})
}

func (h Handler) enrichPackageVersionsFromIndex(state *core.AppState, repo *config.Repository, storagePath string, details *core.CargoPackageDetails) {
	if details == nil || details.Package == nil {
		return
	}
	for _, v := range details.Versions {
		if v != nil {
			v.HasDocs = cargodocs.HasCargodoc(state, repo.Name, details.Package.Name, v.Version)
		}
	}
	if h.Store == nil {
		return
	}
	indexFilePath := cargoIndexPath(storagePath, repo, details.Package.Name)
	reader, found, err := h.Store.Open(indexFilePath)
	if err != nil || !found || reader == nil {
		return
	}
	defer reader.Close()

	indexEntries := make(map[string]IndexEntry)
	_ = scanIndex(reader, func(line []byte) error {
		var entry IndexEntry
		if json.Unmarshal(line, &entry) == nil && entry.Version != "" {
			indexEntries[entry.Version] = entry
		}
		return nil
	})

	for _, v := range details.Versions {
		if v == nil {
			continue
		}
		if entry, ok := indexEntries[v.Version]; ok {
			if v.Checksum == "" {
				v.Checksum = entry.Checksum
			}
			if v.RustVersion == "" && entry.RustVersion != nil {
				v.RustVersion = *entry.RustVersion
			}
			if v.Links == nil {
				v.Links = entry.Links
			}
			if v.Features == nil {
				v.Features = entry.Features
			}
			if len(v.Deps) == 0 && len(entry.Deps) > 0 {
				deps := make([]core.CargoDependency, 0, len(entry.Deps))
				for _, dep := range entry.Deps {
					deps = append(deps, core.CargoDependency{
						Name:            dep.Name,
						Requirement:     dep.Requirement,
						Features:        dep.Features,
						Optional:        dep.Optional,
						DefaultFeatures: dep.DefaultFeatures,
						Target:          dep.Target,
						Kind:            dep.Kind,
						Registry:        dep.Registry,
						Package:         dep.Package,
					})
				}
				v.Deps = deps
			}
		}
	}
}
