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
	"os"
	"time"

	"renop/internal/core"
	"renop/internal/utils"
)

func StartIndexSaver(state *core.AppState, path string) {
	go func() {
		hasWritten := false
		for {
			time.Sleep(10 * time.Second)

			state.Inner.FileIndex.PruneNotFound()

			dirty := state.Inner.FileIndex.IsDirty.Swap(false)
			if !dirty && hasWritten {
				continue
			}

			tmpPath := path + ".tmp"
			f, err := os.Create(tmpPath)
			if err == nil {
				bw := bufio.NewWriterSize(f, 64*1024)
				err = state.Inner.FileIndex.WriteJSONTo(bw)
				if flushErr := bw.Flush(); err == nil {
					err = flushErr
				}
				if closeErr := f.Close(); err == nil {
					err = closeErr
				}
				if err == nil {
					err = utils.SafeRename(tmpPath, path)
				}
				if err == nil {
					hasWritten = true
				} else {
					_ = os.Remove(tmpPath)
					state.Inner.FileIndex.IsDirty.Store(true)
				}
			} else {
				state.Inner.FileIndex.IsDirty.Store(true)
			}
		}
	}()
}
