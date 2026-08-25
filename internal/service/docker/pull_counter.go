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
	"errors"
	"fmt"
	"sync"

	"renop/internal/core"
)

type pullKey struct {
	repository string
	imageName  string
}

// PullCounter aggregates image pull increments in memory for scheduled batch writes.
type PullCounter struct {
	mu       sync.Mutex
	pending  map[pullKey]int64
	database func() pullCountStore
}

type pullCountStore interface {
	BatchIncrementDockerPullCount(repository, imageName string, delta int64) error
}

// NewPullCounter creates an application-scoped Docker pull counter.
func NewPullCounter(state *core.AppState) *PullCounter {
	return newPullCounter(func() pullCountStore {
		if state == nil {
			return nil
		}
		return state.GetDB()
	})
}

func newPullCounter(database func() pullCountStore) *PullCounter {
	return &PullCounter{
		pending:  make(map[pullKey]int64),
		database: database,
	}
}

// GetPullCounter returns the pull counter owned by state.
func GetPullCounter(state *core.AppState) *PullCounter {
	if state == nil || state.Inner == nil {
		return nil
	}
	state.Inner.DockerPullCounterMu.Lock()
	defer state.Inner.DockerPullCounterMu.Unlock()
	if counter, ok := state.Inner.DockerPullCounter.(*PullCounter); ok {
		return counter
	}
	counter := NewPullCounter(state)
	state.Inner.DockerPullCounter = counter
	return counter
}

// RecordPull atomically records a pull event in memory without triggering database write locks.
func (pc *PullCounter) RecordPull(repository, imageName string) {
	if pc == nil || repository == "" || imageName == "" {
		return
	}
	key := pullKey{repository: repository, imageName: imageName}
	pc.mu.Lock()
	pc.pending[key]++
	pc.mu.Unlock()
}

// Flush synchronously writes all buffered pull counts to the database. Failed
// increments are returned to the pending batch so transient errors do not lose statistics.
func (pc *PullCounter) Flush() error {
	if pc == nil {
		return nil
	}
	pc.mu.Lock()
	if len(pc.pending) == 0 {
		pc.mu.Unlock()
		return nil
	}
	batch := pc.pending
	pc.pending = make(map[pullKey]int64)
	pc.mu.Unlock()

	var db pullCountStore
	if pc.database != nil {
		db = pc.database()
	}
	if db == nil {
		pc.requeue(batch)
		return core.ErrDatabaseUnavailable
	}

	failed := make(map[pullKey]int64)
	var flushErrors []error
	for k, count := range batch {
		if count <= 0 {
			continue
		}
		if err := db.BatchIncrementDockerPullCount(k.repository, k.imageName, count); err != nil {
			failed[k] += count
			flushErrors = append(flushErrors, fmt.Errorf("increment %s/%s: %w", k.repository, k.imageName, err))
		}
	}
	pc.requeue(failed)
	return errors.Join(flushErrors...)
}

func (pc *PullCounter) requeue(batch map[pullKey]int64) {
	if len(batch) == 0 {
		return
	}
	pc.mu.Lock()
	for key, count := range batch {
		pc.pending[key] += count
	}
	pc.mu.Unlock()
}
