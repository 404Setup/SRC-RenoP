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
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func newMavenDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "maven.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	for _, username := range []string{"alice", "bob"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: username, CreatedAt: time.Now().Format(time.RFC3339)}))
	}
	return db
}

func TestMavenDomainOwnershipAndCatalog(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	domain := &core.MavenDomain{
		Repository: "releases", Domain: "com.example", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "example.com", VerificationCode: "renop-verification=test", CreatedAt: now,
	}
	require.NoError(t, db.CreateMavenDomain(domain, "alice"))
	assert.Empty(t, domain.Repository)
	assert.ErrorIs(t, db.CreateMavenDomain(domain, "alice"), core.ErrMavenDomainExists)

	publicPending, err := db.ListMavenDomains("guest", false)
	require.NoError(t, err)
	assert.Empty(t, publicPending)
	aliceDomains, err := db.ListMavenDomains("alice", false)
	require.NoError(t, err)
	require.Len(t, aliceDomains, 1)
	assert.True(t, aliceDomains[0].Member)
	assert.Equal(t, core.MavenPermissionOwner, aliceDomains[0].PermissionLevel)
	require.NoError(t, db.ReserveMavenVerificationAttempt("com.example", "alice", false, now, now-5000))
	assert.ErrorIs(t, db.ReserveMavenVerificationAttempt(
		"com.example", "alice", false, now+4999, now-1,
	), core.ErrMavenVerificationRateLimit)
	require.NoError(t, db.ReserveMavenVerificationAttempt(
		"com.example", "alice", false, now+5000, now,
	))

	require.NoError(t, db.MarkMavenDomainVerified("com.example", domain.VerificationCode, now+1))
	publicDomains, err := db.ListMavenDomains("guest", false)
	require.NoError(t, err)
	require.Len(t, publicDomains, 1)
	assert.True(t, publicDomains[0].Verified)
	assert.False(t, publicDomains[0].Member)
	assert.Equal(t, 1, publicDomains[0].MemberCount)
	assert.Zero(t, publicDomains[0].RepositoryCount)

	duplicate := &core.MavenDomain{Repository: "snapshots", Domain: "com.example", VerificationHost: "example.com", VerificationCode: "other"}
	assert.ErrorIs(t, db.CreateMavenDomain(duplicate, "bob"), core.ErrMavenDomainExists)

	require.NoError(t, db.ForceAddMavenMembers("com.example", "admin", []string{"bob"}, core.MavenPermissionRead))
	assert.ErrorIs(t, db.ForceAddMavenMembers("com.example", "admin", []string{"bob"}, core.MavenPermissionRead), core.ErrMavenMemberExists)
	_, err = db.SQLDB.Exec(`INSERT INTO user_profiles
		(user_id, username, nickname, rename_window_started_at, rename_count, updated_at)
		VALUES (?, ?, '', 0, 0, ?)`, "00000000-0000-0000-0000-000000000099", "ghost", now)
	require.NoError(t, err)
	assert.ErrorIs(t, db.ForceAddMavenMembers("com.example", "admin", []string{"ghost"}, core.MavenPermissionRead), core.ErrUserProfileNotFound)
	_, err = db.SQLDB.Exec(`INSERT INTO maven_domain_members
		(repository, domain, username, user_id, permission_level, added_at) VALUES ('', ?, ?, ?, ?, ?)`,
		"com.example", "ghost", "00000000-0000-0000-0000-000000000099", core.MavenPermissionRead, now)
	require.NoError(t, err)
	require.NoError(t, db.RemoveMavenMember("com.example", "alice", "ghost"))
	require.NoError(t, db.SetMavenMemberLevel("com.example", "alice", "bob", core.MavenPermissionOwner))
	details, err := db.GetMavenDomainDetails("com.example", "bob")
	require.NoError(t, err)
	levels := map[string]int{}
	for _, member := range details.Members {
		levels[member.Username] = member.Level
	}
	assert.Equal(t, core.MavenPermissionOwner, levels["bob"])
	assert.Equal(t, core.MavenPermissionRead, levels["alice"])
	assert.Equal(t, 2, details.Domain.MemberCount)
	require.NoError(t, db.RemoveMavenMember("com.example", "alice", "alice"))
	assert.ErrorIs(t, db.RemoveMavenMember("com.example", "bob", "bob"), core.ErrMavenOwnerCannotLeave)

	artifact := &core.MavenArtifact{
		Repository: "releases", Domain: "com.example", GroupID: "com.example.tools",
		ArtifactID: "demo", Description: "Demo", Publisher: "bob", LatestVersion: "1.0.0",
		CreatedAt: now, UpdatedAt: now,
	}
	version := &core.MavenVersion{
		Repository: "releases", GroupID: artifact.GroupID, ArtifactID: artifact.ArtifactID,
		Version: "1.0.0", Publisher: "bob", Size: 1024, CreatedAt: now,
	}
	require.NoError(t, db.RecordMavenPublication(artifact, version))
	artifacts, total, err := db.ListMavenArtifacts("releases", "com.example", "demo", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, artifacts, 1)
	assert.Equal(t, "1.0.0", artifacts[0].LatestVersion)
	assert.Equal(t, int64(1024), artifacts[0].TotalSize)
	artifactDetails, err := db.GetMavenArtifactDetails("releases", artifact.GroupID, artifact.ArtifactID)
	require.NoError(t, err)
	require.Len(t, artifactDetails.Versions, 1)
	for _, candidate := range []string{"10.0.0", "2.0.0"} {
		artifact.LatestVersion = candidate
		artifact.UpdatedAt++
		version.Version = candidate
		version.CreatedAt++
		require.NoError(t, db.RecordMavenPublication(artifact, version))
	}
	artifactDetails, err = db.GetMavenArtifactDetails("releases", artifact.GroupID, artifact.ArtifactID)
	require.NoError(t, err)
	require.Len(t, artifactDetails.Versions, 3)
	assert.Equal(t, "10.0.0", artifactDetails.Artifact.LatestVersion)
	assert.Equal(t, int64(3*1024), artifactDetails.Artifact.TotalSize)
	assert.Equal(t, []string{"10.0.0", "2.0.0", "1.0.0"}, []string{
		artifactDetails.Versions[0].Version, artifactDetails.Versions[1].Version, artifactDetails.Versions[2].Version,
	})
	require.NoError(t, db.DeleteMavenVersionMetadata("releases", artifact.GroupID, artifact.ArtifactID, "10.0.0"))
	artifactDetails, err = db.GetMavenArtifactDetails("releases", artifact.GroupID, artifact.ArtifactID)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", artifactDetails.Artifact.LatestVersion)
	require.NoError(t, db.DeleteMavenVersionMetadata("releases", artifact.GroupID, artifact.ArtifactID, "2.0.0"))
	require.NoError(t, db.DeleteMavenVersionMetadata("releases", artifact.GroupID, artifact.ArtifactID, "1.0.0"))
	_, err = db.GetMavenArtifactDetails("releases", artifact.GroupID, artifact.ArtifactID)
	assert.True(t, errors.Is(err, core.ErrMavenArtifactNotFound))
	require.NoError(t, db.DeleteMavenRepository("releases"))
	_, err = db.GetMavenDomainDetails("com.example", "bob")
	require.NoError(t, err)
}

