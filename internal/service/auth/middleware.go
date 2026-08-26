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
	"crypto/subtle"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/utils"
)

var InvalidCredentialsUser = &config.User{
	Username:         "__invalid__",
	PasswordHash:     "",
	Tokens:           []string{},
	Roles:            []string{"base"},
	ReadPermissions:  []string{},
	WritePermissions: []string{},
}

var GuestUser = &config.User{
	Username:         "guest",
	PasswordHash:     "",
	Tokens:           []string{},
	Roles:            []string{"base"},
	ReadPermissions:  []string{},
	WritePermissions: []string{},
}

const authTokenExpiresAtLocal = "auth_token_expires_at"

func setAuthTokenExpiry(c fiber.Ctx, accessToken *core.AccessToken) {
	if accessToken != nil && accessToken.ExpiresAt != nil {
		c.Locals(authTokenExpiresAtLocal, *accessToken.ExpiresAt)
	}
}

func authCacheExpiry(c fiber.Ctx, now int64) int64 {
	expiresAt := now + int64((10*time.Minute)/time.Millisecond)
	if tokenExpiresAt, ok := c.Locals(authTokenExpiresAtLocal).(int64); ok && tokenExpiresAt < expiresAt {
		expiresAt = tokenExpiresAt
	}
	return expiresAt
}

func isAccessTokenExpired(accessToken *core.AccessToken) bool {
	return accessToken != nil && accessToken.ExpiresAt != nil && time.Now().UnixMilli() >= *accessToken.ExpiresAt
}

func ValidateAndRenewSession(state *core.AppState, sessionID string) string {
	session := state.GetSession(sessionID)
	if session == nil {
		return ""
	}

	now := time.Now().UnixMilli()
	if now-session.LastActive.Load() > core.SessionIdleTimeoutMillis {
		_, _ = state.RevokeSession(sessionID)
		return ""
	}

	if now-session.LastActive.Load() > core.SessionRenewalIntervalMillis {
		if db := state.GetDB(); db != nil {
			if err := db.UpdateSessionLastActive(sessionID, now); err != nil {
				log.Printf("Failed to persist session activity: %v", err)
			}
		}
		session.LastActive.Store(now)
	}

	return session.Username
}

func isManagerPermissions(permissions []string) bool {
	for _, p := range permissions {
		if p == "manager" || p == "m" || p == "access-token:manager" || p == "admin" {
			return true
		}
	}
	return false
}

func isManager(user *config.User) bool {
	return user.IsManager()
}

func buildSynthUser(accessToken *core.AccessToken) *config.User {
	readPermissions := make([]string, 0, len(accessToken.Permissions))
	writePermissions := make([]string, 0, len(accessToken.Permissions))
	for _, r := range accessToken.Permissions {
		switch {
		case r == "canview:*":
			readPermissions = append(readPermissions, "*")
		case len(r) > 8 && r[:8] == "canview:":
			readPermissions = append(readPermissions, r[8:])
		case r == "canupdate:*":
			writePermissions = append(writePermissions, "*")
		case len(r) > 10 && r[:10] == "canupdate:":
			writePermissions = append(writePermissions, r[10:])
		}
	}

	roles := make([]string, len(accessToken.Permissions))
	copy(roles, accessToken.Permissions)

	if isManagerPermissions(accessToken.Permissions) {
		hasManagerRole := false
		for _, r := range roles {
			if r == "manager" || r == "admin" {
				hasManagerRole = true
				break
			}
		}
		if !hasManagerRole {
			roles = append(roles, "manager")
		}
	}

	if len(roles) == 0 {
		roles = append(roles, "base")
	}

	return &config.User{
		Username:         accessToken.Name,
		PasswordHash:     "",
		Tokens:           []string{},
		Roles:            roles,
		ReadPermissions:  readPermissions,
		WritePermissions: writePermissions,
	}
}

