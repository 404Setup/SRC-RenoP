/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/gpg"
	"renop/internal/utils"
)

const (
	gpgPendingTTL             = 15 * time.Minute
	gpgOptionalSignatureGrace = 5 * time.Second
	maxGPGPendingReleases     = 256
	maxGPGPendingPerUser      = 16
	gpgQuarantineDirName      = ".renop.tmp.gpg"
)

var (
	ErrGPGPendingConflict     = errors.New("another GPG release is already pending for this artifact")
	ErrGPGPendingLimit        = errors.New("too many GPG releases are awaiting publication")
	ErrGPGSignatureLarge      = errors.New("GPG detached signature exceeds the size limit")
	ErrGPGSignatureSuffix     = errors.New("GPG detached signatures must use the lowercase .asc suffix")
	ErrGPGUploaderRequired    = errors.New("authenticated uploader is required for GPG verification")
	ErrGPGRepositoryMissing   = errors.New("repository is no longer available")
	ErrRedeploymentDenied     = errors.New("artifact redeployment is not allowed")
	ErrGPGStoragePathChange   = errors.New("storage path cannot be changed while GPG publications are pending")
	gpgReleaseStorageMutation sync.Mutex
)

var gpgChecksumSuffixes = [...]string{".md5", ".sha1", ".sha256", ".sha512"}

type PreparedUpload struct {
	LocalFilePath     string
	TempPath          string
	Username          string
	FileSize          int64
	ModTime           int64
	Existed           bool
	GenerateChecksums bool
	SignatureExpected bool
	Digests           *ContentDigests
}

type GPGUploadResult struct {
	Pending   bool
	ReleaseID string
}

// GPGUploadLockPath makes a protected artifact, its detached signature, and
// their checksum companions share one in-flight path lock.
func GPGUploadLockPath(localFilePath string) string {
	if artifact, ok := gpg.ArtifactForDetachedSignature(filepath.ToSlash(localFilePath)); ok {
		return filepath.FromSlash(artifact)
	}
	if artifact, _, ok := gpgCompanionForPath(localFilePath); ok {
		return filepath.FromSlash(artifact)
	}
	return localFilePath
}

func gpgCompanionForPath(localFilePath string) (artifactPath, stagingName string, ok bool) {
	pathSlash := filepath.ToSlash(localFilePath)
	for _, suffix := range gpgChecksumSuffixes {
		signatureSuffix := ".asc" + suffix
		if strings.HasSuffix(pathSlash, signatureSuffix) {
			artifactPath = strings.TrimSuffix(pathSlash, signatureSuffix)
			if gpg.IsProtectedArtifact(artifactPath) {
				return artifactPath, "signature" + suffix, true
			}
		}
		if strings.HasSuffix(pathSlash, suffix) {
			artifactPath = strings.TrimSuffix(pathSlash, suffix)
			if gpg.IsProtectedArtifact(artifactPath) {
				return artifactPath, "artifact" + suffix, true
			}
		}
	}
	return "", "", false
}

// AcquireGPGArtifactMutation serializes a direct protected-artifact mutation
// with queued publication and deletion, and rejects it while a durable release
// for the same artifact is active. The caller must invoke the returned unlock
// function exactly once.
func AcquireGPGArtifactMutation(state *core.AppState, localFilePath string) (func(), error) {
	gpgReleaseStorageMutation.Lock()
	unlock := func() {
		gpgReleaseStorageMutation.Unlock()
	}
	repository, relPath, err := repositoryArtifactPath(state, localFilePath)
	if err != nil {
		unlock()
		return nil, err
	}
	artifactPath, isSignature := gpg.ArtifactForDetachedSignature(relPath)
	if !isSignature {
		artifactPath = relPath
	}
	db := state.GetDB()
	if db == nil {
		return unlock, nil
	}
	release, err := db.GetActiveGPGRelease(gpg.ArtifactKey(repository, artifactPath))
	if err != nil {
		unlock()
		return nil, err
	}
	if release != nil {
		unlock()
		return nil, ErrGPGPendingConflict
	}
	return unlock, nil
}

