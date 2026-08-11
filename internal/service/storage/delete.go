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
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/javadocs"
	"renop/internal/service/status"
	"renop/internal/utils"
)

func HandleDelete(c fiber.Ctx, state *core.AppState, path string, localFilePath string) error {
	defer status.MarkStorageUpdated()
	if path == "" || path == "/" {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	isDir := false
	exists := false
	if IsS3Enabled(localFilePath) {
		if state.Inner.FileIndex.HasDir(localFilePath) {
			isDir = true
			exists = true
		} else if state.Inner.FileIndex.HasFile(localFilePath) {
			exists = true
		}
	} else {
		info, err := os.Stat(localFilePath)
		if err == nil {
			exists = true
			isDir = info.IsDir()
		}
	}

	if !exists {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	if isDir {
		if IsS3Enabled(localFilePath) {
			s3Prefix := utils.GetS3Key(localFilePath)
			if !strings.HasSuffix(s3Prefix, "/") {
				s3Prefix += "/"
			}
			if err := DeletePrefixFromS3(s3Prefix); err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
			}
		} else {
			if err := os.RemoveAll(localFilePath); err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
			}
		}
		state.Inner.FileIndex.RemoveDir(localFilePath)
	} else {
		isJar := filepath.Ext(localFilePath) == ".jar"
		dir := filepath.Dir(localFilePath)

		ext := filepath.Ext(localFilePath)
		fileStem := strings.TrimSuffix(filepath.Base(localFilePath), ext)

		deleteFileHelper(localFilePath)
		state.Inner.FileIndex.RemoveFile(localFilePath)
		state.InvalidateFileCache(localFilePath)

		if isJar {
			if strings.HasSuffix(localFilePath, "-javadoc.jar") {
				javadocs.CleanupJavadoc(localFilePath)
			}
			extensions := []string{
				".jar.md5", ".jar.sha1", ".jar.sha256", ".jar.sha512", ".jar.asc",
				".pom", ".pom.md5", ".pom.sha1", ".pom.sha256", ".pom.sha512", ".pom.asc",
				".module", ".module.md5", ".module.sha1", ".module.sha256", ".module.sha512", ".module.asc",
			}

			for _, ext := range extensions {
				toDelete := filepath.Join(dir, fileStem+ext)
				if state.Inner.FileIndex.HasFile(toDelete) {
					deleteFileHelper(toDelete)
					state.Inner.FileIndex.RemoveFile(toDelete)
					state.InvalidateFileCache(toDelete)
				}
			}

			version := filepath.Base(dir)
			artifactDir := filepath.Dir(dir)
			metadataPath := filepath.Join(artifactDir, "maven-metadata.xml")

			if state.Inner.FileIndex.HasFile(metadataPath) {
				_ = state.Inner.FileIndex.UpdateMetadataCallback(func() error {
					var (
						src io.ReadCloser
						err error
					)
					if IsS3Enabled(metadataPath) {
						src, _, err = DownloadFromS3(utils.GetS3Key(metadataPath))
					} else {
						src, err = os.Open(metadataPath)
					}
					if err != nil {
						return nil
					}
					var metadata config.Metadata
					decErr := xml.NewDecoder(src).Decode(&metadata)
					_ = src.Close()
					if decErr != nil {
						return nil
					}
					var newVersions []string
					found := false
					if metadata.Versioning != nil && metadata.Versioning.Versions != nil {
						for _, v := range metadata.Versioning.Versions.Version {
							if v == version {
								found = true
							} else {
								newVersions = append(newVersions, v)
							}
						}
					}

					if !found {
						return nil
					}
					if len(newVersions) == 0 {
						deleteFileHelper(metadataPath)
						state.Inner.FileIndex.RemoveFile(metadataPath)
						state.InvalidateFileCache(metadataPath)
						for _, ext := range []string{".md5", ".sha1", ".sha256", ".sha512"} {
							toDel := metadataPath + ext
							deleteFileHelper(toDel)
							state.Inner.FileIndex.RemoveFile(toDel)
							state.InvalidateFileCache(toDel)
						}
						return nil
					}
					metadata.Versioning.Versions.Version = newVersions
					sortedVersions := make([]string, len(newVersions))
					copy(sortedVersions, newVersions)
					slices.SortFunc(sortedVersions, utils.CompareVersions)
					latest := sortedVersions[len(sortedVersions)-1]
					metadata.Versioning.Latest = &latest
					metadata.Versioning.Release = &latest
					lastUpdated := time.Now().UTC().Format("20060102150405")
					metadata.Versioning.LastUpdated = &lastUpdated

					updatedXML, wErr := xml.Marshal(metadata)
					if wErr != nil {
						return wErr
					}

					if IsS3Enabled(metadataPath) {
						s3Key := utils.GetS3Key(metadataPath)
						err = UploadStreamToS3(s3Key, bytes.NewReader(updatedXML), int64(len(updatedXML)), "application/xml")
					} else {
						tmpMetaPath := metadataPath + ".tmp"
						err = os.WriteFile(tmpMetaPath, updatedXML, 0644)
						if err == nil {
							err = utils.SafeRename(tmpMetaPath, metadataPath)
							if err != nil {
								_ = os.Remove(tmpMetaPath)
							}
						}
					}

					if err == nil {
						state.InvalidateFileCache(metadataPath)
						md5Hash := utils.MD5(updatedXML)
						sha1Hash := utils.SHA1(updatedXML)
						sha256Hash := utils.SHA256(updatedXML)
						sha512Hash := utils.SHA512(updatedXML)

						_ = SaveAndUploadChecksum(state, metadataPath, ".md5", md5Hash)
						_ = SaveAndUploadChecksum(state, metadataPath, ".sha1", sha1Hash)
						_ = SaveAndUploadChecksum(state, metadataPath, ".sha256", sha256Hash)
						_ = SaveAndUploadChecksum(state, metadataPath, ".sha512", sha512Hash)
					}
					return nil
				})
			}
		}
	}

	username, op, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	repoName := c.Params("repo_name")
	details := "Deleted artifact/directory: " + path
	if repoName != "" {
		details = "Repository: " + repoName + ", Path: " + path
	}
	audit.Log(state, &core.AuditLogEntry{
		Username:   username,
		Operator:   op,
		Action:     "DELETE",
		Details:    details,
		AuthMethod: authMethod,
		SessionID:  sessionID,
		IP:         ip,
	})

	return c.Status(fiber.StatusNoContent).SendString("")
}

