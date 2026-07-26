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
	"sync"
	"sync/atomic"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/fsnotify/fsnotify"
	"github.com/llxisdsh/pb"

	"renop/config"
	"renop/index"
)

type AuthCacheEntry struct {
	User      *config.User
	ExpiredAt int64
}

type CachedFile struct {
	Bytes        []byte
	Etag         *string
	LastModified *string
	ContentType  string
}

type StatusSnapshot struct {
	Timestamp   int64  `json:"timestamp"`
	UsedMemory  uint64 `json:"used_memory"`
	UsedThreads uint64 `json:"used_threads"`
	OpenFiles   uint64 `json:"open_files"`
}

type AppStateInner struct {
	Config             *atomic.Value
	ConfigWriteLock    sync.Mutex
	TokenRepository    pb.MapOf[string, *AccessToken]
	TokenIndex         pb.MapOf[string, *AccessToken]
	TokensCount        atomic.Uint64
	TokenWriteLock     sync.Mutex
	StatusSnapshots    atomic.Pointer[[]StatusSnapshot]
	ActiveRequests     atomic.Uint64
	FailuresCount      atomic.Uint64
	AuthCache          pb.MapOf[string, AuthCacheEntry]
	AuthCacheEntries   atomic.Uint64
	AuthCacheWriteLock sync.Mutex
	Sessions           pb.MapOf[string, *Session]
	SessionsIsDirty    atomic.Bool
	// SessionsFlush, when set, schedules an immediate persist of the session store.
	// Used after logout/revocation so deleted sessions cannot reappear after restart.
	SessionsFlush          func()
	FileIndex              *index.FileIndex
	IndexWatcher           *fsnotify.Watcher
	IndexWatcherMutex      sync.Mutex
	StartTime              int64
	FileCache              *bigcache.BigCache
	MetadataCache          pb.MapOf[string, *config.Metadata]
	MetadataCacheEntries   atomic.Uint64
	MetadataCacheWriteLock sync.Mutex
	InFlightDownloads      *InFlightManager
	AnomalyRequests        *bigcache.BigCache
	AnomalyFailures        *bigcache.BigCache
	ProxyClientSemaphore   chan struct{}
}

type AppState struct {
	Inner *AppStateInner
}

func NewAppState() *AppState {
	return &AppState{
		Inner: &AppStateInner{
			Config:               &atomic.Value{},
			ProxyClientSemaphore: make(chan struct{}, 1000),
			StartTime:            time.Now().UnixMilli(),
			InFlightDownloads:    NewInFlightManager(),
		},
	}
}

// MarkSessionsDirty marks the session map for persistence and optionally flushes immediately.
func (state *AppState) MarkSessionsDirty() {
	if state == nil || state.Inner == nil {
		return
	}
	state.Inner.SessionsIsDirty.Store(true)
	if flush := state.Inner.SessionsFlush; flush != nil {
		flush()
	}
}

// RevokeSession removes a browser session by its secret token and invalidates related auth cache.
// Returns true if a session was present and removed.
func (state *AppState) RevokeSession(sessionToken string) bool {
	if state == nil || state.Inner == nil || sessionToken == "" {
		return false
	}
	if _, loaded := state.Inner.Sessions.LoadAndDelete(sessionToken); !loaded {
		return false
	}
	state.DeleteAuthCache("Session " + sessionToken)
	state.MarkSessionsDirty()
	return true
}

// sessionToDto maps an in-memory Session (+ map key) to a public SessionDto.
// Session secret (map key) is never included.
func sessionToDto(secretToken string, session *Session, currentSessionToken string) SessionDto {
	lastActive := session.LastActive.Load()
	return SessionDto{
		PublicId:   session.PublicId,
		Username:   session.Username,
		Ip:         session.Ip,
		UserAgent:  session.UserAgent,
		CreatedAt:  session.CreatedAt,
		LastActive: lastActive,
		ExpiresAt:  lastActive + SessionIdleTimeoutMillis,
		Current:    secretToken != "" && secretToken == currentSessionToken,
	}
}

// ListUserSessions returns browser sessions for username (Basic/Bearer are not sessions).
// currentSessionToken is the secret token of the request's session, if any.
func (state *AppState) ListUserSessions(username, currentSessionToken string) []SessionDto {
	if state == nil || state.Inner == nil || username == "" {
		return []SessionDto{}
	}
	var sessions []SessionDto
	state.Inner.Sessions.Range(func(key string, value *Session) bool {
		if value != nil && value.Username == username {
			sessions = append(sessions, sessionToDto(key, value, currentSessionToken))
		}
		return true
	})
	if sessions == nil {
		return []SessionDto{}
	}
	return sessions
}

// RevokeUserSessionByPublicID removes one session owned by username, identified by public_id.
// Returns (revoked, wasCurrent) where wasCurrent means the revoked secret matched currentSessionToken.
func (state *AppState) RevokeUserSessionByPublicID(username, publicID, currentSessionToken string) (revoked bool, wasCurrent bool) {
	if state == nil || state.Inner == nil || username == "" || publicID == "" {
		return false, false
	}
	var toRemove string
	state.Inner.Sessions.Range(func(key string, value *Session) bool {
		if value != nil && value.Username == username && value.PublicId == publicID {
			toRemove = key
			return false
		}
		return true
	})
	if toRemove == "" {
		return false, false
	}
	wasCurrent = currentSessionToken != "" && toRemove == currentSessionToken
	return state.RevokeSession(toRemove), wasCurrent
}

// RevokeOtherUserSessions removes every session for username except the one matching keepSessionToken.
// If keepSessionToken is empty, all sessions for the user are removed.
func (state *AppState) RevokeOtherUserSessions(username, keepSessionToken string) int {
	if state == nil || state.Inner == nil || username == "" {
		return 0
	}
	var toRemove []string
	state.Inner.Sessions.Range(func(key string, value *Session) bool {
		if value != nil && value.Username == username && key != keepSessionToken {
			toRemove = append(toRemove, key)
		}
		return true
	})
	count := 0
	for _, token := range toRemove {
		if state.RevokeSession(token) {
			count++
		}
	}
	return count
}

// RevokeAllUserSessions removes every browser session for username.
func (state *AppState) RevokeAllUserSessions(username string) int {
	return state.RevokeOtherUserSessions(username, "")
}
