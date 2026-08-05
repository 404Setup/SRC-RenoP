/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package javadocs

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/status"
	"renop/internal/utils"
)

func getJavadocJarName(basePath string) string {
	version := filepath.Base(basePath)
	name := filepath.Base(filepath.Dir(basePath))
	return name + "-" + version + "-javadoc.jar"
}

func hasExtractedJavadoc(cacheDir string) bool {
	info, err := os.Stat(filepath.Join(cacheDir, "index.html"))
	return err == nil && !info.IsDir()
}

func javadocOutputPath(tempDir, name string) (string, error) {
	if name == "" || strings.ContainsRune(name, '\\') {
		return "", errUnsafeJavadocArchive
	}
	cleanName := path.Clean(name)
	if cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, "../") || strings.HasPrefix(cleanName, "/") {
		return "", errUnsafeJavadocArchive
	}
	outPath := filepath.Join(tempDir, filepath.FromSlash(cleanName))
	if !utils.IsSubPath(tempDir, outPath) {
		return "", errUnsafeJavadocArchive
	}
	return outPath, nil
}

func extractJavadoc(jarPath string, cacheDir string) error {
	if hasExtractedJavadoc(cacheDir) {
		return nil
	}

	cfg := getActiveConfig()
	maxExtractedSize := uint64(cfg.MaxJavadocSizeMb) << 20

	var jarSize uint64 = 10 * 1024 * 1024
	if fi, err := os.Stat(jarPath); err == nil {
		jarSize = max(uint64(fi.Size())*3, 10*1024*1024)
	}
	if !status.CanAllocateDiskSpace(nil, jarSize) {
		return errors.New("Insufficient disk space to extract javadoc")
	}

	extractPath := getJavadocExtractPath(cfg)
	tempDir, err := os.MkdirTemp(extractPath, "renop-javadoc-extract-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return err
	}
	defer r.Close()
	if len(r.File) > maxJavadocEntries {
		return fmt.Errorf("%w: too many entries", errUnsafeJavadocArchive)
	}

	var totalSize uint64
	for _, f := range r.File {
		if _, err := javadocOutputPath(tempDir, f.Name); err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > maxJavadocEntrySize || totalSize+f.UncompressedSize64 > maxExtractedSize {
			return fmt.Errorf("%w: extracted data exceeds limit", errUnsafeJavadocArchive)
		}
		totalSize += f.UncompressedSize64
	}

	bufPtr := extractBufPool.Get().(*[]byte)
	buf := *bufPtr
	defer extractBufPool.Put(bufPtr)

	for _, f := range r.File {
		Output, pathErr := javadocOutputPath(tempDir, f.Name)
		if pathErr != nil {
			return pathErr
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(Output, 0755); err != nil {
				return err
			}
			continue
		}

		if err = os.MkdirAll(filepath.Dir(Output), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(Output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			_ = outFile.Close()
			return err
		}

		written, copyErr := io.CopyBuffer(writerOnly{Writer: outFile}, rc, buf)
		closeReadErr := rc.Close()
		closeWriteErr := outFile.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeReadErr != nil {
			return closeReadErr
		}
		if closeWriteErr != nil {
			return closeWriteErr
		}
		if uint64(written) != f.UncompressedSize64 {
			return fmt.Errorf("%w: truncated entry", errUnsafeJavadocArchive)
		}
	}

	if !hasExtractedJavadoc(tempDir) {
		return errors.New("javadoc archive has no index.html")
	}
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0755); err != nil {
		return err
	}
	if err := utils.SafeRename(tempDir, cacheDir); err != nil {
		if removeErr := os.RemoveAll(cacheDir); removeErr != nil {
			return err
		}
		if retryErr := utils.SafeRename(tempDir, cacheDir); retryErr != nil {
			return retryErr
		}
	}

	return nil
}

