/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"time"

	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/utils"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
)

func beginGitHubRegistration(c fiber.Ctx, state *core.AppState, record core.TransientAuthState,
	identity githubAPIIdentity, principals []core.GitHubPrincipal) error {
	if !state.Inner.Config.Load().Registration.Enabled {
		return oauthResultRedirect(c, record.ReturnTo, "registration_disabled")
	}
	email := identity.PrimaryEmail
	if email == "" {
		return oauthResultRedirect(c, record.ReturnTo, "email_missing")
	}
	capability, err := newOAuthState()
	if err != nil {
		return oauthResultRedirect(c, record.ReturnTo, "identity_failed")
	}
	profile := core.RegistrationProfile{GitHubID: identity.ID, GitHubLogin: identity.Login, Principals: principals, GitHubEmails: identity.Emails}
	if value, ok := core.NormalizeUsername(identity.Login); ok {
		profile.Username = value
	}
	if value, ok := core.NormalizeNickname(identity.Name); ok {
		profile.Nickname = value
	}
	payload, err := json.Marshal(profile)
	if err != nil {
		return oauthResultRedirect(c, record.ReturnTo, "identity_failed")
	}
	state.Inner.ConfigWriteLock.Lock()
	cfg := state.Inner.Config.Load()
	now := time.Now().UnixMilli()
	pending := &core.PendingRegistration{IDHash: registrationHash(capability), ProviderKey: "github:" + strconv.FormatInt(identity.ID, 10),
		IPHash: registrationHash(registrationIP(c, &cfg.Server)), Email: email, ProfileJSON: string(payload), CreatedAt: now,
		ExpiresAt: now + (10 * time.Minute).Milliseconds(), CooldownUntil: now + (10*time.Minute + cfg.Registration.ProviderCooldown.Duration()).Milliseconds()}
	if !cfg.Server.GitHubOAuth.Configured() {
		err = core.ErrRegistrationDisabled
	} else if !cfg.Mail.Allows(email) {
		err = core.ErrRegistrationInvalid
	} else {
		err = state.GetDB().BeginRegistration(pending, nil, "", "", cfg.Mail.ManualRate, cfg.Registration)
	}
	state.Inner.ConfigWriteLock.Unlock()
	if err != nil {
		result := "identity_failed"
		switch {
		case errors.Is(err, core.ErrRegistrationDisabled):
			result = "registration_disabled"
		case errors.Is(err, core.ErrRegistrationRateLimited):
			result = "registration_ip_limited"
		case errors.Is(err, core.ErrRegistrationCooldown):
			result = "registration_cooldown"
		case errors.Is(err, core.ErrRegistrationPending):
			result = "registration_pending"
		}
		return oauthResultRedirect(c, record.ReturnTo, result)
	}
	setRegistrationCookie(c, capability)
	return c.Redirect().Status(303).To("/account/register?provider=github&return_to=" + url.QueryEscape(record.ReturnTo))
}

func importRegistrationGitHubAvatar(c fiber.Ctx, state *core.AppState, profile *core.RegistrationProfile) bool {
	ctx, cancel := context.WithTimeout(c.Context(), 15*time.Second)
	defer cancel()
	maxSize := int64(state.Inner.Config.Load().Server.AvatarMaxSizeBytes)
	value, contentType, err := fetchGitHubAvatar(ctx, state, defaultGitHubOAuthProvider, profile.GitHubID, maxSize)
	if err != nil {
		return false
	}
	avatar, err := normalizeAvatar(value, contentType, maxSize)
	if err != nil {
		return false
	}
	if status, _ := persistProfileAvatar(state, profile.Username, avatar); status != 0 {
		return false
	}
	audit.Log(state, &core.AuditLogEntry{Username: profile.Username, Operator: profile.Username, Action: audit.ActionProfileUpdate,
		Details: "Updated profile avatar from GitHub", AuthMethod: "Registration", IP: utils.ExtractIP(c, &state.Inner.Config.Load().Server)})
	return true
}
