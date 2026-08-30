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
	"testing"
)

func createUserWithRoles(roles []string) User {
	return User{
		Username:         "testuser",
		PasswordHash:     "hash",
		Tokens:           []string{},
		Roles:            roles,
		ReadPermissions:  []string{},
		WritePermissions: []string{},
	}
}

func TestUserGetPermissions(t *testing.T) {
	user := createUserWithRoles([]string{})
	isAdmin, isAllView, isShowing := user.GetPermissions()
	if isAdmin || isAllView || isShowing {
		t.Errorf("Expected false, false, false; got %v, %v, %v", isAdmin, isAllView, isShowing)
	}

	userAdmin := createUserWithRoles([]string{"admin"})
	isAdmin, isAllView, isShowing = userAdmin.GetPermissions()
	if !isAdmin || isAllView || isShowing {
		t.Errorf("Expected true, false, false; got %v, %v, %v", isAdmin, isAllView, isShowing)
	}

	userManager := createUserWithRoles([]string{"manager"})
	isAdmin, isAllView, isShowing = userManager.GetPermissions()
	if !isAdmin || isAllView || isShowing {
		t.Errorf("Expected true, false, false; got %v, %v, %v", isAdmin, isAllView, isShowing)
	}

	userAllview := createUserWithRoles([]string{"allview"})
	isAdmin, isAllView, isShowing = userAllview.GetPermissions()
	if isAdmin || !isAllView || isShowing {
		t.Errorf("Expected false, true, false; got %v, %v, %v", isAdmin, isAllView, isShowing)
	}

	userProview := createUserWithRoles([]string{"proview"})
	isAdmin, isAllView, isShowing = userProview.GetPermissions()
	if isAdmin || !isAllView || isShowing {
		t.Errorf("Expected false, true, false; got %v, %v, %v", isAdmin, isAllView, isShowing)
	}

	userShowing := createUserWithRoles([]string{"showing"})
	isAdmin, isAllView, isShowing = userShowing.GetPermissions()
	if isAdmin || isAllView || !isShowing {
		t.Errorf("Expected false, false, true; got %v, %v, %v", isAdmin, isAllView, isShowing)
	}

	userAll := createUserWithRoles([]string{"admin", "allview", "proview", "showing"})
	isAdmin, isAllView, isShowing = userAll.GetPermissions()
	if !isAdmin || !isAllView || !isShowing {
		t.Errorf("Expected true, true, true; got %v, %v, %v", isAdmin, isAllView, isShowing)
	}

	userUnknown := createUserWithRoles([]string{"unknown"})
	isAdmin, isAllView, isShowing = userUnknown.GetPermissions()
	if isAdmin || isAllView || isShowing {
		t.Errorf("Expected false, false, false; got %v, %v, %v", isAdmin, isAllView, isShowing)
	}

	userMixed := createUserWithRoles([]string{"unknown", "allview"})
	isAdmin, isAllView, isShowing = userMixed.GetPermissions()
	if isAdmin || !isAllView || isShowing {
		t.Errorf("Expected false, true, false; got %v, %v, %v", isAdmin, isAllView, isShowing)
	}

	userUppercase := createUserWithRoles([]string{"ADMIN", "Manager"})
	isAdmin, isAllView, isShowing = userUppercase.GetPermissions()
	if isAdmin || isAllView || isShowing {
		t.Errorf("Expected false, false, false; got %v, %v, %v", isAdmin, isAllView, isShowing)
	}
}

func TestCheckReadPermissionPublic(t *testing.T) {
	adminUser := createUserWithRoles([]string{"admin"})
	managerUser := createUserWithRoles([]string{"manager"})
	noRoleUser := createUserWithRoles([]string{})

	if !noRoleUser.CheckReadPermission("releases", "some/path", "PUBLIC", false) {
		t.Error("Failed CheckReadPermission PUBLIC 1")
	}
	if !noRoleUser.CheckReadPermission("releases", "some/path/", "PUBLIC", true) {
		t.Error("Failed CheckReadPermission PUBLIC 2")
	}
	if !noRoleUser.CheckReadPermission("releases", "", "PUBLIC", false) {
		t.Error("Failed CheckReadPermission PUBLIC 3")
	}
	if !adminUser.CheckReadPermission("releases", "some/path", "PUBLIC", false) {
		t.Error("Failed CheckReadPermission PUBLIC 4")
	}
	if !managerUser.CheckReadPermission("releases", "some/path", "PUBLIC", false) {
		t.Error("Failed CheckReadPermission PUBLIC 5")
	}
}

