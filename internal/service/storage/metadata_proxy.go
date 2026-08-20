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
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/proxy"
	"renop/internal/utils"
)

const metadataProxyWaitTimeout = 2 * time.Minute

// FetchMetadataFromMirror obtains one Maven metadata document through the
// repository's normal mirror proxy and consumes the stream so it is persisted
// in the local/S3 cache. The in-flight lock prevents concurrent API callers
// from issuing duplicate upstream requests.
func FetchMetadataFromMirror(state *core.AppState, repoName, localFilePath string) error {
	if state == nil || state.Inner == nil || state.Inner.Config == nil || state.Inner.FileIndex == nil {
		return errors.New("storage state is not initialized")
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return errors.New("configuration is not initialized")
	}
	repo := cfg.Maven.Repositories[repoName]
	if repo == nil || len(repo.Mirrors) == 0 {
		return fiber.ErrNotFound
	}

	repoRoot := filepath.Join(cfg.StoragePath, repoName)
	rel, err := filepath.Rel(repoRoot, localFilePath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fiber.ErrBadRequest
	}
	rel = filepath.ToSlash(rel)
	if !utils.IsSubPath(cfg.StoragePath, localFilePath) || !isMavenMetadataPath(localFilePath) {
		return fiber.ErrBadRequest
	}

	pathStr := filepath.ToSlash(localFilePath)
	dl, loaded := state.Inner.InFlightDownloads.LockPath(pathStr)
	if loaded {
		if !state.Inner.InFlightDownloads.WaitTimeout(dl, metadataProxyWaitTimeout) {
			return errors.New("timed out waiting for metadata fetch")
		}
		if metadataPathExists(state, localFilePath) {
			return nil
		}
		return fiber.ErrNotFound
	}

	stream, err := proxy.ProxyArtifact(state, repo, rel, cfg.StoragePath, pathStr, dl)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(io.Discard, stream)
	closeErr := stream.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if !metadataPathExists(state, localFilePath) {
		return fiber.ErrNotFound
	}
	return nil
}

func metadataPathExists(state *core.AppState, localFilePath string) bool {
	if state != nil && state.Inner != nil && state.Inner.FileIndex != nil && state.Inner.FileIndex.HasFile(localFilePath) {
		return true
	}
	if IsS3Enabled(localFilePath) {
		_, err := StatS3(utils.GetS3Key(localFilePath))
		return err == nil
	}
	_, err := os.Stat(localFilePath)
	return err == nil
}
