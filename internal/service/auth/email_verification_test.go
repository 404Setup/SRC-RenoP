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
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/mail"
	"renop/internal/service/mailqueue"
	"renop/internal/testutil"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

func newEmailVerificationApp(t *testing.T) (*fiber.App, *core.AppState, *database.DB, *config.Config) {
	t.Helper()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "email-verification.db"), MaxOpenConns: 1, MaxIdleConns: 1})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state := core.NewAppState()
	state.Inner.DB = db
	cfg := config.DefaultConfig()
	cfg.Mail.Enabled, cfg.Mail.PublicURL = true, "https://repo.example"
	cfg.Mail.ManualRate.Limit = 100
	require.NoError(t, cfg.Mail.EnsureKey())
	account := mail.Presets()[0].Account
	account.ID, account.From = "primary", "sender@example.com"
	cfg.Mail.Accounts = []mail.Account{account}
	state.Inner.Config.Store(cfg)
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", EncryptedSecret: "password", Permissions: []string{"base"}}))
	_, err = db.UpdateAccountEmail("alice", "old@example.com", time.Now().UnixMilli())
	require.NoError(t, err)
	for _, id := range []string{"original-session", "other-session"} {
		session := &core.Session{PublicID: id, Username: "alice", CreatedAt: time.Now().UnixMilli(), LoginMethod: "password"}
		session.LastActive.Store(session.CreatedAt)
		require.NoError(t, state.SaveSession(session, id))
	}
	app := fiber.New()
	app.Use(AuthMiddleware(state))
	setupAccountSecurityRoutes(app.Group("/api/auth"), state)
	return app, state, db, cfg
}

