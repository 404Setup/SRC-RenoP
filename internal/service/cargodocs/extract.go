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
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zip"

	"github.com/gofiber/fiber/v3"
	"github.com/klauspost/compress/gzip"

	"renop/internal/core"
	"renop/internal/service/status"
	"renop/internal/utils"
)

func hasExtractedCargodoc(cacheDir string) bool {
	info, err := os.Stat(filepath.Join(cacheDir, "index.html"))
	return err == nil && !info.IsDir()
}

func cargodocOutputPath(tempDir, name string) (string, error) {
	if name == "" || strings.ContainsAny(name, "\\\x00:") {
		return "", errUnsafeCargodocArchive
	}
	cleanName := path.Clean(name)
	if cleanName == "." || cleanName == ".." || strings.HasPrefix(cleanName, "../") || strings.HasPrefix(cleanName, "/") {
		return "", errUnsafeCargodocArchive
	}
	outPath := filepath.Join(tempDir, filepath.FromSlash(cleanName))
	if !utils.IsSubPath(tempDir, outPath) {
		return "", errUnsafeCargodocArchive
	}
	return outPath, nil
}

func isGzipArchive(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var header [2]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return false
	}
	return header[0] == 0x1f && header[1] == 0x8b
}

func isZipArchive(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var header [4]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return false
	}
	return header[0] == 0x50 && header[1] == 0x4b && (header[2] == 0x03 || header[2] == 0x05 || header[2] == 0x07)
}

func extractZipCargodoc(archivePath, tempDir, crateName string, maxExtractedSize uint64) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	if len(r.File) > maxCargodocEntries {
		return fmt.Errorf("%w: too many entries", errUnsafeCargodocArchive)
	}

	var totalSize uint64
	for _, f := range r.File {
		if _, err := cargodocOutputPath(tempDir, f.Name); err != nil {
			return err
		}
		if crateName != "" && !IsTargetCrateOrSharedAsset(f.Name, crateName) {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > maxCargodocEntrySize || totalSize+f.UncompressedSize64 > maxExtractedSize {
			return fmt.Errorf("%w: extracted data exceeds limit", errUnsafeCargodocArchive)
		}
		totalSize += f.UncompressedSize64
	}

	bufPtr := extractBufPool.Get()
	buf := *bufPtr
	defer extractBufPool.Put(bufPtr)

	for _, f := range r.File {
		output, pathErr := cargodocOutputPath(tempDir, f.Name)
		if pathErr != nil {
			return pathErr
		}
		if crateName != "" && !IsTargetCrateOrSharedAsset(f.Name, crateName) {
			continue
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(output, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
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
			return fmt.Errorf("%w: truncated entry", errUnsafeCargodocArchive)
		}
	}
	return nil
}

func extractTarGzCargodoc(archivePath, tempDir, crateName string, maxExtractedSize uint64) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzipReader, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(io.LimitReader(gzipReader, int64(maxExtractedSize)+1))
	bufPtr := extractBufPool.Get()
	buf := *bufPtr
	defer extractBufPool.Put(bufPtr)

	var entries int
	var totalSize uint64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: invalid tar entry", errUnsafeCargodocArchive)
		}
		entries++
		if entries > maxCargodocEntries {
			return fmt.Errorf("%w: too many entries", errUnsafeCargodocArchive)
		}

		output, pathErr := cargodocOutputPath(tempDir, header.Name)
		if pathErr != nil {
			return pathErr
		}

		if crateName != "" && !IsTargetCrateOrSharedAsset(header.Name, crateName) {
			continue
		}

		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(output, 0755); err != nil {
				return err
			}
			continue
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		if header.Size < 0 || uint64(header.Size) > maxCargodocEntrySize || totalSize+uint64(header.Size) > maxExtractedSize {
			return fmt.Errorf("%w: extracted data exceeds limit", errUnsafeCargodocArchive)
		}
		totalSize += uint64(header.Size)

		if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
			return err
		}

		outFile, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}

		written, copyErr := io.CopyBuffer(writerOnly{Writer: outFile}, tarReader, buf)
		closeWriteErr := outFile.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeWriteErr != nil {
			return closeWriteErr
		}
		if written != header.Size {
			return fmt.Errorf("%w: truncated entry", errUnsafeCargodocArchive)
		}
	}
	return nil
}

