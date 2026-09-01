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
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"renop/internal/config"
	"renop/internal/core"
)

const maxPackageDeprecationKeyBytes = 8192

func normalizePackageDeprecation(format, repository, packageKey string) (string, string, string, bool) {
	format = strings.ToLower(strings.TrimSpace(format))
	repository = strings.ToLower(strings.TrimSpace(repository))
	packageKey = strings.TrimSpace(packageKey)
	if repository == "" || len(repository) > 255 || packageKey == "" || len(packageKey) > maxPackageDeprecationKeyBytes {
		return "", "", "", false
	}
	switch format {
	case config.RepositoryFormatCargo:
		packageKey = strings.ReplaceAll(strings.ToLower(packageKey), "_", "-")
		return format, repository, packageKey, true
	case config.RepositoryFormatNPM, config.RepositoryFormatDocker:
		packageKey = strings.ToLower(packageKey)
		return format, repository, packageKey, true
	case config.RepositoryFormatMaven:
		return format, repository, packageKey, true
	default:
		return "", "", "", false
	}
}

func packageDeprecationID(format, repository, packageKey string) string {
	canonical := fmt.Sprintf("%d:%s%d:%s%d:%s", len(format), format,
		len(repository), repository, len(packageKey), packageKey)
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:])
}

func packageReviewResourceType(format string) string {
	switch format {
	case config.RepositoryFormatCargo:
		return core.ReviewResourceCargoPackage
	case config.RepositoryFormatNPM:
		return core.ReviewResourceNPMPackage
	case config.RepositoryFormatDocker:
		return core.ReviewResourceDockerImage
	case config.RepositoryFormatMaven:
		return core.ReviewResourceMavenArtifact
	default:
		return ""
	}
}

func hasPendingPackageReviewTx(tx *Tx, resourceType, repository, packageKey string) (bool, error) {
	rows, err := tx.Query(`SELECT kind, resource_key FROM review_tasks WHERE resource_type = ?
		AND repository = ? AND status = ?`, resourceType, repository, core.ReviewStatusPending)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, storedKey string
		if err := rows.Scan(&kind, &storedKey); err != nil {
			return false, err
		}
		if kind == core.ReviewKindPublication {
			decodedKey, _, valid := decodePublicationReviewKey(storedKey)
			if valid && decodedKey == packageKey {
				return true, nil
			}
		} else if storedKey == packageKey {
			return true, nil
		}
	}
	return false, rows.Err()
}

// IsPackageDeprecated reports whether one exact package is permanently frozen.
func (db *DB) IsPackageDeprecated(format, repository, packageKey string) (bool, error) {
	format, repository, packageKey, valid := normalizePackageDeprecation(format, repository, packageKey)
	if !valid {
		return false, core.ErrPackageDeprecationInvalid
	}
	var storedFormat, storedRepository, storedKey string
	err := db.QueryRow(`SELECT format, repository, package_key FROM package_deprecations WHERE id = ?`,
		packageDeprecationID(format, repository, packageKey)).Scan(&storedFormat, &storedRepository, &storedKey)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect package deprecation: %w", err)
	}
	if storedFormat != format || storedRepository != repository || storedKey != packageKey {
		return false, core.ErrPackageDeprecationConflict
	}
	return true, nil
}

// EnsurePackageMutable rejects every package mutation after permanent deprecation.
func (db *DB) EnsurePackageMutable(format, repository, packageKey string) error {
	deprecated, err := db.IsPackageDeprecated(format, repository, packageKey)
	if err != nil {
		return err
	}
	if deprecated {
		return core.ErrPackageDeprecated
	}
	return nil
}

// DeprecatePackage permanently freezes one exact package and cancels its outstanding team invitations.
func (db *DB) DeprecatePackage(format, repository, packageKey string, deprecatedAt int64) error {
	format, repository, packageKey, valid := normalizePackageDeprecation(format, repository, packageKey)
	if !valid || deprecatedAt <= 0 {
		return core.ErrPackageDeprecationInvalid
	}
	id := packageDeprecationID(format, repository, packageKey)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin package deprecation: %w", err)
	}
	defer tx.Rollback()
	var storedFormat, storedRepository, storedKey string
	err = tx.QueryRow(`SELECT format, repository, package_key FROM package_deprecations WHERE id = ?`, id).
		Scan(&storedFormat, &storedRepository, &storedKey)
	if err == nil {
		if storedFormat == format && storedRepository == repository && storedKey == packageKey {
			return core.ErrPackageDeprecated
		}
		return core.ErrPackageDeprecationConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect package deprecation transaction: %w", err)
	}
	pending, err := hasPendingPackageReviewTx(tx, packageReviewResourceType(format), repository, packageKey)
	if err != nil {
		return fmt.Errorf("inspect pending package reviews: %w", err)
	}
	if pending {
		return core.ErrPackageDeprecationPending
	}
	if _, err := tx.Exec(`INSERT INTO package_deprecations
		(id, format, repository, package_key, deprecated_at) VALUES (?, ?, ?, ?, ?)`,
		id, format, repository, packageKey, deprecatedAt); err != nil {
		if uniqueConstraintError(err) {
			return core.ErrPackageDeprecated
		}
		return fmt.Errorf("store package deprecation: %w", err)
	}
	err = nil
	switch format {
	case config.RepositoryFormatCargo:
		err = cancelCargoInvitations(tx, `repository = ? AND normalized_name = ?`,
			[]any{repository, packageKey}, deprecatedAt)
	case config.RepositoryFormatNPM:
		err = cancelNPMInvitations(tx, `repository = ? AND package_name = ?`,
			[]any{repository, packageKey}, deprecatedAt)
	case config.RepositoryFormatDocker:
		err = cancelDockerInvitations(tx, `repository = ? AND image_name = ?`,
			[]any{repository, packageKey}, deprecatedAt)
	}
	if err != nil {
		return fmt.Errorf("cancel package invitations during deprecation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit package deprecation: %w", err)
	}
	return nil
}
