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
	"os"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"renop/core"
	"renop/pb"
	"renop/utils"
)

func StartSessionSaver(state *core.AppState, path string) {
	var persistMu sync.Mutex
	flushCh := make(chan struct{}, 1)

	persist := func() {
		persistMu.Lock()
		defer persistMu.Unlock()

		if !state.Inner.SessionsIsDirty.Swap(false) {
			return
		}

		var dtos []core.SessionDbDto
		state.Inner.Sessions.Range(func(key string, value *core.Session) bool {
			token := key
			session := value
			dtos = append(dtos, core.SessionDbDto{
				PublicId:     session.PublicId,
				SessionToken: token,
				Username:     session.Username,
				Ip:           session.Ip,
				UserAgent:    session.UserAgent,
				CreatedAt:    session.CreatedAt,
				LastActive:   session.LastActive.Load(),
			})
			return true
		})

		store := pb.FromSessionDbDtos(dtos)
		bin, err := proto.Marshal(store)
		if err != nil {
			state.Inner.SessionsIsDirty.Store(true)
			return
		}

		tmpPath := path + ".tmp"
		if writeErr := os.WriteFile(tmpPath, bin, 0644); writeErr != nil {
			state.Inner.SessionsIsDirty.Store(true)
			return
		}
		if renameErr := utils.SafeRename(tmpPath, path); renameErr != nil {
			_ = os.Remove(tmpPath)
			state.Inner.SessionsIsDirty.Store(true)
		}
	}

	requestFlush := func() {
		select {
		case flushCh <- struct{}{}:
		default:
		}
	}
	state.Inner.SessionsFlush = requestFlush

	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
			case <-flushCh:
			}

			now := time.Now().UnixMilli()
			if db := state.GetDB(); db != nil {
				_ = db.DeleteExpiredSessions(now - core.SessionIdleTimeoutMillis)
			}

			var toRemove []string
			state.Inner.Sessions.Range(func(key string, value *core.Session) bool {
				token := key
				session := value
				if now-session.LastActive.Load() > core.SessionIdleTimeoutMillis {
					toRemove = append(toRemove, token)
				}
				return true
			})

			if len(toRemove) > 0 {
				for _, token := range toRemove {
					state.Inner.Sessions.Delete(token)
					state.DeleteAuthCache("Session " + token)
				}
				state.Inner.SessionsIsDirty.Store(true)
			}

			persist()
		}
	}()
}
