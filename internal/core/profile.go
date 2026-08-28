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
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxNicknameRunes           = 36
	MinUsernameLength          = 4
	MaxUsernameLength          = 18
	MaxUsernameChangesPerDay   = 2
	UsernameChangeWindowMillis = int64(24 * 60 * 60 * 1000)
)

var (
	ErrUserProfileNotFound       = errors.New("user profile was not found")
	ErrUsernameAlreadyExists     = errors.New("username already exists")
	ErrUsernameChangeRateLimited = errors.New("username change rate limit exceeded")
)

// UserProfile contains public account identity and durable username-change state.
type UserProfile struct {
	UserID                 string `json:"user_id"`
	Username               string `json:"username"`
	Nickname               string `json:"nickname"`
	CreatedAt              string `json:"created_at"`
	MavenDomainCount       int    `json:"maven_domain_count"`
	CargoPackageCount      int    `json:"cargo_package_count"`
	DockerImageCount       int    `json:"docker_image_count"`
	NPMPackageCount        int    `json:"npm_package_count"`
	UsernameChangeCount    int    `json:"-"`
	UsernameChangeWindowAt int64  `json:"-"`
}

// UserPackageMembership describes one package team linked through an immutable user ID.
type UserPackageMembership struct {
	Format          string `json:"format"`
	Repository      string `json:"repository"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	PermissionLevel int    `json:"permission_level"`
	Archived        bool   `json:"archived,omitempty"`
}

// UsernameChangeRateError reports when another username change becomes available.
type UsernameChangeRateError struct {
	RetryAt int64
}

// Error implements error.
func (err *UsernameChangeRateError) Error() string {
	return ErrUsernameChangeRateLimited.Error()
}

// Unwrap allows errors.Is to match ErrUsernameChangeRateLimited.
func (err *UsernameChangeRateError) Unwrap() error {
	return ErrUsernameChangeRateLimited
}

// NormalizeUsername validates a replacement username and returns its canonical lowercase form.
func NormalizeUsername(username string) (string, bool) {
	username = strings.TrimSpace(username)
	if len(username) < MinUsernameLength || len(username) > MaxUsernameLength {
		return "", false
	}
	for index := 0; index < len(username); index++ {
		value := username[index]
		if (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') ||
			(value >= '0' && value <= '9') || value == '_' {
			continue
		}
		return "", false
	}
	return strings.ToLower(username), true
}

// NormalizeNickname trims and validates a nickname without changing its Unicode contents.
func NormalizeNickname(nickname string) (string, bool) {
	nickname = strings.TrimSpace(nickname)
	if !utf8.ValidString(nickname) || utf8.RuneCountInString(nickname) > MaxNicknameRunes {
		return "", false
	}
	for _, value := range nickname {
		if unicode.IsControl(value) {
			return "", false
		}
	}
	return nickname, true
}
