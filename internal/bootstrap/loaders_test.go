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

		cfg := LoadConfig(cfgPath)
		assert.Equal(t, uint16(9090), cfg.Server.Port)
	})

	t.Run("missing config file creates default", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "missing.yaml")

		cfg := LoadConfig(cfgPath)
		assert.NotNil(t, cfg)
		assert.FileExists(t, cfgPath)
	})

	t.Run("invalid config file falls back to default", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "invalid.yaml")
		require.NoError(t, os.WriteFile(cfgPath, []byte("invalid: : : yaml"), 0644))

		cfg := LoadConfig(cfgPath)
		assert.NotNil(t, cfg)
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

		settings := LoadMaven(mavenPath)
		assert.NotEmpty(t, settings)
	})

	t.Run("missing maven file returns default settings", func(t *testing.T) {
		dir := t.TempDir()
		mavenPath := filepath.Join(dir, "missing.yaml")

		settings := LoadMaven(mavenPath)
		assert.NotEmpty(t, settings)
	})

	t.Run("invalid maven file returns default settings", func(t *testing.T) {
		dir := t.TempDir()
		mavenPath := filepath.Join(dir, "invalid.yaml")
		require.NoError(t, os.WriteFile(mavenPath, []byte("invalid: : : yaml"), 0644))

		settings := LoadMaven(mavenPath)
		assert.NotEmpty(t, settings)
	})
}
