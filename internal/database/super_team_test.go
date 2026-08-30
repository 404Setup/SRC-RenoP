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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/core"
)

func TestSuperTeamLifecycleLimitsAndImmutableMemberships(t *testing.T) {
	db := newMavenDB(t)
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "charlie", CreatedAt: time.Now().Format(time.RFC3339)}))
	now := time.Now().UnixMilli()
	team := &core.SuperTeam{Prefix: "platform-team", Name: "Platform Team", Description: "Shared packages", CreatedAt: now}
	require.NoError(t, db.CreateSuperTeam(team, "alice", 1, 3))
	assert.Equal(t, core.SuperTeamRoleOwner, team.RoleLevel)
	assert.Equal(t, 1, team.MemberCount)
	require.ErrorIs(t, db.CreateSuperTeam(&core.SuperTeam{
		Prefix: "second-team", Name: "Second Team", CreatedAt: now + 1,
	}, "alice", 1, 3), core.ErrSuperTeamCreateLimit)

	teams, total, err := db.ListSuperTeams("alice", false, 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, teams, 1)
	assert.Equal(t, "platform-team", teams[0].Prefix)
	assert.Equal(t, core.SuperTeamRoleOwner, teams[0].RoleLevel)
	bobTeams, bobTotal, err := db.ListSuperTeams("bob", false, 10, 0)
	require.NoError(t, err)
	assert.Empty(t, bobTeams)
	assert.Zero(t, bobTotal)
	adminTeams, adminTotal, err := db.ListSuperTeams("admin", true, 10, 0)
	require.NoError(t, err)
	require.Len(t, adminTeams, 1)
	assert.Equal(t, 1, adminTotal)

	zero := 0
	require.NoError(t, db.SetSuperTeamLimitOverride("bob", nil, &zero, now+2))
	require.ErrorIs(t, db.ForceAddSuperTeamMembers("platform-team", "admin", []string{"bob"},
		core.SuperTeamRoleWrite, 1, 3, now+3), core.ErrSuperTeamJoinLimit)
	two := 2
	require.NoError(t, db.SetSuperTeamLimitOverride("bob", nil, &two, now+4))
	require.NoError(t, db.ForceAddSuperTeamMembers("platform-team", "admin", []string{"bob"},
		core.SuperTeamRoleWrite, 1, 3, now+5))

	details, err := db.GetSuperTeamDetails("platform-team", "bob", false)
	require.NoError(t, err)
	assert.Equal(t, core.SuperTeamRoleWrite, details.Team.RoleLevel)
	require.Len(t, details.Members, 2)
	require.NoError(t, db.SetSuperTeamMemberLevel("platform-team", "admin", "bob", core.SuperTeamRoleManage, true))
	require.ErrorIs(t, db.SetSuperTeamMemberLevel("platform-team", "bob", "alice",
		core.SuperTeamRoleRead, false), core.ErrSuperTeamPermissionDenied)

	status, err := db.GetSuperTeamLimitStatus("bob", 1, 3)
	require.NoError(t, err)
	assert.True(t, status.CreateLimitInherited)
	assert.False(t, status.JoinLimitInherited)
	assert.Equal(t, 2, status.JoinLimit)
	assert.Equal(t, 1, status.JoinedCount)
}

