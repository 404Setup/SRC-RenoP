/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import "errors"

var (
	// ErrRegistrationDisabled indicates that public account creation is closed.
	ErrRegistrationDisabled = errors.New("registration is disabled")
	// ErrRegistrationRateLimited indicates that the IP has consumed its account allowance.
	ErrRegistrationRateLimited = errors.New("registration IP limit reached")
	// ErrRegistrationInvalid indicates a missing, expired, or rejected confirmation.
	ErrRegistrationInvalid = errors.New("registration confirmation is invalid")
	// ErrRegistrationCooldown indicates an unconfirmed provider registration still in its cooldown.
	ErrRegistrationCooldown = errors.New("provider registration is cooling down")
	// ErrRegistrationPending indicates that the provider already has an active confirmation.
	ErrRegistrationPending = errors.New("provider registration is already pending")
)

// PendingRegistration stores a bounded, unprivileged confirmation, never a usable account or password.
type PendingRegistration struct {
	IDHash        string
	IPHash        string
	ProviderKey   string
	Email         string
	CodeHash      string
	ProfileJSON   string
	Attempts      int
	CreatedAt     int64
	ExpiresAt     int64
	CooldownUntil int64
}

// RegistrationProfile retains provider suggestions and an optional encrypted, short-lived avatar capability.
type RegistrationProfile struct {
	OAuth              *OAuthIdentity    `json:"oauth,omitempty"`
	EmailVerified      bool              `json:"email_verified,omitempty"`
	AvatarURL          string            `json:"avatar_url,omitempty"`
	AvatarToken        string            `json:"avatar_token,omitempty"`
	ProviderConfigHash string            `json:"provider_config_hash,omitempty"`
	Username           string            `json:"username"`
	Nickname           string            `json:"nickname"`
	GitHubID           int64             `json:"github_id,omitempty"`
	GitHubLogin        string            `json:"github_login,omitempty"`
	Principals         []GitHubPrincipal `json:"principals,omitempty"`
}

// AccountRegistration carries validated account fields and the browser's confirmation capability.
type AccountRegistration struct {
	Provider     string
	Username     string
	Nickname     string
	Email        string
	PasswordHash string
	IDHash       string
	CodeHash     string
	IPHash       string
	RequireEmail bool
}
