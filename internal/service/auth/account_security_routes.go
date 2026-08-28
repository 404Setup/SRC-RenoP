/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/utils"
)

type privateEmailRequest struct {
	Email string `json:"email"`
}

type passwordLoginRequest struct {
	Enabled bool `json:"enabled"`
}

type passwordRecoveryRequest struct {
	Identifier  string   `json:"identifier"`
	Codes       []string `json:"codes"`
	NewPassword string   `json:"new_password"`
}

func setupAccountSecurityRoutes(auth fiber.Router, state *core.AppState) {
	auth.Get("/profile/security", func(c fiber.Ctx) error { return getAccountSecurity(c, state) })
	auth.Put("/profile/email", func(c fiber.Ctx) error { return putPrivateEmail(c, state) })
	auth.Put("/profile/password-login", func(c fiber.Ctx) error { return putPasswordLogin(c, state) })
	auth.Post("/profile/recovery-codes", func(c fiber.Ctx) error { return postRecoveryCodes(c, state) })
	auth.Post("/recovery/password", func(c fiber.Ctx) error { return postPasswordRecovery(c, state) })
}

func requireAccountSession(c fiber.Ctx) (*config.User, error) {
	user := GetUser(c)
	if user == nil || user.Username == "" || user.Username == "guest" {
		return nil, fiber.ErrUnauthorized
	}
	currentSession, ok := c.Locals("current_session_id").(string)
	if !ok || strings.TrimSpace(currentSession) == "" {
		return nil, fiber.ErrForbidden
	}
	return user, nil
}

func setPrivateResponseHeaders(c fiber.Ctx) {
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("Pragma", "no-cache")
}

func accountSessionError(c fiber.Ctx, err error) error {
	if errors.Is(err, fiber.ErrUnauthorized) {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	return c.Status(fiber.StatusForbidden).SendString("A browser session is required")
}

func getAccountSecurity(c fiber.Ctx, state *core.AppState) error {
	user, err := requireAccountSession(c)
	if err != nil {
		return accountSessionError(c, err)
	}
	security, err := state.GetDB().GetAccountSecurity(user.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load account security")
	}
	setPrivateResponseHeaders(c)
	return c.JSON(security)
}

func putPrivateEmail(c fiber.Ctx, state *core.AppState) error {
	user, err := requireAccountSession(c)
	if err != nil {
		return accountSessionError(c, err)
	}
	var request privateEmailRequest
	if err := utils.ReadJSONLimited(c, &request, utils.MaxJSONBodySize); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid email request")
	}
	email, valid := core.NormalizeEmail(request.Email)
	if !valid || email == "" {
		c.Set("X-Renop-Error-Code", "ACCOUNT_EMAIL_INVALID")
		return c.Status(fiber.StatusBadRequest).SendString("Email address is invalid")
	}
	security, err := state.GetDB().UpdateAccountEmail(user.Username, email, time.Now().UnixMilli())
	if errors.Is(err, core.ErrEmailAlreadyExists) {
		c.Set("X-Renop-Error-Code", "ACCOUNT_EMAIL_CONFLICT")
		return c.Status(fiber.StatusConflict).SendString("Email address is already in use")
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update private email")
	}
	state.InvalidateAccountAuthCache(true, user.Username)
	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionProfileUpdate,
		Details: "Updated private login email", AuthMethod: authMethod, SessionID: sessionID, IP: ip,
	})
	setPrivateResponseHeaders(c)
	return c.JSON(security)
}

func putPasswordLogin(c fiber.Ctx, state *core.AppState) error {
	user, err := requireAccountSession(c)
	if err != nil {
		return accountSessionError(c, err)
	}
	var request passwordLoginRequest
	if err := utils.ReadJSONLimited(c, &request, utils.MaxJSONBodySize); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid password-login request")
	}
	security, err := state.GetDB().SetPasswordLoginEnabled(user.Username, request.Enabled, time.Now().UnixMilli())
	switch {
	case errors.Is(err, core.ErrLastLoginMethod):
		c.Set("X-Renop-Error-Code", "ACCOUNT_LAST_LOGIN_METHOD")
		return c.Status(fiber.StatusConflict).SendString("Another login method is required")
	case errors.Is(err, core.ErrPasswordNotConfigured):
		c.Set("X-Renop-Error-Code", "ACCOUNT_PASSWORD_NOT_CONFIGURED")
		return c.Status(fiber.StatusConflict).SendString("Set a password before enabling password login")
	case err != nil:
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update password login")
	}
	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionPasswordUpdate,
		Details: "Updated password-login policy", AuthMethod: authMethod, SessionID: sessionID, IP: ip,
	})
	state.InvalidateAccountAuthCache(request.Enabled, user.Username)
	setPrivateResponseHeaders(c)
	return c.JSON(security)
}

