/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
	"renop/internal/service/proxy"
	"renop/internal/utils"
)

func setArtifactContentType(c fiber.Ctx, path string) {
	c.Set(fiber.HeaderContentType, utils.ContentTypeByExt(filepath.Ext(path)))
}

func CheckIndexAndCacheConfig(repoName string, path string, repo *config.Repository) (bool, uint64) {
	anyPersist, baseMaxTtl := repo.GetCacheConfig()

	if len(repo.Mirrors) > 0 {
		isSnapshotRepo := strings.Contains(strings.ToLower(repoName), "snapshot")
		isSnapshotPath := strings.Contains(strings.ToUpper(path), "SNAPSHOT")

		if repo.AllowRedeployment || isSnapshotRepo || isSnapshotPath {
			anyPersist = false
			if baseMaxTtl == 0 || baseMaxTtl > 60 {
				baseMaxTtl = 60
			}
		}
	}

	return anyPersist, baseMaxTtl
}

func LoadMetadataAndCheckTTL(state *core.AppState, localFilePath string, pathLossy string, isIndexed bool, exists bool, isDir bool, info index.FileInfo, anyPersist bool, baseMaxTtl uint64) (bool, index.FileInfo, bool) {
	if isIndexed && !exists {
		statInfo, err := os.Stat(localFilePath)
		if err == nil {
			isDir = statInfo.IsDir()
			exists = true
			if !isDir {
				info = index.FileInfo{
					Size:    statInfo.Size(),
					ModTime: statInfo.ModTime().UnixNano(),
				}
				state.Inner.FileIndex.InsertFile(pathLossy, info)
			}
		}
	}

	if isIndexed && exists && !isDir && !anyPersist && baseMaxTtl > 0 {
		modified := time.Unix(0, info.ModTime)
		if time.Since(modified).Seconds() > float64(baseMaxTtl) {
			if IsS3Enabled(localFilePath) {
				s3Key := utils.GetS3Key(localFilePath)
				_ = DeleteFromS3(s3Key)
			} else {
				_ = os.Remove(localFilePath)
			}
			state.Inner.FileIndex.RemoveFile(pathLossy)
			state.InvalidateFileCache(pathLossy)
			isIndexed = false
			exists = false
			info = index.FileInfo{}
			isDir = false
		}
	}

	return exists, info, isDir
}

