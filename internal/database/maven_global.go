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
	"fmt"
	"sort"
	"strings"
	"time"

	"renop/internal/core"
)

const globalMavenRepository = ""

type mavenDomainMigrationRecord struct {
	repository       string
	domain           string
	verificationType string
	verificationHost string
	verificationCode string
	verified         bool
	createdAt        int64
	verifiedAt       int64
	lastCheckAt      int64
}

type mavenMemberMigrationRecord struct {
	domain   string
	username string
	userID   string
	level    int
	addedAt  int64
}

type mavenInvitationMigrationRecord struct {
	id        string
	domain    string
	inviter   string
	recipient string
	level     int
	createdAt int64
}

func (db *DB) migrateGlobalMavenDomains() error {
	var legacyCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM maven_domains WHERE repository <> ?`, globalMavenRepository).Scan(&legacyCount); err != nil {
		return fmt.Errorf("count repository-scoped Maven domains: %w", err)
	}
	if legacyCount == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin global Maven domain migration: %w", err)
	}
	defer tx.Rollback()

	domains, err := loadMavenDomainMigrationRecords(tx)
	if err != nil {
		return err
	}
	members, err := loadMavenMemberMigrationRecords(tx)
	if err != nil {
		return err
	}
	invitations, err := loadMavenInvitationMigrationRecords(tx)
	if err != nil {
		return err
	}

	mergedDomains := mergeMavenDomainRecords(domains)
	mergedMembers := mergeMavenMemberRecords(members, mergedDomains)
	mergedInvitations, cancelledInvitationIDs := mergeMavenInvitationRecords(invitations, mergedDomains, mergedMembers)
	actedAt := time.Now().UnixMilli()
	for _, invitation := range cancelledInvitationIDs {
		if _, err := tx.Exec(`UPDATE user_messages SET action_status = ?, acted_at = ?,
			read_at = CASE WHEN read_at = 0 THEN ? ELSE read_at END
			WHERE id = ? AND recipient = ? AND action_status = ?`, core.MessageActionCancelled,
			actedAt, actedAt, invitation.id, invitation.recipient, core.MessageActionPending); err != nil {
			return fmt.Errorf("cancel duplicate Maven invitation during migration: %w", err)
		}
	}
	for _, query := range []string{
		`DELETE FROM maven_domain_invitations WHERE repository IS NOT NULL`,
		`DELETE FROM maven_domain_members WHERE repository IS NOT NULL`,
		`DELETE FROM maven_domains WHERE repository IS NOT NULL`,
	} {
		if _, err := tx.Exec(query); err != nil {
			return fmt.Errorf("clear repository-scoped Maven domain data: %w", err)
		}
	}
	for _, domain := range mergedDomains {
		verified := 0
		if domain.verified {
			verified = 1
		}
		if _, err := tx.Exec(`INSERT INTO maven_domains
			(repository, domain, verification_type, verification_host, verification_code, verified, created_at, verified_at, last_check_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, globalMavenRepository, domain.domain,
			domain.verificationType, domain.verificationHost, domain.verificationCode, verified,
			domain.createdAt, domain.verifiedAt, domain.lastCheckAt); err != nil {
			return fmt.Errorf("migrate global Maven domain %s: %w", domain.domain, err)
		}
	}
	for _, member := range mergedMembers {
		if _, err := tx.Exec(`INSERT INTO maven_domain_members
			(repository, domain, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
			globalMavenRepository, member.domain, member.username, nullableString(member.userID), member.level, member.addedAt); err != nil {
			return fmt.Errorf("migrate global Maven domain member %s/%s: %w", member.domain, member.username, err)
		}
	}
	for _, invitation := range mergedInvitations {
		if _, err := tx.Exec(`INSERT INTO maven_domain_invitations
			(id, repository, domain, inviter, recipient, permission_level, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			invitation.id, globalMavenRepository, invitation.domain, invitation.inviter,
			invitation.recipient, invitation.level, invitation.createdAt); err != nil {
			return fmt.Errorf("migrate global Maven invitation %s: %w", invitation.id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit global Maven domain migration: %w", err)
	}
	return nil
}

func loadMavenDomainMigrationRecords(tx *Tx) ([]mavenDomainMigrationRecord, error) {
	rows, err := tx.Query(`SELECT repository, domain, verification_type, verification_host,
		verification_code, verified, created_at, verified_at, last_check_at FROM maven_domains`)
	if err != nil {
		return nil, fmt.Errorf("list Maven domains for global migration: %w", err)
	}
	defer rows.Close()
	records := make([]mavenDomainMigrationRecord, 0)
	for rows.Next() {
		var record mavenDomainMigrationRecord
		var verified int
		if err := rows.Scan(&record.repository, &record.domain, &record.verificationType,
			&record.verificationHost, &record.verificationCode, &verified, &record.createdAt,
			&record.verifiedAt, &record.lastCheckAt); err != nil {
			return nil, fmt.Errorf("scan Maven domain for global migration: %w", err)
		}
		record.domain = sanitizeMavenDomain(record.domain)
		record.verified = verified != 0
		if record.domain != "" {
			records = append(records, record)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Maven domains for global migration: %w", err)
	}
	return records, nil
}

func loadMavenMemberMigrationRecords(tx *Tx) ([]mavenMemberMigrationRecord, error) {
	rows, err := tx.Query(`SELECT domain, username, COALESCE(user_id, ''), permission_level, added_at
		FROM maven_domain_members`)
	if err != nil {
		return nil, fmt.Errorf("list Maven members for global migration: %w", err)
	}
	defer rows.Close()
	records := make([]mavenMemberMigrationRecord, 0)
	for rows.Next() {
		var record mavenMemberMigrationRecord
		if err := rows.Scan(&record.domain, &record.username, &record.userID, &record.level, &record.addedAt); err != nil {
			return nil, fmt.Errorf("scan Maven member for global migration: %w", err)
		}
		record.domain = sanitizeMavenDomain(record.domain)
		record.username = sanitizeMavenUsername(record.username)
		record.userID = strings.ToLower(strings.TrimSpace(record.userID))
		if record.domain != "" && record.username != "" {
			records = append(records, record)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Maven members for global migration: %w", err)
	}
	return records, nil
}

func loadMavenInvitationMigrationRecords(tx *Tx) ([]mavenInvitationMigrationRecord, error) {
	rows, err := tx.Query(`SELECT id, domain, inviter, recipient, permission_level, created_at
		FROM maven_domain_invitations`)
	if err != nil {
		return nil, fmt.Errorf("list Maven invitations for global migration: %w", err)
	}
	defer rows.Close()
	records := make([]mavenInvitationMigrationRecord, 0)
	for rows.Next() {
		var record mavenInvitationMigrationRecord
		if err := rows.Scan(&record.id, &record.domain, &record.inviter, &record.recipient, &record.level, &record.createdAt); err != nil {
			return nil, fmt.Errorf("scan Maven invitation for global migration: %w", err)
		}
		record.domain = sanitizeMavenDomain(record.domain)
		record.inviter = sanitizeMavenUsername(record.inviter)
		record.recipient = sanitizeMavenUsername(record.recipient)
		if record.id != "" && record.domain != "" && record.recipient != "" {
			records = append(records, record)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Maven invitations for global migration: %w", err)
	}
	return records, nil
}

func mergeMavenDomainRecords(records []mavenDomainMigrationRecord) []mavenDomainMigrationRecord {
	grouped := make(map[string][]mavenDomainMigrationRecord)
	for _, record := range records {
		grouped[record.domain] = append(grouped[record.domain], record)
	}
	result := make([]mavenDomainMigrationRecord, 0, len(grouped))
	for _, candidates := range grouped {
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].verified != candidates[j].verified {
				return candidates[i].verified
			}
			if candidates[i].createdAt != candidates[j].createdAt {
				return candidates[i].createdAt < candidates[j].createdAt
			}
			return candidates[i].repository < candidates[j].repository
		})
		chosen := candidates[0]
		chosen.repository = globalMavenRepository
		for _, candidate := range candidates[1:] {
			if candidate.createdAt > 0 && (chosen.createdAt == 0 || candidate.createdAt < chosen.createdAt) {
				chosen.createdAt = candidate.createdAt
			}
			chosen.lastCheckAt = max(chosen.lastCheckAt, candidate.lastCheckAt)
		}
		result = append(result, chosen)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].domain < result[j].domain })
	return result
}

