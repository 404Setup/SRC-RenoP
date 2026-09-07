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
	"path/filepath"
	"strings"
	"time"

	"renop/internal/cache"
	"renop/internal/config"
)

// UseRemoteCache offloads serialized values while keeping revocation indexes in this process.
func (state *AppState) UseRemoteCache(remote *cache.Remote) {
	state.Inner.RemoteCache = remote
	state.Inner.AuthCache.Bind(remote, func(entry AuthCacheEntry) AuthCacheEntry {
		if entry.User != nil {
			entry.User = &config.User{Username: entry.User.Username}
		}
		entry.Scopes, entry.Targets = nil, nil
		return entry
	})
	state.Inner.MetadataCache.Bind(remote, nil)
	if state.Inner.FileCache != nil {
		state.Inner.FileCache.UseRemote(remote)
	}
}

func (state *AppState) InvalidateFileCache(pathStr string) {
	pathStr = filepath.ToSlash(pathStr)
	if state.Inner.FileCache != nil {
		state.Inner.FileCache.Delete(pathStr)
	}
	if strings.HasSuffix(strings.ToLower(pathStr), "maven-metadata.xml") {
		state.DeleteMetadataCache(pathStr)
	}
}

const maxAuthCacheEntries = 10000
const maxMetadataCacheEntries = 512

func (state *AppState) StoreAuthCache(key string, entry AuthCacheEntry) {
	state.StoreAuthCacheIfCurrent(key, entry, state.Inner.AuthCacheGeneration.Load())
}

// StoreAuthCacheIfCurrent discards authentication results computed before a credential invalidation.
func (state *AppState) StoreAuthCacheIfCurrent(key string, entry AuthCacheEntry, generation uint64) {
	state.Inner.AuthCacheWriteLock.Lock()
	defer state.Inner.AuthCacheWriteLock.Unlock()
	if generation != state.Inner.AuthCacheGeneration.Load() {
		return
	}

	if state.Inner.AuthCache.Contains(key) {
		state.Inner.AuthCache.StoreTTL(key, entry, time.Until(time.UnixMilli(entry.ExpiredAt)))
		return
	}
	if state.Inner.AuthCacheEntries.Load() >= maxAuthCacheEntries {
		now := time.Now().UnixMilli()
		count := state.Inner.AuthCache.DeleteMatching(func(_ string, entry AuthCacheEntry) bool { return entry.ExpiredAt <= now }, 1000)
		if count == 0 {
			count = state.Inner.AuthCache.DeleteMatching(func(string, AuthCacheEntry) bool { return true }, 2000)
		}
		if count > 0 {
			state.Inner.AuthCacheEntries.Add(^(uint64(count) - 1))
		}
	}
	state.Inner.AuthCache.StoreTTL(key, entry, time.Until(time.UnixMilli(entry.ExpiredAt)))
	state.Inner.AuthCacheEntries.Add(1)
}

func (state *AppState) DeleteAuthCache(key string) {
	state.Inner.AuthCacheWriteLock.Lock()
	defer state.Inner.AuthCacheWriteLock.Unlock()
	state.Inner.AuthCacheGeneration.Add(1)

	if _, loaded := state.Inner.AuthCache.LoadAndDelete(key); loaded {
		state.Inner.AuthCacheEntries.Add(^uint64(0))
	}
}

func (state *AppState) ClearAuthCache() {
	state.Inner.AuthCacheWriteLock.Lock()
	defer state.Inner.AuthCacheWriteLock.Unlock()
	state.Inner.AuthCacheGeneration.Add(1)

	state.Inner.AuthCache.DeleteMatching(func(string, AuthCacheEntry) bool { return true }, 0)
	state.Inner.AuthCacheEntries.Store(0)
}

func (state *AppState) deleteAuthCacheWhere(predicate func(AuthCacheEntry) bool) int {
	state.Inner.AuthCacheWriteLock.Lock()
	defer state.Inner.AuthCacheWriteLock.Unlock()
	state.Inner.AuthCacheGeneration.Add(1)
	deleted := uint64(state.Inner.AuthCache.DeleteMatching(func(_ string, entry AuthCacheEntry) bool { return predicate(entry) }, 0))
	if deleted != 0 {
		state.Inner.AuthCacheEntries.Add(^(deleted - 1))
	}
	return int(deleted)
}

// DeleteExpiredAuthCache removes expired entries with bounded remote deletion batches.
func (state *AppState) DeleteExpiredAuthCache(now time.Time) int {
	if state == nil || state.Inner == nil {
		return 0
	}
	return state.deleteAuthCacheWhere(func(entry AuthCacheEntry) bool { return entry.ExpiredAt <= now.UnixMilli() })
}

// InvalidateAccountAuthCache removes cached credentials for selected accounts.
// When clearFailures is true, negative credential results are also discarded so newly valid credentials work immediately.
func (state *AppState) InvalidateAccountAuthCache(clearFailures bool, usernames ...string) {
	if state == nil || state.Inner == nil {
		return
	}
	names := make(map[string]struct{}, len(usernames))
	for _, username := range usernames {
		if username = strings.ToLower(strings.TrimSpace(username)); username != "" {
			names[username] = struct{}{}
		}
	}
	state.deleteAuthCacheWhere(func(entry AuthCacheEntry) bool {
		if clearFailures && entry.Invalid {
			return true
		}
		if entry.User == nil {
			return false
		}
		_, remove := names[strings.ToLower(entry.User.Username)]
		return remove
	})
}

// InvalidateAPITokenAuthCache removes only authentication results produced by one revoked or suspended API token.
func (state *AppState) InvalidateAPITokenAuthCache(tokenID string) {
	if state == nil || state.Inner == nil || tokenID == "" {
		return
	}
	state.deleteAuthCacheWhere(func(entry AuthCacheEntry) bool {
		return entry.APITokenID == tokenID
	})
}

func (state *AppState) StoreMetadataCache(key string, metadata *config.Metadata) {
	if metadata == nil {
		return
	}
	state.Inner.MetadataCacheWriteLock.Lock()
	defer state.Inner.MetadataCacheWriteLock.Unlock()

	if state.Inner.MetadataCache.Contains(key) {
		state.Inner.MetadataCache.Store(key, metadata)
		return
	}
	if state.Inner.MetadataCacheEntries.Load() >= maxMetadataCacheEntries {
		return
	}
	state.Inner.MetadataCache.Store(key, metadata)
	state.Inner.MetadataCacheEntries.Add(1)
}

func (state *AppState) DeleteMetadataCache(key string) {
	state.Inner.MetadataCacheWriteLock.Lock()
	defer state.Inner.MetadataCacheWriteLock.Unlock()

	if _, loaded := state.Inner.MetadataCache.LoadAndDelete(key); loaded {
		state.Inner.MetadataCacheEntries.Add(^uint64(0))
	}
}
