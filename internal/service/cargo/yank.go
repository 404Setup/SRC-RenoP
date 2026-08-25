/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/utils"
)

func (h Handler) setYanked(c fiber.Ctx, state *core.AppState, repo *config.Repository, storagePath, crateName, version string, yanked bool) error {
	if err := validatePackage(crateName, version); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if _, _, err := authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionVersion, true); err != nil {
		return cargoError(c, err)
	}
	lockPath := cargoPackageLockPath(storagePath, repo, crateName)
	if !utils.IsSubPath(storagePath, lockPath) {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo package path")
	}
	_, release, err := acquireIndexLock(state, lockPath)
	if err != nil {
		return errorResponse(c, fiber.StatusServiceUnavailable, "Cargo registry state is unavailable")
	}
	succeeded := false
	defer func() { release(succeeded) }()
	user, details, err := authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionVersion, true)
	if err != nil {
		return cargoError(c, err)
	}
	var versionRecord *core.CargoVersion
	for _, candidate := range details.Versions {
		if candidate != nil && candidate.Version == version {
			versionRecord = candidate
			break
		}
	}
	if versionRecord == nil {
		return cargoError(c, core.ErrCargoVersionNotFound)
	}
	if !yanked && details.Package.Archived {
		return cargoError(c, core.ErrCargoPackageArchived)
	}
	if !yanked && versionRecord.AdminYanked && !user.IsManager() {
		return cargoError(c, core.ErrCargoAdminYanked)
	}
	indexFilePath := cargoIndexPath(storagePath, repo, details.Package.Name)
	if !utils.IsSubPath(storagePath, indexFilePath) {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo package path")
	}

	indexReader, found, err := h.Store.Open(indexFilePath)
	if err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to read Cargo index")
	}
	if !found {
		return errorResponse(c, fiber.StatusNotFound, "Crate was not found")
	}
	defer indexReader.Close()
	indexStage, err := h.Store.Stage(indexFilePath)
	if err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to prepare Cargo index")
	}
	defer indexStage.Discard()
	versionFound, rewriteErr := rewriteYanked(indexReader, indexStage, version, yanked)
	closeErr := indexReader.Close()
	if rewriteErr == nil {
		rewriteErr = closeErr
	}
	if rewriteErr != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to update Cargo index")
	}
	if !versionFound {
		return errorResponse(c, fiber.StatusNotFound, "Crate version was not found")
	}
	if err := indexStage.Close(); err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to stage Cargo index")
	}
	if err := indexStage.Commit(state); err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to store Cargo index")
	}
	if err := state.GetDB().SetCargoVersionYanked(repo.Name, details.Package.NormalizedName, version, yanked, user.IsManager()); err != nil {
		if rollbackErr := h.rewriteSingleYank(state, indexFilePath, version, versionRecord.Yanked); rollbackErr != nil {
			return errorResponse(c, fiber.StatusInternalServerError, "Failed to update Cargo metadata and roll back the index")
		}
		return cargoError(c, err)
	}
	succeeded = true
	action := "CARGO_UNYANK"
	if yanked {
		action = "CARGO_YANK"
	}
	logCargoAudit(c, state, action, "Repository: "+repo.Name+", crate: "+details.Package.Name+", version: "+version)
	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.JSON(OperationResponse{OK: true})
}

func (h Handler) rewriteSingleYank(state *core.AppState, indexFilePath, version string, yanked bool) error {
	reader, found, err := h.Store.Open(indexFilePath)
	if err != nil {
		return err
	}
	if !found {
		return core.ErrCargoPackageNotFound
	}
	defer reader.Close()
	stage, err := h.Store.Stage(indexFilePath)
	if err != nil {
		return err
	}
	defer stage.Discard()
	versionFound, err := rewriteYanked(reader, stage, version, yanked)
	closeErr := reader.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if !versionFound {
		return core.ErrCargoVersionNotFound
	}
	if err := stage.Close(); err != nil {
		return err
	}
	return stage.Commit(state)
}
