/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package api

import (
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
	"testing"
)

func TestConfiguredLegalDocumentsReplaceFilePolicyAndReload(t *testing.T) {
	cfg := config.DefaultConfig()
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	app := fiber.New()
	app.Use(auth.AuthMiddleware(state))
	SetupAPIRoutes(app.Group("/api"), state)
	for _, path := range []string{"/api/privacy-policy", "/api/legal/privacy-policy", "/api/legal/terms-of-service", "/api/legal/legal-notice"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set(fiber.HeaderAuthorization, "Bearer expired")
		response, err := app.Test(request)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Equal(t, "text/plain; charset=utf-8", response.Header.Get(fiber.HeaderContentType))
		require.Equal(t, "no-store", response.Header.Get(fiber.HeaderCacheControl))
		require.NoError(t, response.Body.Close())
	}
	next := cfg.DeepCopy()
	next.Legal.PrivacyPolicy = "# Updated policy\n\n<script>inert text</script>"
	require.NoError(t, next.Legal.Normalize())
	state.Inner.Config.Store(next)
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/privacy-policy", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, next.Legal.PrivacyPolicy, string(body))
	require.Equal(t, "nosniff", response.Header.Get(fiber.HeaderXContentTypeOptions))
}
