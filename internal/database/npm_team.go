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

	"renop/internal/core"
)

var npmTeamRemovalSpec = &teamRemovalSpec{
	format:           "npm",
	table:            "npm_members",
	resourceColumn:   "package_name",
	manageLevel:      core.NPMPermissionTeam,
	ownerLevel:       core.NPMPermissionOwner,
	invalidBatch:     errors.New("npm member removal batch is invalid"),
	invalidName:      errors.New("npm member name is invalid"),
	resourceNotFound: core.ErrNPMPackageNotFound,
	permissionDenied: core.ErrNPMPermissionDenied,
	ownerCannotLeave: core.ErrNPMOwnerCannotLeave,
	lastOwner:        core.ErrNPMLastFullMember,
	lock:             lockNPMPackage,
}

// ListNPMMembers returns one package team ordered by permission and username.
func (db *DB) ListNPMMembers(repository, packageName string) ([]*core.NPMMember, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	rows, err := db.Query(`SELECT m.user_id, COALESCE(p.username, m.username), m.permission_level, m.added_at
		FROM npm_members m LEFT JOIN user_profiles p ON p.user_id = m.user_id
		WHERE m.repository = ? AND m.package_name = ?
		ORDER BY m.permission_level DESC, COALESCE(p.username, m.username)`, repository, packageName)
	if err != nil {
		return nil, fmt.Errorf("list npm members: %w", err)
	}
	members := make([]*core.NPMMember, 0)
	for rows.Next() {
		member := &core.NPMMember{}
		if err := rows.Scan(&member.UserID, &member.Username, &member.Level, &member.AddedAt); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan npm member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate npm members: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close npm members: %w", err)
	}
	return members, nil
}

