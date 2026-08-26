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
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"renop/internal/core"
	"renop/internal/service/gpg"
	"renop/internal/service/index"
	"renop/internal/service/repositorygate"
	"renop/internal/utils"
)

var errGPGReleaseInactive = errors.New("GPG release is no longer active")

// NotifyGPGReleaseWorker asks the per-process publisher to inspect the durable
// queue. The buffered signal coalesces bursts without blocking upload requests.
func NotifyGPGReleaseWorker(state *core.AppState) {
	if state == nil || state.Inner == nil || state.Inner.GPGReleaseWake == nil {
		return
	}
	select {
	case state.Inner.GPGReleaseWake <- struct{}{}:
	default:
	}
}

// RestoreGPGReleaseState resets interrupted validation jobs and restores the
// database-backed blacklist before file watchers or index rebuilds begin.
func RestoreGPGReleaseState(state *core.AppState) error {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil {
		return errors.New("application state unavailable")
	}
	db := state.GetDB()
	if db == nil {
		return core.ErrDatabaseUnavailable
	}
	if err := db.ResetValidatingGPGReleases(); err != nil {
		return err
	}
	releases, err := db.ListPendingGPGReleases()
	if err != nil {
		return err
	}
	for _, release := range releases {
		if release == nil {
			continue
		}
		if err := validateReleaseStagingPaths(state, release); err != nil {
			return fmt.Errorf("invalid GPG release %s: %w", release.ID, err)
		}
		if release.Status == core.GPGReleaseSuccess {
			continue
		}
		blockReleasePaths(state, release)
	}
	return cleanupOrphanGPGQuarantine(state, releases)
}

// StartGPGReleaseWorker launches exactly one single-threaded publisher for the
// application state. All cryptographic checks and final storage mutations run
// serially in this goroutine.
func StartGPGReleaseWorker(state *core.AppState) {
	if state == nil || state.Inner == nil || !state.Inner.GPGReleaseWorkerActive.CompareAndSwap(false, true) {
		return
	}
	go func() {
		processGPGReleaseQueue(state)
		for range state.Inner.GPGReleaseWake {
			processGPGReleaseQueue(state)
		}
	}()
}

func processGPGReleaseQueue(state *core.AppState) {
	db := state.GetDB()
	if db == nil {
		return
	}
	pending, err := db.ListPendingGPGReleases()
	if err != nil {
		log.Printf("Failed to inspect GPG release queue: %v", err)
		return
	}
	now := time.Now()
	for _, release := range pending {
		if release == nil {
			continue
		}
		if release.CleanupPending {
			gpgReleaseStorageMutation.Lock()
			cleanupErr := cleanupTerminalGPGRelease(state, release)
			gpgReleaseStorageMutation.Unlock()
			if cleanupErr != nil {
				log.Printf("Failed to clean GPG release %s: %v", release.ID, cleanupErr)
			}
			continue
		}
		if release.Status != core.GPGReleaseQueued || now.Sub(time.UnixMilli(release.CreatedAt)) < gpgPendingTTL {
			continue
		}
		if failErr := expireGPGReleaseIfIncomplete(state, release, now); failErr != nil {
			log.Printf("Failed to expire GPG release %s: %v", release.ID, failErr)
		}
	}

	for {
		gpgReleaseStorageMutation.Lock()
		release, claimErr := db.ClaimNextGPGRelease(time.Now().Add(-gpgOptionalSignatureGrace).UnixMilli())
		gpgReleaseStorageMutation.Unlock()
		if claimErr != nil {
			log.Printf("Failed to claim GPG release: %v", claimErr)
			return
		}
		if release == nil {
			return
		}
		if publishErr := publishGPGRelease(state, release); publishErr != nil {
			if !errors.Is(publishErr, errGPGReleaseInactive) {
				reason := gpgReleaseFailureReason(publishErr)
				if failErr := failClaimedGPGRelease(state, release, reason); failErr != nil {
					log.Printf("Failed to mark GPG release %s failed after %v: %v", release.ID, publishErr, failErr)
				}
			}
		}
	}
}

