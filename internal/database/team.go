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
	"time"

	"renop/internal/core"
)

type teamRemovalSpec struct {
	format              string
	table               string
	resourceColumn      string
	manageLevel         int
	ownerLevel          int
	invalidBatch        error
	invalidName         error
	resourceNotFound    error
	permissionDenied    error
	ownerCannotLeave    error
	lastOwner           error
	lock                func(*Tx, string, string) error
	effectivePermission func(*Tx, string, string, string) (int, bool, error)
}

func (db *DB) removeTeamMembers(repository, resource, actor string, usernames []string, sanitizeUsername func(string) string, spec *teamRemovalSpec) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(usernames) == 0 || len(usernames) > 20 {
		return spec.invalidBatch
	}
	unique := make([]string, 0, len(usernames))
	userIDs := make(map[string]string, len(usernames))
	seen := make(map[string]struct{}, len(usernames))
	for _, candidate := range usernames {
		username := sanitizeUsername(candidate)
		if username == "" {
			return spec.invalidName
		}
		if _, exists := seen[username]; exists {
			continue
		}
		seen[username] = struct{}{}
		userID, err := db.userIDForUsername(username)
		if err != nil {
			return spec.resourceNotFound
		}
		userIDs[username] = userID
		unique = append(unique, username)
	}
	actorID := ""
	if actor != "" {
		var err error
		actorID, err = db.userIDForUsername(actor)
		if err != nil {
			return spec.permissionDenied
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin %s member removal: %w", spec.format, err)
	}
	defer tx.Rollback()
	if err := spec.lock(tx, repository, resource); err != nil {
		return err
	}
	predicate := "repository = ? AND " + spec.resourceColumn + " = ?"
	if actor != "" {
		var actorLevel int
		if spec.effectivePermission != nil {
			level, member, permissionErr := spec.effectivePermission(tx, repository, resource, actorID)
			if errors.Is(permissionErr, spec.resourceNotFound) {
				return spec.resourceNotFound
			}
			if permissionErr != nil {
				return fmt.Errorf("inspect %s effective removal permission: %w", spec.format, permissionErr)
			}
			if !member {
				return spec.permissionDenied
			}
			actorLevel = level
		} else {
			if err := tx.QueryRow("SELECT permission_level FROM "+spec.table+" WHERE "+predicate+" AND user_id = ?",
				repository, resource, actorID).Scan(&actorLevel); errors.Is(err, sql.ErrNoRows) {
				return spec.permissionDenied
			} else if err != nil {
				return fmt.Errorf("inspect %s removal actor: %w", spec.format, err)
			}
		}
		for _, username := range unique {
			if username != actor && actorLevel < spec.manageLevel {
				return spec.permissionDenied
			}
			if username == actor && actorLevel == spec.ownerLevel {
				return spec.ownerCannotLeave
			}
		}
	}
	ownersRemoved := 0
	removedAt := time.Now().UnixMilli()
	for _, username := range unique {
		var current int
		if err := tx.QueryRow("SELECT permission_level FROM "+spec.table+" WHERE "+predicate+" AND user_id = ?",
			repository, resource, userIDs[username]).Scan(&current); errors.Is(err, sql.ErrNoRows) {
			return spec.resourceNotFound
		} else if err != nil {
			return fmt.Errorf("inspect %s member removal: %w", spec.format, err)
		}
		if current == spec.ownerLevel {
			ownersRemoved++
		}
	}
	if ownersRemoved > 0 {
		var ownerCount int
		if err := tx.QueryRow("SELECT COUNT(*) FROM "+spec.table+" WHERE "+predicate+" AND permission_level = ?",
			repository, resource, spec.ownerLevel).Scan(&ownerCount); err != nil {
			return fmt.Errorf("count %s L4 members: %w", spec.format, err)
		}
		if ownersRemoved >= ownerCount {
			return spec.lastOwner
		}
	}
	for _, username := range unique {
		if _, err := tx.Exec("DELETE FROM "+spec.table+" WHERE "+predicate+" AND user_id = ?",
			repository, resource, userIDs[username]); err != nil {
			return fmt.Errorf("remove %s member: %w", spec.format, err)
		}
		if actor == "" || username != actor {
			if err := insertTeamRemovalMessage(tx, username, strings.ToLower(spec.format),
				repository, resource, removedAt); err != nil {
				return fmt.Errorf("notify removed %s member: %w", spec.format, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s member removal: %w", spec.format, err)
	}
	return nil
}
