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
	"time"

	"renop/internal/core"
	"renop/internal/utils"
)

const maxMavenDomainsPerOwner = 64

func sanitizeMavenRepository(value string) string {
	return SanitizeInputString(strings.ToLower(strings.TrimSpace(value)), 64)
}

func sanitizeMavenDomain(value string) string {
	return SanitizeInputString(strings.ToLower(strings.TrimSpace(value)), 253)
}

func sanitizeMavenUsername(value string) string {
	return SanitizeInputString(strings.ToLower(strings.TrimSpace(value)), 255)
}

const mavenDomainSelectPrefix = `d.repository, d.domain, d.verification_type, d.verification_host,
	d.verification_code, d.super_team_prefix, d.verified, d.created_at, d.verified_at, d.last_check_at,
	d.closed_at, d.release_at, d.claim_status, d.claim_verified_at,
	COALESCE(m.permission_level, 0), CASE WHEN m.user_id IS NULL THEN 0 ELSE 1 END,
	COALESCE(stm.role_level, 0), CASE WHEN stm.user_id IS NULL THEN 0 ELSE 1 END,`

const mavenDomainSelectSuffix = `,
	(SELECT COUNT(DISTINCT a.repository) FROM maven_artifacts a WHERE a.domain = d.domain),
	(SELECT COUNT(*) FROM maven_domain_members dm WHERE dm.repository = d.repository AND dm.domain = d.domain)`

const mavenDomainSelectColumns = mavenDomainSelectPrefix + `
	(SELECT COUNT(*) FROM maven_artifacts a WHERE a.domain = d.domain)` + mavenDomainSelectSuffix

const mavenRepositoryDomainSelectColumns = mavenDomainSelectPrefix + `
	catalog.artifact_count` + mavenDomainSelectSuffix

const mavenGlobalPermissionSQL = `CASE WHEN stm.user_id IS NULL THEN -1
	WHEN stm.role_level = 1 THEN 0 WHEN stm.role_level = 2 THEN 2 ELSE stm.role_level END`

const mavenEffectivePermissionSQL = `CASE WHEN COALESCE(m.permission_level, -1) >= (` + mavenGlobalPermissionSQL + `)
	THEN COALESCE(m.permission_level, -1) ELSE (` + mavenGlobalPermissionSQL + `) END`

type mavenDomainScanner interface {
	Scan(dest ...any) error
}

func scanMavenDomain(scanner mavenDomainScanner) (*core.MavenDomain, error) {
	domain := &core.MavenDomain{}
	var verified, explicitLevel, explicitMember, superRole, superMember int
	if err := scanner.Scan(&domain.Repository, &domain.Domain, &domain.VerificationType,
		&domain.VerificationHost, &domain.VerificationCode, &domain.SuperTeamPrefix, &verified,
		&domain.CreatedAt, &domain.VerifiedAt, &domain.LastCheckAt, &domain.ClosedAt, &domain.ReleaseAt,
		&domain.ClaimStatus, &domain.ClaimVerifiedAt, &explicitLevel, &explicitMember,
		&superRole, &superMember, &domain.ArtifactCount, &domain.RepositoryCount, &domain.MemberCount); err != nil {
		return nil, err
	}
	domain.Verified = verified != 0
	domain.Released = domain.ClosedAt > 0 && domain.ReleaseAt > 0 && domain.ReleaseAt <= time.Now().UnixMilli()
	domain.PermissionLevel, domain.Member = effectiveBoundPermission(
		explicitLevel, explicitMember != 0, superRole, superMember != 0)
	return domain, nil
}

// EnsureMirroredMavenDomain records an unverified namespace discovered through an upstream mirror.
func (db *DB) EnsureMirroredMavenDomain(domain string, createdAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	domain = sanitizeMavenDomain(domain)
	if domain == "" || createdAt <= 0 {
		return errors.New("mirrored Maven domain is invalid")
	}
	var existing int
	err := db.QueryRow(`SELECT 1 FROM maven_domains WHERE repository = ? AND domain = ?`,
		globalMavenRepository, domain).Scan(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect mirrored Maven domain: %w", err)
	}
	_, err = db.Exec(`INSERT INTO maven_domains
		(repository, domain, verification_type, verification_host, verification_code, verified, created_at, verified_at, last_check_at)
		VALUES (?, ?, ?, ?, '', 0, ?, 0, 0)`, globalMavenRepository, domain,
		core.MavenVerificationMirror, domain, createdAt)
	if err != nil {
		if lookupErr := db.QueryRow(`SELECT 1 FROM maven_domains WHERE repository = ? AND domain = ?`,
			globalMavenRepository, domain).Scan(&existing); lookupErr == nil {
			return nil
		}
		return fmt.Errorf("create mirrored Maven domain: %w", err)
	}
	return nil
}

// EnsureImportedMavenDomain records a verified namespace discovered in a legacy repository.
// Imported namespaces intentionally have no team until an administrator assigns one.
func (db *DB) EnsureImportedMavenDomain(domain *core.MavenDomain) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if domain == nil {
		return errors.New("imported Maven domain is missing")
	}
	name := sanitizeMavenDomain(domain.Domain)
	if name == "" || !domain.Verified {
		return errors.New("imported Maven domain is invalid")
	}
	domain.Repository = globalMavenRepository
	var existingVerified int
	var closedAt int64
	var claimStatus string
	err := db.QueryRow(`SELECT verified, closed_at, claim_status FROM maven_domains WHERE repository = ? AND domain = ?`,
		globalMavenRepository, name).Scan(&existingVerified, &closedAt, &claimStatus)
	if err == nil {
		if existingVerified != 0 || closedAt != 0 || claimStatus != "" {
			return nil
		}
		_, err = db.Exec(`UPDATE maven_domains SET verification_type = ?, verification_host = ?,
			verification_code = ?, verified = 1, verified_at = ? WHERE repository = ? AND domain = ?
			AND closed_at = 0 AND claim_status = ''`,
			SanitizeInputString(domain.VerificationType, 16), SanitizeInputString(domain.VerificationHost, 253),
			SanitizeInputString(domain.VerificationCode, 128), domain.VerifiedAt, globalMavenRepository, name)
		if err != nil {
			return fmt.Errorf("verify imported Maven domain: %w", err)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect imported Maven domain: %w", err)
	}
	_, err = db.Exec(`INSERT INTO maven_domains
		(repository, domain, verification_type, verification_host, verification_code, verified, created_at, verified_at, last_check_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?, 0)`, globalMavenRepository, name,
		SanitizeInputString(domain.VerificationType, 16), SanitizeInputString(domain.VerificationHost, 253),
		SanitizeInputString(domain.VerificationCode, 128), domain.CreatedAt, domain.VerifiedAt)
	if err != nil {
		if lookupErr := db.QueryRow(`SELECT verified FROM maven_domains WHERE repository = ? AND domain = ?`,
			globalMavenRepository, name).Scan(&existingVerified); lookupErr == nil {
			return nil
		}
		return fmt.Errorf("create imported Maven domain: %w", err)
	}
	return nil
}