func gpgReleaseReadyForValidation(release *core.GPGRelease) bool {
	if release == nil {
		return false
	}
	readyPair := release.ArtifactStagingPath != "" && release.SignatureStagingPath != ""
	readyStandaloneSignature := release.SignatureStagingPath != "" && release.ArtifactStagingPath == "" && release.ArtifactExisted
	readyUnsigned := release.ArtifactStagingPath != "" && release.SignatureStagingPath == "" && !release.RequireSignature
	return readyPair || readyStandaloneSignature || readyUnsigned
}

func expireGPGReleaseIfIncomplete(state *core.AppState, observed *core.GPGRelease, now time.Time) error {
	if state == nil || observed == nil || observed.ActiveKey == "" {
		return nil
	}
	gpgReleaseStorageMutation.Lock()
	defer gpgReleaseStorageMutation.Unlock()
	db := state.GetDB()
	if db == nil {
		return core.ErrDatabaseUnavailable
	}
	current, err := db.GetActiveGPGRelease(observed.ActiveKey)
	if err != nil {
		return err
	}
	if current == nil || current.ID != observed.ID || current.Status != core.GPGReleaseQueued ||
		now.Sub(time.UnixMilli(current.CreatedAt)) < gpgPendingTTL || gpgReleaseReadyForValidation(current) {
		return nil
	}
	reason := "Detached GPG signature was not uploaded before the publication deadline"
	if current.ArtifactStagingPath == "" {
		reason = "Maven artifact was not uploaded before the publication deadline"
	}
	return failGPGRelease(state, current, reason)
}

func gpgReleaseFailureReason(err error) string {
	switch {
	case errors.Is(err, gpg.ErrSignatureInvalid):
		return "The detached GPG signature is invalid"
	case errors.Is(err, gpg.ErrSigningKeyUnregistered):
		return "The signing key is not registered for the uploader"
	case errors.Is(err, ErrRedeploymentDenied):
		return "Artifact redeployment is not allowed"
	case errors.Is(err, ErrGPGRepositoryMissing):
		return "The target repository no longer exists"
	case errors.Is(err, os.ErrNotExist):
		return "The quarantined artifact or signature is no longer available"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "GPG validation timed out"
	default:
		return "Publication failed during GPG validation or storage commit"
	}
}