func TestCheckReadPermissionPrivate(t *testing.T) {
	adminUser := createUserWithRoles([]string{"admin"})
	managerUser := createUserWithRoles([]string{"manager"})
	allviewUser := createUserWithRoles([]string{"allview"})
	proviewUser := createUserWithRoles([]string{"proview"})
	showingUser := createUserWithRoles([]string{"showing"})
	noRoleUser := createUserWithRoles([]string{})

	if !adminUser.CheckReadPermission("private", "path", "PRIVATE", false) {
		t.Error("Failed CheckReadPermission PRIVATE 1")
	}
	if !managerUser.CheckReadPermission("private", "path", "PRIVATE", false) {
		t.Error("Failed CheckReadPermission PRIVATE 2")
	}
	if !allviewUser.CheckReadPermission("private", "path", "PRIVATE", false) {
		t.Error("Failed CheckReadPermission PRIVATE 3")
	}
	if !proviewUser.CheckReadPermission("private", "path", "PRIVATE", false) {
		t.Error("Failed CheckReadPermission PRIVATE 4")
	}
	if showingUser.CheckReadPermission("private", "path", "PRIVATE", false) {
		t.Error("Failed CheckReadPermission PRIVATE 5")
	}
	if noRoleUser.CheckReadPermission("private", "path", "PRIVATE", false) {
		t.Error("Failed CheckReadPermission PRIVATE 6")
	}
	if !adminUser.CheckReadPermission("private", "dir/", "PRIVATE", true) {
		t.Error("Failed CheckReadPermission PRIVATE 7")
	}
	if !managerUser.CheckReadPermission("private", "", "PRIVATE", false) {
		t.Error("Failed CheckReadPermission PRIVATE 8")
	}
	if !allviewUser.CheckReadPermission("private", "a/b", "PRIVATE", true) {
		t.Error("Failed CheckReadPermission PRIVATE 9")
	}
	if noRoleUser.CheckReadPermission("private", "dir/", "PRIVATE", true) {
		t.Error("Failed CheckReadPermission PRIVATE 10")
	}
	if showingUser.CheckReadPermission("private", "", "PRIVATE", false) {
		t.Error("Failed CheckReadPermission PRIVATE 11")
	}
}

func TestCheckReadPermissionHidden(t *testing.T) {
	adminUser := createUserWithRoles([]string{"admin"})
	managerUser := createUserWithRoles([]string{"manager"})
	allviewUser := createUserWithRoles([]string{"allview"})
	proviewUser := createUserWithRoles([]string{"proview"})
	showingUser := createUserWithRoles([]string{"showing"})
	noRoleUser := createUserWithRoles([]string{})

	if !noRoleUser.CheckReadPermission("hidden", "some/file.txt", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 1")
	}
	if !adminUser.CheckReadPermission("hidden", "some/file.txt", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 2")
	}
	if !managerUser.CheckReadPermission("hidden", "some/file.txt", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 3")
	}
	if !allviewUser.CheckReadPermission("hidden", "some/file.txt", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 4")
	}
	if !proviewUser.CheckReadPermission("hidden", "some/file.txt", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 5")
	}
	if !showingUser.CheckReadPermission("hidden", "some/file.txt", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 6")
	}

	if !adminUser.CheckReadPermission("hidden", "some/dir/", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 7")
	}
	if !managerUser.CheckReadPermission("hidden", "some/dir/", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 8")
	}
	if !allviewUser.CheckReadPermission("hidden", "some/dir/", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 9")
	}
	if !proviewUser.CheckReadPermission("hidden", "some/dir/", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 10")
	}
	if !showingUser.CheckReadPermission("hidden", "some/dir/", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 11")
	}
	if noRoleUser.CheckReadPermission("hidden", "some/dir/", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 12")
	}

	if !adminUser.CheckReadPermission("hidden", "some/file.txt", "HIDDEN", true) {
		t.Error("Failed CheckReadPermission HIDDEN 13")
	}
	if !managerUser.CheckReadPermission("hidden", "some/file.txt", "HIDDEN", true) {
		t.Error("Failed CheckReadPermission HIDDEN 14")
	}
	if !allviewUser.CheckReadPermission("hidden", "some/file.txt", "HIDDEN", true) {
		t.Error("Failed CheckReadPermission HIDDEN 15")
	}
	if !proviewUser.CheckReadPermission("hidden", "some/file.txt", "HIDDEN", true) {
		t.Error("Failed CheckReadPermission HIDDEN 16")
	}
	if !showingUser.CheckReadPermission("hidden", "some/file.txt", "HIDDEN", true) {
		t.Error("Failed CheckReadPermission HIDDEN 17")
	}
	if noRoleUser.CheckReadPermission("hidden", "some/file.txt", "HIDDEN", true) {
		t.Error("Failed CheckReadPermission HIDDEN 18")
	}

	if !adminUser.CheckReadPermission("hidden", "", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 19")
	}
	if !managerUser.CheckReadPermission("hidden", "", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 20")
	}
	if !allviewUser.CheckReadPermission("hidden", "", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 21")
	}
	if !proviewUser.CheckReadPermission("hidden", "", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 22")
	}
	if !showingUser.CheckReadPermission("hidden", "", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 23")
	}
	if noRoleUser.CheckReadPermission("hidden", "", "HIDDEN", false) {
		t.Error("Failed CheckReadPermission HIDDEN 24")
	}
}

