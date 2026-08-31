/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/klauspost/compress/zip"
	brrr "github.com/molecule-man/go-brrr"

	"renop/internal/utils"
)

var (
	isInstalling         atomic.Bool
	CanAllocateDiskSpace func(requiredBytes uint64) bool
)

const (
	maxUpdatePackageSize    int64 = 2 << 30
	maxUpdateExecutableSize       = 512 << 20
)

type pendingBinaryState struct {
	mu   sync.Mutex
	path string
}

func (s *pendingBinaryState) set(newPath string) {
	s.mu.Lock()
	old := s.path
	s.path = newPath
	s.mu.Unlock()
	if old != "" && old != newPath {
		_ = os.Remove(old)
	}
}

func (s *pendingBinaryState) get() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path
}

func (s *pendingBinaryState) consume() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.path
	s.path = ""
	return p
}

var pendingBinary pendingBinaryState

func targetExecutableName() string {
	if runtime.GOOS == "windows" {
		return "renop.exe"
	}
	return "renop"
}

func archiveMatchesPlatform(name, goos, goarch string) bool {
	nameLower := strings.ToLower(filepath.ToSlash(name))
	goos = strings.ToLower(goos)
	goarch = strings.ToLower(goarch)

	markers := []string{goos + "-" + goarch}
	if goarch == "amd64" {
		markers = []string{
			goos + "-amd64v4",
			goos + "-amd64v3",
			goos + "-amd64v2",
			goos + "-amd64v1",
			goos + "-amd64",
		}
	}

	for _, marker := range markers {
		for _, src := range []string{filepath.Base(nameLower), nameLower} {
			idx := strings.Index(src, marker)
			if idx < 0 {
				continue
			}
			end := idx + len(marker)
			if end >= len(src) {
				return true
			}
			switch src[end] {
			case '.', '-', '_', '/', '\\':
				return true
			}
		}
	}
	return false
}

// ExtractExecutableFromZip locates and validates the current-platform executable in a legacy ZIP package.
func ExtractExecutableFromZip(zipTempFile *os.File) (string, error) {
	fi, err := zipTempFile.Stat()
	if err != nil {
		return "", err
	}
	if fi.Size() < 0 || fi.Size() > maxUpdatePackageSize {
		return "", fmt.Errorf("update package exceeds %d bytes", maxUpdatePackageSize)
	}

	zipReader, err := zip.NewReader(zipTempFile, fi.Size())
	if err != nil {
		return "", fmt.Errorf("invalid zip file: %w", err)
	}

	exeName := targetExecutableName()
	goos, goarch := runtime.GOOS, runtime.GOARCH

	var platformInner *zip.File
	var anyInner []*zip.File
	var directExe *zip.File

	for _, f := range zipReader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(strings.ToLower(filepath.ToSlash(f.Name)))
		switch {
		case strings.HasSuffix(base, ".zip"):
			anyInner = append(anyInner, f)
			if archiveMatchesPlatform(f.Name, goos, goarch) {
				platformInner = f
			}
		case base == exeName:
			if directExe == nil {
				directExe = f
			}
		}
	}

	if platformInner != nil {
		path, err := extractExecutableFromNestedZip(platformInner, exeName)
		if err == nil {
			return finalizeExtractedBinary(path)
		}
	}

	if directExe != nil {
		path, err := materializeZipEntryAsExecutable(directExe)
		if err == nil {
			return finalizeExtractedBinary(path)
		}
	}

	for _, inner := range anyInner {
		if platformInner != nil && inner == platformInner {
			continue
		}
		path, err := extractExecutableFromNestedZip(inner, exeName)
		if err != nil {
			continue
		}
		out, err := finalizeExtractedBinary(path)
		if err != nil {
			continue
		}
		return out, nil
	}

	return "", fmt.Errorf("target executable not found in update package for %s/%s", goos, goarch)
}