func readQuarantinedSignature(path string, expectedSize int64) ([]byte, error) {
	if expectedSize < 0 || expectedSize > gpg.MaxDetachedSignatureSize {
		return nil, ErrGPGSignatureLarge
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, gpg.MaxDetachedSignatureSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > gpg.MaxDetachedSignatureSize {
		return nil, ErrGPGSignatureLarge
	}
	return data, nil
}

func openReleaseArtifact(release *core.GPGRelease, localArtifactPath string) (io.ReadCloser, error) {
	if release.ArtifactStagingPath != "" {
		return os.Open(release.ArtifactStagingPath)
	}
	return openStoredArtifact(localArtifactPath)
}

func openStoredArtifact(localFilePath string) (io.ReadCloser, error) {
	if IsS3Enabled(localFilePath) {
		reader, _, err := DownloadFromS3(utils.GetS3Key(localFilePath))
		return reader, err
	}
	return os.Open(localFilePath)
}

func verifyGPGRelease(state *core.AppState, release *core.GPGRelease, localArtifactPath string) (*core.GPGSignature, error) {
	if release.SignatureStagingPath == "" {
		if release.RequireSignature {
			return nil, gpg.ErrSignatureInvalid
		}
		return nil, nil
	}
	signatureBytes, err := readQuarantinedSignature(release.SignatureStagingPath, release.SignatureSize)
	if err != nil {
		return nil, err
	}
	artifactReader, err := openReleaseArtifact(release, localArtifactPath)
	if err != nil {
		return nil, err
	}
	record, verifyErr := gpg.VerifyDetached(
		context.Background(), state, release.Uploader, artifactReader, signatureBytes,
		release.Repository, release.ArtifactPath,
	)
	closeErr := artifactReader.Close()
	if verifyErr != nil {
		return nil, verifyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return record, nil
}

func validateQuarantinePath(state *core.AppState, path string) error {
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return errors.New("configuration unavailable")
	}
	root := filepath.Join(cfg.StoragePath, gpgQuarantineDirName)
	if path == "" || !utils.IsSubPath(root, path) {
		return errors.New("invalid GPG quarantine file path")
	}
	return nil
}

func releaseQuarantineDirectory(state *core.AppState, release *core.GPGRelease) (string, error) {
	if state == nil || state.Inner == nil || release == nil {
		return "", errors.New("application state unavailable")
	}
	parsedID, err := uuid.Parse(release.ID)
	if err != nil || !strings.EqualFold(parsedID.String(), release.ID) {
		return "", errors.New("invalid GPG release ID")
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return "", errors.New("configuration unavailable")
	}
	root := filepath.Join(cfg.StoragePath, gpgQuarantineDirName)
	dir := filepath.Join(root, release.ID)
	if !utils.IsSubPath(root, dir) {
		return "", errors.New("invalid GPG quarantine directory")
	}
	return dir, nil
}

func validateReleaseStagingPaths(state *core.AppState, release *core.GPGRelease) error {
	dir, err := releaseQuarantineDirectory(state, release)
	if err != nil {
		return err
	}
	for _, stagingPath := range []string{release.ArtifactStagingPath, release.SignatureStagingPath} {
		if stagingPath == "" {
			continue
		}
		if err := validateQuarantinePath(state, stagingPath); err != nil {
			return err
		}
		if filepath.Clean(filepath.Dir(stagingPath)) != filepath.Clean(dir) {
			return errors.New("GPG staging file does not belong to its release")
		}
	}
	return nil
}

func cleanupOrphanGPGQuarantine(state *core.AppState, releases []*core.GPGRelease) error {
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return errors.New("configuration unavailable")
	}
	root := filepath.Join(cfg.StoragePath, gpgQuarantineDirName)
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	referenced := make(map[string]struct{}, len(releases))
	for _, release := range releases {
		if release == nil {
			continue
		}
		dir, dirErr := releaseQuarantineDirectory(state, release)
		if dirErr != nil {
			return dirErr
		}
		referenced[filepath.Clean(dir)] = struct{}{}
	}
	for _, entry := range entries {
		entryPath := filepath.Join(root, entry.Name())
		if _, keep := referenced[filepath.Clean(entryPath)]; keep {
			continue
		}
		if !utils.IsSubPath(root, entryPath) {
			return errors.New("invalid GPG quarantine entry")
		}
		if err := utils.RemoveAll(entryPath); err != nil {
			return err
		}
	}
	return nil
}

func cloneQuarantinedFile(state *core.AppState, source, destination string) (string, error) {
	if err := validateQuarantinePath(state, source); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return "", err
	}
	publishTemp := destination + ".tmp.gpg-publish." + uuid.NewString()
	src, err := os.Open(source)
	if err != nil {
		return "", err
	}
	dst, err := os.OpenFile(publishTemp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		_ = src.Close()
		return "", err
	}
	bufPtr := bufferPool128k.Get()
	_, copyErr := io.CopyBuffer(dst, src, *bufPtr)
	bufferPool128k.Put(bufPtr)
	closeDstErr := dst.Close()
	closeSrcErr := src.Close()
	if copyErr != nil || closeDstErr != nil || closeSrcErr != nil {
		_ = os.Remove(publishTemp)
		return "", errors.Join(copyErr, closeDstErr, closeSrcErr)
	}
	return publishTemp, nil
}

func releaseArtifactDigests(release *core.GPGRelease) *ContentDigests {
	if release == nil || !release.ArtifactGenerateChecksums {
		return nil
	}
	return &ContentDigests{
		MD5: release.ArtifactMD5, SHA1: release.ArtifactSHA1,
		SHA256: release.ArtifactSHA256, SHA512: release.ArtifactSHA512,
	}
}

func releaseSignatureDigests(release *core.GPGRelease) *ContentDigests {
	if release == nil || !release.SignatureGenerateChecksums {
		return nil
	}
	return &ContentDigests{
		MD5: release.SignatureMD5, SHA1: release.SignatureSHA1,
		SHA256: release.SignatureSHA256, SHA512: release.SignatureSHA512,
	}
}