// AcquireGPGStoragePathChange prevents a configuration update from moving the
// quarantine root while durable publications or terminal cleanup are pending.
func AcquireGPGStoragePathChange(state *core.AppState) (func(), error) {
	gpgReleaseStorageMutation.Lock()
	unlock := func() {
		gpgReleaseStorageMutation.Unlock()
	}
	if state == nil || state.Inner == nil {
		unlock()
		return nil, errors.New("application state unavailable")
	}
	db := state.GetDB()
	if db == nil {
		return unlock, nil
	}
	releases, err := db.ListPendingGPGReleases()
	if err != nil {
		unlock()
		return nil, err
	}
	if len(releases) != 0 {
		unlock()
		return nil, ErrGPGStoragePathChange
	}
	return unlock, nil
}

func repositoryArtifactPath(state *core.AppState, localFilePath string) (string, string, error) {
	if state == nil || state.Inner == nil {
		return "", "", errors.New("application state unavailable")
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return "", "", errors.New("configuration unavailable")
	}
	rel, err := filepath.Rel(cfg.StoragePath, localFilePath)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", errors.New("upload path is outside storage")
	}
	parts := strings.SplitN(filepath.ToSlash(rel), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("invalid repository artifact path")
	}
	return parts[0], parts[1], nil
}

func releaseLocalArtifactPath(state *core.AppState, release *core.GPGRelease) (string, error) {
	if state == nil || state.Inner == nil || release == nil {
		return "", errors.New("application state unavailable")
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return "", errors.New("configuration unavailable")
	}
	rel, ok := utils.SanitizePath(filepath.ToSlash(release.ArtifactPath))
	if !ok || rel == "" || !utils.IsValidRepositoryName(release.Repository) {
		return "", errors.New("invalid GPG release path")
	}
	target := filepath.Join(cfg.StoragePath, release.Repository, filepath.FromSlash(rel))
	if !utils.IsSubPath(cfg.StoragePath, target) {
		return "", errors.New("GPG release path is outside storage")
	}
	return target, nil
}