func mergeMavenMemberRecords(records []mavenMemberMigrationRecord, domains []mavenDomainMigrationRecord) []mavenMemberMigrationRecord {
	validDomains := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		validDomains[domain.domain] = struct{}{}
	}
	grouped := make(map[string]map[string]mavenMemberMigrationRecord)
	for _, record := range records {
		if _, exists := validDomains[record.domain]; !exists {
			continue
		}
		identity := record.userID
		if identity == "" {
			identity = "username:" + record.username
		}
		if grouped[record.domain] == nil {
			grouped[record.domain] = make(map[string]mavenMemberMigrationRecord)
		}
		current, exists := grouped[record.domain][identity]
		if !exists || record.level > current.level || (record.level == current.level && record.addedAt < current.addedAt) {
			if exists && current.addedAt > 0 && (record.addedAt == 0 || current.addedAt < record.addedAt) {
				record.addedAt = current.addedAt
			}
			grouped[record.domain][identity] = record
		}
	}
	result := make([]mavenMemberMigrationRecord, 0)
	for domain, membersByID := range grouped {
		members := make([]mavenMemberMigrationRecord, 0, len(membersByID))
		for _, member := range membersByID {
			member.domain = domain
			member.level = min(max(member.level, core.MavenPermissionRead), core.MavenPermissionOwner)
			members = append(members, member)
		}
		sort.SliceStable(members, func(i, j int) bool {
			if members[i].addedAt != members[j].addedAt {
				return members[i].addedAt < members[j].addedAt
			}
			return members[i].username < members[j].username
		})
		owner := -1
		for i := range members {
			if members[i].level == core.MavenPermissionOwner && owner == -1 {
				owner = i
			} else if members[i].level == core.MavenPermissionOwner {
				members[i].level = core.MavenPermissionManage
			}
		}
		if owner == -1 && len(members) > 0 {
			owner = 0
			for i := 1; i < len(members); i++ {
				if members[i].level > members[owner].level {
					owner = i
				}
			}
			members[owner].level = core.MavenPermissionOwner
		}
		result = append(result, members...)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].domain != result[j].domain {
			return result[i].domain < result[j].domain
		}
		if result[i].level != result[j].level {
			return result[i].level > result[j].level
		}
		return result[i].username < result[j].username
	})
	return result
}

