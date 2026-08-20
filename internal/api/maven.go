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
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/storage"
	"renop/internal/utils"
)

func FindMetadata(state *core.AppState, repoName string, gav string) (*config.Metadata, error) {
	const maxMetadataSize = 2 * 1024 * 1024

	cfg := state.Inner.Config.Load()
	if !utils.IsValidRepositoryName(repoName) {
		return nil, fiber.ErrBadRequest
	}
	repo, ok := cfg.Maven.Repositories[repoName]
	if !ok || repo == nil {
		return nil, fiber.ErrNotFound
	}

	sanitizedGav, ok := utils.SanitizePath(gav)
	if !ok || sanitizedGav == "" {
		return nil, fiber.ErrBadRequest
	}

	metadataPaths := metadataPathCandidates(cfg.StoragePath, repoName, sanitizedGav)
	for _, localFilePath := range metadataPaths {
		if !utils.IsSubPath(cfg.StoragePath, localFilePath) {
			return nil, fiber.ErrBadRequest
		}

		cacheKey := filepath.ToSlash(localFilePath)
		if cachedMeta, ok := state.Inner.MetadataCache.Load(cacheKey); ok {
			return cachedMeta, nil
		}

		if !metadataPathExists(state, localFilePath) {
			if fetchErr := storage.FetchMetadataFromMirror(state, repoName, localFilePath); fetchErr != nil &&
				!errors.Is(fetchErr, fiber.ErrNotFound) && !errors.Is(fetchErr, fiber.ErrBadRequest) {
				log.Printf("failed to fetch Maven metadata %s from mirror: %v", localFilePath, fetchErr)
			}
		}

		metadata, err := readMetadataFile(localFilePath, maxMetadataSize)
		if err == nil {
			state.StoreMetadataCache(cacheKey, metadata)
			return metadata, nil
		}
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) {
			return nil, err
		}
		if !errors.Is(err, fiber.ErrNotFound) {
			return nil, err
		}
	}

	return nil, fiber.ErrNotFound
}

func metadataPathCandidates(storagePath, repoName, gav string) []string {
	trimmed := strings.Trim(gav, "/")
	metadataName := strings.ToLower(filepath.Base(trimmed))
	if metadataName == "maven-metadata.xml" {
		return []string{filepath.Join(storagePath, repoName, filepath.FromSlash(trimmed))}
	}

	base := filepath.Join(storagePath, repoName, filepath.FromSlash(trimmed), "maven-metadata.xml")
	candidates := []string{base}
	parts := strings.Split(trimmed, "/")
	if len(parts) >= 3 && looksLikeMavenVersionSegment(parts[len(parts)-1]) {
		parent := strings.Join(parts[:len(parts)-1], "/")
		fallback := filepath.Join(storagePath, repoName, filepath.FromSlash(parent), "maven-metadata.xml")
		if filepath.ToSlash(fallback) != filepath.ToSlash(base) {
			candidates = append(candidates, fallback)
		}
	}
	return candidates
}

func looksLikeMavenVersionSegment(segment string) bool {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return false
	}
	upper := strings.ToUpper(segment)
	if strings.Contains(upper, "SNAPSHOT") || strings.Contains(segment, ".") {
		return true
	}
	return segment[0] >= '0' && segment[0] <= '9'
}

func metadataPathExists(state *core.AppState, localFilePath string) bool {
	if state != nil && state.Inner != nil && state.Inner.FileIndex != nil && state.Inner.FileIndex.HasFile(localFilePath) {
		return true
	}
	if storage.IsS3Enabled(localFilePath) {
		_, err := storage.StatS3(utils.GetS3Key(localFilePath))
		return err == nil
	}
	_, err := os.Stat(localFilePath)
	return err == nil
}

func readMetadataFile(localFilePath string, maxMetadataSize int) (*config.Metadata, error) {
	var r io.ReadCloser
	var err error
	if storage.IsS3Enabled(localFilePath) {
		r, _, err = storage.DownloadFromS3(utils.GetS3Key(localFilePath))
	} else {
		r, err = os.Open(localFilePath)
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fiber.ErrNotFound
		}
		return nil, fiber.ErrInternalServerError
	}
	limited := &io.LimitedReader{R: r, N: int64(maxMetadataSize) + 1}
	var metadata config.Metadata
	decodeErr := xml.NewDecoder(limited).Decode(&metadata)
	closeErr := r.Close()
	if limited.N <= 0 {
		return nil, fiber.ErrRequestEntityTooLarge
	}
	if decodeErr != nil {
		return nil, fiber.ErrInternalServerError
	}
	if closeErr != nil {
		return nil, fiber.ErrInternalServerError
	}
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
