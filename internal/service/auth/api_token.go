/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"

	"renop/internal/core"
	"renop/internal/utils"
)

const (
	APITokenScopeRepositoryRead     = core.APITokenScopeRepositoryRead
	APITokenScopeRepositoryPublish  = core.APITokenScopeRepositoryPublish
	APITokenScopeRepositoryDelete   = core.APITokenScopeRepositoryDelete
	APITokenScopePackageCreate      = core.APITokenScopePackageCreate
	APITokenScopePackageMetadata    = core.APITokenScopePackageMetadata
	APITokenScopePackageLifecycle   = core.APITokenScopePackageLifecycle
	APITokenScopeTeamManage         = core.APITokenScopeTeamManage
	APITokenScopeDomainRead         = core.APITokenScopeDomainRead
	APITokenScopeDomainCreate       = core.APITokenScopeDomainCreate
	APITokenScopeDomainVerify       = core.APITokenScopeDomainVerify
	APITokenScopeDomainDelete       = core.APITokenScopeDomainDelete
	APITokenScopePackageManage      = core.APITokenScopePackageManage
	APITokenScopeDomainManage       = core.APITokenScopeDomainManage
	APITokenScopeMessagesRead       = core.APITokenScopeMessagesRead
	APITokenScopeAccountRead        = core.APITokenScopeAccountRead
	APITokenScopeAccountWrite       = core.APITokenScopeAccountWrite
	APITokenScopeStatisticsRead     = core.APITokenScopeStatisticsRead
	APITokenScopeAdminUsers         = core.APITokenScopeAdminUsers
	APITokenScopeAdminRepositories  = core.APITokenScopeAdminRepositories
	APITokenScopeAdminSettings      = core.APITokenScopeAdminSettings
	APITokenScopeAdminAudit         = core.APITokenScopeAdminAudit
	APITokenScopeAdminNotifications = core.APITokenScopeAdminNotifications
	APITokenScopeAdminUpdates       = core.APITokenScopeAdminUpdates
	APITokenScopeAdminStatistics    = core.APITokenScopeAdminStatistics

	credentialKindSession  = "session"
	credentialKindPassword = "password"
	credentialKindAPIToken = "api_token"
	credentialKindInvalid  = "invalid"
	credentialKindLocal    = "auth_credential_kind"
	authSchemeLocal        = "auth_scheme"
	apiTokenScopesLocal    = "auth_api_token_scopes"
	apiTokenTargetsLocal   = "auth_api_token_targets"
	apiTokenIDLocal        = "auth_api_token_id"
)

var (
	errCredentialExpired     = errors.New("credential expired")
	errAPITokenNameInvalid   = errors.New("API token name is invalid")
	errAPITokenScopesInvalid = errors.New("API token scopes are invalid")
	errAPITokenExpiryInvalid = errors.New("API token expiration is invalid")
)

type apiTokenScopeDefinition struct {
	Scope       string
	ManagerOnly bool
	TargetKind  string
}

var apiTokenScopeDefinitions = []apiTokenScopeDefinition{
	{Scope: APITokenScopeRepositoryRead, TargetKind: "repository"},
	{Scope: APITokenScopeRepositoryPublish, TargetKind: "repository"},
	{Scope: APITokenScopeRepositoryDelete, TargetKind: "repository"},
	{Scope: APITokenScopePackageCreate, TargetKind: "repository"},
	{Scope: APITokenScopePackageMetadata, TargetKind: "package"},
	{Scope: APITokenScopePackageLifecycle, TargetKind: "package"},
	{Scope: APITokenScopeTeamManage, TargetKind: "team"},
	{Scope: APITokenScopeDomainRead, TargetKind: "domain"},
	{Scope: APITokenScopeDomainCreate, TargetKind: "domain"},
	{Scope: APITokenScopeDomainVerify, TargetKind: "domain"},
	{Scope: APITokenScopeDomainDelete, TargetKind: "domain"},
	{Scope: APITokenScopeMessagesRead},
	{Scope: APITokenScopeAccountRead},
	{Scope: APITokenScopeAccountWrite},
	{Scope: APITokenScopeStatisticsRead},
	{Scope: APITokenScopeAdminUsers, ManagerOnly: true},
	{Scope: APITokenScopeAdminRepositories, ManagerOnly: true},
	{Scope: APITokenScopeAdminSettings, ManagerOnly: true},
	{Scope: APITokenScopeAdminAudit, ManagerOnly: true},
	{Scope: APITokenScopeAdminNotifications, ManagerOnly: true},
	{Scope: APITokenScopeAdminUpdates, ManagerOnly: true},
	{Scope: APITokenScopeAdminStatistics, ManagerOnly: true},
}

