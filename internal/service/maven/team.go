/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"fmt"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"renop/internal/core"
	"renop/internal/service/audit"
)

type memberRequest struct {
	Users []string `json:"users"`
	Level int      `json:"level"`
}

type levelRequest struct {
	Level int `json:"level"`
}

func listMembers(c fiber.Ctx, state *core.AppState) error {
	_, details, err := authorizedDomain(c, state, core.MavenPermissionPublish)
	if err != nil {
		return apiError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"members": details.Members})
}

func inviteMembers(c fiber.Ctx, state *core.AppState) error {
	user, details, err := authorizedDomain(c, state, core.MavenPermissionManage)
	if err != nil {
		return apiError(c, err)
	}
	if len(c.Body()) > maxManagementRequestSize {
		return c.SendStatus(fiber.StatusRequestEntityTooLarge)
	}
	var request memberRequest
	if err := c.Bind().Body(&request); err != nil || len(request.Users) == 0 || len(request.Users) > 20 {
		return c.Status(fiber.StatusBadRequest).SendString("Choose between 1 and 20 Maven domain members")
	}
	if request.Level < core.MavenPermissionRead || request.Level > core.MavenPermissionOwner {
		return c.Status(fiber.StatusBadRequest).SendString("Maven permission level must be between 0 and 4")
	}
	if request.Level == core.MavenPermissionOwner && len(request.Users) != 1 {
		return c.Status(fiber.StatusBadRequest).SendString("Maven L4 ownership can only be offered to one member")
	}
	if details.Administrator {
		if err := state.GetDB().ForceAddMavenMembers(details.Domain.Domain, user.Username, request.Users, request.Level); err != nil {
			return apiError(c, err)
		}
		logAudit(c, state, audit.ActionMavenTeamAdd, fmt.Sprintf("Domain: %s, members: %d, level: L%d",
			details.Domain.Domain, len(request.Users), request.Level))
		return c.JSON(fiber.Map{"ok": true, "added": len(request.Users)})
	}
	now := time.Now().UnixMilli()
	seen := make(map[string]struct{}, len(request.Users))
	invitations := make([]*core.MavenInvitation, 0, len(request.Users))
	messages := make([]*core.UserMessage, 0, len(request.Users))
	for _, candidate := range request.Users {
		recipient := strings.ToLower(strings.TrimSpace(candidate))
		if recipient == "" || len(recipient) > 255 || strings.EqualFold(recipient, user.Username) {
			return c.Status(fiber.StatusBadRequest).SendString("Maven invitation recipient is invalid")
		}
		if _, duplicate := seen[recipient]; duplicate {
			continue
		}
		seen[recipient] = struct{}{}
		if state.GetTokenByName(recipient) == nil {
			return apiError(c, core.ErrUserProfileNotFound)
		}
		id := uuid.NewString()
		payload, err := json.Marshal(map[string]any{
			"domain": details.Domain.Domain, "inviter": user.Username, "level": request.Level,
		})
		if err != nil {
			return apiError(c, err)
		}
		invitations = append(invitations, &core.MavenInvitation{
			ID: id, Domain: details.Domain.Domain, Inviter: user.Username,
			Recipient: recipient, Level: request.Level, CreatedAt: now,
		})
		messages = append(messages, &core.UserMessage{
			ID: id, Recipient: recipient, Sender: user.Username, Kind: "maven_domain_invite", Severity: "info",
			Title:   "Maven domain invitation",
			Body:    fmt.Sprintf("%s invited you to Maven domain %s with L%d permission.", user.Username, details.Domain.Domain, request.Level),
			Payload: payload, ActionKind: "maven_domain_invite", ActionStatus: core.MessageActionPending,
			CreatedAt: now, ExpiresAt: now + 7*24*3600*1000,
			DedupeKey: fmt.Sprintf("maven:%s:%s", details.Domain.Domain, recipient),
		})
	}
	if len(invitations) == 0 {
		return c.Status(fiber.StatusBadRequest).SendString("Maven invitation has no recipients")
	}
	if err := state.GetDB().CreateMavenInvitations(invitations, messages); err != nil {
		return apiError(c, err)
	}
	logAudit(c, state, audit.ActionMavenTeamInvite, fmt.Sprintf("Domain: %s, recipients: %d, level: L%d",
		details.Domain.Domain, len(invitations), request.Level))
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true, "invited": len(invitations)})
}

