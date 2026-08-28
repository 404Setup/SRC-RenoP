/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"errors"
	"strings"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/utils"
)

func requireLifecycleScope(c fiber.Ctx, repository, packageName string) bool {
	target := repository + "/" + packageName
	return auth.CurrentCredentialHasScopeTarget(c, core.APITokenScopePackageLifecycle, target) ||
		auth.CurrentCredentialHasScope(c, core.APITokenScopePackageManage)
}

func npmMutationActor(user *config.User, repository string) string {
	if user != nil && (user.IsManager() || user.CheckUpdatePermission(repository)) {
		return ""
	}
	if user == nil {
		return ""
	}
	return user.Username
}

func handleDistTags(c fiber.Ctx, state *core.AppState, repo *config.Repository, packageName, tag string) error {
	if state.GetDB() == nil {
		return npmError(c, fiber.StatusServiceUnavailable, "database unavailable", "npm dist-tags are unavailable")
	}
	if c.Method() == fiber.MethodGet {
		allowed, err := CanReadPackage(state, auth.GetUser(c), repo, packageName)
		if err != nil || !allowed {
			return npmError(c, fiber.StatusNotFound, "not_found", "npm package was not found")
		}
		details, err := state.GetDB().GetNPMPackageDetails(repo.Name, packageName, auth.GetUser(c).Username)
		if err != nil || details == nil {
			return npmError(c, fiber.StatusNotFound, "not_found", "npm package was not found")
		}
		return c.JSON(details.DistTags)
	}
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return npmError(c, fiber.StatusUnauthorized, "ENEEDAUTH", "authentication is required")
	}
	if !canWriteRepository(user, repo) {
		return npmError(c, fiber.StatusForbidden, "forbidden", "npm repository update permission is required")
	}
	if !requireLifecycleScope(c, repo.Name, packageName) {
		return npmError(c, fiber.StatusForbidden, "forbidden", "API token package lifecycle scope is required")
	}
	var err error
	switch c.Method() {
	case fiber.MethodPut:
		if tag == "" {
			return npmError(c, fiber.StatusBadRequest, "invalid dist-tag", "a dist-tag name is required")
		}
		var version string
		if err := utils.ReadJSONLimited(c, &version, 1024); err != nil || !validNPMVersion(version) {
			return npmError(c, fiber.StatusBadRequest, "invalid version", "dist-tag target must be an existing semantic version")
		}
		err = state.GetDB().SetNPMDistTag(repo.Name, packageName, tag, version,
			npmMutationActor(user, repo.Name), 0)
	case fiber.MethodDelete:
		if tag == "" {
			return npmError(c, fiber.StatusBadRequest, "invalid dist-tag", "a dist-tag name is required")
		}
		err = state.GetDB().DeleteNPMDistTag(repo.Name, packageName, tag,
			npmMutationActor(user, repo.Name), 0)
	default:
		return npmError(c, fiber.StatusMethodNotAllowed, "method not allowed", "npm dist-tag endpoint supports GET, PUT, and DELETE")
	}
	if err != nil {
		return lifecycleError(c, err)
	}
	logNPMAudit(c, state, audit.ActionNPMDistTag,
		"Repository: "+repo.Name+", package: "+packageName+", tag: "+tag)
	return c.Status(fiber.StatusCreated).JSON(operationResponse{OK: true, ID: packageName})
}

func lifecycleError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, core.ErrNPMPackageNotFound), errors.Is(err, core.ErrNPMVersionNotFound):
		return npmError(c, fiber.StatusNotFound, "not_found", "npm package or version was not found")
	case errors.Is(err, core.ErrNPMPermissionDenied):
		return npmError(c, fiber.StatusForbidden, "forbidden", "npm package lifecycle permission is required")
	case errors.Is(err, core.ErrNPMRevisionConflict):
		return npmError(c, fiber.StatusConflict, "conflict", "npm package metadata changed; fetch it again")
	default:
		return npmError(c, fiber.StatusInternalServerError, "metadata failure", "failed to update npm package metadata")
	}
}

func packumentMetadata(document *publishDocument) (map[string]string, map[string]string, error) {
	if document == nil || len(document.Versions) > 5000 {
		return nil, nil, errors.New("npm packument update is invalid")
	}
	deprecations := make(map[string]string, len(document.Versions))
	knownVersions := make(map[string]struct{}, len(document.Versions))
	for version, raw := range document.Versions {
		if !validNPMVersion(version) || len(raw) == 0 || len(raw) > maxStoredManifestJSON {
			return nil, nil, errors.New("npm packument version is invalid")
		}
		var manifest map[string]any
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return nil, nil, errors.New("npm packument version is invalid")
		}
		deprecated, _ := manifest["deprecated"].(string)
		deprecations[version] = strings.TrimSpace(deprecated)
		knownVersions[version] = struct{}{}
	}
	tags := make(map[string]string, len(document.DistTags))
	for tag, version := range document.DistTags {
		if !validNPMTag(tag) {
			return nil, nil, errors.New("npm packument dist-tag is invalid")
		}
		if _, known := knownVersions[version]; !known {
			return nil, nil, errors.New("npm packument dist-tag points to a missing version")
		}
		tags[strings.ToLower(tag)] = version
	}
	return deprecations, tags, nil
}

