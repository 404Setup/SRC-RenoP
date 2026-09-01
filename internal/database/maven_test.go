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
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/testutil"
)

func newMavenDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "maven.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	for _, username := range []string{"alice", "bob", "admin"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: username, CreatedAt: time.Now().Format(time.RFC3339)}))
	}
	return db
}

func TestMavenDomainLifecycleColumnsMigrateFromLegacySchema(t *testing.T) {
	databasePath := filepath.Join(testutil.TempDir(t), "legacy-maven-domain.db")
	rawDB, err := sql.Open("sqlite", databasePath)
	require.NoError(t, err)
	_, err = rawDB.Exec(`CREATE TABLE maven_domains (
		repository VARCHAR(64) NOT NULL, domain VARCHAR(253) NOT NULL,
		verification_type VARCHAR(16) NOT NULL, verification_host VARCHAR(253) NOT NULL,
		verification_code VARCHAR(128) NOT NULL, super_team_prefix VARCHAR(64) NOT NULL DEFAULT '',
		verified INT NOT NULL DEFAULT 0, created_at BIGINT NOT NULL,
		verified_at BIGINT NOT NULL DEFAULT 0, last_check_at BIGINT NOT NULL DEFAULT 0,
		PRIMARY KEY (repository, domain)
	)`)
	require.NoError(t, err)
	_, err = rawDB.Exec(`INSERT INTO maven_domains
		(repository, domain, verification_type, verification_host, verification_code, verified, created_at)
		VALUES ('', 'com.legacy', 'dns', 'legacy.com', 'legacy-code', 1, 1)`)
	require.NoError(t, err)
	require.NoError(t, rawDB.Close())

	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: databasePath, MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	details, err := db.GetMavenDomainDetails("com.legacy", "")
	require.NoError(t, err)
	assert.True(t, details.Domain.Verified)
	assert.Zero(t, details.Domain.ClosedAt)
	assert.Zero(t, details.Domain.ReleaseAt)
	assert.Empty(t, details.Domain.ClaimStatus)
	assert.Zero(t, details.Domain.ClaimVerifiedAt)
}

