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

func (db *DB) attachSuperTeamLocks(teams []*core.SuperTeam) error {
	if len(teams) == 0 {
		return nil
	}
	if len(teams) > 100 {
		return core.ErrResourceLockInvalid
	}
	byPrefix := make(map[string]*core.SuperTeam, len(teams))
	args := make([]any, 0, len(teams))
	for _, team := range teams {
		byPrefix[team.Prefix] = team
		args = append(args, team.Prefix)
	}
	rows, err := db.Query(`SELECT resource_name, source, mode, reason, reason_text, locked_at FROM resource_locks
		WHERE format = 'superteam' AND repository = '' AND resource_name IN (`+
		strings.TrimSuffix(strings.Repeat("?,", len(teams)), ",")+`) ORDER BY resource_name, source`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		lock := &core.ResourceLock{ResourceLockTarget: core.ResourceLockTarget{Format: "superteam"}}
		if err := rows.Scan(&lock.Name, &lock.Source, &lock.Mode, &lock.Reason, &lock.ReasonText, &lock.LockedAt); err != nil {
			return err
		}
		team := byPrefix[lock.Name]
		team.Locks = append(team.Locks, lock)
	}
	return rows.Err()
}

func superTeamReadCondition(prefixColumn string) string {
	return `NOT EXISTS (SELECT 1 FROM resource_locks l WHERE l.format = 'superteam'
		AND l.repository = '' AND l.resource_name = ` + prefixColumn + ` AND l.mode = 'read')`
}
