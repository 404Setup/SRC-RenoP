/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package captcha verifies provider challenges and consumes bounded browser approvals.
package captcha

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/outboundproxy"
	"renop/internal/utils"
)

const (
	ProofHeader      = "X-Renop-Captcha"
	ScopeHeader      = "X-Renop-Captcha-Scope"
	nonceCookie      = "renop_captcha_nonce"
	maxResponseToken = 16 << 10
	approvalLifetime = 2 * time.Minute
)

var verifySlots = make(chan struct{}, 16)
var errInvalid = errors.New("invalid CAPTCHA response")
var errUnavailable = errors.New("CAPTCHA provider unavailable")

// IsPublicPath keeps challenge metadata and verification available without a valid login.
func IsPublicPath(path string) bool {
	return path == "/api/captcha" || path == "/api/captcha/verify" || path == "/api/captcha/widget"
}

func failure(c fiber.Ctx, status int, code, scope string) error {
	c.Set("X-Renop-Error-Code", code)
	if scope != "" {
		c.Set(ScopeHeader, scope)
	}
	return fiber.NewError(status, code)
}

func browserBinding(c fiber.Ctx) string {
	nonce := c.Cookies(nonceCookie)
	if uuid.Validate(nonce) != nil {
		return ""
	}
	digest := sha256.Sum256([]byte(nonce + "\x00" + c.Cookies("renop_session")))
	return hex.EncodeToString(digest[:])
}

// Require checks only browser/anonymous actions; authenticated protocol and API-token clients remain exempt.
func Require(c fiber.Ctx, state *core.AppState, scopes ...string) error {
	kind, _ := c.Locals("auth_credential_kind").(string)
	if kind == "api_token" || kind == "password" {
		return nil
	}
	cfg := state.Inner.Config.Load().Captcha
	scope := cfg.EffectiveScope(scopes...)
	if scope == "" {
		return nil
	}
	if c.Locals("captcha_verified_scope") == scope && c.Locals("captcha_verified_policy") == cfg.PolicyHash() {
		return nil
	}
	raw := c.Get(ProofHeader)
	if raw == "" || len(raw) > 128 {
		return failure(c, 428, "captcha_required", scope)
	}
	proof, ok := state.Inner.CaptchaApprovals.Consume(raw, scope, time.Now().UnixMilli())
	binding := browserBinding(c)
	if !ok || binding == "" || binding != proof.SessionHash || proof.ConfigHash != cfg.PolicyHash() {
		return failure(c, 400, "captcha_invalid", scope)
	}
	c.Locals("captcha_verified_scope", scope)
	c.Locals("captcha_verified_policy", cfg.PolicyHash())
	return nil
}

// SetupRoutes exposes public configuration, an isolated widget document, and one-time browser approvals.
func SetupRoutes(router fiber.Router, state *core.AppState) {
	router.Get("/captcha", func(c fiber.Ctx) error {
		value := state.Inner.Config.Load().Captcha
		if value.Provider != "disabled" && uuid.Validate(c.Cookies(nonceCookie)) != nil {
			c.Cookie(&fiber.Cookie{Name: nonceCookie, Value: uuid.NewString(), Path: "/", MaxAge: 600,
				HTTPOnly: true, Secure: utils.IsHTTPSRequest(c), SameSite: "Lax"})
		}
		c.Set(fiber.HeaderCacheControl, "no-store")
		return c.JSON(fiber.Map{"provider": value.Provider, "site_key": value.SiteKey,
			"scopes": value.Scopes, "friendly_region": value.FriendlyRegion})
	})
	router.Post("/captcha/verify", func(c fiber.Ctx) error { return verifyBrowser(c, state, nil) })
	router.Get("/captcha/widget", serveWidget)
}

func verifyBrowser(c fiber.Ctx, state *core.AppState, client *http.Client) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	var input struct {
		Scope    string `json:"scope"`
		Provider string `json:"provider"`
		SiteKey  string `json:"site_key"`
		Response string `json:"response"`
	}
	if !c.Is("json") {
		return failure(c, 415, "captcha_invalid", "")
	}
	if err := utils.ReadJSONLimited(c, &input, 32<<10); err != nil || len(input.Response) == 0 || len(input.Response) > maxResponseToken {
		return failure(c, 400, "captcha_invalid", "")
	}
	cfg := state.Inner.Config.Load()
	policy := cfg.Captcha
	if policy.EffectiveScope(input.Scope) == "" || input.Provider != policy.Provider || input.SiteKey != policy.SiteKey {
		return failure(c, 409, "captcha_changed", "")
	}
	binding := browserBinding(c)
	if binding == "" {
		return failure(c, 400, "captcha_cookies_required", "")
	}
	select {
	case verifySlots <- struct{}{}:
		defer func() { <-verifySlots }()
	default:
		return failure(c, 503, "captcha_unavailable", "")
	}
	if client == nil {
		selected, err := outboundproxy.Selected(cfg.Proxy)
		if err != nil {
			return failure(c, 503, "captcha_unavailable", "")
		}
		transport := utils.DefaultTransport.Clone()
		if err := outboundproxy.ConfigureTransport(transport, selected); err != nil {
			return failure(c, 503, "captcha_unavailable", "")
		}
		client = &http.Client{Transport: transport, Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		defer client.CloseIdleConnections()
	}
	err := verifyToken(c.Context(), client, policy, cfg.Server.Domains, input.Scope, input.Response, utils.ExtractIP(c, &cfg.Server))
	if err != nil {
		if errors.Is(err, errInvalid) {
			return failure(c, 400, "captcha_invalid", "")
		}
		return failure(c, 503, "captcha_unavailable", "")
	}
	if state.Inner.Config.Load().Captcha.PolicyHash() != policy.PolicyHash() {
		return failure(c, 409, "captcha_changed", "")
	}
	proof := uuid.NewString()
	now := time.Now().UnixMilli()
	if !state.Inner.CaptchaApprovals.Put(proof, core.TransientAuthState{Provider: input.Scope,
		SessionHash: binding, ConfigHash: policy.PolicyHash(), ExpiresAt: now + approvalLifetime.Milliseconds()}, now) {
		return failure(c, 503, "captcha_unavailable", "")
	}
	return c.JSON(fiber.Map{"proof": proof, "expires_in": int(approvalLifetime.Seconds())})
}

