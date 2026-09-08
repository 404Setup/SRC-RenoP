/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/utils"
	"renop/internal/utils/secretcipher"
)

func oauthAvatarUsesToken(p config.OAuthProviderConfig, target string) bool {
	if p.Type == "microsoft" {
		return target == "https://graph.microsoft.com/v1.0/me/photo/$value"
	}
	avatar, err := url.Parse(target)
	userinfo, infoErr := url.Parse(p.UserInfoURL)
	return p.Type == "custom" && err == nil && infoErr == nil && avatar.Scheme == userinfo.Scheme && avatar.Host == userinfo.Host
}

func oauthAvatarAAD(profile *core.RegistrationProfile) []byte {
	return []byte("oauth-avatar:" + profile.OAuth.Key() + ":" + profile.AvatarURL)
}

func sealOAuthAvatarToken(state *core.AppState, profile *core.RegistrationProfile, token string) string {
	cipher, err := secretcipher.New(state.Inner.Config.Load().MFAEncryptionKey)
	if err != nil || token == "" || len(token) > 16384 {
		return ""
	}
	return base64.RawStdEncoding.EncodeToString(cipher.Seal(nil, nil, []byte(token), oauthAvatarAAD(profile)))
}

func fetchOAuthAvatar(ctx context.Context, state *core.AppState, p config.OAuthProviderConfig, target, token string) (*core.UserAvatar, error) {
	invalid := errors.New("OAuth avatar is unavailable")
	if !config.ValidOAuthURL(target) {
		return nil, invalid
	}
	u, _ := url.Parse(target)
	userinfo, _ := url.Parse(p.UserInfoURL)
	trustedOrigin := (p.Type == "custom" || p.Type == "gitlab") && u.Scheme == userinfo.Scheme && u.Host == userinfo.Host
	addresses, err := net.DefaultResolver.LookupIP(ctx, "ip", u.Hostname())
	if err != nil || len(addresses) == 0 || len(addresses) > 32 {
		return nil, invalid
	}
	chosen := addresses[0]
	for _, address := range addresses {
		if !trustedOrigin && !utils.IsPublicIP(address) {
			return nil, invalid
		}
		if address.To4() != nil {
			chosen = address
		}
	}
	client, err := oauthHTTPClient(state.Inner.Config.Load())
	if err != nil {
		return nil, invalid
	}
	defer client.CloseIdleConnections()
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = u.Hostname()
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	host := u.Host
	// Pin the destination for direct and proxied requests while retaining the original Host and TLS name.
	u.Host = net.JoinHostPort(chosen.String(), port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, invalid
	}
	req.Host = host
	if oauthAvatarUsesToken(p, target) {
		if token == "" || len(token) > 16384 || strings.ContainsAny(token, "\r\n\x00") {
			return nil, invalid
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/webp")
	req.Header.Set("User-Agent", "RenoP-OAuth-Avatar/1")
	response, err := client.Do(req)
	if err != nil {
		return nil, invalid
	}
	defer utils.DiscardHTTPBody(response.Body, response.ContentLength)
	limit := avatarSizeLimit(state)
	if response.StatusCode != 200 || response.ContentLength > limit {
		return nil, invalid
	}
	data, err := utils.ReadAllLimited(response.Body, limit)
	if err != nil {
		return nil, invalid
	}
	return normalizeAvatar(data, response.Header.Get("Content-Type"), limit)
}

func importRegistrationOAuthAvatar(c fiber.Ctx, state *core.AppState, profile *core.RegistrationProfile) bool {
	if profile.OAuth == nil || profile.AvatarURL == "" {
		return false
	}
	p, ok := state.Inner.Config.Load().Server.OAuthProvider(profile.OAuth.ProviderID)
	if !ok || oauthConfigurationHash(p) != profile.ProviderConfigHash {
		return false
	}
	token := ""
	if profile.AvatarToken != "" {
		cipher, err := secretcipher.New(state.Inner.Config.Load().MFAEncryptionKey)
		if err != nil {
			return false
		}
		sealed, err := base64.RawStdEncoding.DecodeString(profile.AvatarToken)
		if err != nil {
			return false
		}
		value, err := cipher.Open(nil, nil, sealed, oauthAvatarAAD(profile))
		if err != nil {
			return false
		}
		token = string(value)
	}
	ctx, cancel := context.WithTimeout(c.Context(), 15*time.Second)
	defer cancel()
	avatar, err := fetchOAuthAvatar(ctx, state, p, profile.AvatarURL, token)
	if err != nil {
		return false
	}
	if status, _ := persistProfileAvatar(state, profile.Username, avatar); status != 0 {
		return false
	}
	audit.Log(state, &core.AuditLogEntry{Username: profile.Username, Operator: profile.Username, Action: audit.ActionProfileUpdate,
		Details: "Updated profile avatar from OAuth provider " + p.ID, AuthMethod: "Registration", IP: utils.ExtractIP(c, &state.Inner.Config.Load().Server)})
	return true
}

func finishOAuthAvatar(c fiber.Ctx, state *core.AppState, record core.TransientAuthState, p config.OAuthProviderConfig, info oauthUserInfo, token string) error {
	result := func(code string) error { return providerOAuthRedirect(c, record.ReturnTo, p.ID, code) }
	identity, err := state.GetDB().GetOAuthIdentity(info.Identity)
	if err != nil || identity == nil || identity.UserID != record.UserID {
		return result("identity_linked")
	}
	ctx, cancel := context.WithTimeout(c.Context(), 15*time.Second)
	defer cancel()
	avatar, err := fetchOAuthAvatar(ctx, state, p, info.AvatarURL, token)
	if err != nil {
		return result("avatar_failed")
	}
	state.Inner.ConfigWriteLock.Lock()
	defer state.Inner.ConfigWriteLock.Unlock()
	if !oauthConfigurationCurrent(state, record) {
		return result("configuration_changed")
	}
	mfa, err := state.GetDB().GetMFAState(identity.Username)
	if err != nil || mfa.Snapshot != record.Snapshot {
		return result("session_changed")
	}
	if status, _ := persistProfileAvatar(state, identity.Username, avatar); status != 0 {
		return result("avatar_failed")
	}
	username, operator, method, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{Username: username, Operator: operator, Action: audit.ActionProfileUpdate,
		Details: "Updated profile avatar from OAuth provider " + p.ID, AuthMethod: method, SessionID: sessionID, IP: ip})
	return result("avatar_updated")
}
