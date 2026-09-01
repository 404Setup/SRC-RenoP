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
	"sync"
	"time"

	"renop/internal/core"
)

const (
	maxSuperTeamMutationMembers = 20
	maxSuperTeamLimit           = 1000
)

var superTeamMutationLock sync.Mutex

func sanitizeSuperTeamPrefix(prefix string) (string, bool) {
	prefix = SanitizeInputString(strings.TrimSpace(prefix), core.MaxSuperTeamPrefixLength)
	return core.NormalizeSuperTeamPrefix(prefix)
}

func normalizeSuperTeam(team *core.SuperTeam) error {
	if team == nil {
		return errors.New("global team is missing")
	}
	prefix, valid := sanitizeSuperTeamPrefix(team.Prefix)
	if !valid {
		return errors.New("global team prefix is invalid")
	}
	name, valid := core.NormalizeSuperTeamText(team.Name, core.MaxSuperTeamNameRunes, false)
	if !valid {
		return errors.New("global team name is invalid")
	}
	description, valid := core.NormalizeSuperTeamText(team.Description, core.MaxSuperTeamDescription, true)
	if !valid {
		return errors.New("global team description is invalid")
	}
	team.Prefix = prefix
	team.Name = name
	team.Description = description
	return nil
}

func effectiveSuperTeamLimitsTx(tx *Tx, userID string, globalCreateLimit, globalJoinLimit int) (
	createLimit, joinLimit int, createInherited, joinInherited bool, err error,
) {
	if globalCreateLimit < 1 || globalCreateLimit > maxSuperTeamLimit ||
		globalJoinLimit < 1 || globalJoinLimit > maxSuperTeamLimit {
		return 0, 0, false, false, errors.New("global team limits are invalid")
	}
	createLimit = globalCreateLimit
	joinLimit = globalJoinLimit
	createInherited = true
	joinInherited = true
	var createOverride, joinOverride int
	err = tx.QueryRow(`SELECT create_limit, join_limit FROM user_super_team_limits WHERE user_id = ?`, userID).
		Scan(&createOverride, &joinOverride)
	if errors.Is(err, sql.ErrNoRows) {
		return createLimit, joinLimit, createInherited, joinInherited, nil
	}
	if err != nil {
		return 0, 0, false, false, fmt.Errorf("load global team limit override: %w", err)
	}
	if createOverride >= 0 {
		createLimit = createOverride
		createInherited = false
	}
	if joinOverride >= 0 {
		joinLimit = joinOverride
		joinInherited = false
	}
	return createLimit, joinLimit, createInherited, joinInherited, nil
}

