/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"renop/internal/core"
)

const npmPackageColumns = `repository, package_name, description, publisher, latest_version,
	private, archived, mirrored, publish_enabled, revision, created_at, updated_at`

const (
	maxNPMVersionsPerPackage = 5000
	maxNPMManifestUnits      = 4 << 20
)

type npmPackageScanner interface {
	Scan(dest ...any) error
}

func sanitizeNPMKey(repository, packageName string) (string, string) {
	repository = strings.ToLower(SanitizeInputString(strings.TrimSpace(repository), 64))
	packageName = strings.ToLower(SanitizeInputString(strings.TrimSpace(packageName), 214))
	return repository, packageName
}

func sanitizeNPMUsername(username string) string {
	return strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
}

func scanNPMPackage(scanner npmPackageScanner) (*core.NPMPackage, error) {
	result := &core.NPMPackage{}
	var privateValue, archived, mirrored, publishEnabled int
	if err := scanner.Scan(
		&result.Repository, &result.Name, &result.Description, &result.Publisher, &result.LatestVersion,
		&privateValue, &archived, &mirrored, &publishEnabled, &result.Revision,
		&result.CreatedAt, &result.UpdatedAt,
	); err != nil {
		return nil, err
	}
	result.Private = privateValue != 0
	result.Archived = archived != 0
	result.Mirrored = mirrored != 0
	result.PublishEnabled = publishEnabled != 0
	return result, nil
}

