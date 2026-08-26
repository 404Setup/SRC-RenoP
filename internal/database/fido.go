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

	"renop/internal/core"
)

func (db *DB) ListFidoDevices(username string) ([]*core.FidoDevice, error) {
	if db == nil || db.SQLDB == nil || username == "" {
		return []*core.FidoDevice{}, nil
	}
	username = SanitizeInputString(strings.TrimSpace(username), 255)
	if username == "" {
		return []*core.FidoDevice{}, nil
	}

	lowerName := strings.ToLower(username)
	query := `SELECT id, username, name, credential_id, public_key, attestation_type, aaguid, sign_count, created_at, user_present, user_verified, backup_eligible, backup_state FROM fido_devices WHERE username = ?`
	rows, err := db.Query(query, lowerName)
	if err != nil {
		return nil, fmt.Errorf("failed to query fido devices for user (%s): %w", lowerName, err)
	}
	defer rows.Close()

	devices := make([]*core.FidoDevice, 0, 4)
	for rows.Next() {
		dev := &core.FidoDevice{}
		var userPresent, userVerified, backupEligible, backupState int
		if err := rows.Scan(&dev.ID, &dev.Username, &dev.Name, &dev.CredentialID, &dev.PublicKey, &dev.AttestationType, &dev.AAGUID, &dev.SignCount, &dev.CreatedAt, &userPresent, &userVerified, &backupEligible, &backupState); err != nil {
			return nil, fmt.Errorf("failed to scan fido device row: %w", err)
		}
		dev.UserPresent = userPresent != 0
		dev.UserVerified = userVerified != 0
		dev.BackupEligible = backupEligible != 0
		dev.BackupState = backupState != 0
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
	if db == nil || db.SQLDB == nil || len(credentialID) == 0 {
		return nil, nil
	}

	query := `SELECT id, username, name, credential_id, public_key, attestation_type, aaguid, sign_count, created_at, user_present, user_verified, backup_eligible, backup_state FROM fido_devices WHERE credential_id = ?`
	row := db.QueryRow(query, credentialID)

	dev := &core.FidoDevice{}
	var userPresent, userVerified, backupEligible, backupState int
	err := row.Scan(&dev.ID, &dev.Username, &dev.Name, &dev.CredentialID, &dev.PublicKey, &dev.AttestationType, &dev.AAGUID, &dev.SignCount, &dev.CreatedAt, &userPresent, &userVerified, &backupEligible, &backupState)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query fido device by credential id: %w", err)
	}
	dev.UserPresent = userPresent != 0
	dev.UserVerified = userVerified != 0
	dev.BackupEligible = backupEligible != 0
	dev.BackupState = backupState != 0

	return dev, nil
}

