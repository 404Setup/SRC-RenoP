/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"fmt"
	"strings"

	"renop/internal/core"
)

func mavenDomainLockTarget(domain string) core.ResourceLockTarget {
	return core.ResourceLockTarget{Format: "maven-domain", Name: domain}
}

func (db *DB) attachMavenDomainLocks(domains []*core.MavenDomain) error {
	for start := 0; start < len(domains); start += 128 {
		batch := domains[start:min(start+128, len(domains))]
		byName := make(map[string]*core.MavenDomain, len(batch))
		args := make([]any, 0, len(batch))
		for _, domain := range batch {
			byName[domain.Domain] = domain
			args = append(args, domain.Domain)
		}
		rows, err := db.Query(`SELECT resource_name, source, mode, reason, locked_at, inherited FROM `+
			resourceLocksQuery("maven-domain")+` WHERE format = 'maven-domain' AND resource_name IN (`+
			strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")+`) ORDER BY resource_name, source`, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			lock := &core.ResourceLock{ResourceLockTarget: core.ResourceLockTarget{Format: "maven-domain"}}
			var inherited int
			if err := rows.Scan(&lock.Name, &lock.Source, &lock.Mode, &lock.Reason, &lock.LockedAt, &inherited); err != nil {
				_ = rows.Close()
				return err
			}
			lock.Inherited = inherited != 0
			domain := byName[lock.Name]
			domain.Locks = append(domain.Locks, lock)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	return nil
}

func ensureMavenDomainMutableQuery(queryRow func(string, ...any) row, domain string) error {
	return ensureResourceMutableQuery(queryRow, mavenDomainLockTarget(domain), false)
}

func (db *DB) mavenDomainMetadataVisibility(username string, moderator bool, targets []core.ResourceLockTarget) ([]bool, error) {
	paths := make([]string, len(targets))
	for i, target := range targets {
		paths[i] = strings.ReplaceAll(target.Name, ".", "/")
	}
	return db.MavenDomainPathVisibility("", username, moderator, paths)
}

// MavenDomainPathVisibility applies domain restrictions to a bounded page, including uncatalogued metadata.
func (db *DB) MavenDomainPathVisibility(repository, username string, moderator bool, paths []string) ([]bool, error) {
	if len(paths) > 128 {
		return nil, core.ErrResourceLockInvalid
	}
	visible := make([]bool, len(paths))
	for i := range visible {
		visible[i] = true
	}
	if moderator || len(paths) == 0 {
		return visible, nil
	}
	userID, err := db.mavenViewerID(username)
	if err != nil {
		return nil, err
	}
	args := make([]any, 0, len(paths)+4)
	inputs := make([]string, len(paths))
	for i, path := range paths {
		path = strings.Trim(strings.ReplaceAll(path, `\`, "/"), "/")
		if len(path) > maxPackageDeprecationKeyBytes || len(strings.Split(path, "/")) > core.MaxMavenPathParts {
			return nil, core.ErrResourceLockInvalid
		}
		inputs[i] = fmt.Sprintf("SELECT ? AS candidate, %d AS item_index", i)
		args = append(args, path)
	}
	args = append(args, userID, userID, userID, strings.ToLower(repository))
	candidatePath := resourceLockVersionColumn("maven", "c.candidate")
	artifactPath := resourceLockVersionColumn("maven", "CONCAT(REPLACE(a.group_id, '.', '/'), '/', a.artifact_id)")
	rows, err := db.Query(`SELECT DISTINCT c.item_index FROM (`+strings.Join(inputs, " UNION ALL ")+`) c
		JOIN `+resourceLocksQuery("maven-domain")+` l ON l.format = 'maven-domain' AND l.mode = 'read'
		JOIN maven_domains d ON d.repository = '' AND d.domain = l.resource_name
		LEFT JOIN maven_domain_members m ON m.repository = '' AND m.domain = d.domain AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = d.super_team_prefix AND stm.user_id = ?
		WHERE m.user_id IS NULL AND stm.user_id IS NULL AND (`+mavenDomainContainsPathSQL("d", "c.candidate")+`)
		AND NOT EXISTS (SELECT 1 FROM maven_artifacts a JOIN super_team_members am
			ON am.team_prefix = a.super_team_prefix AND am.user_id = ? WHERE a.repository = ? AND a.domain = d.domain
			AND (`+candidatePath+` = `+artifactPath+` OR SUBSTR(`+candidatePath+`, 1, LENGTH(`+artifactPath+`) + 1) = CONCAT(`+artifactPath+`, '/')))
		AND NOT EXISTS (SELECT 1 FROM maven_domains specific WHERE specific.repository = '' AND specific.verified = 1
			AND LENGTH(specific.domain) > LENGTH(d.domain) AND (`+mavenDomainContainsPathSQL("specific", "c.candidate")+`))`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var index int
		if err := rows.Scan(&index); err != nil {
			return nil, err
		}
		visible[index] = false
	}
	return visible, rows.Err()
}

func mavenDomainContainsPathSQL(alias, path string) string {
	path = "LOWER(" + path + ")"
	domainPath := "REPLACE(" + alias + ".domain, '.', '/')"
	return path + ` = ` + domainPath + ` OR SUBSTR(` + path + `, 1, LENGTH(` + domainPath + `) + 1) = CONCAT(` + domainPath + `, '/')`
}

func (db *DB) mavenPathDomainLocks(path string, descendants bool) ([]*core.ResourceLock, error) {
	name := strings.ToLower(strings.ReplaceAll(strings.Trim(strings.ReplaceAll(path, `\`, "/"), "/"), "/", "."))
	where := `(resource_name = ? OR SUBSTR(?, 1, LENGTH(resource_name) + 1) = CONCAT(resource_name, '.'))`
	args := []any{name, name}
	if descendants {
		if name == "" {
			where, args = `1 = 1`, nil
		} else {
			where += ` OR SUBSTR(resource_name, 1, LENGTH(?) + 1) = CONCAT(?, '.')`
			args = append(args, name, name)
		}
	}
	rows, err := db.Query(`SELECT resource_name, source, mode, reason, locked_at, inherited
		FROM `+resourceLocksQuery("maven-domain")+` l WHERE format = 'maven-domain' AND (`+where+`)
		AND NOT EXISTS (SELECT 1 FROM maven_domains specific WHERE specific.repository = '' AND specific.verified = 1
			AND LENGTH(specific.domain) > LENGTH(l.resource_name)
			AND (specific.domain = ? OR SUBSTR(?, 1, LENGTH(specific.domain) + 1) = CONCAT(specific.domain, '.')))
		LIMIT 8193`, append(args, name, name)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	locks := make([]*core.ResourceLock, 0)
	for rows.Next() {
		lock := &core.ResourceLock{ResourceLockTarget: core.ResourceLockTarget{Format: "maven-domain"}}
		var inherited int
		if err := rows.Scan(&lock.Name, &lock.Source, &lock.Mode, &lock.Reason, &lock.LockedAt, &inherited); err != nil {
			return nil, err
		}
		lock.Inherited = inherited != 0
		locks = append(locks, lock)
		if len(locks) > 8192 {
			return nil, core.ErrResourceLockInvalid
		}
	}
	return locks, rows.Err()
}
