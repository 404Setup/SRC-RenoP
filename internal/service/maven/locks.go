/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/repositorygate"
	"renop/internal/utils"
)

func mavenLockTarget(repository, group, artifact, version string) core.ResourceLockTarget {
	return core.ResourceLockTarget{Format: "maven", Repository: repository, Name: group + ":" + artifact, Version: version}
}

// EnsurePathMutable covers version directories, arbitrary companions, and shared metadata.
func EnsurePathMutable(state *core.AppState, repo *config.Repository, path string) error {
	if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatMaven {
		return nil
	}
	if state == nil || state.GetDB() == nil {
		return core.ErrDatabaseUnavailable
	}
	locks, err := state.GetDB().GetMavenPathLocks(repo.Name, path, true)
	if err != nil {
		return err
	}
	if len(locks) > 0 {
		return core.ErrResourceLocked
	}
	if group, artifact, ok := pathArtifactCandidate(path); ok {
		return state.GetDB().EnsurePackageMutable(config.RepositoryFormatMaven, repo.Name, group+":"+artifact)
	}
	return nil
}

func mavenPathTargets(repository, path string) ([]core.ResourceLockTarget, error) {
	parts := strings.Split(strings.Trim(strings.ReplaceAll(path, `\`, "/"), "/"), "/")
	if len(parts) > core.MaxMavenPathParts {
		return nil, core.ErrResourceLockInvalid
	}
	targets := make([]core.ResourceLockTarget, 0, len(parts))
	for j := 2; j < len(parts); j++ {
		version := ""
		if j+1 < len(parts) && !strings.HasPrefix(strings.ToLower(parts[j+1]), "maven-metadata.xml") {
			version = parts[j+1]
		}
		target := mavenLockTarget(repository, strings.ToLower(strings.Join(parts[:j], ".")), parts[j], version)
		target.Name = core.ResourceLockVersionKey("maven", target.Name)
		target.Version = core.ResourceLockVersionKey("maven", version)
		targets = append(targets, target)
	}
	return targets, nil
}

// MetadataPathFilter takes one bounded lock snapshot for repository-wide index scans.
func MetadataPathFilter(state *core.AppState, user *config.User, repository string) (func(string) bool, error) {
	if state == nil || state.GetDB() == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	locks, err := state.GetDB().GetMavenPathLocks(repository, "", true)
	if err != nil {
		return nil, err
	}
	targets := make([]core.ResourceLockTarget, 0, len(locks))
	for _, lock := range locks {
		if lock.Mode == core.ResourceLockRead {
			targets = append(targets, lock.ResourceLockTarget)
		}
	}
	hidden := make(map[core.ResourceLockTarget]bool)
	username, moderator := "", false
	if user != nil {
		username, moderator = user.Username, user.CheckModeratePermission(repository)
	}
	for start := 0; start < len(targets); start += 128 {
		batch := targets[start:min(start+128, len(targets))]
		visible, err := state.GetDB().ResourceMetadataVisibility("maven", repository, username, moderator, batch)
		if err != nil {
			return nil, err
		}
		for i, allowed := range visible {
			if !allowed {
				target := batch[i]
				target.Version = core.ResourceLockVersionKey("maven", target.Version)
				hidden[target] = true
			}
		}
	}
	return func(path string) bool {
		if len(hidden) == 0 {
			return true
		}
		targets, err := mavenPathTargets(strings.ToLower(repository), path)
		if err != nil {
			return false
		}
		for _, target := range targets {
			if hidden[target] {
				return false
			}
			target.Version = ""
			if hidden[target] {
				return false
			}
		}
		return true
	}, nil
}

// VisibleMetadataPaths applies all possible artifact ancestors in bounded database batches.
func VisibleMetadataPaths(state *core.AppState, user *config.User, repository string, paths []string) ([]bool, error) {
	if state == nil || state.GetDB() == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	username, moderator := "", false
	if user != nil {
		username, moderator = user.Username, user.CheckModeratePermission(repository)
	}
	visible := make([]bool, len(paths))
	targets, indexes := make([]core.ResourceLockTarget, 0, 128), make([]int, 0, 128)
	flush := func() error {
		result, err := state.GetDB().ResourceMetadataVisibility("maven", repository, username, moderator, targets)
		if err != nil {
			return err
		}
		for i, allowed := range result {
			visible[indexes[i]] = visible[indexes[i]] && allowed
		}
		targets, indexes = targets[:0], indexes[:0]
		return nil
	}
	for i, path := range paths {
		visible[i] = true
		candidates, err := mavenPathTargets(repository, path)
		if err != nil {
			return nil, err
		}
		for _, candidate := range candidates {
			targets = append(targets, candidate)
			indexes = append(indexes, i)
			if len(targets) == 128 {
				if err := flush(); err != nil {
					return nil, err
				}
			}
		}
	}
	if len(targets) > 0 {
		if err := flush(); err != nil {
			return nil, err
		}
	}
	return visible, nil
}

func applyArtifactLocks(state *core.AppState, details *core.MavenArtifactDetails) error {
	artifact := details.Artifact
	locks, err := state.GetDB().GetResourceLocks(mavenLockTarget(artifact.Repository, artifact.GroupID, artifact.ArtifactID, ""), true)
	if err != nil || len(locks) == 0 {
		return err
	}
	byVersion := make(map[string][]*core.ResourceLock)
	for _, lock := range locks {
		if lock.Version == "" {
			artifact.Locks = append(artifact.Locks, lock)
		} else {
			artifact.VersionLocked = true
			key := core.ResourceLockVersionKey("maven", lock.Version)
			byVersion[key] = append(byVersion[key], lock)
		}
	}
	inspect := details.Member || details.Moderator
	if core.ReadLocked(artifact.Locks) && !inspect {
		return core.ErrMavenArtifactNotFound
	}
	versions := details.Versions[:0]
	artifact.LatestVersion, artifact.VersionCount, artifact.TotalSize = "", 0, 0
	for _, version := range details.Versions {
		version.Locks = byVersion[core.ResourceLockVersionKey("maven", version.Version)]
		if core.ReadLocked(version.Locks) && !inspect {
			continue
		}
		versions = append(versions, version)
		if version.ReviewStatus == "" {
			artifact.VersionCount++
			artifact.TotalSize += version.Size
			if artifact.LatestVersion == "" || utils.CompareVersions(version.Version, artifact.LatestVersion) > 0 {
				artifact.LatestVersion = version.Version
			}
		}
	}
	details.Versions = versions
	return nil
}

func setResourceLockAPI(c fiber.Ctx, state *core.AppState) error {
	repo, err := repository(c, state)
	if err != nil {
		return apiError(c, err)
	}
	user, session := auth.GetUser(c), auth.CurrentSessionToken(c)
	if user == nil || !user.CheckModeratePermission(repo.Name) || auth.CurrentCredentialKind(c) != "session" || session == "" || c.Cookies("renop_session") != session {
		return apiError(c, core.ErrResourceLockPermission)
	}
	group, artifact := c.Query("group"), c.Query("artifact")
	var request struct {
		Version string `json:"version"`
		Mode    string `json:"mode"`
		Reason  string `json:"reason"`
	}
	if utils.ReadJSONLimited(c, &request, 4096) != nil || group == "" || artifact == "" {
		return apiError(c, core.ErrResourceLockInvalid)
	}
	release := repositorygate.AcquireMigration(repo.Name)
	defer release()
	details, err := state.GetDB().GetMavenArtifactDetails(repo.Name, group, artifact)
	if err != nil {
		return apiError(c, err)
	}
	if request.Version != "" {
		found := false
		for _, version := range details.Versions {
			found = found || version.Version == request.Version
		}
		if !found {
			return apiError(c, core.ErrMavenVersionNotFound)
		}
	}
	target := mavenLockTarget(repo.Name, group, artifact, request.Version)
	action := audit.ActionResourceLock
	if c.Method() == fiber.MethodDelete {
		action = audit.ActionResourceUnlock
		err = state.GetDB().DeleteResourceLock(target, core.ResourceLockManual, user.Username, session)
	} else {
		err = state.GetDB().SetResourceLock(&core.ResourceLock{ResourceLockTarget: target, Source: core.ResourceLockManual,
			Mode: request.Mode, Reason: request.Reason, LockedAt: time.Now().UnixMilli()}, user.Username, session)
	}
	if err != nil {
		return apiError(c, err)
	}
	logAudit(c, state, action, "Repository: "+repo.Name+", package: "+target.Name+", version: "+request.Version+", reason: "+request.Reason)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"ok": true})
}
