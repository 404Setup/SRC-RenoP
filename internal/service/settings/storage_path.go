/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
	"renop/internal/service/javadocs"
)

func normalizeStoragePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(abs)
}

func sameStoragePath(a, b string) bool {
	na, nb := normalizeStoragePath(a), normalizeStoragePath(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(na, nb)
	}
	return na == nb
}

func onStoragePathChanged(state *core.AppState, storagePath string) {
	_ = os.MkdirAll(storagePath, 0755)
	if cfg, ok := state.Inner.Config.Load().(*config.Config); ok && cfg != nil {
		for repoName := range cfg.Maven.Repositories {
			_ = os.MkdirAll(filepath.Join(storagePath, repoName), 0755)
		}
	}

	restartIndexWatcher(state, storagePath)
	index.RebuildIndexAsync(storagePath, state.Inner.FileIndex)
	_ = javadocs.ClearAllJavadocCaches()
}

func restartIndexWatcher(state *core.AppState, storagePath string) {
	state.Inner.IndexWatcherMutex.Lock()
	defer state.Inner.IndexWatcherMutex.Unlock()

	if state.Inner.IndexWatcher != nil {
		_ = state.Inner.IndexWatcher.Close()
		state.Inner.IndexWatcher = nil
	}

	watcher, err := index.StartFileWatcher(storagePath, state.Inner.FileIndex)
	if err != nil {
		return
	}
	state.Inner.IndexWatcher = watcher
}
