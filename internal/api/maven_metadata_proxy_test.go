/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
	"renop/internal/service/storage"
)

func TestFindMetadataFetchesAndCachesMirrorMetadata(t *testing.T) {
	storagePath := t.TempDir()
	var hits atomic.Int64
	metadata := []byte(`<metadata><groupId>com.example</groupId><artifactId>demo</artifactId><versioning><versions><version>1.0.0</version></versions></versioning></metadata>`)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/com/example/demo/maven-metadata.xml" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(metadata)
	}))
	t.Cleanup(upstream.Close)

	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"mirror": {
			Name:       "mirror",
			Visibility: "PUBLIC",
			Mirrors:    []config.Mirror{{URL: upstream.URL, TimeoutSecs: 5}},
		},
	}
	storage.InitS3(cfg)
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()

	got, err := FindMetadata(state, "mirror", "com/example/demo")
	if err != nil {
		t.Fatalf("FindMetadata: %v", err)
	}
	if got.ArtifactID == nil || *got.ArtifactID != "demo" {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream requests = %d, want 1", hits.Load())
	}
	path := filepath.Join(storagePath, "mirror", "com", "example", "demo", "maven-metadata.xml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("metadata was not persisted: %v", err)
	}

	if _, err := FindMetadata(state, "mirror", "com/example/demo"); err != nil {
		t.Fatalf("cached FindMetadata: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("cached lookup made another upstream request: %d", hits.Load())
	}
}

func TestFindMetadataFallsBackToMavenParentMetadata(t *testing.T) {
	storagePath := t.TempDir()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/com/example/demo/maven-metadata.xml" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<metadata><artifactId>demo</artifactId><versioning><versions><version>1.0.0-SNAPSHOT</version></versions></versioning></metadata>`))
	}))
	t.Cleanup(upstream.Close)

	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"mirror": {Name: "mirror", Visibility: "PUBLIC", Mirrors: []config.Mirror{{URL: upstream.URL, TimeoutSecs: 5}}},
	}
	storage.InitS3(cfg)
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()

	metadata, err := FindMetadata(state, "mirror", "com/example/demo/1.0.0-SNAPSHOT")
	if err != nil {
		t.Fatalf("FindMetadata version path: %v", err)
	}
	if metadata.ArtifactID == nil || *metadata.ArtifactID != "demo" {
		t.Fatalf("unexpected fallback metadata: %+v", metadata)
	}
}
