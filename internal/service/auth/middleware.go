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
	Targets   map[string][]string
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
		Targets:   core.CloneAPITokenTargets(credential.Targets),
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

type apiTokenRequirement struct {
	Scope        string
	Alternatives []string
	Target       string
	DeferTarget  bool
}

func (requirement apiTokenRequirement) allows(scopes []string, targets map[string][]string) bool {
	if requirement.Scope == "" {
		return true
	}
	if requirement.DeferTarget {
		if slices.Contains(scopes, requirement.Scope) {
			return true
		}
		for _, alternative := range requirement.Alternatives {
			if slices.Contains(scopes, alternative) {
				return true
			}
		}
		return false
	}
	if apiTokenAuthorizationAllows(scopes, targets, requirement.Scope, requirement.Target) {
		return true
	}
	for _, alternative := range requirement.Alternatives {
		if apiTokenAuthorizationAllows(scopes, targets, alternative, requirement.Target) {
			return true
		}
	}
	return false
}

func requireAPITokenScope(scope string, alternatives ...string) apiTokenRequirement {
	return apiTokenRequirement{Scope: scope, Alternatives: alternatives}
}

func requireAPITokenTarget(scope, target string, alternatives ...string) apiTokenRequirement {
	return apiTokenRequirement{Scope: scope, Alternatives: alternatives, Target: target}
}

func requireAPITokenDeferredTarget(scope string, alternatives ...string) apiTokenRequirement {
	return apiTokenRequirement{Scope: scope, Alternatives: alternatives, DeferTarget: true}
}

func repositoryTargetFromPath(path string) string {
	path = strings.TrimPrefix(path, "/")
	repository, _, _ := strings.Cut(path, "/")
	return repository
}

func cargoPackageTarget(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 5 && parts[1] == "api" && parts[2] == "v1" && parts[3] == "crates" {
		return parts[0] + "/" + parts[4]
	}
	return ""
}

func packageAPIRepository(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 4 && parts[0] == "api" &&
		(parts[1] == "docker" || parts[1] == "npm") && parts[2] == "repositories" {
		return parts[3]
	}
	return ""
}

func packageAPITarget(c fiber.Ctx) string {
	repository := packageAPIRepository(c.Path())
	if repository == "" {
		return ""
	}
	image := strings.Trim(c.Query("image"), "/")
	if image == "" {
		image = strings.Trim(c.Query("package"), "/")
	}
	if image == "" {
		parts := strings.Split(strings.Trim(c.Path(), "/"), "/")
		if len(parts) >= 6 && (parts[4] == "images" || parts[4] == "manifests" || parts[4] == "tags") {
			image = strings.Join(parts[5:], "/")
		}
	}
	if image == "" {
		return ""
	}
	return repository + "/" + image
}

func packageManagementRequirement(c fiber.Ctx) apiTokenRequirement {
	path, method := c.Path(), c.Method()
	repository := packageAPIRepository(path)
	packageTarget := packageAPITarget(c)
	if strings.Contains(path, "/owners") || strings.Contains(path, "/users/search") ||
		strings.Contains(path, "/invitations/") {
		teamTarget := ""
		if packageTarget != "" {
			teamTarget = "package/" + packageTarget
		}
		return requireAPITokenTarget(APITokenScopeTeamManage, teamTarget, APITokenScopePackageManage)
	}
	if strings.Contains(path, "/versions") || method == fiber.MethodDelete && strings.Contains(path, "/packages") {
		return requireAPITokenTarget(APITokenScopePackageLifecycle, packageTarget, APITokenScopePackageManage)
	}
	switch method {
	case fiber.MethodGet, fiber.MethodHead:
		return requireAPITokenTarget(APITokenScopeRepositoryRead, repository)
	case fiber.MethodDelete:
		return requireAPITokenTarget(APITokenScopeRepositoryDelete, repository)
	case fiber.MethodPost:
		return requireAPITokenTarget(APITokenScopePackageCreate, repository, APITokenScopePackageManage)
	default:
		return requireAPITokenTarget(APITokenScopePackageMetadata, packageTarget, APITokenScopePackageManage)
	}
}

func mavenPackageTarget(c fiber.Ctx) string {
	parts := strings.Split(strings.Trim(c.Path(), "/"), "/")
	if len(parts) < 5 || parts[0] != "api" || parts[1] != "maven" || parts[2] != "repositories" {
		return ""
	}
	groupID := strings.Trim(c.Query("group"), "/")
	artifactID := strings.Trim(c.Query("artifact"), "/")
	if groupID == "" || artifactID == "" {
		return ""
	}
	return parts[3] + "/" + groupID + "/" + artifactID
}

func mavenAPIRepository(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "maven" && parts[2] == "repositories" {
		return parts[3]
	}
	return ""
}

func mavenDomainTarget(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for index, part := range parts {
		if part == "domains" && index+1 < len(parts) {
			return strings.ToLower(parts[index+1])
		}
	}
	return ""
}

