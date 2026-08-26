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
	"crypto/sha256"
	"sync"
)

const maxTransientAuthStates = 2048

// TransientAuthState binds one short-lived external authentication flow to its initiating account and route.
type TransientAuthState struct {
	Provider  string
	UserID    string
	ReturnTo  string
	ExpiresAt int64
}

// TransientAuthStateStore is a bounded single-use state store for external authentication redirects.
type TransientAuthStateStore struct {
	mu      sync.Mutex
	entries map[[sha256.Size]byte]TransientAuthState
}

// NewTransientAuthStateStore creates an empty bounded state store.
func NewTransientAuthStateStore() *TransientAuthStateStore {
	return &TransientAuthStateStore{entries: make(map[[sha256.Size]byte]TransientAuthState)}
}

// Put stores a state using only its SHA-256 digest and evicts expired or oldest entries when bounded capacity is reached.
func (store *TransientAuthStateStore) Put(raw string, state TransientAuthState, now int64) bool {
	if store == nil || raw == "" || state.Provider == "" || state.ExpiresAt <= now {
		return false
	}
	key := sha256.Sum256([]byte(raw))
	store.mu.Lock()
	defer store.mu.Unlock()
	var oldestKey [sha256.Size]byte
	oldestExpiry := int64(0)
	for candidate, entry := range store.entries {
		if entry.ExpiresAt <= now {
			delete(store.entries, candidate)
			continue
		}
		if oldestExpiry == 0 || entry.ExpiresAt < oldestExpiry {
			oldestKey = candidate
			oldestExpiry = entry.ExpiresAt
		}
	}
	if len(store.entries) >= maxTransientAuthStates {
		delete(store.entries, oldestKey)
	}
	store.entries[key] = state
	return true
}

// Consume removes and returns a matching unexpired state exactly once.
func (store *TransientAuthStateStore) Consume(raw, provider string, now int64) (TransientAuthState, bool) {
	if store == nil || raw == "" || provider == "" {
		return TransientAuthState{}, false
	}
	key := sha256.Sum256([]byte(raw))
	store.mu.Lock()
	state, ok := store.entries[key]
	delete(store.entries, key)
	store.mu.Unlock()
	if !ok || state.Provider != provider || state.ExpiresAt <= now {
		return TransientAuthState{}, false
	}
	return state, true
}