func TestCheckReadPermissionInvalid(t *testing.T) {
	adminUser := createUserWithRoles([]string{"admin"})
	if adminUser.CheckReadPermission("releases", "path", "UNKNOWN", false) {
		t.Error("Failed CheckReadPermission Invalid 1")
	}
}

func TestCheckReadPermissionSpecificRepo(t *testing.T) {
	canviewPrivateUser := createUserWithRoles([]string{"canview:private"})
	canupdatePrivateUser := createUserWithRoles([]string{"canupdate:private"})
	canviewAllUser := createUserWithRoles([]string{"canview:*"})
	canupdateAllUser := createUserWithRoles([]string{"canupdate:*"})

	if !canviewPrivateUser.CheckReadPermission("private", "path", "PRIVATE", false) {
		t.Error("canview:private should allow reading private repo")
	}
	if canviewPrivateUser.CheckReadPermission("releases-private", "path", "PRIVATE", false) {
		t.Error("canview:private should NOT allow reading another private repo")
	}

	if !canupdatePrivateUser.CheckReadPermission("private", "path", "PRIVATE", false) {
		t.Error("canupdate:private should allow reading private repo")
	}
	if canupdatePrivateUser.CheckReadPermission("releases-private", "path", "PRIVATE", false) {
		t.Error("canupdate:private should NOT allow reading another private repo")
	}

	if !canviewAllUser.CheckReadPermission("private", "path", "PRIVATE", false) {
		t.Error("canview:* should allow reading private repo")
	}

	if !canupdateAllUser.CheckReadPermission("private", "path", "PRIVATE", false) {
		t.Error("canupdate:* should allow reading private repo")
	}
}

func TestCheckUpdatePermission(t *testing.T) {
	user1 := User{Roles: []string{"canupdate:*"}}
	if !user1.CheckUpdatePermission("any_repo") {
		t.Error("Failed CheckUpdatePermission 1")
	}

	user2 := User{Roles: []string{"canupdate:releases"}}
	if !user2.CheckUpdatePermission("releases") {
		t.Error("Failed CheckUpdatePermission 2")
	}
	if user2.CheckUpdatePermission("snapshots") {
		t.Error("Failed CheckUpdatePermission 3")
	}

	user3 := User{Roles: []string{}}
	if user3.CheckUpdatePermission("releases") {
		t.Error("Failed CheckUpdatePermission 4")
	}

	user4 := User{Roles: []string{"admin", "manager", "random_role"}}
	if !user4.CheckUpdatePermission("releases") {
		t.Error("Failed CheckUpdatePermission 5")
	}

	user5 := User{Roles: []string{"canupdate", "canupdate:releases:", "canupdate::*"}}
	if user5.CheckUpdatePermission("releases") {
		t.Error("Failed CheckUpdatePermission 6")
	}

	user6 := User{Roles: []string{"admin", "canupdate:snapshots", "canupdate:releases"}}
	if !user6.CheckUpdatePermission("releases") {
		t.Error("Failed CheckUpdatePermission 7")
	}
	if !user6.CheckUpdatePermission("snapshots") {
		t.Error("Failed CheckUpdatePermission 8")
	}
	if !user6.CheckUpdatePermission("other") {
		t.Error("Failed CheckUpdatePermission 9")
	}
}