func verifyToken(ctx context.Context, client *http.Client, cfg config.CaptchaConfig, domains []string, scope, response, ip string) error {
	endpoint := "https://www.google.com/recaptcha/api/siteverify"
	values := url.Values{"response": {response}, "secret": {cfg.SecretKey}, "remoteip": {ip}}
	switch cfg.Provider {
	case "turnstile":
		endpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	case "hcaptcha":
		endpoint = "https://api.hcaptcha.com/siteverify"
		values.Set("sitekey", cfg.SiteKey)
	case "friendlycaptcha":
		endpoint = "https://" + cfg.FriendlyRegion + ".frcapi.com/api/v2/captcha/siteverify"
		values.Del("secret")
		values.Del("remoteip")
		values.Set("sitekey", cfg.SiteKey)
	case "recaptcha_v2", "recaptcha_invisible", "recaptcha_v3":
	default:
		return errUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return errUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	if cfg.Provider == "friendlycaptcha" {
		request.Header.Set("X-API-Key", cfg.SecretKey)
	}
	reply, err := client.Do(request)
	if err != nil {
		return errUnavailable
	}
	defer utils.DiscardHTTPBody(reply.Body, reply.ContentLength)
	if reply.StatusCode != http.StatusOK || reply.ContentLength > 64<<10 {
		return errUnavailable
	}
	data, err := utils.ReadAllLimited(reply.Body, 64<<10)
	if err != nil {
		return errUnavailable
	}
	var result struct {
		Success  bool     `json:"success"`
		Hostname string   `json:"hostname"`
		Action   string   `json:"action"`
		Score    *float64 `json:"score"`
		Data     struct {
			Challenge struct {
				Origin string `json:"origin"`
			} `json:"challenge"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &result) != nil {
		return errUnavailable
	}
	if !result.Success {
		return errInvalid
	}
	if cfg.Provider == "hcaptcha" {
		return nil
	} // hCaptcha binds the expected sitekey; hostname is only a statistical field.
	hostname := result.Hostname
	if cfg.Provider == "friendlycaptcha" {
		hostname = result.Data.Challenge.Origin
		if hostname == "" {
			return nil
		} // The verified sitekey remains authoritative when origin is unavailable.
		if parsed, err := url.Parse(hostname); err == nil && parsed.Hostname() != "" {
			hostname = parsed.Hostname()
		}
	}
	if !allowedHostname(hostname, domains) {
		return errInvalid
	}
	if cfg.Provider == "recaptcha_v3" || cfg.Provider == "turnstile" {
		if result.Action != "renop_"+scope {
			return errInvalid
		}
	}
	if cfg.Provider == "recaptcha_v3" && (result.Score == nil || math.IsNaN(*result.Score) ||
		*result.Score < cfg.MinScore || *result.Score < 0 || *result.Score > 1) {
		return errInvalid
	}
	return nil
}

func allowedHostname(hostname string, domains []string) bool {
	hostname = strings.ToLower(strings.TrimSuffix(hostname, "."))
	if hostname == "" {
		return false
	}
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSuffix(domain, "."))
		if hostname == domain || strings.HasPrefix(domain, "*.") && strings.HasSuffix(hostname, domain[1:]) {
			return true
		}
	}
	return false
}

func serveWidget(c fiber.Ctx) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderContentType, "text/html; charset=utf-8")
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	c.Set("X-Frame-Options", "SAMEORIGIN")
	c.Set("Content-Security-Policy", "default-src 'none'; script-src 'self' https://www.google.com https://www.gstatic.com https://www.recaptcha.net https://js.hcaptcha.com https://*.hcaptcha.com https://challenges.cloudflare.com https://cdn.jsdelivr.net https://*.frcapi.com 'wasm-unsafe-eval'; style-src 'unsafe-inline' https://*.hcaptcha.com; img-src https: data: blob:; connect-src https://www.google.com https://www.recaptcha.net https://*.hcaptcha.com https://challenges.cloudflare.com https://*.frcapi.com; frame-src https://www.google.com https://www.recaptcha.net https://*.hcaptcha.com https://challenges.cloudflare.com https://*.frcapi.com; worker-src blob:; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
	return c.SendString(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>CAPTCHA</title><script type="module" src="/js/captcha-widget.js"></script></head><body><div id="widget"></div></body></html>`)
}