// CreateNPMPackage reserves a package and assigns its initial L4 owner.
func (db *DB) CreateNPMPackage(repository, packageName, owner string, private bool, createdAt int64) (*core.NPMPackage, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	owner = sanitizeNPMUsername(owner)
	if repository == "" || packageName == "" || owner == "" || owner == "guest" || createdAt <= 0 {
		return nil, core.ErrNPMPermissionDenied
	}
	ownerID, err := db.ensureUserProfile(owner)
	if err != nil {
		return nil, core.ErrNPMPermissionDenied
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin npm package creation: %w", err)
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM npm_packages WHERE repository = ? AND package_name = ?`,
		repository, packageName).Scan(&exists); err == nil {
		return nil, core.ErrNPMPackageExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("inspect npm package creation: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO npm_packages
		(repository, package_name, description, publisher, latest_version, private, archived,
		mirrored, publish_enabled, revision, created_at, updated_at)
		VALUES (?, ?, '', ?, '', ?, 0, 0, 1, 1, ?, ?)`,
		repository, packageName, owner, boolInt(private), createdAt, createdAt); err != nil {
		return nil, fmt.Errorf("create npm package: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO npm_members
		(repository, package_name, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
		repository, packageName, owner, ownerID, core.NPMPermissionOwner, createdAt); err != nil {
		return nil, fmt.Errorf("create npm package owner: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit npm package creation: %w", err)
	}
	return &core.NPMPackage{
		Repository: repository, Name: packageName, Publisher: owner, Private: private,
		PublishEnabled: true, PermissionLevel: core.NPMPermissionOwner, Revision: 1,
		CreatedAt: createdAt, UpdatedAt: createdAt,
	}, nil
}

// GetNPMPackage returns one package reservation or mirror record.
func (db *DB) GetNPMPackage(repository, packageName string) (*core.NPMPackage, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	result, err := scanNPMPackage(db.QueryRow(
		`SELECT `+npmPackageColumns+` FROM npm_packages WHERE repository = ? AND package_name = ?`,
		repository, packageName,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get npm package: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM npm_versions
		WHERE repository = ? AND package_name = ? AND unpublished = 0`, repository, packageName).Scan(&result.VersionCount); err != nil {
		return nil, fmt.Errorf("count npm versions: %w", err)
	}
	return result, nil
}

// GetNPMPackageAccess returns visibility, publication state, and exact membership.
func (db *DB) GetNPMPackageAccess(repository, packageName, username string) (
	exists, private, publishEnabled, member bool, level int, err error,
) {
	if db == nil || db.SQLDB == nil {
		return false, false, false, false, 0, core.ErrDatabaseUnavailable
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	username = sanitizeNPMUsername(username)
	userID := ""
	if username != "" && username != "guest" {
		userID, err = db.userIDForUsername(username)
		if errors.Is(err, core.ErrUserProfileNotFound) {
			err = nil
			userID = ""
		}
		if err != nil {
			return false, false, false, false, 0, err
		}
	}
	var privateValue, publishEnabledValue, memberValue int
	err = db.QueryRow(`SELECT p.private, p.publish_enabled, COALESCE(m.permission_level, 0),
		CASE WHEN m.user_id IS NULL THEN 0 ELSE 1 END
		FROM npm_packages p LEFT JOIN npm_members m ON m.repository = p.repository
		AND m.package_name = p.package_name AND m.user_id = ?
		WHERE p.repository = ? AND p.package_name = ?`, userID, repository, packageName).Scan(
		&privateValue, &publishEnabledValue, &level, &memberValue)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, false, false, 0, nil
	}
	if err != nil {
		return false, false, false, false, 0, fmt.Errorf("inspect npm package access: %w", err)
	}
	return true, privateValue != 0, publishEnabledValue != 0, memberValue != 0, level, nil
}

// HasNPMMembership reports whether an account belongs to any package in a repository.
func (db *DB) HasNPMMembership(repository, username string) (bool, error) {
	if db == nil || db.SQLDB == nil {
		return false, core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeNPMKey(repository, "")
	username = sanitizeNPMUsername(username)
	userID, err := db.userIDForUsername(username)
	if errors.Is(err, core.ErrUserProfileNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var exists int
	err = db.QueryRow(`SELECT 1 FROM npm_members WHERE repository = ? AND user_id = ? LIMIT 1`,
		repository, userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check npm membership: %w", err)
	}
	return true, nil
}

// ListNPMPackages returns one bounded package catalog with private-package filtering.
func (db *DB) ListNPMPackages(repository, username string, administrator bool, limit, offset int) ([]*core.NPMPackage, int, error) {
	return db.queryNPMPackages(repository, username, "", administrator, limit, offset)
}

// SearchNPMPackages searches one bounded npm package catalog.
func (db *DB) SearchNPMPackages(repository, query, username string, administrator bool, limit, offset int) ([]*core.NPMPackage, int, error) {
	return db.queryNPMPackages(repository, username, query, administrator, limit, offset)
}

func (db *DB) queryNPMPackages(repository, username, search string, administrator bool, limit, offset int) ([]*core.NPMPackage, int, error) {
	if db == nil || db.SQLDB == nil {
		return nil, 0, core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeNPMKey(repository, "")
	username = sanitizeNPMUsername(username)
	search = strings.ToLower(SanitizeInputString(strings.TrimSpace(search), 128))
	search = strings.NewReplacer("%", "", "_", "").Replace(search)
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 || offset > 1_000_000 {
		offset = 0
	}
	userID := ""
	if username != "" && username != "guest" {
		var err error
		userID, err = db.userIDForUsername(username)
		if err != nil && !errors.Is(err, core.ErrUserProfileNotFound) {
			return nil, 0, err
		}
	}
	where := `p.repository = ? AND p.archived = 0`
	args := []any{repository}
	if search != "" {
		where += ` AND (p.package_name LIKE ? OR LOWER(p.description) LIKE ?)`
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern)
	}
	if !administrator {
		where += ` AND (p.private = 0 OR EXISTS (SELECT 1 FROM npm_members visible
			WHERE visible.repository = p.repository AND visible.package_name = p.package_name AND visible.user_id = ?))`
		args = append(args, userID)
	}
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM npm_packages p WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count npm packages: %w", err)
	}
	queryArgs := make([]any, 0, len(args)+4)
	queryArgs = append(queryArgs, userID)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, search, limit, offset)
	rows, err := db.Query(`SELECT `+strings.ReplaceAll(npmPackageColumns, "repository", "p.repository")+`,
		(SELECT COUNT(*) FROM npm_versions v WHERE v.repository = p.repository
			AND v.package_name = p.package_name AND v.unpublished = 0),
		COALESCE((SELECT permission_level FROM npm_members own WHERE own.repository = p.repository
			AND own.package_name = p.package_name AND own.user_id = ?), 0)
		FROM npm_packages p WHERE `+where+`
		ORDER BY CASE WHEN p.package_name = ? THEN 0 ELSE 1 END, p.package_name LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list npm packages: %w", err)
	}
	packages := make([]*core.NPMPackage, 0, limit)
	for rows.Next() {
		pkg := &core.NPMPackage{}
		var privateValue, archived, mirrored, publishEnabled int
		if err := rows.Scan(
			&pkg.Repository, &pkg.Name, &pkg.Description, &pkg.Publisher, &pkg.LatestVersion,
			&privateValue, &archived, &mirrored, &publishEnabled, &pkg.Revision,
			&pkg.CreatedAt, &pkg.UpdatedAt, &pkg.VersionCount, &pkg.PermissionLevel,
		); err != nil {
			_ = rows.Close()
			return nil, 0, fmt.Errorf("scan npm package: %w", err)
		}
		pkg.Private = privateValue != 0
		pkg.Archived = archived != 0
		pkg.Mirrored = mirrored != 0
		pkg.PublishEnabled = publishEnabled != 0
		packages = append(packages, pkg)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, fmt.Errorf("iterate npm packages: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, 0, fmt.Errorf("close npm packages: %w", err)
	}
	return packages, total, nil
}

// GetNPMPackageDetails returns versions, tags, and team metadata for one package.
func (db *DB) GetNPMPackageDetails(repository, packageName, username string) (*core.NPMPackageDetails, error) {
	pkg, err := db.GetNPMPackage(repository, packageName)
	if err != nil || pkg == nil {
		return nil, err
	}
	username = sanitizeNPMUsername(username)
	member := false
	if username != "" && username != "guest" {
		userID, identityErr := db.userIDForUsername(username)
		if identityErr != nil && !errors.Is(identityErr, core.ErrUserProfileNotFound) {
			return nil, identityErr
		}
		if userID != "" {
			permissionErr := db.QueryRow(`SELECT permission_level FROM npm_members
				WHERE repository = ? AND package_name = ? AND user_id = ?`,
				pkg.Repository, pkg.Name, userID).Scan(&pkg.PermissionLevel)
			if permissionErr != nil && !errors.Is(permissionErr, sql.ErrNoRows) {
				return nil, fmt.Errorf("get npm package permission: %w", permissionErr)
			}
			member = permissionErr == nil
		}
	}
	versionRows, err := db.Query(`SELECT version, manifest_json, publisher, tarball_path, shasum,
		integrity, size, deprecated, unpublished, mirrored, created_at FROM npm_versions
		WHERE repository = ? AND package_name = ? ORDER BY created_at DESC, version DESC`, pkg.Repository, pkg.Name)
	if err != nil {
		return nil, fmt.Errorf("list npm versions: %w", err)
	}
	versions := make([]*core.NPMVersion, 0)
	for versionRows.Next() {
		version := &core.NPMVersion{Repository: pkg.Repository, Package: pkg.Name}
		var unpublished, mirrored int
		if err := versionRows.Scan(&version.Version, &version.ManifestJSON, &version.Publisher,
			&version.TarballPath, &version.Shasum, &version.Integrity, &version.Size,
			&version.Deprecated, &unpublished, &mirrored, &version.CreatedAt); err != nil {
			_ = versionRows.Close()
			return nil, fmt.Errorf("scan npm version: %w", err)
		}
		version.Unpublished = unpublished != 0
		version.Mirrored = mirrored != 0
		versions = append(versions, version)
	}
	if err := versionRows.Err(); err != nil {
		_ = versionRows.Close()
		return nil, fmt.Errorf("iterate npm versions: %w", err)
	}
	if err := versionRows.Close(); err != nil {
		return nil, fmt.Errorf("close npm versions: %w", err)
	}
	tags := make(map[string]string)
	tagRows, err := db.Query(`SELECT tag, version FROM npm_dist_tags
		WHERE repository = ? AND package_name = ? ORDER BY tag`, pkg.Repository, pkg.Name)
	if err != nil {
		return nil, fmt.Errorf("list npm dist-tags: %w", err)
	}
	for tagRows.Next() {
		var tag, version string
		if err := tagRows.Scan(&tag, &version); err != nil {
			_ = tagRows.Close()
			return nil, fmt.Errorf("scan npm dist-tag: %w", err)
		}
		tags[tag] = version
	}
	if err := tagRows.Err(); err != nil {
		_ = tagRows.Close()
		return nil, fmt.Errorf("iterate npm dist-tags: %w", err)
	}
	if err := tagRows.Close(); err != nil {
		return nil, fmt.Errorf("close npm dist-tags: %w", err)
	}
	members, err := db.ListNPMMembers(pkg.Repository, pkg.Name)
	if err != nil {
		return nil, err
	}
	return &core.NPMPackageDetails{
		Package: pkg, Versions: versions, DistTags: tags, Members: members,
		MemberCount: len(members), Member: member,
	}, nil
}

func lockNPMPackage(tx *Tx, repository, packageName string) error {
	if _, err := tx.Exec(`UPDATE npm_packages SET updated_at = updated_at
		WHERE repository = ? AND package_name = ?`, repository, packageName); err != nil {
		return fmt.Errorf("lock npm package: %w", err)
	}
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM npm_packages WHERE repository = ? AND package_name = ?`,
		repository, packageName).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return core.ErrNPMPackageNotFound
	} else if err != nil {
		return fmt.Errorf("inspect npm package lock: %w", err)
	}
	return nil
}

func upsertNPMTag(tx *Tx, repository, packageName, tag, version string, updatedAt int64) error {
	tag = strings.ToLower(SanitizeInputString(strings.TrimSpace(tag), 128))
	version = SanitizeInputString(strings.TrimSpace(version), 128)
	if tag == "" || version == "" {
		return errors.New("npm dist-tag is invalid")
	}
	result, err := tx.Exec(`UPDATE npm_dist_tags SET version = ?, updated_at = ?
		WHERE repository = ? AND package_name = ? AND tag = ?`,
		version, updatedAt, repository, packageName, tag)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed > 0 {
		return nil
	}
	_, err = tx.Exec(`INSERT INTO npm_dist_tags
		(repository, package_name, tag, version, updated_at) VALUES (?, ?, ?, ?, ?)`,
		repository, packageName, tag, version, updatedAt)
	return err
}

func npmTagVersionTx(tx *Tx, repository, packageName, tag string) (string, error) {
	var version string
	err := tx.QueryRow(`SELECT version FROM npm_dist_tags
		WHERE repository = ? AND package_name = ? AND tag = ?`, repository, packageName, tag).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return version, nil
}

// RecordNPMPublication atomically records one immutable version and its dist-tags.
func (db *DB) RecordNPMPublication(pkg *core.NPMPackage, version *core.NPMVersion,
	tags map[string]string, username string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if pkg == nil || version == nil {
		return errors.New("npm publication metadata is missing")
	}
	repository, packageName := sanitizeNPMKey(pkg.Repository, pkg.Name)
	username = sanitizeNPMUsername(username)
	versionName := SanitizeInputString(strings.TrimSpace(version.Version), 128)
	if repository == "" || packageName == "" || username == "" || versionName == "" ||
		len(version.ManifestJSON) == 0 || len(version.ManifestJSON) > 4<<20 {
		return errors.New("npm publication metadata is invalid")
	}
	userID, err := db.userIDForUsername(username)
	if err != nil {
		return core.ErrNPMPermissionDenied
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm publication: %w", err)
	}
	defer tx.Rollback()
	if err := lockNPMPackage(tx, repository, packageName); err != nil {
		return err
	}
	var archived, mirrored, publishEnabled int
	if err := tx.QueryRow(`SELECT archived, mirrored, publish_enabled FROM npm_packages
		WHERE repository = ? AND package_name = ?`, repository, packageName).Scan(
		&archived, &mirrored, &publishEnabled); err != nil {
		return fmt.Errorf("inspect npm publication package: %w", err)
	}
	if archived != 0 {
		return core.ErrNPMPackageArchived
	}
	if mirrored != 0 || publishEnabled == 0 {
		return core.ErrNPMPackageMirrored
	}
	var level int
	if err := tx.QueryRow(`SELECT permission_level FROM npm_members
		WHERE repository = ? AND package_name = ? AND user_id = ?`,
		repository, packageName, userID).Scan(&level); errors.Is(err, sql.ErrNoRows) || level < core.NPMPermissionPublish {
		return core.ErrNPMPermissionDenied
	} else if err != nil {
		return fmt.Errorf("inspect npm publication permission: %w", err)
	}
	var existing int
	if err := tx.QueryRow(`SELECT 1 FROM npm_versions WHERE repository = ? AND package_name = ? AND version = ?`,
		repository, packageName, versionName).Scan(&existing); err == nil {
		return core.ErrNPMVersionExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect npm version: %w", err)
	}
	var versionCount int
	var manifestUnits int64
	if err := tx.QueryRow(`SELECT COUNT(*), COALESCE(SUM(LENGTH(manifest_json)), 0)
		FROM npm_versions WHERE repository = ? AND package_name = ?`, repository, packageName).Scan(
		&versionCount, &manifestUnits); err != nil {
		return fmt.Errorf("inspect npm package metadata limits: %w", err)
	}
	if versionCount >= maxNPMVersionsPerPackage ||
		manifestUnits+int64(len(version.ManifestJSON)) > maxNPMManifestUnits {
		return core.ErrNPMPackageLimit
	}
	if _, err := tx.Exec(`INSERT INTO npm_versions
		(repository, package_name, version, manifest_json, publisher, tarball_path, shasum,
		integrity, size, deprecated, unpublished, mirrored, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?)`,
		repository, packageName, versionName,
		version.ManifestJSON, username,
		SanitizeInputString(version.TarballPath, 1024), strings.ToLower(SanitizeInputString(version.Shasum, 40)),
		SanitizeInputString(version.Integrity, 255), max(version.Size, 0),
		SanitizeInputString(version.Deprecated, 4000), version.CreatedAt); err != nil {
		return fmt.Errorf("insert npm version: %w", err)
	}
	for tag, tagVersion := range tags {
		if tagVersion != versionName {
			continue
		}
		if err := upsertNPMTag(tx, repository, packageName, tag, tagVersion, pkg.UpdatedAt); err != nil {
			return fmt.Errorf("set npm publication dist-tag: %w", err)
		}
	}
	latest, err := npmTagVersionTx(tx, repository, packageName, "latest")
	if err != nil {
		return fmt.Errorf("read npm latest dist-tag: %w", err)
	}
	if _, err := tx.Exec(`UPDATE npm_packages SET description = ?, publisher = ?, latest_version = ?,
		revision = revision + 1, updated_at = ? WHERE repository = ? AND package_name = ?`,
		SanitizeInputString(strings.TrimSpace(pkg.Description), 4000), username, latest,
		pkg.UpdatedAt, repository, packageName); err != nil {
		return fmt.Errorf("update npm package publication: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm publication: %w", err)
	}
	return nil
}

// RecordNPMMirrorPublication upserts a pull-only public package from an upstream packument.
func (db *DB) RecordNPMMirrorPublication(pkg *core.NPMPackage, versions []*core.NPMVersion,
	tags map[string]string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if pkg == nil {
		return errors.New("npm mirror metadata is missing")
	}
	repository, packageName := sanitizeNPMKey(pkg.Repository, pkg.Name)
	if repository == "" || packageName == "" {
		return errors.New("npm mirror metadata is invalid")
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm mirror publication: %w", err)
	}
	defer tx.Rollback()
	var mirrored int
	err = tx.QueryRow(`SELECT mirrored FROM npm_packages WHERE repository = ? AND package_name = ?`,
		repository, packageName).Scan(&mirrored)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.Exec(`INSERT INTO npm_packages
			(repository, package_name, description, publisher, latest_version, private, archived,
			mirrored, publish_enabled, revision, created_at, updated_at)
			VALUES (?, ?, ?, 'mirror', ?, ?, 0, 1, 0, 1, ?, ?)`, repository, packageName,
			SanitizeInputString(strings.TrimSpace(pkg.Description), 4000),
			SanitizeInputString(pkg.LatestVersion, 128), boolInt(pkg.Private), pkg.CreatedAt, pkg.UpdatedAt); err != nil {
			return fmt.Errorf("create mirrored npm package: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect mirrored npm package: %w", err)
	} else if mirrored == 0 {
		return core.ErrNPMPackageExists
	} else if _, err := tx.Exec(`UPDATE npm_packages SET description = ?, latest_version = ?, private = ?,
		revision = revision + 1, updated_at = ? WHERE repository = ? AND package_name = ?`,
		SanitizeInputString(strings.TrimSpace(pkg.Description), 4000),
		SanitizeInputString(pkg.LatestVersion, 128), boolInt(pkg.Private), pkg.UpdatedAt, repository, packageName); err != nil {
		return fmt.Errorf("refresh mirrored npm package: %w", err)
	}
	if _, err := tx.Exec(`UPDATE npm_versions SET unpublished = 1
		WHERE repository = ? AND package_name = ? AND mirrored = 1`, repository, packageName); err != nil {
		return fmt.Errorf("tombstone stale mirrored npm versions: %w", err)
	}
	for _, version := range versions {
		if version == nil {
			continue
		}
		versionName := SanitizeInputString(strings.TrimSpace(version.Version), 128)
		if versionName == "" || len(version.ManifestJSON) == 0 || len(version.ManifestJSON) > 4<<20 {
			continue
		}
		result, err := tx.Exec(`UPDATE npm_versions SET manifest_json = ?, tarball_path = ?, shasum = ?,
			integrity = ?, size = ?, deprecated = ?, unpublished = 0, mirrored = 1 WHERE repository = ? AND package_name = ?
			AND version = ? AND mirrored = 1`,
			version.ManifestJSON, SanitizeInputString(version.TarballPath, 1024),
			strings.ToLower(SanitizeInputString(version.Shasum, 40)), SanitizeInputString(version.Integrity, 255),
			max(version.Size, 0), SanitizeInputString(version.Deprecated, 4000), repository, packageName, versionName)
		if err != nil {
			return fmt.Errorf("refresh mirrored npm version: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect mirrored npm version refresh: %w", err)
		}
		if changed == 0 {
			if _, err := tx.Exec(`INSERT INTO npm_versions
				(repository, package_name, version, manifest_json, publisher, tarball_path, shasum,
				integrity, size, deprecated, unpublished, mirrored, created_at)
				VALUES (?, ?, ?, ?, 'mirror', ?, ?, ?, ?, ?, 0, 1, ?)`,
				repository, packageName, versionName, version.ManifestJSON,
				SanitizeInputString(version.TarballPath, 1024), strings.ToLower(SanitizeInputString(version.Shasum, 40)),
				SanitizeInputString(version.Integrity, 255), max(version.Size, 0),
				SanitizeInputString(version.Deprecated, 4000), version.CreatedAt); err != nil {
				var local int
				if checkErr := tx.QueryRow(`SELECT mirrored FROM npm_versions WHERE repository = ?
					AND package_name = ? AND version = ?`, repository, packageName, versionName).Scan(&local); checkErr != nil || local != 0 {
					return fmt.Errorf("insert mirrored npm version: %w", err)
				}
			}
		}
	}
	if _, err := tx.Exec(`DELETE FROM npm_versions WHERE repository = ? AND package_name = ?
		AND mirrored = 1 AND unpublished = 1`, repository, packageName); err != nil {
		return fmt.Errorf("remove stale mirrored npm versions: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM npm_dist_tags WHERE repository = ? AND package_name = ?`, repository, packageName); err != nil {
		return fmt.Errorf("replace mirrored npm dist-tags: %w", err)
	}
	for tag, version := range tags {
		if err := upsertNPMTag(tx, repository, packageName, tag, version, pkg.UpdatedAt); err != nil {
			return fmt.Errorf("insert mirrored npm dist-tag: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm mirror publication: %w", err)
	}
	return nil
}

// UpdateNPMTarballSize records the observed size of one mirrored tarball.
func (db *DB) UpdateNPMTarballSize(repository, tarballPath string, size int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeNPMKey(repository, "")
	tarballPath = SanitizeInputString(strings.Trim(strings.ReplaceAll(tarballPath, `\`, "/"), "/"), 1024)
	if repository == "" || tarballPath == "" || size < 0 {
		return errors.New("npm tarball size update is invalid")
	}
	_, err := db.Exec(`UPDATE npm_versions SET size = ? WHERE repository = ? AND tarball_path = ? AND mirrored = 1`,
		size, repository, tarballPath)
	if err != nil {
		return fmt.Errorf("update mirrored npm tarball size: %w", err)
	}
	return nil
}

func requireNPMPermissionTx(tx *Tx, repository, packageName, actor string, required int) error {
	actor = sanitizeNPMUsername(actor)
	if actor == "" {
		return nil
	}
	if actor == "guest" {
		return core.ErrNPMPermissionDenied
	}
	var userID string
	if err := tx.QueryRow(`SELECT user_id FROM user_profiles WHERE username = ?`, actor).Scan(&userID); err != nil {
		return core.ErrNPMPermissionDenied
	}
	var level int
	err := tx.QueryRow(`SELECT permission_level FROM npm_members
		WHERE repository = ? AND package_name = ? AND user_id = ?`,
		repository, packageName, userID).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) || level < required {
		return core.ErrNPMPermissionDenied
	}
	if err != nil {
		return fmt.Errorf("inspect npm package permission: %w", err)
	}
	return nil
}

func requireNPMRevisionTx(tx *Tx, repository, packageName string, expectedRevision int64) error {
	if expectedRevision <= 0 {
		return nil
	}
	var revision int64
	if err := tx.QueryRow(`SELECT revision FROM npm_packages WHERE repository = ? AND package_name = ?`,
		repository, packageName).Scan(&revision); err != nil {
		return err
	}
	if revision != expectedRevision {
		return core.ErrNPMRevisionConflict
	}
	return nil
}

func latestNPMVersionTx(tx *Tx, repository, packageName string) (string, error) {
	rows, err := tx.Query(`SELECT version FROM npm_versions WHERE repository = ? AND package_name = ?
		AND unpublished = 0 ORDER BY created_at DESC, version DESC`, repository, packageName)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	latest := ""
	fallback := ""
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return "", err
		}
		if fallback == "" {
			fallback = version
		}
		candidate := "v" + version
		if semver.IsValid(candidate) && (latest == "" || semver.Compare(candidate, "v"+latest) > 0) {
			latest = version
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if latest == "" {
		latest = fallback
	}
	return latest, nil
}

// SetNPMDistTag updates one tag after rechecking package-team permission and revision.
func (db *DB) SetNPMDistTag(repository, packageName, tag, version, actor string, expectedRevision int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm dist-tag update: %w", err)
	}
	defer tx.Rollback()
	if err := lockNPMPackage(tx, repository, packageName); err != nil {
		return err
	}
	if err := requireNPMPermissionTx(tx, repository, packageName, actor, core.NPMPermissionLifecycle); err != nil {
		return err
	}
	if err := requireNPMRevisionTx(tx, repository, packageName, expectedRevision); err != nil {
		return err
	}
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM npm_versions WHERE repository = ? AND package_name = ?
		AND version = ? AND unpublished = 0`, repository, packageName, version).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return core.ErrNPMVersionNotFound
	} else if err != nil {
		return fmt.Errorf("inspect npm dist-tag version: %w", err)
	}
	now := time.Now().UnixMilli()
	if err := upsertNPMTag(tx, repository, packageName, tag, version, now); err != nil {
		return fmt.Errorf("update npm dist-tag: %w", err)
	}
	latest, err := npmTagVersionTx(tx, repository, packageName, "latest")
	if err != nil {
		return fmt.Errorf("read npm latest dist-tag: %w", err)
	}
	if _, err := tx.Exec(`UPDATE npm_packages SET latest_version = ?, revision = revision + 1, updated_at = ?
		WHERE repository = ? AND package_name = ?`, latest, now, repository, packageName); err != nil {
		return fmt.Errorf("update npm package tag revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm dist-tag update: %w", err)
	}
	return nil
}

// DeleteNPMDistTag removes one tag after rechecking package-team permission and revision.
func (db *DB) DeleteNPMDistTag(repository, packageName, tag, actor string, expectedRevision int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	tag = strings.ToLower(SanitizeInputString(strings.TrimSpace(tag), 128))
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm dist-tag deletion: %w", err)
	}
	defer tx.Rollback()
	if err := lockNPMPackage(tx, repository, packageName); err != nil {
		return err
	}
	if err := requireNPMPermissionTx(tx, repository, packageName, actor, core.NPMPermissionLifecycle); err != nil {
		return err
	}
	if err := requireNPMRevisionTx(tx, repository, packageName, expectedRevision); err != nil {
		return err
	}
	result, err := tx.Exec(`DELETE FROM npm_dist_tags WHERE repository = ? AND package_name = ? AND tag = ?`,
		repository, packageName, tag)
	if err != nil {
		return fmt.Errorf("delete npm dist-tag: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect npm dist-tag deletion: %w", err)
	}
	if changed == 0 {
		return core.ErrNPMVersionNotFound
	}
	latest, err := npmTagVersionTx(tx, repository, packageName, "latest")
	if err != nil {
		return fmt.Errorf("read npm latest dist-tag: %w", err)
	}
	now := time.Now().UnixMilli()
	if _, err := tx.Exec(`UPDATE npm_packages SET latest_version = ?, revision = revision + 1, updated_at = ?
		WHERE repository = ? AND package_name = ?`, latest, now, repository, packageName); err != nil {
		return fmt.Errorf("update npm package tag deletion: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm dist-tag deletion: %w", err)
	}
	return nil
}

// SetNPMVersionDeprecated updates one version's deprecation message.
func (db *DB) SetNPMVersionDeprecated(repository, packageName, version, deprecated, actor string,
	expectedRevision int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	version = SanitizeInputString(strings.TrimSpace(version), 128)
	deprecated = SanitizeInputString(strings.TrimSpace(deprecated), 4000)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm deprecation update: %w", err)
	}
	defer tx.Rollback()
	if err := lockNPMPackage(tx, repository, packageName); err != nil {
		return err
	}
	if err := requireNPMPermissionTx(tx, repository, packageName, actor, core.NPMPermissionLifecycle); err != nil {
		return err
	}
	if err := requireNPMRevisionTx(tx, repository, packageName, expectedRevision); err != nil {
		return err
	}
	result, err := tx.Exec(`UPDATE npm_versions SET deprecated = ? WHERE repository = ?
		AND package_name = ? AND version = ? AND unpublished = 0`, deprecated, repository, packageName, version)
	if err != nil {
		return fmt.Errorf("update npm version deprecation: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect npm version deprecation: %w", err)
	}
	if changed == 0 {
		return core.ErrNPMVersionNotFound
	}
	now := time.Now().UnixMilli()
	if _, err := tx.Exec(`UPDATE npm_packages SET revision = revision + 1, updated_at = ?
		WHERE repository = ? AND package_name = ?`, now, repository, packageName); err != nil {
		return fmt.Errorf("update npm package deprecation revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm version deprecation: %w", err)
	}
	return nil
}

// UpdateNPMPackument applies npm CLI metadata edits in one revision-checked transaction.
func (db *DB) UpdateNPMPackument(repository, packageName, actor string, expectedRevision int64,
	deprecations, tags map[string]string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm packument update: %w", err)
	}
	defer tx.Rollback()
	if err := lockNPMPackage(tx, repository, packageName); err != nil {
		return err
	}
	if err := requireNPMPermissionTx(tx, repository, packageName, actor, core.NPMPermissionLifecycle); err != nil {
		return err
	}
	if err := requireNPMRevisionTx(tx, repository, packageName, expectedRevision); err != nil {
		return err
	}
	for version, deprecated := range deprecations {
		version = SanitizeInputString(strings.TrimSpace(version), 128)
		deprecated = SanitizeInputString(strings.TrimSpace(deprecated), 4000)
		result, err := tx.Exec(`UPDATE npm_versions SET deprecated = ? WHERE repository = ?
			AND package_name = ? AND version = ? AND unpublished = 0`, deprecated, repository, packageName, version)
		if err != nil {
			return fmt.Errorf("update npm packument deprecation: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect npm packument deprecation: %w", err)
		}
		if changed == 0 {
			return core.ErrNPMVersionNotFound
		}
	}
	if _, err := tx.Exec(`DELETE FROM npm_dist_tags WHERE repository = ? AND package_name = ?`,
		repository, packageName); err != nil {
		return fmt.Errorf("replace npm packument tags: %w", err)
	}
	now := time.Now().UnixMilli()
	for tag, version := range tags {
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM npm_versions WHERE repository = ? AND package_name = ?
			AND version = ? AND unpublished = 0`, repository, packageName, version).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return core.ErrNPMVersionNotFound
		} else if err != nil {
			return fmt.Errorf("inspect npm packument tag version: %w", err)
		}
		if err := upsertNPMTag(tx, repository, packageName, tag, version, now); err != nil {
			return fmt.Errorf("replace npm packument tag: %w", err)
		}
	}
	latest := tags["latest"]
	if _, err := tx.Exec(`UPDATE npm_packages SET latest_version = ?, revision = revision + 1, updated_at = ?
		WHERE repository = ? AND package_name = ?`, latest, now, repository, packageName); err != nil {
		return fmt.Errorf("update npm packument revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm packument update: %w", err)
	}
	return nil
}

// UnpublishNPMVersion tombstones one immutable version and returns its tarball path.
func (db *DB) UnpublishNPMVersion(repository, packageName, version, actor string,
	expectedRevision int64) (string, error) {
	if db == nil || db.SQLDB == nil {
		return "", core.ErrDatabaseUnavailable
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	version = SanitizeInputString(strings.TrimSpace(version), 128)
	tx, err := db.Begin()
	if err != nil {
		return "", fmt.Errorf("begin npm version unpublish: %w", err)
	}
	defer tx.Rollback()
	if err := lockNPMPackage(tx, repository, packageName); err != nil {
		return "", err
	}
	if err := requireNPMPermissionTx(tx, repository, packageName, actor, core.NPMPermissionLifecycle); err != nil {
		return "", err
	}
	if err := requireNPMRevisionTx(tx, repository, packageName, expectedRevision); err != nil {
		return "", err
	}
	var path string
	var unpublished int
	if err := tx.QueryRow(`SELECT tarball_path, unpublished FROM npm_versions WHERE repository = ?
		AND package_name = ? AND version = ?`, repository, packageName, version).Scan(&path, &unpublished); errors.Is(err, sql.ErrNoRows) {
		return "", core.ErrNPMVersionNotFound
	} else if err != nil {
		return "", fmt.Errorf("inspect npm version unpublish: %w", err)
	}
	if unpublished != 0 {
		return "", core.ErrNPMVersionNotFound
	}
	if _, err := tx.Exec(`UPDATE npm_versions SET unpublished = 1 WHERE repository = ?
		AND package_name = ? AND version = ?`, repository, packageName, version); err != nil {
		return "", fmt.Errorf("tombstone npm version: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM npm_dist_tags WHERE repository = ? AND package_name = ? AND version = ?`,
		repository, packageName, version); err != nil {
		return "", fmt.Errorf("remove npm version dist-tags: %w", err)
	}
	latest, err := latestNPMVersionTx(tx, repository, packageName)
	if err != nil {
		return "", fmt.Errorf("choose npm latest version: %w", err)
	}
	now := time.Now().UnixMilli()
	if latest != "" {
		if err := upsertNPMTag(tx, repository, packageName, "latest", latest, now); err != nil {
			return "", fmt.Errorf("restore npm latest tag: %w", err)
		}
	}
	if _, err := tx.Exec(`UPDATE npm_packages SET latest_version = ?, revision = revision + 1, updated_at = ?
		WHERE repository = ? AND package_name = ?`, latest, now, repository, packageName); err != nil {
		return "", fmt.Errorf("update npm package after unpublish: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit npm version unpublish: %w", err)
	}
	return path, nil
}

// UpdateNPMPackageDescription changes user-visible package metadata.
func (db *DB) UpdateNPMPackageDescription(repository, packageName, description, actor string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	description = SanitizeInputString(strings.TrimSpace(description), 4000)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm package description update: %w", err)
	}
	defer tx.Rollback()
	if err := lockNPMPackage(tx, repository, packageName); err != nil {
		return err
	}
	if err := requireNPMPermissionTx(tx, repository, packageName, actor, core.NPMPermissionLifecycle); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE npm_packages SET description = ?, revision = revision + 1, updated_at = ?
		WHERE repository = ? AND package_name = ?`, description, time.Now().UnixMilli(), repository, packageName); err != nil {
		return fmt.Errorf("update npm package description: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm package description update: %w", err)
	}
	return nil
}

// SetNPMPackagePrivate changes scoped-package visibility after an L4 check.
func (db *DB) SetNPMPackagePrivate(repository, packageName, actor string, private bool) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm package visibility update: %w", err)
	}
	defer tx.Rollback()
	if err := lockNPMPackage(tx, repository, packageName); err != nil {
		return err
	}
	if err := requireNPMPermissionTx(tx, repository, packageName, actor, core.NPMPermissionOwner); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE npm_packages SET private = ?, revision = revision + 1, updated_at = ?
		WHERE repository = ? AND package_name = ?`, boolInt(private), time.Now().UnixMilli(), repository, packageName); err != nil {
		return fmt.Errorf("update npm package visibility: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm package visibility update: %w", err)
	}
	return nil
}