// IsMavenRepositoryUpgraded reports whether legacy Maven content was cataloged.
func (db *DB) IsMavenRepositoryUpgraded(repository string) (bool, error) {
	if db == nil || db.SQLDB == nil {
		return false, core.ErrDatabaseUnavailable
	}
	var exists int
	err := db.QueryRow(`SELECT 1 FROM maven_repository_upgrades WHERE repository = ?`,
		sanitizeMavenRepository(repository)).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect Maven repository upgrade: %w", err)
	}
	return true, nil
}

// MarkMavenRepositoryUpgraded records a completed legacy catalog import.
func (db *DB) MarkMavenRepositoryUpgraded(repository string, completedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository = sanitizeMavenRepository(repository)
	if repository == "" || completedAt <= 0 {
		return errors.New("maven repository upgrade marker is invalid")
	}
	var exists int
	err := db.QueryRow(`SELECT 1 FROM maven_repository_upgrades WHERE repository = ?`, repository).Scan(&exists)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect Maven repository upgrade marker: %w", err)
	}
	if _, err := db.Exec(`INSERT INTO maven_repository_upgrades (repository, completed_at) VALUES (?, ?)`,
		repository, completedAt); err != nil {
		return fmt.Errorf("mark Maven repository upgraded: %w", err)
	}
	return nil
}