func ensureRootIndexHTML(tempDir, crateName string) error {
	rootIndex := filepath.Join(tempDir, "index.html")
	if info, err := os.Stat(rootIndex); err == nil && !info.IsDir() {
		return nil
	}

	// Look for cargo doc output in standard subpaths (e.g. <crate>/index.html or <crate_underscored>/index.html)
	crateUnderscored := strings.ReplaceAll(crateName, "-", "_")
	crateHyphenated := strings.ReplaceAll(crateName, "_", "-")

	candidates := []string{
		crateUnderscored,
		crateHyphenated,
		crateName,
		"doc",
		filepath.Join("doc", crateUnderscored),
		filepath.Join("doc", crateHyphenated),
		filepath.Join("target", "doc"),
		filepath.Join("target", "doc", crateUnderscored),
		filepath.Join("target", "doc", crateHyphenated),
	}

	var targetRelativePath string
	for _, c := range candidates {
		candidateIndex := filepath.Join(tempDir, c, "index.html")
		if info, err := os.Stat(candidateIndex); err == nil && !info.IsDir() {
			targetRelativePath = filepath.ToSlash(filepath.Join(c, "index.html"))
			break
		}
	}

	if targetRelativePath == "" {
		// Scan top-level directories to find any directory containing index.html
		entries, err := os.ReadDir(tempDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					candidateIndex := filepath.Join(tempDir, entry.Name(), "index.html")
					if info, err := os.Stat(candidateIndex); err == nil && !info.IsDir() {
						targetRelativePath = filepath.ToSlash(filepath.Join(entry.Name(), "index.html"))
						break
					}
				}
			}
		}
	}

	if targetRelativePath == "" {
		_ = filepath.WalkDir(tempDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || targetRelativePath != "" {
				return err
			}
			if !d.IsDir() && strings.EqualFold(d.Name(), "index.html") {
				rel, relErr := filepath.Rel(tempDir, p)
				if relErr == nil && rel != "index.html" {
					targetRelativePath = filepath.ToSlash(rel)
					return filepath.SkipAll
				}
			}
			return nil
		})
	}

	if targetRelativePath == "" {
		return errors.New("cargo doc archive contains no index.html")
	}

	// Create root redirect index.html
	redirectHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta http-equiv="refresh" content="0; url=./%s">
    <title>Redirecting to documentation...</title>
</head>
<body>
    <p>Redirecting to <a href="./%s">%s</a>...</p>
