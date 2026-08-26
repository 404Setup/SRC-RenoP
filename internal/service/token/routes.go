/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package token manages repository access tokens and permissions.
package token

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func SetupTokenRoutes(app fiber.Router, state *core.AppState, opChan chan<- TokenOp) {
	api := app.Group("/tokens")
	api.Get("", func(c fiber.Ctx) error { return FindAllTokens(c, state) })
	api.Get("/:name", func(c fiber.Ctx) error { return FindToken(c, state) })
	api.Put("/:name", func(c fiber.Ctx) error { return UpsertToken(c, state, opChan) })
	api.Delete("/:name", func(c fiber.Ctx) error { return DeleteToken(c, state, opChan) })
	api.Get("/:name/sessions", func(c fiber.Ctx) error { return ListUserSessions(c, state) })
	api.Post("/:name/sessions/revoke-all", func(c fiber.Ctx) error { return RevokeAllUserSessions(c, state) })
	api.Delete("/:name/sessions/:session_id", func(c fiber.Ctx) error { return DeleteUserSession(c, state) })
}

func getUserFromCtx(c fiber.Ctx) *config.User {
	if val := c.Locals("user"); val != nil {
		if u, ok := val.(*config.User); ok {
			return u
		}
	}
	return nil
}

func currentSessionToken(c fiber.Ctx) string {
	if id, ok := c.Locals("current_session_id").(string); ok {
		return id
	}
	return ""
}

func RequireManager(user *config.User) bool {
	if user == nil {
		return false
	}
	for _, r := range user.Roles {
		if r == "manager" || r == "admin" {
			return true
		}
	}
	return false
}

func DefaultCreatedAt() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func FindAllTokens(c fiber.Ctx, state *core.AppState) error {
	user := getUserFromCtx(c)
	if !RequireManager(user) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	rawTokens := state.GetAllTokens()
	tokens := make([]core.AccessTokenDto, 0, len(rawTokens))
	apiTokenCounts, err := state.GetDB().CountAPITokensByUsername()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to count API tokens")
	}

	for _, token := range rawTokens {
		if token == nil {
			continue
		}
		tokens = append(tokens, core.AccessTokenDto{
			Identifier:  token.Identifier,
			Name:        token.Name,
			CreatedAt:   token.CreatedAt,
			Description: token.Description,
			ExpiresAt:   token.ExpiresAt,
			Tokens:      make([]string, apiTokenCounts[token.Name]),
			Permissions: token.Permissions,
		})
	}

	return protohttp.Write(c, pb.FromAccessTokenList(tokens))
}

func FindToken(c fiber.Ctx, state *core.AppState) error {
	name := strings.ToLower(c.Params("name"))
	user := getUserFromCtx(c)
	if !RequireManager(user) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	if token := state.GetTokenByName(name); token != nil {
		apiTokenCount, err := state.GetDB().CountAPITokens(name)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to count API tokens")
		}
		dto := core.AccessTokenDto{
			Identifier:  token.Identifier,
			Name:        name,
			CreatedAt:   token.CreatedAt,
			Description: token.Description,
			ExpiresAt:   token.ExpiresAt,
			Tokens:      make([]string, apiTokenCount),
			Permissions: token.Permissions,
		}
		return protohttp.Write(c, pb.FromAccessTokenDto(dto))
	}

	return c.Status(fiber.StatusNotFound).SendString("Not found")
}

