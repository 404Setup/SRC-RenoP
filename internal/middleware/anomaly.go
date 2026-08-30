/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package middleware provides shared Fiber request middleware.
package middleware

import (
	"errors"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/llxisdsh/pb"
	"golang.org/x/time/rate"

	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/utils"
)

const (
	MaxFailuresPerMinute = 5
	MaxRequestsPerMinute = 100
	MaxRequestsBurst     = 60
)

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen atomic.Int64
}

type IPLimiter struct {
	limiters   pb.MapOf[string, *limiterEntry]
	r          rate.Limit
	b          int
	count      atomic.Int64
	isCleaning atomic.Bool
}

func NewIPLimiter(r rate.Limit, b int) *IPLimiter {
	return &IPLimiter{
		r: r,
		b: b,
	}
}

func (i *IPLimiter) cleanup() int64 {
	if i == nil {
		return 0
	}
	nowUnix := time.Now().UnixNano()
	timeoutNano := (5 * time.Minute).Nanoseconds()

	var removed int64
	i.limiters.Range(func(key string, value *limiterEntry) bool {
		if nowUnix-value.lastSeen.Load() > timeoutNano {
			if _, deleted := i.limiters.LoadAndDelete(key); deleted {
				removed++
			}
		}
		return true
	})
	if removed > 0 {
		i.count.Add(-removed)
	}
	return removed
}

func (i *IPLimiter) GetLimiter(ip string) *rate.Limiter {
	nowUnix := time.Now().UnixNano()

	if value, exists := i.limiters.Load(ip); exists {
		value.lastSeen.Store(nowUnix)
		return value.limiter
	}

	newEntry := &limiterEntry{
		limiter: rate.NewLimiter(i.r, i.b),
	}
	newEntry.lastSeen.Store(nowUnix)

	value, loaded := i.limiters.LoadOrStore(ip, newEntry)

	if !loaded {
		if i.count.Add(1) > 10000 {
			if i.isCleaning.CompareAndSwap(false, true) {
				go func() {
					defer i.isCleaning.Store(false)
					i.cleanup()
				}()
			}
		}
	} else {
		value.lastSeen.Store(nowUnix)
	}

	return value.limiter
}

var GlobalIPLimiter = NewIPLimiter(rate.Every(time.Minute/time.Duration(MaxRequestsPerMinute)), MaxRequestsBurst)

// PruneIPLimiters removes inactive global request limiters.
func PruneIPLimiters() int64 {
	return GlobalIPLimiter.cleanup()
}

func verifyTokenSecret(state *core.AppState, username, secret string) bool {
	token := state.GetTokenByName(strings.ToLower(username))
	if token == nil {
		return false
	}
	credential, err := auth.VerifyAccountCredential(state, token, secret)
	return err == nil && credential != nil
}

func isVerifiedAuthenticatedRequest(c fiber.Ctx, state *core.AppState) bool {
	authHeader := c.Get(fiber.HeaderAuthorization, "")
	if strings.HasPrefix(authHeader, "Session ") {
		return false
	}

	if authHeader == "" {
		if cookie := c.Cookies("renop_session"); cookie != "" {
			if state.GetSession(cookie) != nil {
				authHeader = "Session " + cookie
			}
		}
		if authHeader == "" {
			return false
		}
	}

	if authHeader == "" {
		return false
	}

	switch {
	case strings.HasPrefix(authHeader, "Session "):
		sessionID := strings.TrimPrefix(authHeader, "Session ")
		val := state.GetSession(sessionID)
		if val == nil {
			return false
		}
		if time.Now().UnixMilli()-val.LastActive.Load() > core.SessionIdleTimeoutMillis {
			return false
		}
		return true

	case strings.HasPrefix(authHeader, "Bearer "):
		bearer := strings.TrimPrefix(authHeader, "Bearer ")
		if before, after, ok := strings.Cut(bearer, ":"); ok {
			if before == "" {
				return false
			}
			return verifyTokenSecret(state, before, after)
		}
		if bearer == "" {
			return false
		}
		credential, err := auth.VerifyBearerCredential(state, bearer)
		return err == nil && credential != nil

	case strings.HasPrefix(authHeader, "Basic "):
		decoded, err := utils.DecodeB64(strings.TrimPrefix(authHeader, "Basic "))
		if err != nil {
			return false
		}
		decodedStr := string(decoded)
		idx := strings.IndexByte(decodedStr, ':')
		if idx <= 0 {
			return false
		}
		return verifyTokenSecret(state, decodedStr[:idx], decodedStr[idx+1:])
	}

	return false
}