func superTeamUsageTx(tx *Tx, userID string) (created, joined int, err error) {
	if err := tx.QueryRow(`SELECT COUNT(*) FROM super_teams WHERE created_by = ?`, userID).Scan(&created); err != nil {
		return 0, 0, fmt.Errorf("count created global teams: %w", err)
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM super_team_members WHERE user_id = ?`, userID).Scan(&joined); err != nil {
		return 0, 0, fmt.Errorf("count joined global teams: %w", err)
	}
	return created, joined, nil
}

// GetSuperTeamLimitStatus returns effective global-team limits and current usage for an account.
func (db *DB) GetSuperTeamLimitStatus(username string, globalCreateLimit, globalJoinLimit int) (*core.SuperTeamLimitStatus, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	userID, err := db.userIDForExistingAccount(username)
	if err != nil {
		return nil, core.ErrUserProfileNotFound
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin global team limit lookup: %w", err)
	}
	defer tx.Rollback()
	createLimit, joinLimit, createInherited, joinInherited, err :=
		effectiveSuperTeamLimitsTx(tx, userID, globalCreateLimit, globalJoinLimit)
	if err != nil {
		return nil, err
	}
	created, joined, err := superTeamUsageTx(tx, userID)
	if err != nil {
		return nil, err
	}
	return &core.SuperTeamLimitStatus{
		CreateLimit: createLimit, JoinLimit: joinLimit, CreatedCount: created, JoinedCount: joined,
		CreateLimitInherited: createInherited, JoinLimitInherited: joinInherited,
	}, nil
}

// SetSuperTeamLimitOverride stores nullable per-account overrides for global team limits.
func (db *DB) SetSuperTeamLimitOverride(username string, createLimit, joinLimit *int, updatedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if updatedAt <= 0 || createLimit != nil && (*createLimit < 0 || *createLimit > maxSuperTeamLimit) ||
		joinLimit != nil && (*joinLimit < 0 || *joinLimit > maxSuperTeamLimit) {
		return errors.New("global team limit override is invalid")
	}
	userID, err := db.userIDForExistingAccount(username)
	if err != nil {
		return core.ErrUserProfileNotFound
	}
	superTeamMutationLock.Lock()
	defer superTeamMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin global team limit update: %w", err)
	}
	defer tx.Rollback()
	if createLimit == nil && joinLimit == nil {
		if _, err := tx.Exec(`DELETE FROM user_super_team_limits WHERE user_id = ?`, userID); err != nil {
			return fmt.Errorf("clear global team limit override: %w", err)
		}
	} else {
		createValue, joinValue := -1, -1
		if createLimit != nil {
			createValue = *createLimit
		}
		if joinLimit != nil {
			joinValue = *joinLimit
		}
		var exists int
		err := tx.QueryRow(`SELECT 1 FROM user_super_team_limits WHERE user_id = ?`, userID).Scan(&exists)
		switch {
		case err == nil:
			if _, err := tx.Exec(`UPDATE user_super_team_limits SET create_limit = ?, join_limit = ?, updated_at = ?
				WHERE user_id = ?`, createValue, joinValue, updatedAt, userID); err != nil {
				return fmt.Errorf("update global team limit override: %w", err)
			}
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.Exec(`INSERT INTO user_super_team_limits
				(user_id, create_limit, join_limit, updated_at) VALUES (?, ?, ?, ?)`,
				userID, createValue, joinValue, updatedAt); err != nil {
				return fmt.Errorf("create global team limit override: %w", err)
			}
		default:
			return fmt.Errorf("inspect global team limit override: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit global team limit update: %w", err)
	}
	return nil
}

// CreateSuperTeam creates a global team and its initial T4 owner atomically.
func (db *DB) CreateSuperTeam(team *core.SuperTeam, owner string, globalCreateLimit, globalJoinLimit int) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if err := normalizeSuperTeam(team); err != nil || team.CreatedAt <= 0 {
		return core.ErrSuperTeamPermissionDenied
	}
	ownerID, err := db.userIDForExistingAccount(owner)
	if err != nil {
		return core.ErrSuperTeamPermissionDenied
	}
	superTeamMutationLock.Lock()
	defer superTeamMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin global team creation: %w", err)
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM super_teams WHERE prefix = ?`, team.Prefix).Scan(&exists); err == nil {
		return core.ErrSuperTeamExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect global team prefix: %w", err)
	}
	createLimit, joinLimit, _, _, err := effectiveSuperTeamLimitsTx(tx, ownerID, globalCreateLimit, globalJoinLimit)
	if err != nil {
		return err
	}
	created, joined, err := superTeamUsageTx(tx, ownerID)
	if err != nil {
		return err
	}
	if created >= createLimit {
		return core.ErrSuperTeamCreateLimit
	}
	if joined >= joinLimit {
		return core.ErrSuperTeamJoinLimit
	}
	if _, err := tx.Exec(`INSERT INTO super_teams
		(prefix, name, description, created_by, created_by_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		team.Prefix, team.Name, team.Description, ownerID, strings.ToLower(strings.TrimSpace(owner)),
		team.CreatedAt, team.CreatedAt); err != nil {
		return fmt.Errorf("create global team: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO super_team_members
		(team_prefix, user_id, role_level, added_at) VALUES (?, ?, ?, ?)`,
		team.Prefix, ownerID, core.SuperTeamRoleOwner, team.CreatedAt); err != nil {
		return fmt.Errorf("create global team owner: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit global team creation: %w", err)
	}
	team.CreatedBy = strings.ToLower(strings.TrimSpace(owner))
	team.RoleLevel = core.SuperTeamRoleOwner
	team.MemberCount = 1
	team.UpdatedAt = team.CreatedAt
	return nil
}

const superTeamSelectColumns = `t.prefix, t.name, t.description, COALESCE(creator.username, t.created_by_name),
	COALESCE(member.role_level, 0), COALESCE(member_counts.member_count, 0), t.created_at, t.updated_at`

func scanSuperTeam(scanner row) (*core.SuperTeam, error) {
	team := &core.SuperTeam{}
	if err := scanner.Scan(&team.Prefix, &team.Name, &team.Description, &team.CreatedBy,
		&team.RoleLevel, &team.MemberCount, &team.CreatedAt, &team.UpdatedAt); err != nil {
		return nil, err
	}
	return team, nil
}

// ListSuperTeams returns a bounded page of teams visible to one user.
func (db *DB) ListSuperTeams(username string, administrator bool, limit, offset int) ([]*core.SuperTeam, int, error) {
	if db == nil || db.SQLDB == nil {
		return nil, 0, core.ErrDatabaseUnavailable
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, errors.New("global team page is invalid")
	}
	userID, err := db.userIDForExistingAccount(username)
	if err != nil {
		return nil, 0, core.ErrUserProfileNotFound
	}
	visibility := `member.user_id IS NOT NULL`
	if administrator {
		visibility = `1 = 1`
	}
	baseJoin := ` FROM super_teams t
		LEFT JOIN user_profiles creator ON creator.user_id = t.created_by
		LEFT JOIN super_team_members member ON member.team_prefix = t.prefix AND member.user_id = ?
		LEFT JOIN (SELECT team_prefix, COUNT(*) AS member_count FROM super_team_members GROUP BY team_prefix) member_counts
			ON member_counts.team_prefix = t.prefix WHERE ` + visibility
	var total int
	if err := db.QueryRow(`SELECT COUNT(*)`+baseJoin, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count visible global teams: %w", err)
	}
	rows, err := db.Query(`SELECT `+superTeamSelectColumns+baseJoin+`
		ORDER BY t.prefix LIMIT ? OFFSET ?`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list global teams: %w", err)
	}
	defer rows.Close()
	teams := make([]*core.SuperTeam, 0, min(limit, total))
	for rows.Next() {
		team := &core.SuperTeam{}
		if err := rows.Scan(&team.Prefix, &team.Name, &team.Description, &team.CreatedBy,
			&team.RoleLevel, &team.MemberCount, &team.CreatedAt, &team.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan global team: %w", err)
		}
		teams = append(teams, team)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate global teams: %w", err)
	}
	return teams, total, nil
}

