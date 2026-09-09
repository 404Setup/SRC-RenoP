/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package ticket

import (
	"fmt"
	"strings"
	"time"

	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/repositorygate"
	"renop/internal/service/ticketnotify"
	"renop/internal/utils"

	"github.com/gofiber/fiber/v3"
)

func logSupportAudit(c fiber.Ctx, state *core.AppState, task *core.ReviewTask, action, details string) {
	if task.Kind != core.TicketKindReport {
		logReviewAudit(c, state, action, details)
		return
	}
	// A reported administrator can read global logs, but must not identify either party through them.
	audit.Log(state, &core.AuditLogEntry{Kind: "system", Operator: "system", Trigger: "web",
		Action: action, Details: details, CreatedAt: time.Now().UnixMilli()})
}

func getTask(c fiber.Ctx, state *core.AppState) error {
	username, _, err := currentUser(c)
	if err != nil {
		return reviewError(c, err)
	}
	task, err := state.GetDB().GetTicket(c.Params("id"), username)
	if err != nil {
		return reviewError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(task)
}

func createTask(c fiber.Ctx, state *core.AppState) error {
	username, _, err := currentUser(c)
	if err != nil {
		return reviewError(c, err)
	}
	var request core.TicketRequest
	if err := utils.ReadJSONLimited(c, &request, 48<<10); err != nil {
		return reviewError(c, fiber.ErrBadRequest)
	}
	request.Repository = strings.ToLower(strings.TrimSpace(request.Repository))
	if request.Kind == core.TicketKindReport {
		request.Target.Repository = strings.ToLower(strings.TrimSpace(request.Target.Repository))
		request.Target.Format = strings.ToLower(strings.TrimSpace(request.Target.Format))
		request.Repository = request.Target.Repository
	}
	release := repositorygate.AcquireMutation(request.Repository)
	defer release()
	if request.Repository != "" {
		cfg := state.Inner.Config.Load()
		if cfg == nil {
			return reviewError(c, core.ErrDatabaseUnavailable)
		}
		repo := cfg.Maven.Repositories[request.Repository]
		if repo == nil || !auth.GetUser(c).CheckReadPermission(repo.Name, "", repo.Visibility, true) ||
			request.Kind == core.TicketKindReport && repo.NormalizedFormat() != request.Target.Format {
			return reviewError(c, core.ErrReviewPermissionDenied)
		}
	}
	task, err := state.GetDB().CreateTicket(request, username, auth.CurrentSessionToken(c), time.Now().UnixMilli())
	if err != nil {
		return reviewError(c, err)
	}
	ticketnotify.DeliverTask(state, task)
	logSupportAudit(c, state, task, audit.ActionTicketCreate, fmt.Sprintf("Ticket: %s, kind: %s, repository: %s", task.ID, task.Kind, task.Repository))
	c.Set(fiber.HeaderLocation, "/api/tickets/"+task.ID)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Status(fiber.StatusCreated).JSON(task)
}

func transitionTask(c fiber.Ctx, state *core.AppState) error {
	username, _, err := currentUser(c)
	if err != nil {
		return reviewError(c, err)
	}
	var action core.TicketAction
	if err := utils.ReadJSONLimited(c, &action, 24<<10); err != nil || action.Force && action.Action != "claim" {
		return reviewError(c, fiber.ErrBadRequest)
	}
	ticketMutationLock.Lock()
	defer ticketMutationLock.Unlock()
	task, err := state.GetDB().TransitionTicket(c.Params("id"), username, auth.CurrentSessionToken(c), action, time.Now().UnixMilli())
	if err != nil {
		return reviewError(c, err)
	}
	if task.Status != core.ReviewStatusPending {
		ticketnotify.DeliverDecision(state, task)
	} else if action.Action == "escalate" || action.Action == "release" {
		ticketnotify.DeliverPendingTransition(state, task)
	}
	logSupportAudit(c, state, task, audit.ActionTicketUpdate, fmt.Sprintf("Ticket: %s, action: %s", task.ID, action.Action))
	return getTask(c, state)
}
