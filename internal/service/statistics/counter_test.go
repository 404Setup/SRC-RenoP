/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package statistics

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
)

type downloadStoreStub struct {
	mu         sync.Mutex
	events     []*core.DownloadStatisticDelta
	reset      string
	failWrites bool
}

func (store *downloadStoreStub) BatchIncrementDownloadStatistics(events []*core.DownloadStatisticDelta) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.failWrites {
		return errors.New("write failed")
	}
	store.events = append(store.events, events...)
	return nil
}

func (store *downloadStoreStub) ResetDownloadStatistics(repository string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.reset = repository
	return nil
}

func TestDownloadCounterAggregatesAndRetriesFailedFlush(t *testing.T) {
	store := &downloadStoreStub{failWrites: true}
	counter := newCounter(func() downloadStore { return store })
	for range 3 {
		counter.Record(core.DownloadStatisticDelta{
			Username: "Alice", Repository: "Releases", Format: "maven",
			Namespace: "com.example", Package: "com.example:demo", Version: "1.0", Bytes: 1024,
		})
	}
	require.Error(t, counter.Flush())
	store.failWrites = false
	require.NoError(t, counter.Flush())
	require.Len(t, store.events, 1)
	assert.Equal(t, "alice", store.events[0].Username)
	assert.Equal(t, "releases", store.events[0].Repository)
	assert.Equal(t, int64(3), store.events[0].Count)
	assert.Equal(t, int64(3072), store.events[0].Bytes)
}

func TestClassifyNPMDownloadCountsOnlyCanonicalVersionTarballs(t *testing.T) {
	repository := &config.Repository{Name: "npm", Format: config.RepositoryFormatNPM}
	namespace, packageName, version, ok := classifyRepositoryDownload(
		repository, "@example/library/-/library-1.2.3.tgz")
	require.True(t, ok)
	assert.Equal(t, "@example", namespace)
	assert.Equal(t, "@example/library", packageName)
	assert.Equal(t, "1.2.3", version)

	namespace, packageName, version, ok = classifyRepositoryDownload(repository, "demo/-/demo-2.0.0.tgz")
	require.True(t, ok)
	assert.Empty(t, namespace)
	assert.Equal(t, "demo", packageName)
	assert.Equal(t, "2.0.0", version)
	for _, path := range []string{"demo", "-/v1/search", "demo/-/demo-latest.tgz"} {
		_, _, _, ok = classifyRepositoryDownload(repository, path)
		assert.Falsef(t, ok, "npm metadata path %q must not count as a package download", path)
	}
}

func TestDownloadCounterResetDropsPendingRepository(t *testing.T) {
	store := &downloadStoreStub{}
	counter := newCounter(func() downloadStore { return store })
	counter.Record(core.DownloadStatisticDelta{Repository: "one", Format: "files", Package: "a.zip"})
	counter.Record(core.DownloadStatisticDelta{Repository: "two", Format: "files", Package: "b.zip"})
	require.NoError(t, counter.ResetRepository("one"))
	require.NoError(t, counter.Flush())
	assert.Equal(t, "one", store.reset)
	require.Len(t, store.events, 1)
	assert.Equal(t, "two", store.events[0].Repository)
}

func TestDownloadCounterFailedBatchCannotGrowPastKeyLimit(t *testing.T) {
	counter := newCounter(func() downloadStore { return &downloadStoreStub{} })
	counter.maxPendingKeys = 1
	counter.Record(core.DownloadStatisticDelta{
		Repository: "current", Format: "files", Package: "current.zip",
	})
	counter.requeue(map[downloadKey]downloadValue{
		{repository: "failed", format: "files", packageName: "failed.zip"}: {count: 2},
	})

	counter.mu.Lock()
	defer counter.mu.Unlock()
	require.Len(t, counter.pending, 1)
	assert.Contains(t, counter.pending,
		downloadKey{repository: "current", format: "files", packageName: "current.zip"})
	assert.Equal(t, uint64(2), counter.dropped.Load())
}

func TestRepositoryDownloadClassificationExcludesCompanionsAndDocs(t *testing.T) {
	namespace, packageName, version, ok := classifyMavenDownload("com/example/demo/1.0/demo-1.0.jar")
	require.True(t, ok)
	assert.Equal(t, "com.example", namespace)
	assert.Equal(t, "com.example:demo", packageName)
	assert.Equal(t, "1.0", version)
	for _, path := range []string{
		"com/example/demo/1.0/demo-1.0.jar.sha256",
		"com/example/demo/1.0/demo-1.0.jar.asc",
		"com/example/demo/1.0/demo-1.0-javadoc.jar",
		"com/example/demo/maven-metadata.xml",
	} {
		_, _, _, ok := classifyMavenDownload(path)
		assert.False(t, ok, path)
	}
	packageName, version, ok = classifyCargoDownload("api/v1/crates/demo/2.0/download")
	require.True(t, ok)
	assert.Equal(t, "demo", packageName)
	assert.Equal(t, "2.0", version)
}
