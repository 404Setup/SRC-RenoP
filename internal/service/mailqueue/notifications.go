/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package mailqueue

import (
	"errors"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"renop/internal/core"
	"renop/internal/mail"
	"renop/internal/service/audit"
)

func ignorableNotification(err error) bool {
	return errors.Is(err, ErrDisabled) || errors.Is(err, ErrNoAccount) || errors.Is(err, ErrRecipientBlocked) || errors.Is(err, core.ErrAccountDeleted) || errors.Is(err, core.ErrUserProfileNotFound)
}

func (w *worker) notifications(control mail.Control, now time.Time) error {
	db := w.state.GetDB()
	entries, err := db.MailAuditEvents(control.AuditCursor)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		scene := ""
		switch entry.Action {
		case audit.ActionUserRegister:
			scene = "registration_success"
		case audit.ActionProfileUpdate:
			if entry.Details == "Updated private login email" {
				scene = "email_changed"
			}
		case audit.ActionPasswordUpdate:
			scene = "password_changed"
			if entry.Details == "Updated password-login policy" {
				scene = "security_changed"
			}
		case audit.ActionFIDOUpdate:
			scene = "security_changed"
		case audit.ActionUserPermissionUpdate:
			scene = "permission_changed"
		case audit.ActionUserBan:
			scene = "account_banned"
		case audit.ActionUserUnban:
			scene = "account_unbanned"
		case audit.ActionReviewRequest:
			scene = "review_requested"
		case audit.ActionPublicationQuotaUpdate:
			if strings.HasPrefix(entry.Details, "Owner type: user,") {
				scene = "quota_changed"
			}
		case audit.ActionLogin:
			previous, err := db.PreviousMailLoginIP(entry.Username, entry.ID)
			if err != nil {
				return err
			}
			if differentNetwork(previous, entry.IP) {
				scene = "unusual_login"
			}
		}
		if scene != "" && entry.CreatedAt >= control.EnabledSince && entry.CreatedAt > now.Add(-24*time.Hour).UnixMilli() {
			_, err = Enqueue(w.state, Request{ID: "audit-" + strconv.FormatInt(entry.ID, 10), Username: entry.Username, Actor: entry.Operator, Scene: scene, Data: mail.TemplateData{}})
			if err != nil && !ignorableNotification(err) {
				return err
			}
		}
		if err = db.AdvanceMailAuditCursor(w.owner, entry.ID, time.Now().UnixMilli()); err != nil {
			return err
		}
	}
	messages, err := db.MailMessageEvents(max(control.EnabledSince, now.Add(-24*time.Hour).UnixMilli()))
	if err != nil {
		return err
	}
	for _, message := range messages {
		scene := "notification"
		switch message.Kind {
		case "review_pending":
			scene = "pending_reviews"
		case "review_result":
			scene = "review_status"
		default:
			if strings.Contains(message.Kind, "invitation") {
				scene = "collaboration_invitation"
				if strings.Contains(message.Kind, "super_team") {
					scene = "super_team_invitation"
				}
			}
		}
		_, err = Enqueue(w.state, Request{ID: message.ID, Username: message.Recipient, Actor: "system", Scene: scene})
		if err != nil && !ignorableNotification(err) {
			return err
		}
		if err = db.AcknowledgeMailMessage(message.ID, time.Now().UnixMilli()); err != nil {
			return err
		}
	}
	return nil
}

func differentNetwork(before, after string) bool {
	previous, err := netip.ParseAddr(before)
	if err != nil {
		return false
	}
	current, err := netip.ParseAddr(after)
	if err != nil {
		return false
	}
	previous = previous.Unmap()
	current = current.Unmap()
	if previous.Is4() != current.Is4() {
		return true
	}
	bits := 56
	if current.Is4() {
		bits = 24
	}
	return netip.PrefixFrom(previous, bits).Masked() != netip.PrefixFrom(current, bits).Masked()
}
