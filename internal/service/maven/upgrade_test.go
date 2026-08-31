/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/index"
	"renop/internal/testutil"
)

func TestUpgradeLegacyRepositoryImportsCatalogWithoutGrantingTeamAccess(t *testing.T) {
	storagePath := testutil.TempDir(t)
	repositoryRoot := filepath.Join(storagePath, "legacy")
	artifactPath := filepath.Join(repositoryRoot, "com", "example", "demo", "1.0", "demo-1.0.jar")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0o755))
	require.NoError(t, os.WriteFile(artifactPath, []byte("legacy"), 0o644))

	state := core.NewAppState()
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"legacy": {Name: "legacy", Format: config.RepositoryFormatMaven, Visibility: "PUBLIC"},
	}
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "legacy.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state.Inner.DB = db

	require.NoError(t, UpgradeLegacyRepository(state, "legacy"))
	completed, err := db.IsMavenRepositoryUpgraded("legacy")
	require.NoError(t, err)
	assert.True(t, completed)
	details, err := db.GetMavenDomainDetails("com.example", "guest")
	require.NoError(t, err)
	assert.True(t, details.Domain.Verified)
	assert.Empty(t, details.Members)
	artifact, err := db.GetMavenArtifactDetails("legacy", "com.example", "demo")
	require.NoError(t, err)
	require.Len(t, artifact.Versions, 1)
	assert.Equal(t, "1.0", artifact.Versions[0].Version)
	require.NoError(t, UpgradeLegacyRepository(state, "legacy"))
}

func TestRebuildRepositoryCatalogStreamsS3IndexAndIgnoresArbitraryFiles(t *testing.T) {
	storagePath := testutil.TempDir(t)
	repositoryRoot := filepath.Join(storagePath, "files")
	artifactPath := filepath.Join(repositoryRoot, "org", "example", "demo", "2.0", "demo-2.0.jar")
	arbitraryPath := filepath.Join(repositoryRoot, "notes", "readme.txt")

	state := core.NewAppState()
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"files": {
			Name: "files", Format: config.RepositoryFormatFiles, Visibility: "PUBLIC",
			S3: &config.S3Config{Enabled: true},
		},
	}
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndexCustom(true)
	state.Inner.FileIndex.InsertDir(repositoryRoot)
	state.Inner.FileIndex.EnsureParentDirs(artifactPath)
	state.Inner.FileIndex.InsertFile(artifactPath, index.FileInfo{Size: 42, ModTime: time.Now().UnixNano()})
	state.Inner.FileIndex.EnsureParentDirs(arbitraryPath)
	state.Inner.FileIndex.InsertFile(arbitraryPath, index.FileInfo{Size: 7, ModTime: time.Now().UnixNano()})
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "rebuild.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	state.Inner.DB = db

	require.NoError(t, RebuildRepositoryCatalog(state, "files"))
	details, err := db.GetMavenArtifactDetails("files", "org.example", "demo")
	require.NoError(t, err)
	require.Len(t, details.Versions, 1)
	assert.Equal(t, "2.0", details.Versions[0].Version)
	assert.Equal(t, int64(42), details.Versions[0].Size)
	_, total, err := db.ListMavenArtifacts("files", "", "readme", 10, 0)
	require.NoError(t, err)
	assert.Zero(t, total)
}
