/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

package tasks

import (
	"time"

	"renop/internal/core"
)

// PruneAuthCache removes expired authentication cache entries.
func PruneAuthCache(state *core.AppState, now time.Time) int {
	if state == nil || state.Inner == nil {
		return 0
	}
	removed := 0
	nowMillis := now.UnixMilli()
	state.Inner.AuthCache.Range(func(key string, value core.AuthCacheEntry) bool {
		if value.ExpiredAt <= nowMillis {
			state.DeleteAuthCache(key)
			removed++
		}
		return true
	})
	return removed
}
