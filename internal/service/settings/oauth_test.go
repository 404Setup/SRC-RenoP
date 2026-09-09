/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"renop/internal/config"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"
)

func TestOAuthSettingsKeepCredentialsPrivate(t *testing.T) {
	cfg := config.DefaultConfig()
	app, state := setupSettingsTestApp(t, cfg)
	p := config.OAuthProviderConfig{ID: "stack", Type: "stackexchange", Name: "Stack Exchange", ClientID: "client", ClientSecret: "private-secret",
		APIKey: "private-key", Enabled: true, CallbackURL: "https://renop.example/api/auth/oauth/stack/callback"}
	put := func(providers []oauthProviderSettings) *http.Response {
		data, err := json.Marshal(map[string]any{"providers": providers})
		require.NoError(t, err)
		request := httptest.NewRequest("PUT", "/oauth-providers", bytes.NewReader(data))
		request.Header.Set("Content-Type", "application/json")
		response, err := app.Test(request)
		require.NoError(t, err)
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	response := put([]oauthProviderSettings{{OAuthProviderConfig: p}})
	require.Equal(t, 200, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NotContains(t, string(body), "private-secret")
	require.NotContains(t, string(body), "private-key")
	require.Contains(t, string(body), `"api_key_configured":true`)
	require.Len(t, state.Inner.Config.Load().Server.OAuthProviders, 1)
	for _, invalid := range []string{`{}`, `{"providers":null}`} {
		request := httptest.NewRequest("PUT", "/oauth-providers", strings.NewReader(invalid))
		request.Header.Set("Content-Type", "application/json")
		rejected, err := app.Test(request)
		require.NoError(t, err)
		require.NoError(t, rejected.Body.Close())
		require.Equal(t, 400, rejected.StatusCode, "a missing provider list must not clear saved clients")
		require.Len(t, state.Inner.Config.Load().Server.OAuthProviders, 1)
	}
	p.ClientSecret, p.APIKey = "", ""
	response = put([]oauthProviderSettings{{OAuthProviderConfig: p}})
	require.Equal(t, 200, response.StatusCode)
	require.Equal(t, "private-secret", state.Inner.Config.Load().Server.OAuthProviders[0].ClientSecret)
	require.Equal(t, "private-key", state.Inner.Config.Load().Server.OAuthProviders[0].APIKey)
	response = put([]oauthProviderSettings{{OAuthProviderConfig: p}, {OAuthProviderConfig: p}})
	require.Equal(t, 400, response.StatusCode)
	p.Type = "custom"
	p.AuthorizeURL, p.TokenURL, p.UserInfoURL, p.Claims.Subject = "https://other.example/auth", "https://other.example/token", "https://other.example/me", "id"
	response = put([]oauthProviderSettings{{OAuthProviderConfig: p}})
	require.Equal(t, 400, response.StatusCode, "changing provider must not send saved credentials to another service")
	p.Type, p.Enabled = "stackexchange", false
	response = put([]oauthProviderSettings{{OAuthProviderConfig: p, ClearClientSecret: true, ClearAPIKey: true}})
	require.Equal(t, 200, response.StatusCode)
	require.Empty(t, state.Inner.Config.Load().Server.OAuthProviders[0].ClientSecret)
	require.Empty(t, state.Inner.Config.Load().Server.OAuthProviders[0].APIKey)
	response = put([]oauthProviderSettings{})
	require.Equal(t, 200, response.StatusCode)
	require.Empty(t, state.Inner.Config.Load().Server.OAuthProviders)
}
