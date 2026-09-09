/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"errors"
	"strings"

	"renop/internal/core"
	"renop/internal/utils"
)

func mavenLockTarget(repository, group, artifact, version string) core.ResourceLockTarget {
	return core.ResourceLockTarget{Format: "maven", Repository: repository, Name: group + ":" + artifact, Version: version}
}

func lockMavenArtifactTargetTx(tx *Tx, target core.ResourceLockTarget) error {
	var err error
	target, err = normalizeResourceLockTarget(target)
	if err != nil {
		return err
	}
	group, artifact, ok := strings.Cut(target.Name, ":")
	if !ok {
		return core.ErrResourceLockInvalid
	}
	_, err = tx.Exec(`UPDATE maven_artifacts SET updated_at = updated_at WHERE repository = ?
		AND group_id = ? AND `+resourceLockVersionColumn("maven", "artifact_id")+` = ?`,
		target.Repository, strings.ToLower(group), core.ResourceLockVersionKey("maven", artifact))
	return err
}

// IsMavenArtifactMember includes domain collaborators and both global-team bindings.
func (db *DB) IsMavenArtifactMember(repository, group, artifact, username string) (bool, error) {
	userID, err := db.mavenViewerID(username)
	if err != nil || userID == "" {
		return false, err
	}
	args := make([]any, 0, 6)
	condition := mavenInspectCondition("a", userID, nil, &args)
	args = append(args, sanitizeMavenRepository(repository), sanitizeMavenDomain(group), artifact)
	var member int
	err = db.QueryRow(`SELECT CASE WHEN `+condition+` THEN 1 ELSE 0 END FROM maven_artifacts a
		WHERE a.repository = ? AND a.group_id = ? AND a.artifact_id = ?`, args...).Scan(&member)
	return member != 0, err
}

func ensureMavenMutableTx(tx *Tx, repository, group, artifact, version string, allVersions bool) error {
	target := mavenLockTarget(repository, group, artifact, version)
	if err := lockMavenArtifactTargetTx(tx, target); err != nil {
		return err
	}
	return ensureResourceMutableQuery(tx.QueryRow, target, allVersions)
}

func (db *DB) mavenResourceName(alias string) string {
	expression := alias + `.group_id || ':' || ` + alias + `.artifact_id`
	if db.Dialect.Name() == "mysql" {
		expression = `CONCAT(` + alias + `.group_id, ':', ` + alias + `.artifact_id)`
	}
	return resourceLockVersionColumn("maven", "("+expression+")")
}

func (db *DB) mavenViewerID(username string) (string, error) {
	if username == "" || strings.EqualFold(username, "guest") {
		return "", nil
	}
	id, err := db.userIDForUsername(username)
	if errors.Is(err, core.ErrUserProfileNotFound) {
		return "", nil
	}
	return id, err
}

func mavenInspectCondition(alias, userID string, moderated []string, args *[]any) string {
	*args = append(*args, userID, userID, userID)
	return `(EXISTS (SELECT 1 FROM maven_domain_members m WHERE m.repository = '' AND m.domain = ` + alias + `.domain AND m.user_id = ?)
		OR EXISTS (SELECT 1 FROM super_team_members m WHERE m.team_prefix = ` + alias + `.super_team_prefix AND m.user_id = ?)
		OR EXISTS (SELECT 1 FROM maven_domains d JOIN super_team_members m ON m.team_prefix = d.super_team_prefix
		WHERE d.repository = '' AND d.domain = ` + alias + `.domain AND m.user_id = ?)
		OR ` + resourceRepositoryCondition(alias+".repository", normalizeResourceRepositories(moderated), args) + `)`
}

// mavenReadCondition includes explicit L0 members and either bound global team.
func (db *DB) mavenReadCondition(alias, version, userID string, moderated []string, args *[]any) string {
	return `(NOT EXISTS (SELECT 1 FROM resource_locks l WHERE l.format = 'maven' AND l.mode = 'read'
		AND l.repository = ` + alias + `.repository AND l.resource_name = ` + db.mavenResourceName(alias) + `
		AND (l.version = '' OR ` + resourceLockVersionColumn("maven", "l.version") + ` = ` + version + `))
		OR ` + mavenInspectCondition(alias, userID, moderated, args) + `)`
}

