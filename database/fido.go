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

	"renop/core"
)

func (db *DB) ListFidoDevices(username string) ([]*core.FidoDevice, error) {
	if db == nil || db.SqlDB == nil || username == "" {
		return []*core.FidoDevice{}, nil
	}

	lowerName := strings.ToLower(username)
	query := `SELECT id, username, name, credential_id, public_key, attestation_type, aaguid, sign_count, created_at FROM fido_devices WHERE username = ?`
	rows, err := db.SqlDB.Query(query, lowerName)
	if err != nil {
		return nil, fmt.Errorf("failed to query fido devices for user (%s): %w", lowerName, err)
	}
	defer rows.Close()

	var devices []*core.FidoDevice
	for rows.Next() {
		dev := &core.FidoDevice{}
		if err := rows.Scan(&dev.ID, &dev.Username, &dev.Name, &dev.CredentialID, &dev.PublicKey, &dev.AttestationType, &dev.AAGUID, &dev.SignCount, &dev.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan fido device row: %w", err)
		}
		devices = append(devices, dev)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if devices == nil {
		return []*core.FidoDevice{}, nil
	}

	return devices, nil
}

func (db *DB) GetFidoDeviceByCredentialID(credentialID []byte) (*core.FidoDevice, error) {
	if db == nil || db.SqlDB == nil || len(credentialID) == 0 {
		return nil, nil
	}

	query := `SELECT id, username, name, credential_id, public_key, attestation_type, aaguid, sign_count, created_at FROM fido_devices WHERE credential_id = ?`
	row := db.SqlDB.QueryRow(query, credentialID)

	dev := &core.FidoDevice{}
	err := row.Scan(&dev.ID, &dev.Username, &dev.Name, &dev.CredentialID, &dev.PublicKey, &dev.AttestationType, &dev.AAGUID, &dev.SignCount, &dev.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query fido device by credential id: %w", err)
	}

	return dev, nil
}

func (db *DB) SaveFidoDevice(device *core.FidoDevice) error {
	if db == nil || db.SqlDB == nil || device == nil || device.Username == "" {
		return nil
	}

	lowerName := strings.ToLower(device.Username)
	query := `INSERT INTO fido_devices (id, username, name, credential_id, public_key, attestation_type, aaguid, sign_count, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := db.SqlDB.Exec(query, device.ID, lowerName, device.Name, device.CredentialID, device.PublicKey, device.AttestationType, device.AAGUID, device.SignCount, device.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to save fido device (%s): %w", device.ID, err)
	}

	return nil
}

func (db *DB) DeleteFidoDevice(username, deviceID string) error {
	if db == nil || db.SqlDB == nil || username == "" || deviceID == "" {
		return nil
	}

	lowerName := strings.ToLower(username)
	_, err := db.SqlDB.Exec(`DELETE FROM fido_devices WHERE username = ? AND id = ?`, lowerName, deviceID)
	if err != nil {
		return fmt.Errorf("failed to delete fido device (%s) for user (%s): %w", deviceID, lowerName, err)
	}

	return nil
}

func (db *DB) DeleteFidoDevicesByUsername(username string) error {
	if db == nil || db.SqlDB == nil || username == "" {
		return nil
	}

	lowerName := strings.ToLower(username)
	_, err := db.SqlDB.Exec(`DELETE FROM fido_devices WHERE username = ?`, lowerName)
	if err != nil {
		return fmt.Errorf("failed to delete fido devices for user (%s): %w", lowerName, err)
	}

	return nil
}

func (db *DB) UpdateFidoSignCount(credentialID []byte, signCount uint32) error {
	if db == nil || db.SqlDB == nil || len(credentialID) == 0 {
		return nil
	}

	query := `UPDATE fido_devices SET sign_count = ? WHERE credential_id = ?`
	_, err := db.SqlDB.Exec(query, signCount, credentialID)
	if err != nil {
		return fmt.Errorf("failed to update fido sign count: %w", err)
	}

	return nil
}

