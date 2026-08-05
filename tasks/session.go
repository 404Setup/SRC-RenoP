/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package tasks

import (
	"time"

	"renop/core"
)

// StartSessionCleaner periodically removes expired sessions from the DB and in-memory cache.
func StartSessionCleaner(state *core.AppState) {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			now := time.Now().UnixMilli()
			if db := state.GetDB(); db != nil {
				_ = db.DeleteExpiredSessions(now - core.SessionIdleTimeoutMillis)
			}

			var toRemove []string
			state.Inner.Sessions.Range(func(key string, value *core.Session) bool {
				if now-value.LastActive.Load() > core.SessionIdleTimeoutMillis {
					toRemove = append(toRemove, key)
				}
				return true
			})

			for _, token := range toRemove {
				state.Inner.Sessions.Delete(token)
				state.DeleteAuthCache("Session " + token)
			}
		}
	}()
}
