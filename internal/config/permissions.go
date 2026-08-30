/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package config

import (
	"strings"

	"github.com/llxisdsh/pb"
)

func (u *User) initPermissions() {
	u.once.Do(func() {
		u.updateRepos = make(map[string]bool)
		u.moderateRepos = make(map[string]bool)
		u.viewRepos = make(map[string]bool)
		for _, r := range u.Roles {
			if r == "manager" {
				u.isManager = true
				u.isAdmin = true
			} else if r == "admin" {
				u.isAdmin = true
				u.isManager = true
			} else if r == "allview" || r == "proview" {
				u.isAllView = true
			} else if r == "showing" {
				u.isShowing = true
			} else if r == "canupdate:*" {
				u.canUpdateAll = true
			} else if len(r) > 10 && r[:10] == "canupdate:" {
				u.updateRepos[r[10:]] = true
			} else if r == "canmoderate:*" {
				u.canModerateAll = true
			} else if len(r) > 12 && r[:12] == "canmoderate:" {
				u.moderateRepos[r[12:]] = true
			} else if r == "canview:*" {
				u.canViewAll = true
			} else if len(r) > 8 && r[:8] == "canview:" {
				u.viewRepos[r[8:]] = true
			}
		}
	})
}

func (u *User) GetPermissions() (isAdmin bool, isAllView bool, isShowing bool) {
	u.initPermissions()
	return u.isAdmin, u.isAllView, u.isShowing
}

func equalFoldASCII(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i]|0x20 != t[i]|0x20 {
			return false
		}
	}
	return true
}

func (u *User) CheckReadPermission(repoName string, path string, repoVisibility string, isRoot bool) bool {
	u.initPermissions()

	isPublic := equalFoldASCII(repoVisibility, "PUBLIC")
	isHidden := equalFoldASCII(repoVisibility, "HIDDEN")
	isPrivate := equalFoldASCII(repoVisibility, "PRIVATE")

	if !isPublic && !isHidden && !isPrivate {
		return false
	}

	if u.isAdmin {
		return true
	}

	if u.canUpdateAll || u.updateRepos[repoName] || u.canModerateAll || u.moderateRepos[repoName] {
		return true
	}

	if u.canViewAll || u.viewRepos[repoName] {
		return true
	}

	if isPublic {
		return true
	} else if isHidden {
		if isRoot || strings.HasSuffix(path, "/") || path == "" {
			return u.isAllView || u.isShowing
		}
		return true
	} else if isPrivate {
		return u.isAllView
	}

	return false
}

// CheckModeratePermission reports whether the account may review content in one repository.
func (u *User) CheckModeratePermission(repoName string) bool {
	u.initPermissions()
	return u.isAdmin || u.canModerateAll || u.moderateRepos[repoName]
}

func (u *User) CheckUpdatePermission(repoName string) bool {
	u.initPermissions()

	if u.isAdmin || u.canUpdateAll {
		return true
	}

	return u.updateRepos[repoName]
}

var repoCacheConfigs pb.MapOf[string, RepositoryCacheConfig]

type RepositoryCacheConfig struct {
	Persist bool
	TTL     uint64
}

func (r *Repository) GetCacheConfig() (persist bool, ttl uint64) {
	if val, ok := repoCacheConfigs.Load(r.Name); ok {
		return val.Persist, val.TTL
	}

	var baseMaxTTL uint64 = 0
	var anyPersist = false

	if len(r.Mirrors) > 0 {
		for _, mirror := range r.Mirrors {
			if mirror.Persist {
				anyPersist = true
			} else {
				if mirror.CacheTTLSecs > baseMaxTTL {
					baseMaxTTL = mirror.CacheTTLSecs
				}
			}
		}
	} else {
		anyPersist = true
	}

	config := RepositoryCacheConfig{Persist: anyPersist, TTL: baseMaxTTL}
	repoCacheConfigs.Store(r.Name, config)

	return anyPersist, baseMaxTTL
}

func ClearRepoCacheConfigs() {
	repoCacheConfigs.Range(func(key string, _ RepositoryCacheConfig) bool {
		repoCacheConfigs.Delete(key)
		return true
	})
}
