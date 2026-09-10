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
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/legal"
	"renop/internal/utils"
)

func issueBrowserSession(c fiber.Ctx, state *core.AppState, user *config.User, method string) error {
	if user == nil {
		return core.ErrUserProfileNotFound
	}
	if err := legal.RequireConsent(c, state); err != nil {
		return err
	}
	accessToken := state.GetTokenByName(user.Username)
	if accessToken == nil {
		return core.ErrUserProfileNotFound
	}
	if err := accountAccessError(accessToken); err != nil {
		return err
	}
	snapshot, err := prepareBrowserLogin(c, state, user.Username, method, user.AuthenticationSnapshot)
	if err != nil {
		return err
	}
	sessionToken := uuid.NewString()
	publicID := uuid.NewString()
	now := time.Now().UnixMilli()
	cfg := state.Inner.Config.Load()
	ip := utils.ExtractIP(c, &cfg.Server)
	credentialID, _ := c.Locals("verified_fido_credential").([]byte)
	session := &core.Session{
		AuthenticationSnapshot: snapshot,
		FidoCredentialID:       credentialID,
		PublicID:               publicID,
		Username:               user.Username,
		IP:                     utils.Intern(ip),
		UserAgent:              utils.Intern(c.Get(fiber.HeaderUserAgent, "Unknown")),
		CreatedAt:              now,
		LoginMethod:            method,
	}
	session.LastActive.Store(now)
	if err := state.SaveSession(session, sessionToken); err != nil {
		return err
	}
	setSessionCookie(c, sessionToken, int(core.SessionIdleTimeoutMillis/1000))
	authMethod := "Password"
	primary, factor, _ := strings.Cut(method, "+")
	switch primary {
	case "fido":
		authMethod = "FIDO"
	case "github":
		authMethod = "GitHub"
	}
	if strings.HasPrefix(primary, "oauth:") {
		authMethod = "OAuth " + strings.TrimPrefix(primary, "oauth:")
	}
	if factor == "totp" {
		authMethod += " + TOTP"
	}
	if factor == "passkey" {
		authMethod += " + Passkey"
	}
	audit.Log(state, &core.AuditLogEntry{
		Username: user.Username, Operator: user.Username, Action: audit.ActionLogin,
		Details: "User logged in successfully; accepted policy " + legal.AcceptedRevision(c), AuthMethod: authMethod, SessionID: publicID, IP: ip,
	})
	return nil
}