// ExtractExecutableFromBrotli decompresses a raw Brotli executable with a strict output bound.
func ExtractExecutableFromBrotli(packageFile *os.File) (string, error) {
	if packageFile == nil {
		return "", errors.New("brotli update package is missing")
	}
	info, err := packageFile.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() <= 0 || info.Size() > maxUpdatePackageSize {
		return "", fmt.Errorf("update package exceeds %d bytes", maxUpdatePackageSize)
	}
	if _, err := packageFile.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	reader := brrr.NewReader(packageFile)
	targetFile, err := os.CreateTemp("", "renop-new-*")
	if err != nil {
		_ = reader.Close()
		return "", err
	}
	targetPath := targetFile.Name()
	removeTarget := true
	defer func() {
		_ = reader.Close()
		_ = targetFile.Close()
		if removeTarget {
			_ = os.Remove(targetPath)
		}
	}()
	buffer := make([]byte, 128*1024)
	written, copyErr := io.CopyBuffer(targetFile, io.LimitReader(reader, maxUpdateExecutableSize+1), buffer)
	if copyErr != nil {
		return "", fmt.Errorf("decompress Brotli update package: %w", copyErr)
	}
	if err := reader.Close(); err != nil {
		return "", fmt.Errorf("finalize Brotli update package: %w", err)
	}
	if written > maxUpdateExecutableSize {
		return "", fmt.Errorf("update executable exceeds %d bytes", maxUpdateExecutableSize)
	}
	if err := targetFile.Sync(); err != nil {
		return "", err
	}
	if err := targetFile.Close(); err != nil {
		return "", err
	}
	removeTarget = false
	return finalizeExtractedBinary(targetPath)
}

// IsSupportedUpdatePackageName reports whether a package name selects ZIP or raw Brotli decoding.
func IsSupportedUpdatePackageName(name string) bool {
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	return extension == ".zip" || extension == ".br"
}

// EstimateUploadedPackageDiskSpace returns a conservative temporary-space budget for an offline package.
func EstimateUploadedPackageDiskSpace(name string, compressedSize int64) int64 {
	const fallback = int64(100 << 20)
	if compressedSize <= 0 {
		return fallback
	}
	factor := int64(3)
	if strings.EqualFold(filepath.Ext(strings.TrimSpace(name)), ".br") {
		factor = 6
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	if compressedSize > maxInt64/factor {
		return maxInt64
	}
	estimate := compressedSize * factor
	if estimate < fallback {
		return fallback
	}
	return estimate
}

func extractExecutableFromPackage(packageFile *os.File, packageName string) (string, error) {
	if packageFile == nil {
		return "", errors.New("update package is missing")
	}
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(packageName)))
	if extension == ".br" {
		return ExtractExecutableFromBrotli(packageFile)
	}
	if extension == ".zip" {
		return ExtractExecutableFromZip(packageFile)
	}
	var signature [4]byte
	if _, err := packageFile.ReadAt(signature[:], 0); err == nil &&
		signature[0] == 'P' && signature[1] == 'K' {
		return ExtractExecutableFromZip(packageFile)
	}
	return ExtractExecutableFromBrotli(packageFile)
}

func extractExecutableFromNestedZip(inner *zip.File, exeName string) (string, error) {
	if inner.UncompressedSize64 > uint64(maxUpdatePackageSize) {
		return "", fmt.Errorf("nested update package exceeds %d bytes", maxUpdatePackageSize)
	}
	rc, err := inner.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	innerTemp, err := os.CreateTemp("", "renop-inner-*.zip")
	if err != nil {
		return "", err
	}
	innerPath := innerTemp.Name()
	defer func() {
		_ = innerTemp.Close()
		_ = os.Remove(innerPath)
	}()

	if _, err := io.Copy(innerTemp, rc); err != nil {
		return "", err
	}
	if err := innerTemp.Sync(); err != nil {
		return "", err
	}
	if _, err := innerTemp.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	ifi, err := innerTemp.Stat()
	if err != nil {
		return "", err
	}
	innerReader, err := zip.NewReader(innerTemp, ifi.Size())
	if err != nil {
		return "", fmt.Errorf("invalid nested zip %s: %w", inner.Name, err)
	}

	for _, f := range innerReader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if filepath.Base(strings.ToLower(filepath.ToSlash(f.Name))) == exeName {
			return materializeZipEntryAsExecutable(f)
		}
	}
	return "", fmt.Errorf("executable %s not found in nested package %s", exeName, filepath.Base(inner.Name))
}

func materializeZipEntryAsExecutable(f *zip.File) (string, error) {
	if f.FileInfo().IsDir() {
		return "", errors.New("entry is a directory")
	}
	if f.UncompressedSize64 > uint64(maxUpdateExecutableSize) {
		return "", fmt.Errorf("update executable exceeds %d bytes", maxUpdateExecutableSize)
	}
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	targetFile, err := os.CreateTemp("", "renop-new-*")
	if err != nil {
		return "", err
	}
	targetPath := targetFile.Name()

	bufOut := bufio.NewWriterSize(targetFile, 128*1024)
	if _, err := io.Copy(bufOut, rc); err != nil {
		_ = targetFile.Close()
		_ = os.Remove(targetPath)
		return "", err
	}
	if err := bufOut.Flush(); err != nil {
		_ = targetFile.Close()
		_ = os.Remove(targetPath)
		return "", err
	}
	if err := targetFile.Close(); err != nil {
		_ = os.Remove(targetPath)
		return "", err
	}
	return targetPath, nil
}