func UpsertToken(c fiber.Ctx, state *core.AppState, opChan chan<- TokenOp) error {
	name := strings.Clone(strings.ToLower(c.Params("name")))
	user := getUserFromCtx(c)
	if !RequireManager(user) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	var reqMsg pb.CreateAccessTokenRequest
	var createReq core.CreateAccessTokenRequest
	readErr := protohttp.Read(c, &reqMsg)
	if readErr == fiber.ErrRequestEntityTooLarge {
		return readErr
	}
	if readErr == nil && (len(reqMsg.Permissions) > 0 || reqMsg.NewName != nil || reqMsg.Secret != nil || reqMsg.Nickname != nil || reqMsg.IsCreate) {
		createReq.Permissions = reqMsg.Permissions
		createReq.NewName = reqMsg.NewName
		createReq.Secret = reqMsg.Secret
		createReq.IsCreate = reqMsg.IsCreate
		createReq.Nickname = reqMsg.Nickname
	} else {
		if err := c.Bind().JSON(&createReq); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Bad request")
		}
	}

	origToken := state.GetTokenByName(name)
	isExisting := origToken != nil
	isNew := !isExisting

	if createReq.IsCreate && isExisting {
		return c.Status(fiber.StatusConflict).SendString("Token name already exists")
	}

	targetName := name
	if createReq.NewName != nil && *createReq.NewName != "" {
		targetName = strings.Clone(strings.ToLower(*createReq.NewName))
	}
	if isNew || targetName != name {
		normalizedName, valid := core.NormalizeUsername(targetName)
		if !valid {
			return c.Status(fiber.StatusBadRequest).SendString("Username must contain 4 to 18 letters, numbers, or underscores")
		}
		targetName = normalizedName
		if state.GetTokenByName(targetName) != nil {
			return c.Status(fiber.StatusConflict).SendString("Token name already exists")
		}
	}

	var generatedSecret string
	if isNew && (createReq.Secret == nil || *createReq.Secret == "") {
		generatedSecret = uuid.NewString()
	} else if createReq.Secret != nil {
		generatedSecret = strings.Clone(*createReq.Secret)
	}

	var hashedSecret string
	if generatedSecret != "" {
		hashBytes, err := bcrypt.GenerateFromPassword([]byte(generatedSecret), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
		}
		hashedSecret = string(hashBytes)
	}

	returnedSecret := generatedSecret

	var token *core.AccessToken
	if isNew {
		token = &core.AccessToken{
			Identifier: core.AccessTokenIdentifier{
				Type:  core.Persistent,
				Value: int32(state.Inner.TokensCount.Load() + 1),
			},
			Name:            "",
			EncryptedSecret: "",
			PasswordHash:    "",
			Tokens:          []string{},
			CreatedAt:       strings.Clone(DefaultCreatedAt()),
			Description:     "Generated by API",
			ExpiresAt:       nil,
			Permissions:     []string{},
		}
	} else {
		if origToken == nil {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		tCopy := *origToken
		token = &tCopy
	}

	if hashedSecret != "" {
		token.EncryptedSecret = strings.Clone(hashedSecret)
	}

	token.Name = strings.Clone(targetName)
	if createReq.Permissions != nil {
		clonedPerms := make([]string, len(createReq.Permissions))
		for i, p := range createReq.Permissions {
			clonedPerms[i] = strings.Clone(p)
		}
		token.Permissions = clonedPerms
	}
	nickname := ""
	if isExisting {
		db := state.GetDB()
		if db == nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
		}
		profile, err := db.GetUserProfile(name)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load user profile")
		}
		nickname = profile.Nickname
	}
	if createReq.Nickname != nil {
		var valid bool
		nickname, valid = core.NormalizeNickname(*createReq.Nickname)
		if !valid {
			return c.Status(fiber.StatusBadRequest).SendString("Nickname must not exceed 36 characters or contain control characters")
		}
	}

	dto := core.AccessTokenDto{
		Identifier:  token.Identifier,
		Name:        strings.Clone(token.Name),
		CreatedAt:   strings.Clone(token.CreatedAt),
		Description: strings.Clone(token.Description),
		ExpiresAt:   token.ExpiresAt,
		Tokens:      token.Tokens,
		Permissions: token.Permissions,
	}

	errChan := make(chan error, 1)
	if !isNew {
		opChan <- TokenOp{
			Type: OpUserProfileUpdate, Name: strings.Clone(name), NewName: strings.Clone(targetName),
			Token: token, Nickname: nickname,
			AccountChanges: core.AccountTokenChanges{
				Password: hashedSecret != "", Permissions: createReq.Permissions != nil,
			},
			ChangedAt: time.Now().UnixMilli(),
			ErrChan:   errChan,
		}
	} else {
		opChan <- TokenOp{
			Type: OpTokenStore, Name: strings.Clone(targetName), Token: token,
			Nickname: nickname, ChangedAt: time.Now().UnixMilli(), ErrChan: errChan,
		}
	}
	if err := <-errChan; err != nil {
		switch {
		case errors.Is(err, core.ErrUsernameAlreadyExists):
			return c.Status(fiber.StatusConflict).SendString("Username is already in use")
		case errors.Is(err, core.ErrUsernameChangeRateLimited):
			return c.Status(fiber.StatusTooManyRequests).SendString("Username can only be changed twice per 24-hour window")
		default:
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to save token")
		}
	}

	_, op, authMethod, sID, ip := audit.ExtractAuthDetails(c, state)
	actionDesc := "Updated user permissions / details"
	if isNew {
		actionDesc = "Created new user account"
	}
	audit.Log(state, &core.AuditLogEntry{
		Username:   targetName,
		Operator:   op,
		Action:     audit.ActionUserPermissionUpdate,
		Details:    actionDesc,
		AuthMethod: authMethod,
		SessionID:  sID,
		IP:         ip,
	})

	res := core.CreateAccessTokenResponse{
		AccessToken: dto,
		Secret:      returnedSecret,
	}

	return protohttp.Write(c, pb.FromCreateAccessTokenResponse(res))
}