func updatePackument(c fiber.Ctx, state *core.AppState, repo *config.Repository,
	packageName string, document *publishDocument, expectedRevision int64) error {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return npmError(c, fiber.StatusUnauthorized, "ENEEDAUTH", "authentication is required")
	}
	if !canWriteRepository(user, repo) {
		return npmError(c, fiber.StatusForbidden, "forbidden", "npm repository update permission is required")
	}
	if !requireLifecycleScope(c, repo.Name, packageName) {
		return npmError(c, fiber.StatusForbidden, "forbidden", "API token package lifecycle scope is required")
	}
	if expectedRevision <= 0 {
		expectedRevision = parseRevision(document.Revision)
	}
	deprecations, tags, err := packumentMetadata(document)
	if err != nil {
		return npmError(c, fiber.StatusBadRequest, "invalid packument", err.Error())
	}
	if err := state.GetDB().UpdateNPMPackument(repo.Name, packageName, npmMutationActor(user, repo.Name),
		expectedRevision, deprecations, tags); err != nil {
		return lifecycleError(c, err)
	}
	logNPMAudit(c, state, audit.ActionNPMMetadataUpdate,
		"Repository: "+repo.Name+", package: "+packageName)
	return c.Status(fiber.StatusCreated).JSON(operationResponse{OK: true, ID: packageName})
}

func handleRevisionRequest(c fiber.Ctx, state *core.AppState, repo *config.Repository, store Store,
	packageName string, revision int64, tarball bool) error {
	if tarball {
		if c.Method() != fiber.MethodDelete {
			return npmError(c, fiber.StatusMethodNotAllowed, "method not allowed", "npm tarball revisions only support DELETE")
		}
		return c.Status(fiber.StatusOK).JSON(operationResponse{OK: true, ID: packageName})
	}
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return npmError(c, fiber.StatusUnauthorized, "ENEEDAUTH", "authentication is required")
	}
	if !canWriteRepository(user, repo) {
		return npmError(c, fiber.StatusForbidden, "forbidden", "npm repository update permission is required")
	}
	if !requireLifecycleScope(c, repo.Name, packageName) {
		return npmError(c, fiber.StatusForbidden, "forbidden", "API token package lifecycle scope is required")
	}
	switch c.Method() {
	case fiber.MethodDelete:
		paths, err := state.GetDB().DeleteNPMPackage(repo.Name, packageName,
			npmMutationActor(user, repo.Name), revision)
		if err != nil {
			return lifecycleError(c, err)
		}
		removeNPMTarballs(state, store, repo.Name, paths)
		logNPMAudit(c, state, audit.ActionNPMPackageDelete,
			"Repository: "+repo.Name+", package: "+packageName)
		return c.Status(fiber.StatusOK).JSON(operationResponse{OK: true, ID: packageName})
	case fiber.MethodPut:
		document, err := decodePublishDocument(c)
		if err != nil {
			return npmError(c, fiber.StatusBadRequest, "invalid packument", "npm unpublish metadata is invalid")
		}
		details, err := state.GetDB().GetNPMPackageDetails(repo.Name, packageName, user.Username)
		if err != nil || details == nil {
			return npmError(c, fiber.StatusNotFound, "not_found", "npm package was not found")
		}
		missing := make([]*core.NPMVersion, 0, 1)
		for _, version := range details.Versions {
			if version == nil || version.Unpublished {
				continue
			}
			if _, present := document.Versions[version.Version]; !present {
				missing = append(missing, version)
			}
		}
		if len(missing) == 0 {
			return updatePackument(c, state, repo, packageName, document, revision)
		}
		if len(missing) != 1 {
			return npmError(c, fiber.StatusBadRequest, "invalid unpublish", "remove exactly one npm version per request")
		}
		path, err := state.GetDB().UnpublishNPMVersion(repo.Name, packageName, missing[0].Version,
			npmMutationActor(user, repo.Name), revision)
		if err != nil {
			return lifecycleError(c, err)
		}
		removeNPMTarballs(state, store, repo.Name, []string{path})
		logNPMAudit(c, state, audit.ActionNPMVersionDelete,
			"Repository: "+repo.Name+", package: "+packageName+", version: "+missing[0].Version)
		return c.Status(fiber.StatusCreated).JSON(operationResponse{OK: true, ID: packageName})
	default:
		return npmError(c, fiber.StatusMethodNotAllowed, "method not allowed", "npm revisions support PUT and DELETE")
	}
}
