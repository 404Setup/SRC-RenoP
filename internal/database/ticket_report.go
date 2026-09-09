/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"

	"renop/internal/core"
)

// CreateTicket stores a bounded support request and its immutable report recipients together.
func (db *DB) CreateTicket(request core.TicketRequest, actor, session string, at int64) (*core.ReviewTask, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	title, validTitle := core.NormalizeSuperTeamText(request.Title, 160, false)
	body, validBody := normalizeTicketText(request.Body, 8000)
	if !validTitle || !validBody || title == "" || body == "" || at <= 0 ||
		(request.Kind != core.TicketKindFeedback && request.Kind != core.TicketKindSuggestion && request.Kind != core.TicketKindReport) {
		return nil, core.ErrReviewInvalidRequest
	}
	reviewTaskMutationLock.Lock()
	defer reviewTaskMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	account, err := accountEmailSessionTx(tx, strings.ToLower(strings.TrimSpace(actor)), session)
	if err != nil {
		return nil, core.ErrReviewPermissionDenied
	}
	task := &core.ReviewTask{ID: uuid.NewString(), Kind: request.Kind, ResourceType: "support",
		Repository: strings.ToLower(strings.TrimSpace(request.Repository)), ResourceName: title,
		RequestedByID: account.UserID, RequestedBy: strings.ToLower(actor), Status: core.ReviewStatusPending,
		CreatedAt: at, UpdatedAt: at, TicketState: core.TicketState{Status: core.TicketUnprocessed, Title: title, Body: body}}
	if request.Kind == core.TicketKindReport {
		target, recipients, err := reportTargetTx(tx, request.Target, account.UserID)
		if err != nil {
			return nil, err
		}
		task.Repository, task.ResourceType, task.ResourceName = target.Repository, target.Format, target.Name
		task.ResourceKey, task.ResourceVersion = target.Name, target.Version
		encoded, err := json.Marshal(recipients)
		if err != nil {
			return nil, err
		}
		task.TargetUserIDs = string(encoded)
	}
	storedKey := task.ResourceKey
	if task.Kind == core.TicketKindReport {
		storedKey = publicationReviewKey(task.ResourceKey, "v:"+task.ResourceVersion)
	}
	if len(task.Repository) > 64 || len(storedKey) > maxReviewResourceKey {
		return nil, core.ErrReviewInvalidRequest
	}
	var pending, recent, total int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM review_tasks WHERE status = ?`, core.ReviewStatusPending).Scan(&total); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(`SELECT COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN created_at >= ? THEN 1 ELSE 0 END), 0) FROM review_tasks
		WHERE requested_by_id = ? AND kind IN (?, ?, ?)`, core.ReviewStatusPending, at-(24*time.Hour).Milliseconds(),
		account.UserID, core.TicketKindFeedback, core.TicketKindSuggestion, core.TicketKindReport).Scan(&pending, &recent); err != nil {
		return nil, err
	}
	if total >= maxPendingPublicationReviews || pending >= 16 || recent >= 24 {
		return nil, core.ErrReviewFileLimit
	}
	var activeKey any
	if task.Kind == core.TicketKindReport {
		digest := sha256.Sum256([]byte(strings.Join([]string{task.Kind, task.RequestedByID, task.Repository, task.ResourceType, storedKey}, "\x00")))
		task.ActiveKey = hex.EncodeToString(digest[:])
		activeKey = task.ActiveKey
		var existing string
		if err := tx.QueryRow(`SELECT id FROM review_tasks WHERE active_key = ?`, activeKey).Scan(&existing); err == nil {
			return nil, core.ErrReviewTaskExists
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO review_tasks (id, kind, resource_type, repository, resource_key, resource_name,
		review_team_prefix, requested_by_id, requested_by_name, status, created_at, active_key)
		VALUES (?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?)`, task.ID, task.Kind, task.ResourceType, task.Repository, storedKey,
		task.ResourceName, task.RequestedByID, task.RequestedBy, task.Status, at, activeKey); err != nil {
		return nil, err
	}
	if err := insertTicketStateTx(tx, task); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return task, nil
}