func TestCheckModeratePermissionIsRepositoryScoped(t *testing.T) {
	moderator := createUserWithRoles([]string{"canmoderate:releases"})
	if !moderator.CheckModeratePermission("releases") {
		t.Fatal("repository moderator could not moderate the assigned repository")
	}
	if moderator.CheckModeratePermission("snapshots") {
		t.Fatal("repository moderator could moderate an unassigned repository")
	}
	if moderator.IsManager() || moderator.CheckUpdatePermission("releases") {
		t.Fatal("repository moderator inherited system or deployment permission")
	}
	if !moderator.CheckReadPermission("releases", "private/package", "PRIVATE", false) {
		t.Fatal("repository moderator could not inspect private review content")
	}
	if moderator.CheckReadPermission("snapshots", "private/package", "PRIVATE", false) {
		t.Fatal("repository moderator could inspect another private repository")
	}

	globalModerator := createUserWithRoles([]string{"canmoderate:*"})
	if !globalModerator.CheckModeratePermission("releases") ||
		!globalModerator.CheckModeratePermission("snapshots") {
		t.Fatal("global moderator did not cover every repository")
	}
	if globalModerator.IsManager() || globalModerator.CheckUpdatePermission("releases") {
		t.Fatal("global moderator inherited system or deployment permission")
	}

	manager := createUserWithRoles([]string{"manager"})
	if !manager.CheckModeratePermission("releases") {
		t.Fatal("system manager did not inherit moderation permission")
	}
}

func TestGetCacheConfigNoMirrors(t *testing.T) {
	repo := Repository{
		Name:              "test",
		Visibility:        "PUBLIC",
		Mirrors:           []Mirror{},
		AllowRedeployment: false,
	}
	persist, ttl := repo.GetCacheConfig()
	if !persist || ttl != 0 {
		t.Errorf("Expected true, 0; got %v, %v", persist, ttl)
	}
}

func TestGetCacheConfigWithMirrorsPersistOnly(t *testing.T) {
	repo := Repository{
		Name:       "test",
		Visibility: "PUBLIC",
		Mirrors: []Mirror{
			{
				URL:           "http://example.com",
				Persist:       true,
				CacheTTLSecs:  100,
				NegativeCache: true,
				TimeoutSecs:   30,
				Authorization: nil,
			},
		},
		AllowRedeployment: false,
	}
	persist, ttl := repo.GetCacheConfig()
	if !persist || ttl != 0 {
		t.Errorf("Expected true, 0; got %v, %v", persist, ttl)
	}
}

func TestGetCacheConfigWithMirrorsNoPersist(t *testing.T) {
	repo := Repository{
		Name:       "test3",
		Visibility: "PUBLIC",
		Mirrors: []Mirror{
			{
				URL:           "http://example.com",
				Persist:       false,
				CacheTTLSecs:  100,
				NegativeCache: true,
				TimeoutSecs:   30,
				Authorization: nil,
			},
			{
				URL:           "http://example.com/2",
				Persist:       false,
				CacheTTLSecs:  200,
				NegativeCache: true,
				TimeoutSecs:   30,
				Authorization: nil,
			},
		},
		AllowRedeployment: false,
	}
	persist, ttl := repo.GetCacheConfig()
	if persist || ttl != 200 {
		t.Errorf("Expected false, 200; got %v, %v", persist, ttl)
	}
}

func TestGetCacheConfigWithMirrorsMixed(t *testing.T) {
	repo := Repository{
		Name:       "test4",
		Visibility: "PUBLIC",
		Mirrors: []Mirror{
			{
				URL:           "http://example.com",
				Persist:       false,
				CacheTTLSecs:  100,
				NegativeCache: true,
				TimeoutSecs:   30,
				Authorization: nil,
			},
			{
				URL:           "http://example.com/2",
				Persist:       true,
				CacheTTLSecs:  200,
				NegativeCache: true,
				TimeoutSecs:   30,
				Authorization: nil,
			},
			{
				URL:           "http://example.com/3",
				Persist:       false,
				CacheTTLSecs:  300,
				NegativeCache: true,
				TimeoutSecs:   30,
				Authorization: nil,
			},
		},
		AllowRedeployment: false,
	}
	persist, ttl := repo.GetCacheConfig()
	if !persist || ttl != 300 {
		t.Errorf("Expected true, 300; got %v, %v", persist, ttl)
	}
}
