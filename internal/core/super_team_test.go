/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSuperTeamPrefixAndPermissionMapping(t *testing.T) {
	t.Parallel()
	for input, expected := range map[string]string{
		"Team-One": "team-one",
		"npm_team": "npm_team",
		"a1":       "a1",
	} {
		actual, valid := NormalizeSuperTeamPrefix(input)
		assert.True(t, valid, input)
		assert.Equal(t, expected, actual)
	}
	for _, invalid := range []string{"a", "-team", "team-", "team/name", "team name", "@team"} {
		_, valid := NormalizeSuperTeamPrefix(invalid)
		assert.False(t, valid, invalid)
	}
	assert.Equal(t, 0, SuperTeamPackagePermission(SuperTeamRoleRead))
	assert.Equal(t, 2, SuperTeamPackagePermission(SuperTeamRoleWrite))
	assert.Equal(t, 3, SuperTeamPackagePermission(SuperTeamRoleManage))
	assert.Equal(t, 4, SuperTeamPackagePermission(SuperTeamRoleOwner))
	assert.Equal(t, -1, SuperTeamPackagePermission(0))
	dockerPrefix, namespaced := DockerImageSuperTeamPrefix("Platform/API")
	assert.True(t, namespaced)
	assert.Equal(t, "platform", dockerPrefix)
	_, namespaced = DockerImageSuperTeamPrefix("standalone")
	assert.False(t, namespaced)
	npmPrefix, scoped := NPMPackageSuperTeamPrefix("@Platform/tool")
	assert.True(t, scoped)
	assert.Equal(t, "platform", npmPrefix)
	_, scoped = NPMPackageSuperTeamPrefix("tool")
	assert.False(t, scoped)
}
