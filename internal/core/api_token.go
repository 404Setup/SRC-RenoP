/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
)

const (
	// MaxAPITokensPerUser bounds durable credentials owned by one account.
	MaxAPITokensPerUser = 50
	// MaxAPITokenNameLength bounds user-controlled token labels.
	MaxAPITokenNameLength = 80
	// MaxAPITokenScopes bounds authorization work and persisted JSON size.
	MaxAPITokenScopes = 24
	// APITokenSecretBytes gives machine credentials 256 bits of entropy.
	APITokenSecretBytes = 32
	// LegacyAPITokenNamePrefix reserves metadata labels created by plaintext-token migration.
	LegacyAPITokenNamePrefix = "Migrated upload token "

	// API token scopes define the individual capabilities assignable to durable credentials.
	APITokenScopeRepositoryRead     = "repository:read"
	APITokenScopeRepositoryPublish  = "repository:publish"
	APITokenScopeRepositoryDelete   = "repository:delete"
	APITokenScopePackageManage      = "package:manage"
	APITokenScopeDomainManage       = "domain:manage"
	APITokenScopeMessagesRead       = "messages:read"
	APITokenScopeAccountRead        = "account:read"
	APITokenScopeAccountWrite       = "account:write"
	APITokenScopeStatisticsRead     = "statistics:read"
	APITokenScopeAdminUsers         = "admin:users"
	APITokenScopeAdminRepositories  = "admin:repositories"
	APITokenScopeAdminSettings      = "admin:settings"
	APITokenScopeAdminAudit         = "admin:audit"
	APITokenScopeAdminNotifications = "admin:notifications"
	APITokenScopeAdminUpdates       = "admin:updates"
	APITokenScopeAdminStatistics    = "admin:statistics"
)

var (
	// ErrAPITokenNotFound indicates that a token ID or credential does not exist for an account.
	ErrAPITokenNotFound = errors.New("API token not found")
	// ErrAPITokenNameExists indicates that an account already uses the requested token label.
	ErrAPITokenNameExists = errors.New("API token name already exists")
	// ErrAPITokenLimit indicates that an account reached its durable token limit.
	ErrAPITokenLimit = errors.New("API token limit reached")
)

// APIToken is non-secret metadata for one fine-grained durable credential.
type APIToken struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	CreatedAt int64    `json:"created_at"`
	ExpiresAt *int64   `json:"expires_at,omitempty"`
}

// APITokenCredential combines token metadata with its owning account during authentication.
type APITokenCredential struct {
	Token   *APIToken
	Account *AccessToken
}

// GenerateAPITokenSecret returns a prefixed 256-bit Base64URL credential.
func GenerateAPITokenSecret() (string, error) {
	value := make([]byte, APITokenSecretBytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "rnp_pat_" + base64.RawURLEncoding.EncodeToString(value), nil
}

// HashAPITokenSecret returns the lowercase SHA-256 lookup digest persisted by the database.
func HashAPITokenSecret(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}
