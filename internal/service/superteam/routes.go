/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package superteam manages engine-independent global publishing teams.
package superteam

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
)

const (
	maxRequestBytes         = 16 << 10
	maxInvitationRecipients = 20
	invitationLifetime      = 7 * 24 * time.Hour
	maxUserSuggestions      = 8
	maxLimitOverride        = 1000
)

type createRequest struct {
	Prefix      string `json:"prefix"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type updateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type membersRequest struct {
	Users []string `json:"users"`
	Level int      `json:"level"`
}

type levelRequest struct {
	Level int `json:"level"`
}

type limitOverrideRequest struct {
	CreateLimit *int `json:"create_limit"`
	JoinLimit   *int `json:"join_limit"`
}

// SetupRoutes registers global-team account and administrator APIs.
func SetupRoutes(router fiber.Router, state *core.AppState) {
	base := router.Group("/super-teams")
	base.Get("/limits", func(c fiber.Ctx) error { return getOwnLimits(c, state) })
	base.Get("/eligible", func(c fiber.Ctx) error { return listEligibleTeams(c, state) })
	base.Get("/users/:username/limits", func(c fiber.Ctx) error { return getUserLimits(c, state) })
	base.Put("/users/:username/limits", func(c fiber.Ctx) error { return putUserLimits(c, state) })
	base.Post("/invitations/:id/:decision", func(c fiber.Ctx) error { return respondInvitation(c, state) })
	base.Get("", func(c fiber.Ctx) error { return listTeams(c, state) })
	base.Post("", func(c fiber.Ctx) error { return createTeam(c, state) })
	base.Get("/:prefix/users/search", func(c fiber.Ctx) error { return searchUsers(c, state) })
	base.Get("/:prefix", func(c fiber.Ctx) error { return getTeam(c, state) })
	base.Put("/:prefix", func(c fiber.Ctx) error { return updateTeam(c, state) })
	base.Delete("/:prefix", func(c fiber.Ctx) error { return deleteTeam(c, state) })
	base.Post("/:prefix/members", func(c fiber.Ctx) error { return addMembers(c, state) })
	base.Put("/:prefix/members/:username", func(c fiber.Ctx) error { return setMemberLevel(c, state) })
	base.Delete("/:prefix/members/:username", func(c fiber.Ctx) error { return removeMember(c, state) })
}

func authenticated(c fiber.Ctx) (*config.User, error) {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return nil, fiber.ErrUnauthorized
	}
	return user, nil
}

func teamConfig(state *core.AppState) config.SuperTeamConfig {
	return state.Inner.Config.Load().SuperTeams
}

func apiError(c fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	code := "operation_failed"
	switch {
	case errors.Is(err, fiber.ErrBadRequest):
		status, code = fiber.StatusBadRequest, "invalid_request"
	case errors.Is(err, fiber.ErrUnauthorized):
		status, code = fiber.StatusUnauthorized, "authentication_required"
	case errors.Is(err, core.ErrUserProfileNotFound):
		status, code = fiber.StatusBadRequest, "user_not_found"
	case errors.Is(err, core.ErrSuperTeamNotFound):
		status, code = fiber.StatusNotFound, "team_not_found"
	case errors.Is(err, core.ErrSuperTeamPermissionDenied):
		status, code = fiber.StatusForbidden, "permission_denied"
	case errors.Is(err, core.ErrSuperTeamExists):
		status, code = fiber.StatusConflict, "team_exists"
	case errors.Is(err, core.ErrSuperTeamMemberExists):
		status, code = fiber.StatusConflict, "member_exists"
	case errors.Is(err, core.ErrSuperTeamInvitationExists):
		status, code = fiber.StatusConflict, "invitation_pending"
	case errors.Is(err, core.ErrSuperTeamInvitationInvalid):
		status, code = fiber.StatusConflict, "invitation_invalid"
	case errors.Is(err, core.ErrSuperTeamLastOwner), errors.Is(err, core.ErrSuperTeamOwnerCannotLeave):
		status, code = fiber.StatusConflict, "last_owner"
	case errors.Is(err, core.ErrSuperTeamCreateLimit):
		status, code = fiber.StatusConflict, "create_limit"
	case errors.Is(err, core.ErrSuperTeamJoinLimit):
		status, code = fiber.StatusConflict, "join_limit"
	case errors.Is(err, core.ErrSuperTeamNotEmpty):
		status, code = fiber.StatusConflict, "team_not_empty"
	case errors.Is(err, core.ErrDatabaseUnavailable):
		status, code = fiber.StatusServiceUnavailable, "service_unavailable"
	}
	c.Set("X-Renop-Error-Code", code)
	return c.Status(status).SendString(code)
}

func auditAction(c fiber.Ctx, state *core.AppState, action, details string) {
	username, operator, method, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: action, Details: details,
		AuthMethod: method, SessionID: sessionID, IP: ip, CreatedAt: time.Now().UnixMilli(),
	})
}

func parsePage(c fiber.Ctx) (limit, offset int, err error) {
	limit, err = strconv.Atoi(c.Query("limit", "12"))
	if err != nil || limit < 1 || limit > 100 {
		return 0, 0, fiber.ErrBadRequest
	}
	offset, err = strconv.Atoi(c.Query("offset", "0"))
	if err != nil || offset < 0 {
		return 0, 0, fiber.ErrBadRequest
	}
	return limit, offset, nil
}

func listTeams(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		return apiError(c, err)
	}
	teams, total, err := state.GetDB().ListSuperTeams(user.Username, user.IsManager(), limit, offset)
	if err != nil {
		return apiError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{
		"teams": teams, "total": total, "limit": limit, "offset": offset, "administrator": user.IsManager(),
	})
}

func listEligibleTeams(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		return apiError(c, err)
	}
	minimumRole, err := strconv.Atoi(c.Query("minimum_role", strconv.Itoa(core.SuperTeamRoleManage)))
	if err != nil || minimumRole < core.SuperTeamRoleRead || minimumRole > core.SuperTeamRoleOwner {
		return apiError(c, fiber.ErrBadRequest)
	}
	teams, total, err := state.GetDB().ListManageableSuperTeams(
		user.Username, minimumRole, limit, offset)
	if err != nil {
		return apiError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"teams": teams, "total": total, "limit": limit, "offset": offset})
}

func createTeam(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	if len(c.Body()) > maxRequestBytes {
		return c.SendStatus(fiber.StatusRequestEntityTooLarge)
	}
	var request createRequest
	if err := c.Bind().Body(&request); err != nil {
		return apiError(c, fiber.ErrBadRequest)
	}
	prefix, prefixValid := core.NormalizeSuperTeamPrefix(request.Prefix)
	name, nameValid := core.NormalizeSuperTeamText(request.Name, core.MaxSuperTeamNameRunes, false)
	description, descriptionValid := core.NormalizeSuperTeamText(
		request.Description, core.MaxSuperTeamDescription, true)
	if !prefixValid || !nameValid || !descriptionValid {
		return apiError(c, fiber.ErrBadRequest)
	}
	team := &core.SuperTeam{
		Prefix: prefix, Name: name, Description: description, CreatedAt: time.Now().UnixMilli(),
	}
	if !auth.CurrentCredentialHasScopeTarget(c, core.APITokenScopeTeamManage, "global/"+prefix) {
		c.Set("X-Renop-Required-Scope", core.APITokenScopeTeamManage)
		return apiError(c, core.ErrSuperTeamPermissionDenied)
	}
	limits := teamConfig(state)
	if err := state.GetDB().CreateSuperTeam(team, user.Username, limits.CreateLimit, limits.JoinLimit); err != nil {
		return apiError(c, err)
	}
	auditAction(c, state, audit.ActionSuperTeamCreate, "Prefix: "+team.Prefix)
	c.Set(fiber.HeaderLocation, "/api/super-teams/"+team.Prefix)
	return c.Status(fiber.StatusCreated).JSON(team)
}

func getTeam(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	details, err := state.GetDB().GetSuperTeamDetails(c.Params("prefix"), user.Username, user.IsManager())
	if err != nil {
		return apiError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(details)
}

func updateTeam(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	if len(c.Body()) > maxRequestBytes {
		return c.SendStatus(fiber.StatusRequestEntityTooLarge)
	}
	var request updateRequest
	if err := c.Bind().Body(&request); err != nil {
		return apiError(c, fiber.ErrBadRequest)
	}
	name, nameValid := core.NormalizeSuperTeamText(request.Name, core.MaxSuperTeamNameRunes, false)
	description, descriptionValid := core.NormalizeSuperTeamText(
		request.Description, core.MaxSuperTeamDescription, true)
	if !nameValid || !descriptionValid {
		return apiError(c, fiber.ErrBadRequest)
	}
	prefix := c.Params("prefix")
	if err := state.GetDB().UpdateSuperTeam(prefix, user.Username, name, description,
		user.IsManager(), time.Now().UnixMilli()); err != nil {
		return apiError(c, err)
	}
	auditAction(c, state, audit.ActionSuperTeamUpdate, "Prefix: "+strings.ToLower(prefix))
	return getTeam(c, state)
}

func deleteTeam(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	prefix := c.Params("prefix")
	if err := state.GetDB().DeleteSuperTeam(prefix, user.Username, user.IsManager(), time.Now().UnixMilli()); err != nil {
		return apiError(c, err)
	}
	auditAction(c, state, audit.ActionSuperTeamDelete, "Prefix: "+strings.ToLower(prefix))
	return c.SendStatus(fiber.StatusNoContent)
}

func addMembers(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	if len(c.Body()) > maxRequestBytes {
		return c.SendStatus(fiber.StatusRequestEntityTooLarge)
	}
	var request membersRequest
	if err := c.Bind().Body(&request); err != nil || len(request.Users) == 0 ||
		len(request.Users) > maxInvitationRecipients || request.Level < core.SuperTeamRoleRead ||
		request.Level > core.SuperTeamRoleOwner {
		return apiError(c, fiber.ErrBadRequest)
	}
	prefix, valid := core.NormalizeSuperTeamPrefix(c.Params("prefix"))
	if !valid {
		return apiError(c, core.ErrSuperTeamNotFound)
	}
	limits := teamConfig(state)
	if user.IsManager() {
		if err := state.GetDB().ForceAddSuperTeamMembers(prefix, user.Username, request.Users, request.Level,
			limits.CreateLimit, limits.JoinLimit, time.Now().UnixMilli()); err != nil {
			return apiError(c, err)
		}
		auditAction(c, state, audit.ActionSuperTeamMemberAdd,
			fmt.Sprintf("Prefix: %s, members: %d, role: T%d", prefix, len(request.Users), request.Level))
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"added": len(request.Users)})
	}

	details, err := state.GetDB().GetSuperTeamDetails(prefix, user.Username, false)
	if err != nil || details.Team.RoleLevel < core.SuperTeamRoleManage ||
		request.Level >= core.SuperTeamRoleManage && details.Team.RoleLevel < core.SuperTeamRoleOwner {
		return apiError(c, core.ErrSuperTeamPermissionDenied)
	}
	now := time.Now().UnixMilli()
	seen := make(map[string]struct{}, len(request.Users))
	invitations := make([]*core.SuperTeamInvitation, 0, len(request.Users))
	messages := make([]*core.UserMessage, 0, len(request.Users))
	for _, candidate := range request.Users {
		recipient := strings.ToLower(strings.TrimSpace(candidate))
		if recipient == "" || recipient == strings.ToLower(user.Username) || len(recipient) > 255 {
			return apiError(c, fiber.ErrBadRequest)
		}
		if _, duplicate := seen[recipient]; duplicate {
			continue
		}
		seen[recipient] = struct{}{}
		if state.GetTokenByName(recipient) == nil {
			return apiError(c, core.ErrUserProfileNotFound)
		}
		id := uuid.NewString()
		payload, marshalErr := json.Marshal(map[string]any{
			"prefix": prefix, "inviter": user.Username, "level": request.Level,
		})
		if marshalErr != nil {
			return apiError(c, marshalErr)
		}
		expiresAt := now + invitationLifetime.Milliseconds()
		invitations = append(invitations, &core.SuperTeamInvitation{
			ID: id, TeamPrefix: prefix, Inviter: user.Username, Recipient: recipient,
			Level: request.Level, CreatedAt: now, ExpiresAt: expiresAt,
		})
		messages = append(messages, &core.UserMessage{
			ID: id, Recipient: recipient, Sender: user.Username, Kind: "super_team_invite", Severity: "info",
			Title:   "Global team invitation",
			Body:    fmt.Sprintf("You were invited to global team %s with T%d permission.", prefix, request.Level),
			Payload: payload, ActionKind: "super_team_invite", ActionStatus: core.MessageActionPending,
			CreatedAt: now, ExpiresAt: expiresAt, DedupeKey: "super-team:" + prefix + ":" + recipient,
		})
	}
	if len(invitations) == 0 {
		return apiError(c, fiber.ErrBadRequest)
	}
	if err := state.GetDB().CreateSuperTeamInvitations(invitations, messages); err != nil {
		return apiError(c, err)
	}
	auditAction(c, state, audit.ActionSuperTeamInvite,
		fmt.Sprintf("Prefix: %s, recipients: %d, role: T%d", prefix, len(invitations), request.Level))
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"invited": len(invitations)})
}

func setMemberLevel(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	var request levelRequest
	if len(c.Body()) > maxRequestBytes || c.Bind().Body(&request) != nil ||
		request.Level < core.SuperTeamRoleRead || request.Level > core.SuperTeamRoleOwner {
		return apiError(c, fiber.ErrBadRequest)
	}
	prefix := c.Params("prefix")
	target := strings.ToLower(strings.TrimSpace(c.Params("username")))
	if target == "" || len(target) > 255 {
		return apiError(c, fiber.ErrBadRequest)
	}
	if err := state.GetDB().SetSuperTeamMemberLevel(prefix, user.Username, target, request.Level, user.IsManager()); err != nil {
		return apiError(c, err)
	}
	auditAction(c, state, audit.ActionSuperTeamMemberLevel,
		fmt.Sprintf("Prefix: %s, member: %s, role: T%d", strings.ToLower(prefix), target, request.Level))
	return c.JSON(fiber.Map{"ok": true})
}

func removeMember(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	prefix := c.Params("prefix")
	target := strings.ToLower(strings.TrimSpace(c.Params("username")))
	if target == "" || len(target) > 255 {
		return apiError(c, fiber.ErrBadRequest)
	}
	if err := state.GetDB().RemoveSuperTeamMember(prefix, user.Username, target, user.IsManager(),
		time.Now().UnixMilli()); err != nil {
		return apiError(c, err)
	}
	auditAction(c, state, audit.ActionSuperTeamMemberRemove,
		"Prefix: "+strings.ToLower(prefix)+", member: "+target)
	return c.SendStatus(fiber.StatusNoContent)
}

func respondInvitation(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	id := strings.TrimSpace(c.Params("id"))
	decision := strings.ToLower(strings.TrimSpace(c.Params("decision")))
	if uuid.Validate(id) != nil || decision != "accept" && decision != "reject" {
		return apiError(c, fiber.ErrBadRequest)
	}
	if err := state.GetDB().RespondSuperTeamInvitation(id, user.Username, decision == "accept",
		teamConfig(state).JoinLimit, time.Now().UnixMilli()); err != nil {
		return apiError(c, err)
	}
	auditAction(c, state, audit.ActionSuperTeamInvitation, "Invitation: "+id+", decision: "+decision)
	return c.JSON(fiber.Map{"ok": true})
}

func getOwnLimits(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	limits := teamConfig(state)
	status, err := state.GetDB().GetSuperTeamLimitStatus(user.Username, limits.CreateLimit, limits.JoinLimit)
	if err != nil {
		return apiError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(status)
}

func getUserLimits(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	if !user.IsManager() {
		return apiError(c, core.ErrSuperTeamPermissionDenied)
	}
	limits := teamConfig(state)
	status, err := state.GetDB().GetSuperTeamLimitStatus(c.Params("username"), limits.CreateLimit, limits.JoinLimit)
	if err != nil {
		return apiError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(status)
}

func putUserLimits(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	if !user.IsManager() {
		return apiError(c, core.ErrSuperTeamPermissionDenied)
	}
	var request limitOverrideRequest
	if len(c.Body()) > 2048 || c.Bind().Body(&request) != nil ||
		request.CreateLimit == nil || request.JoinLimit == nil ||
		*request.CreateLimit < -1 || *request.JoinLimit < -1 ||
		*request.CreateLimit > maxLimitOverride || *request.JoinLimit > maxLimitOverride {
		return apiError(c, fiber.ErrBadRequest)
	}
	var createLimit, joinLimit *int
	if *request.CreateLimit >= 0 {
		createLimit = request.CreateLimit
	}
	if *request.JoinLimit >= 0 {
		joinLimit = request.JoinLimit
	}
	target := strings.ToLower(strings.TrimSpace(c.Params("username")))
	if err := state.GetDB().SetSuperTeamLimitOverride(target, createLimit, joinLimit, time.Now().UnixMilli()); err != nil {
		return apiError(c, err)
	}
	auditAction(c, state, audit.ActionSuperTeamLimit, "Account: "+target)
	return getUserLimits(c, state)
}

func searchUsers(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	details, err := state.GetDB().GetSuperTeamDetails(c.Params("prefix"), user.Username, user.IsManager())
	if err != nil || !user.IsManager() && details.Team.RoleLevel < core.SuperTeamRoleManage {
		return apiError(c, core.ErrSuperTeamPermissionDenied)
	}
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	if query == "" {
		return c.JSON(fiber.Map{"users": []string{}})
	}
	if len(query) > 255 {
		return apiError(c, fiber.ErrBadRequest)
	}
	users, err := state.GetDB().SearchTokenNames(query, maxUserSuggestions, time.Now().UnixMilli())
	if err != nil {
		return apiError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"users": users})
}