func TestProfileEmailVerificationPreservesAddressUntilConfirmed(t *testing.T) {
	app, state, db, cfg := newEmailVerificationApp(t)
	profile, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	require.NoError(t, db.SetUserLocale("alice", "original-session", "zh-HK", profile.UserID))
	response := accountSecurityRequest(t, app, "PUT", "/api/auth/profile/email", map[string]string{"email": "New@Example.com"}, "original-session")
	require.Equal(t, 202, response.StatusCode)
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	var receipt mailqueue.Receipt
	require.NoError(t, json.NewDecoder(response.Body).Decode(&receipt))
	require.NoError(t, response.Body.Close())
	job, err := db.GetMailJob(receipt.ID, cfg.Mail.EncryptionKey)
	require.NoError(t, err)
	require.NotNil(t, job)
	require.Equal(t, "email_verify", job.Scene)
	require.Contains(t, job.Message.HTML, `lang="zh-HK"`)
	code := regexp.MustCompile(`(?m)^\d{8}$`).FindString(job.Message.Text)
	require.Len(t, code, 8)
	security, err := db.GetAccountSecurity("alice")
	require.NoError(t, err)
	require.Equal(t, "old@example.com", security.Email)
	var hash, sessionHash string
	require.NoError(t, db.QueryRow(`SELECT code_hash, session_hash FROM user_email_changes WHERE user_id = ?`, job.UserID).Scan(&hash, &sessionHash))
	require.NotContains(t, hash, code)
	require.NotContains(t, sessionHash, "original-session")

	response = accountSecurityRequest(t, app, "PUT", "/api/auth/profile/email", map[string]string{"email": "new@example.com"}, "original-session")
	require.Equal(t, 429, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = accountSecurityRequest(t, app, "POST", "/api/auth/profile/email/confirm", map[string]string{"email": "new@example.com", "code": code}, "other-session")
	require.Equal(t, 400, response.StatusCode)
	require.NoError(t, response.Body.Close())
	wrong := code[:7] + string('0'+(code[7]-'0'+1)%10)
	for index, value := range []string{wrong, code, code} {
		response = accountSecurityRequest(t, app, "POST", "/api/auth/profile/email/confirm", map[string]string{"email": "new@example.com", "code": value}, "original-session")
		expected := 400
		if index == 1 {
			expected = 200
			var result core.AccountSecurity
			require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
			require.Equal(t, "new@example.com", result.Email)
			require.True(t, result.EmailVerificationRequired)
		}
		require.Equal(t, expected, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	old, err := db.GetTokenByEmail("old@example.com")
	require.NoError(t, err)
	require.Nil(t, old)
	require.NotNil(t, state.GetSession("original-session"))

	cfg = cfg.DeepCopy()
	cfg.Mail.Enabled = false
	cfg.Mail.Addresses = []string{"@blocked.example"}
	state.Inner.Config.Store(cfg)
	response = accountSecurityRequest(t, app, "POST", "/api/auth/profile/email/confirm", map[string]string{}, "original-session")
	require.Equal(t, 404, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = accountSecurityRequest(t, app, "PUT", "/api/auth/profile/email", map[string]string{"email": "new@blocked.example"}, "original-session")
	require.Equal(t, 400, response.StatusCode)
	require.Equal(t, "mail_recipient_blocked", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())
	response = accountSecurityRequest(t, app, "PUT", "/api/auth/profile/email", map[string]string{"email": "offline@example.com"}, "original-session")
	require.Equal(t, 200, response.StatusCode)
	require.NoError(t, response.Body.Close())
	security, err = db.GetAccountSecurity("alice")
	require.NoError(t, err)
	require.Equal(t, "offline@example.com", security.Email)
}

func TestProfileEmailAliasRequiresProofAndKeepsPrimary(t *testing.T) {
	app, _, db, cfg := newEmailVerificationApp(t)
	response := accountSecurityRequest(t, app, "PUT", "/api/auth/profile/email", map[string]any{"email": "alias@example.com", "alias": true}, "original-session")
	require.Equal(t, 202, response.StatusCode)
	var receipt mailqueue.Receipt
	require.NoError(t, json.NewDecoder(response.Body).Decode(&receipt))
	require.NoError(t, response.Body.Close())
	job, err := db.GetMailJob(receipt.ID, cfg.Mail.EncryptionKey)
	require.NoError(t, err)
	code := regexp.MustCompile(`(?m)^\d{8}$`).FindString(job.Message.Text)
	response = accountSecurityRequest(t, app, "POST", "/api/auth/profile/email/confirm", map[string]string{"email": "alias@example.com", "code": code}, "original-session")
	require.Equal(t, 200, response.StatusCode)
	var security core.AccountSecurity
	require.NoError(t, json.NewDecoder(response.Body).Decode(&security))
	require.NoError(t, response.Body.Close())
	require.Equal(t, "old@example.com", security.Email)
	require.Equal(t, []string{"alias@example.com"}, security.EmailAliases)
	response = accountSecurityRequest(t, app, "DELETE", "/api/auth/profile/email/alias", map[string]string{"email": "old@example.com"}, "original-session")
	require.Equal(t, 409, response.StatusCode)
	require.Equal(t, "ACCOUNT_EMAIL_PRIMARY", response.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, response.Body.Close())
	response = accountSecurityRequest(t, app, "DELETE", "/api/auth/profile/email/alias", map[string]string{"email": "alias@example.com"}, "original-session")
	require.Equal(t, 200, response.StatusCode)
	require.NoError(t, json.NewDecoder(response.Body).Decode(&security))
	require.NoError(t, response.Body.Close())
	require.Empty(t, security.EmailAliases)
}

func TestGitHubEmailVerificationBindsSessionAndUsesVerifiedContact(t *testing.T) {
	app, state, db, cfg := newEmailVerificationApp(t)
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"provider-token","token_type":"bearer","scope":"user:email"}`))
		case "/user/emails":
			require.Equal(t, "Bearer provider-token", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`[{"email":"fake@example.com","primary":true,"verified":false},{"email":"123@users.noreply.github.com","verified":true},{"email":"Real@Example.com","verified":true}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(providerServer.Close)
	cfg = cfg.DeepCopy()
	cfg.Mail.Enabled = false
	cfg.Server.GitHubOAuth = config.GitHubOAuthConfig{Enabled: true, ClientID: "client", ClientSecret: "secret", CallbackURL: "https://repo.example/api/auth/github/callback"}
	state.Inner.Config.Store(cfg)
	provider := githubOAuthProvider{AuthorizeURL: providerServer.URL + "/authorize", TokenURL: providerServer.URL + "/token", APIURL: providerServer.URL}
	setupGitHubRoutesWithProvider(app.Group("/api/auth"), state, nil, provider)
	var oauthCookie *http.Cookie
	start := func() string {
		response := accountSecurityRequest(t, app, "GET", "/api/auth/github/start?intent=email&return_to=%2Fuser%2Falice%2Fedit", nil, "original-session")
		require.Equal(t, 303, response.StatusCode)
		location, err := url.Parse(response.Header.Get("Location"))
		require.NoError(t, err)
		require.Equal(t, "user:email", location.Query().Get("scope"))
		oauthCookie = response.Cookies()[0]
		require.NoError(t, response.Body.Close())
		return "/api/auth/github/callback?code=code&state=" + url.QueryEscape(location.Query().Get("state"))
	}
	callback := func(path, session, result string) {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session})
		request.AddCookie(oauthCookie)
		response, err := app.Test(request)
		require.NoError(t, err)
		require.Equal(t, 303, response.StatusCode)
		require.Contains(t, response.Header.Get("Location"), "github_oauth="+result)
		require.Empty(t, response.Cookies())
		require.NoError(t, response.Body.Close())
	}
	callback(start(), "other-session", "session_changed")
	path := start()
	callback(path, "original-session", "email_updated")
	callback(path, "original-session", "state_invalid")
	security, err := db.GetAccountSecurity("alice")
	require.NoError(t, err)
	require.Equal(t, "real@example.com", security.Email)
	identity, err := db.GetGitHubIdentity("alice")
	require.NoError(t, err)
	require.Nil(t, identity, "email verification must not create a login binding")
	path = start()
	_, err = db.UpdateAccountEmail("alice", "changed@example.com", time.Now().UnixMilli())
	require.NoError(t, err)
	callback(path, "original-session", "session_changed")
	security, err = db.GetAccountSecurity("alice")
	require.NoError(t, err)
	require.Equal(t, "changed@example.com", security.Email)
}
