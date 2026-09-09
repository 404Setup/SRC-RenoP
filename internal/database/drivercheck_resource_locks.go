/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"renop/internal/core"
)

func checkResourceLocks(db *DB, repository, npmRepository, dockerRepository, prefix, owner, suffix string, now int64) error {
	moderator := "lockmod-" + suffix
	if err := db.SaveToken(&core.AccessToken{Name: moderator, Permissions: []string{"canmoderate:" + repository, "canmoderate:" + npmRepository, "canmoderate:" + dockerRepository}}); err != nil {
		return err
	}
	sessionToken := "lock-session-" + suffix
	session := &core.Session{PublicID: "lock-" + suffix, Username: moderator, CreatedAt: now}
	session.LastActive.Store(time.Now().UnixMilli())
	if err := db.SaveSession(session, sessionToken); err != nil {
		return err
	}
	pkg := &core.CargoPackage{Repository: repository, Name: "lock-demo", NormalizedName: "lock-demo",
		SuperTeamPrefix: prefix, CreatedAt: now, UpdatedAt: now}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		if err := db.RecordCargoPublication(pkg, &core.CargoVersion{Repository: repository, Package: pkg.Name,
			Version: version, Publisher: owner, CreatedAt: now}, owner); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`UPDATE cargo_packages SET super_team_prefix = ? WHERE repository = ? AND normalized_name = ?`,
		prefix, repository, pkg.Name); err != nil {
		return err
	}
	target := core.ResourceLockTarget{Format: "cargo", Repository: repository, Name: pkg.Name, Version: "2.0.0"}
	lock := &core.ResourceLock{ResourceLockTarget: target, Source: core.ResourceLockManual,
		Mode: core.ResourceLockRead, Reason: "trojan", LockedAt: now}
	if err := db.SetResourceLock(lock, moderator, sessionToken); err != nil {
		return err
	}
	if err := db.EnsureResourceMutable(target, false); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "locked version mutation denial")
	}
	packages, total, err := db.SearchCargoPackages(repository, pkg.Name, "", false, 10, 0)
	if err != nil || total != 1 || len(packages) != 1 || packages[0].MaxVersion != "1.0.0" {
		return errorsOrMissing(err, "visible latest Cargo version")
	}
	visible, err := db.ResourceMetadataVisibility("cargo", repository, "", false, []core.ResourceLockTarget{target})
	if err != nil || len(visible) != 1 || visible[0] {
		return errorsOrMissing(err, "locked Cargo path filtering")
	}
	lock.Source = core.ResourceLockSystem
	if err := db.SetResourceLock(lock, "", ""); err != nil {
		return err
	}
	if err := db.DeleteResourceLock(target, core.ResourceLockManual, moderator, sessionToken); err != nil {
		return err
	}
	locks, err := db.GetResourceLocks(target, false)
	if err != nil || len(locks) != 1 || locks[0].Source != core.ResourceLockSystem {
		return errorsOrMissing(err, "independent system lock")
	}
	if err := db.DeleteResourceLock(target, core.ResourceLockSystem, "", ""); err != nil {
		return err
	}
	target.Version = ""
	lock.ResourceLockTarget, lock.Source = target, core.ResourceLockManual
	if err := db.SetResourceLock(lock, moderator, sessionToken); err != nil {
		return err
	}
	packages, total, err = db.SearchCargoPackages(repository, pkg.Name, "", false, 10, 0)
	if err != nil || total != 0 || len(packages) != 0 {
		return errorsOrMissing(err, "locked Cargo search totals")
	}
	profile, err := db.GetUserProfile(owner)
	if err != nil {
		return err
	}
	memberships, err := db.ListUserPackageMemberships(profile.UserID, "cargo", "", nil)
	if err != nil {
		return err
	}
	for _, membership := range memberships {
		if membership.Repository == repository && membership.Name == pkg.Name {
			return errors.New("locked Cargo package exposed on public profile")
		}
	}
	options := core.SuperTeamResourceListOptions{Prefix: prefix, Format: "cargo", VisibleRepositories: []string{repository}, Limit: 10}
	_, total, err = db.ListSuperTeamResources(options)
	if err != nil || total != 0 {
		return errorsOrMissing(err, "locked global-team resource filtering")
	}
	options.ModeratedRepositories = []string{repository}
	_, total, err = db.ListSuperTeamResources(options)
	if err != nil || total != 1 {
		return errorsOrMissing(err, "moderator global-team resource visibility")
	}
	if err := db.EnsureRepositoryResourcesMutable(repository); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "locked repository reconfiguration denial")
	}
	if err := db.DeleteResourceLock(target, core.ResourceLockManual, moderator, sessionToken); err != nil {
		return err
	}
	if err := checkNPMLocks(db, npmRepository, prefix, owner, moderator, sessionToken, now); err != nil {
		return err
	}
	return checkDockerLocks(db, dockerRepository, prefix, owner, moderator, sessionToken, now)
}