// CreateMavenDomain creates a global namespace and its initial L4 owner atomically.
func (db *DB) CreateMavenDomain(domain *core.MavenDomain, owner string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if domain == nil {
		return errors.New("maven domain is missing")
	}
	domain.Repository = globalMavenRepository
	domain.Domain = sanitizeMavenDomain(domain.Domain)
	domain.VerificationType = SanitizeInputString(strings.ToLower(strings.TrimSpace(domain.VerificationType)), 16)
	domain.VerificationHost = SanitizeInputString(strings.ToLower(strings.TrimSpace(domain.VerificationHost)), 253)
	domain.VerificationCode = SanitizeInputString(strings.TrimSpace(domain.VerificationCode), 128)
	var bindingErr error
	domain.SuperTeamPrefix, bindingErr = normalizeOptionalSuperTeamPrefix(domain.SuperTeamPrefix)
	if bindingErr != nil {
		return bindingErr
	}
	owner = sanitizeMavenUsername(owner)
	if domain.Domain == "" || domain.VerificationHost == "" || domain.VerificationCode == "" || owner == "" {
		return errors.New("maven domain is invalid")
	}
	ownerID, err := db.userIDForExistingAccount(owner)
	if err != nil {
		return core.ErrMavenPermissionDenied
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Maven domain creation: %w", err)
	}
	defer tx.Rollback()
	if err := requireSuperTeamRoleTx(tx, domain.SuperTeamPrefix, ownerID, core.SuperTeamRoleManage); err != nil {
		return err
	}
	var existingType string
	var existingVerified int
	var existingClosedAt, existingReleaseAt int64
	var existingClaimStatus string
	existing := false
	loadExisting := func() error {
		return tx.QueryRow(`SELECT verification_type, verified, closed_at, release_at, claim_status
			FROM maven_domains WHERE repository = ? AND domain = ?`, domain.Repository, domain.Domain).
			Scan(&existingType, &existingVerified, &existingClosedAt, &existingReleaseAt, &existingClaimStatus)
	}
	if err := loadExisting(); err == nil {
		existing = true
		if err := lockMavenDomain(tx, domain.Domain); err != nil {
			return err
		}
		if err := loadExisting(); err != nil {
			return fmt.Errorf("reload Maven domain claim: %w", err)
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect Maven domain: %w", err)
	}
	mirroredReservation := existing && existingType == core.MavenVerificationMirror && existingVerified == 0 &&
		existingClosedAt == 0 && existingClaimStatus == ""
	releasedReservation := existing && existingClosedAt > 0 && existingReleaseAt > 0 &&
		existingReleaseAt <= domain.CreatedAt
	if existing && !mirroredReservation && !releasedReservation {
		if existingClosedAt > 0 {
			return core.ErrMavenDomainLocked
		}
		return core.ErrMavenDomainExists
	}
	var owned int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM maven_domain_members WHERE user_id = ? AND permission_level = ?
		AND NOT (repository = ? AND domain = ?)`, ownerID, core.MavenPermissionOwner,
		globalMavenRepository, domain.Domain).Scan(&owned); err != nil {
		return fmt.Errorf("count Maven domain ownerships: %w", err)
	}
	if owned >= maxMavenDomainsPerOwner {
		return errors.New("maven domain ownership limit reached")
	}
	verified := 0
	storedVerifiedAt := domain.VerifiedAt
	if domain.Verified {
		verified = 1
	}
	claimStatus := ""
	claimVerifiedAt := int64(0)
	if releasedReservation {
		verified = 0
		storedVerifiedAt = 0
		claimStatus = core.MavenDomainClaimAwaitingVerification
		if domain.Verified {
			claimStatus = core.MavenDomainClaimPending
			claimVerifiedAt = domain.VerifiedAt
		}
		if err := cancelMavenInvitations(tx, `repository = ? AND domain = ?`,
			[]any{globalMavenRepository, domain.Domain}, domain.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM maven_domain_members WHERE repository = ? AND domain = ?`,
			globalMavenRepository, domain.Domain); err != nil {
			return fmt.Errorf("replace released Maven domain members: %w", err)
		}
	}
	if mirroredReservation || releasedReservation {
		result, err := tx.Exec(`UPDATE maven_domains SET verification_type = ?, verification_host = ?,
			verification_code = ?, super_team_prefix = ?, verified = ?, created_at = ?, verified_at = ?, last_check_at = 0,
			closed_at = 0, release_at = 0, claim_status = ?, claim_verified_at = ?
			WHERE repository = ? AND domain = ? AND verified = 0 AND
			((verification_type = ? AND closed_at = 0 AND claim_status = '') OR
			(closed_at = ? AND release_at = ? AND release_at <= ?))`,
			domain.VerificationType, domain.VerificationHost, domain.VerificationCode, domain.SuperTeamPrefix, verified,
			domain.CreatedAt, storedVerifiedAt, claimStatus, claimVerifiedAt, domain.Repository, domain.Domain,
			core.MavenVerificationMirror, existingClosedAt, existingReleaseAt, domain.CreatedAt)
		if err != nil {
			return fmt.Errorf("claim Maven domain: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("count claimed Maven domains: %w", err)
		}
		if changed != 1 {
			return core.ErrMavenDomainExists
		}
	} else if _, err := tx.Exec(`INSERT INTO maven_domains
		(repository, domain, verification_type, verification_host, verification_code, super_team_prefix,
		verified, created_at, verified_at, last_check_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`, domain.Repository, domain.Domain,
		domain.VerificationType, domain.VerificationHost, domain.VerificationCode, domain.SuperTeamPrefix, verified,
		domain.CreatedAt, domain.VerifiedAt); err != nil {
		return fmt.Errorf("create Maven domain: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO maven_domain_members
		(repository, domain, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
		domain.Repository, domain.Domain, owner, ownerID, core.MavenPermissionOwner, domain.CreatedAt); err != nil {
		return fmt.Errorf("create Maven domain owner: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Maven domain creation: %w", err)
	}
	if releasedReservation {
		domain.Verified = false
		domain.VerifiedAt = 0
		domain.ClosedAt = 0
		domain.ReleaseAt = 0
		domain.ClaimStatus = claimStatus
		domain.ClaimVerifiedAt = claimVerifiedAt
	}
	return nil
}

// ListMavenDomains lists verified public domains plus domains visible to the caller.
func (db *DB) ListMavenDomains(username string, includeAll bool) ([]*core.MavenDomain, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	username = sanitizeMavenUsername(username)
	userID := ""
	if username != "" && username != "guest" {
		if resolved, err := db.userIDForUsername(username); err == nil {
			userID = resolved
		} else if !errors.Is(err, core.ErrUserProfileNotFound) {
			return nil, err
		}
	}
	query := `SELECT ` + mavenDomainSelectColumns + `
		FROM maven_domains d LEFT JOIN maven_domain_members m ON m.repository = d.repository
		AND m.domain = d.domain AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = d.super_team_prefix AND stm.user_id = ?
		WHERE d.repository = ?`
	if !includeAll {
		query += ` AND (d.verified = 1 OR m.user_id IS NOT NULL OR stm.user_id IS NOT NULL)`
	}
	query += ` ORDER BY d.domain`
	rows, err := db.Query(query, userID, userID, globalMavenRepository)
	if err != nil {
		return nil, fmt.Errorf("list Maven domains: %w", err)
	}
	defer rows.Close()
	domains := make([]*core.MavenDomain, 0)
	for rows.Next() {
		domain, scanErr := scanMavenDomain(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan Maven domain: %w", scanErr)
		}
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Maven domains: %w", err)
	}
	return domains, nil
}

// ListManagedMavenDomains returns one filtered page for the account domain-management view.
func (db *DB) ListManagedMavenDomains(options core.MavenDomainListOptions) ([]*core.MavenDomain, int, error) {
	if db == nil || db.SQLDB == nil {
		return nil, 0, core.ErrDatabaseUnavailable
	}
	options.Username = sanitizeMavenUsername(options.Username)
	if options.Limit <= 0 {
		options.Limit = 20
	} else if options.Limit > 100 {
		options.Limit = 100
	}
	if options.Offset < 0 {
		options.Offset = 0
	}
	userID := ""
	if options.Username != "" && options.Username != "guest" {
		resolved, err := db.userIDForUsername(options.Username)
		if err != nil {
			return nil, 0, err
		}
		userID = resolved
	}
	where := []string{"d.repository = ?"}
	args := []any{userID, userID, globalMavenRepository}
	if !options.Administrator {
		where = append(where, "(m.user_id IS NOT NULL OR stm.user_id IS NOT NULL)")
	}
	if options.Filtered {
		filters := make([]string, 0, len(options.PermissionLevels)+3)
		for _, level := range options.PermissionLevels {
			if level < core.MavenPermissionRead || level > core.MavenPermissionOwner {
				continue
			}
			filters = append(filters, mavenEffectivePermissionSQL+" = ?")
			args = append(args, level)
		}
		if options.IncludeUnverified {
			filters = append(filters, "d.verified = 0")
		}
		if options.IncludeMirrored {
			filters = append(filters, "d.verification_type = ?")
			args = append(args, core.MavenVerificationMirror)
		}
		if options.IncludeClaimReview {
			filters = append(filters, "d.claim_status = ?")
			args = append(args, core.MavenDomainClaimPending)
		}
		if len(filters) == 0 {
			return []*core.MavenDomain{}, 0, nil
		}
		where = append(where, "("+strings.Join(filters, " OR ")+")")
	}
	from := ` FROM maven_domains d LEFT JOIN maven_domain_members m ON m.repository = d.repository
		AND m.domain = d.domain AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = d.super_team_prefix AND stm.user_id = ?
		WHERE ` + strings.Join(where, " AND ")
	var total int
	if err := db.QueryRow(`SELECT COUNT(*)`+from, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count managed Maven domains: %w", err)
	}
	query := `SELECT ` + mavenDomainSelectColumns +
		from + ` ORDER BY d.domain LIMIT ? OFFSET ?`
	pageArgs := append(append([]any(nil), args...), options.Limit, options.Offset)
	rows, err := db.Query(query, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list managed Maven domains: %w", err)
	}
	defer rows.Close()
	domains := make([]*core.MavenDomain, 0, min(total, options.Limit))
	for rows.Next() {
		domain, scanErr := scanMavenDomain(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan managed Maven domain: %w", scanErr)
		}
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate managed Maven domains: %w", err)
	}
	return domains, total, nil
}

// ListMavenRepositoryDomains lists verified global namespaces that contain artifacts in one repository.
func (db *DB) ListMavenRepositoryDomains(repository, username string) ([]*core.MavenDomain, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository = sanitizeMavenRepository(repository)
	username = sanitizeMavenUsername(username)
	if repository == "" {
		return nil, errors.New("maven repository is invalid")
	}
	userID := ""
	if username != "" && username != "guest" {
		if resolved, err := db.userIDForUsername(username); err == nil {
			userID = resolved
		} else if !errors.Is(err, core.ErrUserProfileNotFound) {
			return nil, err
		}
	}
	rows, err := db.Query(`SELECT `+mavenRepositoryDomainSelectColumns+`
		FROM maven_domains d JOIN (
			SELECT domain, COUNT(*) AS artifact_count FROM maven_artifacts
			WHERE repository = ? GROUP BY domain
		) catalog ON catalog.domain = d.domain
		LEFT JOIN maven_domain_members m ON m.repository = d.repository
		AND m.domain = d.domain AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = d.super_team_prefix AND stm.user_id = ?
		WHERE d.repository = ? AND d.verified = 1 ORDER BY d.domain`,
		repository, userID, userID, globalMavenRepository)
	if err != nil {
		return nil, fmt.Errorf("list Maven repository domains: %w", err)
	}
	defer rows.Close()
	domains := make([]*core.MavenDomain, 0)
	for rows.Next() {
		domain, scanErr := scanMavenDomain(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan Maven repository domain: %w", scanErr)
		}
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Maven repository domains: %w", err)
	}
	return domains, nil
}

// SearchMavenRepositoryDomains returns a bounded domain page containing artifacts in one repository.
func (db *DB) SearchMavenRepositoryDomains(repository, query string, limit int) ([]*core.MavenDomain, int, error) {
	if db == nil || db.SQLDB == nil {
		return nil, 0, core.ErrDatabaseUnavailable
	}
	repository = sanitizeMavenRepository(repository)
	query = SanitizeInputString(strings.ToLower(strings.TrimSpace(query)), 128)
	if repository == "" || query == "" || limit < 1 || limit > 100 {
		return nil, 0, errors.New("maven repository domain search is invalid")
	}
	pattern := "%" + query + "%"
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM maven_domains d WHERE d.repository = ? AND d.verified = 1
		AND LOWER(d.domain) LIKE ? AND EXISTS (
			SELECT 1 FROM maven_artifacts a WHERE a.repository = ? AND a.domain = d.domain
		)`, globalMavenRepository, pattern, repository).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count Maven repository domain search: %w", err)
	}
	rows, err := db.Query(`SELECT d.domain, d.super_team_prefix, d.verified_at, COUNT(a.artifact_id)
		FROM maven_domains d JOIN maven_artifacts a ON a.domain = d.domain AND a.repository = ?
		WHERE d.repository = ? AND d.verified = 1 AND LOWER(d.domain) LIKE ?
		GROUP BY d.domain, d.verified_at ORDER BY d.domain LIMIT ?`, repository, globalMavenRepository, pattern, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("search Maven repository domains: %w", err)
	}
	defer rows.Close()
	domains := make([]*core.MavenDomain, 0, min(limit, total))
	for rows.Next() {
		domain := &core.MavenDomain{Repository: globalMavenRepository, Verified: true}
		if err := rows.Scan(&domain.Domain, &domain.SuperTeamPrefix, &domain.VerifiedAt, &domain.ArtifactCount); err != nil {
			return nil, 0, fmt.Errorf("scan Maven repository domain search: %w", err)
		}
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate Maven repository domain search: %w", err)
	}
	return domains, total, nil
}

// GetMavenDomainDetails returns one domain and its current team.
func (db *DB) GetMavenDomainDetails(domain, username string) (*core.MavenDomainDetails, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	domain = sanitizeMavenDomain(domain)
	username = sanitizeMavenUsername(username)
	userID := ""
	if username != "" && username != "guest" {
		if resolved, err := db.userIDForUsername(username); err == nil {
			userID = resolved
		} else if !errors.Is(err, core.ErrUserProfileNotFound) {
			return nil, err
		}
	}
	result := &core.MavenDomainDetails{}
	domainRecord, err := scanMavenDomain(db.QueryRow(`SELECT `+mavenDomainSelectColumns+`
		FROM maven_domains d LEFT JOIN maven_domain_members m ON m.repository = d.repository
		AND m.domain = d.domain AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = d.super_team_prefix AND stm.user_id = ?
		WHERE d.repository = ? AND d.domain = ?`, userID, userID, globalMavenRepository, domain))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrMavenDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load Maven domain: %w", err)
	}
	result.Domain = domainRecord
	rows, err := db.Query(`SELECT COALESCE(user_id, ''), username, permission_level, added_at FROM maven_domain_members
		WHERE repository = ? AND domain = ? ORDER BY permission_level DESC, username`, globalMavenRepository, domain)
	if err != nil {
		return nil, fmt.Errorf("list Maven domain members: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		member := &core.MavenMember{}
		if err := rows.Scan(&member.UserID, &member.Username, &member.Level, &member.AddedAt); err != nil {
			return nil, fmt.Errorf("scan Maven domain member: %w", err)
		}
		result.Members = append(result.Members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Maven domain members: %w", err)
	}
	return result, nil
}

// ReserveMavenVerificationAttempt rate-limits and records one external verification check.
func (db *DB) ReserveMavenVerificationAttempt(domain, actor string, administrator bool, checkedAt, minimumPrevious int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	domain = sanitizeMavenDomain(domain)
	actor = sanitizeMavenUsername(actor)
	if db.Dialect.Name() == "clickhouse" {
		actorID := ""
		if !administrator {
			var identityErr error
			actorID, identityErr = db.userIDForUsername(actor)
			if identityErr != nil {
				return core.ErrMavenPermissionDenied
			}
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin ClickHouse Maven verification reservation: %w", err)
		}
		defer tx.Rollback()
		if !administrator {
			if err := requireMavenMemberPermission(
				tx, domain, actorID, core.MavenPermissionOwner); err != nil {
				return err
			}
		}
		result, err := tx.Exec(`UPDATE maven_domains SET last_check_at = ? WHERE repository = ? AND domain = ?
			AND verified = 0 AND closed_at = 0 AND claim_status <> ? AND last_check_at <= ?`, checkedAt,
			globalMavenRepository, domain, core.MavenDomainClaimPending, minimumPrevious)
		if err != nil {
			return fmt.Errorf("reserve ClickHouse Maven verification attempt: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("count ClickHouse Maven verification reservation: %w", err)
		}
		if changed == 0 {
			return core.ErrMavenVerificationRateLimit
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit ClickHouse Maven verification reservation: %w", err)
		}
		return nil
	}
	var updateResult result
	var err error
	if administrator {
		updateResult, err = db.Exec(`UPDATE maven_domains SET last_check_at = ? WHERE repository = ? AND domain = ?
			AND verified = 0 AND closed_at = 0 AND claim_status <> ? AND last_check_at <= ?`, checkedAt,
			globalMavenRepository, domain, core.MavenDomainClaimPending, minimumPrevious)
	} else {
		actorID, identityErr := db.userIDForUsername(actor)
		if identityErr != nil {
			return core.ErrMavenPermissionDenied
		}
		updateResult, err = db.Exec(`UPDATE maven_domains SET last_check_at = ? WHERE repository = ? AND domain = ?
			AND verified = 0 AND closed_at = 0 AND claim_status <> ? AND last_check_at <= ? AND (EXISTS (
				SELECT 1 FROM maven_domain_members m WHERE m.repository = maven_domains.repository
				AND m.domain = maven_domains.domain AND m.user_id = ? AND m.permission_level = ?
			) OR EXISTS (SELECT 1 FROM super_team_members stm
				WHERE stm.team_prefix = maven_domains.super_team_prefix AND stm.user_id = ? AND stm.role_level = ?
			))`, checkedAt, globalMavenRepository, domain, core.MavenDomainClaimPending, minimumPrevious,
			actorID, core.MavenPermissionOwner,
			actorID, core.SuperTeamRoleOwner)
	}
	if err != nil {
		return fmt.Errorf("reserve Maven verification attempt: %w", err)
	}
	changed, err := updateResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("count Maven verification reservation: %w", err)
	}
	if changed == 0 {
		return core.ErrMavenVerificationRateLimit
	}
	return nil
}

// MarkMavenDomainVerified completes verification only if the assigned code still matches.
func (db *DB) MarkMavenDomainVerified(domain, code string, verifiedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	domain = sanitizeMavenDomain(domain)
	code = SanitizeInputString(strings.TrimSpace(code), 128)
	if domain == "" || verifiedAt <= 0 {
		return core.ErrMavenVerificationFailed
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Maven domain verification: %w", err)
	}
	defer tx.Rollback()
	if err := lockMavenDomain(tx, domain); err != nil {
		return err
	}
	var storedCode, claimStatus string
	var verified int
	var closedAt int64
	if err := tx.QueryRow(`SELECT verification_code, verified, closed_at, claim_status FROM maven_domains
		WHERE repository = ? AND domain = ?`, globalMavenRepository, domain).
		Scan(&storedCode, &verified, &closedAt, &claimStatus); err != nil {
		return fmt.Errorf("inspect Maven domain verification state: %w", err)
	}
	if storedCode != code || verified != 0 {
		return core.ErrMavenVerificationFailed
	}
	if closedAt != 0 {
		return core.ErrMavenDomainClosed
	}
	var result result
	switch claimStatus {
	case "":
		result, err = tx.Exec(`UPDATE maven_domains SET verified = 1, verified_at = ?
			WHERE repository = ? AND domain = ? AND verification_code = ? AND verified = 0
			AND closed_at = 0 AND claim_status = ''`, verifiedAt, globalMavenRepository, domain, code)
	case core.MavenDomainClaimAwaitingVerification, core.MavenDomainClaimRejected:
		result, err = tx.Exec(`UPDATE maven_domains SET claim_status = ?, claim_verified_at = ?
			WHERE repository = ? AND domain = ? AND verification_code = ? AND verified = 0
			AND closed_at = 0 AND claim_status = ?`, core.MavenDomainClaimPending, verifiedAt,
			globalMavenRepository, domain, code, claimStatus)
	default:
		return core.ErrMavenClaimReviewInvalid
	}
	if err != nil {
		return fmt.Errorf("verify Maven domain: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return core.ErrMavenVerificationFailed
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Maven domain verification: %w", err)
	}
	return nil
}

func hasPendingMavenDomainReviewTx(tx *Tx, domain string) (bool, error) {
	rows, err := tx.Query(`SELECT kind, resource_type, resource_key FROM review_tasks WHERE status = ?
		AND resource_type IN (?, ?)`, core.ReviewStatusPending,
		core.ReviewResourceMavenDomain, core.ReviewResourceMavenArtifact)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, resourceType, resourceKey string
		if err := rows.Scan(&kind, &resourceType, &resourceKey); err != nil {
			return false, err
		}
		if resourceType == core.ReviewResourceMavenDomain {
			if sanitizeMavenDomain(resourceKey) == domain {
				return true, nil
			}
			continue
		}
		if kind == core.ReviewKindPublication {
			decoded, _, valid := decodePublicationReviewKey(resourceKey)
			if !valid {
				continue
			}
			resourceKey = decoded
		}
		groupID, _, valid := splitMavenArtifactReviewKey(resourceKey)
		if valid && (groupID == domain || strings.HasPrefix(groupID, domain+".")) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// CloseMavenDomain makes a namespace read-only and reserves it for 31 days before another claim is accepted.
func (db *DB) CloseMavenDomain(domain, actor string, administrator bool, closedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	domain = sanitizeMavenDomain(domain)
	actor = sanitizeMavenUsername(actor)
	if domain == "" || actor == "" || closedAt <= 0 ||
		closedAt > (1<<63-1)-core.MavenDomainReleaseLockMillis {
		return core.ErrMavenPermissionDenied
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Maven domain closure: %w", err)
	}
	defer tx.Rollback()
	if err := lockMavenDomain(tx, domain); err != nil {
		return err
	}
	if !administrator {
		actorID, err := userIDForUsernameTx(tx, actor)
		if err != nil {
			return core.ErrMavenPermissionDenied
		}
		if err := requireMavenMemberPermission(tx, domain, actorID, core.MavenPermissionOwner); err != nil {
			return err
		}
	}
	var verificationType string
	var existingClosedAt int64
	if err := tx.QueryRow(`SELECT verification_type, closed_at FROM maven_domains WHERE repository = ? AND domain = ?`,
		globalMavenRepository, domain).Scan(&verificationType, &existingClosedAt); err != nil {
		return fmt.Errorf("inspect Maven domain closure state: %w", err)
	}
	if verificationType == core.MavenVerificationMirror {
		return core.ErrMavenPermissionDenied
	}
	if existingClosedAt != 0 {
		return core.ErrMavenDomainClosed
	}
	pending, err := hasPendingMavenDomainReviewTx(tx, domain)
	if err != nil {
		return fmt.Errorf("inspect pending Maven domain reviews: %w", err)
	}
	if pending {
		return core.ErrMavenDomainReviewPending
	}
	if err := cancelMavenInvitations(tx, `repository = ? AND domain = ?`,
		[]any{globalMavenRepository, domain}, closedAt); err != nil {
		return err
	}
	result, err := tx.Exec(`UPDATE maven_domains SET verified = 0, closed_at = ?, release_at = ?,
		claim_status = '', claim_verified_at = 0, last_check_at = 0
		WHERE repository = ? AND domain = ? AND closed_at = 0`, closedAt,
		closedAt+core.MavenDomainReleaseLockMillis, globalMavenRepository, domain)
	if err != nil {
		return fmt.Errorf("close Maven domain: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return core.ErrMavenDomainClosed
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Maven domain closure: %w", err)
	}
	return nil
}

// ReviewMavenDomainClaim activates or rejects a released-domain claim after ownership proof.
func (db *DB) ReviewMavenDomainClaim(domain, decision string, reviewedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	domain = sanitizeMavenDomain(domain)
	decision = strings.ToLower(strings.TrimSpace(decision))
	if domain == "" || reviewedAt <= 0 ||
		(decision != core.ReviewStatusApproved && decision != core.ReviewStatusRejected) {
		return core.ErrMavenClaimReviewInvalid
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Maven domain claim review: %w", err)
	}
	defer tx.Rollback()
	if err := lockMavenDomain(tx, domain); err != nil {
		return err
	}
	var claimStatus string
	var closedAt int64
	if err := tx.QueryRow(`SELECT claim_status, closed_at FROM maven_domains WHERE repository = ? AND domain = ?`,
		globalMavenRepository, domain).Scan(&claimStatus, &closedAt); err != nil {
		return fmt.Errorf("inspect Maven domain claim review: %w", err)
	}
	if closedAt != 0 || claimStatus != core.MavenDomainClaimPending {
		return core.ErrMavenClaimReviewInvalid
	}
	verified := 0
	verifiedAt := int64(0)
	nextStatus := core.MavenDomainClaimRejected
	if decision == core.ReviewStatusApproved {
		verified = 1
		verifiedAt = reviewedAt
		nextStatus = ""
	}
	result, err := tx.Exec(`UPDATE maven_domains SET verified = ?, verified_at = ?, claim_status = ?
		WHERE repository = ? AND domain = ? AND closed_at = 0 AND claim_status = ?`, verified,
		verifiedAt, nextStatus, globalMavenRepository, domain, core.MavenDomainClaimPending)
	if err != nil {
		return fmt.Errorf("review Maven domain claim: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return core.ErrMavenClaimReviewInvalid
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Maven domain claim review: %w", err)
	}
	return nil
}

// HasMavenMembership reports whether a user belongs to any global Maven domain.
func (db *DB) HasMavenMembership(username string) (bool, error) {
	if db == nil || db.SQLDB == nil {
		return false, core.ErrDatabaseUnavailable
	}
	userID, err := db.userIDForUsername(sanitizeMavenUsername(username))
	if errors.Is(err, core.ErrUserProfileNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var exists int
	err = db.QueryRow(`SELECT 1 FROM maven_domains d
		LEFT JOIN maven_domain_members m ON m.repository = d.repository AND m.domain = d.domain AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = d.super_team_prefix AND stm.user_id = ?
		WHERE d.repository = ? AND (m.user_id IS NOT NULL OR stm.user_id IS NOT NULL) LIMIT 1`,
		userID, userID, globalMavenRepository).Scan(&exists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("inspect Maven membership: %w", err)
	}
	if err == nil && exists != 0 {
		return true, nil
	}
	err = db.QueryRow(`SELECT 1 FROM maven_artifacts a JOIN super_team_members stm
		ON stm.team_prefix = a.super_team_prefix AND stm.user_id = ?
		WHERE a.super_team_prefix != '' LIMIT 1`, userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect Maven artifact global-team membership: %w", err)
	}
	return true, nil
}

// RecordMavenPublication upserts one catalog artifact and version after storage publication.
func (db *DB) RecordMavenPublication(artifact *core.MavenArtifact, version *core.MavenVersion) error {
	return db.recordMavenPublication(artifact, version, false)
}

// MavenArtifactExists reports whether a local or mirrored catalog package already exists.
func (db *DB) MavenArtifactExists(repository, groupID, artifactID string) (bool, error) {
	if db == nil || db.SQLDB == nil {
		return false, core.ErrDatabaseUnavailable
	}
	repository = sanitizeMavenRepository(repository)
	groupID = sanitizeMavenDomain(groupID)
	artifactID = SanitizeInputString(strings.TrimSpace(artifactID), 255)
	if repository == "" || groupID == "" || artifactID == "" {
		return false, errors.New("maven artifact key is invalid")
	}
	var exists int
	err := db.QueryRow(`SELECT 1 FROM maven_artifacts WHERE repository = ? AND group_id = ? AND artifact_id = ?`,
		repository, groupID, artifactID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect Maven artifact: %w", err)
	}
	return exists != 0, nil
}

// RecordMavenMirrorPublication upserts catalog metadata for a version fetched from an upstream mirror.
func (db *DB) RecordMavenMirrorPublication(artifact *core.MavenArtifact, version *core.MavenVersion) error {
	return db.recordMavenPublication(artifact, version, true)
}

func (db *DB) recordMavenPublication(artifact *core.MavenArtifact, version *core.MavenVersion, mirrored bool) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if artifact == nil || version == nil {
		return errors.New("maven publication metadata is missing")
	}
	repository := sanitizeMavenRepository(artifact.Repository)
	domain := sanitizeMavenDomain(artifact.Domain)
	groupID := sanitizeMavenDomain(artifact.GroupID)
	artifactID := SanitizeInputString(strings.TrimSpace(artifact.ArtifactID), 255)
	versionName := SanitizeInputString(strings.TrimSpace(version.Version), 255)
	publisher := sanitizeMavenUsername(version.Publisher)
	description := SanitizeInputString(strings.TrimSpace(artifact.Description), 4000)
	readme := sanitizePackageReadme(artifact.Readme)
	superTeamPrefix, bindingErr := normalizeOptionalSuperTeamPrefix(artifact.SuperTeamPrefix)
	if bindingErr != nil {
		return bindingErr
	}
	if mirrored {
		superTeamPrefix = ""
	}
	if repository == "" || domain == "" || groupID == "" || artifactID == "" || versionName == "" {
		return errors.New("maven publication metadata is invalid")
	}
	if groupID != domain && !strings.HasPrefix(groupID, domain+".") {
		return core.ErrMavenPermissionDenied
	}
	createdAt := artifact.CreatedAt
	if createdAt <= 0 {
		createdAt = time.Now().UnixMilli()
	}
	updatedAt := max(artifact.UpdatedAt, createdAt)
	versionCreatedAt := version.CreatedAt
	if versionCreatedAt <= 0 {
		versionCreatedAt = createdAt
	}
	mirroredValue := boolInt(mirrored)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Maven publication: %w", err)
	}
	defer tx.Rollback()
	if superTeamPrefix != "" {
		publisherID, identityErr := userIDForUsernameTx(tx, publisher)
		if identityErr != nil {
			return core.ErrSuperTeamBindingPermission
		}
		if err := requireSuperTeamRoleTx(tx, superTeamPrefix, publisherID, core.SuperTeamRoleManage); err != nil {
			return err
		}
	}
	if !mirrored {
		var verified int
		if err := tx.QueryRow(`SELECT verified FROM maven_domains WHERE repository = ? AND domain = ?`,
			globalMavenRepository, domain).Scan(&verified); errors.Is(err, sql.ErrNoRows) {
			return core.ErrMavenDomainNotFound
		} else if err != nil {
			return fmt.Errorf("inspect Maven publication domain: %w", err)
		}
		if verified == 0 {
			return core.ErrMavenDomainUnverified
		}
	}
	var latestVersion string
	err = tx.QueryRow(`SELECT latest_version FROM maven_artifacts WHERE repository = ? AND group_id = ? AND artifact_id = ?`,
		repository, groupID, artifactID).Scan(&latestVersion)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.Exec(`INSERT INTO maven_artifacts
			(repository, domain, group_id, artifact_id, description, readme, publisher, latest_version,
			super_team_prefix, mirrored, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, repository, domain, groupID, artifactID,
			description, readme, publisher, versionName, superTeamPrefix, mirroredValue, createdAt, updatedAt); err != nil {
			return fmt.Errorf("create Maven artifact: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect Maven artifact: %w", err)
	} else {
		latestPublisher := ""
		if latestVersion == "" || utils.CompareVersions(versionName, latestVersion) >= 0 {
			latestVersion = versionName
			latestPublisher = publisher
		}
		if _, err := tx.Exec(`UPDATE maven_artifacts SET
		domain = ?, description = CASE WHEN ? != '' THEN ? ELSE description END,
		readme = CASE WHEN ? != '' THEN ? ELSE readme END,
		publisher = CASE WHEN ? != '' THEN ? ELSE publisher END,
			latest_version = ?, updated_at = CASE WHEN ? > updated_at THEN ? ELSE updated_at END
		WHERE repository = ? AND group_id = ? AND artifact_id = ?`,
			domain, description, description, readme, readme, latestPublisher, latestPublisher, latestVersion,
			updatedAt, updatedAt,
			repository, groupID, artifactID); err != nil {
			return fmt.Errorf("update Maven artifact: %w", err)
		}
	}
	var exists int
	err = tx.QueryRow(`SELECT 1 FROM maven_versions WHERE repository = ? AND group_id = ? AND artifact_id = ? AND version = ?`,
		repository, groupID, artifactID, versionName).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.Exec(`INSERT INTO maven_versions
			(repository, group_id, artifact_id, version, publisher, size, mirrored, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			repository, groupID, artifactID, versionName, publisher, max(version.Size, 0), mirroredValue, versionCreatedAt); err != nil {
			return fmt.Errorf("create Maven version: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect Maven version: %w", err)
	} else if _, err := tx.Exec(`UPDATE maven_versions SET publisher = CASE WHEN ? != '' THEN ? ELSE publisher END,
		size = CASE WHEN ? > size THEN ? ELSE size END,
		mirrored = CASE WHEN ? = 1 THEN 1 ELSE mirrored END
		WHERE repository = ? AND group_id = ? AND artifact_id = ? AND version = ?`,
		publisher, publisher, version.Size, version.Size, mirroredValue, repository, groupID, artifactID, versionName); err != nil {
		return fmt.Errorf("update Maven version: %w", err)
	}
	mirroredUpdate := `UPDATE maven_artifacts SET mirrored = CASE WHEN EXISTS (
		SELECT 1 FROM maven_versions v WHERE v.repository = maven_artifacts.repository
		AND v.group_id = maven_artifacts.group_id AND v.artifact_id = maven_artifacts.artifact_id
		AND v.mirrored = 1) THEN 1 ELSE 0 END
		WHERE repository = ? AND group_id = ? AND artifact_id = ?`
	mirroredArgs := []any{repository, groupID, artifactID}
	if db.Dialect.Name() == "clickhouse" {
		var mirroredCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM maven_versions WHERE repository = ? AND group_id = ?
			AND artifact_id = ? AND mirrored = 1`, repository, groupID, artifactID).Scan(&mirroredCount); err != nil {
			return fmt.Errorf("count mirrored Maven versions: %w", err)
		}
		mirroredUpdate = `UPDATE maven_artifacts SET mirrored = ? WHERE repository = ? AND group_id = ? AND artifact_id = ?`
		mirroredArgs = []any{boolInt(mirroredCount > 0), repository, groupID, artifactID}
	}
	if _, err := tx.Exec(mirroredUpdate, mirroredArgs...); err != nil {
		return fmt.Errorf("update Maven artifact mirror provenance: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Maven publication: %w", err)
	}
	return nil
}

// ListMavenArtifacts returns a bounded catalog page and total count.
func (db *DB) ListMavenArtifacts(repository, domain, query string, limit, offset int) ([]*core.MavenArtifact, int, error) {
	return db.listMavenArtifacts([]string{repository}, domain, query, limit, offset)
}

// ListMavenDomainArtifacts returns one bounded domain catalog across the supplied repositories.
func (db *DB) ListMavenDomainArtifacts(repositories []string, domain string, limit, offset int) ([]*core.MavenArtifact, int, error) {
	if domain = sanitizeMavenDomain(domain); domain == "" {
		return nil, 0, errors.New("maven domain is invalid")
	}
	return db.listMavenArtifacts(repositories, domain, "", limit, offset)
}

func (db *DB) listMavenArtifacts(repositories []string, domain, query string, limit, offset int) ([]*core.MavenArtifact, int, error) {
	if db == nil || db.SQLDB == nil {
		return nil, 0, core.ErrDatabaseUnavailable
	}
	normalizedRepositories := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		if repository = sanitizeMavenRepository(repository); repository != "" {
			normalizedRepositories = append(normalizedRepositories, repository)
		}
	}
	slices.Sort(normalizedRepositories)
	normalizedRepositories = slices.Compact(normalizedRepositories)
	if len(normalizedRepositories) == 0 {
		return []*core.MavenArtifact{}, 0, nil
	}
	domain = sanitizeMavenDomain(domain)
	query = SanitizeInputString(strings.ToLower(strings.TrimSpace(query)), 128)
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, errors.New("maven artifact page is invalid")
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(normalizedRepositories)), ",")
	where := ` WHERE repository IN (` + placeholders + `)`
	args := make([]any, len(normalizedRepositories))
	for i, repository := range normalizedRepositories {
		args[i] = repository
	}
	if domain != "" {
		where += ` AND domain = ?`
		args = append(args, domain)
	}
	if query != "" {
		where += ` AND (LOWER(group_id) LIKE ? OR LOWER(artifact_id) LIKE ?)`
		pattern := "%" + query + "%"
		args = append(args, pattern, pattern)
	}
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM maven_artifacts`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count Maven artifacts: %w", err)
	}
	rows, err := db.Query(`SELECT repository, domain, group_id, artifact_id, description, publisher,
		latest_version, (SELECT COUNT(*) FROM maven_versions v WHERE v.repository = maven_artifacts.repository
		AND v.group_id = maven_artifacts.group_id AND v.artifact_id = maven_artifacts.artifact_id),
		COALESCE((SELECT SUM(v.size) FROM maven_versions v WHERE v.repository = maven_artifacts.repository
		AND v.group_id = maven_artifacts.group_id AND v.artifact_id = maven_artifacts.artifact_id), 0),
		super_team_prefix, mirrored, created_at, updated_at FROM maven_artifacts`+where+` ORDER BY group_id, artifact_id, repository LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list Maven artifacts: %w", err)
	}
	defer rows.Close()
	artifacts := make([]*core.MavenArtifact, 0, min(limit, total))
	for rows.Next() {
		artifact := &core.MavenArtifact{}
		var mirrored int
		if err := rows.Scan(&artifact.Repository, &artifact.Domain, &artifact.GroupID, &artifact.ArtifactID,
			&artifact.Description, &artifact.Publisher, &artifact.LatestVersion, &artifact.VersionCount,
			&artifact.TotalSize, &artifact.SuperTeamPrefix, &mirrored, &artifact.CreatedAt, &artifact.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan Maven artifact: %w", err)
		}
		artifact.Mirrored = mirrored != 0
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate Maven artifacts: %w", err)
	}
	return artifacts, total, nil
}