func TestSuperTeamInvitationAndOwnerInvariants(t *testing.T) {
	db := newMavenDB(t)
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "charlie", CreatedAt: time.Now().Format(time.RFC3339)}))
	now := time.Now().UnixMilli()
	require.NoError(t, db.CreateSuperTeam(&core.SuperTeam{
		Prefix: "release", Name: "Release", CreatedAt: now,
	}, "alice", 5, 5))
	id := "00000000-0000-4000-8000-000000000301"
	expiresAt := now + int64((7 * 24 * time.Hour / time.Millisecond))
	invitation := &core.SuperTeamInvitation{
		ID: id, TeamPrefix: "release", Inviter: "alice", Recipient: "charlie",
		Level: core.SuperTeamRoleRead, CreatedAt: now + 1, ExpiresAt: expiresAt,
	}
	message := &core.UserMessage{
		ID: id, Recipient: "charlie", Sender: "alice", Kind: "super_team_invite", Severity: "info",
		Title: "Global team invitation", Body: "Invitation", Payload: []byte(`{"prefix":"release"}`),
		ActionKind: "super_team_invite", ActionStatus: core.MessageActionPending,
		CreatedAt: now + 1, ExpiresAt: expiresAt,
	}
	require.NoError(t, db.CreateSuperTeamInvitations(
		[]*core.SuperTeamInvitation{invitation}, []*core.UserMessage{message}))
	require.ErrorIs(t, db.CreateSuperTeamInvitations(
		[]*core.SuperTeamInvitation{invitation}, []*core.UserMessage{message}), core.ErrSuperTeamInvitationExists)
	require.NoError(t, db.RespondSuperTeamInvitation(id, "charlie", true, 5, now+2))
	details, err := db.GetSuperTeamDetails("release", "charlie", false)
	require.NoError(t, err)
	assert.Equal(t, core.SuperTeamRoleRead, details.Team.RoleLevel)
	storedMessage, err := db.GetUserMessage(id, "charlie", now+2)
	require.NoError(t, err)
	require.NotNil(t, storedMessage)
	assert.Equal(t, core.MessageActionAccepted, storedMessage.ActionStatus)

	require.ErrorIs(t, db.SetSuperTeamMemberLevel("release", "admin", "alice",
		core.SuperTeamRoleManage, true), core.ErrSuperTeamLastOwner)
	require.NoError(t, db.SetSuperTeamMemberLevel("release", "admin", "charlie",
		core.SuperTeamRoleOwner, true))
	require.NoError(t, db.SetSuperTeamMemberLevel("release", "alice", "alice",
		core.SuperTeamRoleManage, false))
	require.ErrorIs(t, db.RemoveSuperTeamMember("release", "charlie", "charlie", false, now+3),
		core.ErrSuperTeamOwnerCannotLeave)
}

