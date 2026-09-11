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
	"crypto/rsa"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emmansun/base64"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/mailqueue"

	"github.com/goccy/go-json"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestOAuthRegistrationAndMFALogin(t *testing.T) {
	challenge := ""
	avatarURL := ""
	var verifiedEmail any = "true"
	providerEmail := "confirmed@example.com"
	var changeAvatarConfig atomic.Bool
	var replaceOAuthDuringAvatar func()
	avatarData := avatarPNG(t, 256, 256)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			require.NoError(t, r.ParseForm())
			require.Equal(t, "client", r.Form.Get("client_id"))
			require.Equal(t, "secret", r.Form.Get("client_secret"))
			require.Len(t, r.Form.Get("code_verifier"), 43)
			digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			require.Equal(t, challenge, base64.RawURLEncoding.EncodeToString(digest[:]))
			_, _ = io.WriteString(w, `{"access_token":"provider-secret","token_type":"bearer"}`)
		case "/userinfo":
			require.Equal(t, "Bearer provider-secret", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": int64(9007199254740993), "username": "external",
				"name": "Provider Name", "email": providerEmail, "verified": verifiedEmail, "avatar": avatarURL}})
		case "/avatar":
			require.Equal(t, "Bearer provider-secret", r.Header.Get("Authorization"))
			if changeAvatarConfig.Load() {
				replaceOAuthDuringAvatar()
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(avatarData)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(provider.Close)
	avatarURL = provider.URL + "/avatar"
	app, state, cfg := registrationTestApp(t)
	cfg.Registration.Enabled, cfg.Mail.Enabled = true, true
	cfg.MFAEncryptionKey = base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	p := config.OAuthProviderConfig{ID: "demo", Type: "custom", Name: "Example", Enabled: true, ClientID: "client", ClientSecret: "secret",
		CallbackURL: "https://renop.example/api/auth/oauth/demo/callback", AuthorizeURL: provider.URL + "/authorize",
		TokenURL: provider.URL + "/token", UserInfoURL: provider.URL + "/userinfo",
		Claims: config.OAuthClaims{Subject: "user.id", Username: "user.username", Name: "user.name", Email: "user.email", EmailVerified: "user.verified", Avatar: "user.avatar"}}
	require.NoError(t, p.Validate())
	cfg.Server.OAuthProviders = []config.OAuthProviderConfig{p}
	state.Inner.Config.Store(cfg)
	replaceOAuthDuringAvatar = func() {
		next := cfg.DeepCopy()
		next.Server.OAuthProviders[0].Enabled = false
		state.Inner.ConfigWriteLock.Lock()
		state.Inner.Config.Store(next)
		state.Inner.ConfigWriteLock.Unlock()
	}
	setupOAuthRoutes(app.Group("/api/auth"), state)
	setupMFARoutes(app.Group("/api/auth"), state)
	flow := func() (*http.Response, string) {
		start := registrationRequest(t, app, "/api/auth/oauth/demo/start?intent=login&return_to=/packages", nil, nil)
		require.Equal(t, 303, start.StatusCode)
		u, err := url.Parse(start.Header.Get("Location"))
		require.NoError(t, err)
		challenge = u.Query().Get("code_challenge")
		require.Equal(t, "S256", u.Query().Get("code_challenge_method"))
		return start, "/api/auth/oauth/demo/callback?code=code&state=" + url.QueryEscape(u.Query().Get("state"))
	}
	start, callback := flow()
	foreign := registrationRequest(t, app, callback, nil, nil)
	require.Contains(t, foreign.Header.Get("Location"), "oauth=state_invalid")
	response := registrationRequest(t, app, callback, nil, start.Cookies()[0])
	require.Contains(t, response.Header.Get("Location"), "/account/register?provider=demo")
	cookie := response.Cookies()[0]
	pendingResponse := registrationRequest(t, app, "/api/auth/registration/pending", nil, cookie)
	body, err := io.ReadAll(pendingResponse.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"email_required":true`)
	require.NotContains(t, string(body), "9007199254740993")
	require.NotContains(t, string(body), "provider-secret")
	pending, err := state.GetDB().GetPendingRegistration(registrationHash(cookie.Value), time.Now().UnixMilli())
	require.NoError(t, err)
	require.NotContains(t, pending.ProfileJSON, "provider-secret")
	require.Contains(t, pending.ProfileJSON, "avatar_token")
	request := map[string]any{"provider": "demo", "username": "external", "nickname": "Provider Name", "password": "Password2026!", "email": providerEmail, "import_avatar": true}
	response = registrationRequest(t, app, "/api/auth/registration", request, cookie)
	require.Equal(t, 400, response.StatusCode)
	response = registrationRequest(t, app, "/api/auth/registration/code", map[string]any{"provider": "demo", "email": "different@example.com"}, cookie)
	require.Equal(t, 409, response.StatusCode)
	require.Equal(t, "ACCOUNT_EMAIL_PROOF_REQUIRED", response.Header.Get("X-Renop-Error-Code"))
	request["email"] = "confirmed@example.com"
	response = registrationRequest(t, app, "/api/auth/registration/code", map[string]any{"provider": "demo", "email": request["email"]}, cookie)
	require.Equal(t, 202, response.StatusCode)
	var receipt mailqueue.Receipt
	require.NoError(t, json.NewDecoder(response.Body).Decode(&receipt))
	job, err := state.GetDB().GetMailJob(receipt.ID, cfg.Mail.EncryptionKey)
	require.NoError(t, err)
	request["code"] = regexp.MustCompile(`(?m)^\d{8}$`).FindString(job.Message.Text)
	response = registrationRequest(t, app, "/api/auth/registration", request, cookie)
	require.Equal(t, 201, response.StatusCode)
	avatar, err := state.GetDB().GetUserAvatar("external")
	require.NoError(t, err)
	require.NotNil(t, avatar)
	for _, saved := range response.Cookies() {
		require.NotEqual(t, sessionCookieName, saved.Name, "registration must not create a browser session")
	}
	identities, err := state.GetDB().GetOAuthIdentities("external")
	require.NoError(t, err)
	require.Len(t, identities, 1)
	require.Equal(t, "9007199254740993", identities[0].Subject, "numeric identities must not lose precision")
	start, callback = flow()
	response = registrationRequest(t, app, callback, nil, start.Cookies()[0])
	require.Contains(t, response.Header.Get("Location"), "oauth=success")
	require.Equal(t, sessionCookieName, response.Cookies()[0].Name)
	sessionCookie := response.Cookies()[0]
	profileFlow := func(intent string) *http.Response {
		start := registrationRequest(t, app, "/api/auth/oauth/demo/start?intent="+intent+"&return_to=/user/external", nil, sessionCookie)
		require.Equal(t, 303, start.StatusCode)
		u, err := url.Parse(start.Header.Get("Location"))
		require.NoError(t, err)
		challenge = u.Query().Get("code_challenge")
		request := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/demo/callback?code=code&state="+url.QueryEscape(u.Query().Get("state")), nil)
		request.AddCookie(sessionCookie)
		request.AddCookie(start.Cookies()[0])
		result, err := app.Test(request)
		require.NoError(t, err)
		t.Cleanup(func() { _ = result.Body.Close() })
		return result
	}
	status := registrationRequest(t, app, "/api/auth/profile/oauth", nil, sessionCookie)
	var statuses struct {
		Providers []oauthProfileStatus `json:"providers"`
	}
	require.NoError(t, json.NewDecoder(status.Body).Decode(&statuses))
	require.Len(t, statuses.Providers, 1)
	require.True(t, statuses.Providers[0].CanDisconnect)
	require.True(t, statuses.Providers[0].CanVerifyEmail)
	require.True(t, statuses.Providers[0].CanImportAvatar)
	require.Contains(t, profileFlow("link").Header.Get("Location"), "oauth=linked")
	require.NoError(t, state.GetDB().SaveToken(&core.AccessToken{Name: "otherowner", EncryptedSecret: "password", Permissions: []string{"base"}}))
	_, err = state.GetDB().UpdateAccountEmail("otherowner", "taken@example.com", time.Now().UnixMilli())
	require.NoError(t, err)
	providerEmail = "taken@example.com"
	verifiedEmail = true
	require.Contains(t, profileFlow("link").Header.Get("Location"), "oauth=email_conflict")
	providerEmail, verifiedEmail = "confirmed@example.com", "true"
	require.Contains(t, profileFlow("email").Header.Get("Location"), "oauth=email_missing")
	verifiedEmail = true
	providerEmail = "suggested@example.com"
	require.Contains(t, profileFlow("email").Header.Get("Location"), "oauth=email_updated")
	security, err := state.GetDB().GetAccountSecurity("external")
	require.NoError(t, err)
	require.Equal(t, "suggested@example.com", security.Email)
	require.Contains(t, profileFlow("avatar").Header.Get("Location"), "oauth=avatar_updated")
	changeAvatarConfig.Store(true)
	require.Contains(t, profileFlow("avatar").Header.Get("Location"), "oauth=configuration_changed")
	changeAvatarConfig.Store(false)
	state.Inner.Config.Store(cfg)
	replayed := registrationRequest(t, app, callback, nil, start.Cookies()[0])
	require.Contains(t, replayed.Header.Get("Location"), "oauth=state_invalid")
	mfa, err := state.GetDB().GetMFAState("external")
	require.NoError(t, err)
	secret := "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	sealed, err := encryptMFASecret(state, mfa.UserID, secret)
	require.NoError(t, err)
	require.NoError(t, state.GetDB().UpdateMFA("external", mfa.Snapshot, sealed, false, -1, sessionCookie.Value))
	start, callback = flow()
	response = registrationRequest(t, app, callback, nil, start.Cookies()[0])
	require.Contains(t, response.Header.Get("Location"), "/account/login?mfa=1")
	require.Equal(t, mfaCookieName, response.Cookies()[0].Name)
	verified := registrationRequest(t, app, "/api/auth/mfa/totp", map[string]any{"code": core.TOTPCode(secret, time.Now().Unix()/30)}, response.Cookies()[0])
	require.Equal(t, 200, verified.StatusCode)
	start, callback = flow()
	next := cfg.DeepCopy()
	next.Server.OAuthProviders[0].ClientSecret = "rotated"
	state.Inner.Config.Store(next)
	response = registrationRequest(t, app, callback, nil, start.Cookies()[0])
	require.Contains(t, response.Header.Get("Location"), "oauth=configuration_changed")
}

func TestOAuthGitLabOwnedNamespaces(t *testing.T) {
	raw := json.RawMessage(`{"preferred_username":"Alice","groups":["joined-public"],"https://gitlab.org/claims/groups/owner":["Owned-Group","parent/owned-child","alice","invalid.name"]}`)
	require.Equal(t, []string{"alice", "owned-group"}, gitlabOwnedNamespaces(raw))
	require.Empty(t, gitlabOwnedNamespaces(json.RawMessage(`{"groups":["joined-public"]}`)))
}

func TestOAuthIDTokenValidation(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kid": "signing", "kty": "RSA", "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
	}))
	t.Cleanup(provider.Close)
	p := config.OAuthProviderConfig{Type: "custom", ClientID: "client", Issuer: provider.URL, JWKSURL: provider.URL}
	for _, scenario := range []string{"valid", "audience", "issuer", "nonce", "expired", "missing_expiry", "future", "subject", "presenter", "access_hash", "signature", "algorithm"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Now().Unix()
			claims := jwt.MapClaims{"sub": "stable", "iss": provider.URL, "aud": "client", "nonce": "nonce", "iat": now, "exp": now + 300}
			switch scenario {
			case "audience":
				claims["aud"] = "other"
			case "issuer":
				claims["iss"] = "https://other.example"
			case "nonce":
				claims["nonce"] = "other"
			case "expired":
				claims["exp"] = now - 60
			case "missing_expiry":
				delete(claims, "exp")
			case "future":
				claims["iat"] = now + 60
			case "subject":
				delete(claims, "sub")
			case "presenter":
				claims["azp"] = "other"
			case "access_hash":
				claims["at_hash"] = "wrong"
			}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			token.Header["kid"] = "signing"
			signed, err := token.SignedString(key)
			require.NoError(t, err)
			if scenario == "signature" {
				signed += "a"
			}
			if scenario == "algorithm" {
				token = jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				token.Header["kid"] = "signing"
				signed, err = token.SignedString([]byte("secret"))
				require.NoError(t, err)
			}
			_, err = verifyOAuthIDToken(context.Background(), provider.Client(), p, oauthTokens{IDToken: signed, AccessToken: "access"}, "nonce")
			if scenario == "valid" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	p.Type, p.Tenant = "microsoft", "organizations"
	require.False(t, oauthIssuerAllowed(p, "https://login.microsoftonline.com/9188040d-6c67-4c5b-b112-36a304b66dad/v2.0", "9188040d-6c67-4c5b-b112-36a304b66dad"))
	require.False(t, oauthIssuerAllowed(p, "https://other.example/tenant", "not-a-tenant"))
	p.Tenant = "common"
	require.True(t, oauthIssuerAllowed(p, "https://login.microsoftonline.com/9188040d-6c67-4c5b-b112-36a304b66dad/v2.0", "9188040d-6c67-4c5b-b112-36a304b66dad"))
	require.Equal(t, "", oauthString(json.RawMessage(`{"id":9007199254740993.0}`), "id", true))
	require.False(t, oauthAvatarUsesToken(config.OAuthProviderConfig{Type: "microsoft"}, "https://attacker.example/photo"))
	require.False(t, config.ValidOAuthURL("https://user:secret@example.com/avatar"))
	require.False(t, config.ValidOAuthURL("http://169.254.169.254/latest/meta-data"))
}

func TestCloudflareOAuthFlow(t *testing.T) {
	var receivedAuthHeader string
	var receivedForm url.Values
	var tokenMode string
	var returnSub string = "cf-user-999"
	var serveCFUser bool

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			require.NoError(t, r.ParseForm())
			receivedAuthHeader = r.Header.Get("Authorization")
			receivedForm = r.Form
			if tokenMode == "require_post" && receivedAuthHeader != "" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":             "invalid_client",
					"error_description": "Client authentication failed",
				})
				return
			}
			if tokenMode == "require_basic" && (receivedAuthHeader == "" || receivedForm.Get("client_secret") != "") {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":             "invalid_client",
					"error_description": "Client authentication failed",
				})
				return
			}
			if tokenMode == "always_fail_401" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":             "invalid_client",
					"error_description": "Invalid client credentials",
				})
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"cf-access-token","token_type":"Bearer"}`)
		case "/userinfo":
			require.Equal(t, "Bearer cf-access-token", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(map[string]any{"sub": returnSub})
		case "/client/v4/user":
			require.Equal(t, "Bearer cf-access-token", r.Header.Get("Authorization"))
			if !serveCFUser {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errors": []string{"forbidden"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result": map[string]any{
					"id":         returnSub,
					"email":      "cfuser@example.com",
					"username":   "cfuser",
					"first_name": "Cloud",
					"last_name":  "Flare",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mockServer.Close)

	app, state, cfg := registrationTestApp(t)
	cfg.Registration.Enabled = true

	// 1. Test startOAuth URL generation without scopes or nonce (defaults to client_secret_post)
	cfProvider := config.OAuthProviderConfig{
		ID: "cf", Type: "cloudflare", Name: "Cloudflare", Enabled: true,
		ClientID: "  cf-client-id  ", ClientSecret: "  cf-secret \n",
		CallbackURL: "https://renop.example/api/auth/oauth/cf/callback",
	}.Resolved()
	require.Equal(t, "client_secret_post", cfProvider.TokenAuth)
	require.Equal(t, "cf-client-id", cfProvider.ClientID)
	require.Equal(t, "cf-secret", cfProvider.ClientSecret)
	cfProvider.AuthorizeURL = mockServer.URL + "/auth"
	cfProvider.TokenURL = mockServer.URL + "/token"
	cfProvider.UserInfoURL = mockServer.URL + "/userinfo"
	cfProvider.BaseURL = mockServer.URL
	require.NoError(t, cfProvider.Validate())

	cfg.Server.OAuthProviders = []config.OAuthProviderConfig{cfProvider}
	state.Inner.Config.Store(cfg)
	setupOAuthRoutes(app.Group("/api/auth"), state)

	startResp := registrationRequest(t, app, "/api/auth/oauth/cf/start?intent=login", nil, nil)
	require.Equal(t, 303, startResp.StatusCode)
	targetURL, err := url.Parse(startResp.Header.Get("Location"))
	require.NoError(t, err)

	require.Equal(t, "cf-client-id", targetURL.Query().Get("client_id"))
	require.Equal(t, "https://renop.example/api/auth/oauth/cf/callback", targetURL.Query().Get("redirect_uri"))
	require.Equal(t, "code", targetURL.Query().Get("response_type"))
	require.Empty(t, targetURL.Query().Get("scope"), "Cloudflare must not send scopes by default")
	require.Empty(t, targetURL.Query().Get("nonce"), "Cloudflare must not send nonce")
	require.NotEmpty(t, targetURL.Query().Get("code_challenge"))

	// 2. Test startOAuth with comma-separated scopes normalized to space-separated
	cfProviderWithScopes := cfProvider
	cfProviderWithScopes.Scopes = "memberships.read, user-details.read, offline_access, openid"
	cfg.Server.OAuthProviders = []config.OAuthProviderConfig{cfProviderWithScopes}
	state.Inner.Config.Store(cfg)

	startWithScopes := registrationRequest(t, app, "/api/auth/oauth/cf/start?intent=login", nil, nil)
	require.Equal(t, 303, startWithScopes.StatusCode)
	targetURLWithScopes, err := url.Parse(startWithScopes.Header.Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "memberships.read user-details.read offline_access openid", targetURLWithScopes.Query().Get("scope"))

	// 3. Test code exchange with default client_secret_post (and verify whitespace trimmed)
	receivedAuthHeader = ""
	receivedForm = nil
	tokens, err := exchangeOAuthCode(context.Background(), mockServer.Client(), cfProvider, "code-1", "verifier-1")
	require.NoError(t, err)
	require.Equal(t, "cf-access-token", tokens.AccessToken)
	require.Equal(t, "cf-client-id", receivedForm.Get("client_id"))
	require.Equal(t, "cf-secret", receivedForm.Get("client_secret"))
	require.Equal(t, "verifier-1", receivedForm.Get("code_verifier"))
	require.Empty(t, receivedAuthHeader)

	// 4. Test code exchange with explicit client_secret_basic
	cfBasic := cfProvider
	cfBasic.TokenAuth = "client_secret_basic"
	receivedAuthHeader = ""
	receivedForm = nil
	tokens, err = exchangeOAuthCode(context.Background(), mockServer.Client(), cfBasic, "code-2", "verifier-2")
	require.NoError(t, err)
	require.Equal(t, "cf-access-token", tokens.AccessToken)
	require.NotEmpty(t, receivedAuthHeader)
	require.Empty(t, receivedForm.Get("client_id"), "client_secret_basic must not send client_id in POST body")
	require.Empty(t, receivedForm.Get("client_secret"))
	require.Equal(t, "verifier-2", receivedForm.Get("code_verifier"))

	// 5. Test automatic fallback: configured as client_secret_post, but server requires client_secret_basic
	tokenMode = "require_basic"
	receivedAuthHeader = ""
	receivedForm = nil
	tokens, err = exchangeOAuthCode(context.Background(), mockServer.Client(), cfProvider, "code-fallback-basic", "verifier-fb1")
	require.NoError(t, err, "must fall back to client_secret_basic when client_secret_post returns 401")
	require.Equal(t, "cf-access-token", tokens.AccessToken)
	require.NotEmpty(t, receivedAuthHeader)
	require.Empty(t, receivedForm.Get("client_secret"))

	// 6. Test automatic fallback: configured as client_secret_basic, but server requires client_secret_post
	tokenMode = "require_post"
	receivedAuthHeader = ""
	receivedForm = nil
	tokens, err = exchangeOAuthCode(context.Background(), mockServer.Client(), cfBasic, "code-fallback-post", "verifier-fb2")
	require.NoError(t, err, "must fall back to client_secret_post when client_secret_basic returns 401")
	require.Equal(t, "cf-access-token", tokens.AccessToken)
	require.Equal(t, "cf-client-id", receivedForm.Get("client_id"))
	require.Equal(t, "cf-secret", receivedForm.Get("client_secret"))

	// 7. Test detailed error message when authentication fails
	tokenMode = "always_fail_401"
	_, err = exchangeOAuthCode(context.Background(), mockServer.Client(), cfProvider, "code-bad", "verifier-bad")
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
	require.Contains(t, err.Error(), "invalid_client")
	require.Contains(t, err.Error(), "Invalid client credentials")
	tokenMode = ""

	// 8. Test code exchange with none (PKCE only)
	cfPKCE := cfProvider
	cfPKCE.TokenAuth = "none"
	cfPKCE.ClientSecret = ""
	receivedAuthHeader = ""
	receivedForm = nil
	tokens, err = exchangeOAuthCode(context.Background(), mockServer.Client(), cfPKCE, "code-3", "verifier-3")
	require.NoError(t, err)
	require.Equal(t, "cf-access-token", tokens.AccessToken)
	require.Equal(t, "cf-client-id", receivedForm.Get("client_id"))
	require.Empty(t, receivedForm.Get("client_secret"))
	require.Equal(t, "verifier-3", receivedForm.Get("code_verifier"))
	require.Empty(t, receivedAuthHeader)

	// 9. Test fetchOAuthUserInfo returns subject when /client/v4/user is unavailable
	serveCFUser = false
	info, err := fetchOAuthUserInfo(context.Background(), mockServer.Client(), cfProvider, tokens, "")
	require.NoError(t, err)
	require.Equal(t, "cf-user-999", info.Identity.Subject)
	require.Equal(t, cfProvider.Authority(""), info.Identity.Authority)
	require.Empty(t, info.Email)

	// 10. Test fetchOAuthUserInfo automatically enriches verified email and user profile from /client/v4/user
	serveCFUser = true
	infoEnriched, err := fetchOAuthUserInfo(context.Background(), mockServer.Client(), cfProvider, tokens, "")
	require.NoError(t, err)
	require.Equal(t, "cf-user-999", infoEnriched.Identity.Subject)
	require.Equal(t, "cfuser@example.com", infoEnriched.Email)
	require.True(t, infoEnriched.EmailVerified)
	require.Equal(t, "cfuser", infoEnriched.Username)
	require.Equal(t, "Cloud Flare", infoEnriched.Name)
	require.Equal(t, "cfuser", infoEnriched.Identity.Login)
	require.Len(t, infoEnriched.Identity.Emails, 1)
	require.Equal(t, "cfuser@example.com", infoEnriched.Identity.Emails[0].Email)
	require.True(t, infoEnriched.Identity.Emails[0].Verified)
}
