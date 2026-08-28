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

	"golang.org/x/mod/semver"

	"renop/internal/core"
)

const cargoPackageColumns = `repository, normalized_name, package_name, description,
	repository_url, homepage, documentation,
	archived, admin_archived, mirrored, created_at, updated_at`

var cargoTeamRemovalSpec = &teamRemovalSpec{
	format:           "Cargo",
	table:            "cargo_members",
	resourceColumn:   "normalized_name",
	manageLevel:      core.CargoPermissionManage,
	ownerLevel:       core.CargoPermissionOwner,
	invalidBatch:     errors.New("cargo member removal batch is invalid"),
	invalidName:      errors.New("cargo member name is invalid"),
	resourceNotFound: core.ErrCargoPackageNotFound,
	permissionDenied: core.ErrCargoPermissionDenied,
	ownerCannotLeave: core.ErrCargoOwnerCannotLeave,
	lastOwner:        core.ErrCargoLastFullMember,
	lock:             lockCargoPackageTeam,
}

type cargoPackageScanner interface {
	Scan(dest ...any) error
}

func scanCargoPackage(scanner cargoPackageScanner, includeReadme bool) (*core.CargoPackage, error) {
	result := &core.CargoPackage{}
	var archived, adminArchived, mirrored int
	destinations := []any{
		&result.Repository, &result.NormalizedName, &result.Name, &result.Description,
		&result.RepositoryURL, &result.Homepage, &result.Documentation,
		&archived, &adminArchived, &mirrored, &result.CreatedAt, &result.UpdatedAt,
	}
	if includeReadme {
		destinations = append(destinations, &result.Readme)
	}
	if err := scanner.Scan(destinations...); err != nil {
		return nil, err
	}
	result.Archived = archived != 0
	result.AdminArchived = adminArchived != 0
	result.Mirrored = mirrored != 0
	return result, nil
}

func sanitizeCargoKey(repository, normalizedName string) (string, string) {
	repository = strings.ToLower(SanitizeInputString(strings.TrimSpace(repository), 64))
	normalizedName = strings.ToLower(SanitizeInputString(strings.TrimSpace(normalizedName), 64))
	return repository, normalizedName
}

func sanitizeCargoUsername(username string) string {
	return strings.ToLower(SanitizeInputString(strings.TrimSpace(username), maxTokenNameLen))
}

func (db *DB) GetCargoPackage(repository, normalizedName string) (*core.CargoPackage, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, normalizedName = sanitizeCargoKey(repository, normalizedName)
	result, err := scanCargoPackage(db.QueryRow(
		`SELECT `+cargoPackageColumns+`, COALESCE(readme, '') FROM cargo_packages WHERE repository = ? AND normalized_name = ?`,
		repository, normalizedName,
	), true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get Cargo package: %w", err)
	}
	return result, nil
}

func (db *DB) GetCargoPackageDetails(repository, normalizedName, username string) (*core.CargoPackageDetails, error) {
	pkg, err := db.GetCargoPackage(repository, normalizedName)
	if err != nil || pkg == nil {
		return nil, err
	}
	username = sanitizeCargoUsername(username)
	if username != "" {
		userID, identityErr := db.userIDForUsername(username)
		if identityErr != nil && !errors.Is(identityErr, core.ErrUserProfileNotFound) {
			return nil, identityErr
		}
		err := db.QueryRow(`SELECT permission_level FROM cargo_members
			WHERE repository = ? AND normalized_name = ? AND user_id = ?`,
			pkg.Repository, pkg.NormalizedName, userID).Scan(&pkg.PermissionLevel)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("get Cargo package permission: %w", err)
		}
	}

	versionRows, err := db.Query(`SELECT version, description, publisher, size, checksum, rust_version, license, repository_url, homepage, documentation, yanked, admin_yanked, archive_yanked, mirrored, created_at
		FROM cargo_versions WHERE repository = ? AND normalized_name = ? ORDER BY created_at DESC, version DESC`,
		pkg.Repository, pkg.NormalizedName)
	if err != nil {
		return nil, fmt.Errorf("list Cargo versions: %w", err)
	}
	versions := make([]*core.CargoVersion, 0)
	for versionRows.Next() {
		version := &core.CargoVersion{Repository: pkg.Repository, Package: pkg.NormalizedName}
		var yanked, adminYanked, archiveYanked, mirrored int
		if err := versionRows.Scan(
			&version.Version, &version.Description, &version.Publisher,
			&version.Size, &version.Checksum, &version.RustVersion, &version.License,
			&version.RepositoryURL, &version.Homepage, &version.Documentation,
			&yanked, &adminYanked, &archiveYanked, &mirrored, &version.CreatedAt,
		); err != nil {
			_ = versionRows.Close()
			return nil, fmt.Errorf("scan Cargo version: %w", err)
		}
		version.Yanked = yanked != 0
		version.AdminYanked = adminYanked != 0
		version.ArchiveYanked = archiveYanked != 0
		version.Mirrored = mirrored != 0
		versions = append(versions, version)
	}
	if err := versionRows.Err(); err != nil {
		_ = versionRows.Close()
		return nil, fmt.Errorf("iterate Cargo versions: %w", err)
	}
	if err := versionRows.Close(); err != nil {
		return nil, fmt.Errorf("close Cargo versions: %w", err)
	}

	memberRows, err := db.Query(`SELECT m.user_id, COALESCE(p.username, m.username), m.permission_level, m.added_at
		FROM cargo_members m LEFT JOIN user_profiles p ON p.user_id = m.user_id
		WHERE m.repository = ? AND m.normalized_name = ? ORDER BY m.permission_level DESC, COALESCE(p.username, m.username)`,
		pkg.Repository, pkg.NormalizedName)
	if err != nil {
		return nil, fmt.Errorf("list Cargo members: %w", err)
	}
	members := make([]*core.CargoMember, 0)
	for memberRows.Next() {
		member := &core.CargoMember{}
		if err := memberRows.Scan(&member.UserID, &member.Username, &member.Level, &member.AddedAt); err != nil {
			_ = memberRows.Close()
			return nil, fmt.Errorf("scan Cargo member: %w", err)
		}
		members = append(members, member)
	}
	if err := memberRows.Err(); err != nil {
		_ = memberRows.Close()
		return nil, fmt.Errorf("iterate Cargo members: %w", err)
	}
	if err := memberRows.Close(); err != nil {
		return nil, fmt.Errorf("close Cargo members: %w", err)
	}
	return &core.CargoPackageDetails{Package: pkg, Versions: versions, Members: members}, nil
}

