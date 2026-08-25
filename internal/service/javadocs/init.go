/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package javadocs extracts and serves sandboxed Java documentation.
package javadocs

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

func InitJavadocs(cfg *config.Config) {
	currentConfig.Store(cfg)
}

func getActiveConfig() *config.Config {
	cfg := currentConfig.Load()
	if cfg == nil {
		return &config.Config{
			EnableJavadocPreview: true,
			MaxJavadocSizeMb:     256,
		}
	}
	return cfg
}

func getJavadocExtractPath(cfg *config.Config) string {
	if cfg.JavadocExtractPath != "" {
		return cfg.JavadocExtractPath
	}
	return os.TempDir()
}

var javadocLocks [256]sync.Mutex

const (
	maxJavadocEntries   = 20_000
	maxJavadocEntrySize = 64 << 20
	extractBufSize      = 64 * 1024
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

var errUnsafeJavadocArchive = errors.New("unsafe javadoc archive")

func CleanupJavadoc(jarPath string) {
	hash := utils.HashAndEncode([]byte(jarPath))
	cfg := getActiveConfig()
	extractPath := getJavadocExtractPath(cfg)
	cacheDir := filepath.Join(extractPath, "renop-javadoc-"+hash)
	_ = os.RemoveAll(cacheDir)
}

func isJavadocCacheDirName(name string, isDir bool) bool {
	if !isDir || !strings.HasPrefix(name, "renop-javadoc-") {
		return false
	}
	if strings.HasPrefix(name, "renop-javadoc-extract-") {
		return false
	}
	return true
}

func ClearAllJavadocCaches() error {
	for i := range len(javadocLocks) {
		javadocLocks[i].Lock()
	}
	defer func() {
		for i := range len(javadocLocks) {
			javadocLocks[i].Unlock()
		}
	}()

	cfg := getActiveConfig()
	extractPath := getJavadocExtractPath(cfg)
	entries, err := os.ReadDir(extractPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !isJavadocCacheDirName(name, entry.IsDir()) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(extractPath, name))
	}
	return nil
}