func ensureReleaseDigests(release *core.GPGRelease) error {
	if release.ArtifactGenerateChecksums && release.ArtifactStagingPath != "" && release.ArtifactSHA512 == "" {
		digests, _, err := HashFile(release.ArtifactStagingPath)
		if err != nil {
			return err
		}
		release.ArtifactMD5, release.ArtifactSHA1 = digests.MD5, digests.SHA1
		release.ArtifactSHA256, release.ArtifactSHA512 = digests.SHA256, digests.SHA512
	}
	if release.SignatureGenerateChecksums && release.SignatureStagingPath != "" && release.SignatureSHA512 == "" {
		digests, _, err := HashFile(release.SignatureStagingPath)
		if err != nil {
			return err
		}
		release.SignatureMD5, release.SignatureSHA1 = digests.MD5, digests.SHA1
		release.SignatureSHA256, release.SignatureSHA512 = digests.SHA256, digests.SHA512
	}
	return nil
}

type stagedGPGCompanion struct {
	stagingPath string
	targetPath  string
}

type stagedGPGCompanionPart struct {
	stagingName string
	targetPath  string
}

func stagedGPGCompanions(state *core.AppState, release *core.GPGRelease, localArtifactPath string) ([]stagedGPGCompanion, error) {
	dir, err := releaseQuarantineDirectory(state, release)
	if err != nil {
		return nil, err
	}
	companions := make([]stagedGPGCompanion, 0, len(gpgChecksumSuffixes)*2)
	for _, suffix := range gpgChecksumSuffixes {
		for _, part := range []stagedGPGCompanionPart{
			{stagingName: "artifact" + suffix, targetPath: localArtifactPath + suffix},
			{stagingName: "signature" + suffix, targetPath: localArtifactPath + ".asc" + suffix},
		} {
			stagingPath := filepath.Join(dir, part.stagingName)
			info, statErr := os.Stat(stagingPath)
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			if statErr != nil {
				return nil, statErr
			}
			if info.IsDir() {
				return nil, errors.New("invalid GPG companion staging file")
			}
			companions = append(companions, stagedGPGCompanion{
				stagingPath: stagingPath,
				targetPath:  part.targetPath,
			})
		}
	}
	return companions, nil
}

func publishStagedGPGCompanions(state *core.AppState, release *core.GPGRelease, localArtifactPath string) error {
	companions, err := stagedGPGCompanions(state, release, localArtifactPath)
	if err != nil {
		return err
	}
	for _, companion := range companions {
		info, err := os.Stat(companion.stagingPath)
		if err != nil {
			return err
		}
		if err := publishPreparedReleaseFile(
			state, companion.targetPath, companion.stagingPath,
			info.Size(), info.ModTime().UnixNano(), PathExistsForUpload(state, companion.targetPath), false, nil,
		); err != nil {
			return err
		}
	}
	return nil
}

func removeArtifactChecksums(state *core.AppState, localArtifactPath string) error {
	var result error
	for _, suffix := range []string{".md5", ".sha1", ".sha256", ".sha512"} {
		result = errors.Join(result, deleteIndexedFile(state, localArtifactPath+suffix))
	}
	return result
}

func publishPreparedReleaseFile(state *core.AppState, localPath, stagingPath string, size, modTime int64, existed, generateChecksums bool, digests *ContentDigests) error {
	publishTemp, err := cloneQuarantinedFile(state, stagingPath, localPath)
	if err != nil {
		return err
	}
	if err := CommitUploadedFile(state, localPath, publishTemp, size, modTime, existed, generateChecksums, digests); err != nil {
		_ = os.Remove(publishTemp)
		return err
	}
	return nil
}

