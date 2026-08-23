/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargodocs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	syncv2 "sync/v2"

	"renop/internal/config"
	"renop/internal/service/index"
	"renop/internal/utils"
)

var (
	IsS3Enabled    func(path ...string) bool
	DownloadFromS3 func(s3Key string) (io.ReadCloser, index.FileInfo, error)
	currentConfig  atomic.Pointer[config.Config]
)

func InitCargodocs(cfg *config.Config) {
	currentConfig.Store(cfg)
}

func getActiveConfig() *config.Config {
	cfg := currentConfig.Load()
	if cfg == nil {
		return &config.Config{
			EnableCargodocPreview: true,
			MaxCargodocSizeMb:     256,
		}
	}
	return cfg
}

func getCargodocExtractPath(cfg *config.Config) string {
	if cfg != nil && cfg.CargodocExtractPath != "" {
		return cfg.CargodocExtractPath
	}
	if cfg != nil && cfg.JavadocExtractPath != "" {
		return cfg.JavadocExtractPath
	}
	return os.TempDir()
}

var cargodocLocks [256]sync.Mutex

const (
	maxCargodocEntries            = 50_000
	maxCargodocEntrySize          = 64 << 20
	maxCargodocTotalExtractedSize = 512 << 20
	extractBufSize                = 64 * 1024
)

// extractBufPool reuses per-extraction copy buffers so concurrent extractions
// do not repeatedly allocate large short-lived slices.
var extractBufPool = syncv2.Pool[*[]byte]{
	New: func() *[]byte {
		buf := make([]byte, extractBufSize)
		return &buf
	},
}

type writerOnly struct {
	io.Writer
}

var errUnsafeCargodocArchive = errors.New("unsafe cargo doc archive")

func cargodocHashKey(repoName, crateName, version string) string {
	key := strings.ToLower(repoName) + "/" + strings.ToLower(crateName) + "/" + version
	return utils.HashAndEncode([]byte(key))
}

func CleanupCargodoc(repoName, crateName, version string) {
	hash := cargodocHashKey(repoName, crateName, version)
	cfg := getActiveConfig()
	extractPath := getCargodocExtractPath(cfg)
	cacheDir := filepath.Join(extractPath, "renop-cargodoc-"+hash)
	_ = os.RemoveAll(cacheDir)
}

func isCargodocCacheDirName(name string, isDir bool) bool {
	if !isDir || !strings.HasPrefix(name, "renop-cargodoc-") {
		return false
	}
	if strings.HasPrefix(name, "renop-cargodoc-extract-") {
		return false
	}
	return true
}

func ClearAllCargodocCaches() error {
	for i := range len(cargodocLocks) {
		cargodocLocks[i].Lock()
	}
	defer func() {
		for i := range len(cargodocLocks) {
			cargodocLocks[i].Unlock()
		}
	}()

	cfg := getActiveConfig()
	extractPath := getCargodocExtractPath(cfg)
	entries, err := os.ReadDir(extractPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !isCargodocCacheDirName(name, entry.IsDir()) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(extractPath, name))
	}
	return nil
}
