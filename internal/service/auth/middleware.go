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
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

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

type authResult struct {
	User      *config.User
	Kind      string
	Scheme    string
	TokenID   string
	Scopes    []string
	ExpiresAt *int64
}

func setAuthTokenExpiry(c fiber.Ctx, expiresAt *int64) {
	if expiresAt != nil {
		c.Locals(authTokenExpiresAtLocal, *expiresAt)
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
	return ""
}

func authResultFromCredential(credential *VerifiedCredential, scheme string) *authResult {
	if credential == nil || credential.Account == nil {
		return nil
	}
	return &authResult{
		User: buildSynthUser(credential.Account), Kind: credential.Kind, Scheme: scheme,
		TokenID: credential.TokenID, Scopes: append([]string(nil), credential.Scopes...),
		ExpiresAt: credential.ExpiresAt,
	}
}

func handleBasicAuth(state *core.AppState, authHeader string, c fiber.Ctx) (*authResult, error) {
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
	credential, err := VerifyAccountCredential(state, accessToken, password)
	if errors.Is(err, errCredentialExpired) {
		return nil, fiber.ErrForbidden
	}
	if err != nil || credential == nil {
		return nil, err
	}
	result := authResultFromCredential(credential, "basic")
	setAuthTokenExpiry(c, result.ExpiresAt)
	return result, nil
}

func handleSessionAuth(state *core.AppState, authHeader string, c fiber.Ctx) (*authResult, error) {
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
		return nil, fiber.ErrForbidden
	}

	return &authResult{
		User: synthUserForToken(accessToken, username), Kind: credentialKindSession, Scheme: "cookie",
		ExpiresAt: accessToken.ExpiresAt,
	}, nil
}

func handleBearerAuth(state *core.AppState, authHeader string, c fiber.Ctx) (*authResult, error) {
	bearerAuth := strings.TrimPrefix(authHeader, "Bearer ")
	var credential *VerifiedCredential
	var err error
	idx := strings.IndexByte(bearerAuth, ':')
	if idx > 0 {
		username := strings.ToLower(bearerAuth[:idx])
		secret := bearerAuth[idx+1:]

		accessToken := state.GetTokenByName(username)
		if accessToken == nil {
			return nil, nil
		}

		credential, err = VerifyAccountCredential(state, accessToken, secret)
	} else {
		credential, err = VerifyBearerCredential(state, bearerAuth)
	}
	if errors.Is(err, errCredentialExpired) {
		return nil, fiber.ErrForbidden
	}
	if err != nil || credential == nil {
		return nil, err
	}
	result := authResultFromCredential(credential, "bearer")
	setAuthTokenExpiry(c, result.ExpiresAt)
	return result, nil
}

func isRepositoryRequest(c fiber.Ctx, state *core.AppState) bool {
	if state == nil || state.Inner == nil || strings.HasPrefix(c.Path(), "/api/") {
		return false
	}
	path := strings.TrimPrefix(c.Path(), "/")
	repository, _, _ := strings.Cut(path, "/")
	if repository == "" {
		return false
	}
	cfg := state.Inner.Config.Load()
	return cfg != nil && cfg.Maven.Repositories[repository] != nil
}

func isStandardCredentialRequest(c fiber.Ctx, state *core.AppState) bool {
	path := c.Path()
	return isDockerRequest(c) || isRepositoryRequest(c, state) ||
		strings.HasPrefix(path, "/api/upload/chunked")
}

