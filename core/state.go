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
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	VssMemory   uint64 `json:"vss_memory"`
	UsedThreads uint64 `json:"used_threads"`
	OpenFiles   uint64 `json:"open_files"`
}

type StateDB interface {
	GetTokenByName(name string) (*AccessToken, error)
	GetTokenBySecret(secret string) (*AccessToken, error)
	GetAllTokens() ([]*AccessToken, error)
	SaveToken(token *AccessToken) error
	DeleteToken(name string) error
	RenameToken(oldName, newName string, token *AccessToken) error
	GetSession(sessionToken string) (*Session, error)
	SaveSession(session *Session, sessionToken string) error
	UpdateSessionLastActive(sessionToken string, lastActive int64) error
	DeleteSession(sessionToken string) error
	DeleteSessionsByUsername(username string) error
	ListUserSessions(username, currentSessionToken string) ([]SessionDto, error)
	DeleteExpiredSessions(minActiveTimestamp int64) error
	DeleteUserSessionByPublicID(username, publicID, currentSessionToken string) (token string, revoked bool, wasCurrent bool, err error)
	DeleteOtherUserSessions(username, keepSessionToken string) (tokens []string, err error)
	GetActiveSessions(minActiveTimestamp int64) ([]SessionDbDto, error)
	UpdateSessionsUsername(oldUsername, newUsername string) error
	ListFidoDevices(username string) ([]*FidoDevice, error)
	GetFidoDeviceByCredentialID(credentialID []byte) (*FidoDevice, error)
	SaveFidoDevice(device *FidoDevice) error
	UpdateFidoSignCount(credentialID []byte, signCount uint32) error
	UpdateFidoDeviceState(credentialID []byte, signCount uint32, backupState bool, backupEligible bool) error
	DeleteFidoDevice(username, deviceID string) error
	DeleteFidoDevicesByUsername(username string) error
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
	FidoDevices        pb.MapOf[string, []*FidoDevice]
	FidoWriteLock      sync.Mutex
	DB                 any

	// SessionsFlush, when set, schedules an immediate persist of the session store.
	// Used after logout/revocation so deleted sessions cannot reappear after restart.
	SessionsFlush func()

	FileIndex              *index.FileIndex
	IndexWatcher           *fsnotify.Watcher
	IndexWatcherMutex      sync.Mutex
	StartTime              int64
	FileCache              *FileByteCache
	MetadataCache          pb.MapOf[string, *config.Metadata]
	MetadataCacheEntries   atomic.Uint64
	MetadataCacheWriteLock sync.Mutex
	InFlightDownloads      *InFlightManager
	AnomalyFailures        *AnomalyFailureStore
	ProxyClientSemaphore   chan struct{}
}

type AppState struct {
	Inner *AppStateInner
}

func NewAppState() *AppState {
	return &AppState{
		Inner: &AppStateInner{
			Config:               &atomic.Value{},
			ProxyClientSemaphore: make(chan struct{}, 256),
			StartTime:            time.Now().UnixMilli(),
			InFlightDownloads:    NewInFlightManager(),
			AnomalyFailures:      NewAnomalyFailureStore(),
		},
	}
}

func (state *AppState) GetDB() StateDB {
	if state == nil || state.Inner == nil || state.Inner.DB == nil {
		return nil
	}
	if sdb, ok := state.Inner.DB.(StateDB); ok {
		return sdb
	}
	return nil
}

func (state *AppState) GetTokenByName(name string) *AccessToken {
	if state == nil || state.Inner == nil || name == "" {
		return nil
	}
	if db := state.GetDB(); db != nil {
		tok, err := db.GetTokenByName(name)
		if err == nil {
			return tok
		}
	}
	tok, _ := state.Inner.TokenRepository.Load(strings.ToLower(name))
	return tok
}

func (state *AppState) GetTokenBySecret(secret string) *AccessToken {
	if state == nil || state.Inner == nil || secret == "" {
		return nil
	}
	if db := state.GetDB(); db != nil {
		tok, err := db.GetTokenBySecret(secret)
		if err == nil {
			return tok
		}
	}
	tok, _ := state.Inner.TokenIndex.Load(secret)
	return tok
}

func (state *AppState) GetAllTokens() []*AccessToken {
	if state == nil || state.Inner == nil {
		return []*AccessToken{}
	}
	if db := state.GetDB(); db != nil {
		toks, err := db.GetAllTokens()
		if err == nil && toks != nil {
			return toks
		}
		return []*AccessToken{}
	}
	var tokens []*AccessToken
	state.Inner.TokenRepository.Range(func(key string, value *AccessToken) bool {
		tokens = append(tokens, value)
		return true
	})
	if tokens == nil {
		return []*AccessToken{}
	}
	return tokens
}

func (state *AppState) GetSession(sessionToken string) *Session {
	if state == nil || state.Inner == nil || sessionToken == "" {
		return nil
	}
	if db := state.GetDB(); db != nil {
		sess, err := db.GetSession(sessionToken)
		if err == nil && sess != nil {
			return sess
		}
	}
	sess, _ := state.Inner.Sessions.Load(sessionToken)
	return sess
}

