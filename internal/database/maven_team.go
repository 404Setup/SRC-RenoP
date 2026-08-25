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

func cancelMavenInvitations(tx *Tx, where string, args []any, actedAt int64) error {
	rows, err := tx.Query(`SELECT id, recipient FROM maven_domain_invitations WHERE `+where, args...)
	if err != nil {
		return fmt.Errorf("find Maven invitations to cancel: %w", err)
	}
	type invitationRef struct{ id, recipient string }
	refs := make([]invitationRef, 0)
	for rows.Next() {
		var ref invitationRef
		if err := rows.Scan(&ref.id, &ref.recipient); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan Maven invitation cancellation: %w", err)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate Maven invitation cancellations: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close Maven invitation cancellation rows: %w", err)
	}
	for _, ref := range refs {
		if _, err := tx.Exec(`UPDATE user_messages SET action_status = ?, acted_at = ?,
			read_at = CASE WHEN read_at = 0 THEN ? ELSE read_at END
			WHERE id = ? AND recipient = ? AND action_status = ?`, core.MessageActionCancelled,
			actedAt, actedAt, ref.id, ref.recipient, core.MessageActionPending); err != nil {
			return fmt.Errorf("cancel Maven invitation message: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM maven_domain_invitations WHERE `+where, args...); err != nil {
		return fmt.Errorf("delete Maven invitations: %w", err)
	}
	return nil
}

// CreateMavenInvitations persists domain invitations and their message actions atomically.
func (db *DB) CreateMavenInvitations(invitations []*core.MavenInvitation, messages []*core.UserMessage) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(invitations) == 0 || len(invitations) != len(messages) || len(invitations) > 20 {
		return errors.New("maven invitation batch is invalid")
	}
	for i, invitation := range invitations {
		message := messages[i]
		if invitation == nil || message == nil {
			return errors.New("maven invitation is missing")
		}
		invitation.Repository = globalMavenRepository
		invitation.Domain = sanitizeMavenDomain(invitation.Domain)
		invitation.Inviter = sanitizeMavenUsername(invitation.Inviter)
		invitation.Recipient = sanitizeMavenUsername(invitation.Recipient)
		if invitation.Level < core.MavenPermissionRead || invitation.Level > core.MavenPermissionOwner {
			return errors.New("maven invitation permission level is invalid")
		}
		if invitation.ID == "" || invitation.ID != message.ID || invitation.Recipient != sanitizeMavenUsername(message.Recipient) {
			return errors.New("maven invitation message does not match its workflow record")
		}
		if err := normalizeMessage(message); err != nil {
			return err
		}
	}
	first := invitations[0]
	inviterID, err := db.userIDForExistingAccount(first.Inviter)
	if err != nil {
		return core.ErrMavenPermissionDenied
	}
	recipientIDs := make(map[string]string, len(invitations))
	for _, invitation := range invitations {
		recipientID, err := db.userIDForExistingAccount(invitation.Recipient)
		if err != nil {
			return core.ErrUserProfileNotFound
		}
		recipientIDs[invitation.Recipient] = recipientID
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Maven invitation creation: %w", err)
	}
	defer tx.Rollback()
	if err := lockMavenDomain(tx, first.Domain); err != nil {
		return err
	}
	var inviterLevel int
	if err := tx.QueryRow(`SELECT permission_level FROM maven_domain_members
		WHERE repository = ? AND domain = ? AND user_id = ?`, first.Repository, first.Domain, inviterID).Scan(&inviterLevel); errors.Is(err, sql.ErrNoRows) || inviterLevel < core.MavenPermissionManage {
		return core.ErrMavenPermissionDenied
	} else if err != nil {
		return fmt.Errorf("inspect Maven invitation sender: %w", err)
	}
	for i, invitation := range invitations {
		message := messages[i]
		if invitation.Repository != first.Repository || invitation.Domain != first.Domain || invitation.Inviter != first.Inviter {
			return errors.New("maven invitation batch targets multiple domains")
		}
		if invitation.Level == core.MavenPermissionOwner && inviterLevel < core.MavenPermissionOwner {
			return core.ErrMavenPermissionDenied
		}
		recipientID := recipientIDs[invitation.Recipient]
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM maven_domain_members WHERE repository = ? AND domain = ? AND user_id = ?`,
			invitation.Repository, invitation.Domain, recipientID).Scan(&exists); err == nil {
			return core.ErrMavenMemberExists
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect Maven invitation recipient: %w", err)
		}
		var existingID, existingStatus string
		var existingExpiry int64
		err := tx.QueryRow(`SELECT i.id, COALESCE(m.action_status, ''), COALESCE(m.expires_at, 0)
			FROM maven_domain_invitations i LEFT JOIN user_messages m ON m.id = i.id AND m.recipient = i.recipient
			WHERE i.repository = ? AND i.domain = ? AND i.recipient = ?`, invitation.Repository,
			invitation.Domain, invitation.Recipient).Scan(&existingID, &existingStatus, &existingExpiry)
		if err == nil && existingStatus == core.MessageActionPending && existingExpiry > invitation.CreatedAt {
			return core.ErrMavenInvitationExists
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect pending Maven invitation: %w", err)
		}
		if err == nil {
			if err := cancelMavenInvitations(tx, `repository = ? AND domain = ? AND recipient = ?`,
				[]any{invitation.Repository, invitation.Domain, invitation.Recipient}, invitation.CreatedAt); err != nil {
				return err
			}
		}
		var dedupeKey any
		if message.DedupeKey != "" {
			dedupeKey = message.DedupeKey
		}
		if _, err := tx.Exec(`INSERT INTO user_messages (`+messageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			message.ID, message.Recipient, message.Sender, message.Kind, message.Severity, message.Title,
			message.Body, string(message.Payload), message.ActionKind, message.ActionStatus, message.CreatedAt,
			message.ReadAt, message.ActedAt, message.ExpiresAt, dedupeKey); err != nil {
			return fmt.Errorf("create Maven invitation message: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO maven_domain_invitations
			(id, repository, domain, inviter, recipient, permission_level, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			invitation.ID, invitation.Repository, invitation.Domain, invitation.Inviter,
			invitation.Recipient, invitation.Level, invitation.CreatedAt); err != nil {
			return fmt.Errorf("create Maven invitation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Maven invitations: %w", err)
	}
	return nil
}

// ForceAddMavenMembers immediately adds members for an administrator without overwriting existing roles.
func (db *DB) ForceAddMavenMembers(domain, actor string, usernames []string, level int) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(usernames) == 0 || len(usernames) > 20 || level < core.MavenPermissionRead || level > core.MavenPermissionOwner {
		return errors.New("maven member addition is invalid")
	}
	domain = sanitizeMavenDomain(domain)
	actor = sanitizeMavenUsername(actor)
	unique := make([]string, 0, len(usernames))
	userIDs := make(map[string]string, len(usernames))
	seen := make(map[string]struct{}, len(usernames))
	for _, raw := range usernames {
		username := sanitizeMavenUsername(raw)
		if username == "" {
			return errors.New("maven member name is invalid")
		}
		if _, exists := seen[username]; exists {
			continue
		}
		seen[username] = struct{}{}
		userID, err := db.userIDForExistingAccount(username)
		if err != nil {
			return core.ErrUserProfileNotFound
		}
		unique = append(unique, username)
		userIDs[username] = userID
	}
	if len(unique) == 0 || (level == core.MavenPermissionOwner && len(unique) != 1) {
		return errors.New("maven L4 ownership can only be assigned to one member")
	}
	now := time.Now().UnixMilli()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Maven member addition: %w", err)
	}
	defer tx.Rollback()
	if err := lockMavenDomain(tx, domain); err != nil {
		return err
	}
	for _, username := range unique {
		var current int
		err := tx.QueryRow(`SELECT permission_level FROM maven_domain_members WHERE repository = ? AND domain = ? AND user_id = ?`,
			globalMavenRepository, domain, userIDs[username]).Scan(&current)
		if err == nil {
			return core.ErrMavenMemberExists
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect Maven member: %w", err)
		}
	}
	if level == core.MavenPermissionOwner {
		if _, err := tx.Exec(`UPDATE maven_domain_members SET permission_level = ?
			WHERE repository = ? AND domain = ? AND permission_level = ?`, core.MavenPermissionManage,
			globalMavenRepository, domain, core.MavenPermissionOwner); err != nil {
			return fmt.Errorf("demote previous Maven owner: %w", err)
		}
	}
	for _, username := range unique {
		if _, err := tx.Exec(`INSERT INTO maven_domain_members
			(repository, domain, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
			globalMavenRepository, domain, username, userIDs[username], level, now); err != nil {
			return fmt.Errorf("insert Maven member: %w", err)
		}
		if err := cancelMavenInvitations(tx, `repository = ? AND domain = ? AND recipient = ?`,
			[]any{globalMavenRepository, domain, username}, now); err != nil {
			return err
		}
		if err := insertAcceptedMembershipMessage(tx, username, actor, "maven_domain_invite",
			"Maven domain membership added",
			fmt.Sprintf("%s added you to Maven domain %s with L%d permission.", actor, domain, level),
			map[string]any{"domain": domain, "inviter": actor, "level": level}, now); err != nil {
			return fmt.Errorf("create Maven membership message: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Maven member addition: %w", err)
	}
	return nil
}

// RespondMavenInvitation accepts or rejects one pending Maven domain invitation.
func (db *DB) RespondMavenInvitation(id, recipient string, accept bool, actedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	id = SanitizeInputString(strings.TrimSpace(id), 64)
	recipient = sanitizeMavenUsername(recipient)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Maven invitation response: %w", err)
	}
	defer tx.Rollback()
	invitation := &core.MavenInvitation{ID: id, Recipient: recipient}
	err = tx.QueryRow(`SELECT repository, domain, inviter, permission_level, created_at
			FROM maven_domain_invitations WHERE id = ? AND recipient = ? AND repository = ?`, id, recipient, globalMavenRepository).Scan(
		&invitation.Repository, &invitation.Domain, &invitation.Inviter, &invitation.Level, &invitation.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrMavenInvitationInvalid
	}
	if err != nil {
		return fmt.Errorf("load Maven invitation: %w", err)
	}
	if err := lockMavenDomain(tx, invitation.Domain); err != nil {
		return err
	}
	if accept {
		inviterID, err := userIDForUsernameTx(tx, invitation.Inviter)
		if err != nil {
			return core.ErrMavenInvitationInvalid
		}
		recipientID, err := userIDForUsernameTx(tx, recipient)
		if err != nil {
			return core.ErrMavenInvitationInvalid
		}
		var inviterLevel int
		if err := tx.QueryRow(`SELECT permission_level FROM maven_domain_members
			WHERE repository = ? AND domain = ? AND user_id = ?`, invitation.Repository,
			invitation.Domain, inviterID).Scan(&inviterLevel); errors.Is(err, sql.ErrNoRows) || inviterLevel < core.MavenPermissionManage {
			return core.ErrMavenInvitationInvalid
		} else if err != nil {
			return fmt.Errorf("validate Maven invitation sender: %w", err)
		}
		if invitation.Level == core.MavenPermissionOwner && inviterLevel < core.MavenPermissionOwner {
			return core.ErrMavenInvitationInvalid
		}
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM maven_domain_members WHERE repository = ? AND domain = ? AND user_id = ?`,
			invitation.Repository, invitation.Domain, recipientID).Scan(&exists); err == nil {
			return core.ErrMavenMemberExists
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect Maven invitation membership: %w", err)
		}
		if invitation.Level == core.MavenPermissionOwner {
			if _, err := tx.Exec(`UPDATE maven_domain_members SET permission_level = ?
				WHERE repository = ? AND domain = ? AND permission_level = ? AND user_id != ?`,
				core.MavenPermissionManage, invitation.Repository, invitation.Domain,
				core.MavenPermissionOwner, recipientID); err != nil {
				return fmt.Errorf("demote previous Maven owner: %w", err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO maven_domain_members
			(repository, domain, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
			invitation.Repository, invitation.Domain, recipient, recipientID, invitation.Level, actedAt); err != nil {
			return fmt.Errorf("accept Maven membership: %w", err)
		}
	}
	status := core.MessageActionRejected
	if accept {
		status = core.MessageActionAccepted
	}
	result, err := tx.Exec(`UPDATE user_messages SET action_status = ?, acted_at = ?,
		read_at = CASE WHEN read_at = 0 THEN ? ELSE read_at END WHERE id = ? AND recipient = ?
		AND action_kind = 'maven_domain_invite' AND action_status = ? AND (expires_at = 0 OR expires_at > ?)`,
		status, actedAt, actedAt, id, recipient, core.MessageActionPending, actedAt)
	if err != nil {
		return fmt.Errorf("update Maven invitation message: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return core.ErrMavenInvitationInvalid
	}
	if _, err := tx.Exec(`DELETE FROM maven_domain_invitations WHERE id = ? AND recipient = ?`, id, recipient); err != nil {
		return fmt.Errorf("complete Maven invitation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Maven invitation response: %w", err)
	}
	return nil
}

// SetMavenMemberLevel updates a domain member while preserving exactly one L4 owner.
func (db *DB) SetMavenMemberLevel(domain, actor, username string, level int) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if level < core.MavenPermissionRead || level > core.MavenPermissionOwner {
		return errors.New("maven permission level is invalid")
	}
	domain = sanitizeMavenDomain(domain)
	actor, username = sanitizeMavenUsername(actor), sanitizeMavenUsername(username)
	targetID, err := db.userIDForUsername(username)
	if err != nil {
		return core.ErrUserProfileNotFound
	}
	actorID := ""
	if actor != "" {
		actorID, err = db.userIDForUsername(actor)
		if err != nil {
			return core.ErrMavenPermissionDenied
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Maven member update: %w", err)
	}
	defer tx.Rollback()
	if err := lockMavenDomain(tx, domain); err != nil {
		return err
	}
	actorLevel := 0
	if actor != "" {
		if err := requireMavenMemberPermission(tx, domain, actorID, core.MavenPermissionManage); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT permission_level FROM maven_domain_members WHERE repository = ? AND domain = ? AND user_id = ?`,
			globalMavenRepository, domain, actorID).Scan(&actorLevel); err != nil {
			return fmt.Errorf("inspect Maven actor permission: %w", err)
		}
	}
	var current int
	if err := tx.QueryRow(`SELECT permission_level FROM maven_domain_members WHERE repository = ? AND domain = ? AND user_id = ?`,
		globalMavenRepository, domain, targetID).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return core.ErrMavenDomainNotFound
	} else if err != nil {
		return fmt.Errorf("inspect Maven member: %w", err)
	}
	if current == core.MavenPermissionOwner && level < core.MavenPermissionOwner {
		if err := requireAnotherMavenOwner(tx, domain, targetID); err != nil {
			return err
		}
	}
	if level == core.MavenPermissionOwner && current != core.MavenPermissionOwner {
		if actor != "" {
			if actorLevel != core.MavenPermissionOwner || actor == username {
				return core.ErrMavenPermissionDenied
			}
			if _, err := tx.Exec(`UPDATE maven_domain_members SET permission_level = ?
				WHERE repository = ? AND domain = ? AND user_id = ?`, current, globalMavenRepository, domain, actorID); err != nil {
				return fmt.Errorf("exchange Maven ownership permission: %w", err)
			}
		} else if _, err := tx.Exec(`UPDATE maven_domain_members SET permission_level = ?
			WHERE repository = ? AND domain = ? AND permission_level = ? AND user_id != ?`,
			core.MavenPermissionManage, globalMavenRepository, domain, core.MavenPermissionOwner, targetID); err != nil {
			return fmt.Errorf("demote previous Maven owner: %w", err)
		}
	}
	if _, err := tx.Exec(`UPDATE maven_domain_members SET permission_level = ?
		WHERE repository = ? AND domain = ? AND user_id = ?`, level, globalMavenRepository, domain, targetID); err != nil {
		return fmt.Errorf("update Maven member: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Maven member update: %w", err)
	}
	return nil
}

