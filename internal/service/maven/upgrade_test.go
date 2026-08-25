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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/index"
)

func TestUpgradeLegacyRepositoryImportsCatalogWithoutGrantingTeamAccess(t *testing.T) {
	storagePath := t.TempDir()
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
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "legacy.db"), MaxOpenConns: 1, MaxIdleConns: 1,
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