func isSessionOnlyAPIPath(path string) bool {
	for _, prefix := range []string{
		"/api/auth/logout",
		"/api/auth/profile/security",
		"/api/auth/profile/email",
		"/api/auth/profile/password",
		"/api/auth/profile/password-login",
		"/api/auth/profile/recovery-codes",
		"/api/auth/profile/token",
		"/api/auth/profile/sessions",
		"/api/auth/profile/fido",
		"/api/auth/profile/github",
		"/api/auth/profile/gpg",
		"/api/auth/profile/api-tokens",
		"/api/auth/github/start",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func packageManagementScope(path, method string) string {
	if strings.Contains(path, "/owners") || strings.Contains(path, "/users/search") ||
		strings.Contains(path, "/invitations/") {
		return APITokenScopePackageManage
	}
	switch method {
	case fiber.MethodGet, fiber.MethodHead:
		return APITokenScopeRepositoryRead
	case fiber.MethodDelete:
		return APITokenScopeRepositoryDelete
	default:
		return APITokenScopePackageManage
	}
}

func mavenAPITokenScope(path, method string) string {
	if strings.HasPrefix(path, "/api/maven/details") ||
		strings.HasPrefix(path, "/api/maven/repo-details") ||
		strings.HasPrefix(path, "/api/maven/signatures") ||
		strings.HasPrefix(path, "/api/maven/versions") ||
		strings.HasPrefix(path, "/api/maven/latest/") ||
		strings.HasPrefix(path, "/api/maven/generate/pom/") {
		return APITokenScopeRepositoryRead
	}
	if strings.HasPrefix(path, "/api/maven/repositories/") {
		if strings.Contains(path, "/domains") || strings.Contains(path, "/invitations/") {
			return APITokenScopeDomainManage
		}
		switch {
		case method == fiber.MethodGet || method == fiber.MethodHead:
			return APITokenScopeRepositoryRead
		case method == fiber.MethodDelete:
			return APITokenScopeRepositoryDelete
		case strings.HasSuffix(path, "/package"):
			return APITokenScopePackageManage
		}
	}
	return APITokenScopeDomainManage
}

func requiredAPITokenScope(c fiber.Ctx, state *core.AppState) string {
	path := c.Path()
	method := c.Method()
	if isRepositoryRequest(c, state) {
		lowerPath := strings.ToLower(c.Path())
		if strings.Contains(lowerPath, "/api/v1/crates/") {
			if strings.Contains(lowerPath, "/owners") ||
				(method != fiber.MethodDelete && (strings.HasSuffix(lowerPath, "/archive") ||
					strings.HasSuffix(lowerPath, "/yank") || strings.HasSuffix(lowerPath, "/unyank"))) {
				return APITokenScopePackageManage
			}
		}
		switch method {
		case fiber.MethodGet, fiber.MethodHead:
			return APITokenScopeRepositoryRead
		case fiber.MethodDelete:
			return APITokenScopeRepositoryDelete
		default:
			return APITokenScopeRepositoryPublish
		}
	}
	switch {
	case strings.HasPrefix(path, "/v2"):
		return ""
	case strings.HasPrefix(path, "/api/settings/repositories") ||
		strings.HasPrefix(path, "/api/settings/maven/repositories") ||
		strings.HasPrefix(path, "/api/settings/index/"):
		return APITokenScopeAdminRepositories
	case strings.HasPrefix(path, "/api/settings") || strings.HasPrefix(path, "/api/debug"):
		return APITokenScopeAdminSettings
	case strings.HasPrefix(path, "/api/auth/users/") && strings.Contains(path, "/audit-logs"):
		return APITokenScopeAdminAudit
	case strings.HasPrefix(path, "/api/tokens") || strings.HasPrefix(path, "/api/auth/users/"):
		return APITokenScopeAdminUsers
	case strings.HasPrefix(path, "/api/updater"):
		return APITokenScopeAdminUpdates
	case path == "/api/status/instance" || path == "/api/status/snapshots":
		return APITokenScopeAdminAudit
	case strings.HasPrefix(path, "/api/messages/admin"):
		return APITokenScopeAdminNotifications
	case strings.HasPrefix(path, "/api/messages"):
		return APITokenScopeMessagesRead
	case strings.HasPrefix(path, "/api/statistics/admin") || strings.HasPrefix(path, "/api/statistics/system"):
		return APITokenScopeAdminStatistics
	case strings.HasPrefix(path, "/api/statistics"):
		return APITokenScopeStatisticsRead
	case strings.HasPrefix(path, "/api/maven"):
		return mavenAPITokenScope(path, method)
	case strings.HasPrefix(path, "/api/cargo") || strings.HasPrefix(path, "/api/docker"):
		return packageManagementScope(path, method)
	case strings.HasPrefix(path, "/api/upload/chunked"):
		return APITokenScopeRepositoryPublish
	case path == "/api/auth/me":
		return APITokenScopeAccountRead
	case path == "/api/auth/profile" && (method == fiber.MethodGet || method == fiber.MethodHead):
		return APITokenScopeAccountRead
	case path == "/api/auth/profile":
		return APITokenScopeAccountWrite
	case strings.HasPrefix(path, "/api/auth/profile/audit-logs"):
		if method == fiber.MethodGet || method == fiber.MethodHead {
			return APITokenScopeAccountRead
		}
		return APITokenScopeAccountWrite
	case strings.HasPrefix(path, "/api/repositories"):
		if method == fiber.MethodGet || method == fiber.MethodHead {
			return APITokenScopeRepositoryRead
		}
		return APITokenScopeAdminRepositories
	case method == fiber.MethodGet || method == fiber.MethodHead:
		return APITokenScopeRepositoryRead
	default:
		return APITokenScopeRepositoryPublish
	}
}

func sendInsufficientScope(c fiber.Ctx, scope string) error {
	c.Set("X-Renop-Required-Scope", scope)
	c.Set(fiber.HeaderWWWAuthenticate, `Bearer error="insufficient_scope", scope="`+scope+`"`)
	return c.Status(fiber.StatusForbidden).SendString("API token scope is insufficient")
}

func authorizeCredential(c fiber.Ctx, state *core.AppState, result *authResult) (bool, error) {
	if result == nil {
		return true, nil
	}
	if result.Kind == credentialKindPassword && !isStandardCredentialRequest(c, state) {
		return false, c.Status(fiber.StatusForbidden).SendString("Password credentials are limited to package protocols")
	}
	if result.Kind != credentialKindAPIToken {
		return true, nil
	}
	if isSessionOnlyAPIPath(c.Path()) {
		return false, c.Status(fiber.StatusForbidden).SendString("A browser session is required")
	}
	if (result.Scheme == "basic" || result.Scheme == "cargo") && !isStandardCredentialRequest(c, state) {
		return false, c.Status(fiber.StatusForbidden).SendString("Basic credentials are limited to package protocols")
	}
	required := requiredAPITokenScope(c, state)
	if required != "" && !slices.Contains(result.Scopes, required) {
		return false, sendInsufficientScope(c, required)
	}
	return true, nil
}

func authorizeRequest(c fiber.Ctx, user *config.User) error {
	path := c.Path()
	restricted := strings.HasPrefix(path, "/api/settings") ||
		strings.HasPrefix(path, "/api/tokens") ||
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

func credentialCacheKey(authHeader string, opaqueCargo bool) string {
	namespace := "http\x00"
	if opaqueCargo {
		namespace = "cargo\x00"
	}
	digest := sha256.Sum256([]byte(namespace + authHeader))
	return "credential:" + hex.EncodeToString(digest[:])
}

func AuthMiddleware(state *core.AppState) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Path() == "/api/auth/login" {
			c.Locals("user", GuestUser)
			return c.Next()
		}

		explicitAuthHeader := c.Get(fiber.HeaderAuthorization, "")
		if strings.HasPrefix(explicitAuthHeader, "Session ") {
			return c.Status(fiber.StatusUnauthorized).SendString("Session credentials must use the browser cookie")
		}
		var authenticated *authResult
		authHeader := strings.Clone(extractAuthHeader(c, state))
		isSessionAuth := strings.HasPrefix(authHeader, "Session ")
		isCargoRequest := isCargoRepositoryRequest(c, state)
		isOpaqueCargoAuth := authHeader != "" && isCargoRequest &&
			!strings.HasPrefix(authHeader, "Basic ") &&
			!strings.HasPrefix(authHeader, "Session ") &&
			!strings.HasPrefix(authHeader, "Bearer ")
		authCacheKey := ""
		if authHeader != "" {
			authCacheKey = credentialCacheKey(authHeader, isOpaqueCargoAuth)
		}

		if authHeader != "" {
			if !isSessionAuth {
				if val, ok := state.Inner.AuthCache.Load(authCacheKey); ok {
					if time.Now().UnixMilli() < val.ExpiredAt {
						authenticated = &authResult{
							User: val.User, Kind: val.CredentialKind, Scheme: val.AuthScheme,
							TokenID: val.APITokenID, Scopes: append([]string(nil), val.Scopes...),
						}
					} else {
						state.DeleteAuthCache(authCacheKey)
					}
				}
			}

			if authenticated == nil {
				var err error
				if strings.HasPrefix(authHeader, "Basic ") {
					authenticated, err = handleBasicAuth(state, authHeader, c)
				} else if strings.HasPrefix(authHeader, "Session ") {
					authenticated, err = handleSessionAuth(state, authHeader, c)
				} else if strings.HasPrefix(authHeader, "Bearer ") {
					authenticated, err = handleBearerAuth(state, authHeader, c)
				} else if isOpaqueCargoAuth {
					// Cargo registry credentials are opaque and are sent as the
					// complete Authorization value without a Bearer prefix.
					authenticated, err = handleBearerAuth(state, "Bearer "+authHeader, c)
					if authenticated != nil {
						authenticated.Scheme = "cargo"
					}
				} else {
					return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
				}

				if err != nil {
					return err
				}

				if authenticated != nil {
					if !isSessionAuth {
						state.StoreAuthCache(authCacheKey, core.AuthCacheEntry{
							User: authenticated.User, CredentialKind: authenticated.Kind,
							AuthScheme: authenticated.Scheme, APITokenID: authenticated.TokenID,
							Scopes:    append([]string(nil), authenticated.Scopes...),
							ExpiredAt: authCacheExpiry(c, time.Now().UnixMilli()),
						})
					}
				} else if !isSessionAuth {
					state.StoreAuthCache(authCacheKey, core.AuthCacheEntry{
						User: InvalidCredentialsUser, CredentialKind: credentialKindInvalid,
						ExpiredAt: time.Now().Add(30 * time.Second).UnixMilli(),
					})
				}
			} else if authenticated.User == InvalidCredentialsUser {
				if isCargoRequest {
					return sendInvalidCargoCredentials(c)
				}
				if isDockerRequest(c) {
					c.Locals("user", GuestUser)
					return c.Next()
				}
				return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
			}
		}

		isLogout := c.Path() == "/api/auth/logout"
		if authHeader != "" && authenticated == nil && !isLogout {
			if isCargoRequest {
				return sendInvalidCargoCredentials(c)
			}
			if isDockerRequest(c) {
				c.Locals("user", GuestUser)
				return c.Next()
			}
			return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
		}

		user := GuestUser
		if authenticated != nil && authenticated.User != nil {
			user = authenticated.User
			setCredentialLocals(c, authenticated)
			allowed, err := authorizeCredential(c, state, authenticated)
			if err != nil {
				return err
			}
			if !allowed {
				return nil
			}
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
	credential, err := VerifyAccountCredential(state, accessToken, secret)
	return err == nil && credential != nil
}

func synthUserForToken(accessToken *core.AccessToken, username string) *config.User {
	if accessToken.Name != username {
		tCopy := *accessToken
		tCopy.Name = username
		accessToken = &tCopy
	}
	return buildSynthUser(accessToken)
}