func (db *DB) ListCargoPackages(repository, username string, administrator bool) ([]*core.CargoPackage, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeCargoKey(repository, "")
	username = sanitizeCargoUsername(username)
	userID, identityErr := db.userIDForUsername(username)
	if identityErr != nil && !errors.Is(identityErr, core.ErrUserProfileNotFound) {
		return nil, identityErr
	}
	query := `SELECT p.repository, p.normalized_name, p.package_name, p.description,
		p.repository_url, p.homepage, p.documentation,
		p.archived, p.admin_archived, p.mirrored, p.created_at, p.updated_at, COALESCE(m.permission_level, 0)
		FROM cargo_packages p LEFT JOIN cargo_members m ON m.repository = p.repository
		AND m.normalized_name = p.normalized_name AND m.user_id = ? WHERE p.repository = ?`
	if !administrator {
		query += ` AND m.username IS NOT NULL`
	}
	query += ` ORDER BY p.normalized_name`
	rows, err := db.Query(query, userID, repository)
	if err != nil {
		return nil, fmt.Errorf("list Cargo packages: %w", err)
	}
	defer rows.Close()
	packages := make([]*core.CargoPackage, 0)
	for rows.Next() {
		pkg := &core.CargoPackage{}
		var archived, adminArchived, mirrored int
		if err := rows.Scan(
			&pkg.Repository, &pkg.NormalizedName, &pkg.Name, &pkg.Description,
			&pkg.RepositoryURL, &pkg.Homepage, &pkg.Documentation,
			&archived, &adminArchived, &mirrored, &pkg.CreatedAt, &pkg.UpdatedAt, &pkg.PermissionLevel,
		); err != nil {
			return nil, fmt.Errorf("scan Cargo package list: %w", err)
		}
		pkg.Archived = archived != 0
		pkg.AdminArchived = adminArchived != 0
		pkg.Mirrored = mirrored != 0
		packages = append(packages, pkg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Cargo packages: %w", err)
	}
	return packages, nil
}

func (db *DB) SearchCargoPackages(repository, query string, limit, offset int) ([]*core.CargoPackage, int, error) {
	if db == nil || db.SQLDB == nil {
		return nil, 0, core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeCargoKey(repository, "")
	query = strings.ToLower(SanitizeInputString(strings.TrimSpace(query), 128))
	query = strings.NewReplacer("%", "", "_", "").Replace(query)
	if limit < 1 || limit > 100 {
		limit = 10
	}
	if offset < 0 || offset > 1000000 {
		offset = 0
	}
	pattern := "%" + query + "%"
	where := `repository = ? AND archived = 0 AND (normalized_name LIKE ? OR LOWER(description) LIKE ?)`
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM cargo_packages WHERE `+where,
		repository, pattern, pattern).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count Cargo search results: %w", err)
	}
	rows, err := db.Query(`SELECT `+cargoPackageColumns+` FROM cargo_packages WHERE `+where+
		` ORDER BY CASE WHEN normalized_name = ? THEN 0 ELSE 1 END, normalized_name LIMIT ? OFFSET ?`,
		repository, pattern, pattern, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("search Cargo packages: %w", err)
	}
	packages := make([]*core.CargoPackage, 0, limit)
	for rows.Next() {
		pkg, err := scanCargoPackage(rows, false)
		if err != nil {
			_ = rows.Close()
			return nil, 0, fmt.Errorf("scan Cargo search result: %w", err)
		}
		packages = append(packages, pkg)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, 0, fmt.Errorf("iterate Cargo search results: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, 0, fmt.Errorf("close Cargo search results: %w", err)
	}
	if len(packages) == 0 {
		return packages, total, nil
	}

	arguments := make([]any, 0, len(packages)+1)
	arguments = append(arguments, repository)
	placeholders := make([]string, 0, len(packages))
	packagesByName := make(map[string]*core.CargoPackage, len(packages))
	for _, pkg := range packages {
		placeholders = append(placeholders, "?")
		arguments = append(arguments, pkg.NormalizedName)
		packagesByName[pkg.NormalizedName] = pkg
	}
	versionRows, err := db.Query(`SELECT normalized_name, version FROM cargo_versions
		WHERE repository = ? AND yanked = 0 AND normalized_name IN (`+strings.Join(placeholders, ",")+`)`, arguments...)
	if err != nil {
		return nil, 0, fmt.Errorf("list Cargo search versions: %w", err)
	}
	for versionRows.Next() {
		var normalizedName, version string
		if err := versionRows.Scan(&normalizedName, &version); err != nil {
			_ = versionRows.Close()
			return nil, 0, fmt.Errorf("scan Cargo search version: %w", err)
		}
		pkg := packagesByName[normalizedName]
		candidate := "v" + version
		if pkg != nil && semver.IsValid(candidate) &&
			(pkg.MaxVersion == "" || semver.Compare(candidate, "v"+pkg.MaxVersion) > 0) {
			pkg.MaxVersion = version
		}
	}
	if err := versionRows.Err(); err != nil {
		_ = versionRows.Close()
		return nil, 0, fmt.Errorf("iterate Cargo search versions: %w", err)
	}
	if err := versionRows.Close(); err != nil {
		return nil, 0, fmt.Errorf("close Cargo search versions: %w", err)
	}
	return packages, total, nil
}

func (db *DB) HasCargoMembership(repository, username string) (bool, error) {
	if db == nil || db.SQLDB == nil {
		return false, core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeCargoKey(repository, "")
	username = sanitizeCargoUsername(username)
	userID, err := db.userIDForUsername(username)
	if errors.Is(err, core.ErrUserProfileNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var exists int
	err = db.QueryRow(`SELECT 1 FROM cargo_members WHERE repository = ? AND user_id = ? LIMIT 1`,
		repository, userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check Cargo membership: %w", err)
	}
	return true, nil
}

func (db *DB) RecordCargoPublication(pkg *core.CargoPackage, version *core.CargoVersion, username string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if pkg == nil || version == nil {
		return errors.New("cargo publication metadata is missing")
	}
	repository, normalizedName := sanitizeCargoKey(pkg.Repository, pkg.NormalizedName)
	username = sanitizeCargoUsername(username)
	packageName := SanitizeInputString(strings.TrimSpace(pkg.Name), 64)
	description := SanitizeInputString(strings.TrimSpace(pkg.Description), 4000)
	readme := sanitizePackageReadme(pkg.Readme)
	versionName := SanitizeInputString(strings.TrimSpace(version.Version), 128)
	if repository == "" || normalizedName == "" || username == "" || packageName == "" || versionName == "" {
		return errors.New("cargo publication metadata is invalid")
	}
	userID, err := db.ensureUserProfile(username)
	if err != nil {
		return core.ErrCargoPermissionDenied
	}
	repoURL := SanitizeInputString(strings.TrimSpace(pkg.RepositoryURL), 1024)
	homepageURL := SanitizeInputString(strings.TrimSpace(pkg.Homepage), 1024)
	docURL := SanitizeInputString(strings.TrimSpace(pkg.Documentation), 1024)
	license := SanitizeInputString(strings.TrimSpace(version.License), 255)
	rustVersion := SanitizeInputString(strings.TrimSpace(version.RustVersion), 64)
	checksum := SanitizeInputString(strings.TrimSpace(version.Checksum), 64)
	vRepoURL := SanitizeInputString(strings.TrimSpace(version.RepositoryURL), 1024)
	vHomepageURL := SanitizeInputString(strings.TrimSpace(version.Homepage), 1024)
	vDocURL := SanitizeInputString(strings.TrimSpace(version.Documentation), 1024)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Cargo publication: %w", err)
	}
	defer tx.Rollback()

	var storedName string
	var archived int
	err = tx.QueryRow(`SELECT package_name, archived FROM cargo_packages
		WHERE repository = ? AND normalized_name = ?`, repository, normalizedName).Scan(&storedName, &archived)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err = tx.Exec(`INSERT INTO cargo_packages
			(repository, normalized_name, package_name, description, readme, repository_url, homepage, documentation, archived, admin_archived, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?)`,
			repository, normalizedName, packageName, description, readme, repoURL, homepageURL, docURL, pkg.CreatedAt, pkg.UpdatedAt); err != nil {
			return fmt.Errorf("create Cargo package: %w", err)
		}
		if _, err = tx.Exec(`INSERT INTO cargo_members
			(repository, normalized_name, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
			repository, normalizedName, username, userID, core.CargoPermissionFull, pkg.CreatedAt); err != nil {
			return fmt.Errorf("create Cargo package owner: %w", err)
		}
	case err != nil:
		return fmt.Errorf("inspect Cargo package: %w", err)
	default:
		if archived != 0 {
			return core.ErrCargoPackageArchived
		}
		if storedName != packageName {
			return errors.New("cargo crate name collides with an existing package")
		}
		var level int
		if err := tx.QueryRow(`SELECT permission_level FROM cargo_members
			WHERE repository = ? AND normalized_name = ? AND user_id = ?`,
			repository, normalizedName, userID).Scan(&level); errors.Is(err, sql.ErrNoRows) || level < core.CargoPermissionPublish {
			return core.ErrCargoPermissionDenied
		} else if err != nil {
			return fmt.Errorf("inspect Cargo package permission: %w", err)
		}
	}

	var duplicate int
	err = tx.QueryRow(`SELECT 1 FROM cargo_versions WHERE repository = ? AND normalized_name = ? AND version = ?`,
		repository, normalizedName, versionName).Scan(&duplicate)
	if err == nil {
		return core.ErrCargoVersionExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect Cargo version: %w", err)
	}
	if _, err = tx.Exec(`INSERT INTO cargo_versions
		(repository, normalized_name, version, description, publisher, size, checksum, rust_version, license, repository_url, homepage, documentation, yanked, admin_yanked, archive_yanked, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, ?)`,
		repository, normalizedName, versionName, description, username, version.Size, checksum, rustVersion, license, vRepoURL, vHomepageURL, vDocURL, version.CreatedAt); err != nil {
		return fmt.Errorf("record Cargo version: %w", err)
	}
	if _, err = tx.Exec(`UPDATE cargo_packages SET description = ?, readme = ?,
		repository_url = CASE WHEN ? != '' THEN ? ELSE repository_url END,
		homepage = CASE WHEN ? != '' THEN ? ELSE homepage END,
		documentation = CASE WHEN ? != '' THEN ? ELSE documentation END,
		updated_at = ?
		WHERE repository = ? AND normalized_name = ?`,
		description, readme, repoURL, repoURL, homepageURL, homepageURL, docURL, docURL, pkg.UpdatedAt, repository, normalizedName); err != nil {
		return fmt.Errorf("update Cargo package: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Cargo publication: %w", err)
	}
	return nil
}

// RecordCargoMirrorPublication records a crate version fetched from an upstream mirror without creating a local owner.
func (db *DB) RecordCargoMirrorPublication(pkg *core.CargoPackage, version *core.CargoVersion) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if pkg == nil || version == nil {
		return errors.New("mirrored Cargo metadata is missing")
	}
	repository, normalizedName := sanitizeCargoKey(pkg.Repository, pkg.NormalizedName)
	packageName := SanitizeInputString(strings.TrimSpace(pkg.Name), 64)
	versionName := SanitizeInputString(strings.TrimSpace(version.Version), 128)
	description := SanitizeInputString(strings.TrimSpace(pkg.Description), 4000)
	readme := sanitizePackageReadme(pkg.Readme)
	createdAt := pkg.CreatedAt
	if createdAt <= 0 {
		createdAt = time.Now().UnixMilli()
	}
	updatedAt := max(pkg.UpdatedAt, createdAt)
	if repository == "" || normalizedName == "" || packageName == "" || versionName == "" {
		return errors.New("mirrored Cargo metadata is invalid")
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin mirrored Cargo publication: %w", err)
	}
	defer tx.Rollback()
	var storedName string
	err = tx.QueryRow(`SELECT package_name FROM cargo_packages WHERE repository = ? AND normalized_name = ?`,
		repository, normalizedName).Scan(&storedName)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.Exec(`INSERT INTO cargo_packages
			(repository, normalized_name, package_name, description, readme, repository_url, homepage, documentation,
			archived, admin_archived, mirrored, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, '', '', '', 0, 0, 1, ?, ?)`,
			repository, normalizedName, packageName, description, readme, createdAt, updatedAt); err != nil {
			return fmt.Errorf("create mirrored Cargo package: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect mirrored Cargo package: %w", err)
	} else {
		if storedName != packageName {
			return errors.New("mirrored Cargo crate name collides with an existing package")
		}
		if _, err := tx.Exec(`UPDATE cargo_packages SET mirrored = 1,
			description = CASE WHEN description = '' AND ? != '' THEN ? ELSE description END,
			readme = CASE WHEN COALESCE(readme, '') = '' AND ? != '' THEN ? ELSE readme END,
			updated_at = CASE WHEN ? > updated_at THEN ? ELSE updated_at END
			WHERE repository = ? AND normalized_name = ?`, description, description, readme, readme, updatedAt, updatedAt,
			repository, normalizedName); err != nil {
			return fmt.Errorf("update mirrored Cargo package: %w", err)
		}
	}
	var exists int
	err = tx.QueryRow(`SELECT 1 FROM cargo_versions WHERE repository = ? AND normalized_name = ? AND version = ?`,
		repository, normalizedName, versionName).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		versionCreatedAt := version.CreatedAt
		if versionCreatedAt <= 0 {
			versionCreatedAt = createdAt
		}
		if _, err := tx.Exec(`INSERT INTO cargo_versions
			(repository, normalized_name, version, description, publisher, size, checksum, rust_version, license,
			repository_url, homepage, documentation, yanked, admin_yanked, archive_yanked, mirrored, created_at)
			VALUES (?, ?, ?, ?, '', ?, '', '', '', '', '', '', 0, 0, 0, 1, ?)`, repository,
			normalizedName, versionName, description, max(version.Size, 0), versionCreatedAt); err != nil {
			return fmt.Errorf("record mirrored Cargo version: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect mirrored Cargo version: %w", err)
	} else if _, err := tx.Exec(`UPDATE cargo_versions SET mirrored = 1,
		size = CASE WHEN ? > size THEN ? ELSE size END
		WHERE repository = ? AND normalized_name = ? AND version = ?`,
		version.Size, version.Size, repository, normalizedName, versionName); err != nil {
		return fmt.Errorf("update mirrored Cargo version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit mirrored Cargo publication: %w", err)
	}
	return nil
}

