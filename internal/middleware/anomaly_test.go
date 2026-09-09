/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package middleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/time/rate"

	"github.com/stretchr/testify/require"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/testutil"
)

func TestAccountIPBanRejectsEveryRequestAndHonorsTrustedProxies(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.CdnIPHeader = "X-Forwarded-For"
	cfg.Server.TrustedProxies = []string{"0.0.0.0"}
	cfg.Server.ParseTrustedProxies()
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "middleware-ip-ban.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state.Inner.DB = db
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", Permissions: []string{"base"}}))
	now := time.Now().UnixMilli()
	session := &core.Session{PublicID: "alice-session", Username: "alice", IP: "192.0.2.10", CreatedAt: now}
	session.LastActive.Store(now)
	require.NoError(t, db.SaveSession(session, "alice-session-secret"))
	app := fiber.New()
	app.Use(AnomalyMiddleware(state))
	app.Use(func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	request := func(method, path, ip string, expected int) {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("X-Forwarded-For", ip)
		req.Header.Set("Authorization", "Bearer irrelevant-credential")
		response, err := app.Test(req)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, expected, response.StatusCode, path)
		if expected == http.StatusForbidden {
			require.Equal(t, "IP_BANNED", response.Header.Get("X-Renop-Error-Code"))
		}
	}
	request(http.MethodGet, "/api/auth/me", "192.0.2.10", http.StatusOK)
	require.NoError(t, db.SetAccountBan("alice", &core.AccountBan{Reason: "Abuse", CreatedAt: now}, true))
	for _, path := range []string{"/", "/api/auth/me", "/api/auth/login", "/api/auth/register", "/v2/example/blobs/sha256:digest", "/repo/pkg/file.jar", "/css/artifact.jar"} {
		request(http.MethodGet, path, "::ffff:192.0.2.10", http.StatusForbidden)
		request(http.MethodPost, path, "192.0.2.10", http.StatusForbidden)
	}
	request(http.MethodGet, "/api/auth/me", "192.0.2.11", http.StatusOK)
	untrusted := config.DefaultConfig()
	untrusted.Server.CdnIPHeader = "X-Forwarded-For"
	untrusted.Server.TrustedProxies = nil
	untrusted.Server.ParsedTrustedProxies = nil
	state.Inner.Config.Store(untrusted)
	request(http.MethodGet, "/api/auth/me", "192.0.2.10", http.StatusOK)
	state.Inner.Config.Store(cfg)
	require.NoError(t, db.SetAccountBan("alice", nil))
	request(http.MethodGet, "/api/auth/me", "192.0.2.10", http.StatusOK)
}

func TestIPLimiterCleanupRemovesInactiveEntries(t *testing.T) {
	limiter := NewIPLimiter(rate.Every(time.Second), 1)
	limiter.GetLimiter("192.0.2.1")
	entry, ok := limiter.limiters.Load("192.0.2.1")
	if !ok {
		t.Fatal("limiter entry was not stored")
	}
	entry.lastSeen.Store(1)
	if removed := limiter.cleanup(); removed != 1 {
		t.Fatalf("removed limiters = %d, want 1", removed)
	}
	if got := limiter.count.Load(); got != 0 {
		t.Fatalf("limiter count = %d, want 0", got)
	}
}

func TestIPLimiterBoundsFreshEntries(t *testing.T) {
	limiter := NewIPLimiter(rate.Every(time.Second), 1)
	for n := range maxIPLimiterEntries {
		limiter.GetLimiter(strconv.Itoa(n))
	}

	if got := limiter.GetLimiter("overflow"); got != limiter.overflow {
		t.Fatal("overflow client received a retained per-IP limiter")
	}
	if got := limiter.count.Load(); got != maxIPLimiterEntries {
		t.Fatalf("limiter count = %d, want %d", got, maxIPLimiterEntries)
	}
}

func TestFrontendShellAndAssetPathClassification(t *testing.T) {
	for _, requestPath := range []string{
		"/", "/index.html", "/assets/app.js", "/js/main.js", "/css/app.css", "/svg/logo.svg",
		"/user/alice", "/user/alice/edit", "/user/alice/npm", "/account/tickets", "/account/reviews", "/account/login", "/account/recovery", "/account/forgot-password",
		"/account/teams", "/account/teams/core", "/account/maven-domains", "/account/maven-domains/com.example",
	} {
		if !isFrontendShellOrAssetPath(requestPath) {
			t.Errorf("isFrontendShellOrAssetPath(%q) = false, want true", requestPath)
		}
	}
	for _, requestPath := range []string{
		"/api/tickets", "/v2/example/manifests/latest", "/repository/file.jar", "/account/unknown", "/account/login/extra", "/account/recovery/extra",
		"/account/tickets/extra", "/account/reviews/extra", "/account/teams/core/extra", "/user/alice/unknown", "/user/alice/edit/extra",
	} {
		if isFrontendShellOrAssetPath(requestPath) {
			t.Errorf("isFrontendShellOrAssetPath(%q) = true, want false", requestPath)
		}
	}
}

func TestAuthenticatedPermissionDenialsDoNotAccumulateAnomalyFailures(t *testing.T) {
	state := core.NewAppState()
	state.Inner.Config.Store(config.DefaultConfig())
	session := &core.Session{Username: "alice"}
	session.LastActive.Store(time.Now().UnixMilli())
	state.Inner.Sessions.Store("valid-session", session)

	app := fiber.New()
	app.Use(AnomalyMiddleware(state))
	observedIP := ""
	app.Get("/protected", func(c fiber.Ctx) error {
		observedIP = c.IP()
		return c.SendStatus(fiber.StatusForbidden)
	})

	for range MaxAuthenticationFailures + 2 {
		request := httptest.NewRequest(http.MethodGet, "/protected", nil)
		request.AddCookie(&http.Cookie{Name: "renop_session", Value: "valid-session"})
		response, err := app.Test(request)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusForbidden {
			_ = response.Body.Close()
			t.Fatalf("permission denial status = %d, want %d", response.StatusCode, http.StatusForbidden)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if failures := state.Inner.AnomalyFailures.Count(observedIP); failures != 0 {
		t.Fatalf("valid authenticated permission denials recorded %d anomaly failures", failures)
	}

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.AddCookie(&http.Cookie{Name: "renop_session", Value: "invalid-session"})
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if failures := state.Inner.AnomalyFailures.Count(observedIP); failures != 1 {
		t.Fatalf("invalid authenticated denial recorded %d anomaly failures, want 1", failures)
	}
}

func TestAuthenticationFailureBanStartsAfterTenFailures(t *testing.T) {
	state := core.NewAppState()
	state.Inner.Config.Store(config.DefaultConfig())
	app := fiber.New()
	app.Use(AnomalyMiddleware(state))
	app.Post("/api/auth/login", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusUnauthorized)
	})
	for attempt := 1; attempt <= 11; attempt++ {
		response, err := app.Test(httptest.NewRequest(http.MethodPost, "/api/auth/login", nil))
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		want := http.StatusUnauthorized
		if attempt == 11 {
			want = http.StatusForbidden
		}
		if response.StatusCode != want {
			t.Fatalf("attempt %d: status = %d, want %d", attempt, response.StatusCode, want)
		}
	}
}
