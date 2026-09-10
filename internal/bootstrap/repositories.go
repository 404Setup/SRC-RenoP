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
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/google/uuid"
	"go.yaml.in/yaml/v3"

	"renop/internal/config"
	"renop/internal/database"
	"renop/internal/utils"
)

func loadRepositorySettings(db *database.DB, legacyPath string) (config.MavenSettings, error) {
	stored, err := db.GetRepositorySettings()
	if err != nil {
		return config.MavenSettings{}, err
	}
	if stored != nil {
		return *stored, nil
	}
	settings, err := readLegacyRepositories(legacyPath)
	imported := err == nil
	if errors.Is(err, os.ErrNotExist) {
		settings = config.DefaultMavenSettings()
	} else if err != nil {
		return config.MavenSettings{}, err
	}
	stored, err = db.InitializeRepositorySettings(settings)
	if err != nil {
		return config.MavenSettings{}, err
	}
	if imported {
		archive := legacyPath + ".migrated." + uuid.NewString()
		if err := os.Rename(legacyPath, archive); err != nil {
			log.Printf("Repository settings are committed to the database; could not archive the legacy file: %v", err)
		} else {
			log.Printf("Repository settings migrated to the database; legacy file archived at %s", archive)
		}
	}
	return *stored, nil
}

func readLegacyRepositories(path string) (config.MavenSettings, error) {
	file, err := os.Open(path)
	if err != nil {
		return config.MavenSettings{}, err
	}
	defer file.Close()
	data, err := utils.ReadAllLimited(file, database.MaxRepositorySettingsSize)
	if err != nil {
		return config.MavenSettings{}, err
	}
	// Keep a missing mapping distinct from an intentionally empty repository set.
	var envelope struct {
		Repositories map[string]*config.Repository `yaml:"repositories"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&envelope); err != nil {
		return config.MavenSettings{}, fmt.Errorf("decode legacy repositories: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return config.MavenSettings{}, errors.New("legacy repositories must contain one YAML document")
	}
	if envelope.Repositories == nil {
		return config.MavenSettings{}, errors.New("legacy repositories must contain a repositories mapping")
	}
	for name, repo := range envelope.Repositories {
		if repo != nil && repo.Name == "" {
			repo.Name = name
		}
	}
	settings := config.MavenSettings{Repositories: envelope.Repositories}
	if err := settings.Normalize(); err != nil {
		return config.MavenSettings{}, err
	}
	return settings, nil
}
