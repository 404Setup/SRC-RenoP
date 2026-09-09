/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"io"
	"net/http/httptest"
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

func TestEmailPasswordResetRoutes(t *testing.T) {
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "reset-routes.db"), MaxOpenConns: 1, MaxIdleConns: 1})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state := core.NewAppState()
	state.Inner.DB = db
	cfg := config.DefaultConfig()
	cfg.Mail.PublicURL = "https://renop.example"
	cfg.Mail.ManualRate.Limit = 100
	require.NoError(t, cfg.Mail.EnsureKey())
	account := mail.Presets()[0].Account
	account.ID, account.From = "primary", "sender@example.com"
	cfg.Mail.Accounts = []mail.Account{account}
	state.Inner.Config.Store(cfg)
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", EncryptedSecret: "old-password", Permissions: []string{"base"}}))
	_, err = db.UpdateAccountEmail("alice", "alice@example.com", time.Now().UnixMilli())
	require.NoError(t, err)
	session := &core.Session{PublicID: "before-reset", Username: "alice", CreatedAt: time.Now().UnixMilli(), LoginMethod: "password"}
	session.LastActive.Store(session.CreatedAt)
	require.NoError(t, state.SaveSession(session, "old-session"))
	profile, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	require.NoError(t, db.SetUserLocale("alice", "old-session", "ja-JP", profile.UserID))
	app := fiber.New()
	app.Use(AuthMiddleware(state))
	setupPasswordResetRoutes(app.Group("/api/auth"), state)
	app.Get("/api/auth/mail/:id", func(c fiber.Ctx) error { return getMailJobStatus(c, state) })
	for _, path := range []string{"request", "confirm"} {
		response := accountSecurityRequest(t, app, "POST", "/api/auth/password-reset/"+path, map[string]string{}, "")
		require.Equal(t, 404, response.StatusCode)
		require.Equal(t, "mail_disabled", response.Header.Get("X-Renop-Error-Code"))
		require.NoError(t, response.Body.Close())
	}
	cfg.Mail.Enabled = true
	state.Inner.Config.Store(cfg)
	var knownCode string
	for _, email := range []string{"Alice@Example.COM", "unknown@example.com"} {
		response := accountSecurityRequest(t, app, "POST", "/api/auth/password-reset/request", map[string]string{"email": email}, "")
		require.Equal(t, 202, response.StatusCode)
		require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
		var receipt mailqueue.Receipt
		require.NoError(t, json.NewDecoder(response.Body).Decode(&receipt))
		require.NoError(t, response.Body.Close())
		require.NotEmpty(t, receipt.Ticket)
		require.Equal(t, "queued", receipt.Status)
		job, err := db.GetMailJob(receipt.ID, cfg.Mail.EncryptionKey)
		require.NoError(t, err)
		require.NotNil(t, job)
		require.Equal(t, strings.ToLower(email), job.Message.To)
		code := regexp.MustCompile(`(?m)^\d{8}$`).FindString(job.Message.Text)
		require.Len(t, code, 8)
		require.NotContains(t, job.Message.Text, "alice")
		require.LessOrEqual(t, job.ExpiresAt-job.CreatedAt, int64(600000))
		request := httptest.NewRequest("GET", "/api/auth/mail/"+receipt.ID, nil)
		request.Header.Set("X-Renop-Mail-Ticket", receipt.Ticket)
		response, err = app.Test(request)
		require.NoError(t, err)
		require.Equal(t, 200, response.StatusCode)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		for _, secret := range []string{email, code, receipt.Ticket, "user_id", "username"} {
			require.NotContains(t, string(body), secret)
		}
		if strings.HasPrefix(email, "Alice") {
			require.Contains(t, job.Message.HTML, `lang="ja-JP"`)
			knownCode = code
		} else {
			require.Contains(t, job.Message.HTML, `lang="en-US"`)
			response = accountSecurityRequest(t, app, "POST", "/api/auth/password-reset/confirm", map[string]string{"email": email, "code": code, "new_password": "new-password"}, "")
			require.Equal(t, 401, response.StatusCode)
			require.NoError(t, response.Body.Close())
		}
	}
	response := accountSecurityRequest(t, app, "POST", "/api/auth/password-reset/request", map[string]string{"email": "alice@example.com"}, "")
	require.Equal(t, 429, response.StatusCode)
	require.NoError(t, response.Body.Close())
	for index, code := range []string{knownCode[:7] + string('0'+(knownCode[7]-'0'+1)%10), knownCode, knownCode} {
		response = accountSecurityRequest(t, app, "POST", "/api/auth/password-reset/confirm", map[string]string{"email": "alice@example.com", "code": code, "new_password": "new-password"}, "")
		expected := 401
		if index == 1 {
			expected = 200
		}
		require.Equal(t, expected, response.StatusCode)
		if expected == 401 {
			require.Equal(t, "ACCOUNT_EMAIL_CODE_INVALID", response.Header.Get("X-Renop-Error-Code"))
		}
		if index == 0 {
			require.NotNil(t, state.GetSession("old-session"))
		}
		require.NoError(t, response.Body.Close())
	}
	require.Nil(t, state.GetSession("old-session"))
	token, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(token.EncryptedSecret), []byte("new-password")))
	for _, scenario := range []struct {
		contentType, body string
		status            int
	}{
		{"application/x-www-form-urlencoded", "email=alice@example.com", 415},
		{"application/json", `{"email":"` + strings.Repeat("a", 4100) + `"}`, 413},
		{"application/json", `{"email":"bad-address"}`, 400},
	} {
		request := httptest.NewRequest("POST", "/api/auth/password-reset/request", strings.NewReader(scenario.body))
		request.Header.Set("Content-Type", scenario.contentType)
		response, err := app.Test(request)
		require.NoError(t, err)
		require.Equal(t, scenario.status, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
}
