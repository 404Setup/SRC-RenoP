/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
)

func TestSuperTeamResourcesAreBoundedAndPrivateAware(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	require.NoError(t, db.CreateSuperTeam(&core.SuperTeam{
		Prefix: "platform", Name: "Platform", CreatedAt: now,
	}, "alice", 2, 4))
	require.NoError(t, db.CreateMavenDomain(&core.MavenDomain{
		Domain: "org.example", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "example.org", VerificationCode: "renop=platform",
		SuperTeamPrefix: "platform", CreatedAt: now,
	}, "alice"))
	require.NoError(t, db.MarkMavenDomainVerified("org.example", "renop=platform", now+1))
	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{
		Repository: "cargo-public", Name: "platform-crate", NormalizedName: "platform-crate",
		Description: "Cargo resource", SuperTeamPrefix: "platform", CreatedAt: now, UpdatedAt: now,
	}, &core.CargoVersion{
		Repository: "cargo-public", Package: "platform-crate", Version: "1.0.0",
		Publisher: "alice", CreatedAt: now,
	}, "alice"))
	_, err := db.Exec(`UPDATE cargo_packages SET super_team_prefix = ? WHERE repository = ? AND normalized_name = ?`,
		"platform", "cargo-public", "platform-crate")
	require.NoError(t, err)
	_, err = db.CreateDockerImageForTeam("docker-private", "platform/image", "alice", "platform", true, now)
	require.NoError(t, err)
	_, err = db.CreateNPMPackageForTeam("npm-public", "@platform/public", "alice", "platform", false, now)
	require.NoError(t, err)
	_, err = db.CreateNPMPackageForTeam("npm-public", "@platform/private", "alice", "platform", true, now+1)
	require.NoError(t, err)

	resources, total, err := db.ListSuperTeamResources(core.SuperTeamResourceListOptions{
		Prefix: "platform", Format: config.RepositoryFormatMaven, Limit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, resources, 1)
	require.Equal(t, "org.example", resources[0].Name)
	resources, total, err = db.ListSuperTeamResources(core.SuperTeamResourceListOptions{
		Prefix: "platform", Format: config.RepositoryFormatCargo,
		VisibleRepositories: []string{"cargo-public"}, Limit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "Cargo resource", resources[0].Description)
	resources, total, err = db.ListSuperTeamResources(core.SuperTeamResourceListOptions{
		Prefix: "platform", Format: config.RepositoryFormatDocker,
		VisibleRepositories: []string{"docker-private"}, Limit: 10,
	})
	require.NoError(t, err)
	require.Empty(t, resources)
	require.Zero(t, total)
	resources, total, err = db.ListSuperTeamResources(core.SuperTeamResourceListOptions{
		Prefix: "platform", Format: config.RepositoryFormatNPM,
		VisibleRepositories: []string{"npm-public"}, Limit: 1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, resources, 1)
	require.Equal(t, "@platform/public", resources[0].Name)

	require.NoError(t, db.ForceAddSuperTeamMembers("platform", "admin", []string{"bob"},
		core.SuperTeamRoleRead, 2, 4, now+2))
	resources, total, err = db.ListSuperTeamResources(core.SuperTeamResourceListOptions{
		Prefix: "platform", Format: config.RepositoryFormatDocker, Viewer: "bob", Limit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, resources, 1)
	resources, total, err = db.ListSuperTeamResources(core.SuperTeamResourceListOptions{
		Prefix: "platform", Format: config.RepositoryFormatNPM, Viewer: "bob",
		VisibleRepositories: []string{"npm-public"}, Limit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, resources, 2)
}
