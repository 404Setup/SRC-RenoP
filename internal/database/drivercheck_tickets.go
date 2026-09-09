/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"errors"
	"fmt"

	"renop/internal/core"
)

func checkTickets(db *DB, suffix string, now int64) error {
	reporter, target := "ticket-reporter-"+suffix, "ticket-target-"+suffix
	moderator, peer := "ticket-mod-"+suffix, "ticket-peer-"+suffix
	for name, roles := range map[string][]string{
		reporter: {"base"}, target: {"manager"}, moderator: {"canmoderate:*"}, peer: {"manager"},
	} {
		if err := db.SaveToken(&core.AccessToken{Name: name, Permissions: roles}); err != nil {
			return err
		}
		session := &core.Session{PublicID: name, Username: name, CreatedAt: now}
		session.LastActive.Store(now)
		if err := db.SaveSession(session, name+"-session"); err != nil {
			return err
		}
	}
	request := core.TicketRequest{Kind: core.TicketKindReport, Title: "Abuse report", Body: "Please investigate.",
		Target: core.ResourceLockTarget{Format: "user", Name: target}}
	task, err := db.CreateTicket(request, reporter, reporter+"-session", now)
	if err != nil {
		return err
	}
	if _, err := db.CreateTicket(request, reporter, reporter+"-session", now); !errors.Is(err, core.ErrReviewTaskExists) {
		return fmt.Errorf("duplicate report: %v", err)
	}
	if _, err := db.GetTicket(task.ID, target); !errors.Is(err, core.ErrReviewPermissionDenied) {
		return fmt.Errorf("reported administrator privacy: %v", err)
	}
	act := func(actor, action string, force bool) (*core.ReviewTask, error) {
		return db.TransitionTicket(task.ID, actor, actor+"-session", core.TicketAction{
			Action: action, Force: force, Outcome: "upheld", Response: "The report was investigated."}, now+1)
	}
	if _, err := act(moderator, "complete", false); !errors.Is(err, core.ErrTicketClaimRequired) {
		return fmt.Errorf("unclaimed completion: %v", err)
	}
	if _, err := act(moderator, "claim", false); err != nil {
		return err
	}
	if err := db.UpdateToken(moderator, func(token *core.AccessToken) { token.Permissions = []string{"manager"} }); err != nil {
		return err
	}
	if view, err := db.GetTicket(task.ID, peer); err != nil || !view.AssigneeAdmin {
		return errorsOrMissing(err, "promoted ticket assignee authority")
	}
	if _, err := act(peer, "claim", true); !errors.Is(err, core.ErrTicketOccupied) {
		return fmt.Errorf("promoted administrator takeover: %v", err)
	}
	if err := db.UpdateToken(moderator, func(token *core.AccessToken) { token.Permissions = []string{"canmoderate:*"} }); err != nil {
		return err
	}
	if _, err := act(peer, "claim", false); !errors.Is(err, core.ErrTicketOccupied) {
		return fmt.Errorf("occupied claim: %v", err)
	}
	if _, err := act(peer, "claim", true); err != nil {
		return err
	}
	if err := db.UpdateToken(peer, func(token *core.AccessToken) { token.Permissions = []string{"base"} }); err != nil {
		return err
	}
	if err := db.UpdateToken(moderator, func(token *core.AccessToken) { token.Permissions = []string{"manager"} }); err != nil {
		return err
	}
	if view, err := db.GetTicket(task.ID, moderator); err != nil || view.AssigneeAdmin {
		return errorsOrMissing(err, "demoted ticket assignee authority")
	}
	if _, err := act(moderator, "claim", true); err != nil {
		return err
	}
	if _, err := act(moderator, "release", false); err != nil {
		return err
	}
	if err := db.UpdateToken(peer, func(token *core.AccessToken) { token.Permissions = []string{"manager"} }); err != nil {
		return err
	}
	if _, err := act(peer, "claim", false); err != nil {
		return err
	}
	if _, err := act(moderator, "complete", false); !errors.Is(err, core.ErrTicketClaimRequired) {
		return fmt.Errorf("superseded assignee: %v", err)
	}
	if task, err = act(peer, "process", false); err != nil || task.TicketState.Status != core.TicketProcessed {
		return errorsOrMissing(err, "processed ticket persistence")
	}
	if task, err = act(peer, "complete", false); err != nil || task.TicketState.Status != core.TicketCompleted {
		return errorsOrMissing(err, "completed ticket persistence")
	}
	if _, err := act(peer, "complete", false); !errors.Is(err, core.ErrReviewTaskConflict) {
		return fmt.Errorf("duplicate completion: %v", err)
	}
	view, err := db.GetTicket(task.ID, reporter)
	if err != nil || view.DecidedBy != "" || view.Assignee != "" || view.Outcome != "upheld" {
		return errorsOrMissing(err, "requester identity redaction")
	}
	view, err = db.GetTicket(task.ID, moderator)
	if err != nil || view.DecidedBy != peer || view.Assignee != peer {
		return errorsOrMissing(err, "staff identity visibility")
	}
	list, total, err := db.ListReviewTasks(core.ReviewTaskListOptions{Username: moderator,
		TicketStatus: core.TicketCompleted, ResourceTypes: []string{"user"}, Limit: 100})
	if err != nil || total != 1 || len(list) != 1 || list[0].ID != task.ID {
		return errorsOrMissing(err, "completed report listing")
	}
	for _, name := range []string{reporter, target, moderator, peer} {
		if err := db.DeleteSession(name + "-session"); err != nil {
			return err
		}
	}
	return nil
}