func resolveMemberReference(state *core.AppState, reference string) (string, error) {
	reference = strings.ToLower(strings.TrimSpace(reference))
	if reference == "" {
		return "", core.ErrMavenDomainNotFound
	}
	if _, err := uuid.Parse(reference); err != nil {
		return reference, nil
	}
	profile, err := state.GetDB().GetUserProfileByID(reference)
	if err != nil {
		return "", err
	}
	return profile.Username, nil
}

func setMemberLevel(c fiber.Ctx, state *core.AppState) error {
	user, details, err := authorizedDomain(c, state, core.MavenPermissionManage)
	if err != nil {
		return apiError(c, err)
	}
	target, err := resolveMemberReference(state, c.Params("username"))
	if err != nil {
		return apiError(c, core.ErrMavenDomainNotFound)
	}
	var request levelRequest
	if len(c.Body()) > maxManagementRequestSize || c.Bind().Body(&request) != nil ||
		request.Level < core.MavenPermissionRead || request.Level > core.MavenPermissionOwner {
		return c.Status(fiber.StatusBadRequest).SendString("Maven permission level must be between 0 and 4")
	}
	actor := user.Username
	if details.Administrator && !(request.Level == core.MavenPermissionOwner &&
		details.Domain.PermissionLevel == core.MavenPermissionOwner && !strings.EqualFold(target, user.Username)) {
		actor = ""
	}
	if err := state.GetDB().SetMavenMemberLevel(details.Domain.Domain, actor, target, request.Level); err != nil {
		return apiError(c, err)
	}
	logAudit(c, state, audit.ActionMavenTeamLevel, fmt.Sprintf("Domain: %s, member: %s, level: L%d",
		details.Domain.Domain, target, request.Level))
	return c.JSON(fiber.Map{"ok": true})
}

func removeMember(c fiber.Ctx, state *core.AppState) error {
	user, details, err := authorizedDomain(c, state, core.MavenPermissionRead)
	if err != nil {
		return apiError(c, err)
	}
	target, err := resolveMemberReference(state, c.Params("username"))
	if err != nil {
		return apiError(c, core.ErrMavenDomainNotFound)
	}
	isSelf := strings.EqualFold(target, user.Username)
	if !isSelf && !details.Administrator && details.Domain.PermissionLevel < core.MavenPermissionManage {
		return apiError(c, core.ErrMavenPermissionDenied)
	}
	actor := user.Username
	if details.Administrator && !isSelf {
		actor = ""
	}
	if err := state.GetDB().RemoveMavenMember(details.Domain.Domain, actor, target); err != nil {
		return apiError(c, err)
	}
	logAudit(c, state, audit.ActionMavenTeamRemove, fmt.Sprintf("Domain: %s, member: %s",
		details.Domain.Domain, target))
	return c.SendStatus(fiber.StatusNoContent)
}

func respondInvitation(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	decision := strings.ToLower(strings.TrimSpace(c.Params("decision")))
	if decision != "accept" && decision != "reject" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid Maven invitation decision")
	}
	id := strings.TrimSpace(c.Params("id"))
	if uuid.Validate(id) != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid Maven invitation ID")
	}
	if err := state.GetDB().RespondMavenInvitation(id, user.Username, decision == "accept", time.Now().UnixMilli()); err != nil {
		return apiError(c, err)
	}
	logAudit(c, state, audit.ActionMavenTeamInvitation, fmt.Sprintf("Invitation: %s, decision: %s", id, decision))
	return c.JSON(fiber.Map{"ok": true})
}
