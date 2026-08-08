/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package audit

import (
	"time"

	"renop/internal/config"
	"renop/internal/core"
)

// StartAuditLogConsumer processes audit log entries asynchronously in a single thread.
func StartAuditLogConsumer(state *core.AppState) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case entry, ok := <-state.Inner.AuditLogChan:
			if !ok {
				return
			}
			saveLog(state, entry)
		case <-ticker.C:
			cleanLogs(state)
		}
	}
}

func saveLog(state *core.AppState, entry *core.AuditLogEntry) {
	if state == nil || state.Inner == nil || entry == nil {
		return
	}
	if db := state.GetDB(); db != nil {
		_ = db.SaveAuditLog(entry)
	}
}

func cleanLogs(state *core.AppState) {
	if state == nil || state.Inner == nil {
		return
	}
	cfgVal := state.Inner.Config.Load()
	if cfgVal == nil {
		return
	}
	cfg := cfgVal.(*config.Config)

	if db := state.GetDB(); db != nil {
		_ = db.CleanExpiredAuditLogs(cfg.AuditLog.RetentionDays, cfg.AuditLog.MaxRows)
	}
}
