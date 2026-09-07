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
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"renop/internal/core"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func invalidLogFilter(c fiber.Ctx) error {
	c.Set("X-Renop-Error-Code", "LOG_FILTER_INVALID")
	return c.Status(fiber.StatusBadRequest).SendString("Invalid log filter")
}

func parseLogFilter(c fiber.Ctx) (core.AuditLogFilter, error) {
	f := core.AuditLogFilter{
		Username:  strings.ToLower(strings.TrimSpace(c.Query("username"))),
		Operator:  strings.ToLower(strings.TrimSpace(c.Query("operator"))),
		Initiator: strings.ToLower(strings.TrimSpace(c.Query("initiator"))),
		Kind:      c.Query("kind"), Action: c.Query("action"), Trigger: c.Query("trigger"), Severity: c.Query("severity"),
	}
	invalid := errors.New("invalid log filter")
	if len(f.Username) > 255 || len(f.Operator) > 255 || len(f.Initiator) > 255 || len(f.Action) > 64 || len(f.Trigger) > 64 ||
		(f.Kind != "" && f.Kind != "audit" && f.Kind != "system") ||
		(f.Severity != "" && f.Severity != "info" && f.Severity != "warning" && f.Severity != "error") {
		return f, invalid
	}
	for _, field := range []struct {
		name  string
		value *int64
	}{{"from", &f.From}, {"until", &f.Until}} {
		if raw := c.Query(field.name); raw != "" {
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || parsed < 1 || parsed > 253402300799999 {
				return f, invalid
			}
			*field.value = parsed
		}
	}
	if f.From > 0 && f.Until > 0 && f.From > f.Until {
		return f, invalid
	}
	return f, nil
}

// GetGlobalLogs exposes operational diagnostics and activity only to system administrators.
func GetGlobalLogs(c fiber.Ctx, state *core.AppState) error {
	if !isManager(getUserFromCtx(c)) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	filter, err := parseLogFilter(c)
	if err != nil {
		return invalidLogFilter(c)
	}
	limit, offset, page, pageSize := parsePageParams(c)
	logs, total, err := fetchLogs(state, filter, limit, offset)
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return protohttp.Write(c, pb.FromAuditLogList(logs, total, page, pageSize))
}
