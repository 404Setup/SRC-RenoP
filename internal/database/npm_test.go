/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func TestNPMPackageLifecycleVisibilityAndTeamOwnership(t *testing.T) {
	db := newMavenDB(t)
	for _, username := range []string{"alice", "bob", "charlie"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: username, Permissions: []string{"base"}}))
	}
	now := time.Now().UnixMilli()
	publicPackage, err := db.CreateNPMPackage("npm", "demo", "alice", false, now)
	require.NoError(t, err)
	assert.Equal(t, core.NPMPermissionOwner, publicPackage.PermissionLevel)
	_, err = db.CreateNPMPackage("npm", "@team/secret", "alice", true, now+1)
	require.NoError(t, err)
	_, err = db.CreateNPMPackage("npm", "demo", "alice", false, now+2)
	require.ErrorIs(t, err, core.ErrNPMPackageExists)

	guestPackages, guestTotal, err := db.ListNPMPackages("npm", "guest", false, 20, 0)
	require.NoError(t, err)
	require.Len(t, guestPackages, 1)
	assert.Equal(t, 1, guestTotal)
	assert.Equal(t, "demo", guestPackages[0].Name)
	alicePackages, aliceTotal, err := db.ListNPMPackages("npm", "alice", false, 20, 0)
	require.NoError(t, err)
	require.Len(t, alicePackages, 2)
	assert.Equal(t, 2, aliceTotal)

	manifest := `{"name":"demo","version":"1.0.0","description":"Demo package"}`
	require.NoError(t, db.RecordNPMPublication(&core.NPMPackage{
		Repository: "npm", Name: "demo", Description: "Demo package", UpdatedAt: now + 3,
	}, &core.NPMVersion{
		Repository: "npm", Package: "demo", Version: "1.0.0", ManifestJSON: manifest,
		Publisher: "alice", TarballPath: "demo/-/demo-1.0.0.tgz", Shasum: "0123456789012345678901234567890123456789",
		Integrity: "sha512-ZGVtbw==", Size: 128, CreatedAt: now + 3,
	}, map[string]string{"latest": "1.0.0"}, "alice"))
	details, err := db.GetNPMPackageDetails("npm", "demo", "alice")
	require.NoError(t, err)
	require.Len(t, details.Versions, 1)
	assert.Equal(t, 1, details.MemberCount)
	assert.Equal(t, "1.0.0", details.Package.LatestVersion)
	assert.Equal(t, "1.0.0", details.DistTags["latest"])
	require.ErrorIs(t, db.RecordNPMPublication(&core.NPMPackage{
		Repository: "npm", Name: "demo", UpdatedAt: now + 4,
	}, &core.NPMVersion{
		Repository: "npm", Package: "demo", Version: "1.0.0", ManifestJSON: manifest,
		TarballPath: "demo/-/demo-1.0.0.tgz", CreatedAt: now + 4,
	}, nil, "alice"), core.ErrNPMVersionExists)

	require.NoError(t, db.SetNPMDistTag("npm", "demo", "stable", "1.0.0", "alice", 0))
	require.NoError(t, db.SetNPMVersionDeprecated("npm", "demo", "1.0.0", "Use 2.x", "alice", 0))
	tarballPath, err := db.UnpublishNPMVersion("npm", "demo", "1.0.0", "alice", 0)
	require.NoError(t, err)
	assert.Equal(t, "demo/-/demo-1.0.0.tgz", tarballPath)
	details, err = db.GetNPMPackageDetails("npm", "demo", "alice")
	require.NoError(t, err)
	assert.True(t, details.Versions[0].Unpublished)
	assert.Empty(t, details.DistTags)
	require.ErrorIs(t, db.RecordNPMPublication(&core.NPMPackage{
		Repository: "npm", Name: "demo", UpdatedAt: now + 5,
	}, &core.NPMVersion{
		Repository: "npm", Package: "demo", Version: "1.0.0", ManifestJSON: manifest,
		TarballPath: "demo/-/demo-1.0.0.tgz", CreatedAt: now + 5,
	}, nil, "alice"), core.ErrNPMVersionExists)

	require.NoError(t, db.ForceAddNPMMembers("npm", "@team/secret", "alice", []string{"bob"}, core.NPMPermissionRead))
	require.NoError(t, db.ForceAddNPMMembers("npm", "@team/secret", "alice", []string{"charlie"}, core.NPMPermissionRead))
	require.NoError(t, db.RemoveNPMMember("npm", "@team/secret", "alice", "charlie"))
	requireTeamRemovalMessage(t, db, "charlie", "npm", "npm", "@team/secret", "alice")
	bobPackages, bobTotal, err := db.ListNPMPackages("npm", "bob", false, 20, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, bobTotal)
	require.Len(t, bobPackages, 2)
	require.ErrorIs(t, db.RemoveNPMMember("npm", "@team/secret", "alice", "alice"), core.ErrNPMOwnerCannotLeave)
	require.NoError(t, db.SetNPMMemberLevel("npm", "@team/secret", "alice", "bob", core.NPMPermissionOwner))
	require.NoError(t, db.RemoveNPMMember("npm", "@team/secret", "alice", "alice"))
	members, err := db.ListNPMMembers("npm", "@team/secret")
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, "bob", members[0].Username)
	assert.Equal(t, core.NPMPermissionOwner, members[0].Level)
	details, err = db.GetNPMPackageDetails("npm", "@team/secret", "bob")
	require.NoError(t, err)
	assert.Equal(t, 1, details.MemberCount)

	profile, err := db.GetUserProfile("bob")
	require.NoError(t, err)
	assert.Equal(t, 1, profile.NPMPackageCount)
	_, err = db.UnpublishNPMVersion("npm", "missing", "1.0.0", "alice", 0)
	assert.True(t, errors.Is(err, core.ErrNPMPackageNotFound))
}

