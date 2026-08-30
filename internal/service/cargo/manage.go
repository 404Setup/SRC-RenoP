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
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/cargodocs"
	"renop/internal/utils"
)

func (h Handler) deleteVersion(c fiber.Ctx, state *core.AppState, repo *config.Repository, storagePath, crateName, version string) error {
	if err := validatePackage(crateName, version); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	_, details, err := authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionVersion)
	if err != nil {
		return cargoError(c, err)
	}
	if !hasCargoVersion(details, version) {
		return cargoError(c, core.ErrCargoVersionNotFound)
	}
	lockPath := cargoPackageLockPath(storagePath, repo, crateName)
	if !utils.IsSubPath(storagePath, lockPath) {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo package path")
	}
	release, err := acquireIndexLock(state, lockPath)
	if err != nil {
		return errorResponse(c, fiber.StatusServiceUnavailable, "Cargo registry state is unavailable")
	}
	succeeded := false
	defer func() { release(succeeded) }()
	_, details, err = authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionVersion)
	if err != nil {
		return cargoError(c, err)
	}
	if !hasCargoVersion(details, version) {
		return cargoError(c, core.ErrCargoVersionNotFound)
	}
	indexFilePath := cargoIndexPath(storagePath, repo, details.Package.Name)
	cratePath := filepath.Join(storagePath, repo.Name, "api", "v1", "crates", details.Package.Name, version, "download")
	if !utils.IsSubPath(storagePath, indexFilePath) || !utils.IsSubPath(storagePath, cratePath) {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo package path")
	}

	remaining, err := h.removeVersionFromIndex(state, indexFilePath, version)
	if err != nil {
		return cargoError(c, err)
	}
	if remaining == 0 {
		if err := h.Store.Delete(state, indexFilePath); err != nil {
			return errorResponse(c, fiber.StatusInternalServerError, "Failed to delete Cargo index")
		}
	}
	if err := h.Store.Delete(state, cratePath); err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to delete Cargo crate")
	}
	for _, cand := range candidateDocStoragePaths(storagePath, repo, details.Package.Name, version) {
		exists, err := h.Store.Exists(cand)
		if err == nil && exists {
			_ = h.Store.Delete(state, cand)
		}
	}
	cargodocs.CleanupCargodoc(repo.Name, details.Package.Name, version)
	if err := state.GetDB().DeleteCargoVersion(repo.Name, details.Package.NormalizedName, version); err != nil {
		return cargoError(c, err)
	}
	succeeded = true
	logCargoAudit(c, state, audit.ActionCargoVersionDelete, "Repository: "+repo.Name+", crate: "+details.Package.Name+", version: "+version)
	return c.JSON(OperationResponse{OK: true})
}

func (h Handler) removeVersionFromIndex(state *core.AppState, indexFilePath, version string) (int, error) {
	reader, found, err := h.Store.Open(indexFilePath)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, core.ErrCargoVersionNotFound
	}
	defer reader.Close()
	stage, err := h.Store.Stage(indexFilePath)
	if err != nil {
		return 0, err
	}
	defer stage.Discard()
	removed, remaining, rewriteErr := rewriteRemoveVersion(reader, stage, version)
	closeErr := reader.Close()
	if rewriteErr != nil {
		return 0, rewriteErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	if !removed {
		return 0, core.ErrCargoVersionNotFound
	}
	if remaining == 0 {
		return 0, nil
	}
	if err := stage.Close(); err != nil {
		return 0, err
	}
	if err := stage.Commit(state); err != nil {
		return 0, err
	}
	return remaining, nil
}