func deleteFileHelper(path string) {
	if IsS3Enabled(path) {
		s3Key := utils.GetS3Key(path)
		_ = DeleteFromS3(s3Key)
	} else {
		_ = os.Remove(path)
	}
}

// RemoveRepositoryStorage deletes a repository's on-disk (and optional S3) data
// and purges the corresponding file-index / watcher entries.
// s3Cfg may be nil; when non-nil it is used even if the repo is already gone from config.
func RemoveRepositoryStorage(state *core.AppState, storagePath, repoName string, s3Cfg *config.S3Config) {
	if state == nil || storagePath == "" || repoName == "" {
		return
	}
	if !utils.IsValidRepositoryName(repoName) {
		return
	}

	repoDir := filepath.Join(storagePath, repoName)
	cleanStorage := filepath.Clean(storagePath)
	cleanRepo := filepath.Clean(repoDir)
	if cleanRepo == cleanStorage || !utils.IsSubPath(storagePath, repoDir) {
		return
	}

	if s3Cfg != nil && s3Cfg.Enabled {
		s3Prefix := utils.GetS3Key(repoDir)
		if !strings.HasSuffix(s3Prefix, "/") {
			s3Prefix += "/"
		}
		_ = DeletePrefixFromS3Config(s3Cfg, s3Prefix)
	}

	_ = os.RemoveAll(repoDir)

	pathNorm := filepath.ToSlash(cleanRepo)
	if state.Inner.FileIndex != nil {
		state.Inner.FileIndex.RemoveDir(pathNorm)
	}

	state.Inner.IndexWatcherMutex.Lock()
	if state.Inner.IndexWatcher != nil {
		_ = state.Inner.IndexWatcher.Remove(pathNorm)
	}
	state.Inner.IndexWatcherMutex.Unlock()

	status.MarkStorageUpdated()
}
