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
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
	"renop/internal/utils"
)

type mavenPublicationPreparation struct {
	relative  string
	candidate bool
	strict    bool
	blocked   bool
}

func prepareMavenPublication(state *core.AppState, repo *config.Repository, localFilePath string) (mavenPublicationPreparation, error) {
	var preparation mavenPublicationPreparation
	if state == nil || state.Inner == nil || repo == nil ||
		repo.NormalizedFormat() != config.RepositoryFormatMaven || MavenPublicationReviewCandidate == nil {
		return preparation, nil
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return preparation, core.ErrDatabaseUnavailable
	}
	relative, err := filepath.Rel(filepath.Join(cfg.StoragePath, repo.Name), localFilePath)
	if err != nil || filepath.IsAbs(relative) || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return preparation, core.ErrReviewInvalidRequest
	}
	preparation.relative = filepath.ToSlash(relative)
	preparation.candidate = MavenPublicationReviewCandidate(preparation.relative)
	preparation.strict = repo.PublicationReviewPolicy() != config.PublicationReviewOff
	if preparation.candidate && preparation.strict {
		if state.Inner.FileIndex == nil {
			return mavenPublicationPreparation{}, core.ErrDatabaseUnavailable
		}
		state.Inner.FileIndex.BlockFile(localFilePath)
		state.InvalidateFileCache(localFilePath)
		preparation.blocked = true
	}
	return preparation, nil
}

func abortMavenPublication(state *core.AppState, localFilePath string, preparation mavenPublicationPreparation) {
	if !preparation.blocked || state == nil || state.Inner == nil || state.Inner.FileIndex == nil {
		return
	}
	state.Inner.FileIndex.UnblockFile(localFilePath)
	state.InvalidateFileCache(localFilePath)
	reindexPathIfPresent(state, localFilePath)
}

func finishMavenPublication(state *core.AppState, repo *config.Repository, upload *PreparedUpload,
	preparation mavenPublicationPreparation,
) (*core.PublicationReviewResult, error) {
	if !preparation.candidate || MavenPublicationProcessor == nil {
		abortMavenPublication(state, upload.LocalFilePath, preparation)
		return &core.PublicationReviewResult{}, nil
	}
	result, err := MavenPublicationProcessor(state, repo, upload.Username, []*core.ReviewFile{{
		Path: preparation.relative, Size: upload.FileSize, AddedAt: time.Now().UnixMilli(),
	}})
	if err != nil {
		if !preparation.strict {
			abortMavenPublication(state, upload.LocalFilePath, preparation)
			log.Printf("failed to update Maven publication catalog for %s: %v", preparation.relative, err)
			return &core.PublicationReviewResult{}, nil
		}
		cleanupErr := deleteIndexedFile(state, upload.LocalFilePath)
		state.Inner.FileIndex.UnblockFile(upload.LocalFilePath)
		state.InvalidateFileCache(upload.LocalFilePath)
		return nil, errors.Join(err, cleanupErr)
	}
	if result != nil && result.Pending && !preparation.blocked {
		state.Inner.FileIndex.BlockFile(upload.LocalFilePath)
		state.InvalidateFileCache(upload.LocalFilePath)
		preparation.blocked = true
	}
	if result == nil || !result.Pending {
		abortMavenPublication(state, upload.LocalFilePath, preparation)
	}
	return result, nil
}

func reviewFileLocalPath(state *core.AppState, file *core.ReviewFile) (string, error) {
	if state == nil || state.Inner == nil || file == nil || !utils.IsValidRepositoryName(file.Repository) {
		return "", core.ErrReviewFileNotFound
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return "", core.ErrDatabaseUnavailable
	}
	relative, valid := utils.SanitizePath(file.Path)
	if !valid || relative == "" {
		return "", core.ErrReviewFileNotFound
	}
	path := filepath.Join(cfg.StoragePath, file.Repository, filepath.FromSlash(relative))
	if !utils.IsSubPath(cfg.StoragePath, path) {
		return "", core.ErrReviewFileNotFound
	}
	return path, nil
}