func mavenAPITokenRequirement(c fiber.Ctx) apiTokenRequirement {
	path, method := c.Path(), c.Method()
	if strings.HasPrefix(path, "/api/maven/details") ||
		strings.HasPrefix(path, "/api/maven/repo-details") ||
		strings.HasPrefix(path, "/api/maven/signatures") ||
		strings.HasPrefix(path, "/api/maven/versions") ||
		strings.HasPrefix(path, "/api/maven/latest/") ||
		strings.HasPrefix(path, "/api/maven/generate/pom/") {
		return requireAPITokenScope(APITokenScopeRepositoryRead)
	}
	if strings.Contains(path, "/invitations/") || strings.Contains(path, "/members") {
		domain := mavenDomainTarget(path)
		teamTarget := ""
		if domain != "" {
			teamTarget = "domain/" + domain
		}
		return requireAPITokenTarget(APITokenScopeTeamManage, teamTarget, APITokenScopeDomainManage)
	}
	if domainIndex := strings.Index(path, "/domains"); domainIndex >= 0 {
		domainTail := strings.Trim(path[domainIndex+len("/domains"):], "/")
		domain := mavenDomainTarget(path)
		switch {
		case strings.Contains(domainTail, "/verify"):
			return requireAPITokenTarget(APITokenScopeDomainVerify, domain, APITokenScopeDomainManage)
		case method == fiber.MethodDelete:
			return requireAPITokenTarget(APITokenScopeDomainDelete, domain, APITokenScopeDomainManage)
		case method == fiber.MethodPost && domainTail == "":
			return requireAPITokenDeferredTarget(APITokenScopeDomainCreate, APITokenScopeDomainManage)
		default:
			return requireAPITokenTarget(APITokenScopeDomainRead, domain, APITokenScopeDomainManage)
		}
	}
	if strings.HasPrefix(path, "/api/maven/repositories/") {
		repository := mavenAPIRepository(path)
		switch {
		case method == fiber.MethodGet || method == fiber.MethodHead:
			return requireAPITokenTarget(APITokenScopeRepositoryRead, repository)
		case method == fiber.MethodDelete:
			return requireAPITokenTarget(APITokenScopeRepositoryDelete, repository)
		case strings.HasSuffix(path, "/package"):
			return requireAPITokenTarget(APITokenScopePackageMetadata, mavenPackageTarget(c), APITokenScopePackageManage)
		}
	}
	return requireAPITokenScope(APITokenScopeDomainRead, APITokenScopeDomainManage)
}