func isFrontendShellOrAssetPath(reqPath string) bool {
	cleaned := path.Clean(reqPath)
	if cleaned == "/" ||
		cleaned == "/index.html" ||
		strings.HasPrefix(cleaned, "/assets/") ||
		strings.HasPrefix(cleaned, "/js/") ||
		strings.HasPrefix(cleaned, "/css/") ||
		strings.HasPrefix(cleaned, "/svg/") {
		return true
	}
	if strings.HasPrefix(cleaned, "/user/") {
		remainder := strings.TrimPrefix(cleaned, "/user/")
		separator := strings.IndexByte(remainder, '/')
		if separator < 0 {
			return remainder != ""
		}
		if separator == 0 || strings.Contains(remainder[separator+1:], "/") {
			return false
		}
		section := remainder[separator+1:]
		return section == "edit" || section == "maven" || section == "cargo" ||
			section == "docker" || section == "npm"
	}
	if cleaned == "/account/reviews" || cleaned == "/account/teams" || cleaned == "/account/maven-domains" {
		return true
	}
	for _, prefix := range [...]string{"/account/teams/", "/account/maven-domains/"} {
		if strings.HasPrefix(cleaned, prefix) {
			remainder := strings.TrimPrefix(cleaned, prefix)
			return remainder != "" && !strings.Contains(remainder, "/")
		}
	}
	return false
}

func AnomalyMiddleware(state *core.AppState) fiber.Handler {
	return func(c fiber.Ctx) error {
		cfg := state.Inner.Config.Load()

		maxActive := uint64(cfg.Server.MaxActiveRequests)
		if maxActive > 0 {
			active := state.Inner.ActiveRequests.Add(1)
			if active > maxActive {
				state.Inner.ActiveRequests.Add(^uint64(0))
				c.Set(fiber.HeaderConnection, "close")
				return c.SendStatus(fiber.StatusServiceUnavailable)
			}
			defer state.Inner.ActiveRequests.Add(^uint64(0))
		} else {
			state.Inner.ActiveRequests.Add(1)
			defer state.Inner.ActiveRequests.Add(^uint64(0))
		}

		if (c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead) && isFrontendShellOrAssetPath(c.Path()) {
			return c.Next()
		}

		ip := utils.ExtractIP(c, &cfg.Server)

		if state.Inner.AnomalyFailures != nil && state.Inner.AnomalyFailures.Count(ip) >= MaxFailuresPerMinute {
			c.Set(fiber.HeaderConnection, "close")
			return c.SendStatus(fiber.StatusForbidden)
		}

		verifiedAuthentication := isVerifiedAuthenticatedRequest(c, state)
		if !verifiedAuthentication {
			limiter := GlobalIPLimiter.GetLimiter(ip)
			if !limiter.Allow() {
				c.Set(fiber.HeaderConnection, "close")
				return c.SendStatus(fiber.StatusTooManyRequests)
			}
		}

		err := c.Next()
		status := c.Response().StatusCode()
		if err != nil {
			var fiberErr *fiber.Error
			if errors.As(err, &fiberErr) {
				status = fiberErr.Code
			}
		}

		if !verifiedAuthentication && (status == fiber.StatusUnauthorized || status == fiber.StatusForbidden) {
			authHeader := c.Get(fiber.HeaderAuthorization, "")
			cookie := c.Cookies("renop_session", "")
			isAuthPath := strings.HasPrefix(c.Path(), "/api/auth/") || strings.HasPrefix(c.Path(), "/api/token/")
			// Only count as anomaly failure when authentication credentials were provided and failed,
			// or when attempting authentication on an auth endpoint.
			// Unauthenticated guest requests receiving 401 challenges or forbidden repo checks must not trigger global IP ban.
			if (authHeader != "" || cookie != "" || isAuthPath) && state.Inner.AnomalyFailures != nil {
				state.Inner.AnomalyFailures.Increment(ip)
			}
		}

		return err
	}
}
