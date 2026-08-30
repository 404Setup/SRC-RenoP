/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package npm implements npm-compatible registry and package-management services.
package npm

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/service/repositorygate"
)

// Handler routes npm registry requests while package blobs remain in shared storage.
type Handler struct {
	Store Store
}

func npmBrowserRoute(method, accept, requestPath string) bool {
	if method != fiber.MethodGet && method != fiber.MethodHead ||
		!strings.Contains(strings.ToLower(accept), fiber.MIMETextHTML) {
		return false
	}
	return requestPath == "" || requestPath == "packages" || strings.HasPrefix(requestPath, "packages/")
}

func parseDistTagPath(requestPath string) (packageName, tag string, ok bool) {
	decoded, valid := decodeRegistryPath(requestPath)
	if !valid || !strings.HasPrefix(decoded, "-/package/") {
		return "", "", false
	}
	remainder := strings.TrimPrefix(decoded, "-/package/")
	index := strings.Index(remainder, "/dist-tags")
	if index <= 0 {
		return "", "", false
	}
	packageName, valid = NormalizePackageName(remainder[:index])
	if !valid {
		return "", "", false
	}
	suffix := strings.TrimPrefix(remainder[index:], "/dist-tags")
	if suffix != "" {
		tag = strings.Trim(suffix, "/")
		if !validNPMTag(tag) {
			return "", "", false
		}
	}
	return packageName, tag, true
}

func parseVisibilityPath(requestPath string) (string, bool) {
	decoded, valid := decodeRegistryPath(requestPath)
	if !valid || !strings.HasPrefix(decoded, "-/package/") || !strings.HasSuffix(decoded, "/visibility") {
		return "", false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(decoded, "-/package/"), "/visibility")
	return NormalizePackageName(value)
}

func parseRevisionPath(requestPath string) (packageName string, revision int64, tarball bool, ok bool) {
	decoded, valid := decodeRegistryPath(requestPath)
	if !valid {
		return "", 0, false, false
	}
	before, after, found := strings.Cut(decoded, "/-rev/")
	if !found || after == "" {
		return "", 0, false, false
	}
	revision = parseRevision(after)
	if revision <= 0 {
		return "", 0, false, false
	}
	if packageName, valid = packageFromTarballPath(before); valid {
		return packageName, revision, true, true
	}
	packageName, valid = packageFromMetadataPath(before)
	return packageName, revision, false, valid
}

func (handler Handler) tarballAllowed(c fiber.Ctx, state *core.AppState, repo *config.Repository,
	requestPath, packageName string) (bool, error) {
	if state.GetDB() == nil {
		return false, core.ErrDatabaseUnavailable
	}
	pkg, err := state.GetDB().GetNPMPackage(repo.Name, packageName)
	if err != nil {
		return false, err
	}
	if pkg == nil {
		if len(repo.Mirrors) == 0 {
			return false, nil
		}
		return CanReadRepository(state, auth.GetUser(c), repo, requestPath, false)
	}
	if pkg.Archived {
		return false, nil
	}
	allowed, err := CanReadPackage(state, auth.GetUser(c), repo, packageName)
	if err != nil || !allowed {
		return allowed, err
	}
	details, err := state.GetDB().GetNPMPackageDetails(repo.Name, packageName, auth.GetUser(c).Username)
	if err != nil {
		return false, err
	}
	decoded, valid := decodeRegistryPath(requestPath)
	if !valid {
		return false, nil
	}
	for _, version := range details.Versions {
		if version != nil && !version.Unpublished && version.TarballPath == decoded {
			return true, nil
		}
	}
	return false, nil
}

func handleWhoami(c fiber.Ctx) error {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return npmError(c, fiber.StatusUnauthorized, "ENEEDAUTH", "authentication is required")
	}
	return c.JSON(fiber.Map{"username": user.Username})
}

func handleSecurityAudit(c fiber.Ctx) error {
	if strings.HasSuffix(c.Path(), "/advisories/bulk") {
		return c.JSON(fiber.Map{})
	}
	return c.JSON(fiber.Map{
		"actions": []any{}, "advisories": fiber.Map{}, "muted": []any{},
		"metadata": fiber.Map{
			"vulnerabilities": fiber.Map{"info": 0, "low": 0, "moderate": 0, "high": 0, "critical": 0, "total": 0},
			"dependencies":    0, "devDependencies": 0, "optionalDependencies": 0, "totalDependencies": 0,
		},
	})
}

