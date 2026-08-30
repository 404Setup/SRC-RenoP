/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package updater

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
)

const (
	// APIErrorCodeHeader carries a stable updater error identifier for localized clients.
	APIErrorCodeHeader = "X-Renop-Error-Code"
	// APIErrorForbidden indicates that updater administration is not permitted.
	APIErrorForbidden = "forbidden"
	// APIErrorInsufficientSpace indicates that update staging cannot fit on disk.
	APIErrorInsufficientSpace = "insufficient_space"
	// APIErrorMissingFile indicates that an offline package was not supplied.
	APIErrorMissingFile = "missing_file"
	// APIErrorInstallBusy indicates that another update installation owns the global install gate.
	APIErrorInstallBusy = "install_busy"
	// APIErrorInvalidPackage indicates malformed or incomplete package contents.
	APIErrorInvalidPackage = "invalid_package"
	// APIErrorIncompatibleBinary indicates that the package targets another platform.
	APIErrorIncompatibleBinary = "incompatible_binary"
	// APIErrorPackageTooLarge indicates that a package or executable exceeded its streaming limit.
	APIErrorPackageTooLarge = "package_too_large"
	// APIErrorPackageProcessing indicates an internal package staging failure.
	APIErrorPackageProcessing = "package_processing_failed"
	// APIErrorCheckFailed indicates that the update channel could not be checked.
	APIErrorCheckFailed = "check_failed"
	// APIErrorNotificationFailed indicates that a durable update result could not be delivered.
	APIErrorNotificationFailed = "notification_failed"
	// APIErrorRestartFailed indicates that the process could not restart.
	APIErrorRestartFailed = "restart_failed"
)

// WriteAPIError sends a stable updater error code and a non-sensitive fallback message.
func WriteAPIError(c fiber.Ctx, status int, code, message string) error {
	c.Set(APIErrorCodeHeader, code)
	return c.Status(status).JSON(fiber.Map{"error": message})
}

// PackageAPIError classifies an offline package error without exposing filesystem details.
func PackageAPIError(err error) (status int, code, message string) {
	if err == nil {
		return fiber.StatusInternalServerError, APIErrorPackageProcessing, "Update package could not be processed"
	}
	if errors.Is(err, ErrIncompatibleBinary) {
		return fiber.StatusBadRequest, APIErrorIncompatibleBinary, "Update package is incompatible with this server"
	}
	errorText := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(errorText, "exceeds"):
		return fiber.StatusRequestEntityTooLarge, APIErrorPackageTooLarge, "Update package exceeds the permitted size"
	case strings.Contains(errorText, "invalid"), strings.Contains(errorText, "missing"),
		strings.Contains(errorText, "not found"), strings.Contains(errorText, "decompress"),
		strings.Contains(errorText, "zip"), strings.Contains(errorText, "brotli"):
		return fiber.StatusBadRequest, APIErrorInvalidPackage, "Update package is invalid"
	default:
		return fiber.StatusInternalServerError, APIErrorPackageProcessing, "Update package could not be processed"
	}
}

// WritePackageAPIError sends a classified offline package error.
func WritePackageAPIError(c fiber.Ctx, err error) error {
	status, code, message := PackageAPIError(err)
	return WriteAPIError(c, status, code, message)
}
