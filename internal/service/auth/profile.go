/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/token"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

type UpdatePasswordRequest struct {
	NewPassword string `json:"new_password"`
}

type StatusResponse struct {
	Status string `json:"status"`
}

func UpdatePassword(c fiber.Ctx, state *core.AppState, opChan chan<- token.TokenOp) error {
	userInt := c.Locals("user")
	if userInt == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	user := userInt.(*config.User)

	var req pb.UpdatePasswordRequest
	readErr := protohttp.Read(c, &req)
	if readErr == fiber.ErrRequestEntityTooLarge {
		return readErr
	}
	if readErr != nil || req.NewPassword == "" {
		var jsonReq UpdatePasswordRequest
		if jsonErr := c.Bind().JSON(&jsonReq); jsonErr == nil && jsonReq.NewPassword != "" {
			req.NewPassword = jsonReq.NewPassword
		} else if req.NewPassword == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
		}
	}
	if len(req.NewPassword) < 6 || len(req.NewPassword) > 72 {
		return c.Status(fiber.StatusBadRequest).SendString("Password must be between 6 and 72 bytes")
	}

	hashBytes, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}
	hashed := string(hashBytes)

	state.Inner.TokenWriteLock.Lock()
	err = state.GetDB().SetAccountPassword(user.Username, hashed, time.Now().UnixMilli())
	state.Inner.TokenWriteLock.Unlock()

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update token")
	}
	state.InvalidateAccountAuthCache(true, user.Username)

	_, op, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username:   user.Username,
		Operator:   op,
		Action:     audit.ActionPasswordUpdate,
		Details:    "Account password updated",
		AuthMethod: authMethod,
		SessionID:  sessionID,
		IP:         ip,
	})

	return protohttp.Write(c, pb.StatusOkSuccess())
}

type TokenResponse struct {
	Token string `json:"token"`
}

func GenerateUploadToken(c fiber.Ctx, state *core.AppState, opChan chan<- token.TokenOp) error {
	user, err := requireAccountSession(c)
	if err != nil {
		return accountSessionError(c, err)
	}
	account := state.GetTokenByName(user.Username)
	if account == nil {
		return c.Status(fiber.StatusNotFound).SendString("Account not found")
	}
	name := "Publishing token " + time.Now().UTC().Format("20060102-150405") + "-" + uuid.NewString()[:8]
	_, newToken, err := createAPITokenForAccount(state, account, name, []string{
		APITokenScopeRepositoryRead, APITokenScopeRepositoryPublish,
	}, nil, nil, time.Now().UnixMilli())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update token")
	}

	_, op, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username:   user.Username,
		Operator:   op,
		Action:     audit.ActionTokenGenerate,
		Details:    "New upload token generated",
		AuthMethod: authMethod,
		SessionID:  sessionID,
		IP:         ip,
	})

	setPrivateResponseHeaders(c)
	return protohttp.Write(c, &pb.GenerateTokenResponse{Token: newToken})
}

func ListSessions(c fiber.Ctx, state *core.AppState) error {
	userInt := c.Locals("user")
	if userInt == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	user := userInt.(*config.User)

	sessions := state.ListUserSessions(user.Username, CurrentSessionToken(c))
	return protohttp.Write(c, pb.FromSessionList(sessions))
}

func DeleteSession(c fiber.Ctx, state *core.AppState) error {
	userInt := c.Locals("user")
	if userInt == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	user := userInt.(*config.User)
	sessionID := c.Params("session_id")

	revoked, wasCurrent, err := state.RevokeUserSessionByPublicID(user.Username, sessionID, CurrentSessionToken(c))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to revoke session")
	}
	if wasCurrent && revoked {
		setSessionCookie(c, "", -1)
	}

	_, op, authMethod, sID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username:   user.Username,
		Operator:   op,
		Action:     audit.ActionSessionRevoke,
		Details:    "Revoked browser session (" + sessionID + ")",
		AuthMethod: authMethod,
		SessionID:  sID,
		IP:         ip,
	})

	return protohttp.Write(c, pb.StatusOkSuccess())
}

// RevokeOtherSessions drops every browser session for the current user except this request's session.
func RevokeOtherSessions(c fiber.Ctx, state *core.AppState) error {
	userInt := c.Locals("user")
	if userInt == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	user := userInt.(*config.User)

	if _, err := state.RevokeOtherUserSessions(user.Username, CurrentSessionToken(c)); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to revoke sessions")
	}

	_, op, authMethod, sID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username:   user.Username,
		Operator:   op,
		Action:     audit.ActionSessionRevoke,
		Details:    "Revoked all other browser sessions",
		AuthMethod: authMethod,
		SessionID:  sID,
		IP:         ip,
	})

	return protohttp.Write(c, pb.StatusOkSuccess())
}