func (db *DB) SetCargoVersionYanked(repository, normalizedName, version string, yanked, administrator bool) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, normalizedName = sanitizeCargoKey(repository, normalizedName)
	version = SanitizeInputString(strings.TrimSpace(version), 128)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Cargo yank update: %w", err)
	}
	defer tx.Rollback()
	var packageArchived, adminYanked int
	err = tx.QueryRow(`SELECT p.archived, v.admin_yanked FROM cargo_packages p JOIN cargo_versions v
		ON v.repository = p.repository AND v.normalized_name = p.normalized_name
		WHERE p.repository = ? AND p.normalized_name = ? AND v.version = ?`,
		repository, normalizedName, version).Scan(&packageArchived, &adminYanked)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrCargoVersionNotFound
	}
	if err != nil {
		return fmt.Errorf("inspect Cargo yank state: %w", err)
	}
	if !yanked && packageArchived != 0 {
		return core.ErrCargoPackageArchived
	}
	if !yanked && adminYanked != 0 && !administrator {
		return core.ErrCargoAdminYanked
	}
	adminValue := adminYanked
	if administrator {
		if yanked {
			adminValue = 1
		} else {
			adminValue = 0
		}
	}
	if _, err := tx.Exec(`UPDATE cargo_versions SET yanked = ?, admin_yanked = ?
		WHERE repository = ? AND normalized_name = ? AND version = ?`,
		boolInt(yanked), adminValue, repository, normalizedName, version); err != nil {
		return fmt.Errorf("update Cargo yank state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Cargo yank update: %w", err)
	}
	return nil
}

