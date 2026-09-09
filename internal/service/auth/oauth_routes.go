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
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/emmansun/base64"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
)

type oauthProfileStatus struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Login           string `json:"login,omitempty"`
	Configured      bool   `json:"configured"`
	Linked          bool   `json:"linked"`
	CanDisconnect   bool   `json:"can_disconnect"`
	CanVerifyEmail  bool   `json:"can_verify_email"`
	CanImportAvatar bool   `json:"can_import_avatar"`
	AuthorizedAt    int64  `json:"authorized_at,omitempty"`
}

func setupOAuthRoutes(auth fiber.Router, state *core.AppState) {
	auth.Get("/oauth/providers", func(c fiber.Ctx) error {
		setPrivateResponseHeaders(c)
		providers := []fiber.Map{}
		for _, p := range state.Inner.Config.Load().Server.OAuthProviders {
			if p.Configured() {
				providers = append(providers, fiber.Map{"id": p.ID, "name": p.Name})
			}
		}
		return c.JSON(fiber.Map{"providers": providers})
	})
	auth.Get("/oauth/:provider/start", func(c fiber.Ctx) error { return startOAuth(c, state) })
	auth.Get("/oauth/:provider/callback", func(c fiber.Ctx) error { return finishOAuth(c, state) })
	auth.Get("/profile/oauth", func(c fiber.Ctx) error {
		setPrivateResponseHeaders(c)
		profile, err := currentSessionProfile(c, state)
		if err != nil || profile == nil {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		statuses, err := oauthProfileStatuses(state, profile.Username)
		if err != nil {
			return passwordResetError(c, 503, "oauth_unavailable")
		}
		return c.JSON(fiber.Map{"providers": statuses})
	})
	auth.Delete("/profile/oauth/:provider", func(c fiber.Ctx) error { return disconnectOAuth(c, state) })
}

func oauthConfigurationHash(p config.OAuthProviderConfig) string {
	data, _ := json.Marshal(p)
	return registrationHash(string(data))
}

func oauthConfigurationCurrent(state *core.AppState, record core.TransientAuthState) bool {
	p, ok := state.Inner.Config.Load().Server.OAuthProvider(record.Provider)
	return ok && oauthConfigurationHash(p) == record.ConfigHash
}

func providerOAuthRedirect(c fiber.Ctx, returnTo, provider, result string) error {
	return c.Redirect().Status(303).To(safeOAuthReturnTo(returnTo) + "?" + url.Values{"oauth": {result}, "provider": {provider}}.Encode())
}

func startOAuth(c fiber.Ctx, state *core.AppState) error {
	setPrivateResponseHeaders(c)
	cfg := state.Inner.Config.Load()
	p, ok := cfg.Server.OAuthProvider(c.Params("provider"))
	if !ok {
		return c.SendStatus(404)
	}
	intent := c.Query("intent", "login")
	if intent != "login" && intent != "register" && intent != "link" && intent != "email" && intent != "avatar" {
		return passwordResetError(c, 400, "oauth_invalid")
	}
	if intent == "register" && !cfg.Registration.Enabled {
		return registrationError(c, core.ErrRegistrationDisabled)
	}
	raw, err := newOAuthState()
	if err != nil {
		return passwordResetError(c, 503, "oauth_unavailable")
	}
	verifier, err := newOAuthState()
	if err != nil {
		return passwordResetError(c, 503, "oauth_unavailable")
	}
	record := core.TransientAuthState{Provider: p.ID, Intent: intent, ReturnTo: safeOAuthReturnTo(c.Query("return_to")),
		Verifier: verifier, ConfigHash: oauthConfigurationHash(p), ExpiresAt: time.Now().Add(githubOAuthStateTTL).UnixMilli()}
	if intent != "login" && intent != "register" {
		profile, err := currentSessionProfile(c, state)
		session, _ := c.Locals("current_session_id").(string)
		if err != nil || profile == nil || session == "" || c.Cookies(sessionCookieName) != session {
			return c.SendStatus(403)
		}
		mfa, err := state.GetDB().GetMFAState(profile.Username)
		if err != nil {
			return passwordResetError(c, 503, "oauth_unavailable")
		}
		record.UserID, record.Snapshot, record.SessionHash = profile.UserID, mfa.Snapshot, registrationHash(session)
	}
	if state.Inner.ExternalAuthStates == nil || !state.Inner.ExternalAuthStates.Put(raw, record, time.Now().UnixMilli()) {
		return passwordResetError(c, 503, "oauth_unavailable")
	}
	authorize, _ := url.Parse(p.AuthorizeURL)
	query := authorize.Query()
	query.Set("client_id", p.ClientID)
	query.Set("redirect_uri", p.CallbackURL)
	query.Set("response_type", "code")
	query.Set("response_mode", "query")
	query.Set("state", raw)
	query.Set("scope", p.Scopes)
	if p.Issuer != "" {
		query.Set("nonce", raw)
	}
	if !p.DisablePKCE {
		digest := sha256.Sum256([]byte(verifier))
		query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(digest[:]))
		query.Set("code_challenge_method", "S256")
	}
	authorize.RawQuery = query.Encode()
	c.Cookie(&fiber.Cookie{Name: "renop_oauth_" + p.ID, Value: raw, Path: "/api/auth/oauth/" + p.ID,
		MaxAge: 600, Expires: time.Now().Add(githubOAuthStateTTL), Secure: isSecure(c), HTTPOnly: true, SameSite: "Lax"})
	return c.Redirect().To(authorize.String())
}