func mergeMavenInvitationRecords(records []mavenInvitationMigrationRecord, domains []mavenDomainMigrationRecord, members []mavenMemberMigrationRecord) ([]mavenInvitationMigrationRecord, []mavenInvitationMigrationRecord) {
	validDomains := make(map[string]struct{}, len(domains))
	memberKeys := make(map[string]struct{}, len(members))
	for _, domain := range domains {
		validDomains[domain.domain] = struct{}{}
	}
	for _, member := range members {
		memberKeys[member.domain+"\x00"+member.username] = struct{}{}
	}
	selected := make(map[string]mavenInvitationMigrationRecord)
	cancelled := make([]mavenInvitationMigrationRecord, 0)
	for _, record := range records {
		key := record.domain + "\x00" + record.recipient
		if _, exists := validDomains[record.domain]; !exists {
			cancelled = append(cancelled, record)
			continue
		}
		if _, member := memberKeys[key]; member {
			cancelled = append(cancelled, record)
			continue
		}
		record.level = min(max(record.level, core.MavenPermissionRead), core.MavenPermissionOwner)
		current, exists := selected[key]
		if !exists || record.createdAt > current.createdAt {
			if exists {
				cancelled = append(cancelled, current)
			}
			selected[key] = record
		} else {
			cancelled = append(cancelled, record)
		}
	}
	result := make([]mavenInvitationMigrationRecord, 0, len(selected))
	for _, record := range selected {
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].domain != result[j].domain {
			return result[i].domain < result[j].domain
		}
		return result[i].recipient < result[j].recipient
	})
	return result, cancelled
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