func (db *DB) DeleteCargoVersion(repository, normalizedName, version string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, normalizedName = sanitizeCargoKey(repository, normalizedName)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Cargo version deletion: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.Exec(`DELETE FROM cargo_versions WHERE repository = ? AND normalized_name = ? AND version = ?`,
		repository, normalizedName, SanitizeInputString(strings.TrimSpace(version), 128))
	if err != nil {
		return fmt.Errorf("delete Cargo version: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted Cargo versions: %w", err)
	}
	if rows == 0 {
		return core.ErrCargoVersionNotFound
	}
	updateQuery := `UPDATE cargo_packages SET mirrored = CASE WHEN EXISTS (
		SELECT 1 FROM cargo_versions v WHERE v.repository = cargo_packages.repository
		AND v.normalized_name = cargo_packages.normalized_name AND v.mirrored = 1
	) THEN 1 ELSE 0 END WHERE repository = ? AND normalized_name = ?`
	updateArgs := []any{repository, normalizedName}
	if db.Dialect.Name() == "clickhouse" {
		var mirroredCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM cargo_versions WHERE repository = ? AND normalized_name = ?
			AND mirrored = 1`, repository, normalizedName).Scan(&mirroredCount); err != nil {
			return fmt.Errorf("count remaining mirrored Cargo versions: %w", err)
		}
		updateQuery = `UPDATE cargo_packages SET mirrored = ? WHERE repository = ? AND normalized_name = ?`
		updateArgs = []any{boolInt(mirroredCount > 0), repository, normalizedName}
	}
	if _, err := tx.Exec(updateQuery, updateArgs...); err != nil {
		return fmt.Errorf("update Cargo package mirror provenance: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Cargo version deletion: %w", err)
	}
	return nil
}

func (db *DB) SetCargoPackageArchived(repository, normalizedName string, archived, administrator bool) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, normalizedName = sanitizeCargoKey(repository, normalizedName)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Cargo package archive update: %w", err)
	}
	defer tx.Rollback()
	var adminArchived int
	if err := tx.QueryRow(`SELECT admin_archived FROM cargo_packages WHERE repository = ? AND normalized_name = ?`,
		repository, normalizedName).Scan(&adminArchived); errors.Is(err, sql.ErrNoRows) {
		return core.ErrCargoPackageNotFound
	} else if err != nil {
		return fmt.Errorf("inspect Cargo package archive state: %w", err)
	}
	if !archived && adminArchived != 0 && !administrator {
		return core.ErrCargoAdminArchived
	}
	if archived {
		if _, err := tx.Exec(`UPDATE cargo_versions SET archive_yanked = CASE WHEN yanked = 0 THEN 1 ELSE archive_yanked END, yanked = 1
			WHERE repository = ? AND normalized_name = ?`, repository, normalizedName); err != nil {
			return fmt.Errorf("archive Cargo package versions: %w", err)
		}
		if administrator {
			adminArchived = 1
		}
	} else {
		if _, err := tx.Exec(`UPDATE cargo_versions SET yanked = CASE
			WHEN archive_yanked = 1 AND admin_yanked = 0 THEN 0 ELSE yanked END, archive_yanked = 0
			WHERE repository = ? AND normalized_name = ?`, repository, normalizedName); err != nil {
			return fmt.Errorf("restore Cargo package versions: %w", err)
		}
		if administrator {
			adminArchived = 0
		}
	}
	if _, err := tx.Exec(`UPDATE cargo_packages SET archived = ?, admin_archived = ?
		WHERE repository = ? AND normalized_name = ?`,
		boolInt(archived), adminArchived, repository, normalizedName); err != nil {
		return fmt.Errorf("update Cargo package archive state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Cargo package archive update: %w", err)
	}
	return nil
}

func (db *DB) DeleteCargoPackage(repository, normalizedName string, actedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, normalizedName = sanitizeCargoKey(repository, normalizedName)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Cargo package deletion: %w", err)
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM cargo_packages WHERE repository = ? AND normalized_name = ?`,
		repository, normalizedName).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return core.ErrCargoPackageNotFound
	} else if err != nil {
		return fmt.Errorf("inspect Cargo package deletion: %w", err)
	}
	if err := cancelCargoInvitations(tx, `repository = ? AND normalized_name = ?`, []any{repository, normalizedName}, actedAt); err != nil {
		return err
	}
	for _, table := range []string{"cargo_versions", "cargo_members", "cargo_packages"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE repository = ? AND normalized_name = ?`, repository, normalizedName); err != nil {
			return fmt.Errorf("delete Cargo package from %s: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Cargo package deletion: %w", err)
	}
	return nil
}

func (db *DB) DeleteCargoRepository(repository string, actedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeCargoKey(repository, "")
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Cargo repository cleanup: %w", err)
	}
	defer tx.Rollback()
	if err := cancelCargoInvitations(tx, `repository = ?`, []any{repository}, actedAt); err != nil {
		return err
	}
	for _, table := range []string{"cargo_versions", "cargo_members", "cargo_packages"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE repository = ?`, repository); err != nil {
			return fmt.Errorf("clean Cargo repository from %s: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Cargo repository cleanup: %w", err)
	}
	return nil
}