func secretEqual(a, b string) bool {
	if len(a) != len(b) {
		_ = subtle.ConstantTimeCompare([]byte(a), []byte(a))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func extractAuthHeader(c fiber.Ctx, state *core.AppState) string {
	authHeader := c.Get(fiber.HeaderAuthorization, "")
	if authHeader != "" {
		return authHeader
	}
	if cookieVal := c.Cookies(sessionCookieName); cookieVal != "" {
		if state.GetSession(cookieVal) != nil {
			return "Session " + cookieVal
		}
	}
	if c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead {
		if queryToken := c.Query("token"); queryToken != "" {
			if state.GetSession(queryToken) != nil {
				return "Session " + queryToken
			}
			return "Bearer " + queryToken
		}
	}
	return ""
}

func handleBasicAuth(state *core.AppState, authHeader string, c fiber.Ctx) (*config.User, error) {
	basicAuth := strings.TrimPrefix(authHeader, "Basic ")
	decoded, err := utils.DecodeB64(basicAuth)
	if err != nil {
		return nil, nil
	}

	decodedStr := string(decoded)
	idx := strings.IndexByte(decodedStr, ':')
	if idx <= 0 {
		return nil, nil
	}

	username := strings.ToLower(decodedStr[:idx])
	password := decodedStr[idx+1:]

	accessToken := state.GetTokenByName(username)
	if accessToken == nil {
		return nil, nil
	}

	isValid := !isAccessTokenExpired(accessToken)

	isVerified := false
	if isValid {
		isVerified = verifyTokenSecret(state, accessToken, password)
	}

	if isValid && isVerified {
		setAuthTokenExpiry(c, accessToken)
		return synthUserForToken(accessToken, username), nil
	} else if !isValid {
		return nil, c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	return nil, nil
}

func handleSessionAuth(state *core.AppState, authHeader string, c fiber.Ctx) (*config.User, error) {
	sessionID := strings.TrimPrefix(authHeader, "Session ")
	username := ValidateAndRenewSession(state, sessionID)
	if username == "" {
		return nil, nil
	}

	c.Locals("current_session_id", sessionID)

	accessToken := state.GetTokenByName(username)
	if accessToken == nil {
		return nil, nil
	}
	if isAccessTokenExpired(accessToken) {
		_, _ = state.RevokeSession(sessionID)
		return nil, c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	return synthUserForToken(accessToken, username), nil
}

func handleBearerAuth(state *core.AppState, authHeader string, c fiber.Ctx) (*config.User, error) {
	bearerAuth := strings.TrimPrefix(authHeader, "Bearer ")
	idx := strings.IndexByte(bearerAuth, ':')
	if idx > 0 {
		username := strings.ToLower(bearerAuth[:idx])
		secret := bearerAuth[idx+1:]

		accessToken := state.GetTokenByName(username)
		if accessToken == nil {
			return nil, nil
		}

		if isAccessTokenExpired(accessToken) {
			return nil, c.Status(fiber.StatusForbidden).SendString("Forbidden")
		}

		if verifyTokenSecret(state, accessToken, secret) {
			setAuthTokenExpiry(c, accessToken)
			return synthUserForToken(accessToken, username), nil
		}
	} else {
		matchedUser := state.GetTokenBySecret(bearerAuth)
		if matchedUser == nil {
			return nil, nil
		}

		if isAccessTokenExpired(matchedUser) {
			return nil, nil
		}

		setAuthTokenExpiry(c, matchedUser)
		return buildSynthUser(matchedUser), nil
	}

	return nil, nil
}

func authorizeRequest(c fiber.Ctx, user *config.User) error {
	path := c.Path()
	restricted := strings.HasPrefix(path, "/api/settings") ||
		strings.HasPrefix(path, "/api/tokens") ||
		strings.HasPrefix(path, "/api/statistics") ||
		strings.HasPrefix(path, "/api/debug") ||
		path == "/api/status/instance" ||
		path == "/api/status/snapshots"
	if restricted && !isManager(user) {
		if user.Username == "guest" {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	return nil
}

func AuthMiddleware(state *core.AppState) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Path() == "/api/auth/login" {
			c.Locals("user", GuestUser)
			return c.Next()
		}

		var authenticatedUser *config.User

		authHeader := strings.Clone(extractAuthHeader(c, state))
		isSessionAuth := strings.HasPrefix(authHeader, "Session ")
		isCargoRequest := isCargoRepositoryRequest(c, state)
		isOpaqueCargoAuth := authHeader != "" && isCargoRequest &&
			!strings.HasPrefix(authHeader, "Basic ") &&
			!strings.HasPrefix(authHeader, "Session ") &&
			!strings.HasPrefix(authHeader, "Bearer ")
		authCacheKey := authHeader
		if isOpaqueCargoAuth {
			// A NUL cannot occur in an HTTP header, so this namespace cannot
			// collide with credentials accepted by non-Cargo routes.
			authCacheKey = "\x00cargo\x00" + authHeader
		}

		if authHeader != "" {
			var tempUser *config.User

			if !isSessionAuth {
				if val, ok := state.Inner.AuthCache.Load(authCacheKey); ok {
					if time.Now().UnixMilli() < val.ExpiredAt {
						tempUser = val.User
					} else {
						state.DeleteAuthCache(authCacheKey)
					}
				}
			}

			if tempUser == nil {
				var err error
				if strings.HasPrefix(authHeader, "Basic ") {
					tempUser, err = handleBasicAuth(state, authHeader, c)
				} else if strings.HasPrefix(authHeader, "Session ") {
					tempUser, err = handleSessionAuth(state, authHeader, c)
				} else if strings.HasPrefix(authHeader, "Bearer ") {
					tempUser, err = handleBearerAuth(state, authHeader, c)
				} else if isOpaqueCargoAuth {
					// Cargo registry credentials are opaque and are sent as the
					// complete Authorization value without a Bearer prefix.
					tempUser, err = handleBearerAuth(state, "Bearer "+authHeader, c)
				} else {
					return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
				}

				if err != nil {
					return err
				}

				if tempUser != nil {
					if !isSessionAuth {
						state.StoreAuthCache(authCacheKey, core.AuthCacheEntry{
							User:      tempUser,
							ExpiredAt: authCacheExpiry(c, time.Now().UnixMilli()),
						})
					}
				} else if !isSessionAuth {
					state.StoreAuthCache(authCacheKey, core.AuthCacheEntry{
						User:      InvalidCredentialsUser,
						ExpiredAt: time.Now().Add(30 * time.Second).UnixMilli(),
					})
				}
			} else if tempUser == InvalidCredentialsUser {
				if isCargoRequest {
					return sendInvalidCargoCredentials(c)
				}
				if isDockerRequest(c) {
					c.Locals("user", GuestUser)
					return c.Next()
				}
				return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
			}
			authenticatedUser = tempUser
		}

		isLogout := c.Path() == "/api/auth/logout"
		if authHeader != "" && authenticatedUser == nil && !isLogout {
			if isCargoRequest {
				return sendInvalidCargoCredentials(c)
			}
			if isDockerRequest(c) {
				c.Locals("user", GuestUser)
				return c.Next()
			}
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}

		user := authenticatedUser
		if user == nil {
			user = GuestUser
		}

		if err := authorizeRequest(c, user); err != nil {
			return err
		}

		c.Locals("user", user)

		return c.Next()
	}
}

func sendInvalidCargoCredentials(c fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
		"errors": []fiber.Map{{"detail": "Cargo registry credentials are invalid"}},
	})
}

func isCargoRepositoryRequest(c fiber.Ctx, state *core.AppState) bool {
	if state == nil || state.Inner == nil {
		return false
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return false
	}
	path := strings.TrimPrefix(c.Path(), "/")
	repository, _, _ := strings.Cut(path, "/")
	if repository == "" {
		return false
	}
	repo := cfg.Maven.Repositories[repository]
	return repo != nil && repo.NormalizedFormat() == config.RepositoryFormatCargo
}

func isDockerRequest(c fiber.Ctx) bool {
	p := c.Path()
	return p == "/v2" || strings.HasPrefix(p, "/v2/")
}

func GetUser(c fiber.Ctx) *config.User {
	if val := c.Locals("user"); val != nil {
		if u, ok := val.(*config.User); ok {
			return u
		}
	}
	return GuestUser
}

func RequireManager(c fiber.Ctx) error {
	user := GetUser(c)
	if !isManager(user) {
		if user.Username == "guest" {
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	return c.Next()
}

func CurrentSessionToken(c fiber.Ctx) string {
	if id, ok := c.Locals("current_session_id").(string); ok {
		return id
	}
	return ""
}

func verifyTokenSecret(state *core.AppState, accessToken *core.AccessToken, secret string) bool {
	for _, t := range accessToken.Tokens {
		if secretEqual(t, secret) {
			return true
		}
	}
	if accessToken.EncryptedSecret != "" {
		passwordEnabled, err := state.GetDB().PasswordLoginEnabled(accessToken.Name)
		if err == nil && passwordEnabled {
			if err := bcrypt.CompareHashAndPassword([]byte(accessToken.EncryptedSecret), []byte(secret)); err == nil {
				return true
			}
		}
	}
	return false
}

func synthUserForToken(accessToken *core.AccessToken, username string) *config.User {
	if accessToken.Name != username {
		tCopy := *accessToken
		tCopy.Name = username
		accessToken = &tCopy
	}
	return buildSynthUser(accessToken)
}
