/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/testutil"
)

func TestMavenLocksCoverMetadataCompanionsAndCatalogMutations(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	require.NoError(t, db.CreateMavenDomain(&core.MavenDomain{Domain: "com.example", VerificationType: "dns",
		VerificationHost: "example.com", VerificationCode: "proof", CreatedAt: now}, "alice"))
	require.NoError(t, db.MarkMavenDomainVerified("com.example", "proof", now))
	require.NoError(t, db.ForceAddMavenMembers("com.example", "alice", []string{"bob"}, 0))
	publish := func(version string) error {
		return db.RecordMavenPublication(&core.MavenArtifact{Repository: "maven", Domain: "com.example",
			GroupID: "com.example", ArtifactID: "demo", Publisher: "alice", CreatedAt: now},
			&core.MavenVersion{Version: version, Publisher: "alice", CreatedAt: now})
	}
	require.NoError(t, publish("1.0-SNAPSHOT"))
	require.NoError(t, publish("2.0"))
	lock := &core.ResourceLock{ResourceLockTarget: core.ResourceLockTarget{Format: "maven", Repository: "maven",
		Name: "com.example:demo", Version: "1.0-SNAPSHOT"}, Source: core.ResourceLockSystem,
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
	lock := &core.ResourceLock{ResourceLockTarget: core.ResourceLockTarget{Format: "docker", Repository: "docker", Name: "demo", Version: index},
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
	lock := &core.ResourceLock{ResourceLockTarget: core.ResourceLockTarget{Format: "npm", Repository: "npm", Name: "demo", Version: "2.0.0"},
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
