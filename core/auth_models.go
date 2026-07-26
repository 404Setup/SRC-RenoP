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
	"sync/atomic"
)

const (
	SessionIdleTimeoutMillis     int64 = 7 * 24 * 60 * 60 * 1000
	SessionRenewalIntervalMillis int64 = 5 * 60 * 1000
)

type AccessTokenPermission struct {
	Identifier string `json:"identifier"`
	Shortcut   string `json:"shortcut"`
}

type RoutePermission struct {
	Identifier string `json:"identifier"`
	Shortcut   string `json:"shortcut"`
}

type Route struct {
	Path       string          `json:"path"`
	Permission RoutePermission `json:"permission"`
}

type SessionDetails struct {
	AccessToken  AccessTokenDto          `json:"access_token"`
	Permissions  []AccessTokenPermission `json:"permissions"`
	Routes       []Route                 `json:"routes"`
	SessionToken string                  `json:"session_token"`
}

type LoginRequest struct {
	Name   string `json:"name"`
	Secret string `json:"secret"`
}

type Session struct {
	PublicId   string
	Username   string
	Ip         string
	UserAgent  string
	CreatedAt  int64
	LastActive atomic.Int64
}

type SessionDbDto struct {
	PublicId     string `json:"public_id"`
	SessionToken string `json:"session_token"`
	Username     string `json:"username"`
	Ip           string `json:"ip"`
	UserAgent    string `json:"user_agent"`
	CreatedAt    int64  `json:"created_at"`
	LastActive   int64  `json:"last_active"`
}

type SessionDto struct {
	PublicId   string `json:"public_id"`
	Username   string `json:"username"`
	Ip         string `json:"ip"`
	UserAgent  string `json:"user_agent"`
	CreatedAt  int64  `json:"created_at"`
	LastActive int64  `json:"last_active"`
	ExpiresAt  int64  `json:"expires_at"`
	Current    bool   `json:"current"`
}

type CurrentSessionId struct {
	ID string
}
