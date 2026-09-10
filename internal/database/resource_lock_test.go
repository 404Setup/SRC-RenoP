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
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestCustomResourceLockReasonSurvivesMigrationAndRestart(t *testing.T) {
	cfg := config.DatabaseConfig{Driver: "sqlite3", Dsn: filepath.Join(testutil.TempDir(t), "custom-lock.db")}
	db, err := database.InitDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	lock := &core.ResourceLock{ResourceLockTarget: core.ResourceLockTarget{Format: "cargo", Repository: "cargo", Name: "demo"},
		Source: core.ResourceLockSystem, Mode: core.ResourceLockWrite, Reason: "hold", LockedAt: time.Now().UnixMilli()}
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	_, err = db.Exec(`ALTER TABLE resource_locks DROP COLUMN reason_text`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	db, err = database.InitDB(cfg)
	require.NoError(t, err)
	locks, err := db.GetResourceLocks(lock.ResourceLockTarget, false)
	require.NoError(t, err)
	require.Len(t, locks, 1)
	require.Equal(t, "hold", locks[0].Reason)
	require.Empty(t, locks[0].ReasonText)
	lock.Reason, lock.ReasonText = "custom", "需要核对来源 <script>plain text</script>"
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	require.NoError(t, db.Close())
	db, err = database.InitDB(cfg)
	require.NoError(t, err)
	for _, invalid := range []string{"", " ", "line\nbreak", "\x00", "\xff", strings.Repeat("界", 257)} {
		copy := *lock
		copy.ReasonText = invalid
		require.ErrorIs(t, db.SetResourceLock(&copy, "", ""), core.ErrResourceLockInvalid)
	}
	locks, err = db.GetResourceLocks(lock.ResourceLockTarget, false)
	require.NoError(t, err)
	require.Len(t, locks, 1)
	require.Equal(t, lock.ReasonText, locks[0].ReasonText)
}

func TestMavenLocksCoverMetadataCompanionsAndCatalogMutations(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	require.NoError(t, db.CreateMavenDomain(&core.MavenDomain{Domain: "com.example", VerificationType: "dns",
		VerificationHost: "example.com", VerificationCode: "proof", CreatedAt: now}, "alice"))
	require.NoError(t, db.MarkMavenDomainVerified("com.example", "proof", now, nil))
	require.NoError(t, db.ForceAddMavenMembers("com.example", "alice", []string{"bob"}, 0))
	publish := func(version string) error {
		return db.RecordMavenPublication(&core.MavenArtifact{Repository: "maven", Domain: "com.example",
			GroupID: "com.example", ArtifactID: "demo", Publisher: "alice", CreatedAt: now},
			&core.MavenVersion{Version: version, Publisher: "alice", CreatedAt: now})
	}
	require.NoError(t, publish("1.0-SNAPSHOT"))
	require.NoError(t, publish("2.0"))
	lock := &core.ResourceLock{Format: "maven", Repository: "maven",
		Name: "com.example:demo", Version: "1.0-SNAPSHOT", Source: core.ResourceLockSystem,
		Mode: core.ResourceLockRead, Reason: "trojan", LockedAt: now}
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	for _, path := range []string{"com/example/demo/1.0-SNAPSHOT/demo-1.0-20260909.1.jar",
		"com/example/demo/1.0-SNAPSHOT/maven-metadata.xml.sha256", "com/example/demo/1.0-SNAPSHOT/arbitrary.txt",
		"com/example/demo/maven-metadata.xml", "com/example/demo/1.0-SNAPSHOT"} {
		locks, err := db.GetMavenPathLocks("maven", path, false)
		require.NoError(t, err, path)
		require.Len(t, locks, 1, path)
	}
	locks, err := db.GetMavenPathLocks("maven", "com/example/demo/2.0/demo-2.0.jar", false)
	require.NoError(t, err)
	require.Empty(t, locks)
	locks, err = db.GetMavenPathLocks("maven", "com/example", true)
	require.NoError(t, err)
	require.Len(t, locks, 1)
	for _, viewer := range []string{"guest", "bob", "alice"} {
		visible, err := db.ResourceMetadataVisibility("maven", "maven", viewer, false, []core.ResourceLockTarget{lock.ResourceLockTarget})
		require.NoError(t, err)
		require.Equal(t, []bool{viewer != "guest"}, visible)
		artifacts, total, err := db.ListReadableMavenArtifacts([]string{"maven"}, "", "", viewer, nil, 10, 0)
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, "2.0", artifacts[0].LatestVersion)
		if viewer == "guest" {
			require.Equal(t, 1, artifacts[0].VersionCount)
		} else {
			require.Equal(t, 2, artifacts[0].VersionCount)
		}
	}
	require.ErrorIs(t, publish("1.0-SNAPSHOT"), core.ErrResourceLocked)
	require.ErrorIs(t, db.DeleteMavenVersionMetadata("maven", "com.example", "demo", "1.0-SNAPSHOT"), core.ErrResourceLocked)
	require.ErrorIs(t, db.DeprecatePackage("maven", "maven", "com.example:demo", now), core.ErrResourceLocked)
	if runtime.GOOS == "windows" {
		require.ErrorIs(t, publish("1.0-snapshot"), core.ErrResourceLocked)
		locks, err := db.GetMavenPathLocks("maven", "COM/EXAMPLE/DEMO/1.0-snapshot/other.bin", false)
		require.NoError(t, err)
		require.Len(t, locks, 1)
	}
	require.NoError(t, publish("3.0"))
	require.NoError(t, db.UpdateMavenArtifactDescription("maven", "com.example", "demo", "allowed"))
	lock.Version, lock.Mode = "", core.ResourceLockWrite
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	require.ErrorIs(t, publish("4.0"), core.ErrResourceLocked)
	require.ErrorIs(t, db.UpdateMavenArtifactDescription("maven", "com.example", "demo", "blocked"), core.ErrResourceLocked)
	require.ErrorIs(t, db.UpdateMavenArtifactReadme("maven", "com.example", "demo", "blocked"), core.ErrResourceLocked)
	require.NoError(t, db.RollbackMavenPublication("maven", "com.example", "demo", "3.0"))
	lock.Mode = core.ResourceLockRead
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	artifacts, total, err := db.ListReadableMavenArtifacts([]string{"maven"}, "", "", "guest", nil, 10, 0)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, artifacts)
	artifacts, total, err = db.ListReadableMavenArtifacts([]string{"maven"}, "", "", "guest", []string{"maven"}, 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, artifacts, 1)
}