// GetMavenArtifactDetails loads one artifact and all versions.
func (db *DB) GetMavenArtifactDetails(repository, groupID, artifactID string) (*core.MavenArtifactDetails, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository = sanitizeMavenRepository(repository)
	groupID = sanitizeMavenDomain(groupID)
	artifactID = SanitizeInputString(strings.TrimSpace(artifactID), 255)
	result := &core.MavenArtifactDetails{Artifact: &core.MavenArtifact{}}
	var mirrored int
	err := db.QueryRow(`SELECT repository, domain, group_id, artifact_id, description, COALESCE(readme, ''), publisher,
		latest_version, (SELECT COUNT(*) FROM maven_versions v WHERE v.repository = maven_artifacts.repository
		AND v.group_id = maven_artifacts.group_id AND v.artifact_id = maven_artifacts.artifact_id),
		COALESCE((SELECT SUM(v.size) FROM maven_versions v WHERE v.repository = maven_artifacts.repository
		AND v.group_id = maven_artifacts.group_id AND v.artifact_id = maven_artifacts.artifact_id), 0),
		super_team_prefix, mirrored, created_at, updated_at FROM maven_artifacts WHERE repository = ? AND group_id = ? AND artifact_id = ?`,
		repository, groupID, artifactID).Scan(&result.Artifact.Repository, &result.Artifact.Domain,
		&result.Artifact.GroupID, &result.Artifact.ArtifactID, &result.Artifact.Description,
		&result.Artifact.Readme, &result.Artifact.Publisher, &result.Artifact.LatestVersion, &result.Artifact.VersionCount,
		&result.Artifact.TotalSize, &result.Artifact.SuperTeamPrefix, &mirrored,
		&result.Artifact.CreatedAt, &result.Artifact.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrMavenArtifactNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load Maven artifact: %w", err)
	}
	result.Artifact.Mirrored = mirrored != 0
	rows, err := db.Query(`SELECT repository, group_id, artifact_id, version, publisher, size, mirrored, created_at
		FROM maven_versions WHERE repository = ? AND group_id = ? AND artifact_id = ? ORDER BY created_at DESC`,
		repository, groupID, artifactID)
	if err != nil {
		return nil, fmt.Errorf("list Maven versions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		version := &core.MavenVersion{}
		var versionMirrored int
		if err := rows.Scan(&version.Repository, &version.GroupID, &version.ArtifactID, &version.Version,
			&version.Publisher, &version.Size, &versionMirrored, &version.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan Maven version: %w", err)
		}
		version.Mirrored = versionMirrored != 0
		result.Versions = append(result.Versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Maven versions: %w", err)
	}
	slices.SortFunc(result.Versions, func(left, right *core.MavenVersion) int {
		return -utils.CompareVersions(left.Version, right.Version)
	})
	return result, nil
}

// GetMavenArtifactTeamAccess returns effective global-team permission for one bound artifact.
func (db *DB) GetMavenArtifactTeamAccess(repository, groupID, artifactID, username string) (
	superTeamPrefix string, member bool, level int, err error,
) {
	if db == nil || db.SQLDB == nil {
		return "", false, 0, core.ErrDatabaseUnavailable
	}
	repository = sanitizeMavenRepository(repository)
	groupID = sanitizeMavenDomain(groupID)
	artifactID = SanitizeInputString(strings.TrimSpace(artifactID), 255)
	username = sanitizeMavenUsername(username)
	userID := ""
	if username != "" && username != "guest" {
		userID, err = db.userIDForUsername(username)
		if errors.Is(err, core.ErrUserProfileNotFound) {
			err = nil
			userID = ""
		}
		if err != nil {
			return "", false, 0, err
		}
	}
	var role, memberValue int
	err = db.QueryRow(`SELECT a.super_team_prefix, COALESCE(stm.role_level, 0),
		CASE WHEN stm.user_id IS NULL THEN 0 ELSE 1 END
		FROM maven_artifacts a LEFT JOIN super_team_members stm
		ON stm.team_prefix = a.super_team_prefix AND stm.user_id = ?
		WHERE a.repository = ? AND a.group_id = ? AND a.artifact_id = ?`,
		userID, repository, groupID, artifactID).Scan(&superTeamPrefix, &role, &memberValue)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, 0, core.ErrMavenArtifactNotFound
	}
	if err != nil {
		return "", false, 0, fmt.Errorf("inspect Maven artifact global team: %w", err)
	}
	level, member = effectiveBoundPermission(0, false, role, memberValue != 0)
	return superTeamPrefix, member, level, nil
}

// UpdateMavenArtifactDescription updates the short catalog description.
func (db *DB) UpdateMavenArtifactDescription(repository, groupID, artifactID, description string) error {
	description = SanitizeInputString(strings.TrimSpace(description), 4000)
	result, err := db.Exec(`UPDATE maven_artifacts SET description = ?, updated_at = ?
		WHERE repository = ? AND group_id = ? AND artifact_id = ?`, description, time.Now().UnixMilli(),
		sanitizeMavenRepository(repository), sanitizeMavenDomain(groupID),
		SanitizeInputString(strings.TrimSpace(artifactID), 255))
	if err != nil {
		return fmt.Errorf("update Maven artifact description: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count Maven artifact update: %w", err)
	}
	if changed == 0 {
		return core.ErrMavenArtifactNotFound
	}
	return nil
}

// UpdateMavenArtifactReadme updates bounded Markdown documentation for one catalog artifact.
func (db *DB) UpdateMavenArtifactReadme(repository, groupID, artifactID, readme string) error {
	readme = sanitizePackageReadme(readme)
	result, err := db.Exec(`UPDATE maven_artifacts SET readme = ?, updated_at = ?
		WHERE repository = ? AND group_id = ? AND artifact_id = ?`, readme, time.Now().UnixMilli(),
		sanitizeMavenRepository(repository), sanitizeMavenDomain(groupID),
		SanitizeInputString(strings.TrimSpace(artifactID), 255))
	if err != nil {
		return fmt.Errorf("update Maven artifact README: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count Maven artifact README update: %w", err)
	}
	if changed == 0 {
		return core.ErrMavenArtifactNotFound
	}
	return nil
}

// DeleteMavenVersionMetadata removes a catalog version after storage deletion.
func (db *DB) DeleteMavenVersionMetadata(repository, groupID, artifactID, version string) error {
	repository, groupID = sanitizeMavenRepository(repository), sanitizeMavenDomain(groupID)
	artifactID = SanitizeInputString(strings.TrimSpace(artifactID), 255)
	version = SanitizeInputString(strings.TrimSpace(version), 255)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Maven version deletion: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.Exec(`DELETE FROM maven_versions WHERE repository = ? AND group_id = ? AND artifact_id = ? AND version = ?`,
		repository, groupID, artifactID, version)
	if err != nil {
		return fmt.Errorf("delete Maven version metadata: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count Maven version deletion: %w", err)
	}
	if changed == 0 {
		return core.ErrMavenVersionNotFound
	}
	rows, err := tx.Query(`SELECT version FROM maven_versions WHERE repository = ? AND group_id = ? AND artifact_id = ?`,
		repository, groupID, artifactID)
	if err != nil {
		return fmt.Errorf("list remaining Maven versions: %w", err)
	}
	versions := make([]string, 0)
	for rows.Next() {
		var candidate string
		if err := rows.Scan(&candidate); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan remaining Maven version: %w", err)
		}
		versions = append(versions, candidate)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate remaining Maven versions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close remaining Maven versions: %w", err)
	}
	if len(versions) == 0 {
		if _, err := tx.Exec(`DELETE FROM maven_artifacts WHERE repository = ? AND group_id = ? AND artifact_id = ?`,
			repository, groupID, artifactID); err != nil {
			return fmt.Errorf("delete empty Maven artifact: %w", err)
		}
	} else {
		slices.SortFunc(versions, func(left, right string) int { return -utils.CompareVersions(left, right) })
		updateQuery := `UPDATE maven_artifacts SET latest_version = ?, updated_at = ?,
			mirrored = CASE WHEN EXISTS (SELECT 1 FROM maven_versions v
				WHERE v.repository = maven_artifacts.repository AND v.group_id = maven_artifacts.group_id
				AND v.artifact_id = maven_artifacts.artifact_id AND v.mirrored = 1) THEN 1 ELSE 0 END
		WHERE repository = ? AND group_id = ? AND artifact_id = ?`
		updateArgs := []any{versions[0], time.Now().UnixMilli(), repository, groupID, artifactID}
		if db.Dialect.Name() == "clickhouse" {
			var mirroredCount int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM maven_versions WHERE repository = ? AND group_id = ?
				AND artifact_id = ? AND mirrored = 1`, repository, groupID, artifactID).Scan(&mirroredCount); err != nil {
				return fmt.Errorf("count remaining mirrored Maven versions: %w", err)
			}
			updateQuery = `UPDATE maven_artifacts SET latest_version = ?, updated_at = ?, mirrored = ?
				WHERE repository = ? AND group_id = ? AND artifact_id = ?`
			updateArgs = []any{versions[0], time.Now().UnixMilli(), boolInt(mirroredCount > 0), repository, groupID, artifactID}
		}
		if _, err := tx.Exec(updateQuery, updateArgs...); err != nil {
			return fmt.Errorf("update latest Maven version: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Maven version deletion: %w", err)
	}
	return nil
}

// DeleteMavenRepository deletes repository-local Maven catalog metadata.
func (db *DB) DeleteMavenRepository(repository string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository = sanitizeMavenRepository(repository)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Maven repository metadata deletion: %w", err)
	}
	defer tx.Rollback()
	for _, query := range []string{
		`DELETE FROM maven_versions WHERE repository = ?`,
		`DELETE FROM maven_artifacts WHERE repository = ?`,
		`DELETE FROM maven_repository_upgrades WHERE repository = ?`,
	} {
		if _, err := tx.Exec(query, repository); err != nil {
			return fmt.Errorf("delete Maven repository metadata: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Maven repository metadata deletion: %w", err)
	}
	return nil
}