func checkDockerLocks(db *DB, repository, prefix, owner, moderator, session string, now int64) error {
	image, err := db.CreateDockerImageForTeam(repository, "docker-lock-demo", owner, prefix, false, now)
	if err != nil {
		return err
	}
	digestOf := func(raw string) string { return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(raw))) }
	blob := digestOf("layer")
	childRaw := fmt.Sprintf(`{"schemaVersion":2,"layers":[{"digest":%q}]}`, blob)
	child := digestOf(childRaw)
	indexRaw := fmt.Sprintf(`{"schemaVersion":2,"manifests":[{"digest":%q}]}`, child)
	index := digestOf(indexRaw)
	put := func(digest, raw, tag string, blobs ...string) error {
		return db.PutDockerManifest(&core.DockerManifest{Repository: repository, ImageName: image.ImageName,
			Digest: digest, MediaType: "application/vnd.oci.image.manifest.v1+json", RawJSON: []byte(raw), BlobDigests: blobs}, tag, owner)
	}
	if err := put(child, childRaw, "amd64", blob); err != nil {
		return err
	}
	if err := put(index, indexRaw, "latest"); err != nil {
		return err
	}
	if err := put(digestOf(`{}`), `{}`, "unlocked"); err != nil {
		return err
	}
	lock := &core.ResourceLock{ResourceLockTarget: dockerLockTarget(repository, image.ImageName, index),
		Source: core.ResourceLockManual, Mode: core.ResourceLockRead, Reason: "trojan", LockedAt: now}
	if err := db.SetResourceLock(lock, moderator, session); err != nil {
		return err
	}
	for _, digest := range []string{index, child, blob} {
		target := dockerLockTarget(repository, image.ImageName, digest)
		locks, err := db.GetResourceLocks(target, false)
		if err != nil || len(locks) != 1 || locks[0].Inherited != (digest != index) {
			return errorsOrMissing(err, "Docker index lock inheritance")
		}
		visible, err := db.ResourceMetadataVisibility("docker", repository, "", false, []core.ResourceLockTarget{target})
		if err != nil || len(visible) != 1 || visible[0] {
			return errorsOrMissing(err, "Docker locked digest visibility")
		}
		if err := db.EnsureResourceMutable(target, false); !errors.Is(err, core.ErrResourceLocked) {
			return errorsOrMissing(err, "Docker inherited mutation denial")
		}
	}
	if err := db.DeleteDockerTag(repository, image.ImageName, "latest"); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "Docker locked tag mutation")
	}
	if err := db.EnsureDockerBlobMutable(repository, blob); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "Docker shared blob protection")
	}
	for _, viewer := range []struct {
		name  string
		count int
	}{{"", 1}, {owner, 3}} {
		tags, err := db.ListDockerTagsForViewer(repository, image.ImageName, "", 10, viewer.name, false)
		if err != nil || len(tags) != viewer.count {
			return errorsOrMissing(err, "Docker visible tag pagination")
		}
		if err := db.FilterDockerImageVersions([]*core.DockerRepositoryImage{image}, viewer.name, false); err != nil {
			return err
		}
		if image.TagCount != viewer.count {
			return fmt.Errorf("Docker visible tag count: got %d, want %d", image.TagCount, viewer.count)
		}
		details, err := db.GetDockerImageDetailsForViewer(repository, image.ImageName, viewer.name, false)
		if err != nil || details == nil || len(details.Tags) != viewer.count {
			return errorsOrMissing(err, "Docker visible image details")
		}
	}
	options := core.SuperTeamResourceListOptions{Prefix: prefix, Format: "docker", VisibleRepositories: []string{repository}, Limit: 10}
	_, before, err := db.ListSuperTeamResources(options)
	if err != nil {
		return err
	}
	lock.Version = ""
	if err := db.SetResourceLock(lock, moderator, session); err != nil {
		return err
	}
	_, after, err := db.ListSuperTeamResources(options)
	if err != nil || after != before-1 {
		return errorsOrMissing(err, "Docker locked team resources")
	}
	profile, err := db.GetUserProfile(owner)
	if err != nil {
		return err
	}
	resources, err := db.ListUserPackageMemberships(profile.UserID, "docker", "", nil)
	if err != nil {
		return err
	}
	for _, resource := range resources {
		if resource.Repository == repository && resource.Name == image.ImageName {
			return errors.New("Docker locked image exposed on public profile")
		}
	}
	if err := db.DeleteResourceLock(lock.ResourceLockTarget, core.ResourceLockManual, moderator, session); err != nil {
		return err
	}
	return db.DeleteResourceLock(dockerLockTarget(repository, image.ImageName, index), core.ResourceLockManual, moderator, session)
}