func TestSuperTeamLocksFollowBindingsAndPreserveMembership(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	team := &core.SuperTeam{Prefix: "locked-team", Name: "Locked Team", CreatedAt: now}
	require.NoError(t, db.CreateSuperTeam(team, "alice", 5, 10))
	require.NoError(t, db.ForceAddSuperTeamMembers(team.Prefix, "admin", []string{"bob"}, core.SuperTeamRoleRead, 5, 10, now))
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "moderator", Permissions: []string{"canmoderate:*"}}))
	_, err := db.CreateNPMPackageForTeam("npm", "@locked-team/demo", "alice", team.Prefix, false, now)
	require.NoError(t, err)
	_, err = db.CreateDockerImageForTeam("docker", "locked-team/demo", "alice", team.Prefix, false, now)
	require.NoError(t, err)
	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{Repository: "cargo", Name: "demo", NormalizedName: "demo", CreatedAt: now, UpdatedAt: now},
		&core.CargoVersion{Version: "1.0.0", CreatedAt: now}, "alice"))
	_, err = db.Exec(`UPDATE cargo_packages SET super_team_prefix = ? WHERE repository = ? AND normalized_name = ?`, team.Prefix, "cargo", "demo")
	require.NoError(t, err)
	require.NoError(t, db.CreateMavenDomain(&core.MavenDomain{Domain: "com.example", SuperTeamPrefix: team.Prefix,
		VerificationType: "dns", VerificationHost: "example.com", VerificationCode: "proof", CreatedAt: now}, "alice"))
	require.NoError(t, db.MarkMavenDomainVerified("com.example", "proof", now, nil))
	publish := func(name string) error {
		return db.RecordMavenPublication(&core.MavenArtifact{Repository: "maven", Domain: "com.example",
			GroupID: "com.example", ArtifactID: name, CreatedAt: now}, &core.MavenVersion{Version: "1.0", Publisher: "alice", CreatedAt: now})
	}
	require.NoError(t, publish("demo"))
	lock := &core.ResourceLock{Format: "superteam", Name: team.Prefix,
		Mode: core.ResourceLockRead, Source: core.ResourceLockSystem, Reason: "abuse", LockedAt: now}
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	for _, target := range []core.ResourceLockTarget{
		{Format: "cargo", Repository: "cargo", Name: "demo"},
		{Format: "npm", Repository: "npm", Name: "@locked-team/demo"},
		{Format: "docker", Repository: "docker", Name: "locked-team/demo"},
		{Format: "maven", Repository: "maven", Name: "com.example:demo"},
	} {
		locks, err := db.GetResourceLocks(target, true)
		require.NoError(t, err)
		require.Len(t, locks, 1, target.Format)
		require.True(t, locks[0].Inherited)
		require.ErrorIs(t, db.EnsureResourceMutable(target, false), core.ErrResourceLocked)
		for _, viewer := range []string{"guest", "alice", "bob"} {
			visible, err := db.ResourceMetadataVisibility(target.Format, target.Repository, viewer, false, []core.ResourceLockTarget{target})
			require.NoError(t, err)
			require.Equal(t, []bool{viewer != "guest"}, visible, target.Format)
		}
	}
	for _, viewer := range []string{"alice", "bob", "moderator"} {
		details, err := db.GetPublicSuperTeamDetails(team.Prefix, viewer, false, viewer == "moderator")
		require.NoError(t, err)
		require.Len(t, details.Team.Locks, 1)
	}
	_, err = db.GetPublicSuperTeamDetails(team.Prefix, "", false, false)
	require.ErrorIs(t, err, core.ErrSuperTeamNotFound)
	require.ErrorIs(t, db.UpdateSuperTeam(team.Prefix, "alice", "Changed", "", core.PublicLinks{}, false, now), core.ErrResourceLocked)
	require.ErrorIs(t, db.SetSuperTeamMemberLevel(team.Prefix, "alice", "bob", core.SuperTeamRoleWrite, false), core.ErrResourceLocked)
	require.ErrorIs(t, db.SetSuperTeamMemberVisibility(team.Prefix, "bob", false), core.ErrResourceLocked)
	require.ErrorIs(t, db.RemoveSuperTeamMember(team.Prefix, "alice", "bob", false, now), core.ErrResourceLocked)
	require.ErrorIs(t, db.DeleteSuperTeam(team.Prefix, "admin", true, now), core.ErrResourceLocked)
	require.ErrorIs(t, db.ForceAddMavenMembers("com.example", "alice", []string{"moderator"}, 0), core.ErrResourceLocked)
	require.ErrorIs(t, publish("new"), core.ErrResourceLocked)
	require.ErrorIs(t, db.SetPublicationQuotaOverride(core.PublicationQuotaSubject{OwnerType: core.PublicationQuotaOwnerSuperTeam, OwnerKey: team.Prefix}, core.PublicationQuotaOverride{}, now), core.ErrResourceLocked)
	_, err = db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceMavenDomain, ResourceKey: "com.example"}, "admin", true, now)
	require.ErrorIs(t, err, core.ErrResourceLocked)
	_, err = db.GetPublicSuperTeamDetails(team.Prefix, "moderator", false, false)
	require.ErrorIs(t, err, core.ErrSuperTeamNotFound)

	_, err = db.CreateNPMPackageForTeam("npm", "@locked-team/new", "alice", team.Prefix, false, now)
	require.ErrorIs(t, err, core.ErrResourceLocked)
	eligible, total, err := db.ListManageableSuperTeams("alice", core.SuperTeamRoleManage, 10, 0)
	require.NoError(t, err)
	require.Empty(t, eligible)
	require.Zero(t, total)
	profile, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	visibleTeams, total, err := db.ListVisibleUserSuperTeams(profile.UserID, "", false, false, 10, 0)
	require.NoError(t, err)
	require.Empty(t, visibleTeams)
	require.Zero(t, total)
	require.NoError(t, db.DeleteResourceLock(lock.ResourceLockTarget, core.ResourceLockSystem, "", ""))
	require.NoError(t, publish("new"))
	role, err := db.GetSuperTeamRole(team.Prefix, "bob")
	require.NoError(t, err)
	require.Equal(t, core.SuperTeamRoleRead, role)
}

