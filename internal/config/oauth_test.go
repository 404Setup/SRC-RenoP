/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package config

import (
	"testing"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestOAuthProviderConfiguration(t *testing.T) {
	for _, kind := range []string{"microsoft", "google", "gitlab", "cloudflare", "stackexchange"} {
		p := OAuthProviderConfig{ID: kind, Type: kind, Name: kind, Enabled: true, ClientID: "client", ClientSecret: "secret", APIKey: "key",
			CallbackURL: "https://renop.example/api/auth/oauth/" + kind + "/callback"}
		require.NoError(t, p.Validate(), kind)
		r := p.Resolved()
		if kind == "cloudflare" {
			require.Equal(t, OAuthClaims{Subject: "sub"}, r.Claims)
		}
		require.True(t, ValidOAuthURL(r.AuthorizeURL), kind)
		require.True(t, ValidOAuthURL(r.TokenURL), kind)
		require.True(t, ValidOAuthURL(r.UserInfoURL), kind)
		changed := p
		changed.ClientSecret = "rotated"
		require.Equal(t, p.Authority(r.Issuer), changed.Authority(r.Issuer))
		changed.ClientID = "different-client"
		require.NotEqual(t, p.Authority(r.Issuer), changed.Authority(r.Issuer))
	}
	p := OAuthProviderConfig{ID: "custom", Type: "custom", Name: "Custom", Enabled: true, ClientID: "client", ClientSecret: "secret",
		CallbackURL: "https://renop.example/api/auth/oauth/custom/callback", AuthorizeURL: "https://id.example/auth", TokenURL: "https://id.example/token",
		UserInfoURL: "https://id.example/me", Claims: OAuthClaims{Subject: "data.id"}}
	require.NoError(t, p.Validate())
	for _, change := range []func(*OAuthProviderConfig){
		func(p *OAuthProviderConfig) { p.ID = "github" },
		func(p *OAuthProviderConfig) { p.ClientID = "client\nsecret" },
		func(p *OAuthProviderConfig) { p.CallbackURL = "https://renop.example/other" },
		func(p *OAuthProviderConfig) { p.TokenURL = "http://id.example/token" },
		func(p *OAuthProviderConfig) { p.UserInfoURL = "https://user:secret@id.example/me" },
		func(p *OAuthProviderConfig) { p.TokenAuth, p.DisablePKCE = "none", true },
		func(p *OAuthProviderConfig) { p.Scopes = "openid" },
		func(p *OAuthProviderConfig) { p.Claims.Subject = "" },
		func(p *OAuthProviderConfig) { p.Type, p.Tenant = "microsoft", "not-a-tenant" },
	} {
		bad := p
		change(&bad)
		require.Error(t, bad.Validate())
	}
	cfg := DefaultConfig()
	cfg.Server.OAuthProviders = []OAuthProviderConfig{p}
	copy := cfg.DeepCopy()
	copy.Server.OAuthProviders[0].ClientSecret = "new"
	require.Equal(t, "secret", cfg.Server.OAuthProviders[0].ClientSecret)
	for _, marshal := range []func(any) ([]byte, error){json.Marshal, yaml.Marshal} {
		data, err := marshal(cfg.Server)
		require.NoError(t, err)
		var restored ServerConfig
		if json.Valid(data) {
			err = json.Unmarshal(data, &restored)
		} else {
			err = yaml.Unmarshal(data, &restored)
		}
		require.NoError(t, err)
		require.Equal(t, cfg.Server.OAuthProviders, restored.OAuthProviders)
	}
}

func TestOAuthSubjectSelectorIsolation(t *testing.T) {
	p := OAuthProviderConfig{ID: "custom", Type: "custom", ClientID: "client", UserInfoURL: "https://id.example/me",
		Claims: OAuthClaims{Subject: "data.id", Name: "data.name"}}
	changed := p
	changed.Claims.Subject = "data.alternate_id"
	require.NotEqual(t, p.Authority(""), changed.Authority(""), "different JSON identity fields must not reuse account bindings")
	changed = p
	changed.Claims.Name = "data.display_name"
	require.Equal(t, p.Authority(""), changed.Authority(""), "display metadata does not identify accounts")
}