</body>
</html>`, targetRelativePath, targetRelativePath, crateName)

	return os.WriteFile(rootIndex, []byte(redirectHTML), 0644)
}

func extractCargodocArchive(archivePath, cacheDir, crateName string) error {
	if hasExtractedCargodoc(cacheDir) {
		return nil
	}

	cfg := getActiveConfig()
	maxExtractedSize := uint64(cfg.MaxCargodocSizeMb) << 20

	var archiveSize uint64 = 10 * 1024 * 1024
	if fi, err := os.Stat(archivePath); err == nil {
		archiveSize = max(uint64(fi.Size())*3, 10*1024*1024)
	}
	if !status.CanAllocateDiskSpace(nil, archiveSize) {
		return errors.New("insufficient disk space to extract cargo docs")
	}

	extractPath := getCargodocExtractPath(cfg)
	tempDir, err := os.MkdirTemp(extractPath, "renop-cargodoc-extract-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	if isGzipArchive(archivePath) {
		if err := extractTarGzCargodoc(archivePath, tempDir, crateName, maxExtractedSize); err != nil {
			if zipErr := extractZipCargodoc(archivePath, tempDir, crateName, maxExtractedSize); zipErr != nil {
				return err
			}
		}
	} else if isZipArchive(archivePath) {
		if err := extractZipCargodoc(archivePath, tempDir, crateName, maxExtractedSize); err != nil {
			if gzErr := extractTarGzCargodoc(archivePath, tempDir, crateName, maxExtractedSize); gzErr != nil {
				return err
			}
		}
	} else if strings.HasSuffix(strings.ToLower(archivePath), ".tar.gz") ||
		strings.HasSuffix(strings.ToLower(archivePath), ".tgz") || strings.HasSuffix(strings.ToLower(archivePath), ".crate") {
		if err := extractTarGzCargodoc(archivePath, tempDir, crateName, maxExtractedSize); err != nil {
			if zipErr := extractZipCargodoc(archivePath, tempDir, crateName, maxExtractedSize); zipErr != nil {
				return err
			}
		}
	} else if strings.HasSuffix(strings.ToLower(archivePath), ".zip") {
		if err := extractZipCargodoc(archivePath, tempDir, crateName, maxExtractedSize); err != nil {
			if gzErr := extractTarGzCargodoc(archivePath, tempDir, crateName, maxExtractedSize); gzErr != nil {
				return err
			}
		}
	} else {
		return errors.New("unsupported documentation archive format")
	}

	if err := ensureRootIndexHTML(tempDir, crateName); err != nil {
		return err
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

func candidateDocArchivePaths(storagePath, repoName, crateName, version string) []string {
	normalizedName := strings.ReplaceAll(strings.ToLower(crateName), "_", "-")
	return []string{
		filepath.Join(storagePath, repoName, "crates", crateName, crateName+"-"+version+"-docs.tar.gz"),
		filepath.Join(storagePath, repoName, "crates", crateName, crateName+"-"+version+"-docs.zip"),
		filepath.Join(storagePath, repoName, "crates", normalizedName, normalizedName+"-"+version+"-docs.tar.gz"),
		filepath.Join(storagePath, repoName, "crates", normalizedName, normalizedName+"-"+version+"-docs.zip"),
		filepath.Join(storagePath, repoName, "api", "v1", "crates", crateName, version, "docs.tar.gz"),
		filepath.Join(storagePath, repoName, "api", "v1", "crates", crateName, version, "docs.zip"),
		filepath.Join(storagePath, repoName, ".renop.docs", normalizedName, normalizedName+"-"+version+"-docs.tar.gz"),
		filepath.Join(storagePath, repoName, ".renop.docs", normalizedName, normalizedName+"-"+version+"-docs.zip"),
	}
}

// HasCargodoc reports whether extracted documentation or a valid doc archive exists.
func HasCargodoc(state *core.AppState, repoName, crateName, version string) bool {
	if state == nil || repoName == "" || crateName == "" || version == "" {
		return false
	}
	hash := cargodocHashKey(repoName, crateName, version)
	cfg := getActiveConfig()
	extractPath := getCargodocExtractPath(cfg)
	cacheDir := filepath.Join(extractPath, "renop-cargodoc-"+hash)
	if hasExtractedCargodoc(cacheDir) {
		return true
	}
	_, _, err := findDocArchive(state, repoName, crateName, version)
	return err == nil
}

func findDocArchive(state *core.AppState, repoName, crateName, version string) (string, bool, error) {
	storagePath := ""
	if state != nil && state.Inner != nil {
		if c := state.Inner.Config.Load(); c != nil {
			storagePath = c.StoragePath
		}
	}
	if storagePath == "" {
		storagePath = getActiveConfig().StoragePath
	}
	candidates := candidateDocArchivePaths(storagePath, repoName, crateName, version)

	for _, cand := range candidates {
		isS3 := false
		if IsS3Enabled != nil {
			isS3 = IsS3Enabled(cand)
		}
		if isS3 {
			if state.Inner != nil && state.Inner.FileIndex != nil && state.Inner.FileIndex.HasFile(cand) {
				return cand, true, nil
			}
		} else {
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				return cand, false, nil
			}
		}
	}
	return "", false, fiber.ErrNotFound
}

func EnsureCargodocExtractedBlocking(state *core.AppState, repoName, crateName, version string) (string, error) {
	hash := cargodocHashKey(repoName, crateName, version)
	cfg := getActiveConfig()
	extractPath := getCargodocExtractPath(cfg)
	cacheDir := filepath.Join(extractPath, "renop-cargodoc-"+hash)

	if hasExtractedCargodoc(cacheDir) {
		return cacheDir, nil
	}

	shard := 0
	if len(hash) > 0 {
		shard = int(hash[0]) % 256
	}
	cargodocLocks[shard].Lock()
	defer cargodocLocks[shard].Unlock()

	if hasExtractedCargodoc(cacheDir) {
		return cacheDir, nil
	}

	docArchivePath, isS3, err := findDocArchive(state, repoName, crateName, version)
	if err != nil {
		return "", err
	}

	maxArchiveSize := cfg.MaxCargodocSizeMb << 20
	var localArchivePath string

	if isS3 {
		s3Key := utils.GetS3Key(docArchivePath)
		tempFile, err := os.CreateTemp(extractPath, "renop-cargodoc-download-*.tmp")
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
				_ = tempFile.Close()
				return "", downloadErr
			}
			if info.Size > maxArchiveSize {
				_ = rc.Close()
				_ = tempFile.Close()
				return "", fmt.Errorf("%w: archive exceeds limit", errUnsafeCargodocArchive)
			}
			var written int64
			written, copyErr = io.Copy(tempFile, io.LimitReader(rc, maxArchiveSize+1))
			if copyErr == nil && written > maxArchiveSize {
				copyErr = fmt.Errorf("%w: archive exceeds limit", errUnsafeCargodocArchive)
			}
			_ = rc.Close()
		}
		_ = tempFile.Close()

		if copyErr != nil {
			return "", copyErr
		}
		localArchivePath = tempFileName
	} else {
		fi, err := os.Stat(docArchivePath)
		if err != nil {
			return "", fiber.ErrNotFound
		}
		if fi.Size() > maxArchiveSize {
			return "", fmt.Errorf("%w: archive exceeds limit", errUnsafeCargodocArchive)
		}
		localArchivePath = docArchivePath
	}

	if err := extractCargodocArchive(localArchivePath, cacheDir, crateName); err != nil {
		return "", err
	}

	return cacheDir, nil
}
