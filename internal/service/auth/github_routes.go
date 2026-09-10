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
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emmansun/base64"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/legal"
	"renop/internal/service/token"
)

const githubOAuthStateTTL = 10 * time.Minute
const githubOAuthCookie = "renop_oauth_github"

type githubProfileStatus struct {
	GitHubLogin    string `json:"github_login,omitempty"`
	AuthorizedAt   int64  `json:"authorized_at,omitempty"`
	PrincipalCount int    `json:"principal_count,omitempty"`
	Configured     bool   `json:"configured"`
	Linked         bool   `json:"linked"`
	CanDisconnect  bool   `json:"can_disconnect"`
}

func setupGitHubRoutes(auth fiber.Router, state *core.AppState, opChan chan<- token.TokenOp) {
	setupGitHubRoutesWithProvider(auth, state, opChan, defaultGitHubOAuthProvider)
}

func setupGitHubRoutesWithProvider(auth fiber.Router, state *core.AppState, opChan chan<- token.TokenOp,
	provider githubOAuthProvider) {
	auth.Get("/github/status", func(c fiber.Ctx) error { return getGitHubStatus(c, state) })
	auth.Get("/github/start", func(c fiber.Ctx) error { return startGitHubOAuth(c, state, provider) })
	auth.Get("/github/callback", func(c fiber.Ctx) error {
		return finishGitHubOAuth(c, state, opChan, provider)
	})
	auth.Get("/profile/github", func(c fiber.Ctx) error { return getProfileGitHub(c, state) })
	auth.Delete("/profile/github", func(c fiber.Ctx) error { return deleteProfileGitHub(c, state) })
	auth.Post("/profile/avatar/github", func(c fiber.Ctx) error { return syncGitHubAvatar(c, state, provider) })
}

func getGitHubStatus(c fiber.Ctx, state *core.AppState) error {
	cfg := state.Inner.Config.Load()
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"enabled": cfg != nil && cfg.Server.GitHubOAuth.Configured()})
}

func safeOAuthReturnTo(value string) string {
	if len(value) == 0 || len(value) > 1024 || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "/"
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host != "" || parsed.Scheme != "" || parsed.Path == "" ||
		strings.HasPrefix(parsed.Path, "/api/auth/") {
		return "/"
	}
	return parsed.EscapedPath()
}

func newOAuthState() (string, error) {
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(random[:]), nil
}

func currentSessionProfile(c fiber.Ctx, state *core.AppState) (*core.UserProfile, error) {
	if currentSession, ok := c.Locals("current_session_id").(string); !ok || strings.TrimSpace(currentSession) == "" {
		return nil, nil
	}
	user := GetUser(c)
	if user == nil || user.Username == "" || user.Username == "guest" {
		return nil, nil
	}
	profile, err := state.GetDB().GetUserProfile(user.Username)
	if errors.Is(err, core.ErrUserProfileNotFound) {
		return nil, nil
	}
	return profile, err
}

