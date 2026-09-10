/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package legal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
)

func TestConsentRejectsMissingAndStalePolicies(t *testing.T) {
	cfg := config.DefaultConfig()
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	app := fiber.New()
	app.Post("/", func(c fiber.Ctx) error {
		if err := RequireConsent(c, state); err != nil {
			return err
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	for _, revision := range []string{"", "stale", cfg.Legal.Revision()} {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.Header.Set(ConsentHeader, revision)
		response, err := app.Test(request)
		require.NoError(t, err)
		if revision == cfg.Legal.Revision() {
			require.Equal(t, http.StatusNoContent, response.StatusCode)
		} else {
			require.Equal(t, http.StatusPreconditionRequired, response.StatusCode)
			require.Equal(t, ConsentErrorCode, response.Header.Get("X-Renop-Error-Code"))
		}
		require.NoError(t, response.Body.Close())
	}
	oldRevision := cfg.Legal.Revision()
	next := cfg.DeepCopy()
	next.Legal.TermsOfService = "# New terms"
	require.NoError(t, next.Legal.Normalize())
	state.Inner.Config.Store(next)
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.AddCookie(&http.Cookie{Name: ConsentCookie, Value: oldRevision})
	response, err := app.Test(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusPreconditionRequired, response.StatusCode)
	require.NoError(t, response.Body.Close())
	request = httptest.NewRequest(http.MethodPost, "/", nil)
	request.AddCookie(&http.Cookie{Name: ConsentCookie, Value: next.Legal.Revision()})
	response, err = app.Test(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	require.NoError(t, response.Body.Close())
}
