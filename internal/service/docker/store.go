/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package docker

import (
	"io"

	"renop/internal/core"
)

// Store defines the abstraction for Docker blob and manifest storage.
type Store interface {
	// OpenBlob returns a reader for the blob identified by its sha256 digest.
	OpenBlob(repository, digest string) (io.ReadCloser, int64, bool, error)

	// BlobExists reports whether a blob exists and returns its size.
	BlobExists(repository, digest string) (bool, int64, error)

	// BlobFilePath returns the local filesystem path if stored on disk, and whether it is locally available.
	BlobFilePath(repository, digest string) (string, bool)

	// StageBlob initializes or opens a temporary upload staging file.
	StageBlob(repository, uploadUUID string) (StagedBlob, error)

	// GetStagedBlob returns an existing upload staging file.
	GetStagedBlob(repository, uploadUUID string) (StagedBlob, error)

	// CommitBlob finalizes a staging file and stores it as an immutable blob.
	CommitBlob(state *core.AppState, repository, uploadUUID, digest string) (int64, error)

	// DeleteBlob removes a blob from storage.
	DeleteBlob(state *core.AppState, repository, digest string) error

	// OpenManifest returns the raw JSON content of a manifest by digest.
	OpenManifest(repository, imageName, digest string) ([]byte, bool, error)

	// PutManifest writes manifest JSON content.
	PutManifest(state *core.AppState, repository, imageName, digest string, data []byte) error

	// DeleteManifest removes manifest content.
	DeleteManifest(state *core.AppState, repository, imageName, digest string) error
}

// StagedBlob represents an in-flight chunked or monolithic blob upload.
type StagedBlob interface {
	io.Writer
	WriteAt(p []byte, off int64) (n int, err error)
	Size() (int64, error)
	Close() error
	Discard() error
	Digest() (string, error)
}