func TestDockerLocksProtectAliasesIndexChildrenAndSharedBlobs(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	for _, name := range []string{"alice", "reader"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: name}))
	}
	for _, name := range []string{"demo", "other"} {
		_, err := db.CreateDockerImage("docker", name, "alice", false, now)
		require.NoError(t, err)
	}
	require.NoError(t, db.ForceAddDockerMembers("docker", "demo", "alice", []string{"reader"}, 0))
	digestOf := func(raw string) string { return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(raw))) }
	blob := digestOf("layer")
	childRaw := fmt.Sprintf(`{"schemaVersion":2,"layers":[{"digest":%q}]}`, blob)
	child := digestOf(childRaw)
	indexRaw := fmt.Sprintf(`{"schemaVersion":2,"manifests":[{"digest":%q}]}`, child)
	index := digestOf(indexRaw)
	put := func(image, digest, raw, tag string, blobs ...string) error {
		return db.PutDockerManifest(&core.DockerManifest{Repository: "docker", ImageName: image,
			Digest: digest, MediaType: "application/vnd.oci.image.manifest.v1+json", RawJSON: []byte(raw), BlobDigests: blobs}, tag, "alice")
	}
	require.NoError(t, put("demo", child, childRaw, "amd64", blob))
	require.NoError(t, put("demo", index, indexRaw, "latest"))
	require.NoError(t, put("demo", index, indexRaw, "stable"))
	require.NoError(t, put("other", child, childRaw, "latest", blob))
	lock := &core.ResourceLock{Format: "docker", Repository: "docker", Name: "demo", Version: index,
		Source: core.ResourceLockSystem, Mode: core.ResourceLockRead, Reason: "trojan", LockedAt: now}
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	_, err := db.Exec(`UPDATE docker_manifests SET raw_json = ? WHERE repository = ? AND image_name = ? AND digest = ?`, "invalid", "docker", "demo", index)
	require.NoError(t, err)
	replacement := *lock
	replacement.Mode = core.ResourceLockWrite
	require.ErrorIs(t, db.SetResourceLock(&replacement, "", ""), core.ErrDockerManifestInvalid)
	retained, err := db.GetResourceLocks(core.ResourceLockTarget{Format: "docker", Repository: "docker", Name: "demo", Version: blob}, false)
	require.NoError(t, err)
	require.Len(t, retained, 1)
	require.Equal(t, core.ResourceLockRead, retained[0].Mode)
	_, err = db.Exec(`UPDATE docker_manifests SET raw_json = ? WHERE repository = ? AND image_name = ? AND digest = ?`, indexRaw, "docker", "demo", index)
	require.NoError(t, err)
	for _, digest := range []string{index, child, blob} {
		target := lock.ResourceLockTarget
		target.Version = digest
		require.ErrorIs(t, db.EnsureResourceMutable(target, false), core.ErrResourceLocked)
		locks, err := db.GetResourceLocks(target, false)
		require.NoError(t, err)
		require.Len(t, locks, 1)
		require.Equal(t, digest != index, locks[0].Inherited)
		visible, err := db.ResourceMetadataVisibility("docker", "docker", "guest", false, []core.ResourceLockTarget{target})
		require.NoError(t, err)
		require.Equal(t, []bool{false}, visible)
		visible, err = db.ResourceMetadataVisibility("docker", "docker", "reader", false, []core.ResourceLockTarget{target})
		require.NoError(t, err)
		require.Equal(t, []bool{true}, visible)
	}
	for _, tag := range []string{"latest", "stable", "amd64"} {
		require.ErrorIs(t, db.DeleteDockerTag("docker", "demo", tag), core.ErrResourceLocked)
		require.ErrorIs(t, put("demo", digestOf(`{}`), `{}`, tag), core.ErrResourceLocked)
	}
	require.ErrorIs(t, db.DeleteDockerManifest("docker", "demo", child), core.ErrResourceLocked)
	require.ErrorIs(t, db.DeleteDockerImage("docker", "demo"), core.ErrResourceLocked)
	require.ErrorIs(t, db.DeleteDockerBlob("docker", blob), core.ErrResourceLocked)
	require.ErrorIs(t, db.EnsureDockerBlobMutable("docker", blob), core.ErrResourceLocked)
	require.ErrorIs(t, db.RecordDockerImageBlob("docker", "demo", blob), core.ErrResourceLocked)
	require.ErrorIs(t, db.DeprecatePackage("docker", "docker", "demo", now), core.ErrResourceLocked)
	require.NoError(t, db.UpdateDockerImageDescription("docker", "demo", "Allowed outside the version"))
	require.NoError(t, put("demo", digestOf(`{}`), `{}`, "unlocked"))
	require.NoError(t, db.DeleteResourceLock(lock.ResourceLockTarget, lock.Source, "", ""))
	require.NoError(t, db.EnsureDockerBlobMutable("docker", blob))
	lock.Version = ""
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	require.ErrorIs(t, db.UpdateDockerImageDescription("docker", "demo", "blocked"), core.ErrResourceLocked)
	require.ErrorIs(t, db.ForceAddDockerMembers("docker", "demo", "alice", []string{"new"}, 0), core.ErrResourceLocked)
	require.ErrorIs(t, db.RemoveDockerMember("docker", "demo", "reader", "reader"), core.ErrResourceLocked)
	require.ErrorIs(t, db.EnsureDockerBlobMutable("docker", blob), core.ErrResourceLocked)
}