func TestNPMMirrorRefreshRemovesVersionsDeletedUpstream(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	packageMetadata := &core.NPMPackage{
		Repository: "npm", Name: "mirror-demo", Description: "Mirrored package",
		LatestVersion: "2.0.0", Mirrored: true, CreatedAt: now, UpdatedAt: now,
	}
	versions := []*core.NPMVersion{
		{
			Version: "1.0.0", ManifestJSON: `{"name":"mirror-demo","version":"1.0.0"}`,
			TarballPath: "mirror-demo/-/mirror-demo-1.0.0.tgz", Mirrored: true, CreatedAt: now,
		},
		{
			Version: "2.0.0", ManifestJSON: `{"name":"mirror-demo","version":"2.0.0"}`,
			TarballPath: "mirror-demo/-/mirror-demo-2.0.0.tgz", Mirrored: true, CreatedAt: now + 1,
		},
	}
	require.NoError(t, db.RecordNPMMirrorPublication(packageMetadata, versions,
		map[string]string{"latest": "2.0.0"}))

	packageMetadata.LatestVersion = "1.0.0"
	packageMetadata.UpdatedAt++
	require.NoError(t, db.RecordNPMMirrorPublication(packageMetadata, versions[:1],
		map[string]string{"latest": "1.0.0"}))
	details, err := db.GetNPMPackageDetails("npm", "mirror-demo", "guest")
	require.NoError(t, err)
	require.Len(t, details.Versions, 1)
	states := make(map[string]bool, len(details.Versions))
	for _, version := range details.Versions {
		states[version.Version] = version.Unpublished
	}
	assert.False(t, states["1.0.0"])
	_, staleVersionPresent := states["2.0.0"]
	assert.False(t, staleVersionPresent)
	assert.Equal(t, "1.0.0", details.DistTags["latest"])
}

func TestNPMPackageMetadataAggregateIsBounded(t *testing.T) {
	db := newMavenDB(t)
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "metadata_owner", Permissions: []string{"base"}}))
	now := time.Now().UnixMilli()
	_, err := db.CreateNPMPackage("npm", "metadata-demo", "metadata_owner", false, now)
	require.NoError(t, err)
	record := func(version string, manifestSize int, createdAt int64) error {
		manifest := `{"name":"metadata-demo","version":"` + version + `","payload":"` +
			strings.Repeat("x", manifestSize) + `"}`
		return db.RecordNPMPublication(&core.NPMPackage{
			Repository: "npm", Name: "metadata-demo", UpdatedAt: createdAt,
		}, &core.NPMVersion{
			Repository: "npm", Package: "metadata-demo", Version: version, ManifestJSON: manifest,
			Publisher: "metadata_owner", TarballPath: "metadata-demo/-/metadata-demo-" + version + ".tgz",
			CreatedAt: createdAt,
		}, map[string]string{"latest": version}, "metadata_owner")
	}
	require.NoError(t, record("1.0.0", 3<<20, now+1))
	require.ErrorIs(t, record("2.0.0", 2<<20, now+2), core.ErrNPMPackageLimit)
	details, err := db.GetNPMPackageDetails("npm", "metadata-demo", "metadata_owner")
	require.NoError(t, err)
	require.Len(t, details.Versions, 1)
}

func TestPostgresNPMPackageIntegration(t *testing.T) {
	dsn, _, _ := newPostgresTestSchema(t, "renop_npm_test")
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "postgres", Dsn: dsn, MaxOpenConns: 2, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, username := range []string{"npm_pg_owner", "npm_pg_reader"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: username, Permissions: []string{"base"}}))
	}
	now := time.Now().UnixMilli()
	_, err = db.CreateNPMPackage("npm-pg", "@team/private-demo", "npm_pg_owner", true, now)
	require.NoError(t, err)
	require.NoError(t, db.RecordNPMPublication(&core.NPMPackage{
		Repository: "npm-pg", Name: "@team/private-demo", Description: "PostgreSQL npm package", UpdatedAt: now + 1,
	}, &core.NPMVersion{
		Repository: "npm-pg", Package: "@team/private-demo", Version: "1.0.0",
		ManifestJSON: `{"name":"@team/private-demo","version":"1.0.0"}`,
		Publisher:    "npm_pg_owner", TarballPath: "@team/private-demo/-/private-demo-1.0.0.tgz",
		Shasum: "0123456789012345678901234567890123456789", Integrity: "sha512-ZGVtbw==",
		Size: 64, CreatedAt: now + 1,
	}, map[string]string{"latest": "1.0.0"}, "npm_pg_owner"))
	require.NoError(t, db.ForceAddNPMMembers("npm-pg", "@team/private-demo", "npm_pg_owner",
		[]string{"npm_pg_reader"}, core.NPMPermissionRead))

	guestPackages, guestTotal, err := db.ListNPMPackages("npm-pg", "guest", false, 20, 0)
	require.NoError(t, err)
	require.Empty(t, guestPackages)
	require.Zero(t, guestTotal)
	readerPackages, readerTotal, err := db.ListNPMPackages("npm-pg", "npm_pg_reader", false, 20, 0)
	require.NoError(t, err)
	require.Len(t, readerPackages, 1)
	require.Equal(t, 1, readerTotal)
	details, err := db.GetNPMPackageDetails("npm-pg", "@team/private-demo", "npm_pg_reader")
	require.NoError(t, err)
	require.True(t, details.Member)
	require.Equal(t, "1.0.0", details.DistTags["latest"])
	require.Len(t, details.Versions, 1)
}