// ListManageableSuperTeams returns teams where an account holds at least the requested T-level.
func (db *DB) ListManageableSuperTeams(username string, minimumRole, limit, offset int) ([]*core.SuperTeam, int, error) {
	if db == nil || db.SQLDB == nil {
		return nil, 0, core.ErrDatabaseUnavailable
	}
	if minimumRole < core.SuperTeamRoleRead || minimumRole > core.SuperTeamRoleOwner ||
		limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, errors.New("manageable global team page is invalid")
	}
	userID, err := db.userIDForExistingAccount(username)
	if err != nil {
		return nil, 0, core.ErrUserProfileNotFound
	}
	baseJoin := ` FROM super_teams t
		LEFT JOIN user_profiles creator ON creator.user_id = t.created_by
		JOIN super_team_members member ON member.team_prefix = t.prefix AND member.user_id = ?
		LEFT JOIN (SELECT team_prefix, COUNT(*) AS member_count FROM super_team_members GROUP BY team_prefix) member_counts
			ON member_counts.team_prefix = t.prefix WHERE member.role_level >= ?`
	var total int
	if err := db.QueryRow(`SELECT COUNT(*)`+baseJoin, userID, minimumRole).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count manageable global teams: %w", err)
	}
	rows, err := db.Query(`SELECT `+superTeamSelectColumns+baseJoin+`
		ORDER BY t.prefix LIMIT ? OFFSET ?`, userID, minimumRole, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list manageable global teams: %w", err)
	}
	defer rows.Close()
	teams := make([]*core.SuperTeam, 0, min(limit, total))
	for rows.Next() {
		team, scanErr := scanSuperTeam(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan manageable global team: %w", scanErr)
		}
		teams = append(teams, team)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate manageable global teams: %w", err)
	}
	return teams, total, nil
}

// GetSuperTeamRole returns the caller's exact T-level in one global team.
func (db *DB) GetSuperTeamRole(prefix, username string) (int, error) {
	if db == nil || db.SQLDB == nil {
		return 0, core.ErrDatabaseUnavailable
	}
	prefix, valid := sanitizeSuperTeamPrefix(prefix)
	if !valid {
		return 0, core.ErrSuperTeamNotFound
	}
	userID, err := db.userIDForExistingAccount(username)
	if err != nil {
		return 0, core.ErrSuperTeamPermissionDenied
	}
	var role int
	err = db.QueryRow(`SELECT role_level FROM super_team_members WHERE team_prefix = ? AND user_id = ?`,
		prefix, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, core.ErrSuperTeamPermissionDenied
	}
	if err != nil {
		return 0, fmt.Errorf("get global team role: %w", err)
	}
	return role, nil
}

// GetSuperTeamDetails returns one visible global team and its members.
func (db *DB) GetSuperTeamDetails(prefix, username string, administrator bool) (*core.SuperTeamDetails, error) {
	return db.getSuperTeamDetails(prefix, username, administrator, false)
}

// GetPublicSuperTeamDetails returns one global team and its public member list.
func (db *DB) GetPublicSuperTeamDetails(prefix, username string, administrator bool) (*core.SuperTeamDetails, error) {
	return db.getSuperTeamDetails(prefix, username, administrator, true)
}

func (db *DB) getSuperTeamDetails(prefix, username string, administrator, public bool) (*core.SuperTeamDetails, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	prefix, valid := sanitizeSuperTeamPrefix(prefix)
	if !valid {
		return nil, core.ErrSuperTeamNotFound
	}
	userID := ""
	if username = strings.TrimSpace(username); username != "" && !strings.EqualFold(username, "guest") {
		var err error
		userID, err = db.userIDForExistingAccount(username)
		if err != nil && !public {
			return nil, core.ErrUserProfileNotFound
		}
	} else if !public {
		return nil, core.ErrUserProfileNotFound
	}
	team, err := scanSuperTeam(db.QueryRow(`SELECT `+superTeamSelectColumns+`
		FROM super_teams t
		LEFT JOIN user_profiles creator ON creator.user_id = t.created_by
		LEFT JOIN super_team_members member ON member.team_prefix = t.prefix AND member.user_id = ?
		LEFT JOIN (SELECT team_prefix, COUNT(*) AS member_count FROM super_team_members GROUP BY team_prefix) member_counts
			ON member_counts.team_prefix = t.prefix
		WHERE t.prefix = ? AND (? = 1 OR member.user_id IS NOT NULL)`, userID, prefix, boolInt(administrator || public)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrSuperTeamNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get global team: %w", err)
	}
	rows, err := db.Query(`SELECT m.user_id, p.username, m.role_level, m.added_at
		FROM super_team_members m JOIN user_profiles p ON p.user_id = m.user_id
		WHERE m.team_prefix = ? ORDER BY m.role_level DESC, p.username`, prefix)
	if err != nil {
		return nil, fmt.Errorf("list global team members: %w", err)
	}
	defer rows.Close()
	members := make([]*core.SuperTeamMember, 0, team.MemberCount)
	for rows.Next() {
		member := &core.SuperTeamMember{}
		if err := rows.Scan(&member.UserID, &member.Username, &member.Level, &member.AddedAt); err != nil {
			return nil, fmt.Errorf("scan global team member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate global team members: %w", err)
	}
	return &core.SuperTeamDetails{Team: team, Members: members, Administrator: administrator}, nil
}

// ListSuperTeamReviewerNames returns active T3/T4 members eligible to decide team reviews.
func (db *DB) ListSuperTeamReviewerNames(prefix string) ([]string, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	prefix, valid := sanitizeSuperTeamPrefix(prefix)
	if !valid {
		return nil, core.ErrSuperTeamNotFound
	}
	rows, err := db.Query(`SELECT p.username FROM super_team_members m
		JOIN user_profiles p ON p.user_id = m.user_id
		WHERE m.team_prefix = ? AND m.role_level >= ? ORDER BY p.username`,
		prefix, core.SuperTeamRoleManage)
	if err != nil {
		return nil, fmt.Errorf("list global team reviewers: %w", err)
	}
	defer rows.Close()
	reviewers := make([]string, 0)
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return nil, fmt.Errorf("scan global team reviewer: %w", err)
		}
		reviewers = append(reviewers, username)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate global team reviewers: %w", err)
	}
	return reviewers, nil
}

