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
	"github.com/3JoB/unsafeConvert"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"renop/audit"
	"renop/config"
	"renop/core"
	"renop/pb"
	"renop/token"
	"renop/utils/protohttp"
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
	if err := protohttp.Read(c, &req); err != nil || req.NewPassword == "" {
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
	hashed := unsafeConvert.StringPointer(hashBytes)

	err = token.UpdateTokenSync(opChan, user.Username, func(accessToken *core.AccessToken) {
		accessToken.EncryptedSecret = hashed
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update token")
	}

	_, op, authMethod, sessionID, ip := audit.ExtractAuthDetails(c)
	audit.Log(state, &core.AuditLogEntry{
		Username:   user.Username,
		Operator:   op,
		Action:     "PASSWORD_UPDATE",
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
	userInt := c.Locals("user")
	if userInt == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	user := userInt.(*config.User)

	newToken := uuid.NewString()

	err := token.UpdateTokenSync(opChan, user.Username, func(accessToken *core.AccessToken) {
		accessToken.Tokens = []string{newToken}
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update token")
	}

	_, op, authMethod, sessionID, ip := audit.ExtractAuthDetails(c)
	audit.Log(state, &core.AuditLogEntry{
		Username:   user.Username,
		Operator:   op,
		Action:     "TOKEN_GENERATE",
		Details:    "New upload token generated",
		AuthMethod: authMethod,
		SessionID:  sessionID,
		IP:         ip,
	})

	return protohttp.Write(c, &pb.GenerateTokenResponse{Token: newToken})
}

func currentSessionToken(c fiber.Ctx) string {
	if id, ok := c.Locals("current_session_id").(string); ok {
		return id
	}
	return ""
}

func ListSessions(c fiber.Ctx, state *core.AppState) error {
	userInt := c.Locals("user")
	if userInt == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	user := userInt.(*config.User)

	sessions := state.ListUserSessions(user.Username, currentSessionToken(c))
	return protohttp.Write(c, pb.FromSessionList(sessions))
}

func DeleteSession(c fiber.Ctx, state *core.AppState) error {
	userInt := c.Locals("user")
	if userInt == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	user := userInt.(*config.User)
	sessionID := c.Params("session_id")

	revoked, wasCurrent := state.RevokeUserSessionByPublicID(user.Username, sessionID, currentSessionToken(c))
	if wasCurrent && revoked {
		setSessionCookie(c, "", -1)
	}

	_, op, authMethod, sID, ip := audit.ExtractAuthDetails(c)
	audit.Log(state, &core.AuditLogEntry{
		Username:   user.Username,
		Operator:   op,
		Action:     "SESSION_REVOKE",
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

	state.RevokeOtherUserSessions(user.Username, currentSessionToken(c))

	_, op, authMethod, sID, ip := audit.ExtractAuthDetails(c)
	audit.Log(state, &core.AuditLogEntry{
		Username:   user.Username,
		Operator:   op,
		Action:     "SESSION_REVOKE",
		Details:    "Revoked all other browser sessions",
		AuthMethod: authMethod,
		SessionID:  sID,
		IP:         ip,
	})

	return protohttp.Write(c, pb.StatusOkSuccess())
}
