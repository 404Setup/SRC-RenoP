/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package upload

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// IsUploadPartialName reports single-shot (.tmp.) or chunked (.chunk.) partials.
func IsUploadPartialName(name string) bool {
	base := filepath.Base(name)
	if base == "" {
		return false
	}
	if strings.HasSuffix(base, ".tmp") || strings.Contains(base, ".tmp.") {
		return true
	}
	return strings.Contains(base, ".chunk.")
}

// CleanupOrphanPartials removes leftover upload temps under storageRoot.
// When maxAge <= 0 every matching file is removed (safe after process restart
// when no sessions exist). Otherwise only files older than maxAge are removed.
func CleanupOrphanPartials(storageRoot string, maxAge time.Duration) int {
	if storageRoot == "" {
		return 0
	}
	root, err := filepath.Abs(storageRoot)
	if err != nil {
		return 0
	}
	now := time.Now()
	var removed int
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if !IsUploadPartialName(d.Name()) {
			return nil
		}
		if maxAge > 0 {
			info, statErr := d.Info()
			if statErr != nil {
				return nil
			}
			if now.Sub(info.ModTime()) < maxAge {
				return nil
			}
		}
		if os.Remove(path) == nil {
			removed++
		}
		return nil
	})
	return removed
}

// osTempPrefixes are CreateTemp patterns used by chunked upload and the updater.
var osTempPrefixes = []string{
	"renop-chunk-upload-",
	"renop-upload-",
	"renop-download-",
	"renop-inner-",
}

// readyBinaryPrefix is the extracted executable temp (kept while ready_to_restart).
const readyBinaryPrefix = "renop-new-"

// CleanupStaleOSTempUploads removes abandoned renop-* temps in the OS temp dir.
// When includeReadyBinaries is false, renop-new-* files are left alone so an
// in-progress ready_to_restart binary is not deleted by the periodic sweeper.
// maxAge <= 0 removes matching files of any age.
func CleanupStaleOSTempUploads(maxAge time.Duration, includeReadyBinaries bool) int {
	dir := os.TempDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	now := time.Now()
	var removed int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		match := false
		for _, p := range osTempPrefixes {
			if strings.HasPrefix(name, p) {
				match = true
				break
			}
		}
		if !match && includeReadyBinaries && strings.HasPrefix(name, readyBinaryPrefix) {
			match = true
		}
		if !match {
			continue
		}
		if maxAge > 0 {
			info, statErr := e.Info()
			if statErr != nil {
				continue
			}
			if now.Sub(info.ModTime()) < maxAge {
				continue
			}
		}
		if os.Remove(filepath.Join(dir, name)) == nil {
			removed++
		}
	}
	return removed
}