func finalizeExtractedBinary(path string) (string, error) {
	if err := ValidateExecutableBinary(path); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(path, 0755)
	}
	return path, nil
}

var ErrIncompatibleBinary = errors.New("executable binary does not match current system or architecture")

func ValidateExecutableBinary(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	switch runtime.GOOS {
	case "windows":
		return validateWindowsPE(f)
	case "darwin":
		return validateMachO(f)
	default:
		return validateELF(f, runtime.GOOS, runtime.GOARCH)
	}
}

func validateWindowsPE(f *os.File) error {
	pf, err := pe.NewFile(f)
	if err != nil {
		return fmt.Errorf("%w: not a valid Windows PE executable", ErrIncompatibleBinary)
	}
	defer pf.Close()

	want, known := peMachineForGOARCH(runtime.GOARCH)
	if !known {
		return nil
	}
	if pf.Machine != want {
		return fmt.Errorf("%w: expected Windows %s, got machine 0x%x", ErrIncompatibleBinary, runtime.GOARCH, pf.Machine)
	}
	return nil
}

func peMachineForGOARCH(goarch string) (machine uint16, ok bool) {
	switch goarch {
	case "amd64":
		return pe.IMAGE_FILE_MACHINE_AMD64, true
	case "arm64":
		return pe.IMAGE_FILE_MACHINE_ARM64, true
	case "386":
		return pe.IMAGE_FILE_MACHINE_I386, true
	default:
		return 0, false
	}
}

func validateELF(f *os.File, goos, goarch string) error {
	ef, err := elf.NewFile(f)
	if err != nil {
		return fmt.Errorf("%w: not a valid %s ELF executable", ErrIncompatibleBinary, goos)
	}
	defer ef.Close()

	wantMachine, wantClass, wantData, known := elfIdentityForGOARCH(goarch)
	if !known {
		return nil
	}
	if ef.Machine != wantMachine {
		return fmt.Errorf("%w: expected %s %s, got machine %s", ErrIncompatibleBinary, goos, goarch, ef.Machine)
	}
	if ef.Class != wantClass {
		return fmt.Errorf("%w: expected %s %s class %v, got %v", ErrIncompatibleBinary, goos, goarch, wantClass, ef.Class)
	}
	if wantData != 0 && ef.Data != wantData {
		return fmt.Errorf("%w: expected %s %s endian %v, got %v", ErrIncompatibleBinary, goos, goarch, wantData, ef.Data)
	}
	return nil
}

func elfIdentityForGOARCH(goarch string) (machine elf.Machine, class elf.Class, data elf.Data, ok bool) {
	switch goarch {
	case "amd64":
		return elf.EM_X86_64, elf.ELFCLASS64, elf.ELFDATA2LSB, true
	case "arm64":
		return elf.EM_AARCH64, elf.ELFCLASS64, elf.ELFDATA2LSB, true
	case "386":
		return elf.EM_386, elf.ELFCLASS32, elf.ELFDATA2LSB, true
	case "arm":
		return elf.EM_ARM, elf.ELFCLASS32, elf.ELFDATA2LSB, true
	case "riscv64":
		return elf.EM_RISCV, elf.ELFCLASS64, elf.ELFDATA2LSB, true
	case "loong64":
		return elf.EM_LOONGARCH, elf.ELFCLASS64, elf.ELFDATA2LSB, true
	default:
		return 0, 0, 0, false
	}
}

func validateMachO(f *os.File) error {
	mf, err := macho.NewFile(f)
	if err != nil {
		fatf, fatErr := macho.NewFatFile(f)
		if fatErr != nil {
			return fmt.Errorf("%w: not a valid macOS Mach-O executable", ErrIncompatibleBinary)
		}
		defer fatf.Close()

		want, known := machoCPUForGOARCH(runtime.GOARCH)
		if !known {
			return nil
		}
		for _, arch := range fatf.Arches {
			if arch.Cpu == want {
				return nil
			}
		}
		return fmt.Errorf("%w: macOS Fat binary does not contain %s architecture", ErrIncompatibleBinary, runtime.GOARCH)
	}
	defer mf.Close()

	want, known := machoCPUForGOARCH(runtime.GOARCH)
	if !known {
		return nil
	}
	if mf.Cpu != want {
		return fmt.Errorf("%w: expected macOS %s, got CPU 0x%x", ErrIncompatibleBinary, runtime.GOARCH, mf.Cpu)
	}
	return nil
}