func TestMavenArtifactMovesToMostSpecificDomain(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	for _, candidate := range []struct {
		domain string
		owner  string
	}{
		{domain: "com.example", owner: "alice"},
		{domain: "com.example.tools", owner: "bob"},
	} {
		domain := &core.MavenDomain{
			Domain: candidate.domain, VerificationType: core.MavenVerificationDNS,
			VerificationHost: "example.com", VerificationCode: "renop-verification=" + candidate.domain,
			CreatedAt: now,
		}
		require.NoError(t, db.CreateMavenDomain(domain, candidate.owner))
		require.NoError(t, db.MarkMavenDomainVerified(candidate.domain, domain.VerificationCode, now))
	}

	artifact := &core.MavenArtifact{
		Repository: "releases", Domain: "com.example", GroupID: "com.example.tools",
		ArtifactID: "demo", Publisher: "alice", LatestVersion: "1.0.0", CreatedAt: now, UpdatedAt: now,
	}
	version := &core.MavenVersion{
		Repository: "releases", GroupID: artifact.GroupID, ArtifactID: artifact.ArtifactID,
		Version: artifact.LatestVersion, Publisher: artifact.Publisher, CreatedAt: now,
	}
	require.NoError(t, db.RecordMavenPublication(artifact, version))

	artifact.Domain = "com.example.tools"
	artifact.Publisher = "bob"
	artifact.UpdatedAt++
	require.NoError(t, db.RecordMavenPublication(artifact, version))

	parentArtifacts, parentTotal, err := db.ListMavenArtifacts("releases", "com.example", "", 10, 0)
	require.NoError(t, err)
	assert.Zero(t, parentTotal)
	assert.Empty(t, parentArtifacts)
	childArtifacts, childTotal, err := db.ListMavenArtifacts("releases", "com.example.tools", "", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, childTotal)
	require.Len(t, childArtifacts, 1)
	assert.Equal(t, "com.example.tools", childArtifacts[0].Domain)

	parentDetails, err := db.GetMavenDomainDetails("com.example", "alice")
	require.NoError(t, err)
	assert.Zero(t, parentDetails.Domain.ArtifactCount)
	childDetails, err := db.GetMavenDomainDetails("com.example.tools", "bob")
	require.NoError(t, err)
	assert.Equal(t, 1, childDetails.Domain.ArtifactCount)
	assert.Equal(t, 1, childDetails.Domain.RepositoryCount)
	assert.Equal(t, 1, childDetails.Domain.MemberCount)
	repositoryDomains, err := db.ListMavenRepositoryDomains("releases", "alice")
	require.NoError(t, err)
	require.Len(t, repositoryDomains, 1)
	assert.Equal(t, "com.example.tools", repositoryDomains[0].Domain)
	assert.Equal(t, 1, repositoryDomains[0].ArtifactCount)
}
