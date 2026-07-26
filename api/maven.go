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
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/config"
	"renop/core"
	"renop/storage"
	"renop/utils"
)

func FindMetadata(state *core.AppState, repoName string, gav string) (*config.Metadata, error) {
	const maxMetadataSize = 2 * 1024 * 1024

	cfg := state.Inner.Config.Load().(*config.Config)
	if !utils.IsValidRepositoryName(repoName) {
		return nil, fiber.ErrBadRequest
	}
	if _, ok := cfg.Maven.Repositories[repoName]; !ok {
		return nil, fiber.ErrNotFound
	}

	sanitizedGav, ok := utils.SanitizePath(gav)
	if !ok || sanitizedGav == "" {
		return nil, fiber.ErrBadRequest
	}

	localFilePath := filepath.Join(cfg.StoragePath, repoName, sanitizedGav)
	if !strings.HasSuffix(localFilePath, "maven-metadata.xml") {
		localFilePath = filepath.Join(localFilePath, "maven-metadata.xml")
	}

	if !utils.IsSubPath(cfg.StoragePath, localFilePath) {
		return nil, fiber.ErrBadRequest
	}

	if !state.Inner.FileIndex.HasFile(localFilePath) {
		return nil, fiber.ErrNotFound
	}

	cacheKey := filepath.ToSlash(localFilePath)
	if cachedMeta, ok := state.Inner.MetadataCache.Load(cacheKey); ok {
		return cachedMeta, nil
	}

	var contentBytes []byte
	var err error

	if storage.IsS3Enabled(localFilePath) {
		s3Key := utils.GetS3Key(localFilePath)
		rc, _, downloadErr := storage.DownloadFromS3(s3Key)
		if downloadErr != nil {
			return nil, fiber.ErrNotFound
		}
		contentBytes, err = io.ReadAll(io.LimitReader(rc, maxMetadataSize+1))
		_ = rc.Close()
		if err != nil {
			return nil, fiber.ErrInternalServerError
		}
	} else {
		file, openErr := os.Open(localFilePath)
		if openErr != nil {
			return nil, fiber.ErrInternalServerError
		}
		contentBytes, err = io.ReadAll(io.LimitReader(file, maxMetadataSize+1))
		_ = file.Close()
		if err != nil {
			return nil, fiber.ErrInternalServerError
		}
	}
	if len(contentBytes) > maxMetadataSize {
		return nil, fiber.ErrRequestEntityTooLarge
	}

	var metadata config.Metadata
	if err := xml.Unmarshal(contentBytes, &metadata); err != nil {
		return nil, fiber.ErrInternalServerError
	}

	state.StoreMetadataCache(cacheKey, &metadata)

	return &metadata, nil
}

func FindVersionsInternal(metadata *config.Metadata, filter *string, sorted bool) (bool, []string) {
	isSnapshot := false
	var extractedVersions []string

	if metadata.Versioning != nil {
		if metadata.Versioning.Versions != nil {
			extractedVersions = append(extractedVersions, metadata.Versioning.Versions.Version...)
		} else if metadata.Versioning.SnapshotVersions != nil {
			isSnapshot = true
			for _, sv := range metadata.Versioning.SnapshotVersions.SnapshotVersion {
				if sv.Value != nil {
					extractedVersions = append(extractedVersions, *sv.Value)
				}
			}
		}
	}

	if len(extractedVersions) == 0 && metadata.Version != nil {
		isSnapshot = true
		extractedVersions = append(extractedVersions, *metadata.Version)
	}

	if filter != nil {
		f := *filter
		filtered := make([]string, 0, len(extractedVersions))
		if strings.HasPrefix(f, "has:") {
			prefixMatch := f[4:]
			for _, v := range extractedVersions {
				if strings.Contains(v, prefixMatch) {
					filtered = append(filtered, v)
				}
			}
		} else if strings.HasPrefix(f, "none:") {
			prefixMatch := f[5:]
			for _, v := range extractedVersions {
				if !strings.Contains(v, prefixMatch) {
					filtered = append(filtered, v)
				}
			}
		} else {
			for _, v := range extractedVersions {
				if strings.HasPrefix(v, f) {
					filtered = append(filtered, v)
				}
			}
		}
		extractedVersions = filtered
	}

	if sorted {
		slices.SortFunc(extractedVersions, utils.CompareVersions)
	}

	return isSnapshot, extractedVersions
}
