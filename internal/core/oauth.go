/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

var (
	// ErrOAuthIdentityLinked indicates an identity or account already linked to another identity for this provider.
	ErrOAuthIdentityLinked = errors.New("OAuth identity is already linked")
	// ErrOAuthIdentityNotFound indicates that the requested provider binding does not exist.
	ErrOAuthIdentityNotFound = errors.New("OAuth identity was not found")
)

// OAuthIdentity binds a provider's stable subject and configuration authority to an immutable account.
type OAuthIdentity struct {
	ProviderID   string   `json:"provider"`
	Subject      string   `json:"subject"`
	Authority    string   `json:"authority"`
	UserID       string   `json:"-"`
	Username     string   `json:"-"`
	Login        string   `json:"login"`
	Namespaces   []string `json:"namespaces,omitempty"`
	AuthorizedAt int64    `json:"authorized_at"`
}

// Key hashes the case-sensitive identity within its configured authority; display names never identify accounts.
func (i OAuthIdentity) Key() string {
	hash := sha256.Sum256([]byte(i.ProviderID + "\x00" + i.Authority + "\x00" + i.Subject))
	return hex.EncodeToString(hash[:])
}

// Valid reports whether an identity fits the persisted bounds without ambiguous separators.
func (i OAuthIdentity) Valid() bool {
	if i.ProviderID == "" || i.ProviderID == "github" || len(i.ProviderID) > 32 || len(i.Authority) != 64 ||
		i.Subject == "" || len(i.Subject) > 255 || len(i.Login) > 255 || strings.ContainsAny(i.Subject+i.Login, "\x00\r\n") {
		return false
	}
	for _, r := range i.ProviderID {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	if len(i.Namespaces) > 1001 {
		return false
	}
	for _, namespace := range i.Namespaces {
		if namespace == "" || len(namespace) > 63 || strings.ContainsAny(namespace, "\x00\r\n/.") {
			return false
		}
	}
	_, err := hex.DecodeString(i.Authority)
	return err == nil
}
