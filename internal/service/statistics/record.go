/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package statistics

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/service/npm"
	"renop/internal/utils"
)

func downloadUsername(c fiber.Ctx) string {
	user := auth.GetUser(c)
	if user == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(user.Username))
}

func successfulDownload(c fiber.Ctx) bool {
	if c.Method() != fiber.MethodGet {
		return false
	}
	status := c.Response().StatusCode()
	if status == fiber.StatusOK || status == fiber.StatusFound || status == fiber.StatusTemporaryRedirect ||
		status == fiber.StatusPermanentRedirect {
		return true
	}
	if status != fiber.StatusPartialContent {
		return false
	}
	rangeHeader := strings.TrimSpace(c.Get(fiber.HeaderRange))
	return strings.HasPrefix(rangeHeader, "bytes=0-")
}

func responseDownloadSize(c fiber.Ctx) int64 {
	contentRange := string(c.Response().Header.Peek(fiber.HeaderContentRange))
	if separator := strings.LastIndexByte(contentRange, '/'); separator >= 0 && separator+1 < len(contentRange) {
		if size, err := strconv.ParseInt(contentRange[separator+1:], 10, 64); err == nil && size > 0 {
			return size
		}
	}
	contentLength := string(c.Response().Header.Peek(fiber.HeaderContentLength))
	size, _ := strconv.ParseInt(contentLength, 10, 64)
	return max(size, 0)
}

func indexedDownloadSize(state *core.AppState, repository, path string) int64 {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil {
		return 0
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return 0
	}
	indexedPath := filepath.Join(cfg.StoragePath, repository, filepath.FromSlash(path))
	if !utils.IsSubPath(filepath.Join(cfg.StoragePath, repository), indexedPath) {
		return 0
	}
	info, ok := state.Inner.FileIndex.GetFileInfo(indexedPath)
	if !ok {
		return 0
	}
	return max(info.Size, 0)
}

func isDownloadCompanion(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range []string{".asc", ".md5", ".sha1", ".sha256", ".sha512"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func classifyMavenDownload(path string) (namespace, packageName, version string, ok bool) {
	parts := strings.Split(strings.Trim(strings.ReplaceAll(path, `\`, "/"), "/"), "/")
	if len(parts) < 4 {
		return "", "", "", false
	}
	name := parts[len(parts)-1]
	if name == "" || strings.HasPrefix(strings.ToLower(name), "maven-metadata.xml") ||
		isDownloadCompanion(name) || strings.Contains(strings.ToLower(name), "-javadoc.") {
		return "", "", "", false
	}
	artifactID, version := parts[len(parts)-3], parts[len(parts)-2]
	if artifactID == "" || version == "" || !strings.HasPrefix(name, artifactID+"-") {
		return "", "", "", false
	}
	groupParts := parts[:len(parts)-3]
	if len(groupParts) == 0 {
		return "", "", "", false
	}
	groupID := strings.Join(groupParts, ".")
	return groupID, groupID + ":" + artifactID, version, true
}

func classifyCargoDownload(path string) (packageName, version string, ok bool) {
	parts := strings.Split(strings.Trim(strings.ReplaceAll(path, `\`, "/"), "/"), "/")
	if len(parts) != 6 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "crates" ||
		parts[3] == "" || parts[4] == "" || parts[5] != "download" {
		return "", "", false
	}
	return strings.ToLower(parts[3]), parts[4], true
}

func classifyRepositoryDownload(repo *config.Repository, path string) (namespace, packageName, version string, ok bool) {
	if repo == nil || !repo.DownloadStatisticsEnabled() {
		return "", "", "", false
	}
	switch repo.NormalizedFormat() {
	case config.RepositoryFormatMaven:
		return classifyMavenDownload(path)
	case config.RepositoryFormatCargo:
		packageName, version, ok = classifyCargoDownload(path)
		return "", packageName, version, ok
	case config.RepositoryFormatFiles:
		path = strings.Trim(strings.ReplaceAll(path, `\`, "/"), "/")
		name := path
		if separator := strings.LastIndexByte(path, '/'); separator >= 0 {
			name = path[separator+1:]
		}
		if path == "" || isDownloadCompanion(name) || strings.Contains(strings.ToLower(name), "-javadoc.") {
			return "", "", "", false
		}
		return "", path, "", true
	case config.RepositoryFormatNPM:
		packageName, version, matched := npm.ClassifyTarballPath(path)
		if !matched {
			return "", "", "", false
		}
		namespace := ""
		if strings.HasPrefix(packageName, "@") {
			namespace = strings.SplitN(packageName, "/", 2)[0]
		}
		return namespace, packageName, version, true
	default:
		return "", "", "", false
	}
}

func activeStatisticsRepository(state *core.AppState, repository *config.Repository) *config.Repository {
	if state == nil || state.Inner == nil || repository == nil {
		return nil
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return nil
	}
	active := cfg.Maven.Repositories[repository.Name]
	if active == nil || !active.DownloadStatisticsEnabled() {
		return nil
	}
	return active
}

// RecordRepositoryDownload records one successful Maven, Cargo, or files response.
func RecordRepositoryDownload(c fiber.Ctx, state *core.AppState, repo *config.Repository, path string) {
	if !successfulDownload(c) {
		return
	}
	repo = activeStatisticsRepository(state, repo)
	if repo == nil {
		return
	}
	namespace, packageName, version, ok := classifyRepositoryDownload(repo, path)
	if !ok {
		return
	}
	counter := GetCounter(state)
	if counter == nil {
		return
	}
	size := responseDownloadSize(c)
	if size == 0 {
		size = indexedDownloadSize(state, repo.Name, path)
	}
	counter.Record(core.DownloadStatisticDelta{
		Username: downloadUsername(c), Repository: repo.Name, Format: repo.NormalizedFormat(),
		Namespace: namespace, Package: packageName, Version: version,
		Bytes: size,
	})
}

// RecordDockerPull records one successful manifest pull as one image-version download.
func RecordDockerPull(c fiber.Ctx, state *core.AppState, repo *config.Repository,
	imageName, reference string, size int64) {
	repo = activeStatisticsRepository(state, repo)
	if repo == nil || c.Method() == fiber.MethodHead {
		return
	}
	counter := GetCounter(state)
	if counter == nil {
		return
	}
	counter.Record(core.DownloadStatisticDelta{
		Username: downloadUsername(c), Repository: repo.Name, Format: config.RepositoryFormatDocker,
		Package: imageName, Version: reference, Bytes: max(size, 0),
	})
}