func handleSearch(c fiber.Ctx, state *core.AppState, repo *config.Repository) error {
	if state.GetDB() == nil {
		return npmError(c, fiber.StatusServiceUnavailable, "database unavailable", "npm search is unavailable")
	}
	allowed, accessErr := CanReadRepository(state, auth.GetUser(c), repo, "", true)
	if accessErr != nil || !allowed {
		return npmError(c, fiber.StatusNotFound, "not_found", "npm repository was not found")
	}
	query := strings.TrimSpace(c.Query("text"))
	limit, err := strconv.Atoi(c.Query("size", "20"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}
	offset, err := strconv.Atoi(c.Query("from", "0"))
	if err != nil || offset < 0 || offset > 1_000_000 {
		offset = 0
	}
	user := auth.GetUser(c)
	administrator := user != nil && (user.IsManager() || user.CheckUpdatePermission(repo.Name))
	username := ""
	if user != nil {
		username = user.Username
	}
	packages, total, err := state.GetDB().SearchNPMPackages(repo.Name, query, username, administrator, limit, offset)
	if err != nil {
		return npmError(c, fiber.StatusInternalServerError, "search failure", "failed to search npm packages")
	}
	objects := make([]fiber.Map, 0, len(packages))
	for _, pkg := range packages {
		if pkg == nil {
			continue
		}
		scope := "unscoped"
		if strings.HasPrefix(pkg.Name, "@") {
			scope = strings.TrimPrefix(strings.SplitN(pkg.Name, "/", 2)[0], "@")
		}
		objects = append(objects, fiber.Map{
			"package": fiber.Map{
				"name": pkg.Name, "scope": scope, "version": pkg.LatestVersion,
				"description": pkg.Description, "date": time.UnixMilli(pkg.UpdatedAt).UTC().Format(time.RFC3339Nano),
				"publisher": fiber.Map{"username": pkg.Publisher}, "maintainers": []any{}, "links": fiber.Map{},
			},
			"score":       fiber.Map{"final": 1, "detail": fiber.Map{"quality": 1, "popularity": 0, "maintenance": 1}},
			"searchScore": 1,
		})
	}
	return c.JSON(fiber.Map{
		"objects": objects, "total": total, "time": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// Handle dispatches one repository-relative npm request.
func (handler Handler) Handle(c fiber.Ctx, state *core.AppState, repo *config.Repository,
	storagePath, requestPath string) (bool, error) {
	if handler.Store == nil || repo == nil || repo.NormalizedFormat() != config.RepositoryFormatNPM {
		return false, nil
	}
	decoded, valid := decodeRegistryPath(requestPath)
	if !valid {
		return true, npmError(c, fiber.StatusBadRequest, "invalid path", "npm registry path is invalid")
	}
	if npmBrowserRoute(c.Method(), c.Get(fiber.HeaderAccept), decoded) {
		return false, nil
	}
	if c.Method() == fiber.MethodPut || c.Method() == fiber.MethodDelete || c.Method() == fiber.MethodPatch {
		release := repositorygate.AcquireMutation(repo.Name)
		defer release()
	}
	switch decoded {
	case "-/ping":
		if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
			return true, npmError(c, fiber.StatusMethodNotAllowed, "method not allowed", "npm ping only supports GET")
		}
		if c.Method() == fiber.MethodHead {
			return true, c.SendStatus(fiber.StatusOK)
		}
		return true, c.JSON(fiber.Map{})
	case "-/whoami":
		return true, handleWhoami(c)
	case "-/v1/search":
		return true, handleSearch(c, state, repo)
	case "-/npm/v1/security/advisories/bulk", "-/npm/v1/security/audits/quick":
		if c.Method() != fiber.MethodPost {
			return true, npmError(c, fiber.StatusMethodNotAllowed, "method not allowed", "npm audit endpoint requires POST")
		}
		return true, handleSecurityAudit(c)
	}
	if packageName, ok := parseVisibilityPath(requestPath); ok {
		if c.Method() != fiber.MethodGet {
			return true, npmError(c, fiber.StatusMethodNotAllowed, "method not allowed", "npm visibility only supports GET")
		}
		if state.GetDB() == nil {
			return true, npmError(c, fiber.StatusServiceUnavailable, "database unavailable", "npm metadata is unavailable")
		}
		allowed, accessErr := CanReadPackage(state, auth.GetUser(c), repo, packageName)
		if accessErr != nil || !allowed {
			return true, npmError(c, fiber.StatusNotFound, "not_found", "npm package was not found")
		}
		pkg, err := state.GetDB().GetNPMPackage(repo.Name, packageName)
		if err != nil || pkg == nil {
			return true, npmError(c, fiber.StatusNotFound, "not_found", "npm package was not found")
		}
		return true, c.JSON(fiber.Map{"public": !pkg.Private})
	}
	if packageName, tag, ok := parseDistTagPath(requestPath); ok {
		return true, handleDistTags(c, state, repo, packageName, tag)
	}
	if packageName, revision, tarball, ok := parseRevisionPath(requestPath); ok {
		return true, handleRevisionRequest(c, state, repo, handler.Store, packageName, revision, tarball)
	}
	if packageName, ok := packageFromTarballPath(requestPath); ok {
		if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
			return true, npmError(c, fiber.StatusMethodNotAllowed, "method not allowed", "npm tarballs are read-only")
		}
		allowed, err := handler.tarballAllowed(c, state, repo, requestPath, packageName)
		if err != nil {
			return true, npmError(c, fiber.StatusServiceUnavailable, "metadata failure", "npm tarball access is unavailable")
		}
		if !allowed {
			return true, npmError(c, fiber.StatusNotFound, "not_found", "npm tarball was not found")
		}
		return false, nil
	}
	if packageName, ok := packageFromMetadataPath(requestPath); ok {
		switch c.Method() {
		case fiber.MethodGet, fiber.MethodHead:
			return true, servePackument(c, state, repo, packageName)
		case fiber.MethodPut:
			return true, publish(c, state, repo, handler.Store, storagePath, packageName)
		default:
			return true, npmError(c, fiber.StatusMethodNotAllowed, "method not allowed", "npm package endpoint does not support this method")
		}
	}
	if strings.HasPrefix(decoded, "-") {
		return true, npmError(c, fiber.StatusNotFound, "not_found", "npm registry endpoint is not implemented")
	}
	return true, npmError(c, http.StatusNotFound, "not_found", "npm package was not found")
}