func machoCPUForGOARCH(goarch string) (cpu macho.Cpu, ok bool) {
	switch goarch {
	case "amd64":
		return macho.CpuAmd64, true
	case "arm64":
		return macho.CpuArm64, true
	case "386":
		return macho.Cpu386, true
	default:
		return 0, false
	}
}

// SaveAndExtractUploadedZip preserves the legacy ZIP-specific entry point.
func SaveAndExtractUploadedZip(fileHeader *multipart.FileHeader) (string, error) {
	return SaveAndExtractUploadedPackage(fileHeader)
}

// SaveAndExtractUploadedPackage streams an uploaded ZIP or Brotli package to bounded temporary storage.
func SaveAndExtractUploadedPackage(fileHeader *multipart.FileHeader) (string, error) {
	if fileHeader == nil || !IsSupportedUpdatePackageName(fileHeader.Filename) {
		return "", errors.New("uploaded file must be a .br or .zip package")
	}
	src, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	tempZip, err := os.CreateTemp("", "renop-upload-*"+strings.ToLower(filepath.Ext(fileHeader.Filename)))
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tempZipPath := tempZip.Name()
	defer func() {
		_ = tempZip.Close()
		_ = os.Remove(tempZipPath)
	}()

	bufWriter := bufio.NewWriterSize(tempZip, 128*1024)
	written, copyErr := io.Copy(bufWriter, io.LimitReader(src, maxUpdatePackageSize+1))
	if written > maxUpdatePackageSize {
		return "", fmt.Errorf("update package exceeds %d bytes", maxUpdatePackageSize)
	}
	if copyErr != nil {
		return "", fmt.Errorf("failed to save uploaded file: %w", copyErr)
	}
	if err := bufWriter.Flush(); err != nil {
		return "", err
	}

	return extractExecutableFromPackage(tempZip, fileHeader.Filename)
}

// ExtractExecutableFromZipPath preserves the legacy ZIP-path entry point.
func ExtractExecutableFromZipPath(zipPath string) (string, error) {
	return ExtractExecutableFromPackagePath(zipPath, filepath.Base(zipPath))
}

// ExtractExecutableFromPackagePath opens and decodes a ZIP or raw Brotli update package.
func ExtractExecutableFromPackagePath(packagePath, packageName string) (string, error) {
	f, err := os.Open(packagePath)
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer f.Close()
	return extractExecutableFromPackage(f, packageName)
}

func TryBeginInstall() bool {
	return isInstalling.CompareAndSwap(false, true)
}

func EndInstall() {
	isInstalling.Store(false)
}

func SetDownloadingProgress(progress int) {
	updateStateFields(func(s *UpdateState) {
		s.Status = "downloading"
		s.Progress = progress
		s.ErrorMessage = ""
	})
}

func SetError(msg string) {
	updateStateFields(func(s *UpdateState) {
		s.Status = "error"
		s.ErrorMessage = strings.Clone(msg)
	})
}

func SetReadyToRestart(binaryPath, latestVersion string) {
	pendingBinary.set(binaryPath)
	updateStateFields(func(s *UpdateState) {
		s.Status = "ready_to_restart"
		s.Progress = 100
		s.LatestVersion = strings.Clone(latestVersion)
		s.ErrorMessage = ""
	})
}

func newDownloadHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: checkHTTPSRedirect,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout: 15 * time.Second,
			}).DialContext,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
			ForceAttemptHTTP2:      false,
			DisableCompression:     true,
			DisableKeepAlives:      true,
			MaxIdleConns:           0,
			MaxIdleConnsPerHost:    0,
			MaxConnsPerHost:        2,
			IdleConnTimeout:        time.Second,
			TLSHandshakeTimeout:    15 * time.Second,
			ResponseHeaderTimeout:  30 * time.Second,
			ExpectContinueTimeout:  1 * time.Second,
			MaxResponseHeaderBytes: 256 << 10,
		},
		Timeout: 0,
	}
}

var sharedDownloadClient lazyHTTPClient

// downloadHTTPClient returns the lazily initialized bounded download client.
// Downloads stream directly to a temporary file, so a shared transport does
// not retain package-sized response buffers between operations.
func downloadHTTPClient() *http.Client {
	sharedDownloadClient.Do(func() {
		sharedDownloadClient.client = newDownloadHTTPClient()
	})
	return sharedDownloadClient.client
}