// VerifiedCredential is an authenticated non-session account credential.
type VerifiedCredential struct {
	Account   *core.AccessToken
	Kind      string
	TokenID   string
	Scopes    []string
	Targets   map[string][]string
	ExpiresAt *int64
}

// HasScope reports whether a credential is unrestricted by API scopes or includes scope.
func (credential *VerifiedCredential) HasScope(scope string) bool {
	return credential != nil && (credential.Kind != credentialKindAPIToken || slices.Contains(credential.Scopes, scope))
}

// HasScopeTarget reports whether a credential carries scope and permits the exact canonical target.
func (credential *VerifiedCredential) HasScopeTarget(scope, target string) bool {
	return credential != nil && (credential.Kind != credentialKindAPIToken ||
		apiTokenAuthorizationAllows(credential.Scopes, credential.Targets, scope, target))
}

func apiTokenSecretHash(secret string) string {
	return core.HashAPITokenSecret(secret)
}

func generateAPITokenSecret() (string, error) {
	return core.GenerateAPITokenSecret()
}

func allowedAPITokenScopes(account *core.AccessToken) []string {
	manager := account != nil && isManagerPermissions(account.Permissions)
	scopes := make([]string, 0, len(apiTokenScopeDefinitions))
	for _, definition := range apiTokenScopeDefinitions {
		if !definition.ManagerOnly || manager {
			scopes = append(scopes, definition.Scope)
		}
	}
	return scopes
}

func allowedAPITokenTargetKinds(account *core.AccessToken) map[string]string {
	manager := account != nil && isManagerPermissions(account.Permissions)
	targetKinds := make(map[string]string)
	for _, definition := range apiTokenScopeDefinitions {
		if definition.TargetKind != "" && (!definition.ManagerOnly || manager) {
			targetKinds[definition.Scope] = definition.TargetKind
		}
	}
	return targetKinds
}

func normalizeAPITokenScopes(account *core.AccessToken, requested []string) ([]string, error) {
	if len(requested) == 0 || len(requested) > core.MaxAPITokenScopes {
		return nil, errAPITokenScopesInvalid
	}
	allowed := allowedAPITokenScopes(account)
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, scope := range allowed {
		allowedSet[scope] = struct{}{}
	}
	normalized := make([]string, 0, len(requested))
	for _, scope := range requested {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if _, ok := allowedSet[scope]; !ok {
			return nil, errAPITokenScopesInvalid
		}
		normalized = append(normalized, scope)
	}
	slices.Sort(normalized)
	normalized = slices.Compact(normalized)
	if len(normalized) == 0 {
		return nil, errAPITokenScopesInvalid
	}
	return normalized, nil
}

func normalizeRepositoryTarget(value string) (string, bool) {
	value = strings.TrimSpace(value)
	return value, utils.IsValidRepositoryName(value) && len(value) <= 64
}

func normalizeDomainTarget(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if value == "" || len(value) > 253 || strings.ContainsAny(value, "/\\") {
		return "", false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return "", false
		}
	}
	return value, true
}

func normalizePackageTarget(value string) (string, bool) {
	value = strings.Trim(strings.TrimSpace(value), "/")
	repository, name, ok := strings.Cut(value, "/")
	if !ok || name == "" || strings.Contains(name, "\\") || strings.Contains(name, "//") {
		return "", false
	}
	if _, valid := normalizeRepositoryTarget(repository); !valid {
		return "", false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
		for _, character := range part {
			if unicode.IsControl(character) {
				return "", false
			}
		}
	}
	return repository + "/" + name, len(value) <= core.MaxAPITokenTargetLength
}

