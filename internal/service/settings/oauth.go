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
	"errors"
	"strings"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/utils"

	"github.com/gofiber/fiber/v3"
)

type oauthProviderSettings struct {
	config.OAuthProviderConfig
	ClientSecretConfigured bool `json:"client_secret_configured"`
	APIKeyConfigured       bool `json:"api_key_configured"`
	ClearClientSecret      bool `json:"clear_client_secret,omitempty"`
	ClearAPIKey            bool `json:"clear_api_key,omitempty"`
}

func oauthSettings(server config.ServerConfig) fiber.Map {
	github := server.GitHubOAuth
	values := []oauthProviderSettings{{
		ID: "github", Type: "github", Name: "GitHub", Enabled: github.Enabled,
		ClientID: github.ClientID, CallbackURL: github.CallbackURL, ClientSecretConfigured: github.ClientSecret != ""}}
	for _, p := range server.OAuthProviders {
		value := oauthProviderSettings{OAuthProviderConfig: p.Resolved(), ClientSecretConfigured: p.ClientSecret != "", APIKeyConfigured: p.APIKey != ""}
		value.ClientSecret, value.APIKey = "", ""
		values = append(values, value)
	}
	presets := []config.OAuthProviderConfig{}
	for _, item := range [][2]string{{"microsoft", "Microsoft"}, {"google", "Google"}, {"gitlab", "GitLab"},
		{"cloudflare", "Cloudflare"}, {"stackexchange", "Stack Exchange"}, {"custom", "OAuth 2.0"}} {
		presets = append(presets, (config.OAuthProviderConfig{ID: item[0], Type: item[0], Name: item[1]}).Resolved())
	}
	return fiber.Map{"providers": values, "presets": presets}
}

func getOAuthSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(oauthSettings(state.Inner.Config.Load().Server))
}

func normalizeOAuthSettings(current []config.OAuthProviderConfig, request []oauthProviderSettings) ([]config.OAuthProviderConfig, error) {
	invalid := errors.New("OAuth settings are invalid")
	if len(request) > 32 {
		return nil, invalid
	}
	next := make([]config.OAuthProviderConfig, 0, len(request))
	seen := map[string]bool{}
	for _, value := range request {
		p := value.OAuthProviderConfig
		p.ID, p.Type, p.Name = strings.TrimSpace(p.ID), strings.TrimSpace(p.Type), strings.TrimSpace(p.Name)
		p.ClientID = strings.TrimSpace(p.ClientID)
		p.ClientSecret = strings.TrimSpace(p.ClientSecret)
		p.APIKey = strings.TrimSpace(p.APIKey)
		p.CallbackURL = strings.TrimSpace(p.CallbackURL)
		p.BaseURL = strings.TrimSpace(p.BaseURL)
		p.TokenAuth = strings.TrimSpace(p.TokenAuth)
		if seen[p.ID] {
			return nil, invalid
		}
		seen[p.ID] = true
		for _, old := range current {
			if old.ID != p.ID || old.Type != p.Type || strings.TrimSpace(old.ClientID) != p.ClientID || old.Resolved().TokenURL != p.Resolved().TokenURL {
				continue
			}
			if p.ClientSecret == "" && !value.ClearClientSecret {
				p.ClientSecret = strings.TrimSpace(old.ClientSecret)
			}
			if p.APIKey == "" && !value.ClearAPIKey {
				p.APIKey = strings.TrimSpace(old.APIKey)
			}
		}
		if value.ClearClientSecret {
			p.ClientSecret = ""
		}
		if value.ClearAPIKey {
			p.APIKey = ""
		}
		if err := p.Validate(); err != nil {
			return nil, invalid
		}
		next = append(next, p)
	}
	return next, nil
}

func putOAuthSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	var request struct {
		Providers []oauthProviderSettings `json:"providers"`
	}
	if err := utils.ReadJSONLimited(c, &request, 128<<10); err != nil {
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) {
			return err
		}
		return cacheSettingsError(c, 400, "oauth_settings_invalid")
	}
	if request.Providers == nil {
		return cacheSettingsError(c, 400, "oauth_settings_invalid")
	}
	state.Inner.ConfigWriteLock.Lock()
	defer state.Inner.ConfigWriteLock.Unlock()
	current := state.Inner.Config.Load()
	github := current.Server.GitHubOAuth
	providersRequest := make([]oauthProviderSettings, 0, len(request.Providers))
	seenGitHub := false
	for _, value := range request.Providers {
		if value.Type != "github" {
			providersRequest = append(providersRequest, value)
			continue
		}
		if seenGitHub || value.ID != "github" || value.Name != "GitHub" {
			return cacheSettingsError(c, 400, "oauth_settings_invalid")
		}
		seenGitHub = true
		var err error
		github, err = normalizeGitHubOAuthSettings(github, githubOAuthSettingsRequest{
			ClientID: value.ClientID, ClientSecret: value.ClientSecret, CallbackURL: value.CallbackURL,
			Enabled: value.Enabled, ClearClientSecret: value.ClearClientSecret,
		})
		if err != nil {
			return cacheSettingsError(c, 400, "oauth_settings_invalid")
		}
	}
	providers, err := normalizeOAuthSettings(current.Server.OAuthProviders, providersRequest)
	if err != nil {
		return cacheSettingsError(c, 400, "oauth_settings_invalid")
	}
	next := current.DeepCopy()
	next.Server.OAuthProviders = providers
	next.Server.GitHubOAuth = github
	if persistConfigSnapshot(next) != nil {
		return cacheSettingsError(c, 500, "oauth_settings_save_failed")
	}
	state.Inner.Config.Store(next)
	username, operator, method, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{Username: username, Operator: operator, AuthMethod: method,
		SessionID: sessionID, IP: ip, Action: audit.ActionSettingsUpdate, Details: "Updated OAuth providers"})
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(oauthSettings(next.Server))
}