func (db *DB) filterMavenArtifactVersions(artifacts []*core.MavenArtifact, userID string, moderated []string) error {
	if len(artifacts) == 0 {
		return nil
	}
	conditions := make([]string, len(artifacts))
	args := make([]any, 0, len(artifacts)*3+3)
	byTarget := make(map[core.ResourceLockTarget]*core.MavenArtifact, len(artifacts))
	for i, artifact := range artifacts {
		conditions[i] = `(a.repository = ? AND a.group_id = ? AND a.artifact_id = ?)`
		args = append(args, artifact.Repository, artifact.GroupID, artifact.ArtifactID)
		byTarget[mavenLockTarget(artifact.Repository, artifact.GroupID, artifact.ArtifactID, "")] = artifact
	}
	where := ` WHERE (` + strings.Join(conditions, " OR ") + `) AND NOT ` + mavenInspectCondition("a", userID, moderated, &args)
	rows, err := db.Query(`SELECT DISTINCT a.repository, a.group_id, a.artifact_id FROM maven_artifacts a
		JOIN resource_locks l ON l.format = 'maven' AND l.repository = a.repository AND l.resource_name = `+db.mavenResourceName("a")+`
		AND l.mode = 'read' AND l.version != ''`+where, args...)
	if err != nil {
		return err
	}
	affected := make([]core.ResourceLockTarget, 0)
	for rows.Next() {
		target := core.ResourceLockTarget{Format: "maven"}
		var group, artifact string
		if err := rows.Scan(&target.Repository, &group, &artifact); err != nil {
			rows.Close()
			return err
		}
		target.Name = group + ":" + artifact
		affected = append(affected, target)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil || len(affected) == 0 {
		return err
	}
	conditions, args = make([]string, len(affected)), nil
	for i, target := range affected {
		artifact := byTarget[target]
		artifact.LatestVersion, artifact.VersionCount, artifact.TotalSize = "", 0, 0
		conditions[i] = `(a.repository = ? AND a.group_id = ? AND a.artifact_id = ?)`
		args = append(args, artifact.Repository, artifact.GroupID, artifact.ArtifactID)
	}
	where = ` WHERE (` + strings.Join(conditions, " OR ") + `) AND ` + db.mavenReadCondition("a", resourceLockVersionColumn("maven", "v.version"), userID, moderated, &args)
	rows, err = db.Query(`SELECT a.repository, a.group_id, a.artifact_id, v.version, v.size FROM maven_artifacts a
		JOIN maven_versions v ON v.repository = a.repository AND v.group_id = a.group_id AND v.artifact_id = a.artifact_id`+where, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var repository, group, name, version string
		var size int64
		if err := rows.Scan(&repository, &group, &name, &version, &size); err != nil {
			return err
		}
		artifact := byTarget[mavenLockTarget(repository, group, name, "")]
		artifact.VersionCount++
		artifact.TotalSize += size
		if artifact.LatestVersion == "" || utils.CompareVersions(version, artifact.LatestVersion) > 0 {
			artifact.LatestVersion = version
		}
	}
	return rows.Err()
}

func (db *DB) mavenMetadataVisibility(repository, username string, moderator bool, targets []core.ResourceLockTarget) ([]bool, error) {
	if len(targets) > 128 {
		return nil, core.ErrResourceLockInvalid
	}
	visible := make([]bool, len(targets))
	for i := range visible {
		visible[i] = true
	}
	if moderator || len(targets) == 0 {
		return visible, nil
	}
	userID, err := db.mavenViewerID(username)
	if err != nil {
		return nil, err
	}
	args := []any{userID, userID, userID, strings.ToLower(repository)}
	conditions := make([]string, len(targets))
	normalized := make([]core.ResourceLockTarget, len(targets))
	for i, target := range targets {
		target.Format, target.Repository = "maven", repository
		normalized[i], err = normalizeResourceLockTarget(target)
		if err != nil {
			return nil, err
		}
		normalized[i].Version = core.ResourceLockVersionKey("maven", normalized[i].Version)
		conditions[i] = `(l.resource_name = ? AND (l.version = '' OR ` + resourceLockVersionColumn("maven", "l.version") + ` = ?))`
		args = append(args, normalized[i].Name, normalized[i].Version)
	}
	rows, err := db.Query(`SELECT l.resource_name, l.version FROM resource_locks l
		LEFT JOIN maven_artifacts a ON a.repository = l.repository AND `+db.mavenResourceName("a")+` = l.resource_name
		LEFT JOIN maven_domain_members m ON m.repository = '' AND m.domain = a.domain AND m.user_id = ?
		LEFT JOIN super_team_members am ON am.team_prefix = a.super_team_prefix AND am.user_id = ?
		LEFT JOIN maven_domains d ON d.repository = '' AND d.domain = a.domain
		LEFT JOIN super_team_members dm ON dm.team_prefix = d.super_team_prefix AND dm.user_id = ?
		WHERE l.format = 'maven' AND l.repository = ? AND l.mode = 'read'
		AND m.user_id IS NULL AND am.user_id IS NULL AND dm.user_id IS NULL
		AND (`+strings.Join(conditions, " OR ")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hidden := make(map[core.ResourceLockTarget]bool)
	for rows.Next() {
		target := core.ResourceLockTarget{Format: "maven", Repository: strings.ToLower(repository)}
		if err := rows.Scan(&target.Name, &target.Version); err != nil {
			return nil, err
		}
		target.Version = core.ResourceLockVersionKey("maven", target.Version)
		hidden[target] = true
	}
	for i, target := range normalized {
		visible[i] = !hidden[target]
		target.Version = ""
		visible[i] = visible[i] && !hidden[target]
	}
	return visible, rows.Err()
}

// GetMavenPathLocks covers arbitrary files, metadata companions, and optional subtree mutations.
func (db *DB) GetMavenPathLocks(repository, path string, descendants bool) ([]*core.ResourceLock, error) {
	parts := strings.Split(strings.Trim(strings.ReplaceAll(path, `\`, "/"), "/"), "/")
	if len(parts) > core.MaxMavenPathParts {
		return nil, core.ErrResourceLockInvalid
	}
	args := []any{strings.ToLower(repository)}
	conditions := make([]string, 0, len(parts)+2)
	for i := 2; i < len(parts); i++ {
		target, err := normalizeResourceLockTarget(mavenLockTarget(repository, strings.Join(parts[:i], "."), parts[i], ""))
		if err != nil {
			return nil, err
		}
		condition := `resource_name = ?`
		args = append(args, target.Name)
		version := ""
		if i+1 < len(parts) {
			version = parts[i+1]
		}
		if !(descendants && version == "") && !strings.HasPrefix(strings.ToLower(version), "maven-metadata.xml") {
			condition += ` AND (version = '' OR ` + resourceLockVersionColumn("maven", "version") + ` = ?)`
			args = append(args, core.ResourceLockVersionKey("maven", version))
		}
		conditions = append(conditions, "("+condition+")")
	}
	if descendants {
		if len(parts) == 1 && parts[0] == "" {
			conditions = append(conditions, "1 = 1")
		} else {
			prefix := strings.ToLower(strings.Join(parts, "."))
			prefix = strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(prefix)
			conditions = append(conditions, `(resource_name LIKE ? ESCAPE '!' OR resource_name LIKE ? ESCAPE '!')`)
			args = append(args, prefix+".%", prefix+":%")
		}
	}
	if len(conditions) == 0 {
		return nil, nil
	}
	rows, err := db.Query(`SELECT resource_name, version, source, mode, reason, locked_at FROM resource_locks
		WHERE format = 'maven' AND repository = ? AND (`+strings.Join(conditions, " OR ")+`) LIMIT 8193`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	locks := make([]*core.ResourceLock, 0)
	for rows.Next() {
		lock := &core.ResourceLock{ResourceLockTarget: core.ResourceLockTarget{Format: "maven", Repository: strings.ToLower(repository)}}
		if err := rows.Scan(&lock.Name, &lock.Version, &lock.Source, &lock.Mode, &lock.Reason, &lock.LockedAt); err != nil {
			return nil, err
		}
		locks = append(locks, lock)
		if len(locks) > 8192 {
			return nil, core.ErrResourceLockInvalid
		}
	}
	return locks, rows.Err()
}
