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
