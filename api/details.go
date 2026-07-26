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
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/auth"
	"renop/config"
	"renop/core"
	"renop/index"
	"renop/pb"
	"renop/utils"
	"renop/utils/protohttp"
)

func toPbFileDetails(d *FileDetails) *pb.FileDetails {
	if d == nil {
		return nil
	}
	msg := &pb.FileDetails{
		Type: string(d.Type),
		Name: d.Name,
	}
	if d.ContentLength != nil {
		msg.ContentLength = d.ContentLength
	}
	if d.ContentType != nil {
		msg.ContentType = d.ContentType
	}
	if d.LastModifiedTime != nil {
		msg.LastModifiedTime = d.LastModifiedTime
	}
	if len(d.Files) > 0 {
		msg.Files = make([]*pb.FileDetails, 0, len(d.Files))
		for i := range d.Files {
			msg.Files = append(msg.Files, toPbFileDetails(&d.Files[i]))
		}
	}
	return msg
}

func CreateFileDetails(state *core.AppState, localFilePath string, withChildren bool) *FileDetails {
	idx := state.Inner.FileIndex
	localFilePath = filepath.ToSlash(localFilePath)
	name := path.Base(localFilePath)

	if idx.HasDir(localFilePath) {
		details := &FileDetails{
			Type: FileDetailsTypeDirectory,
			Name: name,
		}

		if withChildren {
			childrenNames := idx.GetChildren(localFilePath)
			files := make([]FileDetails, 0, len(childrenNames))
			for _, childName := range childrenNames {
				childPath := path.Join(localFilePath, childName)
				childDetails := CreateFileDetails(state, childPath, false)
				if childDetails != nil {
					files = append(files, *childDetails)
				}
			}
			details.Files = files
		}
		return details
	}

	if info, ok := idx.GetFileInfo(localFilePath); ok {
		modTime := time.Unix(0, info.ModTime).UTC().Format(time.RFC3339Nano)
		size := info.Size
		details := &FileDetails{
			Type:             FileDetailsTypeFile,
			Name:             name,
			ContentLength:    &size,
			LastModifiedTime: &modTime,
		}
		return details
	}

	info, err := os.Stat(localFilePath)
	if err != nil {
		return nil
	}

	if info.IsDir() {
		details := &FileDetails{
			Type: FileDetailsTypeDirectory,
			Name: name,
		}
		if withChildren {
			entries, err := os.ReadDir(localFilePath)
			if err == nil {
				var files []FileDetails
				for _, entry := range entries {
					entryPath := path.Join(localFilePath, entry.Name())
					childDetails := CreateFileDetails(state, entryPath, false)
					if childDetails != nil {
						files = append(files, *childDetails)
					}
				}
				details.Files = files
			}
		}
		return details
	}

	size := info.Size()
	modTime := info.ModTime().UTC().Format(time.RFC3339Nano)
	details := &FileDetails{
		Type:             FileDetailsTypeFile,
		Name:             name,
		ContentLength:    &size,
		LastModifiedTime: &modTime,
	}
	return details
}

func ResolveAndCheckPath(state *core.AppState, user *config.User, repoName string, path *string) (string, error) {
	cfg := state.Inner.Config.Load().(*config.Config)
	repo, ok := cfg.Maven.Repositories[repoName]
	if !ok {
		return "", fiber.ErrNotFound
	}

	pathStr := ""
	if path != nil {
		pathStr = *path
	}

	isDir := pathStr == "" || pathStr[len(pathStr)-1] == '/'
	sanitizedPath, ok := utils.SanitizePath(pathStr)
	if !ok {
		return "", fiber.ErrBadRequest
	}

	if !user.CheckReadPermission(repoName, sanitizedPath, repo.Visibility, isDir) {
		return "", fiber.ErrNotFound
	}

	localFilePath := filepath.Join(cfg.StoragePath, repoName, sanitizedPath)
	if !state.Inner.FileIndex.HasFile(localFilePath) && !state.Inner.FileIndex.HasDir(localFilePath) {
		return "", fiber.ErrNotFound
	}

	return localFilePath, nil
}

func GetDetailsAllRepos(c fiber.Ctx, state *core.AppState) error {
	user := auth.GetUser(c)

	cfg := state.Inner.Config.Load().(*config.Config)

	var repos []FileDetails
	for repoName, repo := range cfg.Maven.Repositories {
		if user.CheckReadPermission(repoName, "", repo.Visibility, true) {
			repos = append(repos, FileDetails{
				Type: FileDetailsTypeDirectory,
				Name: repoName,
			})
		}
	}

	return protohttp.Write(c, toPbFileDetails(&FileDetails{
		Type:  FileDetailsTypeDirectory,
		Name:  "repositories",
		Files: repos,
	}))
}

func GetDetailsRoot(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")

	user := auth.GetUser(c)

	localFilePath, err := ResolveAndCheckPath(state, user, repoName, nil)
	if err != nil {
		if errors.Is(err, fiber.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}

	details := CreateFileDetails(state, localFilePath, true)
	if details == nil {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	return protohttp.Write(c, toPbFileDetails(details))
}

func GetDetails(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	user := auth.GetUser(c)

	pathParam := c.Params("*")
	localFilePath, err := ResolveAndCheckPath(state, user, repoName, &pathParam)
	if err != nil {
		if errors.Is(err, fiber.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}

	details := CreateFileDetails(state, localFilePath, true)
	if details == nil {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	return protohttp.Write(c, toPbFileDetails(details))
}

func isChecksumOrMetadata(filename string) bool {
	ext := filepath.Ext(filename)
	if ext == ".md5" || ext == ".sha1" || ext == ".sha256" || ext == ".sha512" || ext == ".asc" {
		return true
	}
	if filename == "maven-metadata.xml" || strings.HasPrefix(filename, "maven-metadata.xml.") {
		return true
	}
	return false
}

func GetRepoDetails(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	user := auth.GetUser(c)

	cfg := state.Inner.Config.Load().(*config.Config)
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	if !user.CheckReadPermission(repoName, "", repo.Visibility, true) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	var totalSize int64
	var artifactSize int64
	var metadataSize int64
	var totalFiles int64
	var artifactCount int64
	var metadataCount int64

	repoDir := filepath.Join(cfg.StoragePath, repoName)

	state.Inner.FileIndex.Walk(repoDir, func(filePath string, fileInfo index.FileInfo, isDir bool) bool {
		if isDir {
			return true
		}

		totalFiles++
		totalSize += fileInfo.Size

		filename := filepath.Base(filePath)
		if isChecksumOrMetadata(filename) {
			metadataCount++
			metadataSize += fileInfo.Size
		} else {
			artifactCount++
			artifactSize += fileInfo.Size
		}
		return true
	})

	mirrors := make([]*pb.RepoMirrorInfo, 0, len(repo.Mirrors))
	for _, m := range repo.Mirrors {
		mirrors = append(mirrors, &pb.RepoMirrorInfo{
			Name:          m.Name,
			Url:           m.Url,
			Persist:       m.Persist,
			EnabledDate:   m.EnabledDate,
			CacheTtl:      m.CacheTtlSecs,
			NegativeCache: m.NegativeCache,
		})
	}

	return protohttp.Write(c, &pb.RepoDetailsResponse{
		Name:          repoName,
		Visibility:    repo.Visibility,
		TotalSize:     totalSize,
		ArtifactSize:  artifactSize,
		MetadataSize:  metadataSize,
		TotalFiles:    totalFiles,
		ArtifactCount: artifactCount,
		MetadataCount: metadataCount,
		Mirrors:       mirrors,
	})
}
