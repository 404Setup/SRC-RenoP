/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import "errors"

const (
	TicketKindFeedback   = "feedback"
	TicketKindSuggestion = "suggestion"
	TicketKindReport     = "report"
	TicketUnprocessed    = "unprocessed"
	TicketInProgress     = "in_progress"
	TicketProcessed      = "processed"
	TicketClosed         = "closed"
	TicketCompleted      = "completed"
)

var (
	ErrTicketClaimRequired   = errors.New("ticket must be claimed by the current actor")
	ErrTicketOccupied        = errors.New("ticket is already assigned")
	ErrTicketEscalationLimit = errors.New("ticket escalation limit reached")
)

// TicketState adds the shared support lifecycle to a persisted workflow.
type TicketState struct {
	Status        string `json:"ticket_status"`
	Title         string `json:"title,omitempty"`
	Body          string `json:"body,omitempty"`
	AssigneeID    string `json:"-"`
	Assignee      string `json:"assignee,omitempty"`
	AssigneeAdmin bool   `json:"-"`
	AdminOnly     bool   `json:"admin_only"`
	Escalations   int    `json:"escalations"`
	EscalatedByID string `json:"-"`
	EscalatedBy   string `json:"escalated_by,omitempty"`
	Revision      int64  `json:"-"`
	TargetUserIDs string `json:"-"`
	Outcome       string `json:"outcome,omitempty"`
	Response      string `json:"response,omitempty"`
	ChangedAt     int64  `json:"changed_at"`
}

// TicketRequest describes feedback, a suggestion, or a report of a visible resource.
type TicketRequest struct {
	Kind       string             `json:"kind"`
	Title      string             `json:"title"`
	Body       string             `json:"body"`
	Repository string             `json:"repository"`
	Target     ResourceLockTarget `json:"target"`
}

// TicketAction changes assignment or records a support outcome.
type TicketAction struct {
	Action   string `json:"action"`
	Force    bool   `json:"force"`
	Outcome  string `json:"outcome"`
	Response string `json:"response"`
}

// ValidTicketStatus accepts the complete support lifecycle and the list wildcard.
func ValidTicketStatus(status string) bool {
	switch status {
	case "all", TicketUnprocessed, TicketInProgress, TicketProcessed, TicketClosed, TicketCompleted:
		return true
	default:
		return false
	}
}