// RemoveMavenMember removes one team member while preserving L4 ownership.
func (db *DB) RemoveMavenMember(domain, actor, username string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	domain = sanitizeMavenDomain(domain)
	actor, username = sanitizeMavenUsername(actor), sanitizeMavenUsername(username)
	targetID, err := db.userIDForUsername(username)
	if err != nil {
		return core.ErrUserProfileNotFound
	}
	actorID := ""
	if actor != "" {
		actorID, err = db.userIDForUsername(actor)
		if err != nil {
			return core.ErrMavenPermissionDenied
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Maven member removal: %w", err)
	}
	defer tx.Rollback()
	if err := lockMavenDomain(tx, domain); err != nil {
		return err
	}
	if actor != "" {
		var actorLevel int
		if err := tx.QueryRow(`SELECT permission_level FROM maven_domain_members WHERE repository = ? AND domain = ? AND user_id = ?`,
			globalMavenRepository, domain, actorID).Scan(&actorLevel); errors.Is(err, sql.ErrNoRows) {
			return core.ErrMavenPermissionDenied
		} else if err != nil {
			return fmt.Errorf("inspect Maven removal actor: %w", err)
		}
		if username != actor && actorLevel < core.MavenPermissionManage {
			return core.ErrMavenPermissionDenied
		}
		if username == actor && actorLevel == core.MavenPermissionOwner {
			return core.ErrMavenOwnerCannotLeave
		}
	}
	var current int
	if err := tx.QueryRow(`SELECT permission_level FROM maven_domain_members WHERE repository = ? AND domain = ? AND user_id = ?`,
		globalMavenRepository, domain, targetID).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return core.ErrMavenDomainNotFound
	} else if err != nil {
		return fmt.Errorf("inspect Maven member removal: %w", err)
	}
	if current == core.MavenPermissionOwner {
		if err := requireAnotherMavenOwner(tx, domain, targetID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM maven_domain_members WHERE repository = ? AND domain = ? AND user_id = ?`,
		globalMavenRepository, domain, targetID); err != nil {
		return fmt.Errorf("remove Maven member: %w", err)
	}
	if actor == "" || username != actor {
		if err := insertTeamRemovalMessage(tx, username, "maven", "", domain, time.Now().UnixMilli()); err != nil {
			return fmt.Errorf("notify removed Maven member: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Maven member removal: %w", err)
	}
	return nil
}

func requireMavenMemberPermission(tx *Tx, domain, userID string, required int) error {
	if tx == nil || userID == "" {
		return core.ErrMavenPermissionDenied
	}
	var level int
	err := tx.QueryRow(`SELECT permission_level FROM maven_domain_members
		WHERE repository = ? AND domain = ? AND user_id = ?`, globalMavenRepository, domain, userID).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) || level < required {
		return core.ErrMavenPermissionDenied
	}
	if err != nil {
		return fmt.Errorf("inspect Maven member permission: %w", err)
	}
	return nil
}

func lockMavenDomain(tx *Tx, domain string) error {
	if _, err := tx.Exec(`UPDATE maven_domains SET last_check_at = last_check_at WHERE repository = ? AND domain = ?`,
		globalMavenRepository, domain); err != nil {
		return fmt.Errorf("lock Maven domain team: %w", err)
	}
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM maven_domains WHERE repository = ? AND domain = ?`, globalMavenRepository, domain).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return core.ErrMavenDomainNotFound
	} else if err != nil {
		return fmt.Errorf("inspect Maven domain team lock: %w", err)
	}
	return nil
}

func requireAnotherMavenOwner(tx *Tx, domain, excludedUserID string) error {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM maven_domain_members WHERE repository = ? AND domain = ?
		AND permission_level = ? AND user_id <> ?`, globalMavenRepository, domain, core.MavenPermissionOwner, excludedUserID).Scan(&count); err != nil {
		return fmt.Errorf("count alternate Maven owners: %w", err)
	}
	if count == 0 {
		return core.ErrMavenLastFullMember
	}
	return nil
}