func releaseBlockedPaths(state *core.AppState, release *core.GPGRelease) []string {
	artifactPath, err := releaseLocalArtifactPath(state, release)
	if err != nil {
		return nil
	}
	seen := make(map[string]struct{}, 10)
	paths := make([]string, 0, 10)
	appendPath := func(path string) {
		path = filepath.ToSlash(path)
		if _, exists := seen[path]; exists {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	appendWithChecksums := func(path string) {
		appendPath(path)
		for _, suffix := range []string{".md5", ".sha1", ".sha256", ".sha512"} {
			appendPath(path + suffix)
		}
	}
	appendWithChecksums(artifactPath)
	appendWithChecksums(artifactPath + ".asc")
	return paths
}

func blockReleasePaths(state *core.AppState, release *core.GPGRelease) {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil {
		return
	}
	for _, path := range releaseBlockedPaths(state, release) {
		state.Inner.FileIndex.BlockFile(path)
		state.InvalidateFileCache(path)
	}
}

func unblockReleasePaths(state *core.AppState, release *core.GPGRelease) {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil {
		return
	}
	for _, path := range releaseBlockedPaths(state, release) {
		state.Inner.FileIndex.UnblockFile(path)
		state.InvalidateFileCache(path)
	}
}

func releaseDigests(upload *PreparedUpload) (md5Value, sha1Value, sha256Value, sha512Value string) {
	if upload == nil || upload.Digests == nil {
		return "", "", "", ""
	}
	return upload.Digests.MD5, upload.Digests.SHA1, upload.Digests.SHA256, upload.Digests.SHA512
}

func moveUploadToQuarantine(state *core.AppState, releaseID, part string, upload *PreparedUpload) (string, error) {
	cfg := state.Inner.Config.Load()
	if cfg == nil || cfg.StoragePath == "" {
		return "", errors.New("configuration unavailable")
	}
	dir := filepath.Join(cfg.StoragePath, gpgQuarantineDirName, releaseID)
	if !utils.IsSubPath(cfg.StoragePath, dir) {
		return "", errors.New("invalid GPG quarantine path")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create GPG quarantine: %w", err)
	}
	target := filepath.Join(dir, part+"."+uuid.NewString())
	if err := utils.SafeRename(upload.TempPath, target); err != nil {
		return "", fmt.Errorf("failed to quarantine GPG upload: %w", err)
	}
	_ = os.Chmod(target, 0600)
	return target, nil
}

func newGPGRelease(state *core.AppState, repository, artifactPath, username string) (*core.GPGRelease, error) {
	db := state.GetDB()
	if db == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	total, perUser, err := db.CountPendingGPGReleases(username)
	if err != nil {
		return nil, err
	}
	if total >= maxGPGPendingReleases || perUser >= maxGPGPendingPerUser {
		return nil, ErrGPGPendingLimit
	}
	now := time.Now().UnixMilli()
	return &core.GPGRelease{
		ID:           uuid.NewString(),
		ActiveKey:    gpg.ArtifactKey(repository, artifactPath),
		Repository:   repository,
		ArtifactPath: artifactPath,
		Uploader:     strings.ToLower(username),
		Status:       core.GPGReleaseQueued,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func enqueueGPGRelease(state *core.AppState, repo *config.Repository, upload *PreparedUpload, repository, artifactPath string, isSignature bool) (*core.GPGRelease, error) {
	db := state.GetDB()
	if db == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	activeKey := gpg.ArtifactKey(repository, artifactPath)
	release, err := db.GetActiveGPGRelease(activeKey)
	if err != nil {
		return nil, err
	}
	if release != nil {
		if release.Status != core.GPGReleaseQueued || !strings.EqualFold(release.Uploader, upload.Username) {
			return nil, ErrGPGPendingConflict
		}
	} else {
		release, err = newGPGRelease(state, repository, artifactPath, upload.Username)
		if err != nil {
			return nil, err
		}
	}

	oldStagingPath := release.ArtifactStagingPath
	part := "artifact"
	if isSignature {
		oldStagingPath = release.SignatureStagingPath
		part = "signature"
	}
	stagingPath, err := moveUploadToQuarantine(state, release.ID, part, upload)
	if err != nil {
		return nil, err
	}
	originalTempPath := upload.TempPath
	upload.TempPath = ""

	now := time.Now().UnixMilli()
	release.UpdatedAt = now
	release.FailureReason = ""
	if isSignature {
		release.RequireSignature = true
		release.SignatureStagingPath = stagingPath
		release.SignatureSize = upload.FileSize
		release.SignatureModTime = upload.ModTime
		release.SignatureExisted = upload.Existed
		release.SignatureGenerateChecksums = upload.GenerateChecksums
		release.SignatureMD5, release.SignatureSHA1, release.SignatureSHA256, release.SignatureSHA512 = releaseDigests(upload)
		if release.ArtifactStagingPath == "" {
			artifactLocalPath, pathErr := releaseLocalArtifactPath(state, release)
			if pathErr != nil {
				_ = utils.SafeRename(stagingPath, originalTempPath)
				upload.TempPath = originalTempPath
				return nil, pathErr
			}
			release.ArtifactExisted = PathExistsForUpload(state, artifactLocalPath)
		}
	} else {
		release.RequireSignature = release.RequireSignature || repo.RequireGPGSignature || upload.SignatureExpected
		release.ArtifactStagingPath = stagingPath
		release.ArtifactSize = upload.FileSize
		release.ArtifactModTime = upload.ModTime
		release.ArtifactExisted = upload.Existed
		release.ArtifactGenerateChecksums = upload.GenerateChecksums
		release.ArtifactMD5, release.ArtifactSHA1, release.ArtifactSHA256, release.ArtifactSHA512 = releaseDigests(upload)
	}

	if err := db.SaveGPGRelease(release); err != nil {
		if renameErr := utils.SafeRename(stagingPath, originalTempPath); renameErr == nil {
			upload.TempPath = originalTempPath
		}
		return nil, err
	}
	if oldStagingPath != "" && oldStagingPath != stagingPath {
		_ = os.Remove(oldStagingPath)
	}
	blockReleasePaths(state, release)
	NotifyGPGReleaseWorker(state)
	return release, nil
}

func enqueueGPGCompanion(
	state *core.AppState,
	repo *config.Repository,
	upload *PreparedUpload,
	repository, artifactPath, stagingName string,
) (*core.GPGRelease, error) {
	db := state.GetDB()
	if db == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	activeKey := gpg.ArtifactKey(repository, artifactPath)
	release, err := db.GetActiveGPGRelease(activeKey)
	if err != nil {
		return nil, err
	}
	newRelease := release == nil
	if newRelease {
		if !repo.RequireGPGSignature {
			return nil, nil
		}
		artifactLocalPath, pathErr := releaseLocalArtifactPath(state, &core.GPGRelease{
			Repository:   repository,
			ArtifactPath: artifactPath,
		})
		if pathErr != nil {
			return nil, pathErr
		}
		// Maven clients commonly upload checksum companions after the signed
		// artifact has already been published. Only create a placeholder release
		// when no published artifact exists for the companion yet.
		if PathExistsForUpload(state, artifactLocalPath) {
			return nil, nil
		}
		release, err = newGPGRelease(state, repository, artifactPath, upload.Username)
		if err != nil {
			return nil, err
		}
		release.RequireSignature = true
	} else if (release.Status != core.GPGReleaseQueued && release.Status != core.GPGReleaseValidating) ||
		!strings.EqualFold(release.Uploader, upload.Username) {
		return nil, ErrGPGPendingConflict
	}

	dir, err := releaseQuarantineDirectory(state, release)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create GPG quarantine: %w", err)
	}
	target := filepath.Join(dir, stagingName)
	if !utils.IsSubPath(dir, target) {
		return nil, errors.New("invalid GPG companion staging path")
	}
	originalTempPath := upload.TempPath
	if err := utils.SafeRename(originalTempPath, target); err != nil {
		return nil, fmt.Errorf("failed to quarantine GPG companion: %w", err)
	}
	upload.TempPath = ""
	_ = os.Chmod(target, 0600)

	if newRelease {
		if err := db.SaveGPGRelease(release); err != nil {
			if renameErr := utils.SafeRename(target, originalTempPath); renameErr == nil {
				upload.TempPath = originalTempPath
			}
			return nil, err
		}
	}
	blockReleasePaths(state, release)
	NotifyGPGReleaseWorker(state)
	return release, nil
}

// ProcessUploadedFile commits ordinary files immediately and queues protected
// Maven artifacts, signatures, and active publication checksum companions.
func ProcessUploadedFile(ctx context.Context, state *core.AppState, repo *config.Repository, upload *PreparedUpload) (GPGUploadResult, error) {
	if state == nil || state.Inner == nil || repo == nil || upload == nil || upload.TempPath == "" {
		return GPGUploadResult{}, errors.New("invalid prepared upload")
	}
	if err := ctx.Err(); err != nil {
		return GPGUploadResult{}, err
	}
	if repo.NormalizedFormat() == config.RepositoryFormatFiles {
		err := CommitUploadedFile(state, upload.LocalFilePath, upload.TempPath, upload.FileSize, upload.ModTime,
			upload.Existed, false, nil)
		return GPGUploadResult{}, err
	}
	localPathSlash := filepath.ToSlash(upload.LocalFilePath)
	_, localIsSignature := gpg.ArtifactForDetachedSignature(localPathSlash)
	_, _, localIsCompanion := gpgCompanionForPath(localPathSlash)
	if localIsSignature && !strings.HasSuffix(localPathSlash, ".asc") {
		return GPGUploadResult{}, ErrGPGSignatureSuffix
	}
	if !localIsSignature && !localIsCompanion && !gpg.IsProtectedArtifact(localPathSlash) {
		err := CommitUploadedFile(state, upload.LocalFilePath, upload.TempPath, upload.FileSize, upload.ModTime,
			upload.Existed, upload.GenerateChecksums, upload.Digests)
		return GPGUploadResult{}, err
	}
	gpgReleaseStorageMutation.Lock()
	defer gpgReleaseStorageMutation.Unlock()
	repository, relPath, err := repositoryArtifactPath(state, upload.LocalFilePath)
	if err != nil {
		return GPGUploadResult{}, err
	}
	cfg := state.Inner.Config.Load()
	currentRepo, exists := cfg.Maven.Repositories[repository]
	if !exists || currentRepo == nil {
		return GPGUploadResult{}, ErrGPGRepositoryMissing
	}
	repo = currentRepo
	artifactPath, isSignature := gpg.ArtifactForDetachedSignature(relPath)
	companionStagingName := ""
	if companionArtifact, stagingName, isCompanion := gpgCompanionForPath(relPath); isCompanion {
		artifactPath = companionArtifact
		companionStagingName = stagingName
	} else if !isSignature {
		artifactPath = relPath
	}
	if companionStagingName != "" {
		if strings.TrimSpace(upload.Username) == "" {
			return GPGUploadResult{}, ErrGPGUploaderRequired
		}
		release, companionErr := enqueueGPGCompanion(
			state, repo, upload, repository, artifactPath, companionStagingName,
		)
		if companionErr != nil {
			return GPGUploadResult{}, companionErr
		}
		if release == nil {
			err := CommitUploadedFile(state, upload.LocalFilePath, upload.TempPath, upload.FileSize, upload.ModTime,
				upload.Existed, upload.GenerateChecksums, upload.Digests)
			return GPGUploadResult{}, err
		}
		return GPGUploadResult{Pending: true, ReleaseID: release.ID}, nil
	}
	if !isSignature && !repo.RequireGPGSignature && !upload.SignatureExpected {
		db := state.GetDB()
		if db != nil {
			active, activeErr := db.GetActiveGPGRelease(gpg.ArtifactKey(repository, artifactPath))
			if activeErr != nil {
				return GPGUploadResult{}, activeErr
			}
			if active != nil {
				return GPGUploadResult{}, ErrGPGPendingConflict
			}
		}
		if err := RemoveArtifactGPGSignature(state, upload.LocalFilePath); err != nil {
			return GPGUploadResult{}, fmt.Errorf("failed to invalidate prior GPG signature: %w", err)
		}
		err := CommitUploadedFile(state, upload.LocalFilePath, upload.TempPath, upload.FileSize, upload.ModTime,
			upload.Existed, upload.GenerateChecksums, upload.Digests)
		return GPGUploadResult{}, err
	}
	if strings.TrimSpace(upload.Username) == "" {
		return GPGUploadResult{}, ErrGPGUploaderRequired
	}
	if isSignature && upload.FileSize > gpg.MaxDetachedSignatureSize {
		return GPGUploadResult{}, ErrGPGSignatureLarge
	}
	release, err := enqueueGPGRelease(state, repo, upload, repository, artifactPath, isSignature)
	if err != nil {
		return GPGUploadResult{}, err
	}
	return GPGUploadResult{Pending: true, ReleaseID: release.ID}, nil
}

func GPGUploadErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, ErrGPGSignatureLarge):
		return http.StatusRequestEntityTooLarge, "GPG detached signature exceeds the size limit"
	case errors.Is(err, ErrGPGSignatureSuffix):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, ErrGPGPendingConflict), errors.Is(err, ErrRedeploymentDenied):
		return http.StatusConflict, err.Error()
	case errors.Is(err, ErrGPGPendingLimit):
		return http.StatusTooManyRequests, err.Error()
	case errors.Is(err, core.ErrDatabaseUnavailable):
		return http.StatusServiceUnavailable, "GPG publication database is unavailable"
	case errors.Is(err, ErrGPGUploaderRequired):
		return http.StatusUnauthorized, err.Error()
	case errors.Is(err, ErrGPGRepositoryMissing):
		return http.StatusNotFound, err.Error()
	default:
		if strings.HasPrefix(err.Error(), "Failed to upload to S3:") {
			return http.StatusInternalServerError, err.Error()
		}
		return http.StatusInternalServerError, "Internal Server Error"
	}
}