func (h Handler) setPackageArchived(c fiber.Ctx, state *core.AppState, repo *config.Repository, storagePath, crateName string, archived bool) error {
	if err := validateCrateName(crateName); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if _, _, err := authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionFull); err != nil {
		return cargoError(c, err)
	}
	lockPath := cargoPackageLockPath(storagePath, repo, crateName)
	if !utils.IsSubPath(storagePath, lockPath) {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo package path")
	}
	release, err := acquireIndexLock(state, lockPath)
	if err != nil {
		return errorResponse(c, fiber.StatusServiceUnavailable, "Cargo registry state is unavailable")
	}
	succeeded := false
	defer func() { release(succeeded) }()
	user, details, err := authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionFull)
	if err != nil {
		return cargoError(c, err)
	}
	if !archived && details.Package.AdminArchived && !user.IsManager() {
		return cargoError(c, core.ErrCargoAdminArchived)
	}
	if details.Package.Archived == archived && !(archived && user.IsManager() && !details.Package.AdminArchived) {
		succeeded = true
		return c.JSON(OperationResponse{OK: true})
	}
	indexFilePath := cargoIndexPath(storagePath, repo, details.Package.Name)
	if !utils.IsSubPath(storagePath, indexFilePath) {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo package path")
	}

	original := make(map[string]bool, len(details.Versions))
	desired := make(map[string]bool, len(details.Versions))
	for _, version := range details.Versions {
		if version == nil {
			continue
		}
		original[version.Version] = version.Yanked
		if archived {
			desired[version.Version] = true
		} else if version.ArchiveYanked && !version.AdminYanked {
			desired[version.Version] = false
		} else {
			desired[version.Version] = version.Yanked
		}
	}
	if len(desired) > 0 {
		if err := h.rewritePackageIndex(state, indexFilePath, desired); err != nil {
			return errorResponse(c, fiber.StatusInternalServerError, "Failed to update Cargo package index")
		}
	}
	if err := state.GetDB().SetCargoPackageArchived(repo.Name, details.Package.NormalizedName, archived, user.IsManager()); err != nil {
		if len(original) > 0 {
			if rollbackErr := h.rewritePackageIndex(state, indexFilePath, original); rollbackErr != nil {
				log.Printf("failed to roll back Cargo archive state %s/%s: %v", repo.Name, details.Package.Name, rollbackErr)
			}
		}
		return cargoError(c, err)
	}
	succeeded = true
	action := audit.ActionCargoPackageRestore
	if archived {
		action = audit.ActionCargoPackageArchive
	}
	logCargoAudit(c, state, action, "Repository: "+repo.Name+", crate: "+details.Package.Name)
	return c.JSON(OperationResponse{OK: true})
}

func (h Handler) rewritePackageIndex(state *core.AppState, indexFilePath string, desired map[string]bool) error {
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
	updated, rewriteErr := rewritePackageYanked(reader, stage, desired)
	closeErr := reader.Close()
	if rewriteErr != nil {
		return rewriteErr
	}
	if closeErr != nil {
		return closeErr
	}
	if updated != len(desired) {
		return fmt.Errorf("cargo index version count does not match package metadata")
	}
	if err := stage.Close(); err != nil {
		return err
	}
	return stage.Commit(state)
}

func (h Handler) deletePackage(c fiber.Ctx, state *core.AppState, repo *config.Repository, storagePath, crateName string) error {
	if err := validateCrateName(crateName); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if _, _, err := authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionFull); err != nil {
		return cargoError(c, err)
	}
	lockPath := cargoPackageLockPath(storagePath, repo, crateName)
	if !utils.IsSubPath(storagePath, lockPath) {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo package path")
	}
	release, err := acquireIndexLock(state, lockPath)
	if err != nil {
		return errorResponse(c, fiber.StatusServiceUnavailable, "Cargo registry state is unavailable")
	}
	succeeded := false
	defer func() { release(succeeded) }()
	_, details, err := authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionFull)
	if err != nil {
		return cargoError(c, err)
	}
	indexFilePath := cargoIndexPath(storagePath, repo, details.Package.Name)
	if !utils.IsSubPath(storagePath, indexFilePath) {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo package path")
	}

	for _, version := range details.Versions {
		if version == nil {
			continue
		}
		cratePath := filepath.Join(storagePath, repo.Name, "api", "v1", "crates", details.Package.Name, version.Version, "download")
		if !utils.IsSubPath(storagePath, cratePath) {
			return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo package path")
		}
		if err := h.Store.Delete(state, cratePath); err != nil {
			return errorResponse(c, fiber.StatusInternalServerError, "Failed to delete Cargo crate")
		}
		for _, cand := range candidateDocStoragePaths(storagePath, repo, details.Package.Name, version.Version) {
			exists, err := h.Store.Exists(cand)
			if err == nil && exists {
				_ = h.Store.Delete(state, cand)
			}
		}
		cargodocs.CleanupCargodoc(repo.Name, details.Package.Name, version.Version)
	}
	if err := h.Store.Delete(state, indexFilePath); err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to delete Cargo index")
	}
	if err := state.GetDB().DeleteCargoPackage(repo.Name, details.Package.NormalizedName, time.Now().UnixMilli()); err != nil {
		return cargoError(c, err)
	}
	succeeded = true
	logCargoAudit(c, state, audit.ActionCargoPackageDelete, "Repository: "+repo.Name+", crate: "+details.Package.Name)
	return c.JSON(OperationResponse{OK: true})
}

func hasCargoVersion(details *core.CargoPackageDetails, version string) bool {
	if details == nil {
		return false
	}
	for _, candidate := range details.Versions {
		if candidate != nil && candidate.Version == version {
			return true
		}
	}
	return false
}
