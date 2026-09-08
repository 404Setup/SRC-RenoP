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
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/mail"
	"renop/internal/service/mailqueue"
	"renop/internal/testutil"
)

func TestMailJobStatusRequiresOwnerOrPrivateTicket(t *testing.T) {
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "mail-status.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state := core.NewAppState()
	state.Inner.DB = db
	cfg := config.DefaultConfig()
	cfg.Mail.Enabled = true
	cfg.Mail.PublicURL = "https://renop.example"
	require.NoError(t, cfg.Mail.EnsureKey())
	account := mail.Presets()[0].Account
	account.ID, account.From = "primary", "sender@example.com"
	cfg.Mail.Accounts = []mail.Account{account}
	state.Inner.Config.Store(cfg)
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "owner", Permissions: []string{"base"}}))
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "other", Permissions: []string{"base"}}))
	_, err = db.UpdateAccountEmail("owner", "owner@example.com", time.Now().UnixMilli())
	require.NoError(t, err)
	receipt, err := mailqueue.Enqueue(state, mailqueue.Request{Username: "owner", Scene: "test", Manual: true, IP: "192.0.2.1"})
	require.NoError(t, err)
	app := fiber.New()
	app.Use(AuthMiddleware(state))
	app.Get("/api/auth/mail/:id", func(c fiber.Ctx) error { return getMailJobStatus(c, state) })
	for _, scenario := range []struct {
		username, ticket string
		want             int
	}{{"", "", 404}, {"", "wrong-ticket", 404}, {"", receipt.Ticket, 200}, {"owner", "", 200}, {"other", "", 404}} {
		t.Run(scenario.username+"-"+scenario.ticket, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/api/auth/mail/"+receipt.ID, nil)
			request.Header.Set("X-Renop-Mail-Ticket", scenario.ticket)
			if scenario.username != "" {
				now := time.Now().UnixMilli()
				session := &core.Session{Username: scenario.username, PublicID: "mail-" + scenario.username, CreatedAt: now, IP: "192.0.2.1", LoginMethod: "password"}
				session.LastActive.Store(now)
				require.NoError(t, db.SaveSession(session, "mail-session-"+scenario.username))
				request.Header.Set("Cookie", sessionCookieName+"=mail-session-"+scenario.username)
			}
			response, err := app.Test(request)
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			require.NoError(t, err)
			require.Equal(t, scenario.want, response.StatusCode, string(body))
			require.NotContains(t, string(body), "owner@example.com")
			require.NotContains(t, string(body), "ticket_hash")
			require.NotContains(t, string(body), cfg.Mail.EncryptionKey)
			require.False(t, strings.Contains(string(body), "Your account"))
		})
	}
}
