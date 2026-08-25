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
	"bufio"
	"context"
	"os"

	"renop/internal/core"
	"renop/internal/utils"
)

// NewIndexSaveTask returns a non-reentrant callback that prunes negative index
// entries and atomically persists dirty index state.
func NewIndexSaveTask(state *core.AppState, path string) func(context.Context) {
	hasWritten := false
	return func(ctx context.Context) {
		if ctx == nil || ctx.Err() != nil || state == nil || state.Inner == nil || state.Inner.FileIndex == nil || path == "" {
			return
		}
		state.Inner.FileIndex.PruneNotFound()
		dirty := state.Inner.FileIndex.IsDirty.Swap(false)
		if !dirty && hasWritten {
			return
		}

		tmpPath := path + ".tmp"
		file, err := os.Create(tmpPath)
		if err == nil {
			writer := bufio.NewWriterSize(file, 64*1024)
			err = state.Inner.FileIndex.WriteJSONTo(writer)
			if flushErr := writer.Flush(); err == nil {
				err = flushErr
			}
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
			if err == nil {
				err = utils.SafeRename(tmpPath, path)
			}
		}
		if err == nil {
			hasWritten = true
			return
		}
		_ = os.Remove(tmpPath)
		state.Inner.FileIndex.IsDirty.Store(true)
	}
}
