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
	"sync"
	"time"

	"renop/internal/core"
)

type pullKey struct {
	repository string
	imageName  string
}

// PullCounter aggregates image pull increments in memory and flushes them to the database asynchronously.
type PullCounter struct {
	mu      sync.Mutex
	pending map[pullKey]int64
	state   *core.AppState
}

var (
	globalPullCounter *PullCounter
	pullCounterOnce   sync.Once
)

// GetPullCounter returns the singleton PullCounter instance initialized with background flushing.
func GetPullCounter(state *core.AppState) *PullCounter {
	pullCounterOnce.Do(func() {
		globalPullCounter = &PullCounter{
			pending: make(map[pullKey]int64),
			state:   state,
		}
		go globalPullCounter.flusher()
	})
	return globalPullCounter
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

// Flush synchronously writes all buffered pull counts to the database.
func (pc *PullCounter) Flush() {
	if pc == nil {
		return
	}
	pc.mu.Lock()
	if len(pc.pending) == 0 {
		pc.mu.Unlock()
		return
	}
	batch := pc.pending
	pc.pending = make(map[pullKey]int64)
	pc.mu.Unlock()

	db := pc.state.GetDB()
	if db == nil {
		return
	}

	for k, count := range batch {
		if count > 0 {
			_ = db.BatchIncrementDockerPullCount(k.repository, k.imageName, count)
		}
	}
}

func (pc *PullCounter) flusher() {
	ticker := time.NewTicker(2 * time.Second)
	for range ticker.C {
		pc.Flush()
	}
}
