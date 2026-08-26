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
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/utils"
)

var fallbackPersistMu sync.Mutex

// ExtractAuthDetails gets caller info, IP, auth method, and session ID from Fiber context.
func ExtractAuthDetails(c fiber.Ctx, state *core.AppState) (username string, operator string, authMethod string, sessionID string, ip string) {
	if c != nil {
		userVal := c.Locals("user")
		if userVal != nil {
			if u, ok := userVal.(*config.User); ok {
				username = u.Username
				operator = u.Username
			}
		}

		sCfg := config.DefaultServerConfig()
		if state != nil && state.Inner != nil {
			if cfg := state.Inner.Config.Load(); cfg != nil {
				sCfg = cfg.Server
			}
		}
		ip = utils.ExtractIP(c, &sCfg)

		authHeader := c.Get(fiber.HeaderAuthorization)
		cookieVal := c.Cookies("renop_session")
		credentialKind, _ := c.Locals("auth_credential_kind").(string)
		apiTokenID, _ := c.Locals("auth_api_token_id").(string)

		if currentSess, ok := c.Locals("current_session_id").(string); ok && currentSess != "" {
			sessionID = core.SafeAuditSessionID(currentSess)
			authMethod = fmt.Sprintf("Web (SessionID: %s)", sessionID)
		} else if strings.HasPrefix(authHeader, "Session ") || cookieVal != "" {
			sID := strings.TrimPrefix(authHeader, "Session ")
			if sID == "" {
				sID = cookieVal
			}
			sessionID = core.SafeAuditSessionID(sID)
			authMethod = fmt.Sprintf("Web (SessionID: %s)", sessionID)
		} else if credentialKind == "api_token" {
			authMethod = "APIToken"
			if apiTokenID != "" {
				authMethod += " (ID: " + core.SafeAuditSessionID(apiTokenID) + ")"
			}
		} else if credentialKind == "password" || strings.HasPrefix(authHeader, "Basic ") {
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
		fallbackPersistMu.Lock()
		persistAuditEntry(state, entry, 1)
		fallbackPersistMu.Unlock()
	}
}