func finishOAuth(c fiber.Ctx, state *core.AppState) error {
	setPrivateResponseHeaders(c)
	provider := c.Params("provider")
	raw := c.Query("state")
	if len(raw) != 43 || len(provider) > 32 || subtle.ConstantTimeCompare([]byte(raw), []byte(c.Cookies("renop_oauth_"+provider))) != 1 {
		return providerOAuthRedirect(c, "/account/login", provider, "state_invalid")
	}
	record, ok := state.Inner.ExternalAuthStates.Consume(raw, provider, time.Now().UnixMilli())
	if !ok {
		return providerOAuthRedirect(c, "/account/login", provider, "state_invalid")
	}
	result := func(code string) error { return providerOAuthRedirect(c, record.ReturnTo, provider, code) }
	if c.Query("error") != "" {
		return result("provider_denied")
	}
	cfg := state.Inner.Config.Load()
	p, ok := cfg.Server.OAuthProvider(provider)
	if !ok || oauthConfigurationHash(p) != record.ConfigHash {
		return result("configuration_changed")
	}
	code := c.Query("code")
	if code == "" || len(code) > 4096 {
		return result("exchange_failed")
	}
	client, err := oauthHTTPClient(cfg)
	if err != nil {
		return result("exchange_failed")
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(c.Context(), 25*time.Second)
	defer cancel()
	tokens, err := exchangeOAuthCode(ctx, client, p, code, record.Verifier)
	if err != nil {
		return result("exchange_failed")
	}
	info, err := fetchOAuthUserInfo(ctx, client, p, tokens, raw)
	if err != nil {
		return result("identity_failed")
	}
	if !oauthConfigurationCurrent(state, record) {
		return result("configuration_changed")
	}
	if record.UserID != "" {
		return finishOAuthProfile(c, state, record, p, info, tokens.AccessToken)
	}
	linked, err := state.GetDB().GetOAuthIdentity(info.Identity)
	if err != nil {
		return result("identity_failed")
	}
	if linked == nil {
		return beginOAuthRegistration(c, state, record, p, info, tokens.AccessToken)
	}
	state.Inner.ConfigWriteLock.Lock()
	defer state.Inner.ConfigWriteLock.Unlock()
	if !oauthConfigurationCurrent(state, record) {
		return result("configuration_changed")
	}
	if err = state.GetDB().RefreshOAuthIdentity(linked.UserID, info.Identity, time.Now().UnixMilli()); err != nil {
		if code := providerEmailErrorCode(err); code != "" {
			return result(code)
		}
		return result("session_changed")
	}
	state.InvalidateAccountAuthCache(true, linked.Username)
	mfa, err := state.GetDB().GetMFAState(linked.Username)
	if errors.Is(err, core.ErrAccountDeleted) {
		return result("account_deleted")
	}
	if err != nil || mfa.UserID != linked.UserID {
		return result("session_failed")
	}
	// Read the binding after the security snapshot so unlinking cannot authorize a later session.
	current, err := state.GetDB().GetOAuthIdentity(info.Identity)
	if err != nil || current == nil || current.UserID != linked.UserID {
		return result("session_changed")
	}
	account := state.GetTokenByName(linked.Username)
	if account == nil {
		return result("account_deleted")
	}
	if err = accountAccessError(account); err != nil {
		if errors.Is(err, core.ErrAccountBanned) {
			return result("account_banned")
		}
		return result("account_deleted")
	}
	user := buildSynthUser(account)
	user.AuthenticationSnapshot = mfa.Snapshot
	if err = issueBrowserSession(c, state, user, "oauth:"+provider); err != nil {
		if errors.Is(err, errMFARequired) {
			return c.Redirect().To("/account/login?mfa=1&return_to=" + url.QueryEscape(record.ReturnTo))
		}
		return result("session_failed")
	}
	return result("success")
}

func finishOAuthProfile(c fiber.Ctx, state *core.AppState, record core.TransientAuthState, p config.OAuthProviderConfig, info oauthUserInfo, accessToken string) error {
	result := func(code string) error { return providerOAuthRedirect(c, record.ReturnTo, p.ID, code) }
	profile, err := currentSessionProfile(c, state)
	session, _ := c.Locals("current_session_id").(string)
	if err != nil || profile == nil || profile.UserID != record.UserID || session == "" ||
		c.Cookies(sessionCookieName) != session || registrationHash(session) != record.SessionHash {
		return result("session_changed")
	}
	if record.Intent == "avatar" {
		return finishOAuthAvatar(c, state, record, p, info, accessToken)
	}
	state.Inner.ConfigWriteLock.Lock()
	defer state.Inner.ConfigWriteLock.Unlock()
	if !oauthConfigurationCurrent(state, record) {
		return result("configuration_changed")
	}
	if record.Intent == "email" {
		if !info.EmailVerified {
			return result("email_missing")
		}
		if !state.Inner.Config.Load().Mail.Allows(info.Email) {
			return result("email_blocked")
		}
		_, err = state.GetDB().UpdateAccountEmailFromSession(profile.Username, session, info.Email, record.Snapshot, time.Now().UnixMilli(), core.ProviderEmail{Email: info.Email, Verified: true})
		if code := providerEmailErrorCode(err); code != "" {
			return result(code)
		}
		if errors.Is(err, core.ErrEmailAlreadyExists) {
			return result("email_conflict")
		}
		if err != nil {
			return result("session_changed")
		}
		recordPrivateEmailChange(c, state, profile.Username)
		return result("email_updated")
	}
	if err = state.GetDB().LinkOAuthIdentity(profile.Username, session, record.Snapshot, info.Identity, time.Now().UnixMilli()); err != nil {
		if errors.Is(err, core.ErrOAuthIdentityLinked) {
			return result("identity_linked")
		}
		if code := providerEmailErrorCode(err); code != "" {
			return result(code)
		}
		return result("session_changed")
	}
	state.InvalidateAccountAuthCache(true, profile.Username)
	username, operator, method, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{Username: username, Operator: operator, Action: audit.ActionProfileUpdate,
		Details: "Connected OAuth provider " + p.ID, AuthMethod: method, SessionID: sessionID, IP: ip})
	return result("linked")
}