func discardPendingGPGUploads(state *core.AppState, localPrefix, reason string) error {
	if state == nil || state.Inner == nil || localPrefix == "" {
		return nil
	}
	db := state.GetDB()
	if db == nil {
		return core.ErrDatabaseUnavailable
	}
	releases, err := db.ListPendingGPGReleases()
	if err != nil {
		return err
	}
	cleanPrefix := filepath.Clean(localPrefix)
	now := time.Now().UnixMilli()
	for _, release := range releases {
		if release == nil || release.ActiveKey == "" {
			continue
		}
		artifactPath, pathErr := releaseLocalArtifactPath(state, release)
		if pathErr != nil || (filepath.Clean(artifactPath) != cleanPrefix && !utils.IsSubPath(cleanPrefix, artifactPath)) {
			continue
		}
		release.Status = core.GPGReleaseFailed
		release.FailureReason = reason
		release.CleanupPending = true
		release.CompletedAt = now
		release.UpdatedAt = now
		if saveErr := db.SaveGPGRelease(release); saveErr != nil {
			return saveErr
		}
	}
	NotifyGPGReleaseWorker(state)
	return nil
}

func removeArtifactSignature(state *core.AppState, repository, artifactPath, localArtifactPath string) error {
	var result error
	for _, suffix := range []string{".asc", ".asc.md5", ".asc.sha1", ".asc.sha256", ".asc.sha512"} {
		result = errors.Join(result, deleteIndexedFile(state, localArtifactPath+suffix))
	}
	if db := state.GetDB(); db != nil {
		result = errors.Join(result, db.DeleteGPGSignature(gpg.ArtifactKey(repository, artifactPath)))
	}
	return result
}