func postRecoveryCodes(c fiber.Ctx, state *core.AppState) error {
	user, err := requireAccountSession(c)
	if err != nil {
		return accountSessionError(c, err)
	}
	displayCodes, hashes, err := generateRecoveryCodeSet()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate recovery codes")
	}
	if err := state.GetDB().ReplaceRecoveryCodes(user.Username, hashes); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to store recovery codes")
	}
	security, err := state.GetDB().GetAccountSecurity(user.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to reload account security")
	}
	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionProfileUpdate,
		Details: "Generated a new recovery-code set", AuthMethod: authMethod, SessionID: sessionID, IP: ip,
	})
	setPrivateResponseHeaders(c)
	return c.JSON(fiber.Map{"codes": displayCodes, "security": security})
}

func recoveryVerification(state *core.AppState, identifier string,
	codes []string) (string, []string, bool, error) {
	parsed := make([]parsedRecoveryCode, core.RecoveryCodesRequired)
	selectors := make([]string, core.RecoveryCodesRequired)
	validInput := len(codes) == core.RecoveryCodesRequired
	seen := make(map[string]struct{}, core.RecoveryCodesRequired)
	for index := range parsed {
		code := ""
		if index < len(codes) {
			code = codes[index]
		}
		parsed[index] = parseRecoveryCode(code)
		selectors[index] = parsed[index].selectorHash
		if _, duplicate := seen[selectors[index]]; duplicate {
			validInput = false
		}
		seen[selectors[index]] = struct{}{}
		if !parsed[index].valid {
			validInput = false
		}
	}

	username, records, lookupErr := state.GetDB().GetRecoveryCodes(identifier, selectors)
	recordBySelector := make(map[string]string, len(records))
	for _, record := range records {
		recordBySelector[record.SelectorHash] = record.PasswordHash
	}
	valid := validInput && lookupErr == nil && len(records) == core.RecoveryCodesRequired
	for _, code := range parsed {
		encoded, exists := recordBySelector[code.selectorHash]
		if !code.valid || !exists {
			consumeDummyRecoveryWork(code.value)
			valid = false
			continue
		}
		if !verifyRecoveryCode(code.value, encoded) {
			valid = false
		}
	}
	if lookupErr != nil && !errors.Is(lookupErr, core.ErrRecoveryCodesInvalid) {
		return "", selectors, false, lookupErr
	}
	return username, selectors, valid, nil
}

func purgeRecoveredSessions(state *core.AppState, username string) {
	state.Inner.Sessions.Range(func(sessionToken string, session *core.Session) bool {
		if session != nil && strings.EqualFold(session.Username, username) {
			state.DeleteAuthCache("Session " + sessionToken)
			state.Inner.Sessions.Delete(sessionToken)
		}
		return true
	})
	state.InvalidateAccountAuthCache(true, username)
}

func postPasswordRecovery(c fiber.Ctx, state *core.AppState) error {
	var request passwordRecoveryRequest
	if err := utils.ReadJSONLimited(c, &request, utils.MaxJSONBodySize); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid password-recovery request")
	}
	request.Identifier = strings.TrimSpace(request.Identifier)
	if request.Identifier == "" || len(request.Identifier) > core.MaxEmailLength ||
		len(request.NewPassword) < 6 || len(request.NewPassword) > 72 ||
		len(request.Codes) != core.RecoveryCodesRequired {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid password-recovery request")
	}
	_, selectors, valid, err := recoveryVerification(state, request.Identifier, request.Codes)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Password recovery is unavailable")
	}
	if !valid {
		return c.Status(fiber.StatusUnauthorized).SendString("Invalid recovery credentials")
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(request.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Password recovery is unavailable")
	}
	state.Inner.TokenWriteLock.Lock()
	username, err := state.GetDB().ResetPasswordWithRecoveryCodes(
		request.Identifier, selectors, string(passwordHash), time.Now().UnixMilli())
	if err == nil {
		purgeRecoveredSessions(state, username)
	}
	state.Inner.TokenWriteLock.Unlock()
	if errors.Is(err, core.ErrRecoveryCodesInvalid) {
		return c.Status(fiber.StatusUnauthorized).SendString("Invalid recovery credentials")
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Password recovery is unavailable")
	}
	cfg := state.Inner.Config.Load()
	ip := utils.ExtractIP(c, &cfg.Server)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: username, Action: audit.ActionPasswordUpdate,
		Details: "Password reset with recovery codes", AuthMethod: "RecoveryCode", IP: ip,
	})
	setSessionCookie(c, "", -1)
	setPrivateResponseHeaders(c)
	return c.JSON(fiber.Map{"status": "success", "username": username})
}
