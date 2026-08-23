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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"renop/internal/core"
	"renop/internal/service/docker"
	"renop/internal/service/index"
	"renop/internal/service/status"
	"renop/internal/utils"
)

type dockerStore struct {
	storagePath string
}

func NewDockerStore(storagePath string) docker.Store {
	return &dockerStore{storagePath: storagePath}
}

type dockerStagedBlob struct {
	tempPath string
	file     *os.File
	closed   bool
}

var _ docker.Store = (*dockerStore)(nil)
var _ docker.StagedBlob = (*dockerStagedBlob)(nil)

func cleanDigestHash(digest string) string {
	digest = strings.TrimSpace(digest)
	if idx := strings.Index(digest, ":"); idx != -1 {
		digest = digest[idx+1:]
	}
	return strings.ToLower(digest)
}

func (s *dockerStore) blobPath(repository, digest string) string {
	hash := cleanDigestHash(digest)
	return filepath.Join(s.storagePath, repository, "blobs", "sha256", hash)
}

func (s *dockerStore) stagingPath(repository, uploadUUID string) string {
	return filepath.Join(s.storagePath, repository, ".renop.tmp.docker", uploadUUID)
}

func (s *dockerStore) manifestPath(repository, imageName, digest string) string {
	hash := cleanDigestHash(digest)
	return filepath.Join(s.storagePath, repository, "manifests", filepath.FromSlash(imageName), hash)
}

func (s *dockerStore) OpenBlob(repository, digest string) (io.ReadCloser, int64, bool, error) {
	path := s.blobPath(repository, digest)
	if IsS3Enabled(path) {
		reader, info, err := DownloadFromS3(utils.GetS3Key(path))
		if err != nil {
			if isS3NotFound(err) {
				return nil, 0, false, nil
			}
			return nil, 0, false, err
		}
		return reader, info.Size, true, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, false, nil
		}
		return nil, 0, false, err
	}

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, false, nil
		}
		return nil, 0, false, err
	}
	return file, info.Size(), true, nil
}

func (s *dockerStore) BlobExists(repository, digest string) (bool, int64, error) {
	path := s.blobPath(repository, digest)
	if IsS3Enabled(path) {
		info, err := StatS3(utils.GetS3Key(path))
		if err == nil {
			return true, info.Size, nil
		}
		if isS3NotFound(err) {
			return false, 0, nil
		}
		return false, 0, err
	}

	info, err := os.Stat(path)
	if err == nil {
		return true, info.Size(), nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, 0, nil
	}
	return false, 0, err
}

func (s *dockerStore) BlobFilePath(repository, digest string) (string, bool) {
	path := s.blobPath(repository, digest)
	if IsS3Enabled(path) {
		return "", false
	}
	if _, err := os.Stat(path); err == nil {
		return path, true
	}
	return "", false
}

func (s *dockerStore) StageBlob(repository, uploadUUID string) (docker.StagedBlob, error) {
	tempPath := s.stagingPath(repository, uploadUUID)
	if err := os.MkdirAll(filepath.Dir(tempPath), 0755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	return &dockerStagedBlob{tempPath: tempPath, file: file}, nil
}

func (s *dockerStore) GetStagedBlob(repository, uploadUUID string) (docker.StagedBlob, error) {
	tempPath := s.stagingPath(repository, uploadUUID)
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, core.ErrDockerBlobUploadNotFound
		}
		return nil, err
	}
	return &dockerStagedBlob{tempPath: tempPath, file: file}, nil
}

func (b *dockerStagedBlob) Write(data []byte) (int, error) {
	if b == nil || b.file == nil || b.closed {
		return 0, fs.ErrClosed
	}
	return b.file.Write(data)
}

func (b *dockerStagedBlob) WriteAt(p []byte, off int64) (n int, err error) {
	if b == nil || b.file == nil || b.closed {
		return 0, fs.ErrClosed
	}
	return b.file.WriteAt(p, off)
}

