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
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
)

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

// CanReadDocker checks whether a user has read access to a Docker repository.
func CanReadDocker(state *core.AppState, user *config.User, repo *config.Repository, path string) bool {
	if repo == nil {
		return false
	}
	if strings.EqualFold(repo.Visibility, "PUBLIC") || strings.EqualFold(repo.Visibility, "HIDDEN") {
		return true
	}
	if user != nil && user.CheckReadPermission(repo.Name, path, repo.Visibility, false) {
		return true
	}
	return false
}

// CanWriteDocker checks whether a user has push/mutate access to a Docker repository.
func CanWriteDocker(state *core.AppState, user *config.User, repo *config.Repository, repoName string) bool {
	if repo == nil || user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return false
	}
	return user.CheckUpdatePermission(repoName)
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
