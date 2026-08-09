/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package api

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/service/storage"
	"renop/internal/utils"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func FindVersions(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	path := c.Params("*")

	sanitizedPath, ok := utils.SanitizePath(path)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad path")
	}

	user := auth.GetUser(c)
	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	if !user.CheckReadPermission(repoName, sanitizedPath, repo.Visibility, false) {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	metadata, err := FindMetadata(state, repoName, sanitizedPath)
	if err != nil {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	filter := c.Query("filter")
	var filterPtr *string
	if filter != "" {
		filterPtr = &filter
	}
	sorted := c.Query("sorted", "true") == "true"

	isSnapshot, versions := FindVersionsInternal(metadata, filterPtr, sorted)

	res := &pb.VersionsResponse{
		IsSnapshot: isSnapshot,
		Versions:   versions,
	}

	return protohttp.Write(c, res)
}

func LatestVersion(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	path := c.Params("*")

	sanitizedPath, ok := utils.SanitizePath(path)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad path")
	}

	user := auth.GetUser(c)
	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	if !user.CheckReadPermission(repoName, sanitizedPath, repo.Visibility, false) {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	metadata, err := FindMetadata(state, repoName, sanitizedPath)
	if err != nil {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	filter := c.Query("filter")
	var filterPtr *string
	if filter != "" {
		filterPtr = &filter
	}
	sorted := c.Query("sorted", "true") == "true"

	isSnapshot, versions := FindVersionsInternal(metadata, filterPtr, sorted)
	if len(versions) == 0 {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	version := versions[len(versions)-1]
	resType := c.Query("type")

	if resType == "raw" {
		return c.Status(fiber.StatusOK).SendString(version)
	}

	res := &pb.LatestVersionResponse{
		IsSnapshot: isSnapshot,
		Version:    version,
	}

	return protohttp.Write(c, res)
}

func isBadPath(str string) bool {
	return strings.IndexByte(str, '/') != -1 || strings.IndexByte(str, '\\') != -1 || strings.Contains(str, "..")
}

func ResolveLatestPath(state *core.AppState, repoName string, gav string, query *ArtifactDetailsQuery) (string, bool, error) {
	sanitizedGav, ok := utils.SanitizePath(gav)
	if !ok {
		return "", false, fiber.ErrBadRequest
	}

	metadata, err := FindMetadata(state, repoName, sanitizedGav)
	if err != nil {
		return "", false, fiber.ErrNotFound
	}

	isSnapshot, versions := FindVersionsInternal(metadata, query.Filter, true)
	if len(versions) == 0 {
		return "", false, fiber.ErrNotFound
	}

	version := versions[len(versions)-1]

	if query.Classifier != nil && isBadPath(*query.Classifier) {
		return "", false, fiber.ErrBadRequest
	}
	if query.Extension != nil && isBadPath(*query.Extension) {
		return "", false, fiber.ErrBadRequest
	}

	extension := "jar"
	if query.Extension != nil && *query.Extension != "" {
		extension = *query.Extension
	}

	var lastPart string
	trimmedGav := strings.TrimRight(sanitizedGav, "/")
	idx := strings.LastIndexByte(trimmedGav, '/')
	if idx != -1 {
		lastPart = trimmedGav[idx+1:]
	} else {
		lastPart = trimmedGav
	}

	if lastPart == "" {
		return "", false, fiber.ErrBadRequest
	}

	classifierLen := 0
	if query.Classifier != nil && *query.Classifier != "" {
		classifierLen = len(*query.Classifier) + 1
	}

	extensionLen := 0
	if extension != "" {
		extensionLen = len(extension) + 1
	}

	capacity := len(lastPart) + 1 + len(version) + classifierLen + extensionLen
	var fileName strings.Builder
	fileName.Grow(capacity)
	fileName.WriteString(lastPart)
	fileName.WriteString("-")
	fileName.WriteString(version)

	if query.Classifier != nil && *query.Classifier != "" {
		fileName.WriteString("-")
		fileName.WriteString(*query.Classifier)
	}
	if extension != "" {
		fileName.WriteString(".")
		fileName.WriteString(extension)
	}

	cfg := state.Inner.Config.Load()
	basePath := filepath.Join(cfg.StoragePath, repoName, sanitizedGav)

	var fullPath string
	if isSnapshot {
		fullPath = filepath.Join(basePath, fileName.String())
	} else {
		fullPath = filepath.Join(basePath, version, fileName.String())
	}

	if !utils.IsSubPath(cfg.StoragePath, fullPath) {
		return "", false, fiber.ErrBadRequest
	}

	info, err := os.Stat(fullPath)
	isDir := false
	if err == nil {
		isDir = info.IsDir()
	} else if errors.Is(err, fs.ErrNotExist) && !state.Inner.FileIndex.HasFile(fullPath) {
		return "", false, fiber.ErrNotFound
	}

	return fullPath, isDir, nil
}

func LatestDetails(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	gav := c.Params("*")

	sanitizedGav, ok := utils.SanitizePath(gav)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad path")
	}

	user := auth.GetUser(c)
	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	if !user.CheckReadPermission(repoName, sanitizedGav, repo.Visibility, false) {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	ext := c.Query("extension")
	cls := c.Query("classifier")
	flt := c.Query("filter")

	var query ArtifactDetailsQuery
	if ext != "" {
		query.Extension = &ext
	}
	if cls != "" {
		query.Classifier = &cls
	}
	if flt != "" {
		query.Filter = &flt
	}

	localFilePath, _, err := ResolveLatestPath(state, repoName, gav, &query)
	if err != nil {
		if errors.Is(err, fiber.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		if errors.Is(err, fiber.ErrBadRequest) {
			return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Error")
	}

	var isDirectory bool
	var isFileExists bool
	var size int64
	var modTimeNano int64

	if state.Inner.FileIndex.HasDir(localFilePath) {
		isDirectory = true
		isFileExists = true
	} else if fileInfo, ok := state.Inner.FileIndex.GetFileInfo(localFilePath); ok {
		isDirectory = false
		isFileExists = true
		size = fileInfo.Size
		modTimeNano = fileInfo.ModTime
	} else {
		info, err := os.Stat(localFilePath)
		if err == nil {
			isFileExists = true
			isDirectory = info.IsDir()
			size = info.Size()
			modTimeNano = info.ModTime().UnixNano()
		}
	}

	if !isFileExists {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	var details pb.FileDetails
	if isDirectory {
		details = pb.FileDetails{
			Type: string(FileDetailsTypeDirectory),
			Name: filepath.Base(localFilePath),
		}
	} else {
		modTimeStr := time.Unix(0, modTimeNano).UTC().Format(time.RFC3339Nano)
		details = pb.FileDetails{
			Type:             string(FileDetailsTypeFile),
			Name:             filepath.Base(localFilePath),
			ContentLength:    &size,
			LastModifiedTime: &modTimeStr,
		}
	}

	return protohttp.Write(c, &details)
}

func LatestFile(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	gav := c.Params("*")

	sanitizedGav, ok := utils.SanitizePath(gav)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad path")
	}

	user := auth.GetUser(c)
	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	if !user.CheckReadPermission(repoName, sanitizedGav, repo.Visibility, false) {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	ext := c.Query("extension")
	cls := c.Query("classifier")
	flt := c.Query("filter")

	var query ArtifactDetailsQuery
	if ext != "" {
		query.Extension = &ext
	}
	if cls != "" {
		query.Classifier = &cls
	}
	if flt != "" {
		query.Filter = &flt
	}

	localFilePath, isDir, err := ResolveLatestPath(state, repoName, gav, &query)
	if err != nil {
		if errors.Is(err, fiber.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		if errors.Is(err, fiber.ErrBadRequest) {
			return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Error")
	}

	if isDir {
		return c.Status(fiber.StatusBadRequest).SendString("Is a dir")
	}

	fileInfo, ok := state.Inner.FileIndex.GetFileInfo(localFilePath)
	if !ok {
		info, err := os.Stat(localFilePath)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		fileInfo.Size = info.Size()
	}

	c.Set("Content-Security-Policy", "default-src 'none'; sandbox")

	if storage.IsS3Enabled(localFilePath) {
		s3Key := filepath.ToSlash(localFilePath)
		s3Key = strings.TrimPrefix(s3Key, "./")
		s3Key = strings.TrimPrefix(s3Key, "/")
		if reqRange := c.Get(fiber.HeaderRange); reqRange != "" {
			start, end, ok := utils.ParseRange(reqRange, uint64(fileInfo.Size))
			if !ok {
				c.Set(fiber.HeaderContentRange, "bytes */"+strconv.FormatInt(fileInfo.Size, 10))
				return c.Status(fiber.StatusRequestedRangeNotSatisfiable).SendString("")
			}
			rc, err := storage.DownloadRangeFromS3(s3Key, int64(start), int64(end))
			if err != nil {
				if rc != nil {
					_ = rc.Close()
				}
				return c.Status(fiber.StatusNotFound).SendString("Not found")
			}
			c.Set(fiber.HeaderContentRange, "bytes "+strconv.FormatUint(start, 10)+"-"+strconv.FormatUint(end, 10)+"/"+strconv.FormatInt(fileInfo.Size, 10))
			c.Set(fiber.HeaderAcceptRanges, "bytes")
			c.Status(fiber.StatusPartialContent)
			return c.SendStream(rc, int(end-start+1))
		}
		rc, _, err := storage.DownloadFromS3(s3Key)
		if err == nil {
			return c.SendStream(rc, int(fileInfo.Size))
		}
		if rc != nil {
			_ = rc.Close()
		}
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	return c.SendFile(localFilePath)
}
