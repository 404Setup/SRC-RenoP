/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"renop/internal/config"
	"renop/internal/core"
)

func beginOAuthRegistration(c fiber.Ctx, state *core.AppState, record core.TransientAuthState, p config.OAuthProviderConfig, info oauthUserInfo, accessToken string) error {
	result := func(code string) error { return providerOAuthRedirect(c, record.ReturnTo, p.ID, code) }
	capability, err := newOAuthState()
	if err != nil {
		return result("identity_failed")
	}
	profile := core.RegistrationProfile{OAuth: &info.Identity, EmailVerified: info.EmailVerified, AvatarURL: info.AvatarURL,
		ProviderConfigHash: record.ConfigHash}
	username := info.Username
	if username == "" && info.Email != "" {
		username, _, _ = strings.Cut(info.Email, "@")
	}
	profile.Username, _ = core.NormalizeUsername(username)
	profile.Nickname, _ = core.NormalizeNickname(info.Name)
	if oauthAvatarUsesToken(p, info.AvatarURL) {
		profile.AvatarToken = sealOAuthAvatarToken(state, &profile, accessToken)
	}
	payload, err := json.Marshal(profile)
	if err != nil {
		return result("identity_failed")
	}
	state.Inner.ConfigWriteLock.Lock()
	cfg := state.Inner.Config.Load()
	current, configured := cfg.Server.OAuthProvider(p.ID)
	now := time.Now().UnixMilli()
	pending := &core.PendingRegistration{IDHash: registrationHash(capability), IPHash: registrationHash(registrationIP(c, &cfg.Server)),
		ProviderKey: p.ID + ":" + info.Identity.Key(), Email: info.Email, ProfileJSON: string(payload), CreatedAt: now,
		ExpiresAt: now + 600000, CooldownUntil: now + 600000 + cfg.Registration.ProviderCooldown.Duration().Milliseconds()}
	if !configured || oauthConfigurationHash(current) != record.ConfigHash || !cfg.Registration.Enabled {
		err = core.ErrRegistrationDisabled
	} else if info.EmailVerified && !cfg.Mail.Allows(info.Email) {
		state.Inner.ConfigWriteLock.Unlock()
		return result("email_blocked")
	} else {
		err = state.GetDB().BeginRegistration(pending, nil, "", "", cfg.Mail.ManualRate, cfg.Registration)
	}
	state.Inner.ConfigWriteLock.Unlock()
	if err != nil {
		code := "identity_failed"
		switch {
		case errors.Is(err, core.ErrRegistrationDisabled):
			code = "registration_disabled"
		case errors.Is(err, core.ErrRegistrationRateLimited):
			code = "registration_ip_limited"
		case errors.Is(err, core.ErrRegistrationCooldown):
			code = "registration_cooldown"
		case errors.Is(err, core.ErrRegistrationPending):
			code = "registration_pending"
		}
		return result(code)
	}
	setRegistrationCookie(c, capability)
	return c.Redirect().Status(303).To("/account/register?" + url.Values{"provider": {p.ID}, "return_to": {record.ReturnTo}}.Encode())
}