func TestManagedMavenDomainsFilterPaginationAndMirrorClaim(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	for _, name := range []string{"com.alpha", "net.alpha"} {
		record := &core.MavenDomain{
			Domain: name, VerificationType: core.MavenVerificationDNS,
			VerificationHost: strings.TrimPrefix(name, strings.Split(name, ".")[0]+"."),
			VerificationCode: "renop-verification=" + name, CreatedAt: now,
		}
		require.NoError(t, db.CreateMavenDomain(record, "alice"))
	}
	require.NoError(t, db.MarkMavenDomainVerified("com.alpha", "renop-verification=com.alpha", now))
	require.NoError(t, db.EnsureMirroredMavenDomain("org.mirror", now))
	require.NoError(t, db.EnsureMirroredMavenDomain("org.mirror", now))

	page, total, err := db.ListManagedMavenDomains(core.MavenDomainListOptions{
		Username: "alice", PermissionLevels: []int{core.MavenPermissionOwner},
		Limit: 1, Filtered: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, page, 1)
	assert.Equal(t, "com.alpha", page[0].Domain)

	page, total, err = db.ListManagedMavenDomains(core.MavenDomainListOptions{
		Username: "admin", Administrator: true, IncludeMirrored: true, Filtered: true, Limit: 20,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, page, 1)
	assert.Equal(t, core.MavenVerificationMirror, page[0].VerificationType)
	assert.False(t, page[0].Verified)
	assert.False(t, page[0].Member)

	claim := &core.MavenDomain{
		Domain: "org.mirror", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "mirror.org", VerificationCode: "renop-verification=claim", CreatedAt: now + 1,
	}
	require.NoError(t, db.CreateMavenDomain(claim, "bob"))
	details, err := db.GetMavenDomainDetails("org.mirror", "bob")
	require.NoError(t, err)
	assert.True(t, details.Domain.Member)
	assert.Equal(t, core.MavenPermissionOwner, details.Domain.PermissionLevel)
	assert.Equal(t, core.MavenVerificationDNS, details.Domain.VerificationType)
	assert.Equal(t, "renop-verification=claim", details.Domain.VerificationCode)
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
	requireTeamRemovalMessage(t, db, "ghost", "maven", "", "com.example", "alice")
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
	requireNoTeamRemovalMessage(t, db, "alice")
	assert.ErrorIs(t, db.RemoveMavenMember("com.example", "bob", "bob"), core.ErrMavenOwnerCannotLeave)

	artifact := &core.MavenArtifact{
		Repository: "releases", Domain: "com.example", GroupID: "com.example.tools",
		ArtifactID: "demo", Description: "Demo", Readme: "# Demo\n\nMaven **README**.", Publisher: "bob", LatestVersion: "1.0.0",
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
	assert.Empty(t, artifacts[0].Readme, "catalog pages must not load large README bodies")
	assert.Equal(t, "1.0.0", artifacts[0].LatestVersion)
	assert.Equal(t, int64(1024), artifacts[0].TotalSize)
	artifactDetails, err := db.GetMavenArtifactDetails("releases", artifact.GroupID, artifact.ArtifactID)
	require.NoError(t, err)
	require.Len(t, artifactDetails.Versions, 1)
	assert.Equal(t, "# Demo\n\nMaven **README**.", artifactDetails.Artifact.Readme)
	require.NoError(t, db.UpdateMavenArtifactReadme("releases", artifact.GroupID, artifact.ArtifactID, "# Updated"))
	artifactDetails, err = db.GetMavenArtifactDetails("releases", artifact.GroupID, artifact.ArtifactID)
	require.NoError(t, err)
	assert.Equal(t, "# Updated", artifactDetails.Artifact.Readme)
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

func TestMavenDomainCloseReleaseAndReviewedReclaim(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	domain := &core.MavenDomain{
		Domain: "com.lifecycle", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "lifecycle.com", VerificationCode: "renop-verification=alice",
		CreatedAt: now,
	}
	require.NoError(t, db.CreateMavenDomain(domain, "alice"))
	require.NoError(t, db.MarkMavenDomainVerified(domain.Domain, domain.VerificationCode, now+1))
	require.NoError(t, db.RecordMavenPublication(&core.MavenArtifact{
		Repository: "releases", Domain: domain.Domain, GroupID: domain.Domain, ArtifactID: "demo",
		CreatedAt: now + 2, UpdatedAt: now + 2,
	}, &core.MavenVersion{
		Repository: "releases", GroupID: domain.Domain, ArtifactID: "demo", Version: "1.0.0",
		Publisher: "alice", CreatedAt: now + 2,
	}))
	closedAt := now + 3
	require.NoError(t, db.CloseMavenDomain(domain.Domain, "alice", false, closedAt))
	details, err := db.GetMavenDomainDetails(domain.Domain, "alice")
	require.NoError(t, err)
	assert.False(t, details.Domain.Verified)
	assert.Equal(t, closedAt, details.Domain.ClosedAt)
	assert.Equal(t, closedAt+core.MavenDomainReleaseLockMillis, details.Domain.ReleaseAt)

	claim := &core.MavenDomain{
		Domain: domain.Domain, VerificationType: core.MavenVerificationDNS,
		VerificationHost: "lifecycle.com", VerificationCode: "renop-verification=bob",
		CreatedAt: details.Domain.ReleaseAt - 1,
	}
	require.ErrorIs(t, db.CreateMavenDomain(claim, "bob"), core.ErrMavenDomainLocked)
	claim.CreatedAt = details.Domain.ReleaseAt
	require.NoError(t, db.CreateMavenDomain(claim, "bob"))
	details, err = db.GetMavenDomainDetails(domain.Domain, "bob")
	require.NoError(t, err)
	assert.False(t, details.Domain.Verified)
	assert.Equal(t, core.MavenDomainClaimAwaitingVerification, details.Domain.ClaimStatus)
	require.Len(t, details.Members, 1)
	assert.Equal(t, "bob", details.Members[0].Username)
	artifact, err := db.GetMavenArtifactDetails("releases", domain.Domain, "demo")
	require.NoError(t, err)
	require.NotNil(t, artifact.Artifact)

	require.NoError(t, db.MarkMavenDomainVerified(domain.Domain, claim.VerificationCode, claim.CreatedAt+1))
	details, err = db.GetMavenDomainDetails(domain.Domain, "bob")
	require.NoError(t, err)
	assert.False(t, details.Domain.Verified)
	assert.Equal(t, core.MavenDomainClaimPending, details.Domain.ClaimStatus)
	require.NoError(t, db.ReviewMavenDomainClaim(domain.Domain, core.ReviewStatusRejected, claim.CreatedAt+2))
	require.NoError(t, db.MarkMavenDomainVerified(domain.Domain, claim.VerificationCode, claim.CreatedAt+3))
	require.NoError(t, db.ReviewMavenDomainClaim(domain.Domain, core.ReviewStatusApproved, claim.CreatedAt+4))
	details, err = db.GetMavenDomainDetails(domain.Domain, "bob")
	require.NoError(t, err)
	assert.True(t, details.Domain.Verified)
	assert.Empty(t, details.Domain.ClaimStatus)
	assert.Equal(t, claim.CreatedAt+4, details.Domain.VerifiedAt)

	pendingDomain := &core.MavenDomain{
		Domain: "com.pending", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "pending.com", VerificationCode: "renop-verification=pending",
		CreatedAt: claim.CreatedAt + 5,
	}
	require.NoError(t, db.CreateMavenDomain(pendingDomain, "alice"))
	_, err = db.CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceMavenArtifact, Repository: "releases",
		ResourceKey: "com.pending:demo", ResourceName: "com.pending:demo", Version: "1.0.0",
		RequestedBy: "alice", Policy: config.PublicationReviewEveryVersion,
		Files:     []*core.ReviewFile{{Path: "com/pending/demo/1.0/demo-1.0.jar", Size: 10}},
		CreatedAt: claim.CreatedAt + 6,
	})
	require.NoError(t, err)
	require.ErrorIs(t, db.CloseMavenDomain(pendingDomain.Domain, "alice", false, claim.CreatedAt+7),
		core.ErrMavenDomainReviewPending)
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

func TestMavenMirrorPublicationRetainsVersionProvenance(t *testing.T) {
	db := newMavenDB(t)
	const now int64 = 4000
	artifact := &core.MavenArtifact{
		Repository: "central", Domain: "com.example", GroupID: "com.example", ArtifactID: "demo",
		LatestVersion: "1.0.0", CreatedAt: now, UpdatedAt: now,
	}
	version := &core.MavenVersion{
		Repository: "central", GroupID: "com.example", ArtifactID: "demo",
		Version: "1.0.0", Size: 4096, CreatedAt: now,
	}
	require.NoError(t, db.RecordMavenMirrorPublication(artifact, version))
	details, err := db.GetMavenArtifactDetails("central", "com.example", "demo")
	require.NoError(t, err)
	assert.True(t, details.Artifact.Mirrored)
	require.Len(t, details.Versions, 1)
	assert.True(t, details.Versions[0].Mirrored)
	artifacts, total, err := db.ListMavenArtifacts("central", "", "", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, artifacts, 1)
	assert.True(t, artifacts[0].Mirrored)
	require.NoError(t, db.DeleteMavenVersionMetadata("central", "com.example", "demo", "1.0.0"))
}