func normalizeTeamTarget(value string) (string, bool) {
	value = strings.Trim(strings.TrimSpace(value), "/")
	switch {
	case strings.HasPrefix(value, "domain/"):
		domain, ok := normalizeDomainTarget(strings.TrimPrefix(value, "domain/"))
		return "domain/" + domain, ok
	case strings.HasPrefix(value, "package/"):
		pkg, ok := normalizePackageTarget(strings.TrimPrefix(value, "package/"))
		return "package/" + pkg, ok
	case strings.HasPrefix(value, "global/"):
		prefix, ok := core.NormalizeSuperTeamPrefix(strings.TrimPrefix(value, "global/"))
		return "global/" + prefix, ok
	default:
		return "", false
	}
}

func normalizeAPITokenTarget(kind, value string) (string, bool) {
	switch kind {
	case "repository":
		return normalizeRepositoryTarget(value)
	case "domain":
		return normalizeDomainTarget(value)
	case "package":
		return normalizePackageTarget(value)
	case "team":
		return normalizeTeamTarget(value)
	default:
		return "", false
	}
}

func normalizeAPITokenTargets(account *core.AccessToken, scopes []string,
	requested map[string][]string) (map[string][]string, error) {
	if len(requested) == 0 {
		return nil, nil
	}
	selected := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		selected[scope] = struct{}{}
	}
	targetKinds := allowedAPITokenTargetKinds(account)
	normalized := make(map[string][]string, len(requested))
	total := 0
	for scope, values := range requested {
		if _, ok := selected[scope]; !ok || len(values) == 0 {
			return nil, errAPITokenScopesInvalid
		}
		kind, ok := targetKinds[scope]
		if !ok {
			return nil, errAPITokenScopesInvalid
		}
		targets := make([]string, 0, len(values))
		for _, value := range values {
			target, valid := normalizeAPITokenTarget(kind, value)
			if !valid {
				return nil, errAPITokenScopesInvalid
			}
			targets = append(targets, target)
		}
		slices.Sort(targets)
		targets = slices.Compact(targets)
		total += len(targets)
		normalized[scope] = targets
	}
	if total == 0 || total > core.MaxAPITokenTargets {
		return nil, errAPITokenScopesInvalid
	}
	return normalized, nil
}

func apiTokenAuthorizationAllows(scopes []string, targets map[string][]string, scope, target string) bool {
	if !slices.Contains(scopes, scope) {
		return false
	}
	restricted, ok := targets[scope]
	if !ok {
		return true
	}
	return target != "" && slices.Contains(restricted, target)
}

func effectiveCredentialExpiry(account, apiToken *int64) *int64 {
	if account == nil {
		return apiToken
	}
	if apiToken == nil || *account < *apiToken {
		return account
	}
	return apiToken
}

// VerifyAccountCredential validates an API token, migrated upload token, or enabled account password.
func VerifyAccountCredential(state *core.AppState, account *core.AccessToken, secret string) (*VerifiedCredential, error) {
	if state == nil || state.GetDB() == nil || account == nil || secret == "" {
		return nil, nil
	}
	if account.ExpiresAt != nil && time.Now().UnixMilli() >= *account.ExpiresAt {
		return nil, errCredentialExpired
	}
	credential, err := state.GetDB().GetAPITokenByHash(apiTokenSecretHash(secret), account.Name)
	if err != nil {
		return nil, err
	}
	if credential != nil && credential.Token != nil && credential.Account != nil {
		expiresAt := effectiveCredentialExpiry(credential.Account.ExpiresAt, credential.Token.ExpiresAt)
		if expiresAt != nil && time.Now().UnixMilli() >= *expiresAt {
			return nil, errCredentialExpired
		}
		return &VerifiedCredential{
			Account: credential.Account, Kind: credentialKindAPIToken, TokenID: credential.Token.ID,
			Scopes:  append([]string(nil), credential.Token.Scopes...),
			Targets: core.CloneAPITokenTargets(credential.Token.Targets), ExpiresAt: expiresAt,
		}, nil
	}
	for _, legacySecret := range account.Tokens {
		if secretEqual(legacySecret, secret) {
			return &VerifiedCredential{
				Account: account, Kind: credentialKindAPIToken,
				Scopes:    []string{APITokenScopeRepositoryPublish, APITokenScopeRepositoryRead},
				ExpiresAt: account.ExpiresAt,
			}, nil
		}
	}
	if account.EncryptedSecret == "" {
		return nil, nil
	}
	passwordEnabled, err := state.GetDB().PasswordLoginEnabled(account.Name)
	if err != nil || !passwordEnabled {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(account.EncryptedSecret), []byte(secret)); err != nil {
		return nil, nil
	}
	if account.ExpiresAt != nil && time.Now().UnixMilli() >= *account.ExpiresAt {
		return nil, errCredentialExpired
	}
	return &VerifiedCredential{
		Account: account, Kind: credentialKindPassword, ExpiresAt: account.ExpiresAt,
	}, nil
}

