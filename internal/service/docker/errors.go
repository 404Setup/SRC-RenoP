/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package docker

import (
	"github.com/gofiber/fiber/v3"
)

const (
	ErrCodeUnauthorized       = "UNAUTHORIZED"
	ErrCodeDenied             = "DENIED"
	ErrCodeNameUnknown        = "NAME_UNKNOWN"
	ErrCodeManifestUnknown    = "MANIFEST_UNKNOWN"
	ErrCodeManifestInvalid    = "MANIFEST_INVALID"
	ErrCodeManifestUnverified = "MANIFEST_UNVERIFIED"
	ErrCodeBlobUnknown        = "BLOB_UNKNOWN"
	ErrCodeBlobUploadUnknown  = "BLOB_UPLOAD_UNKNOWN"
	ErrCodeBlobUploadInvalid  = "BLOB_UPLOAD_INVALID"
	ErrCodeDigestInvalid      = "DIGEST_INVALID"
	ErrCodeSizeInvalid        = "SIZE_INVALID"
	ErrCodeTagInvalid         = "TAG_INVALID"
	ErrCodeUnsupported        = "UNSUPPORTED"
)

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  any    `json:"detail,omitempty"`
}

type ErrorResponse struct {
	Errors []ErrorDetail `json:"errors"`
}

// RespondError writes a standard OCI/Docker JSON error response with the Docker version header.
func RespondError(c fiber.Ctx, statusCode int, code, message string, detail any) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(statusCode).JSON(ErrorResponse{
		Errors: []ErrorDetail{
			{
				Code:    code,
				Message: message,
				Detail:  detail,
			},
		},
	})
}
