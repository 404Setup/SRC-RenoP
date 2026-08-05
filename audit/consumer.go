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

	"renop/config"
	"renop/core"
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
		return
	}

	state.Inner.AuditLogLock.Lock()
	defer state.Inner.AuditLogLock.Unlock()

	cfgVal := state.Inner.Config.Load()
	maxRows := 10000
	if cfgVal != nil {
		if cfg, ok := cfgVal.(*config.Config); ok && cfg.AuditLog.MaxRows > 0 {
			maxRows = cfg.AuditLog.MaxRows
		}
	}

	entry.ID = int64(len(state.Inner.AuditLogsMem) + 1)
	state.Inner.AuditLogsMem = append(state.Inner.AuditLogsMem, entry)

	if len(state.Inner.AuditLogsMem) > maxRows {
		state.Inner.AuditLogsMem = state.Inner.AuditLogsMem[len(state.Inner.AuditLogsMem)-maxRows:]
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
		return
	}

	state.Inner.AuditLogLock.Lock()
	defer state.Inner.AuditLogLock.Unlock()

	if cfg.AuditLog.RetentionDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -cfg.AuditLog.RetentionDays).UnixMilli()
		filtered := make([]*core.AuditLogEntry, 0, len(state.Inner.AuditLogsMem))
		for _, e := range state.Inner.AuditLogsMem {
			if e.CreatedAt >= cutoff {
				filtered = append(filtered, e)
			}
		}
		state.Inner.AuditLogsMem = filtered
	}

	if cfg.AuditLog.MaxRows > 0 && len(state.Inner.AuditLogsMem) > cfg.AuditLog.MaxRows {
		state.Inner.AuditLogsMem = state.Inner.AuditLogsMem[len(state.Inner.AuditLogsMem)-cfg.AuditLog.MaxRows:]
	}
}
