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
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func newPostgresTestSchema(t *testing.T, prefix string) (string, *sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("RENOP_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("RENOP_TEST_POSTGRES_DSN is not configured")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	schema := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	admin, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	require.NoError(t, admin.Ping())
	_, err = admin.Exec(`CREATE SCHEMA "` + schema + `"`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, dropErr := admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
		require.NoError(t, dropErr)
		require.NoError(t, admin.Close())
	})
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), admin, schema
}

func TestPostgresUserProfileIntegration(t *testing.T) {
	dsn, admin, schema := newPostgresTestSchema(t, "renop_profile_test")
	_, err := admin.Exec(`CREATE TABLE "` + schema + `".user_profiles (
		username VARCHAR(255) PRIMARY KEY,
		nickname VARCHAR(144) NOT NULL DEFAULT '',
		rename_window_started_at BIGINT NOT NULL DEFAULT 0,
		rename_count INT NOT NULL DEFAULT 0,
		updated_at BIGINT NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "postgres", Dsn: dsn, MaxOpenConns: 2, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	account := &core.AccessToken{
		Identifier: core.AccessTokenIdentifier{Type: core.Persistent, Value: 1},
		Name:       "profile_pg", CreatedAt: "2026-08-24T00:00:00Z", Permissions: []string{"base"},
	}
	require.NoError(t, db.SaveToken(account))
	const changedAt int64 = 1_800_000_100_000
	profile, err := db.UpdateUserProfile("profile_pg", "profile_pg", "PostgreSQL User", account, changedAt)
	require.NoError(t, err)
	require.Equal(t, "PostgreSQL User", profile.Nickname)
	stableUserID := profile.UserID
	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{
		Repository: "cargo", Name: "profile-pg-crate", NormalizedName: "profile-pg-crate",
		CreatedAt: changedAt, UpdatedAt: changedAt,
	}, &core.CargoVersion{
		Repository: "cargo", Package: "profile-pg-crate", Version: "1.0.0",
		Publisher: account.Name, CreatedAt: changedAt,
	}, account.Name))
	require.NoError(t, db.RecordCargoMirrorPublication(&core.CargoPackage{
		Repository: "cargo", Name: "mirror-pg-crate", NormalizedName: "mirror-pg-crate",
		CreatedAt: changedAt, UpdatedAt: changedAt,
	}, &core.CargoVersion{
		Repository: "cargo", Package: "mirror-pg-crate", Version: "1.0.0", Size: 512, CreatedAt: changedAt,
	}))
	mirroredCargo, err := db.GetCargoPackageDetails("cargo", "mirror-pg-crate", "profile_pg")
	require.NoError(t, err)
	require.True(t, mirroredCargo.Package.Mirrored)
	require.Len(t, mirroredCargo.Versions, 1)
	require.True(t, mirroredCargo.Versions[0].Mirrored)
	_, err = db.CreateDockerImage("docker", "profile/pg", account.Name, false, changedAt)
	require.NoError(t, err)
	require.NoError(t, db.PutDockerManifest(&core.DockerManifest{
		Repository: "docker", ImageName: "profile/pg",
		Digest:    "sha256:abc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdee",
		MediaType: "application/vnd.oci.image.manifest.v1+json", RawJSON: []byte(`{"schemaVersion":2}`),
	}, "latest", account.Name))
	profile, err = db.UpdateUserProfile("profile_pg", "profile_pg_one", profile.Nickname, account, changedAt+1)
	require.NoError(t, err)
	require.Equal(t, stableUserID, profile.UserID)
	profile, err = db.UpdateUserProfile("profile_pg_one", "profile_pg_two", profile.Nickname,
		mustToken(t, db, "profile_pg_one"), changedAt+2)
	require.NoError(t, err)
	require.Equal(t, stableUserID, profile.UserID)
	_, err = db.UpdateUserProfile("profile_pg_two", "profile_pg_three", profile.Nickname,
		mustToken(t, db, "profile_pg_two"), changedAt+3)
	require.ErrorIs(t, err, core.ErrUsernameChangeRateLimited)
	loaded, err := db.GetUserProfiles([]string{"profile_pg_two", "missing"})
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.Equal(t, "PostgreSQL User", loaded["profile_pg_two"].Nickname)
	byID, err := db.GetUserProfileByID(stableUserID)
	require.NoError(t, err)
	require.Equal(t, "profile_pg_two", byID.Username)
	cargoMemberships, err := db.ListUserPackageMemberships(stableUserID, config.RepositoryFormatCargo)
	require.NoError(t, err)
	require.Len(t, cargoMemberships, 1)
	require.Equal(t, "profile-pg-crate", cargoMemberships[0].Name)
	dockerMemberships, err := db.ListUserPackageMemberships(stableUserID, config.RepositoryFormatDocker)
	require.NoError(t, err)
	require.Len(t, dockerMemberships, 1)
	require.Equal(t, "profile/pg", dockerMemberships[0].Name)
}

func TestPostgresMavenDomainsMigrateToGlobalOwnership(t *testing.T) {
	dsn, _, _ := newPostgresTestSchema(t, "renop_maven_global_test")
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "postgres", Dsn: dsn, MaxOpenConns: 2, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	for _, username := range []string{"maven_alice", "maven_bob"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: username, CreatedAt: time.Now().UTC().Format(time.RFC3339)}))
	}
	alice, err := db.GetUserProfile("maven_alice")
	require.NoError(t, err)
	bob, err := db.GetUserProfile("maven_bob")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO maven_domains
		(repository, domain, verification_type, verification_host, verification_code, verified, created_at, verified_at, last_check_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"releases", "com.example", "dns", "example.com", "pending", 0, 100, 0, 100,
		"snapshots", "com.example", "dns", "example.com", "verified", 1, 200, 210, 220)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO maven_domain_members
		(repository, domain, username, user_id, permission_level, added_at)
		VALUES (?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?)`,
		"releases", "com.example", "maven_alice", alice.UserID, core.MavenPermissionOwner, 100,
		"snapshots", "com.example", "maven_bob", bob.UserID, core.MavenPermissionOwner, 50)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	db, err = database.InitDB(config.DatabaseConfig{
		Driver: "postgres", Dsn: dsn, MaxOpenConns: 2, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	details, err := db.GetMavenDomainDetails("com.example", "maven_bob")
	require.NoError(t, err)
	require.True(t, details.Domain.Verified)
	require.Equal(t, "verified", details.Domain.VerificationCode)
	levels := make(map[string]int)
	for _, member := range details.Members {
		levels[member.Username] = member.Level
	}
	require.Equal(t, core.MavenPermissionOwner, levels["maven_bob"])
	require.Equal(t, core.MavenPermissionManage, levels["maven_alice"])
	const publishedAt int64 = 1_800_000_200_000
	require.NoError(t, db.RecordMavenPublication(&core.MavenArtifact{
		Repository: "releases", Domain: "com.example", GroupID: "com.example",
		ArtifactID: "postgres-demo", Publisher: "maven_bob", LatestVersion: "1.0.0",
		CreatedAt: publishedAt, UpdatedAt: publishedAt,
	}, &core.MavenVersion{
		Repository: "releases", GroupID: "com.example", ArtifactID: "postgres-demo",
		Version: "1.0.0", Publisher: "maven_bob", Size: 2048, CreatedAt: publishedAt,
	}))
	require.NoError(t, db.RecordMavenMirrorPublication(&core.MavenArtifact{
		Repository: "releases", Domain: "com.example", GroupID: "com.example",
		ArtifactID: "postgres-demo", LatestVersion: "2.0.0", CreatedAt: publishedAt, UpdatedAt: publishedAt + 1,
	}, &core.MavenVersion{
		Repository: "releases", GroupID: "com.example", ArtifactID: "postgres-demo",
		Version: "2.0.0", Size: 4096, CreatedAt: publishedAt + 1,
	}))
	repositoryDomains, err := db.ListMavenRepositoryDomains("releases", "maven_bob")
	require.NoError(t, err)
	require.Len(t, repositoryDomains, 1)
	require.Equal(t, "com.example", repositoryDomains[0].Domain)
	require.Equal(t, 1, repositoryDomains[0].ArtifactCount)
	require.Equal(t, 1, repositoryDomains[0].RepositoryCount)
	require.Equal(t, 2, repositoryDomains[0].MemberCount)
	artifactDetails, err := db.GetMavenArtifactDetails("releases", "com.example", "postgres-demo")
	require.NoError(t, err)
	require.Equal(t, int64(6144), artifactDetails.Artifact.TotalSize)
	require.True(t, artifactDetails.Artifact.Mirrored)
	require.Len(t, artifactDetails.Versions, 2)
	require.True(t, artifactDetails.Versions[0].Mirrored)
	require.NoError(t, db.RemoveMavenMember("com.example", "maven_bob", "maven_alice"))
	requireTeamRemovalMessage(t, db, "maven_alice", "maven", "", "com.example", "maven_bob")
}
