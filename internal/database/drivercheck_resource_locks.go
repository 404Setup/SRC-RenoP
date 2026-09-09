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
	"strings"
	"time"

	"renop/internal/core"
)

func checkMavenLocks(db *DB, repository, domain, owner, moderator, session string, now int64) error {
	pkg := &core.MavenArtifact{Repository: repository, Domain: domain, GroupID: domain,
		ArtifactID: "lock-demo", CreatedAt: now, UpdatedAt: now}
	for _, version := range []string{"1.0", "2.0-SNAPSHOT"} {
		if err := db.RecordMavenPublication(pkg, &core.MavenVersion{Version: version, Publisher: owner, Size: 10, CreatedAt: now}); err != nil {
			return err
		}
	}
	target := mavenLockTarget(repository, domain, pkg.ArtifactID, "2.0-SNAPSHOT")
	lock := &core.ResourceLock{ResourceLockTarget: target, Source: core.ResourceLockManual,
		Mode: core.ResourceLockRead, Reason: "trojan", LockedAt: now}
	if err := db.SetResourceLock(lock, moderator, session); err != nil {
		return err
	}
	for _, path := range []string{"lock-demo/maven-metadata.xml", "lock-demo/2.0-SNAPSHOT/other.bin"} {
		locks, err := db.GetMavenPathLocks(repository, strings.ReplaceAll(domain, ".", "/")+"/"+path, false)
		if err != nil || len(locks) != 1 {
			return errorsOrMissing(err, "Maven metadata and companion lock resolution")
		}
	}
	if err := db.DeleteMavenVersionMetadata(repository, domain, pkg.ArtifactID, target.Version); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "Maven locked version deletion")
	}
	for _, viewer := range []string{"", owner} {
		artifacts, total, err := db.ListReadableMavenArtifacts([]string{repository}, "", pkg.ArtifactID, viewer, nil, 10, 0)
		if err != nil || total != 1 || len(artifacts) != 1 {
			return errorsOrMissing(err, "Maven filtered artifact listing")
		}
		if viewer == "" && (artifacts[0].VersionCount != 1 || artifacts[0].LatestVersion != "1.0" || artifacts[0].TotalSize != 10) {
			return errors.New("Maven locked version exposed in aggregates")
		}
		visible, err := db.ResourceMetadataVisibility("maven", repository, viewer, false, []core.ResourceLockTarget{target})
		if err != nil || len(visible) != 1 || visible[0] != (viewer == owner) {
			return errorsOrMissing(err, "Maven owner metadata visibility")
		}
	}
	lock.Version = ""
	if err := db.SetResourceLock(lock, moderator, session); err != nil {
		return err
	}
	if err := db.UpdateMavenArtifactReadme(repository, domain, pkg.ArtifactID, "blocked"); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "Maven locked README mutation")
	}
	for _, viewer := range []string{"", owner} {
		domains, err := db.ListMavenRepositoryDomains(repository, viewer, false)
		if err != nil || len(domains) != 1 {
			return errorsOrMissing(err, "Maven visible domain counts")
		}
		want := 1
		if viewer == owner {
			want = 2
		}
		if domains[0].ArtifactCount != want {
			return fmt.Errorf("Maven visible domain artifact count: got %d, want %d", domains[0].ArtifactCount, want)
		}
		searched, total, err := db.SearchMavenRepositoryDomains(repository, domain, viewer, false, 10)
		if err != nil || total != 1 || len(searched) != 1 || searched[0].ArtifactCount != want {
			return errorsOrMissing(err, "Maven domain search aggregates")
		}
	}
	if err := db.DeleteResourceLock(target, core.ResourceLockManual, moderator, session); err != nil {
		return err
	}
	return db.DeleteResourceLock(lock.ResourceLockTarget, core.ResourceLockManual, moderator, session)
}