func HandleGet(c fiber.Ctx, state *core.AppState, repo *config.Repository, storagePath string) error {
	repoName := c.Params("repo_name")
	path := c.Params("*")
	if path == "" {
		path = "/"
	}

	sanitized, ok := utils.SanitizePath(path)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	path = sanitized

	localFilePath := filepath.Join(storagePath, repoName, path)
	if !utils.IsSubPath(storagePath, localFilePath) {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	pathStr := filepath.ToSlash(localFilePath)

	isDir, info, exists, isNotFound := state.Inner.FileIndex.GetPathState(pathStr)
	if isNotFound {
		if TryHTMLFallback(state, c) {
			return nil
		}
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	isIndexed := exists
	anyPersist, baseMaxTtl := CheckIndexAndCacheConfig(repoName, path, repo)

	if isDir && TryHTMLFallback(state, c) {
		return nil
	}

	contentDisposition := "attachment"
	if c.Query("preview") == "true" &&
		(utils.IsImageFile(path) || utils.IsPreviewableTextFile(path) || (strings.HasSuffix(path, ".jar") && strings.Contains(path, "javadoc"))) {
		contentDisposition = "inline"
	}

	isIndexed, info, isDir = LoadMetadataAndCheckTTL(state, localFilePath, pathStr, isIndexed, exists, isDir, info, anyPersist, baseMaxTtl)
	exists = isDir || info.ModTime > 0

	if isIndexed && exists && !isDir {
		handled, err := serveFromCache(c, state, pathStr, contentDisposition, info)
		if handled {
			return err
		}
	}

	var etagHeader *string
	var lastModifiedHeader *string

	if isIndexed && exists && !isDir {
		if info.ModTime > 0 {
			size := info.Size
			unixTime := info.ModTime
			modified := time.Unix(0, unixTime)

			etagVal := "W/\"" + strconv.FormatInt(size, 16) + "-" + strconv.FormatInt(unixTime, 16) + "\""
			lastModifiedStr := modified.UTC().Format(http.TimeFormat)

			if CheckNotModified(c, &etagVal, &lastModifiedStr) {
				return c.Status(fiber.StatusNotModified).SendString("")
			}

			etagHeader = &etagVal
			lastModifiedHeader = &lastModifiedStr
		}
	}

	if isIndexed && exists {
		return serveLocalFile(c, state, localFilePath, pathStr, contentDisposition, info.Size, isDir, etagHeader, lastModifiedHeader)
	}

	handled, err := handleChecksumFallback(c, localFilePath, state)
	if handled {
		return err
	}

	setArtifactContentType(c, path)
	handled, err = handleProxy(c, state, repo, localFilePath, path, pathStr, storagePath, contentDisposition)
	if handled {
		return err
	}

	state.Inner.FailuresCount.Add(1)

	if TryHTMLFallback(state, c) {
		return nil
	}

	return c.Status(fiber.StatusNotFound).SendString("Not found")
}

func HandleHead(c fiber.Ctx, state *core.AppState, repo *config.Repository, storagePath string) error {
	repoName := c.Params("repo_name")
	path := c.Params("*")
	if path == "" {
		path = "/"
	}

	sanitized, ok := utils.SanitizePath(path)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	path = sanitized

	localFilePath := filepath.Join(storagePath, repoName, path)
	if !utils.IsSubPath(storagePath, localFilePath) {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	pathStr := filepath.ToSlash(localFilePath)
	c.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("X-Frame-Options", "SAMEORIGIN")

	isDir, info, exists, isNotFound := state.Inner.FileIndex.GetPathState(pathStr)
	if isNotFound {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	isIndexed := exists
	anyPersist, baseMaxTtl := CheckIndexAndCacheConfig(repoName, path, repo)

	isIndexed, info, isDir = LoadMetadataAndCheckTTL(state, localFilePath, pathStr, isIndexed, exists, isDir, info, anyPersist, baseMaxTtl)

	if isIndexed && isDir {
		return c.Status(fiber.StatusOK).SendString("")
	}
	if isIndexed && info.ModTime > 0 {
		setArtifactContentType(c, path)
		modified := time.Unix(0, info.ModTime)
		etag := "W/\"" + strconv.FormatInt(info.Size, 16) + "-" + strconv.FormatInt(info.ModTime, 16) + "\""
		lastModified := modified.UTC().Format(http.TimeFormat)
		c.Set(fiber.HeaderETag, etag)
		c.Set(fiber.HeaderLastModified, lastModified)
		c.Set(fiber.HeaderContentLength, strconv.FormatInt(info.Size, 10))
		c.Set(fiber.HeaderAcceptRanges, "bytes")
		if CheckNotModified(c, &etag, &lastModified) {
			return c.Status(fiber.StatusNotModified).SendString("")
		}
		return c.Status(fiber.StatusOK).SendString("")
	}

	if len(repo.Mirrors) > 0 {
		existsOnMirror, headers, err := proxy.ProxyHead(state, repo, path)
		if err == nil && existsOnMirror {
			if contentType := headers.Get("Content-Type"); contentType != "" {
				c.Set("Content-Type", contentType)
			}
			if contentLength := headers.Get("Content-Length"); contentLength != "" {
				c.Set("Content-Length", contentLength)
			}
			if lastMod := headers.Get("Last-Modified"); lastMod != "" {
				c.Set("Last-Modified", lastMod)
			}
			if etag := headers.Get("ETag"); etag != "" {
				c.Set("ETag", etag)
			}
			return c.Status(fiber.StatusOK).SendString("")
		}
	}

	return c.Status(fiber.StatusNotFound).SendString("Not found")
}