func cancelCargoInvitations(tx *Tx, where string, args []any, actedAt int64) error {
	rows, err := tx.Query(`SELECT id FROM cargo_invitations WHERE `+where, args...)
	if err != nil {
		return fmt.Errorf("list Cargo invitations for cancellation: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan Cargo invitation for cancellation: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate Cargo invitations for cancellation: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close Cargo invitations for cancellation: %w", err)
	}
	for _, id := range ids {
		if _, err := tx.Exec(`UPDATE user_messages SET action_status = ?, acted_at = ?,
			read_at = CASE WHEN read_at = 0 THEN ? ELSE read_at END
			WHERE id = ? AND action_status = ?`, core.MessageActionCancelled, actedAt, actedAt, id, core.MessageActionPending); err != nil {
			return fmt.Errorf("cancel Cargo invitation message: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM cargo_invitations WHERE `+where, args...); err != nil {
		return fmt.Errorf("delete Cargo invitations: %w", err)
	}
	return nil
}

func (db *DB) CreateCargoInvitations(invitations []*core.CargoInvitation, messages []*core.UserMessage) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(invitations) == 0 || len(invitations) != len(messages) || len(invitations) > 20 {
		return errors.New("cargo invitation is missing")
	}
	for i, invitation := range invitations {
		message := messages[i]
		if invitation == nil || message == nil {
			return errors.New("cargo invitation is missing")
		}
		invitation.Repository, invitation.NormalizedName = sanitizeCargoKey(invitation.Repository, invitation.NormalizedName)
		invitation.Package = SanitizeInputString(strings.TrimSpace(invitation.Package), 64)
		invitation.Inviter = sanitizeCargoUsername(invitation.Inviter)
		invitation.Recipient = sanitizeCargoUsername(invitation.Recipient)
		if invitation.Level < core.CargoPermissionPublish || invitation.Level > core.CargoPermissionOwner {
			return errors.New("cargo invitation permission level is invalid")
		}
		if invitation.ID == "" || invitation.ID != message.ID || invitation.Recipient != strings.ToLower(strings.TrimSpace(message.Recipient)) {
			return errors.New("cargo invitation message does not match its workflow record")
		}
		if err := normalizeMessage(message); err != nil {
			return err
		}
	}
	first := invitations[0]
	inviterID, err := db.ensureUserProfile(first.Inviter)
	if err != nil {
		return core.ErrCargoPermissionDenied
	}
	recipientIDs := make(map[string]string, len(invitations))
	for _, invitation := range invitations {
		recipientID, identityErr := db.ensureUserProfile(invitation.Recipient)
		if identityErr != nil {
			return core.ErrCargoPermissionDenied
		}
		recipientIDs[invitation.Recipient] = recipientID
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Cargo invitation: %w", err)
	}
	defer tx.Rollback()
	if err := lockCargoPackageTeam(tx, first.Repository, first.NormalizedName); err != nil {
		return err
	}
	var inviterLevel int
	if err := tx.QueryRow(`SELECT permission_level FROM cargo_members
		WHERE repository = ? AND normalized_name = ? AND user_id = ?`,
		first.Repository, first.NormalizedName, inviterID).Scan(&inviterLevel); errors.Is(err, sql.ErrNoRows) || inviterLevel < core.CargoPermissionManage {
		return core.ErrCargoPermissionDenied
	} else if err != nil {
		return fmt.Errorf("inspect Cargo inviter permission: %w", err)
	}
	for i, invitation := range invitations {
		message := messages[i]
		if invitation.Repository != first.Repository || invitation.NormalizedName != first.NormalizedName || invitation.Inviter != first.Inviter {
			return errors.New("cargo invitation batch targets multiple packages")
		}
		if invitation.Level == core.CargoPermissionOwner && inviterLevel < core.CargoPermissionOwner {
			return core.ErrCargoPermissionDenied
		}
		recipientID := recipientIDs[invitation.Recipient]
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM cargo_members WHERE repository = ? AND normalized_name = ? AND user_id = ?`,
			invitation.Repository, invitation.NormalizedName, recipientID).Scan(&exists); err == nil {
			return core.ErrCargoMemberExists
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect Cargo invitation recipient: %w", err)
		}
		var existingID, existingStatus string
		var existingExpiry int64
		var existingInviterLevel int
		err := tx.QueryRow(`SELECT i.id, COALESCE(m.action_status, ''), COALESCE(m.expires_at, 0),
			COALESCE(inviter.permission_level, 0) FROM cargo_invitations i
			LEFT JOIN user_messages m ON m.id = i.id AND m.recipient = i.recipient
			LEFT JOIN user_profiles inviter_profile ON inviter_profile.username = i.inviter
			LEFT JOIN cargo_members inviter ON inviter.repository = i.repository
				AND inviter.normalized_name = i.normalized_name AND inviter.user_id = inviter_profile.user_id
			WHERE i.repository = ? AND i.normalized_name = ? AND i.recipient = ?`,
			invitation.Repository, invitation.NormalizedName, invitation.Recipient).Scan(
			&existingID, &existingStatus, &existingExpiry, &existingInviterLevel,
		)
		if err == nil && existingStatus == core.MessageActionPending &&
			existingExpiry > invitation.CreatedAt && existingInviterLevel >= core.CargoPermissionManage {
			return core.ErrCargoInvitationExists
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect pending Cargo invitation: %w", err)
		}
		if err == nil {
			if err := cancelCargoInvitations(tx,
				`repository = ? AND normalized_name = ? AND recipient = ?`,
				[]any{invitation.Repository, invitation.NormalizedName, invitation.Recipient}, invitation.CreatedAt); err != nil {
				return err
			}
		}
		var dedupeKey any
		if message.DedupeKey != "" {
			dedupeKey = message.DedupeKey
		}
		if _, err := tx.Exec(`INSERT INTO user_messages (`+messageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			message.ID, message.Recipient, message.Sender, message.Kind, message.Severity,
			message.Title, message.Body, string(message.Payload), message.ActionKind, message.ActionStatus,
			message.CreatedAt, message.ReadAt, message.ActedAt, message.ExpiresAt, dedupeKey); err != nil {
			return fmt.Errorf("create Cargo invitation message: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO cargo_invitations
			(id, repository, normalized_name, package_name, inviter, recipient, permission_level, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, invitation.ID, invitation.Repository, invitation.NormalizedName,
			invitation.Package, invitation.Inviter, invitation.Recipient, invitation.Level, invitation.CreatedAt); err != nil {
			return fmt.Errorf("create Cargo invitation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Cargo invitation: %w", err)
	}
	return nil
}

func (db *DB) ForceAddCargoMembers(repository, normalizedName, packageName, actor string, usernames []string, level int) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(usernames) == 0 || len(usernames) > 20 {
		return errors.New("cargo member addition batch is invalid")
	}
	if level < core.CargoPermissionPublish || level > core.CargoPermissionOwner {
		return errors.New("cargo permission level is invalid")
	}
	repository, normalizedName = sanitizeCargoKey(repository, normalizedName)
	packageName = SanitizeInputString(strings.TrimSpace(packageName), 64)
	actor = sanitizeCargoUsername(actor)
	normalizedUsers := make([]string, 0, len(usernames))
	userIDs := make(map[string]string, len(usernames))
	seen := make(map[string]struct{}, len(usernames))
	for _, candidate := range usernames {
		username := sanitizeCargoUsername(candidate)
		if username == "" {
			return errors.New("cargo member name is invalid")
		}
		if _, duplicate := seen[username]; duplicate {
			continue
		}
		seen[username] = struct{}{}
		userID, err := db.ensureUserProfile(username)
		if err != nil {
			return core.ErrUserProfileNotFound
		}
		userIDs[username] = userID
		normalizedUsers = append(normalizedUsers, username)
	}
	if len(normalizedUsers) == 0 || (level == core.CargoPermissionOwner && len(normalizedUsers) != 1) {
		return errors.New("cargo L4 ownership can only be assigned to one member at a time")
	}
	now := time.Now().UnixMilli()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Cargo force member addition: %w", err)
	}
	defer tx.Rollback()

	if err := lockCargoPackageTeam(tx, repository, normalizedName); err != nil {
		return err
	}
	for _, username := range normalizedUsers {
		var existingLevel int
		err := tx.QueryRow(`SELECT permission_level FROM cargo_members WHERE repository = ? AND normalized_name = ? AND user_id = ?`,
			repository, normalizedName, userIDs[username]).Scan(&existingLevel)
		if err == nil {
			return core.ErrCargoMemberExists
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check Cargo member: %w", err)
		}
	}
	if level == core.CargoPermissionOwner {
		if _, err := tx.Exec(`UPDATE cargo_members SET permission_level = ?
			WHERE repository = ? AND normalized_name = ? AND permission_level = ?`,
			core.CargoPermissionManage, repository, normalizedName, core.CargoPermissionOwner); err != nil {
			return fmt.Errorf("demote previous Cargo L4 owner: %w", err)
		}
	}
	for _, username := range normalizedUsers {
		if _, err := tx.Exec(`INSERT INTO cargo_members (repository, normalized_name, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
			repository, normalizedName, username, userIDs[username], level, now); err != nil {
			return fmt.Errorf("insert Cargo member: %w", err)
		}

		if err := cancelCargoInvitations(tx, `repository = ? AND normalized_name = ? AND recipient = ?`, []any{repository, normalizedName, username}, now); err != nil {
			return err
		}

		if err := insertAcceptedMembershipMessage(tx, username, actor, "cargo_package_invite",
			"Cargo crate package membership added",
			fmt.Sprintf("%s added you to collaborate on %s with L%d permission.", actor, packageName, level), map[string]any{
				"repository": repository,
				"package":    packageName,
				"inviter":    actor,
				"level":      level,
			}, now); err != nil {
			return fmt.Errorf("create Cargo membership message: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Cargo force member addition: %w", err)
	}
	return nil
}

func (db *DB) RespondCargoInvitation(id, recipient, repository string, accept bool, actedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	id = SanitizeInputString(strings.TrimSpace(id), 64)
	recipient = sanitizeCargoUsername(recipient)
	repository, _ = sanitizeCargoKey(repository, "")
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Cargo invitation response: %w", err)
	}
	defer tx.Rollback()
	invitation := &core.CargoInvitation{ID: id, Recipient: recipient}
	err = tx.QueryRow(`SELECT repository, normalized_name, package_name, inviter, permission_level, created_at
		FROM cargo_invitations WHERE id = ? AND recipient = ? AND repository = ?`, id, recipient, repository).Scan(
		&invitation.Repository, &invitation.NormalizedName, &invitation.Package,
		&invitation.Inviter, &invitation.Level, &invitation.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrCargoInvitationInvalid
	}
	if err != nil {
		return fmt.Errorf("load Cargo invitation: %w", err)
	}
	if err := lockCargoPackageTeam(tx, invitation.Repository, invitation.NormalizedName); err != nil {
		return err
	}
	if accept {
		inviterID, identityErr := userIDForUsernameTx(tx, invitation.Inviter)
		if identityErr != nil {
			return core.ErrCargoInvitationInvalid
		}
		recipientID, identityErr := userIDForUsernameTx(tx, recipient)
		if identityErr != nil {
			return core.ErrCargoInvitationInvalid
		}
		var inviterLevel int
		if err := tx.QueryRow(`SELECT permission_level FROM cargo_members
			WHERE repository = ? AND normalized_name = ? AND user_id = ?`,
			invitation.Repository, invitation.NormalizedName, inviterID).Scan(&inviterLevel); errors.Is(err, sql.ErrNoRows) || inviterLevel < core.CargoPermissionManage {
			return core.ErrCargoInvitationInvalid
		} else if err != nil {
			return fmt.Errorf("validate Cargo inviter: %w", err)
		}
		if invitation.Level == core.CargoPermissionOwner && inviterLevel < core.CargoPermissionOwner {
			return core.ErrCargoInvitationInvalid
		}

		var memberLevel int
		err := tx.QueryRow(`SELECT permission_level FROM cargo_members
			WHERE repository = ? AND normalized_name = ? AND user_id = ?`,
			invitation.Repository, invitation.NormalizedName, recipientID).Scan(&memberLevel)
		if err == nil {
			return core.ErrCargoMemberExists
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect Cargo invitation membership: %w", err)
		}

		if invitation.Level == core.CargoPermissionOwner {
			if _, err := tx.Exec(`UPDATE cargo_members SET permission_level = ?
				WHERE repository = ? AND normalized_name = ? AND permission_level = ? AND user_id != ?`,
				core.CargoPermissionManage, invitation.Repository, invitation.NormalizedName, core.CargoPermissionOwner, recipientID); err != nil {
				return fmt.Errorf("demote previous Cargo L4 owner: %w", err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO cargo_members
			(repository, normalized_name, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
			invitation.Repository, invitation.NormalizedName, recipient, recipientID, invitation.Level, actedAt); err != nil {
			return fmt.Errorf("accept Cargo membership: %w", err)
		}
	}
	status := core.MessageActionRejected
	if accept {
		status = core.MessageActionAccepted
	}
	result, err := tx.Exec(`UPDATE user_messages SET action_status = ?, acted_at = ?,
		read_at = CASE WHEN read_at = 0 THEN ? ELSE read_at END
		WHERE id = ? AND recipient = ? AND action_kind = 'cargo_package_invite'
		AND action_status = ? AND (expires_at = 0 OR expires_at > ?)`,
		status, actedAt, actedAt, id, recipient, core.MessageActionPending, actedAt)
	if err != nil {
		return fmt.Errorf("update Cargo invitation message: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count Cargo invitation response: %w", err)
	}
	if changed != 1 {
		return core.ErrCargoInvitationInvalid
	}
	if _, err := tx.Exec(`DELETE FROM cargo_invitations WHERE id = ? AND recipient = ?`, id, recipient); err != nil {
		return fmt.Errorf("complete Cargo invitation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Cargo invitation response: %w", err)
	}
	return nil
}

func (db *DB) SetCargoMemberLevel(repository, normalizedName, actor, username string, level int) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if level < core.CargoPermissionPublish || level > core.CargoPermissionOwner {
		return errors.New("cargo permission level is invalid")
	}
	repository, normalizedName = sanitizeCargoKey(repository, normalizedName)
	actor = sanitizeCargoUsername(actor)
	username = sanitizeCargoUsername(username)
	targetID, err := db.userIDForUsername(username)
	if err != nil {
		return core.ErrCargoPackageNotFound
	}
	actorID := ""
	if actor != "" {
		actorID, err = db.userIDForUsername(actor)
		if err != nil {
			return core.ErrCargoPermissionDenied
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Cargo member update: %w", err)
	}
	defer tx.Rollback()
	if err := lockCargoPackageTeam(tx, repository, normalizedName); err != nil {
		return err
	}
	actorLevel := 0
	if actor != "" {
		if err := requireCargoMemberPermission(tx, repository, normalizedName, actorID, core.CargoPermissionManage); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT permission_level FROM cargo_members
			WHERE repository = ? AND normalized_name = ? AND user_id = ?`,
			repository, normalizedName, actorID).Scan(&actorLevel); err != nil {
			return fmt.Errorf("inspect Cargo actor permission: %w", err)
		}
	}
	var current int
	if err := tx.QueryRow(`SELECT permission_level FROM cargo_members
		WHERE repository = ? AND normalized_name = ? AND user_id = ?`, repository, normalizedName, targetID).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return core.ErrCargoPackageNotFound
	} else if err != nil {
		return fmt.Errorf("inspect Cargo member: %w", err)
	}
	if current == core.CargoPermissionOwner && level < core.CargoPermissionOwner {
		if err := requireAnotherFullCargoMember(tx, repository, normalizedName, targetID); err != nil {
			return err
		}
	}
	if level == core.CargoPermissionOwner && current != core.CargoPermissionOwner {
		if actor != "" {
			if actorLevel != core.CargoPermissionOwner || actor == username {
				return core.ErrCargoPermissionDenied
			}
			if _, err := tx.Exec(`UPDATE cargo_members SET permission_level = ?
				WHERE repository = ? AND normalized_name = ? AND user_id = ?`,
				current, repository, normalizedName, actorID); err != nil {
				return fmt.Errorf("exchange previous Cargo L4 owner permission: %w", err)
			}
		} else if _, err := tx.Exec(`UPDATE cargo_members SET permission_level = ?
			WHERE repository = ? AND normalized_name = ? AND permission_level = ? AND user_id != ?`,
			core.CargoPermissionManage, repository, normalizedName, core.CargoPermissionOwner, targetID); err != nil {
			return fmt.Errorf("demote previous Cargo L4 owner: %w", err)
		}
	}
	if _, err := tx.Exec(`UPDATE cargo_members SET permission_level = ?
		WHERE repository = ? AND normalized_name = ? AND user_id = ?`, level, repository, normalizedName, targetID); err != nil {
		return fmt.Errorf("update Cargo member: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Cargo member update: %w", err)
	}
	return nil
}

func (db *DB) RemoveCargoMember(repository, normalizedName, actor, username string) error {
	return db.RemoveCargoMembers(repository, normalizedName, actor, []string{username})
}

func (db *DB) RemoveCargoMembers(repository, normalizedName, actor string, usernames []string) error {
	repository, normalizedName = sanitizeCargoKey(repository, normalizedName)
	actor = sanitizeCargoUsername(actor)
	return db.removeTeamMembers(repository, normalizedName, actor, usernames, sanitizeCargoUsername, cargoTeamRemovalSpec)
}

func requireCargoMemberPermission(tx *Tx, repository, normalizedName, userID string, required int) error {
	if tx == nil || userID == "" {
		return core.ErrCargoPermissionDenied
	}
	var level int
	err := tx.QueryRow(`SELECT permission_level FROM cargo_members
		WHERE repository = ? AND normalized_name = ? AND user_id = ?`,
		repository, normalizedName, userID).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) || level < required {
		return core.ErrCargoPermissionDenied
	}
	if err != nil {
		return fmt.Errorf("inspect Cargo member permission: %w", err)
	}
	return nil
}

func lockCargoPackageTeam(tx *Tx, repository, normalizedName string) error {
	if _, err := tx.Exec(`UPDATE cargo_packages SET updated_at = updated_at
		WHERE repository = ? AND normalized_name = ?`, repository, normalizedName); err != nil {
		return fmt.Errorf("lock Cargo package team: %w", err)
	}
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM cargo_packages WHERE repository = ? AND normalized_name = ?`,
		repository, normalizedName).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return core.ErrCargoPackageNotFound
	} else if err != nil {
		return fmt.Errorf("inspect Cargo package team lock: %w", err)
	}
	return nil
}

func requireAnotherFullCargoMember(tx *Tx, repository, normalizedName, excludedUserID string) error {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cargo_members WHERE repository = ? AND normalized_name = ?
		AND permission_level = ? AND user_id <> ?`, repository, normalizedName, core.CargoPermissionOwner, excludedUserID).Scan(&count); err != nil {
		return fmt.Errorf("count Cargo L4 members: %w", err)
	}
	if count == 0 {
		return core.ErrCargoLastFullMember
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