// SetNPMPackageArchived changes package publication state after an L4 check.
func (db *DB) SetNPMPackageArchived(repository, packageName, actor string, archived bool) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm package archive update: %w", err)
	}
	defer tx.Rollback()
	if err := lockNPMPackage(tx, repository, packageName); err != nil {
		return err
	}
	if err := requireNPMPermissionTx(tx, repository, packageName, actor, core.NPMPermissionOwner); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE npm_packages SET archived = ?, revision = revision + 1, updated_at = ?
		WHERE repository = ? AND package_name = ?`, boolInt(archived), time.Now().UnixMilli(), repository, packageName); err != nil {
		return fmt.Errorf("update npm package archive state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm package archive update: %w", err)
	}
	return nil
}

// DeleteNPMPackage tombstones every version and returns the tarballs to remove.
func (db *DB) DeleteNPMPackage(repository, packageName, actor string, expectedRevision int64) ([]string, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin npm package deletion: %w", err)
	}
	defer tx.Rollback()
	if err := lockNPMPackage(tx, repository, packageName); err != nil {
		return nil, err
	}
	if err := requireNPMPermissionTx(tx, repository, packageName, actor, core.NPMPermissionOwner); err != nil {
		return nil, err
	}
	if err := requireNPMRevisionTx(tx, repository, packageName, expectedRevision); err != nil {
		return nil, err
	}
	rows, err := tx.Query(`SELECT tarball_path FROM npm_versions WHERE repository = ? AND package_name = ?
		AND unpublished = 0`, repository, packageName)
	if err != nil {
		return nil, fmt.Errorf("list npm package tarballs: %w", err)
	}
	paths := make([]string, 0)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan npm package tarball: %w", err)
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate npm package tarballs: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close npm package tarballs: %w", err)
	}
	if _, err := tx.Exec(`UPDATE npm_versions SET unpublished = 1 WHERE repository = ? AND package_name = ?`,
		repository, packageName); err != nil {
		return nil, fmt.Errorf("tombstone npm package versions: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM npm_dist_tags WHERE repository = ? AND package_name = ?`,
		repository, packageName); err != nil {
		return nil, fmt.Errorf("delete npm package dist-tags: %w", err)
	}
	if _, err := tx.Exec(`UPDATE npm_packages SET archived = 1, publish_enabled = 0, latest_version = '',
		revision = revision + 1, updated_at = ? WHERE repository = ? AND package_name = ?`,
		time.Now().UnixMilli(), repository, packageName); err != nil {
		return nil, fmt.Errorf("tombstone npm package: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit npm package deletion: %w", err)
	}
	return paths, nil
}

// DeleteNPMRepository removes npm metadata for one deleted repository.
func (db *DB) DeleteNPMRepository(repository string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeNPMKey(repository, "")
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm repository deletion: %w", err)
	}
	defer tx.Rollback()
	for _, table := range []string{"npm_invitations", "npm_members", "npm_dist_tags", "npm_versions", "npm_packages"} {
		if _, err := tx.Exec("DELETE FROM "+table+" WHERE repository = ?", repository); err != nil {
			return fmt.Errorf("delete npm repository metadata from %s: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm repository deletion: %w", err)
	}
	return nil
}
