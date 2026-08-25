/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package docker

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
)

var dockerImageComponentPattern = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$`)

// NormalizeImageName validates and canonicalizes a Docker/OCI image name.
func NormalizeImageName(value string) (string, bool) {
	value = strings.ToLower(strings.Trim(strings.TrimSpace(value), "/"))
	if value == "" || len(value) > 255 || strings.ContainsAny(value, "\x00\r\n") {
		return "", false
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if !dockerImageComponentPattern.MatchString(part) {
			return "", false
		}
	}
	return value, true
}

// ParseRepositoryAndImage splits an OCI image path (e.g. "docker-local/ubuntu" or "docker-local/app/service")
// into the RenoP repository name and the inner image name.
func ParseRepositoryAndImage(fullName string) (string, string) {
	fullName = strings.Trim(fullName, "/")
	parts := strings.Split(fullName, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], parts[0]
	}
	return parts[0], strings.Join(parts[1:], "/")
}

// CanReadDocker checks whether a user has read access to a Docker repository or specific image.
func CanReadDocker(state *core.AppState, user *config.User, repo *config.Repository, path string) bool {
	if repo == nil {
		return false
	}
	username := ""
	if user != nil {
		username = user.Username
	}
	if strings.Contains(strings.Trim(path, "/"), "/") {
		_, imageName := ParseRepositoryAndImage(path)
		if state != nil {
			db := state.GetDB()
			if db == nil {
				return false
			}
			exists, private, _, member, _, err := db.GetDockerImageAccess(repo.Name, imageName, username)
			if err != nil {
				return false
			}
			if exists && private {
				if user != nil && (user.IsManager() || user.CheckUpdatePermission(repo.Name)) {
					return true
				}
				return member
			}
		}
	}
	if strings.EqualFold(repo.Visibility, "PUBLIC") || strings.EqualFold(repo.Visibility, "HIDDEN") {
		return true
	}
	if user != nil && user.CheckReadPermission(repo.Name, path, repo.Visibility, false) {
		return true
	}
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return false
	}
	if state == nil {
		return false
	}
	db := state.GetDB()
	if db == nil {
		return false
	}
	allowed, err := db.HasDockerMembership(repo.Name, user.Username)
	if err == nil && allowed {
		return true
	}
	return false
}

// CanWriteDocker checks whether a user has push/mutate access to a Docker repository or specific image.
func CanWriteDocker(state *core.AppState, user *config.User, repo *config.Repository, repoFullName string) bool {
	if repo == nil || user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return false
	}
	if state == nil || !strings.Contains(strings.Trim(repoFullName, "/"), "/") {
		return false
	}
	db := state.GetDB()
	if db == nil {
		return false
	}
	_, imageName := ParseRepositoryAndImage(repoFullName)
	exists, _, pushEnabled, member, level, err := db.GetDockerImageAccess(repo.Name, imageName, user.Username)
	if err != nil || !exists || !pushEnabled {
		return false
	}
	if user.IsManager() || user.CheckUpdatePermission(repo.Name) {
		return true
	}
	return member && level >= core.DockerPermissionPublish
}

// SendAuthChallenge issues a WWW-Authenticate header directing the Docker CLI to the token endpoint.
func SendAuthChallenge(c fiber.Ctx, service, scope string) error {
	realm := fmt.Sprintf("%s://%s/v2/token", c.Scheme(), c.Host())
	headerVal := fmt.Sprintf(`Bearer realm="%s",service="%s"`, realm, service)
	if scope != "" {
		headerVal += fmt.Sprintf(`,scope="%s"`, scope)
	}
	c.Set(fiber.HeaderWWWAuthenticate, headerVal)
	return RespondError(c, fiber.StatusUnauthorized, ErrCodeUnauthorized, "authentication required", nil)
}