func superTeamAPITokenRequirement(c fiber.Ctx) apiTokenRequirement {
	path, method := c.Path(), c.Method()
	if path == "/api/super-teams/limits" {
		return requireAPITokenScope(APITokenScopeAccountRead)
	}
	if path == "/api/super-teams/eligible" {
		return requireAPITokenScope(APITokenScopeTeamManage)
	}
	if strings.HasPrefix(path, "/api/super-teams/users/") {
		return requireAPITokenScope(APITokenScopeAdminUsers)
	}
	if path == "/api/super-teams" || path == "/api/super-teams/" {
		if method == fiber.MethodPost {
			return requireAPITokenDeferredTarget(APITokenScopeTeamManage)
		}
		return requireAPITokenScope(APITokenScopeTeamManage)
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "super-teams" && parts[2] != "invitations" {
		if prefix, ok := core.NormalizeSuperTeamPrefix(parts[2]); ok {
			return requireAPITokenTarget(APITokenScopeTeamManage, "global/"+prefix)
		}
	}
	return requireAPITokenScope(APITokenScopeTeamManage)
}

func requiredAPITokenScope(c fiber.Ctx, state *core.AppState) apiTokenRequirement {
	path := c.Path()
	method := c.Method()
	if isRepositoryRequest(c, state) {
		lowerPath := strings.ToLower(c.Path())
		repository := repositoryTargetFromPath(c.Path())
		cfg := state.Inner.Config.Load()
		repo := cfg.Maven.Repositories[repository]
		if repo != nil && repo.NormalizedFormat() == config.RepositoryFormatNPM {
			switch method {
			case fiber.MethodGet, fiber.MethodHead:
				return requireAPITokenTarget(APITokenScopeRepositoryRead, repository)
			case fiber.MethodPut:
				return requireAPITokenScope(APITokenScopeRepositoryPublish,
					APITokenScopePackageLifecycle, APITokenScopePackageManage)
			case fiber.MethodDelete:
				return requireAPITokenScope(APITokenScopePackageLifecycle, APITokenScopePackageManage)
			}
		}
		packageTarget := cargoPackageTarget(c.Path())
		if strings.Contains(lowerPath, "/api/v1/invitations/") || strings.Contains(lowerPath, "/owners") {
			teamTarget := ""
			if packageTarget != "" {
				teamTarget = "package/" + packageTarget
			}
			return requireAPITokenTarget(APITokenScopeTeamManage, teamTarget, APITokenScopePackageManage)
		}
		if strings.HasSuffix(lowerPath, "/archive") || strings.HasSuffix(lowerPath, "/yank") ||
			strings.HasSuffix(lowerPath, "/unyank") {
			return requireAPITokenTarget(APITokenScopePackageLifecycle, packageTarget, APITokenScopePackageManage)
		}
		switch method {
		case fiber.MethodGet, fiber.MethodHead:
			return requireAPITokenTarget(APITokenScopeRepositoryRead, repository)
		case fiber.MethodDelete:
			return requireAPITokenTarget(APITokenScopeRepositoryDelete, repository)
		default:
			return requireAPITokenTarget(APITokenScopeRepositoryPublish, repository)
		}
	}
	switch {
	case strings.HasPrefix(path, "/v2"):
		return apiTokenRequirement{}
	case strings.HasPrefix(path, "/api/settings/repositories") ||
		strings.HasPrefix(path, "/api/settings/maven/repositories") ||
		strings.HasPrefix(path, "/api/settings/index/"):
		return requireAPITokenScope(APITokenScopeAdminRepositories)
	case strings.HasPrefix(path, "/api/settings") || strings.HasPrefix(path, "/api/debug"):
		return requireAPITokenScope(APITokenScopeAdminSettings)
	case strings.HasPrefix(path, "/api/auth/users/") && strings.Contains(path, "/audit-logs"):
		return requireAPITokenScope(APITokenScopeAdminAudit)
	case strings.HasPrefix(path, "/api/tokens") || strings.HasPrefix(path, "/api/auth/users/"):
		return requireAPITokenScope(APITokenScopeAdminUsers)
	case strings.HasPrefix(path, "/api/updater"):
		return requireAPITokenScope(APITokenScopeAdminUpdates)
	case path == "/api/status/instance" || path == "/api/status/snapshots":
		return requireAPITokenScope(APITokenScopeAdminAudit)
	case strings.HasPrefix(path, "/api/messages/admin"):
		return requireAPITokenScope(APITokenScopeAdminNotifications)
	case strings.HasPrefix(path, "/api/messages"):
		return requireAPITokenScope(APITokenScopeMessagesRead)
	case strings.HasPrefix(path, "/api/statistics/admin") || strings.HasPrefix(path, "/api/statistics/system"):
		return requireAPITokenScope(APITokenScopeAdminStatistics)
	case strings.HasPrefix(path, "/api/statistics"):
		return requireAPITokenScope(APITokenScopeStatisticsRead)
	case strings.HasPrefix(path, "/api/super-teams"):
		return superTeamAPITokenRequirement(c)
	case strings.HasPrefix(path, "/api/reviews"):
		return requireAPITokenScope(APITokenScopeTeamManage)
	case strings.HasPrefix(path, "/api/maven"):
		return mavenAPITokenRequirement(c)
	case strings.HasPrefix(path, "/api/cargo") || strings.HasPrefix(path, "/api/docker") ||
		strings.HasPrefix(path, "/api/npm"):
		return packageManagementRequirement(c)
	case strings.HasPrefix(path, "/api/upload/chunked"):
		return requireAPITokenDeferredTarget(APITokenScopeRepositoryPublish)
	case path == "/api/auth/me":
		return requireAPITokenScope(APITokenScopeAccountRead)
	case path == "/api/auth/profile" && (method == fiber.MethodGet || method == fiber.MethodHead):
		return requireAPITokenScope(APITokenScopeAccountRead)
	case path == "/api/auth/profile":
		return requireAPITokenScope(APITokenScopeAccountWrite)
	case strings.HasPrefix(path, "/api/auth/profile/audit-logs"):
		if method == fiber.MethodGet || method == fiber.MethodHead {
			return requireAPITokenScope(APITokenScopeAccountRead)
		}
		return requireAPITokenScope(APITokenScopeAccountWrite)
	case strings.HasPrefix(path, "/api/repositories"):
		if method == fiber.MethodGet || method == fiber.MethodHead {
			return requireAPITokenScope(APITokenScopeRepositoryRead)
		}
		return requireAPITokenScope(APITokenScopeAdminRepositories)
	case method == fiber.MethodGet || method == fiber.MethodHead:
		return requireAPITokenScope(APITokenScopeRepositoryRead)
	default:
		return requireAPITokenScope(APITokenScopeRepositoryPublish)
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
	if !required.allows(result.Scopes, result.Targets) {
		return false, sendInsufficientScope(c, required.Scope)
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
							Targets: core.CloneAPITokenTargets(val.Targets),
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
							Targets:   core.CloneAPITokenTargets(authenticated.Targets),
							ExpiredAt: authCacheExpiry(c, time.Now().UnixMilli()),
						})
					}
				} else if !isSessionAuth {
					state.StoreAuthCache(authCacheKey, core.AuthCacheEntry{
						User: InvalidCredentialsUser, CredentialKind: credentialKindInvalid,
						ExpiredAt: time.Now().Add(30 * time.Second).UnixMilli(), Invalid: true,
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
