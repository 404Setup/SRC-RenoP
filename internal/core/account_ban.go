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
	"unicode"
	"unicode/utf8"
)

const MaxAccountBanReasonRunes = 512

var (
	ErrAccountBanned     = errors.New("account is banned")
	ErrAccountBanInvalid = errors.New("account ban is invalid")
)

// AccountBan is a durable administrator suspension. A nil ExpiresAt is permanent.
type AccountBan struct {
	Reason    string `json:"reason" yaml:"reason"`
	CreatedAt int64  `json:"created_at" yaml:"created_at"`
	ExpiresAt *int64 `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

// IsActive reports whether the suspension applies at now.
func (ban *AccountBan) IsActive(now int64) bool {
	return ban != nil && ban.CreatedAt > 0 && (ban.ExpiresAt == nil || now < *ban.ExpiresAt)
}

// Clone returns an independent ban value.
func (ban *AccountBan) Clone() *AccountBan {
	if ban == nil {
		return nil
	}
	cloned := *ban
	if ban.ExpiresAt != nil {
		expiresAt := *ban.ExpiresAt
		cloned.ExpiresAt = &expiresAt
	}
	return &cloned
}

// NormalizeAccountBanReason trims and validates one administrator-visible reason.
func NormalizeAccountBanReason(reason string) (string, bool) {
	reason = strings.TrimSpace(reason)
	if reason == "" || !utf8.ValidString(reason) || utf8.RuneCountInString(reason) > MaxAccountBanReasonRunes {
		return "", false
	}
	for _, character := range reason {
		if unicode.IsControl(character) {
			return "", false
		}
	}
	return reason, true
}
