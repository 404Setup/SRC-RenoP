/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package mailqueue owns RenoP's durable serial email delivery and notification worker.
package mailqueue

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/url"
	"time"

	"renop/internal/core"
	"renop/internal/mail"
)

var ErrDisabled = errors.New("mail_disabled")
var ErrNoAccount = errors.New("mail_no_account")
var ErrRecipientBlocked = errors.New("mail_recipient_blocked")

// Request describes an authorized email operation; browser-facing handlers supply identity and IP.
type Request struct {
	ID        string
	AccountID string
	Username  string
	UserID    string
	Actor     string
	To        string
	Scene     string
	IP        string
	Manual    bool
	ExpiresAt int64
	Data      mail.TemplateData
}

// Receipt exposes a job identifier and a capability for callers without a session.
type Receipt struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Ticket string `json:"ticket,omitempty"`
}

// Enqueue validates live identity, recipient policy, and routing before durable insertion.
func Enqueue(state *core.AppState, request Request) (Receipt, error) {
	if state == nil || state.GetDB() == nil {
		return Receipt{}, core.ErrDatabaseUnavailable
	}
	cfg := state.Inner.Config.Load().Mail
	if !cfg.Enabled {
		return Receipt{}, ErrDisabled
	}
	if request.Username != "" {
		profile, err := state.GetDB().GetUserProfile(request.Username)
		if err != nil {
			return Receipt{}, err
		}
		if profile == nil {
			return Receipt{}, core.ErrUserProfileNotFound
		}
		request.UserID = profile.UserID
		if request.To == "" {
			security, err := state.GetDB().GetAccountSecurity(request.Username)
			if err != nil {
				return Receipt{}, err
			}
			if security == nil || security.Email == "" {
				return Receipt{}, ErrRecipientBlocked
			}
			request.To = security.Email
		}
	}
	address, err := mail.Address(request.To)
	if err != nil {
		return Receipt{}, ErrRecipientBlocked
	}
	if !cfg.Allows(address) {
		return Receipt{}, ErrRecipientBlocked
	}
	account := cfg.SelectAccount(request.Scene)
	if request.AccountID != "" {
		account = nil
		for i := range cfg.Accounts {
			if cfg.Accounts[i].ID == request.AccountID && cfg.Accounts[i].Enabled {
				account = &cfg.Accounts[i]
				break
			}
		}
	}
	if account == nil {
		return Receipt{}, ErrNoAccount
	}
	if request.Data.Username == "" {
		request.Data.Username = request.Username
	}
	if request.Data.URL == "" {
		request.Data.URL = cfg.PublicURL
		if request.Username != "" {
			request.Data.URL = cfg.PublicURL + "/user/" + url.PathEscape(request.Username) + "/edit"
		}
	}
	message, err := cfg.Render(request.Scene, request.Data)
	if err != nil {
		return Receipt{}, err
	}
	if request.ID == "" {
		request.ID = rand.Text()
	}
	now := time.Now().UnixMilli()
	if request.ExpiresAt == 0 {
		request.ExpiresAt = now + int64(24*time.Hour/time.Millisecond)
	}
	message.ID = request.ID
	message.To = address
	message.CreatedAt = now
	job := &mail.Job{ID: request.ID, AccountID: account.ID, UserID: request.UserID, Actor: request.Actor, Scene: request.Scene, Message: message, CreatedAt: now, ExpiresAt: request.ExpiresAt}
	receipt := Receipt{ID: job.ID, Status: "queued"}
	if request.Manual {
		if request.IP == "" {
			return Receipt{}, errors.New("mail_request_invalid")
		}
		receipt.Ticket = rand.Text()
		job.TicketHash = ticketHash(receipt.Ticket)
	}
	ip := ""
	if request.Manual {
		ip = request.IP
	}
	created, err := state.GetDB().QueueMailJob(job, cfg.EncryptionKey, ip, cfg.ManualRate)
	if err != nil {
		return Receipt{}, err
	}
	if created {
		Wake(state)
	}
	return receipt, nil
}

// Wake coalesces enqueue and configuration notifications without creating another worker.
func Wake(state *core.AppState) {
	if state == nil || state.Inner == nil {
		return
	}
	select {
	case state.Inner.MailWake <- struct{}{}:
	default:
	}
}

func ticketHash(ticket string) string {
	hash := sha256.Sum256([]byte(ticket))
	return hex.EncodeToString(hash[:])
}

// CanRead permits the immutable account owner or the original request capability.
func CanRead(job *mail.Job, userID, ticket string) bool {
	if job == nil {
		return false
	}
	if userID != "" && job.UserID == userID {
		return true
	}
	return ticket != "" && len(ticket) <= 128 && job.TicketHash != "" && subtle.ConstantTimeCompare([]byte(job.TicketHash), []byte(ticketHash(ticket))) == 1
}

// View returns stable status codes without provider diagnostics, recipients, or message contents.
func View(job *mail.Job) map[string]any {
	code := job.Result.Code
	if job.Status == "failed" {
		code = "mail_delivery_failed"
	} else if job.Status == "unknown" {
		code = "mail_status_unknown"
	}
	return map[string]any{"id": job.ID, "account_id": job.AccountID, "scene": job.Scene, "status": job.Status, "code": code, "created_at": job.CreatedAt, "updated_at": job.UpdatedAt}
}
