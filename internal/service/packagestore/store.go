/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

// Package packagestore defines the streaming persistence boundary shared by package protocols.
package packagestore

import (
	"io"

	"renop/internal/core"
)

// StagedFile is a temporary blob that can be validated before an atomic commit.
type StagedFile interface {
	io.WriteCloser
	Open() (io.ReadCloser, error)
	Size() (int64, error)
	Commit(state *core.AppState) error
	Discard() error
}

// Store provides streaming package reads, staging, commits, and deletes.
type Store interface {
	Open(path string) (reader io.ReadCloser, found bool, err error)
	Exists(path string) (bool, error)
	Stage(path string) (StagedFile, error)
	Delete(state *core.AppState, path string) error
}
