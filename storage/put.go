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
	"bufio"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/3JoB/unsafeConvert"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/panjf2000/ants/v2"

	"renop/config"
	"renop/core"
	"renop/index"
	"renop/javadocs"
	"renop/status"
	"renop/utils"
)

var bufferPool128k = sync.Pool{
	New: func() any {
		buf := make([]byte, 128*1024)
		return &buf
	},
}

var putWriterPool = sync.Pool{
	New: func() any {
		return bufio.NewWriterSize(nil, 128*1024)
	},
}

func WriteChecksumFile(parent string, baseName string, ext string, hash string, fileIndex *index.FileIndex) error {
	name := baseName + ext
	path := filepath.Join(parent, name)

	err := os.WriteFile(path, unsafeConvert.BytePointer(hash), 0644)
	if err != nil {
		return err
	}

	fileIndex.InsertFile(path, index.FileInfo{
		Size:    int64(len(hash)),
		ModTime: time.Now().UnixNano(),
	})
	status.MarkStorageUpdated()
	return nil
}

func HandlePut(c fiber.Ctx, state *core.AppState, repo *config.Repository, localFilePath string) error {
	lockKey := filepath.ToSlash(localFilePath)
	var upload *core.InFlightDownload
	for {
		var loaded bool
		upload, loaded = state.Inner.InFlightDownloads.LockPath(lockKey)
		if !loaded {
			break
		}
		state.Inner.InFlightDownloads.Wait(upload)
	}
	uploadSucceeded := false
	defer func() {
		state.Inner.InFlightDownloads.UnlockPath(lockKey, upload, uploadSucceeded)
	}()

	contentLength := c.Request().Header.ContentLength()
	var estimatedSize int64
	if contentLength > 0 {
		estimatedSize = int64(contentLength)
	} else {
		estimatedSize = int64(len(c.Body()))
	}
	estimatedRequired := EstimateUploadDiskSpace(localFilePath, estimatedSize)

	if !status.CanAllocateDiskSpace(state, uint64(estimatedRequired)) {
		return c.Status(fiber.StatusInsufficientStorage).JSON(fiber.Map{
			"error": "Insufficient disk space to upload file",
		})
	}

	exists := PathExistsForUpload(state, localFilePath)

	if !repo.AllowRedeployment && exists {
		return c.Status(fiber.StatusConflict).SendString("Conflict")
	}

	generateChecksums := c.Get("X-Generate-Checksums") == "true"

	parentDir := filepath.Dir(localFilePath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	uniqueID := uuid.New().String()
	tmpPath := localFilePath + ".tmp." + uniqueID

	file, err := os.Create(tmpPath)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	keepTmp := false
	fileClosed := false
	defer func() {
		if !fileClosed {
			_ = file.Close()
			fileClosed = true
		}
		if !keepTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	md5Hasher := md5.New()
	sha1Hasher := sha1.New()
	sha256Hasher := sha256.New()
	sha512Hasher := sha512.New()

	bufWriter := putWriterPool.Get().(*bufio.Writer)
	bufWriter.Reset(file)
	defer func() {
		bufWriter.Reset(nil)
		putWriterPool.Put(bufWriter)
	}()

	var writeDest io.Writer
	if generateChecksums {
		writeDest = io.MultiWriter(bufWriter, md5Hasher, sha1Hasher, sha256Hasher, sha512Hasher)
	} else {
		writeDest = bufWriter
	}

	bodyReader := c.Request().BodyStream()
	if bodyReader == nil {
		bodyData := c.Body()
		if _, err := writeDest.Write(bodyData); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
		}
	} else {
		bufPtr := bufferPool128k.Get().(*[]byte)
		buf := *bufPtr
		_, err := io.CopyBuffer(writeDest, bodyReader, buf)
		bufferPool128k.Put(bufPtr)
		if err != nil && err != io.EOF {
			return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
		}
	}

	if err := bufWriter.Flush(); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	var fileSize int64
	var modTime int64
	if stat, err := file.Stat(); err == nil {
		fileSize = stat.Size()
		modTime = stat.ModTime().UnixNano()
	}

	if err := file.Close(); err != nil {
		fileClosed = true
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}
	fileClosed = true

	var digests *ContentDigests
	if generateChecksums {
		digests = &ContentDigests{
			MD5:    hex.EncodeToString(md5Hasher.Sum(nil)),
			SHA1:   hex.EncodeToString(sha1Hasher.Sum(nil)),
			SHA256: hex.EncodeToString(sha256Hasher.Sum(nil)),
			SHA512: hex.EncodeToString(sha512Hasher.Sum(nil)),
		}
	}

	if err := CommitUploadedFile(state, localFilePath, tmpPath, fileSize, modTime, exists, generateChecksums, digests); err != nil {
		msg := "Internal Server Error"
		if strings.HasPrefix(err.Error(), "Failed to upload to S3:") {
			msg = err.Error()
		}
		return c.Status(fiber.StatusInternalServerError).SendString(msg)
	}
	keepTmp = true
	uploadSucceeded = true

	return c.Status(fiber.StatusCreated).SendString("")
}

// ContentDigests holds hex digests produced during upload (optional).
type ContentDigests struct {
	MD5    string
	SHA1   string
	SHA256 string
	SHA512 string
}

// HashFile computes MD5/SHA1/SHA256/SHA512 for an on-disk file.
func HashFile(path string) (*ContentDigests, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	md5Hasher := md5.New()
	sha1Hasher := sha1.New()
	sha256Hasher := sha256.New()
	sha512Hasher := sha512.New()
	w := io.MultiWriter(md5Hasher, sha1Hasher, sha256Hasher, sha512Hasher)

	bufPtr := bufferPool128k.Get().(*[]byte)
	buf := *bufPtr
	n, err := io.CopyBuffer(w, f, buf)
	bufferPool128k.Put(bufPtr)
	if err != nil {
		return nil, 0, err
	}

	return &ContentDigests{
		MD5:    hex.EncodeToString(md5Hasher.Sum(nil)),
		SHA1:   hex.EncodeToString(sha1Hasher.Sum(nil)),
		SHA256: hex.EncodeToString(sha256Hasher.Sum(nil)),
		SHA512: hex.EncodeToString(sha512Hasher.Sum(nil)),
	}, n, nil
}

// CommitUploadedFile places a closed temp file at localFilePath and runs the same
// post-upload bookkeeping as a normal PUT.
func CommitUploadedFile(state *core.AppState, localFilePath, tmpPath string, fileSize, modTime int64, existed, generateChecksums bool, digests *ContentDigests) error {
	if generateChecksums && digests == nil {
		return errors.New("missing content digests")
	}

	if IsS3Enabled(localFilePath) {
		s3Key := utils.GetS3Key(localFilePath)
		if err := UploadToS3(tmpPath, s3Key); err != nil {
			return fmt.Errorf("Failed to upload to S3: %w", err)
		}
		_ = os.Remove(tmpPath)
	} else {
		if err := utils.SafeRename(tmpPath, localFilePath); err != nil {
			return err
		}
	}

	state.Inner.FileIndex.EnsureParentDirs(localFilePath)
	state.Inner.FileIndex.InsertFile(localFilePath, index.FileInfo{
		Size:    fileSize,
		ModTime: modTime,
	})
	status.MarkStorageUpdated()

	if isSnapshotArtifactPath(localFilePath) && !isArtifactCompanionPath(localFilePath) {
		if existed {
			removeArtifactCompanions(state, localFilePath)
		}
		cleanupSupersededUniqueSnapshots(state, localFilePath)
	}

	if generateChecksums {
		checksums := []struct {
			ext  string
			hash string
		}{
			{ext: ".md5", hash: digests.MD5},
			{ext: ".sha1", hash: digests.SHA1},
			{ext: ".sha256", hash: digests.SHA256},
			{ext: ".sha512", hash: digests.SHA512},
		}

		var wg sync.WaitGroup
		var firstErr error
		var errOnce sync.Once
		for _, cs := range checksums {
			wg.Add(1)
			checksum := cs
			submitErr := ants.Submit(func() {
				defer wg.Done()
				if err := SaveAndUploadChecksum(state, localFilePath, checksum.ext, checksum.hash); err != nil {
					errOnce.Do(func() { firstErr = err })
				}
			})
			if submitErr != nil {
				if err := SaveAndUploadChecksum(state, localFilePath, checksum.ext, checksum.hash); err != nil {
					errOnce.Do(func() { firstErr = err })
				}
				wg.Done()
			}
		}
		wg.Wait()

		if firstErr != nil {
			deleteFileHelper(localFilePath)
			state.Inner.FileIndex.RemoveFile(localFilePath)
			for _, generated := range checksums {
				checksumPath := localFilePath + generated.ext
				deleteFileHelper(checksumPath)
				state.Inner.FileIndex.RemoveFile(checksumPath)
			}
			state.InvalidateFileCache(localFilePath)
			return firstErr
		}
	}

	if strings.HasSuffix(localFilePath, "-javadoc.jar") {
		javadocs.CleanupJavadoc(localFilePath)
		if !IsS3Enabled(localFilePath) {
			_ = ants.Submit(func() {
				_, _ = javadocs.EnsureJavadocExtractedBlocking(localFilePath)
			})
		}
	}

	state.InvalidateFileCache(localFilePath)
	return nil
}

// EstimateUploadDiskSpace returns the conservative free-space requirement used by PUT.
func EstimateUploadDiskSpace(localFilePath string, contentLength int64) int64 {
	estimatedRequired := contentLength
	if estimatedRequired <= 0 {
		estimatedRequired = 10 * 1024 * 1024
	}
	if strings.HasSuffix(strings.ToLower(localFilePath), "-javadoc.jar") {
		return estimatedRequired * 4
	}
	return estimatedRequired*2 + 1024*1024
}

// PathExistsForUpload reports whether the destination already exists (index or disk).
func PathExistsForUpload(state *core.AppState, localFilePath string) bool {
	exists := state.Inner.FileIndex.HasFile(localFilePath) || state.Inner.FileIndex.HasDir(localFilePath)
	if !IsS3Enabled(localFilePath) {
		_, err := os.Stat(localFilePath)
		exists = err == nil
	}
	return exists
}

func uploadAndCleanup(localPath string) error {
	if !IsS3Enabled(localPath) {
		return nil
	}
	s3Key := utils.GetS3Key(localPath)

	err := UploadToS3(localPath, s3Key)
	if err != nil {
		return err
	}
	_ = os.Remove(localPath)
	return nil
}
