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
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/config"
	"renop/core"
	"renop/utils"
)

// ExtractAuthDetails gets caller info, IP, auth method, and session ID from Fiber context.
func ExtractAuthDetails(c fiber.Ctx) (username string, operator string, authMethod string, sessionID string, ip string) {
	var sCfg *config.ServerConfig
	if c != nil {
		userVal := c.Locals("user")
		if userVal != nil {
			if u, ok := userVal.(*config.User); ok {
				username = u.Username
				operator = u.Username
			}
		}

		if configVal := c.Locals("config"); configVal != nil {
			if cfg, ok := configVal.(*config.Config); ok {
				sCfg = &cfg.Server
			}
		}
		if sCfg == nil {
			d := config.DefaultServerConfig()
			sCfg = &d
		}
		ip = utils.ExtractIP(c, sCfg)

		authHeader := c.Get(fiber.HeaderAuthorization)
		cookieVal := c.Cookies("renop_session")

		if currentSess, ok := c.Locals("current_session_id").(string); ok && currentSess != "" {
			sessionID = currentSess
			authMethod = fmt.Sprintf("Web (SessionID: %s)", safeSessionPrefix(currentSess))
		} else if strings.HasPrefix(authHeader, "Session ") || cookieVal != "" {
			sID := strings.TrimPrefix(authHeader, "Session ")
			if sID == "" {
				sID = cookieVal
			}
			sessionID = sID
			authMethod = fmt.Sprintf("Web (SessionID: %s)", safeSessionPrefix(sID))
		} else if strings.HasPrefix(authHeader, "Basic ") {
			authMethod = "BasicAuth"
		} else if strings.HasPrefix(authHeader, "Bearer ") {
			authMethod = "UploadToken"
		} else {
			authMethod = "Web"
		}
	}

	if username == "" {
		username = "guest"
		operator = "guest"
	}

	return username, operator, authMethod, sessionID, ip
}

func safeSessionPrefix(s string) string {
	if len(s) > 8 {
		return s[:8] + "..."
	}
	return s
}

func Log(state *core.AppState, entry *core.AuditLogEntry) {
	if state == nil || state.Inner == nil || entry == nil {
		return
	}
	if entry.CreatedAt <= 0 {
		entry.CreatedAt = time.Now().UnixMilli()
	}
	select {
	case state.Inner.AuditLogChan <- entry:
	default:
		// Non-blocking drop if channel is full
	}
}
