/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package statistics records and queries bounded package-download aggregates.
package statistics

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"renop/internal/config"
	"renop/internal/core"
)

const maxPendingDownloadStatisticKeys = 131_072

type downloadKey struct {
	username    string
	repository  string
	format      string
	namespace   string
	packageName string
	version     string
}

type downloadValue struct {
	count     int64
	bytes     int64
	updatedAt int64
}

type downloadStore interface {
	BatchIncrementDownloadStatistics(events []*core.DownloadStatisticDelta) error
	ResetDownloadStatistics(repository string) error
}

// Counter aggregates download increments in memory for scheduled batch writes.
type Counter struct {
	mu             sync.Mutex
	flushMu        sync.Mutex
	pending        map[downloadKey]downloadValue
	maxPendingKeys int
	dropped        atomic.Uint64
	database       func() downloadStore
}

// NewCounter creates an application-scoped download-statistics counter.
func NewCounter(state *core.AppState) *Counter {
	return newCounter(func() downloadStore {
		if state == nil {
			return nil
		}
		return state.GetDB()
	})
}

func newCounter(database func() downloadStore) *Counter {
	return &Counter{
		pending: make(map[downloadKey]downloadValue), maxPendingKeys: maxPendingDownloadStatisticKeys,
		database: database,
	}
}

// GetCounter returns the counter owned by one application state.
func GetCounter(state *core.AppState) *Counter {
	if state == nil || state.Inner == nil {
		return nil
	}
	state.Inner.DownloadStatisticsCounterMu.Lock()
	defer state.Inner.DownloadStatisticsCounterMu.Unlock()
	if counter, ok := state.Inner.DownloadStatisticsCounter.(*Counter); ok {
		return counter
	}
	counter := NewCounter(state)
	state.Inner.DownloadStatisticsCounter = counter
	return counter
}

func boundedStatisticValue(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return strings.Clone(value)
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return strings.Clone(strings.TrimSpace(value[:end]))
}

func normalizeDownloadEvent(event core.DownloadStatisticDelta) (downloadKey, downloadValue, bool) {
	repository := strings.ToLower(boundedStatisticValue(event.Repository, 64))
	format := strings.ToLower(boundedStatisticValue(event.Format, 32))
	if repository == "" || !config.IsSupportedRepositoryFormat(format) {
		return downloadKey{}, downloadValue{}, false
	}
	if format == config.RepositoryFormatMavenClassic {
		format = config.RepositoryFormatMaven
	}
	count := event.Count
	if count <= 0 {
		count = 1
	}
	updatedAt := event.UpdatedAt
	if updatedAt <= 0 {
		updatedAt = time.Now().UnixMilli()
	}
	return downloadKey{
		username:    strings.ToLower(boundedStatisticValue(event.Username, 255)),
		repository:  repository,
		format:      format,
		namespace:   strings.ToLower(boundedStatisticValue(event.Namespace, 253)),
		packageName: boundedStatisticValue(event.Package, 512),
		version:     boundedStatisticValue(event.Version, 255),
	}, downloadValue{count: count, bytes: max(event.Bytes, 0), updatedAt: updatedAt}, true
}

// Record adds one successful package download without taking a database lock.
func (counter *Counter) Record(event core.DownloadStatisticDelta) {
	if counter == nil {
		return
	}
	key, value, ok := normalizeDownloadEvent(event)
	if !ok {
		return
	}
	counter.mu.Lock()
	current, exists := counter.pending[key]
	if !exists && len(counter.pending) >= counter.maxPendingKeys {
		counter.mu.Unlock()
		counter.dropped.Add(uint64(value.count))
		return
	}
	current.count += value.count
	current.bytes += value.bytes
	current.updatedAt = max(current.updatedAt, value.updatedAt)
	counter.pending[key] = current
	counter.mu.Unlock()
}

func downloadEvents(batch map[downloadKey]downloadValue) []*core.DownloadStatisticDelta {
	events := make([]*core.DownloadStatisticDelta, 0, len(batch))
	for key, value := range batch {
		events = append(events, &core.DownloadStatisticDelta{
			Username: key.username, Repository: key.repository, Format: key.format,
			Namespace: key.namespace, Package: key.packageName, Version: key.version,
			Count: value.count, Bytes: value.bytes, UpdatedAt: value.updatedAt,
		})
	}
	return events
}

func (counter *Counter) requeue(batch map[downloadKey]downloadValue) {
	if len(batch) == 0 {
		return
	}
	counter.mu.Lock()
	var dropped uint64
	for key, value := range batch {
		current, exists := counter.pending[key]
		if !exists && len(counter.pending) >= counter.maxPendingKeys {
			dropped += uint64(value.count)
			continue
		}
		current.count += value.count
		current.bytes += value.bytes
		current.updatedAt = max(current.updatedAt, value.updatedAt)
		counter.pending[key] = current
	}
	counter.mu.Unlock()
	if dropped > 0 {
		counter.dropped.Add(dropped)
	}
}

// Flush synchronously persists all buffered aggregates and requeues failed batches.
func (counter *Counter) Flush() error {
	if counter == nil {
		return nil
	}
	counter.flushMu.Lock()
	defer counter.flushMu.Unlock()
	counter.mu.Lock()
	batch := counter.pending
	counter.pending = make(map[downloadKey]downloadValue)
	counter.mu.Unlock()

	var flushErr error
	if len(batch) > 0 {
		var database downloadStore
		if counter.database != nil {
			database = counter.database()
		}
		if database == nil {
			flushErr = core.ErrDatabaseUnavailable
		} else if err := database.BatchIncrementDownloadStatistics(downloadEvents(batch)); err != nil {
			flushErr = err
		}
		if flushErr != nil {
			counter.requeue(batch)
		}
	}
	if dropped := counter.dropped.Swap(0); dropped > 0 {
		flushErr = errors.Join(flushErr,
			fmt.Errorf("download-statistics buffer discarded %d events after reaching its key limit", dropped))
	}
	return flushErr
}

// ResetRepository removes pending and persisted counters for one repository atomically with respect to Record.
func (counter *Counter) ResetRepository(repository string) error {
	if counter == nil {
		return nil
	}
	repository = strings.ToLower(strings.TrimSpace(repository))
	if repository == "" {
		return errors.New("download-statistics repository is required")
	}
	counter.flushMu.Lock()
	defer counter.flushMu.Unlock()
	if counter.database == nil {
		return core.ErrDatabaseUnavailable
	}
	database := counter.database()
	if database == nil {
		return core.ErrDatabaseUnavailable
	}
	counter.mu.Lock()
	defer counter.mu.Unlock()
	removed := make(map[downloadKey]downloadValue)
	for key := range counter.pending {
		if key.repository == repository {
			removed[key] = counter.pending[key]
			delete(counter.pending, key)
		}
	}
	if err := database.ResetDownloadStatistics(repository); err != nil {
		for key, value := range removed {
			current := counter.pending[key]
			current.count += value.count
			current.bytes += value.bytes
			current.updatedAt = max(current.updatedAt, value.updatedAt)
			counter.pending[key] = current
		}
		return err
	}
	return nil
}