func (b *dockerStagedBlob) Size() (int64, error) {
	if b == nil || b.file == nil {
		return 0, fs.ErrClosed
	}
	info, err := b.file.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (b *dockerStagedBlob) Close() error {
	if b == nil || b.file == nil || b.closed {
		return nil
	}
	b.closed = true
	return b.file.Close()
}

func (b *dockerStagedBlob) Discard() error {
	if b == nil {
		return nil
	}
	closeErr := b.Close()
	if b.tempPath != "" {
		_ = os.Remove(b.tempPath)
	}
	return closeErr
}

func (b *dockerStagedBlob) Digest() (string, error) {
	if b == nil || b.tempPath == "" {
		return "", fs.ErrClosed
	}
	if !b.closed && b.file != nil {
		_ = b.file.Sync()
	}
	f, err := os.Open(b.tempPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *dockerStore) CommitBlob(state *core.AppState, repository, uploadUUID, digest string) (int64, error) {
	tempPath := s.stagingPath(repository, uploadUUID)
	targetPath := s.blobPath(repository, digest)

	info, err := os.Stat(tempPath)
	if err != nil {
		return 0, fmt.Errorf("stat staged blob: %w", err)
	}
	size := info.Size()

	f, err := os.Open(tempPath)
	if err != nil {
		return 0, err
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		_ = f.Close()
		return 0, err
	}
	_ = f.Close()

	actualDigest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if actualDigest != digest {
		return 0, fmt.Errorf("digest mismatch: expected %s, got %s", digest, actualDigest)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return 0, err
	}

	if IsS3Enabled(targetPath) {
		if err := UploadToS3(tempPath, utils.GetS3Key(targetPath)); err != nil {
			return 0, err
		}
		_ = os.Remove(tempPath)
	} else {
		if err := utils.SafeRename(tempPath, targetPath); err != nil {
			return 0, err
		}
	}

	if state != nil && state.Inner != nil && state.Inner.FileIndex != nil {
		state.Inner.FileIndex.EnsureParentDirs(targetPath)
		state.Inner.FileIndex.InsertFile(targetPath, index.FileInfo{Size: size, ModTime: info.ModTime().UnixNano()})
		state.InvalidateFileCache(targetPath)
	}
	status.MarkStorageUpdated()
	return size, nil
}

func (s *dockerStore) DeleteBlob(state *core.AppState, repository, digest string) error {
	path := s.blobPath(repository, digest)
	if IsS3Enabled(path) {
		if err := DeleteFromS3(utils.GetS3Key(path)); err != nil && !isS3NotFound(err) {
			return err
		}
	} else {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if state != nil && state.Inner != nil && state.Inner.FileIndex != nil {
		state.Inner.FileIndex.RemoveFile(path)
		state.InvalidateFileCache(path)
	}
	status.MarkStorageUpdated()
	return nil
}

func (s *dockerStore) OpenManifest(repository, imageName, digest string) ([]byte, bool, error) {
	path := s.manifestPath(repository, imageName, digest)
	if IsS3Enabled(path) {
		reader, _, err := DownloadFromS3(utils.GetS3Key(path))
		if err != nil {
			if isS3NotFound(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, false, err
		}
		return data, true, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

func (s *dockerStore) PutManifest(state *core.AppState, repository, imageName, digest string, data []byte) error {
	path := s.manifestPath(repository, imageName, digest)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	tempPath := path + ".tmp." + uuid.NewString()
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	if IsS3Enabled(path) {
		if err := UploadToS3(tempPath, utils.GetS3Key(path)); err != nil {
			_ = os.Remove(tempPath)
			return err
		}
		_ = os.Remove(tempPath)
	} else {
		if err := utils.SafeRename(tempPath, path); err != nil {
			_ = os.Remove(tempPath)
			return err
		}
	}

	if state != nil && state.Inner != nil && state.Inner.FileIndex != nil {
		state.Inner.FileIndex.EnsureParentDirs(path)
		state.Inner.FileIndex.InsertFile(path, index.FileInfo{Size: int64(len(data)), ModTime: osTimeNowNano()})
		state.InvalidateFileCache(path)
	}
	status.MarkStorageUpdated()
	return nil
}

func (s *dockerStore) DeleteManifest(state *core.AppState, repository, imageName, digest string) error {
	path := s.manifestPath(repository, imageName, digest)
	if IsS3Enabled(path) {
		if err := DeleteFromS3(utils.GetS3Key(path)); err != nil && !isS3NotFound(err) {
			return err
		}
	} else {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if state != nil && state.Inner != nil && state.Inner.FileIndex != nil {
		state.Inner.FileIndex.RemoveFile(path)
		state.InvalidateFileCache(path)
	}
	status.MarkStorageUpdated()
	return nil
}

func osTimeNowNano() int64 {
	return 0 // handled by FileIndex ModTime if available
}
