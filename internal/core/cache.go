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

	"renop/internal/config"
)

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
	state.Inner.AuthCacheWriteLock.Lock()
	defer state.Inner.AuthCacheWriteLock.Unlock()

	if _, ok := state.Inner.AuthCache.Load(key); ok {
		state.Inner.AuthCache.Store(key, entry)
		return
	}
	if state.Inner.AuthCacheEntries.Load() >= maxAuthCacheEntries {
		now := time.Now().UnixMilli()
		var count uint64
		state.Inner.AuthCache.Range(func(k string, v AuthCacheEntry) bool {
			if v.ExpiredAt <= now {
				if _, loaded := state.Inner.AuthCache.LoadAndDelete(k); loaded {
					count++
				}
			}
			return count < 1000
		})
		if count > 0 {
			state.Inner.AuthCacheEntries.Add(^(count - 1))
		} else {
			state.Inner.AuthCache.Range(func(k string, _ AuthCacheEntry) bool {
				if _, loaded := state.Inner.AuthCache.LoadAndDelete(k); loaded {
					count++
				}
				return count < 2000
			})
			if count > 0 {
				state.Inner.AuthCacheEntries.Add(^(count - 1))
			}
		}
	}
	state.Inner.AuthCache.Store(key, entry)
	state.Inner.AuthCacheEntries.Add(1)
}

func (state *AppState) DeleteAuthCache(key string) {
	state.Inner.AuthCacheWriteLock.Lock()
	defer state.Inner.AuthCacheWriteLock.Unlock()

	if _, loaded := state.Inner.AuthCache.LoadAndDelete(key); loaded {
		state.Inner.AuthCacheEntries.Add(^uint64(0))
	}
}

func (state *AppState) ClearAuthCache() {
	state.Inner.AuthCacheWriteLock.Lock()
	defer state.Inner.AuthCacheWriteLock.Unlock()

	state.Inner.AuthCache.Range(func(key string, _ AuthCacheEntry) bool {
		state.Inner.AuthCache.Delete(key)
		return true
	})
	state.Inner.AuthCacheEntries.Store(0)
}

func (state *AppState) StoreMetadataCache(key string, metadata *config.Metadata) {
	if metadata == nil {
		return
	}
	state.Inner.MetadataCacheWriteLock.Lock()
	defer state.Inner.MetadataCacheWriteLock.Unlock()

	if _, ok := state.Inner.MetadataCache.Load(key); ok {
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
