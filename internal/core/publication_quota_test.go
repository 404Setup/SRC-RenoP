/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package core

import (
	"testing"
	"time"
)

func TestPublicationQuotaWindowUsesStableUTCBoundaries(t *testing.T) {
	now := time.Date(2026, time.August, 30, 17, 30, 0, 0, time.FixedZone("test", 8*60*60))
	tests := []struct {
		period string
		start  string
		end    string
	}{
		{PublicationQuotaPeriodDay, "2026-08-30T00:00:00Z", "2026-08-31T00:00:00Z"},
		{PublicationQuotaPeriodWeek, "2026-08-24T00:00:00Z", "2026-08-31T00:00:00Z"},
		{PublicationQuotaPeriodMonth, "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z"},
	}
	for _, test := range tests {
		start, end, valid := PublicationQuotaWindow(test.period, now)
		if !valid || start.Format(time.RFC3339) != test.start || end.Format(time.RFC3339) != test.end {
			t.Errorf("PublicationQuotaWindow(%q) = %s..%s, %t", test.period,
				start.Format(time.RFC3339), end.Format(time.RFC3339), valid)
		}
	}
	if _, _, valid := PublicationQuotaWindow("year", now); valid {
		t.Fatal("unsupported publication quota period was accepted")
	}
}
