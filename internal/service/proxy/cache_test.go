/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package proxy

import (
	"path/filepath"
	"testing"
	"time"

	"renop/internal/core"
	"renop/internal/service/index"
)

func TestHandleNegativeCache(t *testing.T) {
	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndex()

	repoName := "test-repo"
	path := "com/example/artifact/1.0/artifact-1.0.jar"
	storagePath := "/tmp/renop_test_storage"
	var negativeTtl uint64 = 300

	nowSecs := time.Now().Unix()

	HandleNegativeCache(state, repoName, path, storagePath, negativeTtl)

	time.Sleep(100 * time.Millisecond)

	expectedPath := filepath.Join(storagePath, repoName, path)

	if !state.Inner.FileIndex.IsNotFound(expectedPath) {
		t.Fatalf("Path should be in negative cache")
	}

	snapshot := state.Inner.FileIndex.Snapshot()
	expireAt, ok := snapshot.NotFound[filepath.ToSlash(expectedPath)]
	if !ok {
		t.Fatalf("Path not in negative cache map")
	}

	expectedExpire := nowSecs + int64(negativeTtl)
	if expireAt < expectedExpire || expireAt > expectedExpire+5 {
		t.Fatalf("TTL is incorrect. Expected ~%d, got %d", expectedExpire, expireAt)
	}
}