// UpdateSuperTeam changes mutable display metadata while preserving the namespace prefix.
func (db *DB) UpdateSuperTeam(prefix, actor, name, description string, administrator bool, updatedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	prefix, valid := sanitizeSuperTeamPrefix(prefix)
	name, nameValid := core.NormalizeSuperTeamText(name, core.MaxSuperTeamNameRunes, false)
	description, descriptionValid := core.NormalizeSuperTeamText(description, core.MaxSuperTeamDescription, true)
	if !valid || !nameValid || !descriptionValid || updatedAt <= 0 {
		return errors.New("global team update is invalid")
	}
	actorID, err := db.userIDForExistingAccount(actor)
	if err != nil {
		return core.ErrSuperTeamPermissionDenied
	}
	superTeamMutationLock.Lock()
	defer superTeamMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin global team update: %w", err)
	}
	defer tx.Rollback()
	if !administrator {
		var level int
		if err := tx.QueryRow(`SELECT role_level FROM super_team_members
			WHERE team_prefix = ? AND user_id = ?`, prefix, actorID).Scan(&level); errors.Is(err, sql.ErrNoRows) ||
			level < core.SuperTeamRoleOwner {
			return core.ErrSuperTeamPermissionDenied
		} else if err != nil {
			return fmt.Errorf("inspect global team owner: %w", err)
		}
	}
	result, err := tx.Exec(`UPDATE super_teams SET name = ?, description = ?, updated_at = ? WHERE prefix = ?`,
		name, description, updatedAt, prefix)
	if err != nil {
		return fmt.Errorf("update global team: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return core.ErrSuperTeamNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit global team update: %w", err)
	}
	return nil
}

