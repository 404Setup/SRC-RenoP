/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"bytes"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/repositorygate"
)

func cargoLockTarget(repository, name, version string) core.ResourceLockTarget {
	return core.ResourceLockTarget{Format: config.RepositoryFormatCargo,
		Repository: repository, Name: normalizeCrateName(name), Version: version}
}

func canInspectLockedPackage(state *core.AppState, user *config.User, repository, name string) (bool, error) {
	if user != nil && user.CheckModeratePermission(repository) {
		return true, nil
	}
	username := ""
	if user != nil {
		username = user.Username
	}
	return state.GetDB().HasCargoPackageMembership(repository, name, username)
}

func cargoPathResource(requestPath string) (name, version string, metadata bool) {
	parts := strings.Split(strings.Trim(strings.ReplaceAll(requestPath, `\`, "/"), "/"), "/")
	if len(parts) >= 4 && strings.EqualFold(parts[0], "api") && strings.EqualFold(parts[1], "v1") && strings.EqualFold(parts[2], "crates") && !strings.EqualFold(parts[3], "new") {
		name, metadata = parts[3], true
		if len(parts) >= 5 && validatePackage(name, parts[4]) == nil {
			version = parts[4]
		}
		if len(parts) >= 6 {
			metadata = version == "" || strings.EqualFold(parts[5], "docs")
		}
		return
	}
	if len(parts) >= 2 && strings.EqualFold(parts[0], "crates") {
		name = parts[1]
		if len(parts) >= 3 {
			version = parts[2]
			if len(version) > len(name) && strings.EqualFold(version[:len(name)+1], name+"-") {
				version = version[len(name)+1:]
			}
			for _, suffix := range []string{".asc", ".sha512", ".sha256", ".sha1", ".md5"} {
				if strings.HasSuffix(strings.ToLower(version), suffix) {
					version = version[:len(version)-len(suffix)]
				}
			}
			for _, suffix := range []string{"-docs.tar.gz", "-docs.zip", ".crate"} {
				if strings.HasSuffix(strings.ToLower(version), suffix) {
					version = version[:len(version)-len(suffix)]
					break
				}
			}
		}
		return
	}
	last := parts[len(parts)-1]
	if validateCrateName(last) == nil && strings.EqualFold(indexPath(last), strings.Join(parts, "/")) {
		return last, "", true
	}
	return "", "", false
}

// VisibleMetadataPaths filters a file-browser page while retaining authorized metadata access.
func VisibleMetadataPaths(state *core.AppState, user *config.User, repository string, paths []string) ([]bool, error) {
	visible := make([]bool, len(paths))
	targets := make([]core.ResourceLockTarget, 0, min(len(paths), 128))
	indexes := make([]int, 0, cap(targets))
	flush := func() error {
		if len(targets) == 0 {
			return nil
		}
		if state == nil || state.GetDB() == nil {
			return core.ErrDatabaseUnavailable
		}
		result, err := state.GetDB().ResourceMetadataVisibility("cargo", repository, user.Username,
			user.CheckModeratePermission(repository), targets)
		if err != nil {
			return err
		}
		for i, value := range result {
			visible[indexes[i]] = value
		}
		targets, indexes = targets[:0], indexes[:0]
		return nil
	}
	for i, path := range paths {
		visible[i] = true
		name, version, _ := cargoPathResource(path)
		if name == "" {
			continue
		}
		targets = append(targets, cargoLockTarget(repository, name, version))
		indexes = append(indexes, i)
		if len(targets) == 128 {
			if err := flush(); err != nil {
				return nil, err
			}
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return visible, nil
}

func applyPackageLocks(state *core.AppState, user *config.User, details *core.CargoPackageDetails) error {
	pkg := details.Package
	locks, err := state.GetDB().GetResourceLocks(cargoLockTarget(pkg.Repository, pkg.Name, ""), true)
	if err != nil || len(locks) == 0 {
		return err
	}
	canInspect, err := canInspectLockedPackage(state, user, pkg.Repository, pkg.NormalizedName)
	if err != nil {
		return err
	}
	byVersion := make(map[string][]*core.ResourceLock)
	for _, lock := range locks {
		if lock.Version == "" {
			pkg.Locks = append(pkg.Locks, lock)
		} else {
			key := core.ResourceLockVersionKey("cargo", lock.Version)
			byVersion[key] = append(byVersion[key], lock)
		}
	}
	if core.ReadLocked(pkg.Locks) && !canInspect {
		return core.ErrCargoPackageNotFound
	}
	versions := details.Versions[:0]
	for _, version := range details.Versions {
		version.Locks = append(version.Locks, byVersion[core.ResourceLockVersionKey("cargo", version.Version)]...)
		if core.ReadLocked(version.Locks) && !canInspect {
			continue
		}
		versions = append(versions, version)
	}
	details.Versions = versions
	return nil
}

func (h Handler) setResourceLock(c fiber.Ctx, state *core.AppState, repo *config.Repository, name string) error {
	user := auth.GetUser(c)
	session := auth.CurrentSessionToken(c)
	if user == nil || !user.CheckModeratePermission(repo.Name) || auth.CurrentCredentialKind(c) != "session" ||
		session == "" || c.Cookies("renop_session") != session {
		return cargoError(c, core.ErrCargoPermissionDenied)
	}
	var request struct {
		Version string `json:"version"`
		Mode    string `json:"mode"`
		Reason  string `json:"reason"`
	}
	if decodeJSON(c, &request) != nil || validateCrateName(name) != nil ||
		(request.Version != "" && validatePackage(name, request.Version) != nil) {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid resource lock")
	}
	release := repositorygate.AcquireMigration(repo.Name)
	defer release()
	details, err := packageDetails(state, repo.Name, name, user.Username)
	if err != nil {
		return cargoError(c, err)
	}
	if request.Version != "" && !hasCargoVersion(details, request.Version) {
		return cargoError(c, core.ErrCargoVersionNotFound)
	}
	target := cargoLockTarget(repo.Name, name, request.Version)
	if c.Method() == fiber.MethodDelete {
		err = state.GetDB().DeleteResourceLock(target, core.ResourceLockManual, user.Username, session)
	} else {
		err = state.GetDB().SetResourceLock(&core.ResourceLock{ResourceLockTarget: target,
			Source: core.ResourceLockManual, Mode: request.Mode, Reason: request.Reason, LockedAt: time.Now().UnixMilli()}, user.Username, session)
	}
	if err != nil {
		return cargoError(c, err)
	}
	action := audit.ActionResourceLock
	if c.Method() == fiber.MethodDelete {
		action = audit.ActionResourceUnlock
	}
	logCargoAudit(c, state, action, "Repository: "+repo.Name+", crate: "+name+", version: "+request.Version+", reason: "+request.Reason)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(OperationResponse{OK: true})
}

// HandleReadLocks rejects restricted file reads and filters sparse metadata before cache or proxy delivery.
func (h Handler) HandleReadLocks(c fiber.Ctx, state *core.AppState, repo *config.Repository, storagePath, requestPath string) (bool, error) {
	if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatCargo {
		return false, nil
	}
	name, version, metadata := cargoPathResource(requestPath)
	if name == "" {
		return false, nil
	}
	if state == nil || state.GetDB() == nil {
		return true, cargoError(c, core.ErrDatabaseUnavailable)
	}
	isIndex := version == "" && metadata && strings.EqualFold(indexPath(name), requestPath)
	locks, err := state.GetDB().GetResourceLocks(cargoLockTarget(repo.Name, name, version), isIndex)
	if err != nil {
		return true, cargoError(c, err)
	}
	if len(locks) == 0 {
		return false, nil
	}
	canInspect := false
	if core.ReadLocked(locks) {
		if !metadata {
			return true, cargoError(c, core.ErrCargoPackageNotFound)
		}
		canInspect, err = canInspectLockedPackage(state, auth.GetUser(c), repo.Name, normalizeCrateName(name))
		if err != nil {
			return true, cargoError(c, err)
		}
		for _, lock := range locks {
			if lock.Mode == core.ResourceLockRead && !canInspect && (lock.Version == "" || !isIndex) {
				return true, cargoError(c, core.ErrCargoPackageNotFound)
			}
		}
	}
	if !metadata || isIndex {
		c.Locals(core.LockedArtifactPathLocal, requestPath)
	}
	if !isIndex || !core.ReadLocked(locks) || canInspect {
		return false, nil
	}
	hidden := make(map[string]bool)
	for _, lock := range locks {
		if lock.Mode == core.ResourceLockRead {
			if lock.Version == "" {
				return true, cargoError(c, core.ErrCargoPackageNotFound)
			}
			hidden[core.ResourceLockVersionKey("cargo", lock.Version)] = true
		}
	}
	reader, exists, err := h.Store.Open(filepath.Join(storagePath, repo.Name, filepath.FromSlash(requestPath)))
	if err != nil {
		return true, cargoError(c, err)
	}
	if !exists || reader == nil {
		return true, cargoError(c, core.ErrCargoPackageNotFound)
	}
	defer reader.Close()
	var output bytes.Buffer
	err = scanIndex(reader, func(line []byte) error {
		var entry cargoIndexValidationEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return err
		}
		if !hidden[core.ResourceLockVersionKey("cargo", entry.Version)] {
			output.Write(line)
			output.WriteByte('\n')
		}
		return nil
	})
	if err != nil {
		return true, cargoError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderContentType, "text/plain; charset=utf-8")
	return true, c.Send(output.Bytes())
}

// EnsureMirrorPathMutable prevents an upstream refresh from replacing locked content or index entries.
func EnsureMirrorPathMutable(state *core.AppState, repo *config.Repository, path string) error {
	if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatCargo {
		return nil
	}
	name, version, _ := cargoPathResource(path)
	if name == "" {
		return nil
	}
	if state == nil || state.GetDB() == nil {
		return core.ErrDatabaseUnavailable
	}
	return state.GetDB().EnsureResourceMutable(cargoLockTarget(repo.Name, name, version), strings.EqualFold(indexPath(name), path))
}
