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
	"net"
	"net/url"
	"strings"
	"unicode"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
)

const maxGitHubOAuthSettingsBody = 16 << 10

type githubOAuthSettingsResponse struct {
	ClientID               string `json:"client_id"`
	CallbackURL            string `json:"callback_url"`
	Enabled                bool   `json:"enabled"`
	ClientSecretConfigured bool   `json:"client_secret_configured"`
}

type githubOAuthSettingsRequest struct {
	ClientID          string `json:"client_id"`
	ClientSecret      string `json:"client_secret"`
	CallbackURL       string `json:"callback_url"`
	Enabled           bool   `json:"enabled"`
	ClearClientSecret bool   `json:"clear_client_secret"`
}

func githubOAuthSettings(configValue config.GitHubOAuthConfig) githubOAuthSettingsResponse {
	return githubOAuthSettingsResponse{
		ClientID:               configValue.ClientID,
		CallbackURL:            configValue.CallbackURL,
		Enabled:                configValue.Enabled,
		ClientSecretConfigured: strings.TrimSpace(configValue.ClientSecret) != "",
	}
}

func validGitHubCredential(value string, maxLength int) bool {
	if value == "" || len(value) > maxLength {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func validGitHubCallbackURL(value string) bool {
	if value == "" || len(value) > 2048 {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.Hostname() == "" || parsed.Path != "/api/auth/github/callback" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	if parsed.Scheme != "http" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	ip := net.ParseIP(host)
	return host == "localhost" || host == "::1" || ip != nil && ip.IsLoopback()
}

func normalizeGitHubOAuthSettings(current config.GitHubOAuthConfig,
	request githubOAuthSettingsRequest) (config.GitHubOAuthConfig, error) {
	next := current.DeepCopy()
	next.Enabled = request.Enabled
	next.ClientID = strings.TrimSpace(request.ClientID)
	next.CallbackURL = strings.TrimSpace(request.CallbackURL)
	if request.ClearClientSecret {
		next.ClientSecret = ""
	} else if secret := strings.TrimSpace(request.ClientSecret); secret != "" {
		next.ClientSecret = secret
	}
	if next.ClientID != "" && !validGitHubCredential(next.ClientID, 128) {
		return config.GitHubOAuthConfig{}, errors.New("GitHub OAuth client ID is invalid")
	}
	if next.ClientSecret != "" && !validGitHubCredential(next.ClientSecret, 512) {
		return config.GitHubOAuthConfig{}, errors.New("GitHub OAuth client secret is invalid")
	}
	if next.CallbackURL != "" && !validGitHubCallbackURL(next.CallbackURL) {
		return config.GitHubOAuthConfig{}, errors.New("GitHub OAuth callback URL is invalid")
	}
	if next.Enabled && !next.Configured() {
		return config.GitHubOAuthConfig{}, errors.New("GitHub OAuth requires a client ID, client secret, and callback URL")
	}
	return next, nil
}

func getGitHubOAuthSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(githubOAuthSettings(state.Inner.Config.Load().Server.GitHubOAuth))
}

func putGitHubOAuthSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	if len(c.Body()) > maxGitHubOAuthSettingsBody {
		return c.SendStatus(fiber.StatusRequestEntityTooLarge)
	}
	var request githubOAuthSettingsRequest
	if err := c.Bind().Body(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid GitHub OAuth settings")
	}

	state.Inner.ConfigWriteLock.Lock()
	current := state.Inner.Config.Load()
	nextGitHub, err := normalizeGitHubOAuthSettings(current.Server.GitHubOAuth, request)
	if err != nil {
		state.Inner.ConfigWriteLock.Unlock()
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}
	next := current.DeepCopy()
	next.Server.GitHubOAuth = nextGitHub
	err = persistConfigSnapshot(next)
	if err == nil {
		state.Inner.Config.Store(next)
	}
	state.Inner.ConfigWriteLock.Unlock()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to save GitHub OAuth settings")
	}

	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionSettingsUpdate,
		Details: "Updated GitHub OAuth settings", AuthMethod: authMethod, SessionID: sessionID, IP: ip,
	})
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(githubOAuthSettings(nextGitHub))
}
