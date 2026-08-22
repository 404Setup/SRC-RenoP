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
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/3JoB/unsafeConvert"
	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/service/status"
	"renop/internal/service/storage"
	"renop/internal/utils"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func ResolveBasePath(state *core.AppState, repoName string, path string) (string, error) {
	cfg := state.Inner.Config.Load()
	if _, ok := cfg.Maven.Repositories[repoName]; !ok {
		return "", fiber.ErrNotFound
	}

	sanitizedPath, ok := utils.SanitizePath(path)
	if !ok {
		return "", fiber.ErrBadRequest
	}

	basePath := filepath.Join(cfg.StoragePath, repoName, sanitizedPath)
	return basePath, nil
}

func GeneratePom(c fiber.Ctx, state *core.AppState) error {
	if !status.CanAllocateDiskSpace(state, 64*1024) {
		return c.Status(fiber.StatusInsufficientStorage).JSON(fiber.Map{
			"error": "Insufficient disk space to generate POM",
		})
	}

	repoName := c.Params("repo_name")
	path := c.Params("*")

	user := auth.GetUser(c)

	if !user.CheckUpdatePermission(repoName) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	var pomMsg pb.PomDetails
	var pomDetails PomDetails
	readErr := protohttp.Read(c, &pomMsg)
	if readErr == fiber.ErrRequestEntityTooLarge {
		return readErr
	}
	if readErr == nil && pomMsg.ArtifactId != "" {
		pomDetails.GroupId = pomMsg.GroupId
		pomDetails.ArtifactId = pomMsg.ArtifactId
		pomDetails.Version = pomMsg.Version
	} else {
		if err := c.Bind().JSON(&pomDetails); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
		}
	}

	if !strings.HasSuffix(path, ".pom") {
		if pomDetails.ArtifactId == "" || pomDetails.Version == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
		}
		pomFilename := pomDetails.ArtifactId + "-" + pomDetails.Version + ".pom"
		if strings.HasSuffix(path, "/") {
			path += pomFilename
		} else {
			path += "/" + pomFilename
		}
	}

	basePath, err := ResolveBasePath(state, repoName, path)
	if err != nil {
		if errors.Is(err, fiber.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}

	cfg := state.Inner.Config.Load()
	repoPath := filepath.Join(cfg.StoragePath, repoName)
	if !strings.HasPrefix(filepath.Clean(basePath), filepath.Clean(repoPath)+string(os.PathSeparator)) {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	repo := cfg.Maven.Repositories[repoName]
	if repo == nil {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}
	if repo.RequireGPGSignature {
		return c.Status(fiber.StatusConflict).SendString("POM generation is unavailable because this repository requires detached GPG signatures")
	}

	lockKey := filepath.ToSlash(basePath)
	upload := state.Inner.InFlightDownloads.AcquirePath(lockKey)
	uploadSucceeded := false
	defer func() {
		state.Inner.InFlightDownloads.UnlockPath(lockKey, upload, uploadSucceeded)
	}()
	releaseUnlock, err := storage.AcquireGPGArtifactMutation(state, basePath)
	if err != nil {
		if errors.Is(err, storage.ErrGPGPendingConflict) {
			return c.Status(fiber.StatusConflict).SendString(err.Error())
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}
	defer releaseUnlock()

	if !repo.AllowRedeployment && storage.PathExistsForUpload(state, basePath) {
		return c.Status(fiber.StatusConflict).SendString("Conflict")
	}

	state.Inner.FileIndex.EnsureParentDirs(basePath)
	parentDir := filepath.Dir(basePath)
	_ = os.MkdirAll(parentDir, 0755)

	var pomXML bytes.Buffer
	pomXML.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<project xsi:schemaLocation=\"http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd\"\n    xmlns=\"http://maven.apache.org/POM/4.0.0\"\n    xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\">\n  <modelVersion>4.0.0</modelVersion>\n  <groupId>")
	xml.EscapeText(&pomXML, unsafeConvert.BytePointer(pomDetails.GroupId))
	pomXML.WriteString("</groupId>\n  <artifactId>")
	xml.EscapeText(&pomXML, unsafeConvert.BytePointer(pomDetails.ArtifactId))
	pomXML.WriteString("</artifactId>\n  <version>")
	xml.EscapeText(&pomXML, unsafeConvert.BytePointer(pomDetails.Version))
	pomXML.WriteString("</version>\n  <description>POM was generated by RenoP</description>\n</project>")

	parentOfParent := filepath.Dir(parentDir)
	if !strings.HasPrefix(filepath.Clean(parentOfParent), filepath.Clean(repoPath)+string(os.PathSeparator)) && filepath.Clean(parentOfParent) != filepath.Clean(repoPath) {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	metadataPath := filepath.Join(parentOfParent, "maven-metadata.xml")

	pomBytes := pomXML.Bytes()
	if err := storage.RemoveArtifactGPGSignature(state, basePath); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}
	err = os.WriteFile(basePath, pomBytes, 0644)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	pomMd5 := utils.MD5(pomBytes)
	pomSha1 := utils.SHA1(pomBytes)
	pomSha256 := utils.SHA256(pomBytes)
	pomSha512 := utils.SHA512(pomBytes)

	if storage.IsS3Enabled(basePath) {
		s3Key := filepath.ToSlash(basePath)
		s3Key = strings.TrimPrefix(s3Key, "./")
		s3Key = strings.TrimPrefix(s3Key, "/")
		err = storage.UploadToS3(basePath, s3Key)
		if err == nil {
			_ = os.Remove(basePath)
		} else {
			return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
		}
	}

	for ext, hash := range map[string]string{
		".md5": pomMd5, ".sha1": pomSha1, ".sha256": pomSha256, ".sha512": pomSha512,
	} {
		if err := storage.SaveAndUploadChecksum(state, basePath, ext, hash); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
		}
	}

	err = state.Inner.FileIndex.UpdateMetadataCallback(func() error {
		var metadata config.Metadata
		var src io.ReadCloser
		if storage.IsS3Enabled(metadataPath) {
			s3Key := filepath.ToSlash(metadataPath)
			s3Key = strings.TrimPrefix(s3Key, "./")
			s3Key = strings.TrimPrefix(s3Key, "/")
			if rc, _, downloadErr := storage.DownloadFromS3(s3Key); downloadErr == nil {
				src = rc
			}
		} else if f, openErr := os.Open(metadataPath); openErr == nil {
			src = f
		}
		if src != nil {
			_ = xml.NewDecoder(src).Decode(&metadata)
			_ = src.Close()
		}

		metadata.GroupId = &pomDetails.GroupId
		metadata.ArtifactId = &pomDetails.ArtifactId

		if metadata.Versioning == nil {
			metadata.Versioning = &config.Versioning{}
		}
		version := pomDetails.Version
		metadata.Versioning.Latest = &version
		metadata.Versioning.Release = &version

		lastUpdated := time.Now().UTC().Format("20060102150405")
		metadata.Versioning.LastUpdated = &lastUpdated

		if metadata.Versioning.Versions == nil {
			metadata.Versioning.Versions = &config.Versions{}
		}

		found := slices.Contains(metadata.Versioning.Versions.Version, version)
		if !found {
			metadata.Versioning.Versions.Version = append(metadata.Versioning.Versions.Version, version)
		}

		updatedXML, wErr := xml.Marshal(metadata)
		if wErr != nil {
			return wErr
		}

		metaMd5 := utils.MD5(updatedXML)
		metaSha1 := utils.SHA1(updatedXML)
		metaSha256 := utils.SHA256(updatedXML)
		metaSha512 := utils.SHA512(updatedXML)

		if storage.IsS3Enabled(metadataPath) {
			s3Key := filepath.ToSlash(metadataPath)
			s3Key = strings.TrimPrefix(s3Key, "./")
			s3Key = strings.TrimPrefix(s3Key, "/")
			if err := storage.UploadStreamToS3(s3Key, bytes.NewReader(updatedXML), int64(len(updatedXML)), "application/xml"); err != nil {
				return err
			}
		} else {
			tmpMetaPath := metadataPath + ".tmp"
			if err := os.WriteFile(tmpMetaPath, updatedXML, 0644); err != nil {
				return err
			}
			if err := utils.SafeRename(tmpMetaPath, metadataPath); err != nil {
				_ = os.Remove(tmpMetaPath)
				return err
			}
		}

		for ext, hash := range map[string]string{
			".md5": metaMd5, ".sha1": metaSha1, ".sha256": metaSha256, ".sha512": metaSha512,
		} {
			if err := storage.SaveAndUploadChecksum(state, metadataPath, ext, hash); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	state.InvalidateFileCache(metadataPath)

	state.Inner.FileIndex.InsertFile(basePath)
	state.Inner.FileIndex.InsertFile(metadataPath)
	status.MarkStorageUpdated()
	uploadSucceeded = true

	return c.Status(fiber.StatusCreated).SendString("")
}