// VerifyBearerCredential resolves an API token supplied without an account-name prefix.
func VerifyBearerCredential(state *core.AppState, secret string) (*VerifiedCredential, error) {
	if state == nil || state.GetDB() == nil || secret == "" {
		return nil, nil
	}
	credential, err := state.GetDB().GetAPITokenByHash(apiTokenSecretHash(secret), "")
	if err != nil {
		return nil, err
	}
	if credential != nil && credential.Token != nil && credential.Account != nil {
		expiresAt := effectiveCredentialExpiry(credential.Account.ExpiresAt, credential.Token.ExpiresAt)
		if expiresAt != nil && time.Now().UnixMilli() >= *expiresAt {
			return nil, errCredentialExpired
		}
		return &VerifiedCredential{
			Account: credential.Account, Kind: credentialKindAPIToken, TokenID: credential.Token.ID,
			Scopes:  append([]string(nil), credential.Token.Scopes...),
			Targets: core.CloneAPITokenTargets(credential.Token.Targets), ExpiresAt: expiresAt,
		}, nil
	}
	legacyAccount := state.GetTokenBySecret(secret)
	if legacyAccount == nil {
		return nil, nil
	}
	return VerifyAccountCredential(state, legacyAccount, secret)
}

func setCredentialLocals(c fiber.Ctx, result *authResult) {
	if c == nil || result == nil {
		return
	}
	c.Locals(credentialKindLocal, result.Kind)
	c.Locals(authSchemeLocal, result.Scheme)
	c.Locals(apiTokenIDLocal, result.TokenID)
	c.Locals(apiTokenScopesLocal, append([]string(nil), result.Scopes...))
	c.Locals(apiTokenTargetsLocal, core.CloneAPITokenTargets(result.Targets))
}

// CurrentCredentialKind returns session, password, or api_token for the authenticated request.
func CurrentCredentialKind(c fiber.Ctx) string {
	value, _ := c.Locals(credentialKindLocal).(string)
	return value
}

// CurrentCredentialIsAPIToken reports whether the request uses a revocable API token.
func CurrentCredentialIsAPIToken(c fiber.Ctx) bool {
	return CurrentCredentialKind(c) == credentialKindAPIToken
}

// CurrentCredentialHasScope reports whether a request is not API-token-authenticated or carries scope.
func CurrentCredentialHasScope(c fiber.Ctx, scope string) bool {
	if CurrentCredentialKind(c) != credentialKindAPIToken {
		return true
	}
	scopes, _ := c.Locals(apiTokenScopesLocal).([]string)
	return slices.Contains(scopes, scope)
}

// CurrentCredentialHasAnyScope reports whether a request is unrestricted or carries at least one supplied scope.
func CurrentCredentialHasAnyScope(c fiber.Ctx, scopes ...string) bool {
	if CurrentCredentialKind(c) != credentialKindAPIToken {
		return true
	}
	granted, _ := c.Locals(apiTokenScopesLocal).([]string)
	for _, scope := range scopes {
		if slices.Contains(granted, scope) {
			return true
		}
	}
	return false
}

// CurrentCredentialHasScopeTarget reports whether a request is unrestricted or permits one canonical target.
func CurrentCredentialHasScopeTarget(c fiber.Ctx, scope, target string) bool {
	if CurrentCredentialKind(c) != credentialKindAPIToken {
		return true
	}
	scopes, _ := c.Locals(apiTokenScopesLocal).([]string)
	targets, _ := c.Locals(apiTokenTargetsLocal).(map[string][]string)
	return apiTokenAuthorizationAllows(scopes, targets, scope, target)
}
