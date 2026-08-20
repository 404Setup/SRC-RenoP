/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

func requireAPINoCacheHeaders(t *testing.T, header http.Header) {
	t.Helper()
	require.Equal(t, apiNoCacheControl, header.Get(fiber.HeaderCacheControl))
	require.Equal(t, "no-cache", header.Get(fiber.HeaderPragma))
	require.Equal(t, "0", header.Get(fiber.HeaderExpires))
}

func TestIsAPIPath(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/api", "/api/", "/api/auth/me"} {
		require.True(t, isAPIPath(path), path)
	}
	for _, path := range []string{"/", "/assets/api.js", "/apiary"} {
		require.False(t, isAPIPath(path), path)
	}
}

func TestAPINoCacheMiddlewareOverridesRouteCaching(t *testing.T) {
	app := fiber.New()
	app.Use(APINoCacheMiddleware())
	app.Get("/api/privacy-policy", func(c fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
		return c.SendString("policy")
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/privacy-policy", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	requireAPINoCacheHeaders(t, resp.Header)
}

func TestAPINoCacheMiddlewareCoversEarlyAPIResponses(t *testing.T) {
	app := fiber.New()
	app.Use(APINoCacheMiddleware())
	app.Use(func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusUnauthorized)
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	requireAPINoCacheHeaders(t, resp.Header)
}

func TestAPINoCacheMiddlewareCoversFiberErrors(t *testing.T) {
	app := fiber.New()
	app.Use(APINoCacheMiddleware())
	app.Get("/api/failure", func(c fiber.Ctx) error {
		return fiber.ErrInternalServerError
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/failure", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	requireAPINoCacheHeaders(t, resp.Header)
}

func TestAPINoCacheMiddlewareLeavesNonAPIResponsesAlone(t *testing.T) {
	app := fiber.New()
	app.Use(APINoCacheMiddleware())
	app.Get("/assets/app.js", func(c fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
		return c.SendString("asset")
	})
	app.Get("/apiary", func(c fiber.Ctx) error {
		return c.SendString("not an API route")
	})

	assetResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	require.NoError(t, err)
	require.Equal(t, "public, max-age=31536000, immutable", assetResp.Header.Get(fiber.HeaderCacheControl))
	require.Empty(t, assetResp.Header.Get(fiber.HeaderPragma))
	require.Empty(t, assetResp.Header.Get(fiber.HeaderExpires))

	apiaryResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/apiary", nil))
	require.NoError(t, err)
	require.Empty(t, apiaryResp.Header.Get(fiber.HeaderCacheControl))
}