// DeleteSuperTeam removes an empty global team and cancels its invitations.
func (db *DB) DeleteSuperTeam(prefix, actor string, administrator bool, actedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	prefix, valid := sanitizeSuperTeamPrefix(prefix)
	if !valid || actedAt <= 0 {
		return core.ErrSuperTeamNotFound
	}
	actorID, err := db.userIDForExistingAccount(actor)
	if err != nil {
		return core.ErrSuperTeamPermissionDenied
	}
	superTeamMutationLock.Lock()
	defer superTeamMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin global team deletion: %w", err)
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM super_teams WHERE prefix = ?`, prefix).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return core.ErrSuperTeamNotFound
	} else if err != nil {
		return fmt.Errorf("inspect global team deletion: %w", err)
	}
	if !administrator {
		var level int
		if err := tx.QueryRow(`SELECT role_level FROM super_team_members WHERE team_prefix = ? AND user_id = ?`,
			prefix, actorID).Scan(&level); errors.Is(err, sql.ErrNoRows) || level < core.SuperTeamRoleOwner {
			return core.ErrSuperTeamPermissionDenied
		} else if err != nil {
			return fmt.Errorf("inspect global team deletion permission: %w", err)
		}
	}
	for _, table := range []string{
		"cargo_packages", "docker_images", "npm_packages", "maven_domains", "maven_artifacts",
	} {
		var bindings int
		if err := tx.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE super_team_prefix = ?", prefix).Scan(&bindings); err != nil {
			return fmt.Errorf("count global team bindings in %s: %w", table, err)
		}
		if bindings > 0 {
			return core.ErrSuperTeamNotEmpty
		}
	}
	var pendingReviews int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM review_tasks WHERE status = ?
		AND (source_team_prefix = ? OR target_team_prefix = ? OR review_team_prefix = ?)`,
		core.ReviewStatusPending, prefix, prefix, prefix).Scan(&pendingReviews); err != nil {
		return fmt.Errorf("count global team transfer reviews: %w", err)
	}
	if pendingReviews > 0 {
		return core.ErrSuperTeamNotEmpty
	}
	if err := cancelSuperTeamInvitations(tx, prefix, "", actedAt); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM super_team_members WHERE team_prefix = ?`, prefix); err != nil {
		return fmt.Errorf("delete global team members: %w", err)
	}
	for _, table := range []string{
		"publication_quota_reservations", "publication_quota_usage", "publication_quota_overrides",
	} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE owner_type = ? AND owner_key = ?`,
			core.PublicationQuotaOwnerSuperTeam, prefix); err != nil {
			return fmt.Errorf("delete global team publication quota state from %s: %w", table, err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM super_teams WHERE prefix = ?`, prefix); err != nil {
		return fmt.Errorf("delete global team: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit global team deletion: %w", err)
	}
	return nil
}

func cancelSuperTeamInvitations(tx *Tx, prefix, recipientID string, actedAt int64) error {
	query := `SELECT id FROM super_team_invitations WHERE team_prefix = ?`
	args := []any{prefix}
	if recipientID != "" {
		query += ` AND recipient_id = ?`
		args = append(args, recipientID)
	}
	rows, err := tx.Query(query, args...)
	if err != nil {
		return fmt.Errorf("list global team invitations for cancellation: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan global team invitation: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate global team invitations: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close global team invitations: %w", err)
	}
	for _, id := range ids {
		if _, err := tx.Exec(`UPDATE user_messages SET action_status = ?, acted_at = ?,
			read_at = CASE WHEN read_at = 0 THEN ? ELSE read_at END
			WHERE id = ? AND action_status = ?`, core.MessageActionCancelled, actedAt, actedAt,
			id, core.MessageActionPending); err != nil {
			return fmt.Errorf("cancel global team invitation message: %w", err)
		}
	}
	deleteQuery := `DELETE FROM super_team_invitations WHERE team_prefix = ?`
	if recipientID != "" {
		deleteQuery += ` AND recipient_id = ?`
	}
	if _, err := tx.Exec(deleteQuery, args...); err != nil {
		return fmt.Errorf("delete global team invitations: %w", err)
	}
	return nil
}

func cancelSuperTeamInvitationsForUser(tx *Tx, userID string, actedAt int64) error {
	rows, err := tx.Query(`SELECT id FROM super_team_invitations WHERE inviter_id = ? OR recipient_id = ?`, userID, userID)
	if err != nil {
		return fmt.Errorf("list account global team invitations for cancellation: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan account global team invitation: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate account global team invitations: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close account global team invitations: %w", err)
	}
	for _, id := range ids {
		if _, err := tx.Exec(`UPDATE user_messages SET action_status = ?, acted_at = ?,
			read_at = CASE WHEN read_at = 0 THEN ? ELSE read_at END WHERE id = ? AND action_status = ?`,
			core.MessageActionCancelled, actedAt, actedAt, id, core.MessageActionPending); err != nil {
			return fmt.Errorf("cancel account global team invitation message: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM super_team_invitations WHERE inviter_id = ? OR recipient_id = ?`, userID, userID); err != nil {
		return fmt.Errorf("delete account global team invitations: %w", err)
	}
	return nil
}