func publishGPGRelease(state *core.AppState, release *core.GPGRelease) error {
	if state == nil || release == nil {
		return errors.New("invalid GPG release")
	}
	releaseMutation := repositorygate.AcquireMutation(release.Repository)
	defer releaseMutation()
	if err := validateReleaseStagingPaths(state, release); err != nil {
		return err
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return errors.New("configuration unavailable")
	}
	repo := cfg.Maven.Repositories[release.Repository]
	if repo == nil {
		return ErrGPGRepositoryMissing
	}
	localArtifactPath, err := releaseLocalArtifactPath(state, release)
	if err != nil {
		return err
	}
	record, err := verifyGPGRelease(state, release, localArtifactPath)
	if err != nil {
		return err
	}
	if release.ArtifactStagingPath != "" && release.ArtifactExisted && !repo.AllowRedeployment {
		return ErrRedeploymentDenied
	}
	if err := ensureReleaseDigests(release); err != nil {
		return err
	}

	gpgReleaseStorageMutation.Lock()
	defer gpgReleaseStorageMutation.Unlock()
	db := state.GetDB()
	if db == nil {
		return core.ErrDatabaseUnavailable
	}
	current, err := db.GetActiveGPGRelease(release.ActiveKey)
	if err != nil {
		return err
	}
	if current == nil || current.ID != release.ID || current.Status != core.GPGReleaseValidating {
		if current != nil && current.ID == release.ID && current.CleanupPending {
			return errors.Join(errGPGReleaseInactive, cleanupTerminalGPGRelease(state, current))
		}
		return errGPGReleaseInactive
	}
	cfg = state.Inner.Config.Load()
	if cfg == nil {
		return ErrGPGRepositoryMissing
	}
	repo = cfg.Maven.Repositories[current.Repository]
	if repo == nil {
		return ErrGPGRepositoryMissing
	}
	if current.ArtifactStagingPath != "" && current.ArtifactExisted && !repo.AllowRedeployment {
		return ErrRedeploymentDenied
	}
	currentLocalArtifactPath, err := releaseLocalArtifactPath(state, current)
	if err != nil {
		return err
	}
	if filepath.Clean(currentLocalArtifactPath) != filepath.Clean(localArtifactPath) {
		return errors.New("storage path changed during GPG validation")
	}
	current.ArtifactMD5, current.ArtifactSHA1 = release.ArtifactMD5, release.ArtifactSHA1
	current.ArtifactSHA256, current.ArtifactSHA512 = release.ArtifactSHA256, release.ArtifactSHA512
	current.SignatureMD5, current.SignatureSHA1 = release.SignatureMD5, release.SignatureSHA1
	current.SignatureSHA256, current.SignatureSHA512 = release.SignatureSHA256, release.SignatureSHA512
	release = current

	release.PublishStarted = true
	release.UpdatedAt = time.Now().UnixMilli()
	if err := db.SaveGPGRelease(release); err != nil {
		return err
	}
	if release.ArtifactStagingPath != "" {
		if err := removeArtifactChecksums(state, localArtifactPath); err != nil {
			return err
		}
		if err := removeArtifactSignature(state, release.Repository, release.ArtifactPath, localArtifactPath); err != nil {
			return err
		}
	}
	if release.ArtifactStagingPath != "" {
		if err := publishPreparedReleaseFile(
			state, localArtifactPath, release.ArtifactStagingPath,
			release.ArtifactSize, release.ArtifactModTime, release.ArtifactExisted,
			release.ArtifactGenerateChecksums, releaseArtifactDigests(release),
		); err != nil {
			return err
		}
	}
	if release.SignatureStagingPath != "" {
		if release.ArtifactStagingPath == "" {
			if err := removeArtifactSignature(state, release.Repository, release.ArtifactPath, localArtifactPath); err != nil {
				return err
			}
		}
		if err := publishPreparedReleaseFile(
			state, localArtifactPath+".asc", release.SignatureStagingPath,
			release.SignatureSize, release.SignatureModTime, release.SignatureExisted,
			release.SignatureGenerateChecksums, releaseSignatureDigests(release),
		); err != nil {
			return err
		}
	}
	if err := publishStagedGPGCompanions(state, release, localArtifactPath); err != nil {
		return err
	}
	if record != nil {
		if err := db.SaveGPGSignature(record); err != nil {
			return err
		}
	}
	if repo.NormalizedFormat() == "maven" && MavenPublicationRecorder != nil {
		if err := MavenPublicationRecorder(state, release.Repository, release.ArtifactPath, release.Uploader,
			release.ArtifactSize, release.ArtifactModTime); err != nil {
			log.Printf("failed to update Maven catalog for verified publication %s: %v", release.ArtifactPath, err)
		}
	}

	now := time.Now().UnixMilli()
	release.Status = core.GPGReleaseSuccess
	release.ActiveKey = ""
	release.FailureReason = ""
	release.CompletedAt = now
	release.UpdatedAt = now
	release.CleanupPending = true
	if err := db.SaveGPGRelease(release); err != nil {
		return err
	}
	unblockReleasePaths(state, release)
	reindexReleasePaths(state, release)
	return cleanupTerminalGPGRelease(state, release)
}

