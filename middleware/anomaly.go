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
	"crypto/subtle"
	"errors"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/3JoB/unsafeConvert"
	"github.com/gofiber/fiber/v3"
	"github.com/llxisdsh/pb"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/time/rate"

	"renop/config"
	"renop/core"
	"renop/utils"
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
	i := &IPLimiter{
		r: r,
		b: b,
	}

	go func() {
		for {
			time.Sleep(5 * time.Minute)
			i.cleanup()
		}
	}()

	return i
}

func (i *IPLimiter) cleanup() {
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

func verifyTokenSecret(state *core.AppState, username, secret string) bool {
	token, ok := state.Inner.TokenRepository.Load(strings.ToLower(username))
	if !ok {
		return false
	}
	if token.ExpiresAt != nil && time.Now().UnixMilli() > *token.ExpiresAt {
		return false
	}
	for _, t := range token.Tokens {
		if len(t) == len(secret) {
			if subtle.ConstantTimeCompare([]byte(t), []byte(secret)) == 1 {
				return true
			}
		} else {
			_ = subtle.ConstantTimeCompare([]byte(t), []byte(t))
		}
	}
	if token.EncryptedSecret != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(token.EncryptedSecret), []byte(secret)); err == nil {
			return true
		}
	}
	return false
}

func isVerifiedAuthenticatedRequest(c fiber.Ctx, state *core.AppState) bool {
	authHeader := c.Get(fiber.HeaderAuthorization, "")

	if authHeader == "" {
		if cookie := c.Cookies("renop_session"); cookie != "" {
			if state.GetSession(cookie) != nil {
				authHeader = "Session " + cookie
			}
		}
		if authHeader == "" && (c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead) {
			queryToken := c.Query("token")
			if queryToken == "" {
				return false
			}
			if state.GetSession(queryToken) != nil {
				authHeader = "Session " + queryToken
			} else {
				authHeader = "Bearer " + queryToken
			}
		}
	}

	if authHeader == "" {
		return false
	}

	switch {
	case strings.HasPrefix(authHeader, "Session "):
		sessionId := strings.TrimPrefix(authHeader, "Session ")
		val := state.GetSession(sessionId)
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
		token, ok := state.Inner.TokenIndex.Load(bearer)
		if !ok {
			return false
		}
		if token.ExpiresAt != nil && time.Now().UnixMilli() > *token.ExpiresAt {
			return false
		}
		return true

	case strings.HasPrefix(authHeader, "Basic "):
		decoded, err := utils.DecodeB64(strings.TrimPrefix(authHeader, "Basic "))
		if err != nil {
			return false
		}
		decodedStr := unsafeConvert.StringPointer(decoded)
		idx := strings.IndexByte(decodedStr, ':')
		if idx <= 0 {
			return false
		}
		return verifyTokenSecret(state, decodedStr[:idx], decodedStr[idx+1:])
	}

	return false
}

func isStaticFrontendPath(reqPath string) bool {
	cleaned := path.Clean(reqPath)
	return cleaned == "/" ||
		cleaned == "/index.html" ||
		strings.HasPrefix(cleaned, "/assets/") ||
		strings.HasPrefix(cleaned, "/js/") ||
		strings.HasPrefix(cleaned, "/css/") ||
		strings.HasPrefix(cleaned, "/svg/")
}

func AnomalyMiddleware(state *core.AppState) fiber.Handler {
	return func(c fiber.Ctx) error {
		cfg := state.Inner.Config.Load().(*config.Config)

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

		if (c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead) && isStaticFrontendPath(c.Path()) {
			return c.Next()
		}

		ip := utils.ExtractIP(c, &cfg.Server)

		if state.Inner.AnomalyFailures != nil && state.Inner.AnomalyFailures.Count(ip) >= MaxFailuresPerMinute {
			c.Set(fiber.HeaderConnection, "close")
			return c.SendStatus(fiber.StatusForbidden)
		}

		if !isVerifiedAuthenticatedRequest(c, state) {
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

		if status == fiber.StatusUnauthorized || status == fiber.StatusForbidden {
			if state.Inner.AnomalyFailures != nil {
				state.Inner.AnomalyFailures.Increment(ip)
			}
		}

		return err
	}
}
