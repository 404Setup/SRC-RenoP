/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const auditSessionHashPrefix = "sha256:"

type AuditLogEntry struct {
	ID         int64  `json:"id"`
	Username   string `json:"username"`
	Operator   string `json:"operator"`
	Initiator  string `json:"initiator"`
	Action     string `json:"action"`
	Details    string `json:"details"`
	AuthMethod string `json:"auth_method"`
	SessionID  string `json:"session_id"`
	IP         string `json:"ip"`
	CreatedAt  int64  `json:"created_at"`
	Kind       string `json:"kind"`
	Trigger    string `json:"trigger"`
	Severity   string `json:"severity"`
}

// AuditLogFilter narrows log queries without changing the caller's visibility scope.
type AuditLogFilter struct {
	Username, Operator, ExcludeOperator string
	Initiator, ExcludeInitiator         string
	Kind, Action, Trigger, Severity     string
	From, Until                         int64
}

// AuditTrigger classifies the origin of existing audit producers.
func AuditTrigger(entry *AuditLogEntry) string {
	if entry.Trigger != "" {
		return entry.Trigger
	}
	method := strings.ToLower(entry.AuthMethod)
	if strings.HasPrefix(method, "web") || method == "password" || method == "passkey" ||
		method == "github" || method == "session" {
		return "web"
	}
	if method != "" && method != "system" {
		return "api"
	}
	return "system"
}

// SafeAuditSessionID returns a stable, non-authenticating identifier for audit
// correlation. It is idempotent so legacy rows can be sanitized when read.
func SafeAuditSessionID(sessionToken string) string {
	if sessionToken == "" {
		return ""
	}
	if strings.HasPrefix(sessionToken, auditSessionHashPrefix) {
		digest := sessionToken[len(auditSessionHashPrefix):]
		if len(digest) == 16 {
			if _, err := hex.DecodeString(digest); err == nil {
				return auditSessionHashPrefix + strings.ToLower(digest)
			}
		}
	}
	sum := sha256.Sum256([]byte(sessionToken))
	return auditSessionHashPrefix + hex.EncodeToString(sum[:8])
}