func checkResourceLocks(db *DB, repository, npmRepository, dockerRepository, mavenRepository, mavenDomain, prefix, owner, suffix string, now int64) error {
	moderator := "lockmod-" + suffix
	if err := db.SaveToken(&core.AccessToken{Name: moderator, Permissions: []string{"canmoderate:" + repository, "canmoderate:" + npmRepository, "canmoderate:" + dockerRepository, "canmoderate:" + mavenRepository}}); err != nil {
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
	if err := checkDockerLocks(db, dockerRepository, prefix, owner, moderator, sessionToken, now); err != nil {
		return err
	}
	if err := checkMavenLocks(db, mavenRepository, mavenDomain, owner, moderator, sessionToken, now); err != nil {
		return err
	}
	if err := checkSuperTeamLocks(db, repository, npmRepository, dockerRepository, mavenRepository, mavenDomain, prefix, owner, now); err != nil {
		return err
	}
	return checkMavenDomainLocks(db, mavenRepository, mavenDomain, owner, now)
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

func checkSuperTeamLocks(db *DB, cargo, npm, docker, maven, domain, prefix, owner string, now int64) error {
	lock := &core.ResourceLock{ResourceLockTarget: core.ResourceLockTarget{Format: "superteam", Name: prefix},
		Source: core.ResourceLockSystem, Mode: core.ResourceLockRead, Reason: "abuse", LockedAt: now}
	if err := db.SetResourceLock(lock, "", ""); err != nil {
		return err
	}
	for _, target := range []core.ResourceLockTarget{
		{Format: "cargo", Repository: cargo, Name: "lock-demo"},
		{Format: "npm", Repository: npm, Name: "npm-lock-demo"},
		{Format: "docker", Repository: docker, Name: "docker-lock-demo"},
		{Format: "maven", Repository: maven, Name: domain + ":lock-demo"},
		{Format: "maven-domain", Name: domain},
	} {
		locks, err := db.GetResourceLocks(target, true)
		if err != nil || len(locks) != 1 || !locks[0].Inherited {
			return errorsOrMissing(err, "inherited "+target.Format+" team lock")
		}
		if err := db.EnsureResourceMutable(target, false); !errors.Is(err, core.ErrResourceLocked) {
			return errorsOrMissing(err, "inherited "+target.Format+" mutation denial")
		}
		for _, viewer := range []string{"", owner} {
			visible, err := db.ResourceMetadataVisibility(target.Format, target.Repository, viewer, false, []core.ResourceLockTarget{target})
			if err != nil || len(visible) != 1 || visible[0] != (viewer == owner) {
				return errorsOrMissing(err, "inherited "+target.Format+" metadata visibility")
			}
		}
		if target.Repository != "" {
			if err := db.EnsureRepositoryResourcesMutable(target.Repository); !errors.Is(err, core.ErrResourceLocked) {
				return errorsOrMissing(err, "inherited repository restriction")
			}
		}
	}
	details, err := db.GetPublicSuperTeamDetails(prefix, owner, false, false)
	if err != nil || len(details.Team.Locks) != 1 {
		return errorsOrMissing(err, "locked team metadata")
	}
	if _, err := db.GetPublicSuperTeamDetails(prefix, "", false, false); !errors.Is(err, core.ErrSuperTeamNotFound) {
		return errorsOrMissing(err, "locked team public visibility")
	}
	if err := db.UpdateSuperTeam(prefix, owner, "Blocked", "", core.PublicLinks{}, false, now); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "locked team update")
	}
	domainDetails, err := db.GetMavenDomainDetails(domain, owner)
	if err != nil || len(domainDetails.Domain.Locks) != 1 {
		return errorsOrMissing(err, "locked domain metadata")
	}
	path := strings.ReplaceAll(domain, ".", "/") + "/uncatalogued/file.bin"
	locks, err := db.GetMavenPathLocks(maven, path, false)
	if err != nil || !core.ReadLocked(locks) {
		return errorsOrMissing(err, "uncatalogued domain file restriction")
	}
	for _, viewer := range []string{"", owner} {
		visible, err := db.MavenDomainPathVisibility(maven, viewer, false, []string{path})
		if err != nil || len(visible) != 1 || visible[0] != (viewer == owner) {
			return errorsOrMissing(err, "uncatalogued domain path visibility")
		}
	}
	if err := db.ReserveMavenVerificationAttempt(domain, owner, false, now+1, now); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "locked domain verification denial")
	}
	if err := db.DeleteResourceLock(lock.ResourceLockTarget, core.ResourceLockSystem, "", ""); err != nil {
		return err
	}
	return db.EnsureResourceMutable(core.ResourceLockTarget{Format: "maven", Repository: maven, Name: domain + ":lock-demo"}, false)
}

func checkMavenDomainLocks(db *DB, repository, domain, owner string, now int64) error {
	target := core.ResourceLockTarget{Format: "maven-domain", Name: domain}
	if err := db.SetResourceLock(&core.ResourceLock{ResourceLockTarget: target, Source: core.ResourceLockSystem,
		Mode: core.ResourceLockRead, Reason: "prohibited", LockedAt: now}, "", ""); err != nil {
		return err
	}
	artifact := mavenLockTarget(repository, domain, "lock-demo", "1.0")
	locks, err := db.GetResourceLocks(artifact, false)
	if err != nil || len(locks) != 1 || !locks[0].Inherited {
		return errorsOrMissing(err, "Maven domain lock inheritance")
	}
	if err := db.EnsureResourceMutable(artifact, false); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "Maven domain publication freeze")
	}
	if err := db.EnsureRepositoryResourcesMutable(repository); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "Maven domain repository freeze")
	}
	for _, viewer := range []string{"", owner} {
		visible, err := db.ResourceMetadataVisibility("maven", repository, viewer, false, []core.ResourceLockTarget{artifact})
		if err != nil || len(visible) != 1 || visible[0] != (viewer == owner) {
			return errorsOrMissing(err, "Maven domain artifact metadata visibility")
		}
		path := strings.ReplaceAll(domain, ".", "/") + "/uncatalogued/file.bin"
		visible, err = db.MavenDomainPathVisibility(repository, viewer, false, []string{path})
		if err != nil || len(visible) != 1 || visible[0] != (viewer == owner) {
			return errorsOrMissing(err, "Maven domain uncatalogued metadata visibility")
		}
	}
	if err := db.ForceAddMavenMembers(domain, owner, []string{owner}, core.MavenPermissionOwner); !errors.Is(err, core.ErrResourceLocked) {
		return errorsOrMissing(err, "Maven locked domain membership preservation")
	}
	if err := db.DeleteResourceLock(target, core.ResourceLockSystem, "", ""); err != nil {
		return err
	}
	return db.EnsureResourceMutable(artifact, false)
}