func (db *DB) SaveFidoDevice(device *core.FidoDevice) error {
	if db == nil || db.SQLDB == nil || device == nil || device.Username == "" {
		return nil
	}
	device.Username = SanitizeInputString(device.Username, 255)
	device.ID = SanitizeInputString(device.ID, 255)
	device.Name = SanitizeInputString(device.Name, 255)
	if device.Username == "" || device.ID == "" {
		return nil
	}

	if device.CredentialID == nil {
		device.CredentialID = []byte{}
	}
	if device.PublicKey == nil {
		device.PublicKey = []byte{}
	}
	if device.AAGUID == nil {
		device.AAGUID = []byte{}
	}

	lowerName := strings.ToLower(device.Username)
	query := `INSERT INTO fido_devices (id, username, name, credential_id, public_key, attestation_type, aaguid, sign_count, created_at, user_present, user_verified, backup_eligible, backup_state)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	userPresentInt := 0
	if device.UserPresent {
		userPresentInt = 1
	}
	userVerifiedInt := 0
	if device.UserVerified {
		userVerifiedInt = 1
	}
	backupEligibleInt := 0
	if device.BackupEligible {
		backupEligibleInt = 1
	}
	backupStateInt := 0
	if device.BackupState {
		backupStateInt = 1
	}
	_, err := db.Exec(query, device.ID, lowerName, device.Name, device.CredentialID, device.PublicKey, device.AttestationType, device.AAGUID, device.SignCount, device.CreatedAt, userPresentInt, userVerifiedInt, backupEligibleInt, backupStateInt)
	if err != nil {
		return fmt.Errorf("failed to save fido device (%s): %w", device.ID, err)
	}

	return nil
}

func (db *DB) DeleteFidoDevice(username, deviceID string) error {
	if db == nil || db.SQLDB == nil || username == "" || deviceID == "" {
		return nil
	}
	username = SanitizeInputString(username, 255)
	deviceID = SanitizeInputString(deviceID, 255)
	if username == "" || deviceID == "" {
		return nil
	}

	lowerName := strings.ToLower(username)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin fido device deletion: %w", err)
	}
	defer tx.Rollback()
	var userID string
	if err := tx.QueryRow(`SELECT user_id FROM user_profiles WHERE username = ?`, lowerName).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if _, deleteErr := tx.Exec(`DELETE FROM fido_devices WHERE username = ? AND id = ?`,
				lowerName, deviceID); deleteErr != nil {
				return fmt.Errorf("delete orphaned fido device: %w", deleteErr)
			}
			return tx.Commit()
		}
		return fmt.Errorf("resolve fido device owner: %w", err)
	}
	if err := lockAccountLoginMethodsTx(tx, userID); err != nil {
		return fmt.Errorf("lock account login methods before fido deletion: %w", err)
	}
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM fido_devices WHERE username = ? AND id = ?`,
		lowerName, deviceID).Scan(&exists); err != nil {
		return fmt.Errorf("inspect fido device: %w", err)
	}
	if exists == 0 {
		return nil
	}
	hasAlternate, err := hasLoginWithoutFidoTx(tx, userID, lowerName, deviceID)
	if err != nil {
		return fmt.Errorf("inspect login methods before fido deletion: %w", err)
	}
	if !hasAlternate {
		return core.ErrLastLoginMethod
	}
	if _, err := tx.Exec(`DELETE FROM fido_devices WHERE username = ? AND id = ?`, lowerName, deviceID); err != nil {
		return fmt.Errorf("failed to delete fido device (%s) for user (%s): %w", deviceID, lowerName, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit fido device deletion: %w", err)
	}
	return nil
}

func (db *DB) DeleteFidoDevicesByUsername(username string) error {
	if db == nil || db.SQLDB == nil || username == "" {
		return nil
	}
	username = SanitizeInputString(username, 255)
	if username == "" {
		return nil
	}

	lowerName := strings.ToLower(username)
	_, err := db.Exec(`DELETE FROM fido_devices WHERE username = ?`, lowerName)
	if err != nil {
		return fmt.Errorf("failed to delete fido devices for user (%s): %w", lowerName, err)
	}

	return nil
}

func (db *DB) UpdateFidoSignCount(credentialID []byte, signCount uint32) error {
	if db == nil || db.SQLDB == nil || len(credentialID) == 0 {
		return nil
	}

	query := `UPDATE fido_devices SET sign_count = ? WHERE credential_id = ?`
	_, err := db.Exec(query, signCount, credentialID)
	if err != nil {
		return fmt.Errorf("failed to update fido sign count: %w", err)
	}

	return nil
}

func (db *DB) UpdateFidoDeviceState(credentialID []byte, signCount uint32, backupState bool, backupEligible bool) error {
	if db == nil || db.SQLDB == nil || len(credentialID) == 0 {
		return nil
	}

	backupStateInt := 0
	if backupState {
		backupStateInt = 1
	}
	backupEligibleInt := 0
	if backupEligible {
		backupEligibleInt = 1
	}

	query := `UPDATE fido_devices SET sign_count = ?, backup_state = ?, backup_eligible = ? WHERE credential_id = ?`
	_, err := db.Exec(query, signCount, backupStateInt, backupEligibleInt, credentialID)
	if err != nil {
		return fmt.Errorf("failed to update fido device state: %w", err)
	}

	return nil
}
