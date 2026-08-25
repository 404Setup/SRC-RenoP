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

import "testing"

func TestAnomalyFailureStoreIncrementAndExpire(t *testing.T) {
	s := &AnomalyFailureStore{entries: make(map[string]anomalyFailure)}

	if got := s.Count("1.2.3.4"); got != 0 {
		t.Fatalf("empty count = %d", got)
	}
	if got := s.Increment("1.2.3.4"); got != 1 {
		t.Fatalf("first increment = %d", got)
	}
	if got := s.Increment("1.2.3.4"); got != 2 {
		t.Fatalf("second increment = %d", got)
	}
	if got := s.Count("1.2.3.4"); got != 2 {
		t.Fatalf("count after increments = %d", got)
	}

	s.mu.Lock()
	e := s.entries["1.2.3.4"]
	e.ExpiresAt = 1
	s.entries["1.2.3.4"] = e
	s.mu.Unlock()

	if got := s.Count("1.2.3.4"); got != 0 {
		t.Fatalf("expired count = %d", got)
	}
	if got := s.Increment("1.2.3.4"); got != 1 {
		t.Fatalf("increment after expiry = %d", got)
	}
}

func TestAnomalyFailureStoreHardCap(t *testing.T) {
	s := &AnomalyFailureStore{entries: make(map[string]anomalyFailure)}
	s.mu.Lock()
	for i := range maxAnomalyFailureEntries {
		s.entries[itoa(i)] = anomalyFailure{Count: 1, ExpiresAt: 1 << 62}
	}
	s.mu.Unlock()

	_ = s.Increment("new-ip")
	s.mu.Lock()
	n := len(s.entries)
	s.mu.Unlock()
	if n > maxAnomalyFailureEntries {
		t.Fatalf("entries after hard-cap insert = %d, want <= %d", n, maxAnomalyFailureEntries)
	}
}

func TestAnomalyFailureStorePruneExpired(t *testing.T) {
	store := &AnomalyFailureStore{entries: map[string]anomalyFailure{
		"expired": {Count: 1, ExpiresAt: 1},
		"live":    {Count: 2, ExpiresAt: 1 << 62},
	}}
	if removed := store.PruneExpired(); removed != 1 {
		t.Fatalf("removed entries = %d, want 1", removed)
	}
	if got := store.Count("live"); got != 2 {
		t.Fatalf("live count = %d, want 2", got)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