func failClaimedGPGRelease(state *core.AppState, claimed *core.GPGRelease, reason string) error {
	if state == nil || claimed == nil {
		return nil
	}
	gpgReleaseStorageMutation.Lock()
	defer gpgReleaseStorageMutation.Unlock()
	db := state.GetDB()
	if db == nil {
		return core.ErrDatabaseUnavailable
	}
	current, err := db.GetActiveGPGRelease(claimed.ActiveKey)
	if err != nil {
		return err
	}
	if current == nil || current.ID != claimed.ID {
		return nil
	}
	if current.Status != core.GPGReleaseValidating {
		if current.CleanupPending {
			return cleanupTerminalGPGRelease(state, current)
		}
		return nil
	}
	return failGPGRelease(state, current, reason)
}

func failGPGRelease(state *core.AppState, release *core.GPGRelease, reason string) error {
	if release == nil {
		return nil
	}
	now := time.Now().UnixMilli()
	release.Status = core.GPGReleaseFailed
	release.FailureReason = strings.TrimSpace(reason)
	release.CompletedAt = now
	release.UpdatedAt = now
	release.CleanupPending = true
	if err := state.GetDB().SaveGPGRelease(release); err != nil {
		return err
	}
	return cleanupTerminalGPGRelease(state, release)
}

func removeReleaseStagingFiles(state *core.AppState, release *core.GPGRelease) error {
	if err := validateReleaseStagingPaths(state, release); err != nil {
		return err
	}
	dir, err := releaseQuarantineDirectory(state, release)
	if err != nil {
		return err
	}
	return utils.RemoveAll(dir)
}

func cleanupPublishedReleaseFiles(state *core.AppState, release *core.GPGRelease) error {
	localArtifactPath, err := releaseLocalArtifactPath(state, release)
	if err != nil {
		return err
	}
	var result error
	if release.ArtifactStagingPath != "" {
		result = errors.Join(result, deleteIndexedFile(state, localArtifactPath))
		result = errors.Join(result, removeArtifactChecksums(state, localArtifactPath))
		result = errors.Join(result, removeArtifactSignature(state, release.Repository, release.ArtifactPath, localArtifactPath))
	} else if release.SignatureStagingPath != "" {
		result = errors.Join(result, removeArtifactSignature(state, release.Repository, release.ArtifactPath, localArtifactPath))
	}
	return result
}

func cleanupTerminalGPGRelease(state *core.AppState, release *core.GPGRelease) error {
	if release == nil || !release.CleanupPending {
		return nil
	}
	if release.Status == core.GPGReleaseFailed && release.PublishStarted {
		if err := cleanupPublishedReleaseFiles(state, release); err != nil {
			return err
		}
	}
	if err := removeReleaseStagingFiles(state, release); err != nil {
		return err
	}
	release.ActiveKey = ""
	release.CleanupPending = false
	release.UpdatedAt = time.Now().UnixMilli()
	if err := state.GetDB().SaveGPGRelease(release); err != nil {
		return err
	}
	unblockReleasePaths(state, release)
	if release.Status == core.GPGReleaseFailed && !release.PublishStarted {
		reindexReleasePaths(state, release)
	}
	return nil
}

func reindexPathIfPresent(state *core.AppState, path string) {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil || state.Inner.FileIndex.IsBlocked(path) {
		return
	}
	var (
		info index.FileInfo
		err  error
	)
	if IsS3Enabled(path) {
		info, err = StatS3(utils.GetS3Key(path))
	} else {
		var stat os.FileInfo
		stat, err = os.Stat(path)
		if err == nil && !stat.IsDir() {
			info = index.FileInfo{Size: stat.Size(), ModTime: stat.ModTime().UnixNano()}
		}
	}
	if err != nil {
		return
	}
	state.Inner.FileIndex.EnsureParentDirs(path)
	state.Inner.FileIndex.InsertFile(path, info)
}

func reindexReleasePaths(state *core.AppState, release *core.GPGRelease) {
	for _, path := range releaseBlockedPaths(state, release) {
		reindexPathIfPresent(state, path)
	}
}