func startGitHubOAuth(c fiber.Ctx, state *core.AppState, provider githubOAuthProvider) error {
	setPrivateResponseHeaders(c)
	cfg := state.Inner.Config.Load()
	if cfg == nil || !cfg.Server.GitHubOAuth.Configured() {
		return c.SendStatus(fiber.StatusNotFound)
	}
	callback, err := url.Parse(cfg.Server.GitHubOAuth.CallbackURL)
	if err != nil || callback.Scheme == "" || callback.Host == "" {
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	returnTo := safeOAuthReturnTo(c.Query("return_to"))
	profile, err := currentSessionProfile(c, state)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("GitHub login is unavailable")
	}
	if c.Query("intent") == "register" && !cfg.Registration.Enabled {
		return registrationError(c, core.ErrRegistrationDisabled)
	}
	if c.Query("intent") == "login" || c.Query("intent") == "register" {
		profile = nil
	}
	if profile == nil && c.Query("intent") != "email" {
		if err := legal.RequireConsent(c, state); err != nil {
			return err
		}
	}
	if profile != nil && returnTo == "/" {
		returnTo = "/user/" + url.PathEscape(profile.Username) + "/edit"
	}
	rawState, err := newOAuthState()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("GitHub login is unavailable")
	}
	record := core.TransientAuthState{
		Provider: "github", ReturnTo: returnTo,
		ExpiresAt: time.Now().Add(githubOAuthStateTTL).UnixMilli(),
	}
	if profile != nil {
		record.UserID = profile.UserID
	}
	if profile != nil || c.Query("intent") == "email" {
		session, _ := c.Locals("current_session_id").(string)
		if profile == nil || session == "" || c.Cookies(sessionCookieName) != session {
			return oauthResultRedirect(c, returnTo, "session_changed")
		}
		account, err := state.GetDB().GetMFAState(profile.Username)
		if err != nil {
			return oauthResultRedirect(c, returnTo, "email_failed")
		}
		if c.Query("intent") == "email" {
			record.Intent = "email"
		}
		record.SessionHash = fmt.Sprintf("%x", sha256.Sum256([]byte(session)))
		record.Snapshot = account.Snapshot
	}
	if state.Inner.ExternalAuthStates == nil ||
		!state.Inner.ExternalAuthStates.Put(rawState, record, time.Now().UnixMilli()) {
		return c.Status(fiber.StatusServiceUnavailable).SendString("GitHub login is unavailable")
	}
	query := url.Values{
		"client_id":    {cfg.Server.GitHubOAuth.ClientID},
		"redirect_uri": {cfg.Server.GitHubOAuth.CallbackURL},
		"scope":        {"read:user read:org user:email"},
		"state":        {rawState},
	}
	if record.Intent == "email" {
		query.Set("scope", "user:email")
	}
	c.Cookie(&fiber.Cookie{Name: githubOAuthCookie, Value: rawState, Path: "/api/auth/github",
		MaxAge: int(githubOAuthStateTTL.Seconds()), Expires: time.Now().Add(githubOAuthStateTTL),
		Secure: isSecure(c), HTTPOnly: true, SameSite: "Lax"})
	return c.Redirect().To(provider.AuthorizeURL + "?" + query.Encode())
}

func oauthResultRedirect(c fiber.Ctx, returnTo, result string) error {
	target := safeOAuthReturnTo(returnTo)
	return c.Redirect().Status(fiber.StatusSeeOther).To(target + "?" +
		url.Values{"github_oauth": {result}}.Encode())
}

