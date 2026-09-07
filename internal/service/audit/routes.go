/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package audit records and exposes security-relevant application activity.
package audit

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
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
	router.Get("/logs", func(c fiber.Ctx) error { return GetGlobalLogs(c, state) })
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
	page = min(page, 1000000/pageSize+1)
	offset = (page - 1) * pageSize
	return limit, offset, page, pageSize
}

func GetSelfAuditLogs(c fiber.Ctx, state *core.AppState) error {
	user := getUserFromCtx(c)
	if user == nil || user.Username == "" || user.Username == "guest" {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}

	limit, offset, page, pageSize := parsePageParams(c)
	filter, err := parseLogFilter(c)
	if err != nil {
		return invalidLogFilter(c)
	}
	filter.Username, filter.Kind = user.Username, "audit"
	for _, field := range []struct{ value, exclude *string }{
		{&filter.Operator, &filter.ExcludeOperator}, {&filter.Initiator, &filter.ExcludeInitiator},
	} {
		if *field.value == "@administrator" {
			*field.value, *field.exclude = "", user.Username
		} else if *field.value != "" && !strings.EqualFold(*field.value, user.Username) {
			return invalidLogFilter(c)
		}
	}
	logs, total, err := fetchLogs(state, filter, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	maskedLogs := make([]*core.AuditLogEntry, len(logs))
	for i, l := range logs {
		lCopy := *l
		if !strings.EqualFold(lCopy.Operator, user.Username) {
			lCopy.Operator = "Administrator"
		}
		if !strings.EqualFold(lCopy.Initiator, user.Username) {
			lCopy.Initiator = "Administrator"
		}
		maskedLogs[i] = &lCopy
	}

	return protohttp.Write(c, pb.FromAuditLogList(maskedLogs, total, page, pageSize))
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
	filter, err := parseLogFilter(c)
	if err != nil {
		return invalidLogFilter(c)
	}
	filter.Username, filter.Kind = targetUsername, "audit"
	logs, total, err := fetchLogs(state, filter, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	return protohttp.Write(c, pb.FromAuditLogList(logs, total, page, pageSize))
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

	logUsername, action := targetUsername, ActionLogClear
	if db := state.GetDB(); db != nil {
		account, err := db.GetTokenByName(targetUsername)
		if err != nil {
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		if account != nil && account.DeletedAt > 0 {
			err = db.PurgeRetiredAccountAuditLogs(targetUsername, time.Now().UnixMilli())
			logUsername, action = user.Username, ActionAccountAuditPurge
		} else {
			err = db.DeleteAuditLogsByUsername(targetUsername)
		}
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
		}
	}

	_, op, authMethod, sessionID, ip := ExtractAuthDetails(c, state)
	Log(state, &core.AuditLogEntry{
		Username:   logUsername,
		Operator:   op,
		Action:     action,
		Details:    "User activity logs cleared by admin for " + targetUsername,
		AuthMethod: authMethod,
		SessionID:  sessionID,
		IP:         ip,
	})

	return protohttp.Write(c, pb.StatusOkSuccess())
}

func fetchLogs(state *core.AppState, filter core.AuditLogFilter, limit, offset int) ([]*core.AuditLogEntry, int, error) {
	if db := state.GetDB(); db != nil {
		return db.FilterAuditLogs(filter, limit, offset)
	}
	return []*core.AuditLogEntry{}, 0, nil
}