// CreateSuperTeamInvitations stores team invitations and action messages atomically.
func (db *DB) CreateSuperTeamInvitations(invitations []*core.SuperTeamInvitation, messages []*core.UserMessage) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(invitations) == 0 || len(invitations) != len(messages) || len(invitations) > maxSuperTeamMutationMembers {
		return errors.New("global team invitation batch is invalid")
	}
	prefix, valid := sanitizeSuperTeamPrefix(invitations[0].TeamPrefix)
	if !valid {
		return core.ErrSuperTeamNotFound
	}
	inviterID, err := db.userIDForExistingAccount(invitations[0].Inviter)
	if err != nil {
		return core.ErrSuperTeamPermissionDenied
	}
	recipientIDs := make([]string, len(invitations))
	seen := make(map[string]struct{}, len(invitations))
	for index, invitation := range invitations {
		message := messages[index]
		if invitation == nil || message == nil || invitation.TeamPrefix != prefix ||
			!strings.EqualFold(invitation.Inviter, invitations[0].Inviter) ||
			invitation.Level < core.SuperTeamRoleRead || invitation.Level > core.SuperTeamRoleOwner ||
			invitation.ID == "" || invitation.ID != message.ID || invitation.ExpiresAt <= invitation.CreatedAt {
			return errors.New("global team invitation is invalid")
		}
		recipientID, identityErr := db.userIDForExistingAccount(invitation.Recipient)
		if identityErr != nil {
			return core.ErrUserProfileNotFound
		}
		if recipientID == inviterID {
			return errors.New("global team invitation recipient is invalid")
		}
		if _, duplicate := seen[recipientID]; duplicate {
			return errors.New("global team invitation recipient is duplicated")
		}
		seen[recipientID] = struct{}{}
		recipientIDs[index] = recipientID
		if !strings.EqualFold(message.Recipient, invitation.Recipient) || normalizeMessage(message) != nil {
			return errors.New("global team invitation message does not match its workflow record")
		}
	}
	superTeamMutationLock.Lock()
	defer superTeamMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin global team invitation: %w", err)
	}
	defer tx.Rollback()
	var inviterLevel int
	if err := tx.QueryRow(`SELECT role_level FROM super_team_members WHERE team_prefix = ? AND user_id = ?`,
		prefix, inviterID).Scan(&inviterLevel); errors.Is(err, sql.ErrNoRows) || inviterLevel < core.SuperTeamRoleManage {
		return core.ErrSuperTeamPermissionDenied
	} else if err != nil {
		return fmt.Errorf("inspect global team inviter: %w", err)
	}
	for index, invitation := range invitations {
		if invitation.Level >= core.SuperTeamRoleManage && inviterLevel < core.SuperTeamRoleOwner {
			return core.ErrSuperTeamPermissionDenied
		}
		recipientID := recipientIDs[index]
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM super_team_members WHERE team_prefix = ? AND user_id = ?`,
			prefix, recipientID).Scan(&exists); err == nil {
			return core.ErrSuperTeamMemberExists
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect global team invitation recipient: %w", err)
		}
		var existingID string
		var existingExpiry int64
		err := tx.QueryRow(`SELECT id, expires_at FROM super_team_invitations
			WHERE team_prefix = ? AND recipient_id = ?`, prefix, recipientID).Scan(&existingID, &existingExpiry)
		if err == nil && existingExpiry > invitation.CreatedAt {
			return core.ErrSuperTeamInvitationExists
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect pending global team invitation: %w", err)
		}
		if err == nil {
			if err := cancelSuperTeamInvitations(tx, prefix, recipientID, invitation.CreatedAt); err != nil {
				return err
			}
		}
		message := messages[index]
		var dedupeKey any
		if message.DedupeKey != "" {
			dedupeKey = message.DedupeKey
		}
		if _, err := tx.Exec(`INSERT INTO user_messages (`+messageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			message.ID, message.Recipient, message.Sender, message.Kind, message.Severity,
			message.Title, message.Body, string(message.Payload), message.ActionKind, message.ActionStatus,
			message.CreatedAt, message.ReadAt, message.ActedAt, message.ExpiresAt, dedupeKey); err != nil {
			return fmt.Errorf("create global team invitation message: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO super_team_invitations
			(id, team_prefix, inviter_id, recipient_id, role_level, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, invitation.ID, prefix, inviterID, recipientID,
			invitation.Level, invitation.CreatedAt, invitation.ExpiresAt); err != nil {
			return fmt.Errorf("create global team invitation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit global team invitation: %w", err)
	}
	return nil
}

// ForceAddSuperTeamMembers adds members immediately for administrator workflows while enforcing account limits.
func (db *DB) ForceAddSuperTeamMembers(prefix, actor string, usernames []string, level,
	globalCreateLimit, globalJoinLimit int, actedAt int64,
) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	prefix, valid := sanitizeSuperTeamPrefix(prefix)
	if !valid || len(usernames) == 0 || len(usernames) > maxSuperTeamMutationMembers ||
		level < core.SuperTeamRoleRead || level > core.SuperTeamRoleOwner || actedAt <= 0 {
		return errors.New("global team member addition is invalid")
	}
	actor = strings.ToLower(strings.TrimSpace(actor))
	type candidate struct {
		username string
		userID   string
	}
	candidates := make([]candidate, 0, len(usernames))
	seen := make(map[string]struct{}, len(usernames))
	for _, rawUsername := range usernames {
		username := strings.ToLower(strings.TrimSpace(rawUsername))
		userID, err := db.userIDForExistingAccount(username)
		if err != nil {
			return core.ErrUserProfileNotFound
		}
		if _, duplicate := seen[userID]; duplicate {
			continue
		}
		seen[userID] = struct{}{}
		candidates = append(candidates, candidate{username: username, userID: userID})
	}
	if len(candidates) == 0 {
		return errors.New("global team member addition is empty")
	}
	superTeamMutationLock.Lock()
	defer superTeamMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin global team member addition: %w", err)
	}
	defer tx.Rollback()
	var teamName string
	if err := tx.QueryRow(`SELECT name FROM super_teams WHERE prefix = ?`, prefix).Scan(&teamName); errors.Is(err, sql.ErrNoRows) {
		return core.ErrSuperTeamNotFound
	} else if err != nil {
		return fmt.Errorf("inspect global team: %w", err)
	}
	for _, target := range candidates {
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM super_team_members WHERE team_prefix = ? AND user_id = ?`,
			prefix, target.userID).Scan(&exists); err == nil {
			return core.ErrSuperTeamMemberExists
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect global team member: %w", err)
		}
		_, joinLimit, _, _, err := effectiveSuperTeamLimitsTx(tx, target.userID, globalCreateLimit, globalJoinLimit)
		if err != nil {
			return err
		}
		var joined int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM super_team_members WHERE user_id = ?`, target.userID).Scan(&joined); err != nil {
			return fmt.Errorf("count global team memberships: %w", err)
		}
		if joined >= joinLimit {
			return core.ErrSuperTeamJoinLimit
		}
		if _, err := tx.Exec(`INSERT INTO super_team_members
			(team_prefix, user_id, role_level, added_at) VALUES (?, ?, ?, ?)`,
			prefix, target.userID, level, actedAt); err != nil {
			return fmt.Errorf("add global team member: %w", err)
		}
		if err := cancelSuperTeamInvitations(tx, prefix, target.userID, actedAt); err != nil {
			return err
		}
		if !strings.EqualFold(actor, target.username) {
			if err := insertAcceptedMembershipMessage(tx, target.username, actor, "super_team_invite",
				"Global team membership added",
				fmt.Sprintf("You were added to global team %s with T%d permission.", teamName, level),
				map[string]any{"prefix": prefix, "inviter": actor, "level": level}, actedAt); err != nil {
				return fmt.Errorf("create global team membership message: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit global team member addition: %w", err)
	}
	return nil
}

// RespondSuperTeamInvitation accepts or rejects one global-team invitation.
func (db *DB) RespondSuperTeamInvitation(id, recipient string, accept bool, globalJoinLimit int, actedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if id == "" || actedAt <= 0 || globalJoinLimit < 1 || globalJoinLimit > maxSuperTeamLimit {
		return core.ErrSuperTeamInvitationInvalid
	}
	recipientID, err := db.userIDForExistingAccount(recipient)
	if err != nil {
		return core.ErrSuperTeamInvitationInvalid
	}
	superTeamMutationLock.Lock()
	defer superTeamMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin global team invitation response: %w", err)
	}
	defer tx.Rollback()
	invitation := &core.SuperTeamInvitation{ID: id, Recipient: strings.ToLower(strings.TrimSpace(recipient))}
	var inviterID string
	err = tx.QueryRow(`SELECT team_prefix, inviter_id, role_level, created_at, expires_at
		FROM super_team_invitations WHERE id = ? AND recipient_id = ?`, id, recipientID).Scan(
		&invitation.TeamPrefix, &inviterID, &invitation.Level, &invitation.CreatedAt, &invitation.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) || invitation.ExpiresAt <= actedAt {
		return core.ErrSuperTeamInvitationInvalid
	}
	if err != nil {
		return fmt.Errorf("load global team invitation: %w", err)
	}
	if accept {
		var inviterLevel int
		if err := tx.QueryRow(`SELECT role_level FROM super_team_members
			WHERE team_prefix = ? AND user_id = ?`, invitation.TeamPrefix, inviterID).Scan(&inviterLevel); errors.Is(err, sql.ErrNoRows) || inviterLevel < core.SuperTeamRoleManage ||
			invitation.Level >= core.SuperTeamRoleManage && inviterLevel < core.SuperTeamRoleOwner {
			return core.ErrSuperTeamInvitationInvalid
		} else if err != nil {
			return fmt.Errorf("validate global team inviter: %w", err)
		}
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM super_team_members WHERE team_prefix = ? AND user_id = ?`,
			invitation.TeamPrefix, recipientID).Scan(&exists); err == nil {
			return core.ErrSuperTeamMemberExists
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect global team invitation membership: %w", err)
		}
		_, joinLimit, _, _, err := effectiveSuperTeamLimitsTx(tx, recipientID, 1, globalJoinLimit)
		if err != nil {
			return err
		}
		var joined int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM super_team_members WHERE user_id = ?`, recipientID).Scan(&joined); err != nil {
			return fmt.Errorf("count global team memberships: %w", err)
		}
		if joined >= joinLimit {
			return core.ErrSuperTeamJoinLimit
		}
		if _, err := tx.Exec(`INSERT INTO super_team_members
			(team_prefix, user_id, role_level, added_at) VALUES (?, ?, ?, ?)`,
			invitation.TeamPrefix, recipientID, invitation.Level, actedAt); err != nil {
			return fmt.Errorf("accept global team membership: %w", err)
		}
	}
	status := core.MessageActionRejected
	if accept {
		status = core.MessageActionAccepted
	}
	result, err := tx.Exec(`UPDATE user_messages SET action_status = ?, acted_at = ?,
		read_at = CASE WHEN read_at = 0 THEN ? ELSE read_at END
		WHERE id = ? AND recipient = ? AND action_kind = 'super_team_invite'
		AND action_status = ? AND (expires_at = 0 OR expires_at > ?)`, status, actedAt, actedAt,
		id, invitation.Recipient, core.MessageActionPending, actedAt)
	if err != nil {
		return fmt.Errorf("update global team invitation message: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return core.ErrSuperTeamInvitationInvalid
	}
	if _, err := tx.Exec(`DELETE FROM super_team_invitations WHERE id = ? AND recipient_id = ?`, id, recipientID); err != nil {
		return fmt.Errorf("complete global team invitation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit global team invitation response: %w", err)
	}
	return nil
}

// SetSuperTeamMemberLevel updates one member without permitting T3 privilege escalation.
func (db *DB) SetSuperTeamMemberLevel(prefix, actor, target string, level int, administrator bool) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	prefix, valid := sanitizeSuperTeamPrefix(prefix)
	if !valid || level < core.SuperTeamRoleRead || level > core.SuperTeamRoleOwner {
		return errors.New("global team member update is invalid")
	}
	actorID, err := db.userIDForExistingAccount(actor)
	if err != nil {
		return core.ErrSuperTeamPermissionDenied
	}
	targetID, err := db.userIDForExistingAccount(target)
	if err != nil {
		return core.ErrSuperTeamNotFound
	}
	superTeamMutationLock.Lock()
	defer superTeamMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin global team member update: %w", err)
	}
	defer tx.Rollback()
	actorLevel := core.SuperTeamRoleOwner
	if !administrator {
		if err := tx.QueryRow(`SELECT role_level FROM super_team_members WHERE team_prefix = ? AND user_id = ?`,
			prefix, actorID).Scan(&actorLevel); errors.Is(err, sql.ErrNoRows) || actorLevel < core.SuperTeamRoleManage {
			return core.ErrSuperTeamPermissionDenied
		} else if err != nil {
			return fmt.Errorf("inspect global team actor: %w", err)
		}
	}
	var current int
	if err := tx.QueryRow(`SELECT role_level FROM super_team_members WHERE team_prefix = ? AND user_id = ?`,
		prefix, targetID).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return core.ErrSuperTeamNotFound
	} else if err != nil {
		return fmt.Errorf("inspect global team member: %w", err)
	}
	if !administrator && actorLevel < core.SuperTeamRoleOwner &&
		(current >= core.SuperTeamRoleManage || level >= core.SuperTeamRoleManage) {
		return core.ErrSuperTeamPermissionDenied
	}
	if current == core.SuperTeamRoleOwner && level < core.SuperTeamRoleOwner {
		if err := requireAnotherSuperTeamOwner(tx, prefix, targetID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE super_team_members SET role_level = ? WHERE team_prefix = ? AND user_id = ?`,
		level, prefix, targetID); err != nil {
		return fmt.Errorf("update global team member: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit global team member update: %w", err)
	}
	return nil
}