func reportTargetTx(tx *Tx, target core.ResourceLockTarget, actorID string) (core.ResourceLockTarget, []string, error) {
	target.Format = strings.ToLower(strings.TrimSpace(target.Format))
	if target.Format == "user" {
		if target.Repository != "" || target.Version != "" || len(target.Name) > maxTokenNameLen {
			return target, nil, core.ErrReviewInvalidRequest
		}
		target.Name = strings.ToLower(strings.TrimSpace(target.Name))
		id, err := userIDForUsernameTx(tx, target.Name)
		if err != nil || id == actorID {
			return target, nil, core.ErrReviewPermissionDenied
		}
		return target, []string{id}, nil
	}
	target, err := normalizeResourceLockTarget(target)
	if err != nil {
		return target, nil, core.ErrReviewInvalidRequest
	}
	var member bool
	var level, private, mirrored int
	var team, publisher, ownerQuery, versionQuery string
	var ownerArgs, versionArgs []any
	switch target.Format {
	case "superteam":
		var name string
		err = tx.QueryRow(`SELECT name FROM super_teams WHERE prefix = ?`, target.Name).Scan(&name)
		team = target.Name
		if err == nil {
			err = tx.QueryRow(`SELECT COALESCE(MAX(role_level), 0), COUNT(*) FROM super_team_members WHERE team_prefix = ? AND user_id = ?`,
				team, actorID).Scan(&level, &private)
			member, private = private > 0, 0
		}
	case "maven-domain":
		err = tx.QueryRow(`SELECT super_team_prefix FROM maven_domains WHERE repository = '' AND domain = ?`, target.Name).Scan(&team)
		if err == nil {
			level, member, err = mavenDomainEffectivePermissionTx(tx, target.Name, actorID)
		}
		ownerQuery, ownerArgs = `SELECT user_id FROM maven_domain_members WHERE repository = '' AND domain = ? AND permission_level = 4`, []any{target.Name}
	case "maven":
		group, artifact, _ := strings.Cut(target.Name, ":")
		var domain string
		err = tx.QueryRow(`SELECT domain, super_team_prefix, publisher, mirrored FROM maven_artifacts WHERE repository = ? AND group_id = ? AND artifact_id = ?`,
			target.Repository, group, artifact).Scan(&domain, &team, &publisher, &mirrored)
		if err == nil {
			level, member, err = mavenArtifactTeamPermissionTx(tx, target.Repository, target.Name, actorID)
		}
		if err == nil {
			domainLevel, domainMember, domainErr := mavenDomainEffectivePermissionTx(tx, domain, actorID)
			if domainErr != nil && !errors.Is(domainErr, core.ErrMavenDomainNotFound) {
				err = domainErr
			}
			level, member = max(level, domainLevel), member || domainMember
		}
		ownerQuery = `SELECT m.user_id FROM maven_domain_members m WHERE m.repository = '' AND m.domain = ? AND m.permission_level = 4
			UNION SELECT m.user_id FROM super_team_members m JOIN maven_domains d ON d.super_team_prefix = m.team_prefix
			WHERE d.repository = '' AND d.domain = ? AND m.role_level = 4`
		ownerArgs = []any{domain, domain}
		versionQuery, versionArgs = `SELECT publisher, mirrored FROM maven_versions WHERE repository = ? AND group_id = ? AND artifact_id = ? AND version = ?`,
			[]any{target.Repository, group, artifact, target.Version}
	case "cargo", "npm", "docker":
		table, members, key, publisherColumn, privateColumn, mirrorColumn := "cargo_packages", "cargo_members", "normalized_name", "''", "0", "mirrored"
		if target.Format == "npm" {
			table, members, key, publisherColumn, privateColumn = "npm_packages", "npm_members", "package_name", "publisher", "private"
		} else if target.Format == "docker" {
			table, members, key, publisherColumn, privateColumn, mirrorColumn = "docker_images", "docker_members", "image_name", "publisher", "private", "CASE WHEN push_enabled = 0 THEN 1 ELSE 0 END"
		}
		err = tx.QueryRow(`SELECT super_team_prefix, `+publisherColumn+`, `+privateColumn+`, `+mirrorColumn+` FROM `+table+` WHERE repository = ? AND `+key+` = ?`,
			target.Repository, target.Name).Scan(&team, &publisher, &private, &mirrored)
		if err == nil {
			switch target.Format {
			case "cargo":
				level, member, err = cargoEffectivePermissionTx(tx, target.Repository, target.Name, actorID)
			case "npm":
				level, member, err = npmEffectivePermissionTx(tx, target.Repository, target.Name, actorID)
			case "docker":
				level, member, err = dockerEffectivePermissionTx(tx, target.Repository, target.Name, actorID)
			}
		}
		ownerQuery, ownerArgs = `SELECT user_id FROM `+members+` WHERE repository = ? AND `+key+` = ? AND permission_level = 4`, []any{target.Repository, target.Name}
		versionQuery = `SELECT publisher, mirrored FROM ` + target.Format + `_versions WHERE repository = ? AND ` + key + ` = ? AND version = ?`
		if target.Format == "npm" {
			versionQuery += ` AND unpublished = 0`
		} else if target.Format == "docker" {
			versionQuery = `SELECT publisher, 0 FROM docker_tags WHERE repository = ? AND image_name = ? AND tag = ?`
			if strings.HasPrefix(target.Version, "sha256:") {
				versionQuery = `SELECT publisher, 0 FROM docker_manifests WHERE repository = ? AND image_name = ? AND digest = ?`
			}
		}
		versionArgs = []any{target.Repository, target.Name, target.Version}
	default:
		return target, nil, core.ErrReviewInvalidRequest
	}
	if errors.Is(err, sql.ErrNoRows) || mirrored != 0 || level >= 4 {
		return target, nil, core.ErrReviewPermissionDenied
	}
	if err != nil {
		return target, nil, err
	}
	user, err := ticketActorTx(tx, actorID)
	if err != nil {
		return target, nil, err
	}
	if !member && !user.CheckModeratePermission(target.Repository) {
		if private != 0 {
			return target, nil, core.ErrReviewPermissionDenied
		}
		var locks int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM `+resourceLocksQuery(target.Format)+` l WHERE l.format = ? AND l.repository = ?
			AND l.resource_name = ? AND l.mode = 'read' AND (l.version = '' OR `+resourceLockVersionColumn(target.Format, "l.version")+` = ?)`,
			target.Format, target.Repository, target.Name, core.ResourceLockVersionKey(target.Format, target.Version)).Scan(&locks); err != nil {
			return target, nil, err
		}
		if locks != 0 {
			return target, nil, core.ErrReviewPermissionDenied
		}
	}
	if target.Version != "" {
		if versionQuery == "" {
			return target, nil, core.ErrReviewInvalidRequest
		}
		if err := tx.QueryRow(versionQuery, versionArgs...).Scan(&publisher, &mirrored); errors.Is(err, sql.ErrNoRows) {
			return target, nil, core.ErrReviewPermissionDenied
		} else if err != nil {
			return target, nil, err
		}
		if mirrored != 0 {
			return target, nil, core.ErrReviewPermissionDenied
		}
	}
	if ownerQuery == "" {
		ownerQuery, ownerArgs = `SELECT user_id FROM super_team_members WHERE team_prefix = ? AND role_level = 4`, []any{team}
	} else {
		ownerQuery += ` UNION SELECT user_id FROM super_team_members WHERE team_prefix = ? AND role_level = 4`
		ownerArgs = append(ownerArgs, team)
	}
	ownerQuery += ` UNION SELECT user_id FROM user_profiles WHERE username = ? LIMIT 101`
	ownerArgs = append(ownerArgs, publisher)
	rows, err := tx.Query(ownerQuery, ownerArgs...)
	if err != nil {
		return target, nil, err
	}
	defer rows.Close()
	ids := make([]string, 0, 4)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return target, nil, err
		}
		if id == actorID {
			return target, nil, core.ErrReviewPermissionDenied
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return target, nil, err
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 || len(ids) > 100 {
		return target, nil, core.ErrReviewPermissionDenied
	}
	return target, ids, nil
}
