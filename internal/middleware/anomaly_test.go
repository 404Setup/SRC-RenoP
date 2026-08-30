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
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/time/rate"

	"renop/internal/config"
	"renop/internal/core"
)

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

func TestFrontendShellAndAssetPathClassification(t *testing.T) {
	for _, requestPath := range []string{
		"/", "/index.html", "/assets/app.js", "/js/main.js", "/css/app.css", "/svg/logo.svg",
		"/user/alice", "/user/alice/edit", "/user/alice/npm", "/account/reviews",
		"/account/teams", "/account/teams/core", "/account/maven-domains", "/account/maven-domains/com.example",
	} {
		if !isFrontendShellOrAssetPath(requestPath) {
			t.Errorf("isFrontendShellOrAssetPath(%q) = false, want true", requestPath)
		}
	}
	for _, requestPath := range []string{
		"/api/reviews", "/v2/example/manifests/latest", "/repository/file.jar", "/account/unknown",
		"/account/reviews/extra", "/account/teams/core/extra", "/user/alice/unknown", "/user/alice/edit/extra",
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

	for range MaxFailuresPerMinute + 2 {
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
