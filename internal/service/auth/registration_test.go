/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/mail"
	"renop/internal/service/mailqueue"
	"renop/internal/testutil"
)

func registrationTestApp(t *testing.T) (*fiber.App, *core.AppState, *config.Config) {
	t.Helper()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "registration.db"), MaxOpenConns: 1, MaxIdleConns: 1})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state := core.NewAppState()
	state.Inner.DB = db
	cfg := config.DefaultConfig()
	cfg.Registration.IPLimit = 10
	cfg.Mail.PublicURL = "https://renop.example"
	cfg.Mail.ManualRate.Limit = 100
	require.NoError(t, cfg.Mail.EnsureKey())
	account := mail.Presets()[0].Account
	account.ID, account.From = "primary", "sender@example.com"
	cfg.Mail.Accounts = []mail.Account{account}
	state.Inner.Config.Store(cfg)
	app := fiber.New()
	app.Use(AuthMiddleware(state))
	setupRegistrationRoutes(app.Group("/api/auth"), state)
	return app, state, cfg
}

func registrationRequest(t *testing.T, app *fiber.App, path string, body map[string]any, cookie *http.Cookie) *http.Response {
	t.Helper()
	var reader io.Reader
	method := "GET"
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
		method = "POST"
	}
	request := httptest.NewRequest(method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := app.Test(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

func TestPublicRegistrationRoutes(t *testing.T) {
	app, state, cfg := registrationTestApp(t)
	body := map[string]any{"username": "manual_user", "password": "ValidPassword2026!", "email": "manual@example.com"}
	for _, path := range []string{"/api/auth/registration", "/api/auth/registration/code"} {
		response := registrationRequest(t, app, path, body, nil)
		require.Equal(t, 404, response.StatusCode)
		require.Equal(t, "registration_disabled", response.Header.Get("X-Renop-Error-Code"))
	}
	cfg.Registration.Enabled = true
	state.Inner.Config.Store(cfg)
	response := registrationRequest(t, app, "/api/auth/registration", body, nil)
	require.Equal(t, 201, response.StatusCode)
	account, err := state.GetDB().GetTokenByName("manual_user")
	require.NoError(t, err)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(account.EncryptedSecret), []byte("ValidPassword2026!")))
	require.Empty(t, account.Tokens)
	require.Equal(t, uint64(1), state.Inner.TokensCount.Load())
	signedIn, err := AuthenticateUser(state, &core.LoginRequest{Name: "manual@example.com", Secret: "ValidPassword2026!"}, nil)
	require.NoError(t, err)
	require.NotNil(t, signedIn)
	cfg.Mail.Enabled = true
	state.Inner.Config.Store(cfg)
	body["username"], body["email"] = "verified_user", "verified@example.com"
	response = registrationRequest(t, app, "/api/auth/registration", body, nil)
	require.Equal(t, 400, response.StatusCode)
	response = registrationRequest(t, app, "/api/auth/registration/code", map[string]any{"email": body["email"]}, nil)
	require.Equal(t, 202, response.StatusCode)
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	var receipt mailqueue.Receipt
	require.NoError(t, json.NewDecoder(response.Body).Decode(&receipt))
	require.Len(t, response.Cookies(), 1)
	cookie := response.Cookies()[0]
	require.True(t, cookie.HttpOnly)
	require.Equal(t, "/api/auth/registration", cookie.Path)
	job, err := state.GetDB().GetMailJob(receipt.ID, cfg.Mail.EncryptionKey)
	require.NoError(t, err)
	code := regexp.MustCompile(`(?m)^\d{8}$`).FindString(job.Message.Text)
	require.Len(t, code, 8)
	pending, err := state.GetDB().GetPendingRegistration(registrationHash(cookie.Value), time.Now().UnixMilli())
	require.NoError(t, err)
	require.NotContains(t, pending.ProfileJSON, body["password"])
	require.NotContains(t, pending.CodeHash, code)
	body["code"] = "00000000"
	if code == body["code"] {
		body["code"] = "11111111"
	}
	response = registrationRequest(t, app, "/api/auth/registration", body, cookie)
	require.Equal(t, 400, response.StatusCode)
	body["code"] = code
	response = registrationRequest(t, app, "/api/auth/registration", body, nil)
	require.Equal(t, 400, response.StatusCode, "a code alone must not replace its browser capability")
	response = registrationRequest(t, app, "/api/auth/registration", body, cookie)
	require.Equal(t, 201, response.StatusCode)
	security, err := state.GetDB().GetAccountSecurity("verified_user")
	require.NoError(t, err)
	require.Equal(t, "verified@example.com", security.Email)
	response = registrationRequest(t, app, "/api/auth/registration", body, cookie)
	require.Equal(t, 400, response.StatusCode)
	cfg.Registration.Enabled = false
	state.Inner.Config.Store(cfg)
	response = registrationRequest(t, app, "/api/auth/registration/pending", nil, cookie)
	require.Equal(t, 404, response.StatusCode)
}

