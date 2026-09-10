/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"path"
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

func npmLockTarget(repository, name, version string) core.ResourceLockTarget {
	return core.ResourceLockTarget{Format: "npm", Repository: repository, Name: name, Version: version}
}

func npmPathResource(value string) (name, version string) {
	decoded, valid := decodeRegistryPath(strings.ReplaceAll(value, `\`, "/"))
	if !valid {
		return "", ""
	}
	if name, valid = packageFromTarballPath(decoded); valid {
		filename, prefix := path.Base(decoded), path.Base(name)+"-"
		if len(filename) > len(prefix)+4 && strings.EqualFold(filename[:len(prefix)], prefix) && strings.EqualFold(filename[len(filename)-4:], ".tgz") {
			candidate := filename[len(prefix) : len(filename)-4]
			if validNPMVersion(candidate) {
				version = candidate
			}
		}
		return
	}
	name, _ = NormalizePackageName(strings.TrimSuffix(decoded, "/-"))
	return name, ""
}

// VisibleMetadataPaths applies package and version restrictions to a file-browser page.
func VisibleMetadataPaths(state *core.AppState, user *config.User, repository string, paths []string) ([]bool, error) {
	visible := make([]bool, len(paths))
	for start := 0; start < len(paths); start += 128 {
		end := min(start+128, len(paths))
		targets, indexes := make([]core.ResourceLockTarget, 0, end-start), make([]int, 0, end-start)
		for i := start; i < end; i++ {
			visible[i] = true
			if name, version := npmPathResource(paths[i]); name != "" {
				targets = append(targets, npmLockTarget(repository, name, version))
				indexes = append(indexes, i)
			}
		}
		if len(targets) == 0 {
			continue
		}
		if state == nil || state.GetDB() == nil {
			return nil, core.ErrDatabaseUnavailable
		}
		result, err := state.GetDB().ResourceMetadataVisibility("npm", repository, user.Username, user.CheckModeratePermission(repository), targets)
		if err != nil {
			return nil, err
		}
		for i, allowed := range result {
			visible[indexes[i]] = allowed
		}
	}
	return visible, nil
}

func applyPackageLocks(state *core.AppState, user *config.User, details *core.NPMPackageDetails) (bool, error) {
	pkg := details.Package
	locks, err := state.GetDB().GetResourceLocks(npmLockTarget(pkg.Repository, pkg.Name, ""), true)
	if err != nil || len(locks) == 0 {
		return false, err
	}
	inspect := details.Member || user != nil && user.CheckModeratePermission(pkg.Repository)
	byVersion := make(map[string][]*core.ResourceLock)
	for _, lock := range locks {
		if lock.Version == "" {
			pkg.Locks = append(pkg.Locks, lock)
		} else {
			key := core.ResourceLockVersionKey("npm", lock.Version)
			byVersion[key] = append(byVersion[key], lock)
		}
	}
	if core.ReadLocked(pkg.Locks) && !inspect {
		return true, core.ErrNPMPackageNotFound
	}
	versions := details.Versions[:0]
	hidden := make(map[string]bool)
	pkg.VersionCount = 0
	for _, version := range details.Versions {
		if version == nil {
			continue
		}
		version.Locks = append(version.Locks, byVersion[core.ResourceLockVersionKey("npm", version.Version)]...)
		if core.ReadLocked(version.Locks) && !inspect {
			hidden[version.Version] = true
			continue
		}
		versions = append(versions, version)
		if !version.Unpublished && version.ReviewStatus == "" {
			pkg.VersionCount++
		}
	}
	details.Versions = versions
	for tag, version := range details.DistTags {
		if hidden[version] {
			delete(details.DistTags, tag)
		}
	}
	if hidden[pkg.LatestVersion] {
		pkg.LatestVersion = ""
		if latest := selectNPMProjectVersion(details); latest != nil {
			pkg.LatestVersion = latest.Version
		}
	}
	return true, nil
}

func setResourceLockAPI(c fiber.Ctx, state *core.AppState) error {
	repo, err := npmRepository(c, state)
	name, valid := npmPackageQuery(c)
	if err != nil || !valid {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid npm resource")
	}
	user, session := auth.GetUser(c), auth.CurrentSessionToken(c)
	if user == nil || !user.CheckModeratePermission(repo.Name) || auth.CurrentCredentialKind(c) != "session" || session == "" || c.Cookies("renop_session") != session {
		return npmManagementError(c, core.ErrResourceLockPermission)
	}
	var request struct {
		Version    string `json:"version"`
		Mode       string `json:"mode"`
		Reason     string `json:"reason"`
		ReasonText string `json:"reason_text"`
	}
	if utils.ReadJSONLimited(c, &request, 4096) != nil || request.Version != "" && (!validNPMVersion(request.Version) || strings.TrimSpace(request.Version) != request.Version) {
		return npmManagementError(c, core.ErrResourceLockInvalid)
	}
	release := repositorygate.AcquireMigration(repo.Name)
	defer release()
	details, err := state.GetDB().GetNPMPackageDetails(repo.Name, name, user.Username)
	if err != nil {
		return npmManagementError(c, err)
	}
	if details == nil || details.Package == nil {
		return npmManagementError(c, core.ErrNPMPackageNotFound)
	}
	if request.Version != "" {
		found := false
		for _, version := range details.Versions {
			found = found || version.Version == request.Version
		}
		if !found {
			return npmManagementError(c, core.ErrNPMVersionNotFound)
		}
	}
	target := npmLockTarget(repo.Name, name, request.Version)
	action := audit.ActionResourceLock
	if c.Method() == fiber.MethodDelete {
		action = audit.ActionResourceUnlock
		err = state.GetDB().DeleteResourceLock(target, core.ResourceLockManual, user.Username, session)
	} else {
		err = state.GetDB().SetResourceLock(&core.ResourceLock{ResourceLockTarget: target, Source: core.ResourceLockManual,
			Mode: request.Mode, Reason: request.Reason, ReasonText: request.ReasonText, LockedAt: time.Now().UnixMilli()}, user.Username, session)
	}
	if err != nil {
		return npmManagementError(c, err)
	}
	logNPMAudit(c, state, action, "Repository: "+repo.Name+", package: "+name+", version: "+request.Version+", reason: "+request.Reason)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(operationResponse{OK: true, ID: name})
}

// HandleReadLocks runs before storage conditionals, cached bytes, and mirror fills.
func HandleReadLocks(c fiber.Ctx, state *core.AppState, repo *config.Repository, requestPath string) (bool, error) {
	if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatNPM {
		return false, nil
	}
	name, version := npmPathResource(requestPath)
	if name == "" {
		return false, nil
	}
	if state == nil || state.GetDB() == nil {
		return true, fiber.ErrServiceUnavailable
	}
	locks, err := state.GetDB().GetResourceLocks(npmLockTarget(repo.Name, name, version), false)
	if err != nil {
		return true, fiber.ErrServiceUnavailable
	}
	if core.ReadLocked(locks) {
		return true, npmError(c, fiber.StatusNotFound, "not_found", "npm tarball was not found")
	}
	if len(locks) > 0 {
		c.Locals(core.LockedArtifactPathLocal, requestPath)
	}
	return false, nil
}

// EnsureMirrorPathMutable prevents a delayed mirror fill from replacing frozen npm bytes.
func EnsureMirrorPathMutable(state *core.AppState, repo *config.Repository, requestPath string) error {
	if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatNPM {
		return nil
	}
	name, version := npmPathResource(requestPath)
	if name == "" {
		return nil
	}
	if state == nil || state.GetDB() == nil {
		return core.ErrDatabaseUnavailable
	}
	return state.GetDB().EnsureResourceMutable(npmLockTarget(repo.Name, name, version), version == "")
}
