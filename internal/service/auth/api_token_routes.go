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
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/utils"
)

const maxAPITokenLifetime = 5 * 365 * 24 * time.Hour

type createAPITokenRequest struct {
	Name      string              `json:"name"`
	Scopes    []string            `json:"scopes"`
	Targets   map[string][]string `json:"targets"`
	ExpiresAt *int64              `json:"expires_at"`
}

func setupAPITokenRoutes(auth fiber.Router, state *core.AppState) {
	auth.Get("/profile/api-tokens", func(c fiber.Ctx) error { return listProfileAPITokens(c, state) })
	auth.Get("/profile/api-tokens/scopes", func(c fiber.Ctx) error { return listProfileAPITokenScopes(c, state) })
	auth.Post("/profile/api-tokens", func(c fiber.Ctx) error { return createProfileAPIToken(c, state) })
	auth.Delete("/profile/api-tokens/:token_id", func(c fiber.Ctx) error { return deleteProfileAPIToken(c, state) })
}

func validAPITokenName(value string) bool {
	if value == "" || len(value) > core.MaxAPITokenNameLength ||
		strings.HasPrefix(strings.ToLower(value), strings.ToLower(core.LegacyAPITokenNamePrefix)) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func normalizeAPITokenExpiry(expiresAt *int64, now int64) (*int64, error) {
	if expiresAt == nil || *expiresAt == 0 {
		return nil, nil
	}
	minimum := now + int64((5*time.Minute)/time.Millisecond)
	maximum := now + int64(maxAPITokenLifetime/time.Millisecond)
	if *expiresAt < minimum || *expiresAt > maximum {
		return nil, errAPITokenExpiryInvalid
	}
	value := *expiresAt
	return &value, nil
}

func createAPITokenForAccount(state *core.AppState, account *core.AccessToken, name string,
	scopes []string, targets map[string][]string, expiresAt *int64, now int64) (*core.APIToken, string, error) {
	if state == nil || state.GetDB() == nil || account == nil {
		return nil, "", core.ErrDatabaseUnavailable
	}
	name = strings.TrimSpace(name)
	if !validAPITokenName(name) {
		return nil, "", errAPITokenNameInvalid
	}
	normalizedScopes, err := normalizeAPITokenScopes(account, scopes)
	if err != nil {
		return nil, "", err
	}
	normalizedTargets, err := normalizeAPITokenTargets(account, normalizedScopes, targets)
	if err != nil {
		return nil, "", err
	}
	expiresAt, err = normalizeAPITokenExpiry(expiresAt, now)
	if err != nil {
		return nil, "", err
	}
	secret, err := generateAPITokenSecret()
	if err != nil {
		return nil, "", err
	}
	token := &core.APIToken{
		ID: uuid.NewString(), Name: name, Scopes: normalizedScopes, Targets: normalizedTargets,
		CreatedAt: now, ExpiresAt: expiresAt,
	}
	if err := state.GetDB().CreateAPIToken(account.Name, token, apiTokenSecretHash(secret)); err != nil {
		return nil, "", err
	}
	return token, secret, nil
}

func listProfileAPITokens(c fiber.Ctx, state *core.AppState) error {
	user, err := requireAccountSession(c)
	if err != nil {
		return accountSessionError(c, err)
	}
	tokens, err := state.GetDB().ListAPITokens(user.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to list API tokens")
	}
	setPrivateResponseHeaders(c)
	return c.JSON(fiber.Map{"tokens": tokens, "limit": core.MaxAPITokensPerUser})
}

func listProfileAPITokenScopes(c fiber.Ctx, state *core.AppState) error {
	user, err := requireAccountSession(c)
	if err != nil {
		return accountSessionError(c, err)
	}
	account := state.GetTokenByName(user.Username)
	if account == nil {
		return c.Status(fiber.StatusNotFound).SendString("Account not found")
	}
	setPrivateResponseHeaders(c)
	return c.JSON(fiber.Map{
		"scopes": allowedAPITokenScopes(account), "target_kinds": allowedAPITokenTargetKinds(account),
		"target_limit": core.MaxAPITokenTargets,
	})
}

func createProfileAPIToken(c fiber.Ctx, state *core.AppState) error {
	user, err := requireAccountSession(c)
	if err != nil {
		return accountSessionError(c, err)
	}
	var request createAPITokenRequest
	if err := utils.ReadJSONLimited(c, &request, utils.MaxJSONBodySize); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid API token request")
	}
	account := state.GetTokenByName(user.Username)
	if account == nil {
		return c.Status(fiber.StatusNotFound).SendString("Account not found")
	}
	token, secret, err := createAPITokenForAccount(state, account, request.Name,
		request.Scopes, request.Targets, request.ExpiresAt, time.Now().UnixMilli())
	switch {
	case errors.Is(err, core.ErrAPITokenNameExists):
		c.Set("X-Renop-Error-Code", "API_TOKEN_NAME_CONFLICT")
		return c.Status(fiber.StatusConflict).SendString("API token name already exists")
	case errors.Is(err, core.ErrAPITokenLimit):
		c.Set("X-Renop-Error-Code", "API_TOKEN_LIMIT")
		return c.Status(fiber.StatusConflict).SendString("API token limit reached")
	case errors.Is(err, errAPITokenNameInvalid), errors.Is(err, errAPITokenScopesInvalid),
		errors.Is(err, errAPITokenExpiryInvalid):
		c.Set("X-Renop-Error-Code", "API_TOKEN_INVALID")
		return c.Status(fiber.StatusBadRequest).SendString("API token request is invalid")
	case err != nil:
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to create API token")
	}
	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionTokenGenerate,
		Details: "Created API token " + token.Name, AuthMethod: authMethod, SessionID: sessionID, IP: ip,
	})
	setPrivateResponseHeaders(c)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"token": token, "secret": secret})
}

func deleteProfileAPIToken(c fiber.Ctx, state *core.AppState) error {
	user, err := requireAccountSession(c)
	if err != nil {
		return accountSessionError(c, err)
	}
	tokenID := strings.TrimSpace(c.Params("token_id"))
	if err := state.GetDB().DeleteAPIToken(user.Username, tokenID); errors.Is(err, core.ErrAPITokenNotFound) {
		return c.SendStatus(fiber.StatusNotFound)
	} else if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to revoke API token")
	}
	state.InvalidateAPITokenAuthCache(tokenID)
	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionTokenRevoke,
		Details: "Revoked API token " + tokenID, AuthMethod: authMethod, SessionID: sessionID, IP: ip,
	})
	return c.SendStatus(fiber.StatusNoContent)
}
