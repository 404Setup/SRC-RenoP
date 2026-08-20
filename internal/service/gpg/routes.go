/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package gpg

import (
	"errors"
	"log"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func SetupProfileRoutes(router fiber.Router, state *core.AppState) {
	profile := router.Group("/auth/profile/gpg")
	profile.Get("", func(c fiber.Ctx) error { return ListProfileKeys(c, state) })
	profile.Get("/releases", func(c fiber.Ctx) error { return ListProfileReleases(c, state) })
	profile.Post("", func(c fiber.Ctx) error { return AddProfileKey(c, state) })
	profile.Delete("/:fingerprint", func(c fiber.Ctx) error { return DeleteProfileKey(c, state) })
}

func ListProfileReleases(c fiber.Ctx, state *core.AppState) error {
	username, err := profileUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	limit = min(max(limit, 1), 100)
	offset = max(offset, 0)
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	releases, total, err := db.ListGPGReleases(*username, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load GPG releases")
	}
	return protohttp.Write(c, pb.FromGPGReleases(releases, total, limit, offset))
}

func profileUser(c fiber.Ctx) (*string, error) {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || user.Username == "guest" {
		return nil, fiber.ErrUnauthorized
	}
	username := user.Username
	return &username, nil
}

func ListProfileKeys(c fiber.Ctx, state *core.AppState) error {
	username, err := profileUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	keys, err := db.ListUserGPGKeys(*username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load GPG keys")
	}
	return protohttp.Write(c, pb.FromUserGPGKeys(keys))
}

func AddProfileKey(c fiber.Ctx, state *core.AppState) error {
	username, err := profileUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	var request pb.GpgKeyReferenceRequest
	if err := protohttp.Read(c, &request); err != nil {
		if err == fiber.ErrRequestEntityTooLarge {
			return err
		}
		return c.Status(fiber.StatusBadRequest).SendString("Invalid GPG key ID or fingerprint")
	}
	key, err := RegisterUserKey(c.Context(), state, *username, request.KeyId)
	if err != nil {
		switch {
		case errors.Is(err, errInvalidKeyReference):
			return c.Status(fiber.StatusBadRequest).SendString("Invalid GPG key ID or fingerprint")
		case errors.Is(err, core.ErrGPGKeyLimit):
			return c.Status(fiber.StatusConflict).SendString("A user may register at most 10 GPG keys")
		case errors.Is(err, errAmbiguousKey):
			return c.Status(fiber.StatusUnprocessableEntity).SendString("GPG key ID is ambiguous; use the full fingerprint")
		case errors.Is(err, errKeyNotFound):
			return c.Status(fiber.StatusUnprocessableEntity).SendString("GPG key could not be resolved by configured key servers")
		default:
			log.Printf("Failed to register GPG key for user %q: %v", *username, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to register GPG key")
		}
	}
	usernameValue, op, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: usernameValue, Operator: op, Action: "GPG_UPDATE",
		Details: "Registered GPG key (" + key.Fingerprint + ")", AuthMethod: authMethod,
		SessionID: sessionID, IP: ip,
	})
	return protohttp.WriteStatus(c, fiber.StatusCreated, pb.FromUserGPGKey(key))
}

func DeleteProfileKey(c fiber.Ctx, state *core.AppState) error {
	username, err := profileUser(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	fingerprint, err := normalizeFingerprint(c.Params("fingerprint"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid GPG fingerprint")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	if err := db.DeleteUserGPGKey(*username, fingerprint); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete GPG key")
	}
	usernameValue, op, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: usernameValue, Operator: op, Action: "GPG_UPDATE",
		Details: "Deleted GPG key (" + strings.ToUpper(fingerprint) + ")", AuthMethod: authMethod,
		SessionID: sessionID, IP: ip,
	})
	return c.SendStatus(fiber.StatusNoContent)
}