func cancelNPMInvitations(tx *Tx, where string, args []any, actedAt int64) error {
	rows, err := tx.Query(`SELECT id FROM npm_invitations WHERE `+where, args...)
	if err != nil {
		return fmt.Errorf("list npm invitations for cancellation: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan npm invitation for cancellation: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate npm invitations for cancellation: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close npm invitations for cancellation: %w", err)
	}
	for _, id := range ids {
		if _, err := tx.Exec(`UPDATE user_messages SET action_status = ?, acted_at = ?,
			read_at = CASE WHEN read_at = 0 THEN ? ELSE read_at END
			WHERE id = ? AND action_status = ?`, core.MessageActionCancelled, actedAt, actedAt,
			id, core.MessageActionPending); err != nil {
			return fmt.Errorf("cancel npm invitation message: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM npm_invitations WHERE `+where, args...); err != nil {
		return fmt.Errorf("delete npm invitations: %w", err)
	}
	return nil
}

// CreateNPMInvitations stores package-team invitations and action messages atomically.
func (db *DB) CreateNPMInvitations(invitations []*core.NPMInvitation, messages []*core.UserMessage) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(invitations) == 0 || len(invitations) != len(messages) || len(invitations) > 20 {
		return errors.New("npm invitation is missing")
	}
	for index, invitation := range invitations {
		message := messages[index]
		if invitation == nil || message == nil {
			return errors.New("npm invitation is missing")
		}
		invitation.Repository, invitation.Package = sanitizeNPMKey(invitation.Repository, invitation.Package)
		invitation.Inviter = sanitizeNPMUsername(invitation.Inviter)
		invitation.Recipient = sanitizeNPMUsername(invitation.Recipient)
		if invitation.Level < core.NPMPermissionRead || invitation.Level > core.NPMPermissionOwner {
			return errors.New("npm invitation permission level is invalid")
		}
		if invitation.ID == "" || invitation.ID != message.ID ||
			invitation.Recipient != strings.ToLower(strings.TrimSpace(message.Recipient)) {
			return errors.New("npm invitation message does not match its workflow record")
		}
		if err := normalizeMessage(message); err != nil {
			return err
		}
	}
	first := invitations[0]
	inviterID, err := db.userIDForExistingAccount(first.Inviter)
	if err != nil {
		return core.ErrNPMPermissionDenied
	}
	recipientIDs := make(map[string]string, len(invitations))
	for _, invitation := range invitations {
		recipientID, identityErr := db.userIDForExistingAccount(invitation.Recipient)
		if identityErr != nil {
			return core.ErrUserProfileNotFound
		}
		recipientIDs[invitation.Recipient] = recipientID
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm invitation: %w", err)
	}
	defer tx.Rollback()
	if err := lockNPMPackage(tx, first.Repository, first.Package); err != nil {
		return err
	}
	var inviterLevel int
	if err := tx.QueryRow(`SELECT permission_level FROM npm_members
		WHERE repository = ? AND package_name = ? AND user_id = ?`,
		first.Repository, first.Package, inviterID).Scan(&inviterLevel); errors.Is(err, sql.ErrNoRows) ||
		inviterLevel < core.NPMPermissionTeam {
		return core.ErrNPMPermissionDenied
	} else if err != nil {
		return fmt.Errorf("inspect npm inviter permission: %w", err)
	}
	for index, invitation := range invitations {
		message := messages[index]
		if invitation.Repository != first.Repository || invitation.Package != first.Package ||
			invitation.Inviter != first.Inviter {
			return errors.New("npm invitation batch targets multiple packages")
		}
		if invitation.Level == core.NPMPermissionOwner && inviterLevel < core.NPMPermissionOwner {
			return core.ErrNPMPermissionDenied
		}
		recipientID := recipientIDs[invitation.Recipient]
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM npm_members WHERE repository = ? AND package_name = ? AND user_id = ?`,
			invitation.Repository, invitation.Package, recipientID).Scan(&exists); err == nil {
			return core.ErrNPMMemberExists
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect npm invitation recipient: %w", err)
		}
		var existingID, existingStatus string
		var existingExpiry int64
		var existingInviterLevel int
		err := tx.QueryRow(`SELECT i.id, COALESCE(m.action_status, ''), COALESCE(m.expires_at, 0),
			COALESCE(inviter.permission_level, 0) FROM npm_invitations i
			LEFT JOIN user_messages m ON m.id = i.id AND m.recipient = i.recipient
			LEFT JOIN user_profiles inviter_profile ON inviter_profile.username = i.inviter
			LEFT JOIN npm_members inviter ON inviter.repository = i.repository
				AND inviter.package_name = i.package_name AND inviter.user_id = inviter_profile.user_id
			WHERE i.repository = ? AND i.package_name = ? AND i.recipient = ?`,
			invitation.Repository, invitation.Package, invitation.Recipient).Scan(
			&existingID, &existingStatus, &existingExpiry, &existingInviterLevel)
		if err == nil && existingStatus == core.MessageActionPending && existingExpiry > invitation.CreatedAt &&
			existingInviterLevel >= core.NPMPermissionTeam {
			return core.ErrNPMInvitationExists
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect pending npm invitation: %w", err)
		}
		if err == nil {
			if err := cancelNPMInvitations(tx, `repository = ? AND package_name = ? AND recipient = ?`,
				[]any{invitation.Repository, invitation.Package, invitation.Recipient}, invitation.CreatedAt); err != nil {
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
			return fmt.Errorf("create npm invitation message: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO npm_invitations
			(id, repository, package_name, inviter, recipient, permission_level, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, invitation.ID, invitation.Repository, invitation.Package,
			invitation.Inviter, invitation.Recipient, invitation.Level, invitation.CreatedAt); err != nil {
			return fmt.Errorf("create npm invitation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm invitation: %w", err)
	}
	return nil
}

// RespondNPMInvitation accepts or rejects one package-team invitation.
func (db *DB) RespondNPMInvitation(id, recipient, repository string, accept bool, actedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	id = SanitizeInputString(strings.TrimSpace(id), 64)
	recipient = sanitizeNPMUsername(recipient)
	repository, _ = sanitizeNPMKey(repository, "")
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm invitation response: %w", err)
	}
	defer tx.Rollback()
	invitation := &core.NPMInvitation{ID: id, Recipient: recipient}
	err = tx.QueryRow(`SELECT repository, package_name, inviter, permission_level, created_at
		FROM npm_invitations WHERE id = ? AND recipient = ? AND repository = ?`, id, recipient, repository).Scan(
		&invitation.Repository, &invitation.Package, &invitation.Inviter, &invitation.Level, &invitation.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrNPMInvitationInvalid
	}
	if err != nil {
		return fmt.Errorf("load npm invitation: %w", err)
	}
	if err := lockNPMPackage(tx, invitation.Repository, invitation.Package); err != nil {
		return err
	}
	if accept {
		inviterID, identityErr := userIDForUsernameTx(tx, invitation.Inviter)
		if identityErr != nil {
			return core.ErrNPMInvitationInvalid
		}
		recipientID, identityErr := userIDForUsernameTx(tx, recipient)
		if identityErr != nil {
			return core.ErrNPMInvitationInvalid
		}
		var inviterLevel int
		if err := tx.QueryRow(`SELECT permission_level FROM npm_members
			WHERE repository = ? AND package_name = ? AND user_id = ?`,
			invitation.Repository, invitation.Package, inviterID).Scan(&inviterLevel); errors.Is(err, sql.ErrNoRows) ||
			inviterLevel < core.NPMPermissionTeam {
			return core.ErrNPMInvitationInvalid
		} else if err != nil {
			return fmt.Errorf("validate npm inviter: %w", err)
		}
		if invitation.Level == core.NPMPermissionOwner && inviterLevel < core.NPMPermissionOwner {
			return core.ErrNPMInvitationInvalid
		}
		var memberLevel int
		err := tx.QueryRow(`SELECT permission_level FROM npm_members
			WHERE repository = ? AND package_name = ? AND user_id = ?`,
			invitation.Repository, invitation.Package, recipientID).Scan(&memberLevel)
		if err == nil {
			return core.ErrNPMMemberExists
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect npm invitation membership: %w", err)
		}
		if invitation.Level == core.NPMPermissionOwner {
			if _, err := tx.Exec(`UPDATE npm_members SET permission_level = ? WHERE repository = ?
				AND package_name = ? AND permission_level = ? AND user_id != ?`,
				core.NPMPermissionTeam, invitation.Repository, invitation.Package,
				core.NPMPermissionOwner, recipientID); err != nil {
				return fmt.Errorf("demote previous npm L4 owner: %w", err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO npm_members
			(repository, package_name, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
			invitation.Repository, invitation.Package, recipient, recipientID, invitation.Level, actedAt); err != nil {
			return fmt.Errorf("accept npm membership: %w", err)
		}
	}
	status := core.MessageActionRejected
	if accept {
		status = core.MessageActionAccepted
	}
	result, err := tx.Exec(`UPDATE user_messages SET action_status = ?, acted_at = ?,
		read_at = CASE WHEN read_at = 0 THEN ? ELSE read_at END
		WHERE id = ? AND recipient = ? AND action_kind = 'npm_package_invite'
		AND action_status = ? AND (expires_at = 0 OR expires_at > ?)`,
		status, actedAt, actedAt, id, recipient, core.MessageActionPending, actedAt)
	if err != nil {
		return fmt.Errorf("update npm invitation message: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return core.ErrNPMInvitationInvalid
	}
	if _, err := tx.Exec(`DELETE FROM npm_invitations WHERE id = ? AND recipient = ?`, id, recipient); err != nil {
		return fmt.Errorf("complete npm invitation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm invitation response: %w", err)
	}
	return nil
}

// ForceAddNPMMembers adds members without an invitation for administrator workflows.
func (db *DB) ForceAddNPMMembers(repository, packageName, actor string, usernames []string, level int) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(usernames) == 0 || len(usernames) > 20 || level < core.NPMPermissionRead || level > core.NPMPermissionOwner {
		return errors.New("npm member addition batch is invalid")
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	actor = sanitizeNPMUsername(actor)
	normalizedUsers := make([]string, 0, len(usernames))
	userIDs := make(map[string]string, len(usernames))
	seen := make(map[string]struct{}, len(usernames))
	for _, candidate := range usernames {
		username := sanitizeNPMUsername(candidate)
		if username == "" {
			return errors.New("npm member name is invalid")
		}
		if _, duplicate := seen[username]; duplicate {
			continue
		}
		seen[username] = struct{}{}
		userID, err := db.userIDForExistingAccount(username)
		if err != nil {
			return core.ErrUserProfileNotFound
		}
		userIDs[username] = userID
		normalizedUsers = append(normalizedUsers, username)
	}
	if len(normalizedUsers) == 0 || (level == core.NPMPermissionOwner && len(normalizedUsers) != 1) {
		return errors.New("npm L4 ownership can only be assigned to one member at a time")
	}
	now := time.Now().UnixMilli()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm force member addition: %w", err)
	}
	defer tx.Rollback()
	if err := lockNPMPackage(tx, repository, packageName); err != nil {
		return err
	}
	for _, username := range normalizedUsers {
		var existing int
		if err := tx.QueryRow(`SELECT 1 FROM npm_members WHERE repository = ? AND package_name = ? AND user_id = ?`,
			repository, packageName, userIDs[username]).Scan(&existing); err == nil {
			return core.ErrNPMMemberExists
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect npm member: %w", err)
		}
	}
	if level == core.NPMPermissionOwner {
		if _, err := tx.Exec(`UPDATE npm_members SET permission_level = ? WHERE repository = ?
			AND package_name = ? AND permission_level = ?`, core.NPMPermissionTeam,
			repository, packageName, core.NPMPermissionOwner); err != nil {
			return fmt.Errorf("demote previous npm L4 owner: %w", err)
		}
	}
	for _, username := range normalizedUsers {
		if _, err := tx.Exec(`INSERT INTO npm_members
			(repository, package_name, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
			repository, packageName, username, userIDs[username], level, now); err != nil {
			return fmt.Errorf("insert npm member: %w", err)
		}
		if err := cancelNPMInvitations(tx, `repository = ? AND package_name = ? AND recipient = ?`,
			[]any{repository, packageName, username}, now); err != nil {
			return err
		}
		if username != actor {
			if err := insertAcceptedMembershipMessage(tx, username, actor, "npm_package_invite",
				"npm package membership added",
				fmt.Sprintf("%s added you to collaborate on %s with L%d permission.", actor, packageName, level),
				map[string]any{"repository": repository, "package": packageName, "inviter": actor, "level": level}, now); err != nil {
				return fmt.Errorf("create npm membership message: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm force member addition: %w", err)
	}
	return nil
}

// SetNPMMemberLevel changes one member's permission while preserving an L4 owner.
func (db *DB) SetNPMMemberLevel(repository, packageName, actor, username string, level int) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if level < core.NPMPermissionRead || level > core.NPMPermissionOwner {
		return errors.New("npm permission level is invalid")
	}
	repository, packageName = sanitizeNPMKey(repository, packageName)
	actor = sanitizeNPMUsername(actor)
	username = sanitizeNPMUsername(username)
	targetID, err := db.userIDForUsername(username)
	if err != nil {
		return core.ErrNPMPackageNotFound
	}
	actorID := ""
	if actor != "" {
		actorID, err = db.userIDForUsername(actor)
		if err != nil {
			return core.ErrNPMPermissionDenied
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin npm member update: %w", err)
	}
	defer tx.Rollback()
	if err := lockNPMPackage(tx, repository, packageName); err != nil {
		return err
	}
	actorLevel := 0
	if actor != "" {
		if err := tx.QueryRow(`SELECT permission_level FROM npm_members WHERE repository = ?
			AND package_name = ? AND user_id = ?`, repository, packageName, actorID).Scan(&actorLevel); err != nil ||
			actorLevel < core.NPMPermissionTeam {
			return core.ErrNPMPermissionDenied
		}
	}
	var current int
	if err := tx.QueryRow(`SELECT permission_level FROM npm_members WHERE repository = ?
		AND package_name = ? AND user_id = ?`, repository, packageName, targetID).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return core.ErrNPMPackageNotFound
	} else if err != nil {
		return fmt.Errorf("inspect npm member: %w", err)
	}
	if current == core.NPMPermissionOwner && level < core.NPMPermissionOwner {
		if err := requireAnotherFullNPMMember(tx, repository, packageName, targetID); err != nil {
			return err
		}
	}
	if level == core.NPMPermissionOwner && current != core.NPMPermissionOwner {
		if actor != "" {
			if actorLevel != core.NPMPermissionOwner || actor == username {
				return core.ErrNPMPermissionDenied
			}
			if _, err := tx.Exec(`UPDATE npm_members SET permission_level = ? WHERE repository = ?
				AND package_name = ? AND user_id = ?`, current, repository, packageName, actorID); err != nil {
				return fmt.Errorf("exchange previous npm L4 owner permission: %w", err)
			}
		} else if _, err := tx.Exec(`UPDATE npm_members SET permission_level = ? WHERE repository = ?
			AND package_name = ? AND permission_level = ? AND user_id != ?`, core.NPMPermissionTeam,
			repository, packageName, core.NPMPermissionOwner, targetID); err != nil {
			return fmt.Errorf("demote previous npm L4 owner: %w", err)
		}
	}
	if _, err := tx.Exec(`UPDATE npm_members SET permission_level = ? WHERE repository = ?
		AND package_name = ? AND user_id = ?`, level, repository, packageName, targetID); err != nil {
		return fmt.Errorf("update npm member: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit npm member update: %w", err)
	}
	return nil
}

// RemoveNPMMember removes one team member.
func (db *DB) RemoveNPMMember(repository, packageName, actor, username string) error {
	return db.RemoveNPMMembers(repository, packageName, actor, []string{username})
}

// RemoveNPMMembers removes a bounded member batch.
func (db *DB) RemoveNPMMembers(repository, packageName, actor string, usernames []string) error {
	repository, packageName = sanitizeNPMKey(repository, packageName)
	actor = sanitizeNPMUsername(actor)
	return db.removeTeamMembers(repository, packageName, actor, usernames, sanitizeNPMUsername, npmTeamRemovalSpec)
}

func requireAnotherFullNPMMember(tx *Tx, repository, packageName, excludedUserID string) error {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM npm_members WHERE repository = ? AND package_name = ?
		AND permission_level = ? AND user_id <> ?`, repository, packageName,
		core.NPMPermissionOwner, excludedUserID).Scan(&count); err != nil {
		return fmt.Errorf("count npm L4 members: %w", err)
	}
	if count == 0 {
		return core.ErrNPMLastFullMember
	}
	return nil
}
