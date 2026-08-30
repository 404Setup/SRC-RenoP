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

	"renop/internal/core"
)

func normalizeOptionalSuperTeamPrefix(prefix string) (string, error) {
	if prefix == "" {
		return "", nil
	}
	normalized, valid := sanitizeSuperTeamPrefix(prefix)
	if !valid {
		return "", core.ErrSuperTeamBindingMismatch
	}
	return normalized, nil
}

func superTeamRoleTx(tx *Tx, prefix, userID string) (int, bool, error) {
	if prefix == "" || userID == "" {
		return 0, false, nil
	}
	var role int
	err := tx.QueryRow(`SELECT role_level FROM super_team_members WHERE team_prefix = ? AND user_id = ?`,
		prefix, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("inspect global team binding permission: %w", err)
	}
	return role, true, nil
}

func requireSuperTeamRoleTx(tx *Tx, prefix, userID string, required int) error {
	if prefix == "" {
		return nil
	}
	role, member, err := superTeamRoleTx(tx, prefix, userID)
	if err != nil {
		return err
	}
	if !member || role < required {
		return core.ErrSuperTeamBindingPermission
	}
	return nil
}

func effectiveBoundPermission(explicitLevel int, explicitMember bool, superTeamRole int, superTeamMember bool) (int, bool) {
	level := explicitLevel
	member := explicitMember
	if superTeamMember {
		mapped := core.SuperTeamPackagePermission(superTeamRole)
		if !member || mapped > level {
			level = mapped
		}
		member = true
	}
	if level < 0 {
		level = 0
	}
	return level, member
}