func EnsureJavadocExtractedBlocking(jarPath string) (string, error) {
	isS3 := false
	if IsS3Enabled != nil {
		isS3 = IsS3Enabled(jarPath)
	}

	hash := utils.HashAndEncode([]byte(jarPath))
	cfg := getActiveConfig()
	extractPath := getJavadocExtractPath(cfg)
	cacheDir := filepath.Join(extractPath, "renop-javadoc-"+hash)

	if hasExtractedJavadoc(cacheDir) {
		return cacheDir, nil
	}

	shard := 0
	if len(hash) > 0 {
		shard = int(hash[0]) % 256
	}
	javadocLocks[shard].Lock()
	defer javadocLocks[shard].Unlock()

	if hasExtractedJavadoc(cacheDir) {
		return cacheDir, nil
	}

	maxArchiveSize := cfg.MaxJavadocSizeMb << 20

	var localJarPath string
	if isS3 {
		s3Key := utils.GetS3Key(jarPath)

		tempFile, err := os.CreateTemp(extractPath, "renop-javadoc-*.jar")
		if err != nil {
			return "", err
		}
		tempFileName := tempFile.Name()
		defer func() {
			_ = os.Remove(tempFileName)
		}()

		var copyErr error
		if DownloadFromS3 != nil {
			rc, info, downloadErr := DownloadFromS3(s3Key)
			if downloadErr != nil {
				tempFile.Close()
				return "", downloadErr
			}
			if info.Size > maxArchiveSize {
				_ = rc.Close()
				_ = tempFile.Close()
				return "", fmt.Errorf("%w: archive exceeds limit", errUnsafeJavadocArchive)
			}
			var written int64
			written, copyErr = io.Copy(tempFile, io.LimitReader(rc, maxArchiveSize+1))
			if copyErr == nil && written > maxArchiveSize {
				copyErr = fmt.Errorf("%w: archive exceeds limit", errUnsafeJavadocArchive)
			}
			rc.Close()
		}
		tempFile.Close()

		if copyErr != nil {
			return "", copyErr
		}
		localJarPath = tempFileName
	} else {
		fi, err := os.Stat(jarPath)
		if err != nil {
			return "", fiber.ErrNotFound
		}
		if fi.Size() > maxArchiveSize {
			return "", fmt.Errorf("%w: archive exceeds limit", errUnsafeJavadocArchive)
		}
		localJarPath = jarPath
	}

	err := extractJavadoc(localJarPath, cacheDir)
	if err != nil {
		return "", err
	}

	return cacheDir, nil
}

func ensureJavadocExtracted(state *core.AppState, repoName string, path string, requireJar bool) (string, error) {
	cfg := state.Inner.Config.Load().(*config.Config)
	basePath := filepath.Join(cfg.StoragePath, repoName, path)

	var isDir bool
	var isExist bool

	isS3 := false
	if IsS3Enabled != nil {
		isS3 = IsS3Enabled(basePath)
	}

	if isS3 {
		if state.Inner.FileIndex.HasDir(basePath) {
			isDir = true
			isExist = true
		} else if state.Inner.FileIndex.HasFile(basePath) {
			isDir = false
			isExist = true
		}
	} else {
		info, err := os.Stat(basePath)
		if err == nil {
			isDir = info.IsDir()
			isExist = true
		}
	}

	if !isExist {
		return "", fiber.ErrNotFound
	}

	var jarPath string
	if isDir {
		defaultJar := filepath.Join(basePath, getJavadocJarName(basePath))
		if _, err := os.Stat(defaultJar); err == nil {
			jarPath = defaultJar
		} else {
			entries, err := os.ReadDir(basePath)
			if err == nil {
				for _, entry := range entries {
					if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), "-javadoc.jar") {
						jarPath = filepath.Join(basePath, entry.Name())
						break
					}
				}
			}
			if jarPath == "" {
				jarPath = defaultJar
			}
		}
	} else {
		jarPath = basePath
	}

	if requireJar {
		isJarS3 := false
		if IsS3Enabled != nil {
			isJarS3 = IsS3Enabled(jarPath)
		}
		if isJarS3 {
			if !state.Inner.FileIndex.HasFile(jarPath) {
				return "", fiber.ErrNotFound
			}
		} else {
			if _, err := os.Stat(jarPath); err != nil {
				return "", fiber.ErrNotFound
			}
		}
	}

	return EnsureJavadocExtractedBlocking(jarPath)
}
