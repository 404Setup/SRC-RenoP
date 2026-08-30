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
	"strconv"
	"testing"
)

func TestTransientAuthStateIsBoundedAndSingleUse(t *testing.T) {
	store := NewTransientAuthStateStore()
	const now = int64(1_000)
	requireState := func(raw string, state TransientAuthState) {
		t.Helper()
		if !store.Put(raw, state, now) {
			t.Fatalf("failed to store state %q", raw)
		}
	}
	requireState("single", TransientAuthState{Provider: "github", ReturnTo: "/", ExpiresAt: now + 100})
	if _, ok := store.Consume("single", "gitlab", now); ok {
		t.Fatal("state was accepted by the wrong provider")
	}
	if _, ok := store.Consume("single", "github", now); ok {
		t.Fatal("provider mismatch did not consume the state")
	}
	requireState("expired", TransientAuthState{Provider: "github", ExpiresAt: now + 1})
	if _, ok := store.Consume("expired", "github", now+1); ok {
		t.Fatal("expired state was accepted")
	}
	for index := range maxTransientAuthStates + 20 {
		requireState("state-"+strconv.Itoa(index), TransientAuthState{
			Provider: "github", ExpiresAt: now + int64(index) + 1_000,
		})
	}
	if len(store.entries) != maxTransientAuthStates {
		t.Fatalf("state store size = %d, want %d", len(store.entries), maxTransientAuthStates)
	}
	if _, ok := store.Consume("state-0", "github", now); ok {
		t.Fatal("oldest state was not evicted")
	}
	if _, ok := store.Consume("state-2067", "github", now); !ok {
		t.Fatal("newest state was unexpectedly evicted")
	}
	if _, ok := store.Consume("state-2067", "github", now); ok {
		t.Fatal("state was accepted more than once")
	}
}