func oauthProfileStatuses(state *core.AppState, username string) ([]oauthProfileStatus, error) {
	identities, err := state.GetDB().GetOAuthIdentities(username)
	if err != nil {
		return nil, err
	}
	security, err := state.GetDB().GetAccountSecurity(username)
	if err != nil {
		return nil, err
	}
	statuses := []oauthProfileStatus{}
	seen := map[string]bool{}
	for _, p := range state.Inner.Config.Load().Server.OAuthProviders {
		p = p.Resolved()
		status := oauthProfileStatus{ID: p.ID, Name: p.Name, Configured: p.Configured()}
		status.CanVerifyEmail = status.Configured && p.Claims.Email != "" && p.Claims.EmailVerified != ""
		status.CanImportAvatar = status.Configured && p.Claims.Avatar != ""
		for _, identity := range identities {
			if identity.ProviderID == p.ID {
				status.Linked, status.Login, status.AuthorizedAt = true, identity.Login, identity.AuthorizedAt
			}
		}
		if !status.Linked && !status.Configured {
			continue
		}
		statuses = append(statuses, status)
		seen[p.ID] = true
	}
	for _, identity := range identities {
		if !seen[identity.ProviderID] {
			statuses = append(statuses, oauthProfileStatus{ID: identity.ProviderID, Name: identity.ProviderID,
				Login: identity.Login, Linked: true, AuthorizedAt: identity.AuthorizedAt})
		}
	}
	canDisconnect := security.PasswordConfigured && security.PasswordLoginEnabled || security.GitHubLinked ||
		security.OAuthIdentityCount > 1 || security.FidoDeviceCount > 0 && !security.PasskeySecondFactor
	for i := range statuses {
		statuses[i].CanDisconnect = statuses[i].Linked && canDisconnect
	}
	return statuses, nil
}

func disconnectOAuth(c fiber.Ctx, state *core.AppState) error {
	setPrivateResponseHeaders(c)
	profile, err := currentSessionProfile(c, state)
	session, _ := c.Locals("current_session_id").(string)
	if err != nil || profile == nil || session == "" || c.Cookies(sessionCookieName) != session {
		return c.SendStatus(403)
	}
	provider := c.Params("provider")
	if len(provider) > 32 {
		return c.SendStatus(404)
	}
	err = state.GetDB().DeleteOAuthIdentity(profile.Username, session, provider, time.Now().UnixMilli())
	if errors.Is(err, core.ErrLastLoginMethod) {
		return passwordResetError(c, 409, "oauth_last_login_method")
	}
	if errors.Is(err, core.ErrOAuthIdentityNotFound) {
		return c.SendStatus(404)
	}
	if err != nil {
		return passwordResetError(c, 503, "oauth_unavailable")
	}
	username, operator, method, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{Username: username, Operator: operator, Action: audit.ActionProfileUpdate,
		Details: "Disconnected OAuth provider " + provider, AuthMethod: method, SessionID: sessionID, IP: ip})
	return c.SendStatus(http.StatusNoContent)
}
