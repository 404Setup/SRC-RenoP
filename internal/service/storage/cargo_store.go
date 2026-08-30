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
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"

	"renop/internal/core"
	"renop/internal/service/cargo"
	"renop/internal/service/index"
	"renop/internal/service/npm"
	"renop/internal/service/packagestore"
	"renop/internal/service/proxy"
	"renop/internal/service/status"
	"renop/internal/utils"
)

// packageStore adapts package protocols to the existing atomic storage pipeline.
type packageStore struct{}

type packageStagedFile struct {
	targetPath string
	tempPath   string
	file       *os.File
	closed     bool
	committed  bool
}

var _ packagestore.Store = packageStore{}
var _ packagestore.StagedFile = (*packageStagedFile)(nil)

var cargoHandler = cargo.Handler{Store: packageStore{}, UpstreamIndexExists: proxy.UpstreamArtifactExists}
var npmHandler = npm.Handler{Store: packageStore{}}

// NewPackageStore returns the shared Disk/S3 package persistence adapter.
func NewPackageStore() packagestore.Store {
	return packageStore{}
}

func (packageStore) Open(path string) (io.ReadCloser, bool, error) {
	var reader io.ReadCloser
	var err error
	if IsS3Enabled(path) {
		reader, _, err = DownloadFromS3(utils.GetS3Key(path))
	} else {
		reader, err = os.Open(path)
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || isS3NotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return reader, true, nil
}

func (packageStore) Exists(path string) (bool, error) {
	if IsS3Enabled(path) {
		_, err := StatS3(utils.GetS3Key(path))
		if err == nil {
			return true, nil
		}
		if isS3NotFound(err) {
			return false, nil
		}
		return false, err
	}
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (packageStore) Stage(path string) (packagestore.StagedFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	tempPath := path + ".tmp.package." + uuid.NewString()
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &packageStagedFile{targetPath: path, tempPath: tempPath, file: file}, nil
}

func (staged *packageStagedFile) Write(data []byte) (int, error) {
	if staged == nil || staged.file == nil || staged.closed {
		return 0, fs.ErrClosed
	}
	return staged.file.Write(data)
}

func (staged *packageStagedFile) Close() error {
	if staged == nil || staged.file == nil || staged.closed {
		return nil
	}
	staged.closed = true
	return staged.file.Close()
}

func (staged *packageStagedFile) Open() (io.ReadCloser, error) {
	if staged == nil || staged.tempPath == "" || staged.committed {
		return nil, fs.ErrNotExist
	}
	if err := staged.Close(); err != nil {
		return nil, err
	}
	return os.Open(staged.tempPath)
}

func (staged *packageStagedFile) Size() (int64, error) {
	if staged == nil || staged.tempPath == "" {
		return 0, fs.ErrNotExist
	}
	if err := staged.Close(); err != nil {
		return 0, err
	}
	info, err := os.Stat(staged.tempPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (staged *packageStagedFile) Commit(state *core.AppState) error {
	if staged == nil || staged.tempPath == "" || staged.targetPath == "" || staged.committed {
		return errors.New("package staged file is unavailable")
	}
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil {
		return errors.New("storage index is unavailable")
	}
	if err := staged.Close(); err != nil {
		return err
	}
	info, err := os.Stat(staged.tempPath)
	if err != nil {
		return err
	}
	if IsS3Enabled(staged.targetPath) {
		if err := UploadToS3(staged.tempPath, utils.GetS3Key(staged.targetPath)); err != nil {
			return err
		}
		if removeErr := os.Remove(staged.tempPath); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			log.Printf("Failed to remove committed package staging file %s: %v", staged.tempPath, removeErr)
		}
	} else if err := utils.SafeRename(staged.tempPath, staged.targetPath); err != nil {
		return err
	}
	staged.committed = true
	state.Inner.FileIndex.EnsureParentDirs(staged.targetPath)
	state.Inner.FileIndex.InsertFile(staged.targetPath, index.FileInfo{Size: info.Size(), ModTime: info.ModTime().UnixNano()})
	state.InvalidateFileCache(staged.targetPath)
	status.MarkStorageUpdated()
	return nil
}

func (staged *packageStagedFile) Discard() error {
	if staged == nil {
		return nil
	}
	closeErr := staged.Close()
	if staged.committed || staged.tempPath == "" {
		return closeErr
	}
	removeErr := os.Remove(staged.tempPath)
	if errors.Is(removeErr, fs.ErrNotExist) {
		removeErr = nil
	}
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func (packageStore) Delete(state *core.AppState, path string) error {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil {
		return errors.New("storage index is unavailable")
	}
	if IsS3Enabled(path) {
		if err := DeleteFromS3(utils.GetS3Key(path)); err != nil && !isS3NotFound(err) {
			return err
		}
	} else if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	state.Inner.FileIndex.RemoveFile(path)
	state.InvalidateFileCache(path)
	status.MarkStorageUpdated()
	return nil
}

func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}
	response := minio.ToErrorResponse(err)
	return response.StatusCode == 404 || response.Code == "NoSuchKey" || response.Code == "NoSuchObject"
}
