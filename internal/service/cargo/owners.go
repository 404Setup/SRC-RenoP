/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
)

const (
	maxOwnerRequestUsers = 20
	invitationLifetime   = 7 * 24 * time.Hour
)

func resolveCargoMemberReference(db core.StateDB, reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if _, err := uuid.Parse(reference); err != nil {
		return reference, nil
	}
	profile, err := db.GetUserProfileByID(reference)
	if err != nil {
		return "", err
	}
	return profile.Username, nil
}

func (h Handler) listOwners(c fiber.Ctx, state *core.AppState, repo *config.Repository, crateName string) error {
	user, err := authenticatedUser(c)
	if err != nil {
		return cargoError(c, err)
	}
	details, err := packageDetails(state, repo.Name, crateName, user.Username)
	if err != nil {
		return cargoError(c, err)
	}
	if !user.IsManager() && details.Package.PermissionLevel == 0 {
		return cargoError(c, core.ErrCargoPermissionDenied)
	}
	owners := make([]owner, 0, len(details.Members))
	for _, member := range details.Members {
		if member == nil {
			continue
		}
		hasher := fnv.New32a()
		identity := member.UserID
		if identity == "" {
			identity = member.Username
		}
		_, _ = hasher.Write([]byte(identity))
		owners = append(owners, owner{ID: hasher.Sum32(), UserID: member.UserID, Login: member.Username, Name: member.Username, Level: member.Level})
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(ownerResponse{Users: owners})
}

func (h Handler) inviteOwners(c fiber.Ctx, state *core.AppState, repo *config.Repository, crateName string) error {
	user, details, err := authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionManage, true)
	if err != nil {
		return cargoError(c, err)
	}
	var request ownerRequest
	if err := decodeJSON(c, &request); err != nil || len(request.Users) == 0 || len(request.Users) > maxOwnerRequestUsers {
		return errorResponse(c, fiber.StatusBadRequest, "Choose between 1 and 20 Cargo package members")
	}
	if request.Level == 0 {
		request.Level = core.CargoPermissionPublish
	}
	if request.Level < core.CargoPermissionPublish || request.Level > core.CargoPermissionOwner {
		return errorResponse(c, fiber.StatusBadRequest, "Cargo permission level must be between 1 and 4")
	}
	db := state.GetDB()
	if db == nil {
		return cargoError(c, core.ErrDatabaseUnavailable)
	}

	isAdmin := user.IsManager()
	if isAdmin {
		validUsers := make([]string, 0, len(request.Users))
		seen := make(map[string]struct{}, len(request.Users))
		for _, candidate := range request.Users {
			recipient := strings.ToLower(strings.TrimSpace(candidate))
			if recipient == "" || len(recipient) > 255 {
				return errorResponse(c, fiber.StatusBadRequest, "Cargo package invitation recipient does not exist")
			}
			token, err := db.GetTokenByName(recipient)
			if err != nil {
				return cargoError(c, err)
			}
			if token == nil {
				return errorResponse(c, fiber.StatusBadRequest, "Cargo package invitation recipient does not exist")
			}
			if _, duplicate := seen[recipient]; duplicate {
				continue
			}
			seen[recipient] = struct{}{}
			validUsers = append(validUsers, recipient)
		}
		if len(validUsers) == 0 {
			return errorResponse(c, fiber.StatusBadRequest, "Cargo package invitation has no recipients")
		}
		if err := db.ForceAddCargoMembers(repo.Name, details.Package.NormalizedName, details.Package.Name, user.Username, validUsers, request.Level); err != nil {
			return cargoError(c, err)
		}
		logCargoAudit(c, state, "CARGO_TEAM_ADD", fmt.Sprintf("Repository: %s, crate: %s, members: %d, level: L%d", repo.Name, details.Package.Name, len(validUsers), request.Level))
		return c.JSON(OperationResponse{OK: true, Message: fmt.Sprintf("Added %d user(s) to crate %s", len(validUsers), details.Package.Name)})
	}

	now := time.Now().UnixMilli()
	seen := make(map[string]struct{}, len(request.Users))
	invitations := make([]*core.CargoInvitation, 0, len(request.Users))
	messages := make([]*core.UserMessage, 0, len(request.Users))
	for _, candidate := range request.Users {
		recipient := strings.ToLower(strings.TrimSpace(candidate))
		if recipient == "" || len(recipient) > 255 {
			return errorResponse(c, fiber.StatusBadRequest, "Cargo package invitation recipient does not exist")
		}
		if recipient == strings.ToLower(user.Username) {
			return errorResponse(c, fiber.StatusBadRequest, "Cannot invite yourself")
		}
		token, err := db.GetTokenByName(recipient)
		if err != nil {
			return cargoError(c, err)
		}
		if token == nil {
			return errorResponse(c, fiber.StatusBadRequest, "Cargo package invitation recipient does not exist")
		}
		if _, duplicate := seen[recipient]; duplicate {
			continue
		}
		seen[recipient] = struct{}{}
		id := uuid.NewString()
		payload, err := json.Marshal(invitationPayload{
			Repository: repo.Name, Package: details.Package.Name,
			Inviter: strings.ToLower(user.Username), Level: request.Level,
		})
		if err != nil {
			return cargoError(c, err)
		}
		invitations = append(invitations, &core.CargoInvitation{
			ID: id, Repository: repo.Name, Package: details.Package.Name,
			NormalizedName: details.Package.NormalizedName, Inviter: user.Username,
			Recipient: recipient, Level: request.Level, CreatedAt: now,
		})
		messages = append(messages, &core.UserMessage{
			ID: id, Recipient: recipient, Sender: strings.ToLower(user.Username),
			Kind: "cargo_package_invite", Severity: "info",
			Title:   "Cargo package invitation",
			Body:    fmt.Sprintf("%s invited you to join %s with L%d permission.", user.Username, details.Package.Name, request.Level),
			Payload: payload, ActionKind: "cargo_package_invite", ActionStatus: core.MessageActionPending,
			CreatedAt: now, ExpiresAt: now + invitationLifetime.Milliseconds(),
		})
	}
	if len(invitations) == 0 {
		return errorResponse(c, fiber.StatusBadRequest, "Cargo package invitation has no recipients")
	}
	if err := db.CreateCargoInvitations(invitations, messages); err != nil {
		return cargoError(c, err)
	}
	logCargoAudit(c, state, "CARGO_TEAM_INVITE", fmt.Sprintf("Repository: %s, crate: %s, recipients: %d, level: L%d", repo.Name, details.Package.Name, len(invitations), request.Level))
	return c.JSON(OperationResponse{OK: true, Message: fmt.Sprintf("Invited %d user(s) to crate %s", len(invitations), details.Package.Name)})
}

