/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import (
	"errors"
	"strings"

	"golang.org/x/net/idna"
)

const (
	// RecoveryCodeCount is the number of one-time recovery codes issued as one set.
	RecoveryCodeCount = 12
	// RecoveryCodesRequired is the number of distinct unused codes required for password recovery.
	RecoveryCodesRequired = 4
	// MaxEmailLength follows the practical RFC mailbox length limit.
	MaxEmailLength = 254
)

var (
	// ErrEmailAlreadyExists indicates that a private login email belongs to another account.
	ErrEmailAlreadyExists = errors.New("email address is already in use")
	// ErrLastLoginMethod indicates that an operation would remove the account's final usable login method.
	ErrLastLoginMethod = errors.New("account must retain another login method")
	// ErrPasswordNotConfigured indicates that password login cannot be enabled without a password hash.
	ErrPasswordNotConfigured = errors.New("account password is not configured")
	// ErrRecoveryCodesInvalid indicates that password recovery did not provide four valid unused codes.
	ErrRecoveryCodesInvalid = errors.New("recovery codes are invalid")
)

// AccountSecurity is the private authentication state visible only to its account owner.
type AccountSecurity struct {
	Email                   string `json:"email"`
	RecoveryGeneratedAt     int64  `json:"recovery_generated_at,omitempty"`
	RecoveryCodeCount       int    `json:"recovery_code_count"`
	RecoveryCodesRemaining  int    `json:"recovery_codes_remaining"`
	FidoDeviceCount         int    `json:"fido_device_count"`
	PasswordLoginEnabled    bool   `json:"password_login_enabled"`
	PasswordConfigured      bool   `json:"password_configured"`
	GitHubLinked            bool   `json:"github_linked"`
	CanDisablePasswordLogin bool   `json:"can_disable_password_login"`
}

// RecoveryCodeHash is one irreversible recovery-code verifier prepared for persistence.
type RecoveryCodeHash struct {
	SelectorHash string
	PasswordHash string
	CreatedAt    int64
}

// RecoveryCodeRecord is one stored recovery verifier returned for constant-time validation.
type RecoveryCodeRecord struct {
	SelectorHash string
	PasswordHash string
}

// NormalizeEmail validates and canonicalizes a private account email.
func NormalizeEmail(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", true
	}
	if len(value) > MaxEmailLength || strings.Count(value, "@") != 1 {
		return "", false
	}
	local, domain, found := strings.Cut(value, "@")
	if !found || len(local) == 0 || len(local) > 64 || domain == "" {
		return "", false
	}
	if local[0] == '.' || local[len(local)-1] == '.' || strings.Contains(local, "..") {
		return "", false
	}
	for index := 0; index < len(local); index++ {
		character := local[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("!#$%&'*+-/=?^_\x60{|}~.", rune(character)) {
			continue
		}
		return "", false
	}
	asciiDomain, err := idna.Lookup.ToASCII(strings.TrimSuffix(strings.ToLower(domain), "."))
	if err != nil || asciiDomain == "" || len(asciiDomain) > 253 {
		return "", false
	}
	for _, label := range strings.Split(asciiDomain, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for index := 0; index < len(label); index++ {
			character := label[index]
			if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
				continue
			}
			return "", false
		}
	}
	normalized := strings.ToLower(local) + "@" + asciiDomain
	return normalized, len(normalized) <= MaxEmailLength
}