func finishGitHubOAuth(c fiber.Ctx, state *core.AppState, opChan chan<- token.TokenOp,
	provider githubOAuthProvider) error {
	setPrivateResponseHeaders(c)
	if state.Inner.ExternalAuthStates == nil {
		return oauthResultRedirect(c, "/", "state_invalid")
	}
	rawState := c.Query("state")
	if len(rawState) != 43 || subtle.ConstantTimeCompare([]byte(rawState), []byte(c.Cookies(githubOAuthCookie))) != 1 {
		return oauthResultRedirect(c, "/", "state_invalid")
	}
	record, ok := state.Inner.ExternalAuthStates.Consume(rawState, "github", time.Now().UnixMilli())
	if !ok {
		return oauthResultRedirect(c, "/", "state_invalid")
	}
	if c.Query("error") != "" {
		return oauthResultRedirect(c, record.ReturnTo, "provider_denied")
	}
	code := strings.TrimSpace(c.Query("code"))
	if code == "" || len(code) > 512 {
		return oauthResultRedirect(c, record.ReturnTo, "exchange_failed")
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil || !cfg.Server.GitHubOAuth.Configured() {
		return oauthResultRedirect(c, record.ReturnTo, "configuration_changed")
	}
	client, err := oauthHTTPClient(cfg)
	if err != nil {
		log.Printf("Failed to configure GitHub OAuth client: %v", err)
		return oauthResultRedirect(c, record.ReturnTo, "exchange_failed")
	}
	if transport, transportOK := client.Transport.(*http.Transport); transportOK {
		defer transport.CloseIdleConnections()
	}
	ctx, cancel := context.WithTimeout(c.Context(), 20*time.Second)
	defer cancel()
	tokenResponse, err := exchangeGitHubCode(ctx, client, provider, cfg.Server.GitHubOAuth, code)
	if err != nil {
		log.Printf("GitHub OAuth code exchange failed: %v", err)
		return oauthResultRedirect(c, record.ReturnTo, "exchange_failed")
	}
	if record.Intent == "email" {
		return finishGitHubEmailVerification(c, state, record, ctx, client, provider, tokenResponse.AccessToken)
	}
	if !githubScopesAuthorized(tokenResponse.Scope) {
		return oauthResultRedirect(c, record.ReturnTo, "scope_missing")
	}
	identity, principals, err := fetchGitHubAuthorization(ctx, client, provider, tokenResponse.AccessToken)
	if err != nil {
		log.Printf("GitHub OAuth identity lookup failed: %v", err)
		return oauthResultRedirect(c, record.ReturnTo, "identity_failed")
	}
	identity.PrimaryEmail, identity.Emails, err = fetchGitHubEmailAddresses(ctx, client, provider, tokenResponse.AccessToken)
	if err != nil {
		if code := providerEmailErrorCode(err); code != "" {
			return oauthResultRedirect(c, record.ReturnTo, code)
		}
		return oauthResultRedirect(c, record.ReturnTo, "email_failed")
	}
	authorizedAt := time.Now().UnixMilli()
	for index := range principals {
		principals[index].AuthorizedAt = authorizedAt
	}
	if record.UserID != "" {
		currentProfile, err := currentSessionProfile(c, state)
		session, _ := c.Locals("current_session_id").(string)
		if err != nil || currentProfile == nil || currentProfile.UserID != record.UserID || session == "" ||
			c.Cookies(sessionCookieName) != session || registrationHash(session) != record.SessionHash {
			return oauthResultRedirect(c, record.ReturnTo, "session_changed")
		}
		err = state.GetDB().LinkGitHubIdentity(currentProfile.Username, session, record.Snapshot, identity.ID, identity.Login,
			principals, authorizedAt, identity.Emails...)
		if errors.Is(err, core.ErrGitHubIdentityLinked) {
			return oauthResultRedirect(c, record.ReturnTo, "identity_linked")
		}
		if code := providerEmailErrorCode(err); code != "" {
			return oauthResultRedirect(c, record.ReturnTo, code)
		}
		if err != nil {
			log.Printf("Failed to link GitHub identity: %v", err)
			return oauthResultRedirect(c, record.ReturnTo, "identity_failed")
		}
		username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
		state.InvalidateAccountAuthCache(true, currentProfile.Username)
		audit.Log(state, &core.AuditLogEntry{
			Username: username, Operator: operator, Action: audit.ActionProfileUpdate,
			Details: "Connected GitHub account", AuthMethod: authMethod, SessionID: sessionID, IP: ip,
		})
		return oauthResultRedirect(c, record.ReturnTo, "linked")
	}
	user, err := resolveGitHubLogin(state, identity, principals, authorizedAt)
	if code := providerEmailErrorCode(err); code != "" {
		return oauthResultRedirect(c, record.ReturnTo, code)
	}
	if errors.Is(err, core.ErrGitHubIdentityNotFound) {
		return beginGitHubRegistration(c, state, record, identity, principals)
	}
	if errors.Is(err, core.ErrGitHubIdentityLinked) {
		return oauthResultRedirect(c, record.ReturnTo, "identity_linked")
	}
	if errors.Is(err, core.ErrAccountBanned) {
		return oauthResultRedirect(c, record.ReturnTo, "account_banned")
	}
	if errors.Is(err, core.ErrAccountDeleted) {
		return oauthResultRedirect(c, record.ReturnTo, "account_deleted")
	}
	if err != nil {
		log.Printf("Failed to resolve GitHub login: %v", err)
		return oauthResultRedirect(c, record.ReturnTo, "identity_failed")
	}
	mfa, err := state.GetDB().GetMFAState(user.Username)
	if err != nil || mfa.GitHubID != identity.ID {
		return oauthResultRedirect(c, record.ReturnTo, "session_failed")
	}
	user.AuthenticationSnapshot = mfa.Snapshot
	if err := issueBrowserSession(c, state, user, "github"); err != nil {
		if errors.Is(err, legal.ErrConsentRequired) {
			return oauthResultRedirect(c, "/account/login", legal.ConsentErrorCode)
		}
		if errors.Is(err, errMFARequired) {
			return c.Redirect().To("/account/login?mfa=1&return_to=" + url.QueryEscape(record.ReturnTo))
		}
		if errors.Is(err, core.ErrAccountBanned) {
			return oauthResultRedirect(c, record.ReturnTo, "account_banned")
		}
		if errors.Is(err, core.ErrAccountDeleted) {
			return oauthResultRedirect(c, record.ReturnTo, "account_deleted")
		}
		log.Printf("Failed to create GitHub browser session: %v", err)
		return oauthResultRedirect(c, record.ReturnTo, "session_failed")
	}
	return oauthResultRedirect(c, record.ReturnTo, "success")
}

func getProfileGitHub(c fiber.Ctx, state *core.AppState) error {
	user := GetUser(c)
	if user == nil || user.Username == "" || user.Username == "guest" {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	status, err := githubProfileStatusForAccount(state, user.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load GitHub identity")
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(status)
}

func githubProfileStatusForAccount(state *core.AppState, username string) (githubProfileStatus, error) {
	cfg := state.Inner.Config.Load()
	status := githubProfileStatus{Configured: cfg != nil && cfg.Server.GitHubOAuth.Configured()}
	identity, err := state.GetDB().GetGitHubIdentity(username)
	if err != nil {
		return status, err
	}
	if identity != nil {
		status.Linked = true
		status.GitHubLogin = identity.GitHubLogin
		status.AuthorizedAt = identity.AuthorizedAt
		status.PrincipalCount = identity.PrincipalCount
		status.CanDisconnect, err = canDisconnectGitHub(state, username)
		if err != nil {
			return status, err
		}
	}
	return status, nil
}

func canDisconnectGitHub(state *core.AppState, username string) (bool, error) {
	security, err := state.GetDB().GetAccountSecurity(username)
	if err != nil {
		return false, err
	}
	return (security.FidoDeviceCount > 0 && !security.PasskeySecondFactor) ||
		(security.PasswordConfigured && security.PasswordLoginEnabled) || security.OAuthIdentityCount > 0, nil
}

func deleteProfileGitHub(c fiber.Ctx, state *core.AppState) error {
	user := GetUser(c)
	if user == nil || user.Username == "" || user.Username == "guest" {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	currentSession, ok := c.Locals("current_session_id").(string)
	if !ok || strings.TrimSpace(currentSession) == "" {
		return c.Status(fiber.StatusForbidden).SendString("A browser session is required")
	}
	canDisconnect, err := canDisconnectGitHub(state, user.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to inspect login methods")
	}
	if !canDisconnect {
		c.Set("X-Renop-Error-Code", "GITHUB_LAST_LOGIN_METHOD")
		return c.Status(fiber.StatusConflict).SendString("Another login method is required")
	}
	if err := state.GetDB().DeleteGitHubIdentity(user.Username); errors.Is(err, core.ErrLastLoginMethod) {
		c.Set("X-Renop-Error-Code", "GITHUB_LAST_LOGIN_METHOD")
		return c.Status(fiber.StatusConflict).SendString("Another login method is required")
	} else if err != nil {
		if errors.Is(err, core.ErrGitHubIdentityNotFound) {
			return c.SendStatus(fiber.StatusNotFound)
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to disconnect GitHub account")
	}
	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionProfileUpdate,
		Details: "Disconnected GitHub account", AuthMethod: authMethod, SessionID: sessionID, IP: ip,
	})
	return c.SendStatus(fiber.StatusNoContent)
}