func TestSuperTeamBindingsEnforceNamespacesAndMapPermissions(t *testing.T) {
	db := newMavenDB(t)
	for _, username := range []string{"charlie", "dana"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{
			Name: username, CreatedAt: time.Now().Format(time.RFC3339),
		}))
	}
	now := time.Now().UnixMilli()
	require.NoError(t, db.CreateSuperTeam(&core.SuperTeam{
		Prefix: "platform", Name: "Platform", CreatedAt: now,
	}, "alice", 5, 10))
	require.NoError(t, db.ForceAddSuperTeamMembers("platform", "admin", []string{"bob"},
		core.SuperTeamRoleWrite, 5, 10, now+1))
	require.NoError(t, db.ForceAddSuperTeamMembers("platform", "admin", []string{"charlie"},
		core.SuperTeamRoleManage, 5, 10, now+2))
	require.NoError(t, db.ForceAddSuperTeamMembers("platform", "admin", []string{"dana"},
		core.SuperTeamRoleRead, 5, 10, now+3))

	_, err := db.CreateDockerImageForTeam("containers", "platform/api", "alice", "", true, now+4)
	require.ErrorIs(t, err, core.ErrSuperTeamBindingRequired)
	_, err = db.CreateDockerImageForTeam("containers", "platform/api", "alice", "another", true, now+4)
	require.ErrorIs(t, err, core.ErrSuperTeamBindingMismatch)
	_, err = db.CreateDockerImageForTeam("containers", "plain", "bob", "platform", false, now+4)
	require.ErrorIs(t, err, core.ErrSuperTeamBindingPermission)
	image, err := db.CreateDockerImageForTeam("containers", "platform/api", "alice", "platform", true, now+4)
	require.NoError(t, err)
	assert.Equal(t, "platform", image.SuperTeamPrefix)
	exists, private, _, member, level, err := db.GetDockerImageAccess("containers", "platform/api", "bob")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.True(t, private)
	assert.True(t, member)
	assert.Equal(t, core.DockerPermissionManage, level)
	_, _, _, member, level, err = db.GetDockerImageAccess("containers", "platform/api", "dana")
	require.NoError(t, err)
	assert.True(t, member)
	assert.Equal(t, core.DockerPermissionRead, level)
	dockerMembers, err := db.ListDockerMembers("containers", "platform/api")
	require.NoError(t, err)
	require.Len(t, dockerMembers, 1, "mapped global-team members must not be copied into package teams")

	_, err = db.CreateNPMPackageForTeam("npm", "@platform/tool", "alice", "", true, now+5)
	require.ErrorIs(t, err, core.ErrSuperTeamBindingRequired)
	npmPackage, err := db.CreateNPMPackageForTeam(
		"npm", "@platform/tool", "alice", "platform", true, now+5)
	require.NoError(t, err)
	assert.Equal(t, "platform", npmPackage.SuperTeamPrefix)
	_, private, _, member, level, err = db.GetNPMPackageAccess("npm", "@platform/tool", "bob")
	require.NoError(t, err)
	assert.True(t, private)
	assert.True(t, member)
	assert.Equal(t, core.NPMPermissionLifecycle, level)
	npmMembers, err := db.ListNPMMembers("npm", "@platform/tool")
	require.NoError(t, err)
	require.Len(t, npmMembers, 1)

	domain := &core.MavenDomain{
		Domain: "com.example", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "example.com", VerificationCode: "verify-platform", SuperTeamPrefix: "platform",
		Verified: true, CreatedAt: now + 6, VerifiedAt: now + 6,
	}
	require.NoError(t, db.CreateMavenDomain(domain, "alice"))
	domainDetails, err := db.GetMavenDomainDetails("com.example", "bob")
	require.NoError(t, err)
	assert.Equal(t, "platform", domainDetails.Domain.SuperTeamPrefix)
	assert.True(t, domainDetails.Domain.Member)
	assert.Equal(t, core.MavenPermissionVersion, domainDetails.Domain.PermissionLevel)
	require.Len(t, domainDetails.Members, 1)

	require.NoError(t, db.RecordMavenPublication(&core.MavenArtifact{
		Repository: "releases", Domain: "com.example", GroupID: "com.example", ArtifactID: "demo",
		SuperTeamPrefix: "platform", CreatedAt: now + 7, UpdatedAt: now + 7,
	}, &core.MavenVersion{Version: "1.0.0", Publisher: "alice", CreatedAt: now + 7}))
	prefix, artifactMember, artifactLevel, err := db.GetMavenArtifactTeamAccess(
		"releases", "com.example", "demo", "bob")
	require.NoError(t, err)
	assert.Equal(t, "platform", prefix)
	assert.True(t, artifactMember)
	assert.Equal(t, core.MavenPermissionVersion, artifactLevel)

	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{
		Repository: "cargo", Name: "demo", NormalizedName: "demo", CreatedAt: now + 8, UpdatedAt: now + 8,
	}, &core.CargoVersion{Version: "1.0.0", CreatedAt: now + 8}, "alice"))
	_, err = db.Exec(`UPDATE cargo_packages SET super_team_prefix = ? WHERE repository = ? AND normalized_name = ?`,
		"platform", "cargo", "demo")
	require.NoError(t, err)
	cargoDetails, err := db.GetCargoPackageDetails("cargo", "demo", "bob")
	require.NoError(t, err)
	assert.Equal(t, "platform", cargoDetails.Package.SuperTeamPrefix)
	assert.Equal(t, core.CargoPermissionVersion, cargoDetails.Package.PermissionLevel)

	manageable, total, err := db.ListManageableSuperTeams("charlie", core.SuperTeamRoleManage, 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, manageable, 1)
	_, total, err = db.ListManageableSuperTeams("bob", core.SuperTeamRoleManage, 10, 0)
	require.NoError(t, err)
	assert.Zero(t, total)
	require.ErrorIs(t, db.DeleteSuperTeam("platform", "alice", false, now+9), core.ErrSuperTeamNotEmpty)
}

func TestAccountDeletionPreservesSuperTeamOwnershipAndCreatorDisplay(t *testing.T) {
	db := newMavenDB(t)
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "charlie", CreatedAt: time.Now().Format(time.RFC3339)}))
	now := time.Now().UnixMilli()
	require.NoError(t, db.CreateSuperTeam(&core.SuperTeam{
		Prefix: "durable", Name: "Durable", CreatedAt: now,
	}, "alice", 5, 5))
	require.ErrorContains(t, db.DeleteToken("alice"), "last T4 owner")
	require.NoError(t, db.ForceAddSuperTeamMembers("durable", "admin", []string{"charlie"},
		core.SuperTeamRoleOwner, 5, 5, now+1))
	require.NoError(t, db.DeleteToken("alice"))
	details, err := db.GetSuperTeamDetails("durable", "charlie", false)
	require.NoError(t, err)
	assert.Equal(t, "alice", details.Team.CreatedBy)
	require.Len(t, details.Members, 1)
	assert.Equal(t, "charlie", details.Members[0].Username)
}
