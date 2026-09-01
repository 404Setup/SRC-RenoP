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
	"slices"
	"strings"

	"renop/internal/config"
	"renop/internal/core"
)

func normalizeResourceRepositories(repositories []string) []string {
	result := make([]string, 0, len(repositories))
	seen := make(map[string]struct{}, len(repositories))
	for _, repository := range repositories {
		repository = strings.ToLower(SanitizeInputString(strings.TrimSpace(repository), 64))
		if repository == "" {
			continue
		}
		if _, duplicate := seen[repository]; duplicate {
			continue
		}
		seen[repository] = struct{}{}
		result = append(result, repository)
	}
	slices.Sort(result)
	return result
}

func resourceRepositoryCondition(column string, repositories []string, args *[]any) string {
	if len(repositories) == 0 {
		return "1 = 0"
	}
	placeholders := make([]string, len(repositories))
	for index, repository := range repositories {
		placeholders[index] = "?"
		*args = append(*args, repository)
	}
	return column + " IN (" + strings.Join(placeholders, ",") + ")"
}

// ListSuperTeamResources returns one authorized, bounded global-team resource page.
func (db *DB) ListSuperTeamResources(options core.SuperTeamResourceListOptions) ([]*core.SuperTeamResource, int, error) {
	if db == nil || db.SQLDB == nil {
		return nil, 0, core.ErrDatabaseUnavailable
	}
	prefix, valid := sanitizeSuperTeamPrefix(options.Prefix)
	format := strings.ToLower(strings.TrimSpace(options.Format))
	if !valid || options.Limit < 1 || options.Limit > 50 || options.Offset < 0 {
		return nil, 0, errors.New("global team resource page is invalid")
	}
	var exists int
	if err := db.QueryRow(`SELECT 1 FROM super_teams WHERE prefix = ?`, prefix).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return nil, 0, core.ErrSuperTeamNotFound
	} else if err != nil {
		return nil, 0, fmt.Errorf("inspect global team resource owner: %w", err)
	}
	visibleRepositories := normalizeResourceRepositories(options.VisibleRepositories)
	privateRepositories := normalizeResourceRepositories(options.PrivateRepositories)
	viewerID := ""
	if options.Viewer = strings.TrimSpace(options.Viewer); options.Viewer != "" &&
		!strings.EqualFold(options.Viewer, "guest") {
		if resolved, err := db.userIDForExistingAccount(options.Viewer); err == nil {
			viewerID = resolved
		}
	}

	selectColumns := ""
	fromClause := ""
	whereClause := ""
	orderClause := ""
	args := make([]any, 0, 8+len(visibleRepositories)+len(privateRepositories))
	switch format {
	case config.RepositoryFormatMaven:
		selectColumns = `'', resource.domain, '', 0`
		fromClause = ` FROM maven_domains resource`
		whereClause = ` WHERE resource.repository = '' AND resource.super_team_prefix = ? AND resource.verified = 1`
		orderClause = ` ORDER BY resource.domain`
		args = append(args, prefix)
	case config.RepositoryFormatCargo:
		selectColumns = `resource.repository, resource.package_name, SUBSTR(resource.description, 1, 4000),
			CASE WHEN resource.archived = 1 OR resource.admin_archived = 1 THEN 1 ELSE 0 END`
		fromClause = ` FROM cargo_packages resource
			LEFT JOIN cargo_members explicit_member ON explicit_member.repository = resource.repository
				AND explicit_member.normalized_name = resource.normalized_name AND explicit_member.user_id = ?
			LEFT JOIN super_team_members team_member ON team_member.team_prefix = resource.super_team_prefix
				AND team_member.user_id = ?`
		args = append(args, viewerID, viewerID, prefix)
		whereClause = ` WHERE resource.super_team_prefix = ? AND (` +
			resourceRepositoryCondition("resource.repository", visibleRepositories, &args) +
			` OR explicit_member.user_id IS NOT NULL OR team_member.user_id IS NOT NULL)`
		orderClause = ` ORDER BY resource.repository, resource.normalized_name`
	case config.RepositoryFormatDocker, config.RepositoryFormatNPM:
		table, nameColumn, memberTable := "docker_images", "image_name", "docker_members"
		if format == config.RepositoryFormatNPM {
			table, nameColumn, memberTable = "npm_packages", "package_name", "npm_members"
		}
		selectColumns = `resource.repository, resource.` + nameColumn + `,
			SUBSTR(resource.description, 1, 4000), resource.archived`
		if format == config.RepositoryFormatDocker {
			selectColumns = `resource.repository, resource.image_name,
				SUBSTR(resource.description, 1, 4000), 0`
		}
		fromClause = ` FROM ` + table + ` resource
			LEFT JOIN ` + memberTable + ` explicit_member ON explicit_member.repository = resource.repository
				AND explicit_member.` + nameColumn + ` = resource.` + nameColumn + ` AND explicit_member.user_id = ?
			LEFT JOIN super_team_members team_member ON team_member.team_prefix = resource.super_team_prefix
				AND team_member.user_id = ?`
		args = append(args, viewerID, viewerID, prefix)
		publicCondition := `(` + resourceRepositoryCondition("resource.repository", visibleRepositories, &args) +
			` OR explicit_member.user_id IS NOT NULL OR team_member.user_id IS NOT NULL)`
		privateConditions := []string{`explicit_member.user_id IS NOT NULL`, `team_member.user_id IS NOT NULL`}
		if len(privateRepositories) > 0 {
			privateConditions = append(privateConditions,
				resourceRepositoryCondition("resource.repository", privateRepositories, &args))
		}
		whereClause = ` WHERE resource.super_team_prefix = ? AND ((resource.private = 0 AND ` + publicCondition +
			`) OR (resource.private = 1 AND (` + strings.Join(privateConditions, " OR ") + `)))`
		orderClause = ` ORDER BY resource.repository, resource.` + nameColumn
	default:
		return nil, 0, errors.New("global team resource format is invalid")
	}

	var total int
	if err := db.QueryRow(`SELECT COUNT(*)`+fromClause+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count %s global team resources: %w", format, err)
	}
	rows, err := db.Query(`SELECT `+selectColumns+fromClause+whereClause+orderClause+` LIMIT ? OFFSET ?`,
		append(args, options.Limit, options.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list %s global team resources: %w", format, err)
	}
	defer rows.Close()
	resources := make([]*core.SuperTeamResource, 0, min(options.Limit, total))
	for rows.Next() {
		resource := &core.SuperTeamResource{Format: format}
		var archived int
		if err := rows.Scan(&resource.Repository, &resource.Name, &resource.Description, &archived); err != nil {
			return nil, 0, fmt.Errorf("scan %s global team resource: %w", format, err)
		}
		resource.Archived = archived != 0
		resources = append(resources, resource)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate %s global team resources: %w", format, err)
	}
	return resources, total, nil
}