func DeleteToken(c fiber.Ctx, state *core.AppState, opChan chan<- TokenOp) error {
	name := strings.Clone(strings.ToLower(c.Params("name")))
	user := getUserFromCtx(c)
	if !RequireManager(user) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	if strings.EqualFold(user.Username, name) {
		return c.Status(fiber.StatusForbidden).SendString("Cannot delete current account")
	}

	if state.GetTokenByName(name) == nil {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	errChan := make(chan error, 1)
	opChan <- TokenOp{
		Type:    OpTokenDelete,
		Name:    strings.Clone(name),
		ErrChan: errChan,
	}
	if err := <-errChan; err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete token")
	}

	_, op, authMethod, sID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username:   name,
		Operator:   op,
		Action:     audit.ActionUserPermissionUpdate,
		Details:    "Deleted user account (" + name + ")",
		AuthMethod: authMethod,
		SessionID:  sID,
		IP:         ip,
	})

	return c.SendStatus(fiber.StatusNoContent)
}

// ListUserSessions lists browser sessions for any account (manager only).
func ListUserSessions(c fiber.Ctx, state *core.AppState) error {
	user := getUserFromCtx(c)
	if !RequireManager(user) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	name := strings.ToLower(c.Params("name"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	if state.GetTokenByName(name) == nil {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	sessions := state.ListUserSessions(name, currentSessionToken(c))
	return protohttp.Write(c, pb.FromSessionList(sessions))
}

// DeleteUserSession revokes one browser session of any account by public_id (manager only).
func DeleteUserSession(c fiber.Ctx, state *core.AppState) error {
	user := getUserFromCtx(c)
	if !RequireManager(user) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	name := strings.ToLower(c.Params("name"))
	sessionID := c.Params("session_id")
	if name == "" || sessionID == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}

	// Allow revoke even if the account was removed; session may still exist.
	if _, _, err := state.RevokeUserSessionByPublicID(name, sessionID, currentSessionToken(c)); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to revoke session")
	}

	_, op, authMethod, sID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username:   name,
		Operator:   op,
		Action:     audit.ActionSessionRevoke,
		Details:    "Session (" + sessionID + ") revoked by manager",
		AuthMethod: authMethod,
		SessionID:  sID,
		IP:         ip,
	})

	return protohttp.Write(c, pb.StatusOkSuccess())
}

// RevokeAllUserSessions revokes every browser session for an account (manager only).
func RevokeAllUserSessions(c fiber.Ctx, state *core.AppState) error {
	user := getUserFromCtx(c)
	if !RequireManager(user) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	name := strings.ToLower(c.Params("name"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}

	keep := ""
	if user != nil && strings.EqualFold(user.Username, name) {
		keep = currentSessionToken(c)
	}
	if _, err := state.RevokeOtherUserSessions(name, keep); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to revoke sessions")
	}

	_, op, authMethod, sID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username:   name,
		Operator:   op,
		Action:     audit.ActionSessionRevoke,
		Details:    "All sessions revoked by manager",
		AuthMethod: authMethod,
		SessionID:  sID,
		IP:         ip,
	})

	return protohttp.Write(c, pb.StatusOkSuccess())
}
