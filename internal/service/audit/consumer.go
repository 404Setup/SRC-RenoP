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
	"log"
	"sync"
	"time"

	"renop/internal/core"
)

// StartAuditLogConsumer captures process logs and returns a stop function that drains pending records.
func StartAuditLogConsumer(state *core.AppState) func() {
	if state == nil || state.Inner == nil {
		return func() {}
	}
	previous := log.Writer()
	capture := &systemLogWriter{output: previous, state: state}
	log.SetOutput(capture)
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case entry, open := <-state.Inner.AuditLogChan:
				if !open {
					return
				}
				persistAuditEntry(state, entry, 3)
			case <-stop:
				for {
					select {
					case entry, open := <-state.Inner.AuditLogChan:
						if !open {
							return
						}
						persistAuditEntry(state, entry, 1)
					default:
						return
					}
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			if log.Writer() == capture {
				log.SetOutput(previous)
			}
			close(stop)
			<-done
		})
	}
}

func saveLog(state *core.AppState, entry *core.AuditLogEntry) error {
	if state == nil || state.Inner == nil || entry == nil {
		return nil
	}
	if db := state.GetDB(); db != nil {
		return db.SaveAuditLog(entry)
	}
	return core.ErrDatabaseUnavailable
}

func persistAuditEntry(state *core.AppState, entry *core.AuditLogEntry, attempts int) {
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
		}
		if err = saveLog(state, entry); err == nil {
			return
		}
	}
	if state != nil && state.Inner != nil {
		state.Inner.FailuresCount.Add(1)
	}
	persistenceDiagnostic("Failed to persist audit log action %q for user %q: %v", entry.Action, entry.Username, err)
}

// CleanExpiredLogs enforces the configured audit retention and row limits once.
func CleanExpiredLogs(state *core.AppState) {
	if state == nil || state.Inner == nil {
		return
	}
	cfgVal := state.Inner.Config.Load()
	if cfgVal == nil {
		return
	}

	if db := state.GetDB(); db != nil {
		if err := db.CleanExpiredAuditLogs(cfgVal.AuditLog.RetentionDays, cfgVal.AuditLog.MaxRows); err != nil {
			state.Inner.FailuresCount.Add(1)
			log.Printf("Failed to clean expired audit logs: %v", err)
		}
	}
}
