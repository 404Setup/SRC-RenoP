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
	"time"

	"renop/internal/core"
)

func HandleNegativeCache(state *core.AppState, repoName string, path string, storagePath string, negativeTtl uint64) {
	expireAt := time.Now().Unix() + int64(negativeTtl)
	localFilePath := filepath.Join(storagePath, repoName, path)
	state.Inner.FileIndex.InsertNotFound(localFilePath, expireAt)
}