func checkNPMLocks(db *DB, repository, prefix, owner, moderator, session string, now int64) error {
	pkg, err := db.CreateNPMPackageForTeam(repository, "npm-lock-demo", owner, prefix, false, now)
	if err != nil {
		return err
	}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		if err := db.RecordNPMPublication(pkg, &core.NPMVersion{Repository: repository, Package: pkg.Name, Version: version,
			ManifestJSON: `{"name":"npm-lock-demo","version":"` + version + `"}`, CreatedAt: now}, map[string]string{"latest": version}, owner); err != nil {
			return err
		}
	}
	target := npmLockTarget(repository, pkg.Name, "2.0.0")
	lock := &core.ResourceLock{ResourceLockTarget: target, Source: core.ResourceLockManual,
		Mode: core.ResourceLockRead, Reason: "abuse", LockedAt: now}
	if err := db.SetResourceLock(lock, moderator, session); err != nil {
		return err
	}
	packages, total, err := db.SearchNPMPackages(repository, pkg.Name, "", true, false, 10, 0)
	if err != nil || total != 1 || len(packages) != 1 || packages[0].LatestVersion != "1.0.0" || packages[0].VersionCount != 1 {
		return errorsOrMissing(err, "npm locked-version search metadata")
	}
	if err := db.DeleteNPMDistTag(repository, pkg.Name, "latest", owner, 0); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "npm locked-version tag deletion")
	}
	if err := db.UpdateNPMPackument(repository, pkg.Name, owner, 0,
		map[string]string{"1.0.0": "old", "2.0.0": ""}, map[string]string{"latest": "2.0.0"}); err != nil {
		return err
	}
	if err := db.UpdateNPMPackument(repository, pkg.Name, owner, 0,
		map[string]string{"2.0.0": "changed"}, map[string]string{"latest": "2.0.0"}); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "npm locked-version bulk metadata mutation")
	}
	visible, err := db.ResourceMetadataVisibility("npm", repository, "", false, []core.ResourceLockTarget{target})
	if err != nil || len(visible) != 1 || visible[0] {
		return errorsOrMissing(err, "npm locked path visibility")
	}
	visible, err = db.ResourceMetadataVisibility("npm", repository, owner, false, []core.ResourceLockTarget{target})
	if err != nil || len(visible) != 1 || !visible[0] {
		return errorsOrMissing(err, "npm locked path member visibility")
	}
	options := core.SuperTeamResourceListOptions{Prefix: prefix, Format: "npm", VisibleRepositories: []string{repository}, Limit: 10}
	_, before, err := db.ListSuperTeamResources(options)
	if err != nil {
		return err
	}
	lock.Version = ""
	if err := db.SetResourceLock(lock, moderator, session); err != nil {
		return err
	}
	if err := db.UpdateNPMPackageDescription(repository, pkg.Name, "changed", owner); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "npm package mutation under lock")
	}
	_, total, err = db.ListSuperTeamResources(options)
	if err != nil || total != before-1 {
		return errorsOrMissing(err, "npm locked team-resource totals")
	}
	profile, err := db.GetUserProfile(owner)
	if err != nil {
		return err
	}
	memberships, err := db.ListUserPackageMemberships(profile.UserID, "npm", "", nil)
	if err != nil {
		return err
	}
	for _, membership := range memberships {
		if membership.Repository == repository && membership.Name == pkg.Name {
			return errors.New("npm locked package exposed on public profile")
		}
	}
	if err := db.DeleteResourceLock(target, core.ResourceLockManual, moderator, session); err != nil {
		return err
	}
	return db.DeleteResourceLock(lock.ResourceLockTarget, core.ResourceLockManual, moderator, session)
}
