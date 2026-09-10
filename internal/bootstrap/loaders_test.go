/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"renop/internal/config"
	"renop/internal/database"
	"renop/internal/testutil"
)

func TestLoadConfig(t *testing.T) {
	t.Run("existing config file", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")

		expected := config.DefaultConfig()
		expected.Server.Port = 9090
		data, err := yaml.Marshal(&expected)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(cfgPath, data, 0644))

		cfg, err := LoadConfig(cfgPath)
		require.NoError(t, err)
		assert.Equal(t, uint16(9090), cfg.Server.Port)
	})

	t.Run("missing config file creates default", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "missing.yaml")

		cfg, err := LoadConfig(cfgPath)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.FileExists(t, cfgPath)
	})

	t.Run("invalid config file is rejected", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "invalid.yaml")
		require.NoError(t, os.WriteFile(cfgPath, []byte("invalid: : : yaml"), 0644))

		cfg, err := LoadConfig(cfgPath)
		require.Error(t, err)
		require.Nil(t, cfg)
	})
}

func TestLoadMaven(t *testing.T) {
	t.Run("valid maven file", func(t *testing.T) {
		dir := t.TempDir()
		mavenPath := filepath.Join(dir, "maven.yaml")

		mavenData := config.DefaultMavenSettings()
		data, err := yaml.Marshal(mavenData)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(mavenPath, data, 0644))

		settings, err := readLegacyRepositories(mavenPath)
		require.NoError(t, err)
		assert.NotEmpty(t, settings)
	})

	t.Run("missing maven file is reported", func(t *testing.T) {
		dir := t.TempDir()
		mavenPath := filepath.Join(dir, "missing.yaml")

		_, err := readLegacyRepositories(mavenPath)
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("invalid maven file is rejected", func(t *testing.T) {
		dir := t.TempDir()
		mavenPath := filepath.Join(dir, "invalid.yaml")
		require.NoError(t, os.WriteFile(mavenPath, []byte("invalid: : : yaml"), 0644))

		_, err := readLegacyRepositories(mavenPath)
		require.Error(t, err)
	})
}

func TestRepositoryMigrationSurvivesRestartAndPreservesEmptySet(t *testing.T) {
	for _, source := range []string{"missing", "repositories: {}", "repositories:\n  snapshot:\n    format: files\n    visibility: PRIVATE\n"} {
		t.Run(source, func(t *testing.T) {
			dir := testutil.TempDir(t)
			path := filepath.Join(dir, "repositories.yaml")
			if source != "missing" {
				require.NoError(t, os.WriteFile(path, []byte(source), 0600))
			}
			dbConfig := config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(dir, "settings.db"), MaxOpenConns: 1}
			db, err := database.InitDB(dbConfig)
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			loaded, err := loadRepositorySettings(db, path)
			require.NoError(t, err)
			switch source {
			case "missing":
				require.Len(t, loaded.Repositories, len(config.DefaultMavenSettings().Repositories))
			case "repositories: {}":
				require.Empty(t, loaded.Repositories)
			default:
				require.Len(t, loaded.Repositories, 1)
				require.Equal(t, "snapshot", loaded.Repositories["snapshot"].Name)
				require.Equal(t, "PRIVATE", loaded.Repositories["snapshot"].Visibility)
			}
			require.NoFileExists(t, path)
			archives, err := filepath.Glob(path + ".migrated.*")
			require.NoError(t, err)
			if source != "missing" {
				require.Len(t, archives, 1)
				archived, err := os.ReadFile(archives[0])
				require.NoError(t, err)
				require.Equal(t, source, string(archived))
			}
			require.NoError(t, db.SaveRepositorySettings(config.MavenSettings{}))
			require.NoError(t, db.Close())
			db, err = database.InitDB(dbConfig)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, []byte("invalid: : : legacy"), 0600))
			loaded, err = loadRepositorySettings(db, path)
			require.NoError(t, err)
			require.Empty(t, loaded.Repositories)
		})
	}
}

func TestRepositoryMigrationRejectsInvalidSourceWithoutCommitting(t *testing.T) {
	dir := testutil.TempDir(t)
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(dir, "settings.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	path := filepath.Join(dir, "repositories.yaml")
	for _, source := range []string{"invalid: : : yaml", "{}", "repositories: null", "repositories: {}\n---\nrepositories: {}", "repositories:\n  bad: null"} {
		require.NoError(t, os.WriteFile(path, []byte(source), 0600))
		_, err := loadRepositorySettings(db, path)
		require.Error(t, err)
		stored, err := db.GetRepositorySettings()
		require.NoError(t, err)
		require.Nil(t, stored)
		preserved, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, source, string(preserved))
	}
}