func TestGitHubRegistrationRequiresExplicitPasswordConfirmation(t *testing.T) {
	app, state, cfg := registrationTestApp(t)
	cfg.Registration.Enabled = true
	cfg.Mail.Enabled = true
	cfg.Server.GitHubOAuth = config.GitHubOAuthConfig{Enabled: true, ClientID: "client", ClientSecret: "secret", CallbackURL: "https://renop.example/api/auth/github/callback"}
	state.Inner.Config.Store(cfg)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_, _ = io.WriteString(w, `{"access_token":"provider-secret","token_type":"bearer","scope":"read:user read:org user:email"}`)
		case "/api/user":
			_, _ = io.WriteString(w, `{"id":42,"login":"occupied","name":"Provider Nickname"}`)
		case "/api/user/orgs":
			_, _ = io.WriteString(w, `[]`)
		case "/api/user/emails":
			_, _ = io.WriteString(w, `[{"email":"fake@users.noreply.github.com","primary":true,"verified":true},{"email":"real@example.com","verified":true}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(provider.Close)
	setupGitHubRoutesWithProvider(app.Group("/api/auth"), state, nil, githubOAuthProvider{AuthorizeURL: "https://github.example/authorize", TokenURL: provider.URL + "/token", APIURL: provider.URL + "/api"})
	require.NoError(t, state.GetDB().CreateToken(&core.AccessToken{Name: "occupied", Permissions: []string{"base"}}, "", 1))
	start := registrationRequest(t, app, "/api/auth/github/start?intent=login&return_to=/packages", nil, nil)
	location, err := url.Parse(start.Header.Get("Location"))
	require.NoError(t, err)
	require.Contains(t, location.Query().Get("scope"), "user:email")
	callback := "/api/auth/github/callback?state=" + url.QueryEscape(location.Query().Get("state")) + "&code=code"
	response := registrationRequest(t, app, callback, nil, start.Cookies()[0])
	require.Equal(t, 303, response.StatusCode)
	require.True(t, strings.HasPrefix(response.Header.Get("Location"), "/account/register?provider=github"))
	require.Len(t, response.Cookies(), 1)
	cookie := response.Cookies()[0]
	require.Equal(t, registrationCookie, cookie.Name)
	identity, err := state.GetDB().GetGitHubIdentityByProviderID(42)
	require.NoError(t, err)
	require.Nil(t, identity)
	response = registrationRequest(t, app, "/api/auth/registration/pending", nil, cookie)
	require.Equal(t, 200, response.StatusCode)
	var pending map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&pending))
	require.Equal(t, "real@example.com", pending["email"])
	require.Equal(t, false, pending["username_available"])
	require.Equal(t, "Provider Nickname", pending["nickname"])
	body := map[string]any{"provider": "github", "username": "occupied", "email": "real@example.com"}
	response = registrationRequest(t, app, "/api/auth/registration", body, cookie)
	require.Equal(t, 400, response.StatusCode, "provider registration must still require a password")
	body["password"] = "ValidPassword2026!"
	response = registrationRequest(t, app, "/api/auth/registration", body, cookie)
	require.Equal(t, 409, response.StatusCode)
	require.Equal(t, "registration_username_conflict", response.Header.Get("X-Renop-Error-Code"))
	body["username"], body["nickname"] = "chosen_name", "Chosen Nickname"
	response = registrationRequest(t, app, "/api/auth/registration", body, nil)
	require.Equal(t, 400, response.StatusCode)
	response = registrationRequest(t, app, "/api/auth/registration", body, cookie)
	require.Equal(t, 201, response.StatusCode)
	identity, err = state.GetDB().GetGitHubIdentityByProviderID(42)
	require.NoError(t, err)
	require.Equal(t, "chosen_name", identity.Username)
	security, err := state.GetDB().GetAccountSecurity("chosen_name")
	require.NoError(t, err)
	require.Equal(t, "real@example.com", security.Email)
	require.True(t, security.PasswordConfigured)
	_, total, err := state.GetDB().ListMailJobs("", "", cfg.Mail.EncryptionKey, 10, 0)
	require.NoError(t, err)
	require.Zero(t, total, "verified provider email must not queue SMTP verification")
	cfg.Mail.Enabled = false
	state.Inner.Config.Store(cfg)
	body["username"], body["email"] = "expired_provider", "another@example.com"
	response = registrationRequest(t, app, "/api/auth/registration", body, nil)
	require.Equal(t, 400, response.StatusCode, "an expired provider cookie must not silently create an unlinked account")
}

func TestRegistrationTreatsEquivalentProxyIPsAsOneAddress(t *testing.T) {
	app, state, cfg := registrationTestApp(t)
	cfg.Registration.Enabled = true
	cfg.Registration.IPLimit = 1
	_, trusted, err := net.ParseCIDR("0.0.0.0/0")
	require.NoError(t, err)
	cfg.Server.ParsedTrustedProxies = []*net.IPNet{trusted}
	cfg.Server.CdnIPHeader = "X-Forwarded-For"
	state.Inner.Config.Store(cfg)
	for index, ip := range []string{"2001:0db8:0000:0000:0000:0000:0000:0001", "2001:db8::1"} {
		body := `{"username":"ipv6_first","password":"ValidPassword2026!"}`
		if index == 1 {
			body = strings.Replace(body, "ipv6_first", "ipv6_second", 1)
		}
		request := httptest.NewRequest("POST", "/api/auth/registration", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Forwarded-For", ip)
		response, err := app.Test(request)
		require.NoError(t, err)
		defer response.Body.Close()
		if index == 0 {
			require.Equal(t, 201, response.StatusCode)
		} else {
			require.Equal(t, 429, response.StatusCode)
		}
	}
}
