/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"strings"

	"renop/internal/core"
)

func npmLockTarget(repository, name, version string) core.ResourceLockTarget {
	return core.ResourceLockTarget{Format: "npm", Repository: repository, Name: name, Version: version}
}

func ensureNPMTagChangeTx(tx *Tx, repository, packageName, tag, version string) error {
	tag = strings.ToLower(strings.TrimSpace(tag))
	previous, err := npmTagVersionTx(tx, repository, packageName, tag)
	if err != nil || previous == version {
		return err
	}
	for _, target := range []string{previous, version} {
		if target != "" {
			if err := ensureResourceMutableQuery(tx.QueryRow, npmLockTarget(repository, packageName, target), false); err != nil {
				return err
			}
		}
	}
	return nil
}

// Full packuments repeat unchanged version metadata; only actual changes may violate a version lock.
func ensureNPMPackumentLocksTx(tx *Tx, repository, packageName string, deprecations, tags map[string]string) error {
	rows, err := tx.Query(`SELECT v.version, v.deprecated FROM npm_versions v
		WHERE v.repository = ? AND v.package_name = ? AND EXISTS (
		SELECT 1 FROM resource_locks l WHERE l.package_id = ? AND `+
		resourceLockVersionColumn("npm", "l.version")+` = `+resourceLockVersionColumn("npm", "v.version")+`)`,
		repository, packageName, packageDeprecationID("npm", repository, packageName))
	if err != nil {
		return err
	}
	locked := make(map[string]bool)
	for rows.Next() {
		var version, deprecated string
		if err := rows.Scan(&version, &deprecated); err != nil {
			_ = rows.Close()
			return err
		}
		locked[version] = true
		if next, exists := deprecations[version]; exists && strings.TrimSpace(next) != deprecated {
			_ = rows.Close()
			return core.ErrResourceLocked
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil || len(locked) == 0 {
		return err
	}
	rows, err = tx.Query(`SELECT tag, version FROM npm_dist_tags WHERE repository = ? AND package_name = ?`, repository, packageName)
	if err != nil {
		return err
	}
	defer rows.Close()
	previous := make(map[string]string)
	for rows.Next() {
		var tag, version string
		if err := rows.Scan(&tag, &version); err != nil {
			return err
		}
		previous[tag] = version
		if locked[version] && tags[tag] != version {
			return core.ErrResourceLocked
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for tag, version := range tags {
		if locked[version] && previous[tag] != version {
			return core.ErrResourceLocked
		}
	}
	return nil
}

func npmVersionVisibilitySQL(resourceAlias, versionColumn string) string {
	return `(own.user_id IS NOT NULL OR stm.user_id IS NOT NULL OR NOT EXISTS (
		SELECT 1 FROM resource_locks l WHERE l.format = 'npm' AND l.mode = 'read'
		AND l.repository = ` + resourceAlias + `.repository AND l.resource_name = ` + resourceAlias + `.package_name
		AND ` + resourceLockVersionColumn("npm", "l.version") + ` = ` + resourceLockVersionColumn("npm", versionColumn) + `))`
}
