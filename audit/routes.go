/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package audit

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/config"
	"renop/core"
)

type AuditLogListResponse struct {
	Logs     []*core.AuditLogEntry `json:"logs"`
	Total    int                   `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
}

func getUserFromCtx(c fiber.Ctx) *config.User {
	if val := c.Locals("user"); val != nil {
		if u, ok := val.(*config.User); ok {
			return u
		}
	}
	return nil
}

func isManager(user *config.User) bool {
	if user == nil {
		return false
	}
	return user.IsManager()
}

func SetupAuditRoutes(router fiber.Router, state *core.AppState) {
	router.Get("/profile/audit-logs", func(c fiber.Ctx) error { return GetSelfAuditLogs(c, state) })
	router.Delete("/profile/audit-logs", func(c fiber.Ctx) error { return DeleteSelfAuditLogs(c) })
	router.Get("/users/:username/audit-logs", func(c fiber.Ctx) error { return GetUserAuditLogs(c, state) })
	router.Delete("/users/:username/audit-logs", func(c fiber.Ctx) error { return DeleteUserAuditLogs(c, state) })
}

func parsePageParams(c fiber.Ctx) (limit int, offset int, page int, pageSize int) {
	page, _ = strconv.Atoi(c.Query("page", "1"))
	pageSize, _ = strconv.Atoi(c.Query("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	limit = pageSize
	offset = (page - 1) * pageSize
	return limit, offset, page, pageSize
}

func GetSelfAuditLogs(c fiber.Ctx, state *core.AppState) error {
	user := getUserFromCtx(c)
	if user == nil || user.Username == "" || user.Username == "guest" {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}

	limit, offset, page, pageSize := parsePageParams(c)
	logs, total, err := fetchLogs(state, user.Username, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	maskedLogs := make([]*core.AuditLogEntry, len(logs))
	for i, l := range logs {
		lCopy := *l
		if !strings.EqualFold(lCopy.Operator, user.Username) {
			lCopy.Operator = "Administrator"
		}
		maskedLogs[i] = &lCopy
	}

	return c.JSON(AuditLogListResponse{
		Logs:     maskedLogs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

func DeleteSelfAuditLogs(c fiber.Ctx) error {
	user := getUserFromCtx(c)
	if user == nil || user.Username == "" || user.Username == "guest" {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	return c.Status(fiber.StatusForbidden).SendString("Users cannot clear their own activity logs")
}

func GetUserAuditLogs(c fiber.Ctx, state *core.AppState) error {
	user := getUserFromCtx(c)
	if !isManager(user) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	targetUsername := strings.ToLower(c.Params("username"))
	if targetUsername == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}

	limit, offset, page, pageSize := parsePageParams(c)
	logs, total, err := fetchLogs(state, targetUsername, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	return c.JSON(AuditLogListResponse{
		Logs:     logs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

func DeleteUserAuditLogs(c fiber.Ctx, state *core.AppState) error {
	user := getUserFromCtx(c)
	if !isManager(user) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	targetUsername := strings.ToLower(c.Params("username"))
	if targetUsername == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}

	if db := state.GetDB(); db != nil {
		if err := db.DeleteAuditLogsByUsername(targetUsername); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
		}
	} else {
		state.Inner.AuditLogLock.Lock()
		filtered := make([]*core.AuditLogEntry, 0, len(state.Inner.AuditLogsMem))
		for _, e := range state.Inner.AuditLogsMem {
			if !strings.EqualFold(e.Username, targetUsername) {
				filtered = append(filtered, e)
			}
		}
		state.Inner.AuditLogsMem = filtered
		state.Inner.AuditLogLock.Unlock()
	}

	_, op, authMethod, sessionID, ip := ExtractAuthDetails(c)
	Log(state, &core.AuditLogEntry{
		Username:   targetUsername,
		Operator:   op,
		Action:     "LOG_CLEAR",
		Details:    "User activity logs cleared by admin",
		AuthMethod: authMethod,
		SessionID:  sessionID,
		IP:         ip,
	})

	return c.JSON(fiber.Map{"status": "ok"})
}

func fetchLogs(state *core.AppState, username string, limit, offset int) ([]*core.AuditLogEntry, int, error) {
	if db := state.GetDB(); db != nil {
		return db.GetAuditLogs(username, limit, offset)
	}

	state.Inner.AuditLogLock.RLock()
	defer state.Inner.AuditLogLock.RUnlock()

	var matching []*core.AuditLogEntry
	lower := strings.ToLower(username)
	for i := len(state.Inner.AuditLogsMem) - 1; i >= 0; i-- {
		e := state.Inner.AuditLogsMem[i]
		if lower == "" || strings.EqualFold(e.Username, lower) {
			matching = append(matching, e)
		}
	}

	total := len(matching)
	if offset >= total {
		return []*core.AuditLogEntry{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return matching[offset:end], total, nil
}
