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
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/cargodocs"
	"renop/internal/service/status"
	"renop/internal/utils"
)

const (
	maxDocArchiveSize = 128 << 20
)

type DocStatusResponse struct {
	HasDocs bool   `json:"has_docs"`
	DocURL  string `json:"doc_url,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

type DocOperationResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	DocURL  string `json:"doc_url,omitempty"`
}

func cargoDocStoragePath(storagePath string, repo *config.Repository, crateName, version string, isZip bool) string {
	ext := ".tar.gz"
	if isZip {
		ext = ".zip"
	}
	return filepath.Join(storagePath, repo.Name, "crates", crateName, crateName+"-"+version+"-docs"+ext)
}

func candidateDocStoragePaths(storagePath string, repo *config.Repository, crateName, version string) []string {
	normalizedName := normalizeCrateName(crateName)
	return []string{
		filepath.Join(storagePath, repo.Name, "crates", crateName, crateName+"-"+version+"-docs.tar.gz"),
		filepath.Join(storagePath, repo.Name, "crates", crateName, crateName+"-"+version+"-docs.zip"),
		filepath.Join(storagePath, repo.Name, "crates", normalizedName, normalizedName+"-"+version+"-docs.tar.gz"),
		filepath.Join(storagePath, repo.Name, "crates", normalizedName, normalizedName+"-"+version+"-docs.zip"),
		filepath.Join(storagePath, repo.Name, "api", "v1", "crates", crateName, version, "docs.tar.gz"),
		filepath.Join(storagePath, repo.Name, "api", "v1", "crates", crateName, version, "docs.zip"),
	}
}

func (h Handler) getDocStatus(c fiber.Ctx, state *core.AppState, repo *config.Repository, storagePath, crateName, version string) error {
	if err := validatePackage(crateName, version); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, err.Error())
	}

	user := auth.GetUser(c)
	canRead, err := CanReadRepository(state, user, repo, "", true)
	if err != nil || !canRead {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	candidates := candidateDocStoragePaths(storagePath, repo, crateName, version)
	for _, cand := range candidates {
		exists, err := h.Store.Exists(cand)
		if err == nil && exists {
			docURL := fmt.Sprintf("/cargodoc/%s/%s/%s/", repo.Name, crateName, version)
			return c.JSON(DocStatusResponse{
				HasDocs: true,
				DocURL:  docURL,
			})
		}
	}

	return c.JSON(DocStatusResponse{
		HasDocs: false,
	})
}

func (h Handler) uploadDocs(c fiber.Ctx, state *core.AppState, repo *config.Repository, storagePath, crateName, version string) error {
	if err := validatePackage(crateName, version); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, err.Error())
	}

	user, details, err := authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionPublish)
	if err != nil {
		return cargoError(c, err)
	}
	if !hasCargoVersion(details, version) {
		return cargoError(c, core.ErrCargoVersionNotFound)
	}

	contentLength := c.Request().Header.ContentLength()
	if contentLength > maxDocArchiveSize {
		return errorResponse(c, fiber.StatusBadRequest, "Documentation archive exceeds size limit")
	}
	if !status.CanAllocateDiskSpace(state, uint64(max(contentLength, 10*1024*1024))) {
		return errorResponse(c, fiber.StatusInsufficientStorage, "Insufficient disk space to upload documentation")
	}

	var header [4]byte
	bodyReader := publishBodyReader(c)
	headerLen, err := io.ReadFull(bodyReader, header[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return errorResponse(c, fiber.StatusBadRequest, "Failed to read documentation archive header")
	}
	if headerLen < 2 {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid or empty documentation archive")
	}

	isZip := headerLen >= 2 && header[0] == 0x50 && header[1] == 0x4b
	isGz := header[0] == 0x1f && header[1] == 0x8b
	if !isZip && !isGz {
		return errorResponse(c, fiber.StatusBadRequest, "Unsupported documentation archive format (expected .tar.gz or .zip)")
	}

	docPath := cargoDocStoragePath(storagePath, repo, details.Package.Name, version, isZip)
	if !utils.IsSubPath(storagePath, docPath) {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo doc path")
	}

	staged, err := h.Store.Stage(docPath)
	if err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to prepare storage for documentation")
	}
	defer staged.Discard()

	combinedReader := io.MultiReader(bytes.NewReader(header[:headerLen]), bodyReader)
	limitedReader := io.LimitReader(combinedReader, maxDocArchiveSize+1)

	if isZip {
		tempRaw, err := os.CreateTemp("", "renop-cargo-raw-doc-*.zip")
		if err != nil {
			return errorResponse(c, fiber.StatusInternalServerError, "Failed to prepare temporary storage for documentation")
		}
		tempRawPath := tempRaw.Name()
		defer func() {
			_ = tempRaw.Close()
			_ = os.Remove(tempRawPath)
		}()

		written, copyErr := io.Copy(tempRaw, limitedReader)
		if copyErr != nil {
			return errorResponse(c, fiber.StatusBadRequest, "Failed to stream documentation archive")
		}
		if written > maxDocArchiveSize {
			return errorResponse(c, fiber.StatusBadRequest, "Documentation archive exceeds size limit")
		}
		if err := cargodocs.SanitizeZipDocArchive(tempRaw, written, details.Package.Name, staged); err != nil {
			return errorResponse(c, fiber.StatusBadRequest, err.Error())
		}
	} else {
		if err := cargodocs.SanitizeTarGzDocArchive(limitedReader, details.Package.Name, staged); err != nil {
			return errorResponse(c, fiber.StatusBadRequest, err.Error())
		}
	}

	if err := staged.Commit(state); err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to commit documentation archive")
	}

	oppositeExt := ".zip"
	if isZip {
		oppositeExt = ".tar.gz"
	}
	oppositePath := filepath.Join(storagePath, repo.Name, "crates", details.Package.Name, details.Package.Name+"-"+version+"-docs"+oppositeExt)
	_ = h.Store.Delete(state, oppositePath)

	cargodocs.CleanupCargodoc(repo.Name, details.Package.Name, version)
	logCargoAudit(c, state, audit.ActionCargoDocsUpload, fmt.Sprintf("Repository: %s, crate: %s, version: %s by %s", repo.Name, details.Package.Name, version, user.Username))

	docURL := fmt.Sprintf("/cargodoc/%s/%s/%s/", repo.Name, details.Package.Name, version)
	return c.Status(fiber.StatusOK).JSON(DocOperationResponse{
		OK:      true,
		Message: "Documentation uploaded successfully",
		DocURL:  docURL,
	})
}

func (h Handler) deleteDocs(c fiber.Ctx, state *core.AppState, repo *config.Repository, storagePath, crateName, version string) error {
	if err := validatePackage(crateName, version); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, err.Error())
	}

	user, details, err := authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionPublish)
	if err != nil {
		return cargoError(c, err)
	}

	candidates := candidateDocStoragePaths(storagePath, repo, details.Package.Name, version)
	for _, cand := range candidates {
		exists, err := h.Store.Exists(cand)
		if err == nil && exists {
			_ = h.Store.Delete(state, cand)
		}
	}

	cargodocs.CleanupCargodoc(repo.Name, details.Package.Name, version)
	logCargoAudit(c, state, audit.ActionCargoDocsDelete, fmt.Sprintf("Repository: %s, crate: %s, version: %s by %s", repo.Name, details.Package.Name, version, user.Username))

	return c.JSON(DocOperationResponse{
		OK:      true,
		Message: "Documentation deleted successfully",
	})
}