func TestResourceLocksPreserveSourcesAndFilterCargoMetadata(t *testing.T) {
	cfg := config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "locks.db")}
	db, err := database.InitDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	now := time.Now().UnixMilli()
	for name, permissions := range map[string][]string{
		"alice": {"base"}, "reader": {"base"}, "moderator": {"canmoderate:cargo"}, "outsider": {"canmoderate:other"},
	} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: name, Permissions: permissions}))
		session := &core.Session{PublicID: name, Username: name, CreatedAt: now}
		session.LastActive.Store(now)
		require.NoError(t, db.SaveSession(session, name+"-session"))
	}
	pkg := &core.CargoPackage{Repository: "cargo", Name: "demo", NormalizedName: "demo", CreatedAt: now, UpdatedAt: now}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		require.NoError(t, db.RecordCargoPublication(pkg, &core.CargoVersion{
			Repository: "cargo", Package: "demo", Version: version, CreatedAt: now, Publisher: "alice",
		}, "alice"))
	}
	reader, err := db.GetUserProfile("reader")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO cargo_members (repository, normalized_name, username, user_id, permission_level, added_at)
		VALUES (?, ?, ?, ?, ?, ?)`, "cargo", "demo", "reader", reader.UserID, 0, now)
	require.NoError(t, err)
	target := core.ResourceLockTarget{Format: "cargo", Repository: "cargo", Name: "DEMO", Version: "2.0.0"}
	lock := &core.ResourceLock{ResourceLockTarget: target, Source: core.ResourceLockManual,
		Mode: core.ResourceLockRead, Reason: "trojan", LockedAt: now}
	require.ErrorIs(t, db.SetResourceLock(lock, "outsider", "outsider-session"), core.ErrResourceLockPermission)
	require.ErrorIs(t, db.SetResourceLock(lock, "moderator", "alice-session"), core.ErrResourceLockPermission)
	require.NoError(t, db.SetResourceLock(lock, "moderator", "moderator-session"))
	require.ErrorIs(t, db.EnsureResourceMutable(target, false), core.ErrResourceLocked)
	sibling := target
	sibling.Version = "3.0.0"
	require.NoError(t, db.EnsureResourceMutable(sibling, false))
	require.ErrorIs(t, db.EnsureResourceMutable(sibling, true), core.ErrResourceLocked)
	packages, total, err := db.SearchCargoPackages("cargo", "demo", "", false, 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "1.0.0", packages[0].MaxVersion)
	packages, _, err = db.SearchCargoPackages("cargo", "demo", "reader", false, 10, 0)
	require.NoError(t, err)
	require.Equal(t, "2.0.0", packages[0].MaxVersion)
	visible, err := db.ResourceMetadataVisibility("cargo", "cargo", "", false, []core.ResourceLockTarget{target, sibling})
	require.NoError(t, err)
	require.Equal(t, []bool{false, true}, visible)
	visible, err = db.ResourceMetadataVisibility("cargo", "cargo", "reader", false, []core.ResourceLockTarget{target})
	require.NoError(t, err)
	require.Equal(t, []bool{true}, visible)
	member, err := db.HasCargoPackageMembership("cargo", "demo", "reader")
	require.NoError(t, err)
	require.True(t, member)
	lock.Source = core.ResourceLockSystem
	lock.Reason = "hold"
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	require.NoError(t, db.Close())
	db, err = database.InitDB(cfg)
	require.NoError(t, err)
	require.ErrorIs(t, db.DeleteResourceLock(target, core.ResourceLockSystem, "moderator", "moderator-session"), core.ErrResourceLockPermission)
	require.NoError(t, db.DeleteResourceLock(target, core.ResourceLockManual, "moderator", "moderator-session"))
	locks, err := db.GetResourceLocks(target, false)
	require.NoError(t, err)
	require.Len(t, locks, 1)
	require.Equal(t, core.ResourceLockSystem, locks[0].Source)
	require.NoError(t, db.DeleteResourceLock(target, core.ResourceLockSystem, "", ""))
	target.Version = ""
	lock.ResourceLockTarget = target
	lock.Source = core.ResourceLockManual
	require.NoError(t, db.SetResourceLock(lock, "moderator", "moderator-session"))
	packages, total, err = db.SearchCargoPackages("cargo", "demo", "", false, 10, 0)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, packages)
	packages, total, err = db.SearchCargoPackages("cargo", "demo", "reader", false, 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, packages, 1)
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "moderator", Permissions: []string{"base"}}))
	require.ErrorIs(t, db.DeleteResourceLock(target, core.ResourceLockManual, "moderator", "moderator-session"), core.ErrResourceLockPermission)
}

func TestNPMLocksFreezeVersionsAndPreserveMetadataVisibility(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	for _, name := range []string{"alice", "reader", "outsider"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: name}))
	}
	pkg, err := db.CreateNPMPackage("npm", "demo", "alice", false, now)
	require.NoError(t, err)
	require.NoError(t, db.ForceAddNPMMembers("npm", "demo", "alice", []string{"reader"}, 0))
	publish := func(version, tag string) error {
		return db.RecordNPMPublication(pkg, &core.NPMVersion{Repository: "npm", Package: "demo", Version: version,
			ManifestJSON: `{"name":"demo","version":"` + version + `"}`, TarballPath: "demo/-/demo-" + version + ".tgz", CreatedAt: now},
			map[string]string{tag: version}, "alice")
	}
	require.NoError(t, publish("1.0.0", "latest"))
	require.NoError(t, publish("2.0.0", "latest"))
	lock := &core.ResourceLock{Format: "npm", Repository: "npm", Name: "demo", Version: "2.0.0",
		Source: core.ResourceLockSystem, Mode: core.ResourceLockRead, Reason: "trojan", LockedAt: now}
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	for _, viewer := range []struct {
		name                     string
		administrator, moderator bool
		latest                   string
		count                    int
	}{
		{"guest", false, false, "1.0.0", 1}, {"outsider", true, false, "1.0.0", 1},
		{"reader", false, false, "2.0.0", 2}, {"outsider", false, true, "2.0.0", 2},
	} {
		packages, total, err := db.ListNPMPackages("npm", viewer.name, viewer.administrator, viewer.moderator, 10, 0)
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, viewer.latest, packages[0].LatestVersion)
		require.Equal(t, viewer.count, packages[0].VersionCount)
	}
	visible, err := db.ResourceMetadataVisibility("npm", "npm", "guest", false, []core.ResourceLockTarget{lock.ResourceLockTarget})
	require.NoError(t, err)
	require.Equal(t, []bool{false}, visible)
	visible, err = db.ResourceMetadataVisibility("npm", "npm", "reader", false, []core.ResourceLockTarget{lock.ResourceLockTarget})
	require.NoError(t, err)
	require.Equal(t, []bool{true}, visible)
	require.ErrorIs(t, db.SetNPMVersionDeprecated("npm", "demo", "2.0.0", "changed", "alice", 0), core.ErrResourceLocked)
	_, err = db.UnpublishNPMVersion("npm", "demo", "2.0.0", "alice", 0)
	require.ErrorIs(t, err, core.ErrResourceLocked)
	require.ErrorIs(t, db.SetNPMDistTag("npm", "demo", "latest", "1.0.0", "alice", 0), core.ErrResourceLocked)
	require.ErrorIs(t, db.DeleteNPMDistTag("npm", "demo", "latest", "alice", 0), core.ErrResourceLocked)
	require.NoError(t, db.UpdateNPMPackument("npm", "demo", "alice", 0,
		map[string]string{"1.0.0": "old", "2.0.0": ""}, map[string]string{"latest": "2.0.0"}))
	require.ErrorIs(t, db.UpdateNPMPackument("npm", "demo", "alice", 0,
		map[string]string{"1.0.0": "rollback", "2.0.0": "changed"}, map[string]string{"latest": "2.0.0"}), core.ErrResourceLocked)
	require.ErrorIs(t, db.UpdateNPMPackument("npm", "demo", "alice", 0,
		nil, map[string]string{"latest": "1.0.0"}), core.ErrResourceLocked)
	previous, err := db.GetNPMPackage("npm", "demo")
	require.NoError(t, err)
	require.NoError(t, publish("3.0.0", "canary"))
	require.NoError(t, db.UpdateNPMPackageDescription("npm", "demo", "updated", "alice"))
	_, err = db.DeleteNPMPackage("npm", "demo", "alice", 0)
	require.ErrorIs(t, err, core.ErrResourceLocked)
	require.ErrorIs(t, db.SetNPMPackageArchived("npm", "demo", "alice", true), core.ErrResourceLocked)
	lock.Version, lock.Mode = "", core.ResourceLockWrite
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	require.ErrorIs(t, publish("4.0.0", "next"), core.ErrResourceLocked)
	require.ErrorIs(t, db.ForceAddNPMMembers("npm", "demo", "alice", []string{"outsider"}, 0), core.ErrResourceLocked)
	require.ErrorIs(t, db.RemoveNPMMember("npm", "demo", "reader", "reader"), core.ErrResourceLocked)
	require.NoError(t, db.RollbackNPMPublicationReview("npm", "demo", "3.0.0", previous, map[string]string{"latest": "2.0.0"}))
	lock.Mode = core.ResourceLockRead
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	packages, total, err := db.ListNPMPackages("npm", "outsider", true, false, 10, 0)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, packages)
}
