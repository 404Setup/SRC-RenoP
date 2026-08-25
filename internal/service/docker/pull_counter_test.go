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
	"sync"
	"testing"

	"renop/internal/core"
)

type pullCountStoreStub struct {
	mu     sync.Mutex
	counts map[pullKey]int64
	err    error
}

func (store *pullCountStoreStub) BatchIncrementDockerPullCount(repository, imageName string, delta int64) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.err != nil {
		return store.err
	}
	store.counts[pullKey{repository: repository, imageName: imageName}] += delta
	return nil
}

func (store *pullCountStoreStub) count(key pullKey) int64 {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.counts[key]
}

func TestPullCounterAggregatesAndFlushes(t *testing.T) {
	store := &pullCountStoreStub{counts: make(map[pullKey]int64)}
	counter := newPullCounter(func() pullCountStore { return store })
	for range 25 {
		counter.RecordPull("containers", "library/alpine")
	}
	if err := counter.Flush(); err != nil {
		t.Fatal(err)
	}
	key := pullKey{repository: "containers", imageName: "library/alpine"}
	if got := store.count(key); got != 25 {
		t.Fatalf("flushed count = %d, want 25", got)
	}
	if err := counter.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := store.count(key); got != 25 {
		t.Fatalf("empty second flush changed count to %d", got)
	}
}

func TestPullCounterRequeuesFailedIncrements(t *testing.T) {
	transientErr := errors.New("temporary database failure")
	store := &pullCountStoreStub{counts: make(map[pullKey]int64), err: transientErr}
	counter := newPullCounter(func() pullCountStore { return store })
	counter.RecordPull("containers", "team/service")
	counter.RecordPull("containers", "team/service")
	if err := counter.Flush(); !errors.Is(err, transientErr) {
		t.Fatalf("first Flush error = %v", err)
	}

	store.mu.Lock()
	store.err = nil
	store.mu.Unlock()
	if err := counter.Flush(); err != nil {
		t.Fatal(err)
	}
	key := pullKey{repository: "containers", imageName: "team/service"}
	if got := store.count(key); got != 2 {
		t.Fatalf("retried count = %d, want 2", got)
	}
}

func TestPullCounterIsScopedToApplicationState(t *testing.T) {
	firstState := core.NewAppState()
	secondState := core.NewAppState()
	first := GetPullCounter(firstState)
	if first == nil || first != GetPullCounter(firstState) {
		t.Fatal("state did not retain its pull counter")
	}
	if first == GetPullCounter(secondState) {
		t.Fatal("different application states shared a pull counter")
	}
	if GetPullCounter(nil) != nil {
		t.Fatal("nil state returned a pull counter")
	}
}
