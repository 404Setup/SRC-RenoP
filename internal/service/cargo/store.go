/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"io"

	"renop/internal/core"
)

// StagedFile is a temporary, bounded-lifetime blob. Callers stream data into
// it, reopen it for validation, then atomically commit or discard it.
type StagedFile interface {
	io.WriteCloser
	Open() (io.ReadCloser, error)
	Size() (int64, error)
	Commit(state *core.AppState) error
	Discard() error
}

// Store is the only persistence boundary used by the Cargo module. Open and
// Stage are streaming so crate and index sizes do not become heap allocations.
type Store interface {
	Open(path string) (reader io.ReadCloser, found bool, err error)
	Exists(path string) (bool, error)
	Stage(path string) (StagedFile, error)
	Delete(state *core.AppState, path string) error
}