// RemoveArtifactGPGSignature invalidates any verified signature and detached
// signature companions for a protected artifact before unsigned replacement.
func RemoveArtifactGPGSignature(state *core.AppState, localArtifactPath string) error {
	if !gpg.IsProtectedArtifact(filepath.ToSlash(localArtifactPath)) {
		return nil
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return errors.New("configuration unavailable")
	}
	rel, err := filepath.Rel(cfg.StoragePath, localArtifactPath)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("artifact path is outside storage")
	}
	parts := strings.SplitN(filepath.ToSlash(rel), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.New("invalid artifact path")
	}
	return removeArtifactSignature(state, parts[0], parts[1], localArtifactPath)
}

func gpgIdentityForLocalPath(state *core.AppState, localPath string) (repository, artifactPath string, ok bool) {
	if state == nil || state.Inner == nil {
		return "", "", false
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil || cfg.StoragePath == "" {
		return "", "", false
	}
	rel, err := filepath.Rel(cfg.StoragePath, localPath)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", false
	}
	parts := strings.SplitN(filepath.ToSlash(rel), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	artifactPath = parts[1]
	if mapped, signature := gpg.ArtifactForDetachedSignature(artifactPath); signature {
		artifactPath = mapped
	} else if !gpg.IsProtectedArtifact(artifactPath) {
		return "", "", false
	}
	return parts[0], artifactPath, true
}

func deleteGPGRecordForLocalPath(state *core.AppState, localPath string) error {
	repository, artifactPath, ok := gpgIdentityForLocalPath(state, localPath)
	if !ok {
		return nil
	}
	if db := state.GetDB(); db != nil {
		return db.DeleteGPGSignature(gpg.ArtifactKey(repository, artifactPath))
	}
	return nil
}

func deleteGPGRecordsByLocalPrefix(state *core.AppState, repository, localPrefix string) error {
	if state == nil || repository == "" {
		return nil
	}
	db := state.GetDB()
	if db == nil {
		return nil
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return nil
	}
	repositoryRoot := filepath.Join(cfg.StoragePath, repository)
	rel, err := filepath.Rel(repositoryRoot, localPrefix)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	if rel == "." {
		return db.DeleteGPGSignaturesByRepository(repository)
	}
	return db.DeleteGPGSignaturesByPrefix(repository, filepath.ToSlash(rel))
}
