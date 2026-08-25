/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package middleware

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestIPLimiterCleanupRemovesInactiveEntries(t *testing.T) {
	limiter := NewIPLimiter(rate.Every(time.Second), 1)
	limiter.GetLimiter("192.0.2.1")
	entry, ok := limiter.limiters.Load("192.0.2.1")
	if !ok {
		t.Fatal("limiter entry was not stored")
	}
	entry.lastSeen.Store(1)
	if removed := limiter.cleanup(); removed != 1 {
		t.Fatalf("removed limiters = %d, want 1", removed)
	}
	if got := limiter.count.Load(); got != 0 {
		t.Fatalf("limiter count = %d, want 0", got)
	}
}