func (h Handler) removeOwners(c fiber.Ctx, state *core.AppState, repo *config.Repository, crateName string) error {
	user, details, err := authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionManage, true)
	if err != nil {
		return cargoError(c, err)
	}
	var request ownerRequest
	if err := decodeJSON(c, &request); err != nil || len(request.Users) == 0 || len(request.Users) > maxOwnerRequestUsers {
		return errorResponse(c, fiber.StatusBadRequest, "Choose between 1 and 20 Cargo package members")
	}
	db := state.GetDB()
	if db == nil {
		return cargoError(c, core.ErrDatabaseUnavailable)
	}
	seen := make(map[string]struct{}, len(request.Users))
	usernames := make([]string, 0, len(request.Users))
	for _, candidate := range request.Users {
		username := strings.ToLower(strings.TrimSpace(candidate))
		if username == "" || len(username) > 255 {
			return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo package member")
		}
		if _, duplicate := seen[username]; duplicate {
			continue
		}
		seen[username] = struct{}{}
		usernames = append(usernames, username)
	}
	actor := user.Username
	removesSelf := false
	for _, username := range usernames {
		if strings.EqualFold(username, user.Username) {
			removesSelf = true
			break
		}
	}
	if user.IsManager() && !removesSelf {
		actor = ""
	}
	if err := db.RemoveCargoMembers(repo.Name, details.Package.NormalizedName, actor, usernames); err != nil {
		return cargoError(c, err)
	}
	logCargoAudit(c, state, "CARGO_TEAM_REMOVE", fmt.Sprintf("Repository: %s, crate: %s, members: %d", repo.Name, details.Package.Name, len(usernames)))
	return c.JSON(OperationResponse{OK: true, Message: "Owners successfully removed"})
}