func (state *AppState) SaveSession(session *Session, sessionToken string) {
	if state == nil || state.Inner == nil || session == nil || sessionToken == "" {
		return
	}
	state.Inner.Sessions.Store(sessionToken, session)
	if db := state.GetDB(); db != nil {
		_ = db.SaveSession(session, sessionToken)
	}
	state.MarkSessionsDirty()
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
	state.DeleteAuthCache("Session " + sessionToken)
	state.Inner.Sessions.Delete(sessionToken)
	if db := state.GetDB(); db != nil {
		_ = db.DeleteSession(sessionToken)
	}
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
	if db := state.GetDB(); db != nil {
		sessions, err := db.ListUserSessions(username, currentSessionToken)
		if err == nil && sessions != nil {
			return sessions
		}
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
	if db := state.GetDB(); db != nil {
		token, revoked, wasCurrent, err := db.DeleteUserSessionByPublicID(username, publicID, currentSessionToken)
		if err == nil {
			if revoked && token != "" {
				state.DeleteAuthCache("Session " + token)
				state.Inner.Sessions.Delete(token)
				state.MarkSessionsDirty()
			}
			return revoked, wasCurrent
		}
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
	if db := state.GetDB(); db != nil {
		deletedTokens, err := db.DeleteOtherUserSessions(username, keepSessionToken)
		if err == nil {
			for _, t := range deletedTokens {
				state.DeleteAuthCache("Session " + t)
				state.Inner.Sessions.Delete(t)
			}
			state.MarkSessionsDirty()
			return len(deletedTokens)
		}
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

func (state *AppState) ListFidoDevices(username string) []*FidoDevice {
	if state == nil || state.Inner == nil || username == "" {
		return []*FidoDevice{}
	}
	lowerName := strings.ToLower(username)
	if db := state.GetDB(); db != nil {
		devs, err := db.ListFidoDevices(lowerName)
		if err == nil && devs != nil {
			return devs
		}
		return []*FidoDevice{}
	}
	devs, _ := state.Inner.FidoDevices.Load(lowerName)
	if devs == nil {
		return []*FidoDevice{}
	}
	return devs
}

func (state *AppState) GetFidoDeviceByCredentialID(credentialID []byte) *FidoDevice {
	if state == nil || state.Inner == nil || len(credentialID) == 0 {
		return nil
	}
	if db := state.GetDB(); db != nil {
		dev, err := db.GetFidoDeviceByCredentialID(credentialID)
		if err == nil && dev != nil {
			return dev
		}
	}
	var matched *FidoDevice
	state.Inner.FidoDevices.Range(func(key string, devices []*FidoDevice) bool {
		for _, d := range devices {
			if string(d.CredentialID) == string(credentialID) {
				matched = d
				return false
			}
		}
		return true
	})
	return matched
}

func (state *AppState) SaveFidoDevice(device *FidoDevice) {
	if state == nil || state.Inner == nil || device == nil || device.Username == "" {
		return
	}
	lowerName := strings.ToLower(device.Username)
	device.Username = lowerName
	if db := state.GetDB(); db != nil {
		_ = db.SaveFidoDevice(device)
		return
	}
	state.Inner.FidoWriteLock.Lock()
	defer state.Inner.FidoWriteLock.Unlock()
	devs, _ := state.Inner.FidoDevices.Load(lowerName)
	newDevs := append([]*FidoDevice{}, devs...)
	newDevs = append(newDevs, device)
	state.Inner.FidoDevices.Store(lowerName, newDevs)
}

func (state *AppState) DeleteFidoDevice(username, deviceID string) bool {
	if state == nil || state.Inner == nil || username == "" || deviceID == "" {
		return false
	}
	lowerName := strings.ToLower(username)
	if db := state.GetDB(); db != nil {
		err := db.DeleteFidoDevice(lowerName, deviceID)
		return err == nil
	}
	state.Inner.FidoWriteLock.Lock()
	defer state.Inner.FidoWriteLock.Unlock()
	devs, _ := state.Inner.FidoDevices.Load(lowerName)
	if devs == nil {
		return false
	}
	var updated []*FidoDevice
	found := false
	for _, d := range devs {
		if d.ID == deviceID {
			found = true
		} else {
			updated = append(updated, d)
		}
	}
	if found {
		state.Inner.FidoDevices.Store(lowerName, updated)
	}
	return found
}

func (state *AppState) DeleteFidoDevicesByUsername(username string) {
	if state == nil || state.Inner == nil || username == "" {
		return
	}
	lowerName := strings.ToLower(username)
	if db := state.GetDB(); db != nil {
		_ = db.DeleteFidoDevicesByUsername(lowerName)
		return
	}
	state.Inner.FidoWriteLock.Lock()
	defer state.Inner.FidoWriteLock.Unlock()
	state.Inner.FidoDevices.Delete(lowerName)
}

func (state *AppState) UpdateFidoSignCount(credentialID []byte, signCount uint32) {
	if state == nil || state.Inner == nil || len(credentialID) == 0 {
		return
	}
	if db := state.GetDB(); db != nil {
		_ = db.UpdateFidoSignCount(credentialID, signCount)
	}
	state.Inner.FidoWriteLock.Lock()
	defer state.Inner.FidoWriteLock.Unlock()
	state.Inner.FidoDevices.Range(func(key string, devices []*FidoDevice) bool {
		for _, d := range devices {
			if string(d.CredentialID) == string(credentialID) {
				d.SignCount = signCount
				return false
			}
		}
		return true
	})
}

func (state *AppState) UpdateFidoDeviceState(credentialID []byte, signCount uint32, backupState bool, backupEligible bool) {
	if state == nil || state.Inner == nil || len(credentialID) == 0 {
		return
	}
	if db := state.GetDB(); db != nil {
		_ = db.UpdateFidoDeviceState(credentialID, signCount, backupState, backupEligible)
	}
	state.Inner.FidoWriteLock.Lock()
	defer state.Inner.FidoWriteLock.Unlock()
	state.Inner.FidoDevices.Range(func(key string, devices []*FidoDevice) bool {
		for _, d := range devices {
			if string(d.CredentialID) == string(credentialID) {
				d.SignCount = signCount
				d.BackupState = backupState
				d.BackupEligible = backupEligible
				return false
			}
		}
		return true
	})
}
