/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"errors"
	"strings"

	"renop/internal/config"
	"renop/internal/core"
)

func canWriteRepository(user *config.User, repo *config.Repository) bool {
	return user != nil && repo != nil && (user.IsManager() || user.CheckUpdatePermission(repo.Name))
}

// CanReadRepository applies repository visibility and npm package membership access.
func CanReadRepository(state *core.AppState, user *config.User, repo *config.Repository, path string, isRoot bool) (bool, error) {
	if repo == nil {
		return false, nil
	}
	if strings.EqualFold(repo.Visibility, "PUBLIC") {
		return true, nil
	}
	if user != nil && user.CheckReadPermission(repo.Name, path, repo.Visibility, isRoot) {
		return true, nil
	}
	if strings.EqualFold(repo.Visibility, "HIDDEN") {
		return false, nil
	}
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return false, nil
	}
	if state == nil || state.GetDB() == nil {
		return false, core.ErrDatabaseUnavailable
	}
	allowed, err := state.GetDB().HasNPMMembership(repo.Name, user.Username)
	if err != nil {
		return false, errors.Join(core.ErrDatabaseUnavailable, err)
	}
	return allowed, nil
}

// CanReadPackage applies package privacy after the repository read boundary.
func CanReadPackage(state *core.AppState, user *config.User, repo *config.Repository, packageName string) (bool, error) {
	if state == nil || state.GetDB() == nil || repo == nil {
		return false, core.ErrDatabaseUnavailable
	}
	username := ""
	if user != nil {
		username = user.Username
	}
	exists, private, _, member, _, err := state.GetDB().GetNPMPackageAccess(repo.Name, packageName, username)
	if err != nil {
		return false, err
	}
	if !exists {
		return CanReadRepository(state, user, repo, packageName, false)
	}
	if private {
		return member || user != nil && (user.IsManager() || user.CheckUpdatePermission(repo.Name) ||
			user.CheckReadPermission(repo.Name, packageName, "PRIVATE", false)), nil
	}
	return CanReadRepository(state, user, repo, packageName, false)
}