func publicationReviewFilesForRelease(state *core.AppState, release *core.GPGRelease) ([]*core.ReviewFile, error) {
	if state == nil || release == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	files := make([]*core.ReviewFile, 0, 10)
	addedAt := time.Now().UnixMilli()
	for _, path := range releaseBlockedPaths(state, release) {
		var (
			info index.FileInfo
			err  error
		)
		if IsS3Enabled(path) {
			info, err = StatS3(utils.GetS3Key(path))
		} else {
			var stat os.FileInfo
			stat, err = os.Stat(path)
			if err == nil && !stat.IsDir() {
				info = index.FileInfo{Size: stat.Size(), ModTime: stat.ModTime().UnixNano()}
			}
		}
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("inspect verified publication file: %w", err)
		}
		repository, relative, err := repositoryArtifactPath(state, path)
		if err != nil || repository != release.Repository {
			return nil, errors.Join(core.ErrReviewInvalidRequest, err)
		}
		files = append(files, &core.ReviewFile{
			Repository: repository, Path: relative, Size: info.Size, AddedAt: addedAt,
		})
	}
	if len(files) == 0 {
		return nil, core.ErrReviewFileNotFound
	}
	return files, nil
}

func publicationReviewPathPending(state *core.AppState, absolutePath string) (bool, error) {
	if state == nil || state.GetDB() == nil {
		return false, core.ErrDatabaseUnavailable
	}
	repository, relative, err := repositoryArtifactPath(state, absolutePath)
	if err != nil {
		return false, err
	}
	return state.GetDB().IsPublicationReviewPathPending(repository, relative)
}

// RestorePublicationReviewState hides every committed file attached to a pending review.
func RestorePublicationReviewState(state *core.AppState) error {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil || state.GetDB() == nil {
		return core.ErrDatabaseUnavailable
	}
	files, err := state.GetDB().ListPendingPublicationReviewFiles()
	if err != nil {
		return err
	}
	for _, file := range files {
		if file == nil || file.Virtual {
			continue
		}
		path, pathErr := reviewFileLocalPath(state, file)
		if pathErr != nil {
			return pathErr
		}
		state.Inner.FileIndex.BlockFile(path)
		state.InvalidateFileCache(path)
	}
	return nil
}

// UnblockPublicationReviewFiles exposes approved files and restores their index metadata.
func UnblockPublicationReviewFiles(state *core.AppState, files []*core.ReviewFile) error {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil || state.GetDB() == nil {
		return core.ErrDatabaseUnavailable
	}
	for _, file := range files {
		if file == nil || file.Virtual {
			continue
		}
		pending, err := state.GetDB().IsPublicationReviewPathPending(file.Repository, file.Path)
		if err != nil {
			return err
		}
		if pending {
			continue
		}
		path, err := reviewFileLocalPath(state, file)
		if err != nil {
			return err
		}
		state.Inner.FileIndex.UnblockFile(path)
		state.InvalidateFileCache(path)
		reindexPathIfPresent(state, path)
	}
	return nil
}

// DeletePublicationReviewFiles removes every rejected file while retaining the review history.
func DeletePublicationReviewFiles(state *core.AppState, files []*core.ReviewFile) error {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil {
		return core.ErrDatabaseUnavailable
	}
	var result error
	for _, file := range files {
		if file == nil || file.Virtual {
			continue
		}
		path, err := reviewFileLocalPath(state, file)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if err := deleteIndexedFile(state, path); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
			continue
		}
		state.Inner.FileIndex.UnblockFile(path)
		state.InvalidateFileCache(path)
	}
	return result
}

// ServePublicationReviewFile streams one otherwise hidden review file to an authorized reviewer.
func ServePublicationReviewFile(c fiber.Ctx, state *core.AppState, file *core.ReviewFile) error {
	path, err := reviewFileLocalPath(state, file)
	if err != nil {
		return err
	}
	name := strings.ReplaceAll(file.Name, `"`, "")
	if name == "" {
		name = "artifact"
	}
	return serveLocalFile(c, state, path, path, `attachment; filename="`+name+`"`, file.Size,
		false, nil, nil)
}

// PublicationReviewErrorResponse maps stable publication-review failures to protocol statuses.
func PublicationReviewErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, core.ErrReviewPublicationSealed):
		return fiber.StatusConflict, "Reviewed publication is sealed"
	case errors.Is(err, core.ErrReviewFileLimit):
		return fiber.StatusTooManyRequests, "Publication review limit reached"
	case errors.Is(err, core.ErrReviewInvalidRequest):
		return fiber.StatusBadRequest, "Invalid publication review"
	case errors.Is(err, core.ErrReviewPermissionDenied):
		return fiber.StatusForbidden, "Publication review permission denied"
	case errors.Is(err, core.ErrDatabaseUnavailable):
		return fiber.StatusServiceUnavailable, "Publication review is unavailable"
	default:
		return fiber.StatusInternalServerError, "Publication review failed"
	}
}

func preservePublicationReviewBlock(state *core.AppState, path string) bool {
	pending, err := publicationReviewPathPending(state, path)
	if err != nil {
		log.Printf("failed to inspect publication review block for %s: %v", filepath.ToSlash(path), err)
		return true
	}
	return pending
}
