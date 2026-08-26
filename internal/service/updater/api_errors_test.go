/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package updater

import (
	"errors"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

func TestPackageAPIErrorsExposeStableNonSensitiveCodes(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "incompatible", err: ErrIncompatibleBinary, wantStatus: fiber.StatusBadRequest, wantCode: APIErrorIncompatibleBinary},
		{name: "too large", err: errors.New("update package exceeds 10 bytes"), wantStatus: fiber.StatusRequestEntityTooLarge, wantCode: APIErrorPackageTooLarge},
		{name: "invalid", err: errors.New("invalid zip file: private path"), wantStatus: fiber.StatusBadRequest, wantCode: APIErrorInvalidPackage},
		{name: "internal", err: errors.New("failed to open C:\\private\\update.br"), wantStatus: fiber.StatusInternalServerError, wantCode: APIErrorPackageProcessing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, code, message := PackageAPIError(test.err)
			require.Equal(t, test.wantStatus, status)
			require.Equal(t, test.wantCode, code)
			require.NotContains(t, message, "private")
		})
	}
}

func TestWriteAPIErrorReturnsStableHeaderAndPublicBody(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		return WriteAPIError(c, fiber.StatusBadRequest, APIErrorInvalidPackage, "Update package is invalid")
	})
	response, err := app.Test(httptest.NewRequest("GET", "/", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, APIErrorInvalidPackage, response.Header.Get(APIErrorCodeHeader))
	require.NotContains(t, string(body), "private")
}