// RemoveSuperTeamMember removes a managed member or lets a non-owner leave.
func (db *DB) RemoveSuperTeamMember(prefix, actor, target string, administrator bool, actedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	prefix, valid := sanitizeSuperTeamPrefix(prefix)
	if !valid || actedAt <= 0 {
		return core.ErrSuperTeamNotFound
	}
	actorID, err := db.userIDForExistingAccount(actor)
	if err != nil {
		return core.ErrSuperTeamPermissionDenied
	}
	targetID, err := db.userIDForExistingAccount(target)
	if err != nil {
		return core.ErrSuperTeamNotFound
	}
	superTeamMutationLock.Lock()
	defer superTeamMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin global team member removal: %w", err)
	}
	defer tx.Rollback()
	actorLevel := core.SuperTeamRoleOwner
	if !administrator {
		if err := tx.QueryRow(`SELECT role_level FROM super_team_members WHERE team_prefix = ? AND user_id = ?`,
			prefix, actorID).Scan(&actorLevel); err != nil {
			return core.ErrSuperTeamPermissionDenied
		}
	}
	var targetLevel int
	if err := tx.QueryRow(`SELECT role_level FROM super_team_members WHERE team_prefix = ? AND user_id = ?`,
		prefix, targetID).Scan(&targetLevel); errors.Is(err, sql.ErrNoRows) {
		return core.ErrSuperTeamNotFound
	} else if err != nil {
		return fmt.Errorf("inspect global team removal target: %w", err)
	}
	self := actorID == targetID
	if self && targetLevel == core.SuperTeamRoleOwner {
		return core.ErrSuperTeamOwnerCannotLeave
	}
	if !self && !administrator && (actorLevel < core.SuperTeamRoleManage ||
		actorLevel < core.SuperTeamRoleOwner && targetLevel >= core.SuperTeamRoleManage) {
		return core.ErrSuperTeamPermissionDenied
	}
	if targetLevel == core.SuperTeamRoleOwner {
		if err := requireAnotherSuperTeamOwner(tx, prefix, targetID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM super_team_members WHERE team_prefix = ? AND user_id = ?`, prefix, targetID); err != nil {
		return fmt.Errorf("remove global team member: %w", err)
	}
	if err := cancelSuperTeamInvitations(tx, prefix, targetID, actedAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit global team member removal: %w", err)
	}
	return nil
}

func requireAnotherSuperTeamOwner(tx *Tx, prefix, excludedUserID string) error {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM super_team_members
		WHERE team_prefix = ? AND role_level = ? AND user_id <> ?`,
		prefix, core.SuperTeamRoleOwner, excludedUserID).Scan(&count); err != nil {
		return fmt.Errorf("count global team T4 owners: %w", err)
	}
	if count == 0 {
		return core.ErrSuperTeamLastOwner
	}
	return nil
}

// CleanExpiredSuperTeamInvitations cancels invitations that can no longer be accepted.
func (db *DB) CleanExpiredSuperTeamInvitations(now int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	superTeamMutationLock.Lock()
	defer superTeamMutationLock.Unlock()
	rows, err := db.Query(`SELECT team_prefix, recipient_id FROM super_team_invitations WHERE expires_at <= ?`, now)
	if err != nil {
		return fmt.Errorf("list expired global team invitations: %w", err)
	}
	type expiredInvitation struct{ prefix, recipientID string }
	expired := make([]expiredInvitation, 0)
	for rows.Next() {
		var invitation expiredInvitation
		if err := rows.Scan(&invitation.prefix, &invitation.recipientID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan expired global team invitation: %w", err)
		}
		expired = append(expired, invitation)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate expired global team invitations: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close expired global team invitations: %w", err)
	}
	for _, invitation := range expired {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin expired global team invitation cleanup: %w", err)
		}
		if err := cancelSuperTeamInvitations(tx, invitation.prefix, invitation.recipientID, now); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit expired global team invitation cleanup: %w", err)
		}
	}
	return nil
}