func DownloadAndExtract(ctx context.Context, downloadURL, expectedSHA256 string) (string, error) {
	parsedURL, err := url.Parse(downloadURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil {
		return "", errors.New("update download URL must be an HTTPS URL without user info")
	}
	expectedDigest, err := hex.DecodeString(strings.TrimSpace(expectedSHA256))
	if err != nil || len(expectedDigest) != sha256.Size {
		return "", errors.New("update package is missing a valid SHA-256 digest")
	}

	client := downloadHTTPClient()
	defer utils.ScheduleNetworkWorkingSetTrim()

	zipTempFile, err := os.CreateTemp("", "renop-download-*")
	if err != nil {
		return "", err
	}
	zipPath := zipTempFile.Name()
	defer func() {
		_ = zipTempFile.Close()
		_ = os.Remove(zipPath)
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "RenoP-Updater")
	req.Close = true

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer utils.DrainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}
	if resp.ContentLength > maxUpdatePackageSize {
		return "", fmt.Errorf("update package exceeds %d bytes", maxUpdatePackageSize)
	}

	bufWriter := bufio.NewWriterSize(zipTempFile, 32*1024)
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(bufWriter, hasher), io.LimitReader(resp.Body, maxUpdatePackageSize+1))
	if written > maxUpdatePackageSize {
		return "", fmt.Errorf("update package exceeds %d bytes", maxUpdatePackageSize)
	}
	flushErr := bufWriter.Flush()
	if copyErr != nil {
		return "", fmt.Errorf("failed to save update package: %w", copyErr)
	}
	if flushErr != nil {
		return "", fmt.Errorf("failed to flush update package: %w", flushErr)
	}
	if subtle.ConstantTimeCompare(hasher.Sum(nil), expectedDigest) != 1 {
		return "", errors.New("update package SHA-256 mismatch")
	}

	return extractExecutableFromPackage(zipTempFile, parsedURL.Path)
}

func moveOrCopyFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	dstDir := filepath.Dir(dst)
	tmpDst, err := os.CreateTemp(dstDir, ".renop-new-*")
	if err != nil {
		return err
	}

	tmpDstPath := tmpDst.Name()
	bufWriter := bufio.NewWriterSize(tmpDst, 128*1024)
	if _, err := io.Copy(bufWriter, in); err != nil {
		_ = tmpDst.Close()
		_ = os.Remove(tmpDstPath)
		return err
	}
	if err := bufWriter.Flush(); err != nil {
		_ = tmpDst.Close()
		_ = os.Remove(tmpDstPath)
		return err
	}
	if err := tmpDst.Close(); err != nil {
		_ = os.Remove(tmpDstPath)
		return err
	}

	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmpDstPath, 0755)
	}

	if err := os.Rename(tmpDstPath, dst); err != nil {
		_ = os.Remove(tmpDstPath)
		return err
	}

	if err := in.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

func CleanOldExecutables() {
	currentExe, err := os.Executable()
	if err != nil {
		return
	}
	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		return
	}
	oldExe := currentExe + ".old"
	_ = os.Remove(oldExe)

	cleanupStaleUpdaterTemps()
}

func cleanupStaleUpdaterTemps() {
	dir := os.TempDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	keep := pendingBinary.get()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		path := filepath.Join(dir, name)
		if keep != "" && path == keep {
			continue
		}
		switch {
		case strings.HasPrefix(name, "renop-upload-"),
			strings.HasPrefix(name, "renop-download-"),
			strings.HasPrefix(name, "renop-inner-"),
			strings.HasPrefix(name, "renop-new-"):
			_ = os.Remove(path)
		}
	}
}

func ApplyUpdateAndRestart(newBinaryPath string) error {
	currentExe, err := os.Executable()
	if err != nil {
		return err
	}
	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		return err
	}

	oldExe := currentExe + ".old"
	_ = os.Remove(oldExe)

	if err := os.Rename(currentExe, oldExe); err != nil {
		return fmt.Errorf("failed to backup current executable: %w", err)
	}

	if err := moveOrCopyFile(newBinaryPath, currentExe); err != nil {
		_ = os.Rename(oldExe, currentExe)
		return fmt.Errorf("failed to replace executable: %w", err)
	}

	pendingBinary.consume()

	if runtime.GOOS != "windows" {
		_ = os.Chmod(currentExe, 0755)
	}

	return scheduleReexec(currentExe)
}

// RestartProcess re-executes the current binary without applying an update.
// Used by Settings → Restart when no pending update package is ready.
func RestartProcess() error {
	currentExe, err := os.Executable()
	if err != nil {
		return err
	}
	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		return err
	}
	return scheduleReexec(currentExe)
}

func scheduleReexec(exePath string) error {
	go func() {
		time.Sleep(500 * time.Millisecond)
		err := reexecProcess(exePath)
		log.Printf("[Updater] Failed to restart process: %v", err)
		os.Exit(1)
	}()
	return nil
}