func (h Handler) setOwnerLevel(c fiber.Ctx, state *core.AppState, repo *config.Repository, crateName, username string) error {
	user, details, err := authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionManage, true)
	if err != nil {
		return cargoError(c, err)
	}
	var request memberLevelRequest
	if err := decodeJSON(c, &request); err != nil || request.Level < core.CargoPermissionPublish || request.Level > core.CargoPermissionOwner {
		return errorResponse(c, fiber.StatusBadRequest, "Cargo permission level must be between 1 and 4")
	}
	username, err = resolveCargoMemberReference(state.GetDB(), username)
	if err != nil {
		return errorResponse(c, fiber.StatusNotFound, "Cargo package member was not found")
	}
	actor := user.Username
	if user.IsManager() && !(request.Level == core.CargoPermissionOwner &&
		details.Package.PermissionLevel == core.CargoPermissionOwner && !strings.EqualFold(username, user.Username)) {
		actor = ""
	}
	if err := state.GetDB().SetCargoMemberLevel(repo.Name, details.Package.NormalizedName, actor, username, request.Level); err != nil {
		return cargoError(c, err)
	}
	logCargoAudit(c, state, "CARGO_TEAM_LEVEL", fmt.Sprintf("Repository: %s, crate: %s, member: %s, level: L%d", repo.Name, details.Package.Name, strings.ToLower(username), request.Level))
	return c.JSON(OperationResponse{OK: true})
}

func (h Handler) removeOwner(c fiber.Ctx, state *core.AppState, repo *config.Repository, crateName, username string) error {
	user, err := authenticatedUser(c)
	if err != nil {
		return cargoError(c, err)
	}
	details, err := packageDetails(state, repo.Name, crateName, user.Username)
	if err != nil {
		return cargoError(c, err)
	}
	username, err = resolveCargoMemberReference(state.GetDB(), username)
	if err != nil {
		return errorResponse(c, fiber.StatusNotFound, "Cargo package member was not found")
	}
	isSelf := strings.EqualFold(username, user.Username)
	if !isSelf && !user.IsManager() && details.Package.PermissionLevel < core.CargoPermissionManage {
		return cargoError(c, core.ErrCargoPermissionDenied)
	}
	actor := user.Username
	if user.IsManager() && !isSelf {
		actor = ""
	}
	if err := state.GetDB().RemoveCargoMember(repo.Name, details.Package.NormalizedName, actor, username); err != nil {
		return cargoError(c, err)
	}
	logCargoAudit(c, state, "CARGO_TEAM_REMOVE", fmt.Sprintf("Repository: %s, crate: %s, member: %s", repo.Name, details.Package.Name, strings.ToLower(username)))
	return c.JSON(OperationResponse{OK: true})
}

func (h Handler) searchUsers(c fiber.Ctx, state *core.AppState, repo *config.Repository, crateName string) error {
	_, _, err := authorizePackageMutation(c, state, repo.Name, crateName, core.CargoPermissionManage, true)
	if err != nil {
		return cargoError(c, err)
	}
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	if query == "" {
		return c.JSON(fiber.Map{"users": []string{}})
	}
	if len(query) > 255 || strings.ContainsAny(query, "\x00\r\n") {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo user search")
	}
	users, err := state.GetDB().SearchTokenNames(query, 8, time.Now().UnixMilli())
	if err != nil {
		return cargoError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"users": users})
}

func (h Handler) respondInvitation(c fiber.Ctx, state *core.AppState, repo *config.Repository, invitationID, decision string) error {
	user, err := authenticatedUser(c)
	if err != nil {
		return cargoError(c, err)
	}
	if uuid.Validate(invitationID) != nil || (decision != "accept" && decision != "reject") {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo invitation action")
	}
	db := state.GetDB()
	if db == nil {
		return cargoError(c, core.ErrDatabaseUnavailable)
	}
	if err := db.RespondCargoInvitation(invitationID, user.Username, repo.Name, decision == "accept", time.Now().UnixMilli()); err != nil {
		return cargoError(c, err)
	}
	logCargoAudit(c, state, "CARGO_INVITE_"+strings.ToUpper(decision), "Repository: "+repo.Name+", invitation: "+invitationID)
	return c.JSON(OperationResponse{OK: true})
}

func logCargoAudit(c fiber.Ctx, state *core.AppState, action, details string) {
	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: action, Details: details,
		AuthMethod: authMethod, SessionID: sessionID, IP: ip,
	})
}
