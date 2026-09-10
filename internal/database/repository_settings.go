/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/goccy/go-json"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/utils"
)

// MaxRepositorySettingsSize bounds the complete persisted repository snapshot.
const MaxRepositorySettingsSize = (16 << 20) - 1

func encodeRepositorySettings(settings config.MavenSettings) ([]byte, error) {
	names := make(map[string]bool, len(settings.Repositories))
	for name, repo := range settings.Repositories {
		if !utils.IsValidRepositoryName(name) || repo == nil || repo.Name != name ||
			!config.IsSupportedRepositoryFormat(repo.ConfiguredFormat()) || names[strings.ToLower(name)] {
			return nil, fmt.Errorf("invalid repository definition %q", name)
		}
		names[strings.ToLower(name)] = true
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxRepositorySettingsSize {
		return nil, errors.New("repository settings exceed the size limit")
	}
	return data, nil
}

// GetRepositorySettings returns nil until the initial snapshot is committed.
func (db *DB) GetRepositorySettings() (*config.MavenSettings, error) {
	if db == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	var data string
	err := db.QueryRow("SELECT payload FROM repository_settings WHERE id = 1").Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > MaxRepositorySettingsSize {
		return nil, errors.New("repository settings exceed the size limit")
	}
	var settings *config.MavenSettings
	if err := json.Unmarshal([]byte(data), &settings); err != nil {
		return nil, fmt.Errorf("decode repository settings: %w", err)
	}
	if settings == nil {
		return nil, errors.New("repository settings are null")
	}
	if _, err := encodeRepositorySettings(*settings); err != nil {
		return nil, err
	}
	return settings, nil
}

// InitializeRepositorySettings imports a snapshot only if the database has none.
func (db *DB) InitializeRepositorySettings(settings config.MavenSettings) (*config.MavenSettings, error) {
	data, err := encodeRepositorySettings(settings)
	if err != nil {
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id int
	err = tx.QueryRow("SELECT id FROM repository_settings WHERE id = 1").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec("INSERT INTO repository_settings (id, payload) VALUES (1, ?)", string(data))
		if uniqueConstraintError(err) {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				return nil, rollbackErr
			}
			return db.GetRepositorySettings()
		}
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return db.GetRepositorySettings()
}

// SaveRepositorySettings atomically replaces the complete snapshot, including an empty repository set.
func (db *DB) SaveRepositorySettings(settings config.MavenSettings) error {
	if db == nil {
		return core.ErrDatabaseUnavailable
	}
	data, err := encodeRepositorySettings(settings)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM repository_settings WHERE id = 1"); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO repository_settings (id, payload) VALUES (1, ?)", string(data)); err != nil {
		return err
	}
	return tx.Commit()
}
