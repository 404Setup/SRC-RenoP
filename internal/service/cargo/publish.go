/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"log"
	"sync"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/publicationquota"
	"renop/internal/service/status"
	"renop/internal/utils"
)

const (
	maxCrateSize    = 128 << 20
	maxMetadataSize = 1 << 20
)

var (
	publishSlots   = make(chan struct{}, 4)
	publishBuffers = sync.Pool{New: func() any {
		buffer := make([]byte, 128<<10)
		return &buffer
	}}
)

func (h Handler) publish(c fiber.Ctx, state *core.AppState, repo *config.Repository, storagePath string) error {
	user, err := authenticatedUser(c)
	if err != nil || user.IsManager() {
		return cargoError(c, core.ErrCargoPermissionDenied)
	}
	db := state.GetDB()
	if db == nil {
		return cargoError(c, core.ErrDatabaseUnavailable)
	}
	select {
	case publishSlots <- struct{}{}:
		defer func() { <-publishSlots }()
	default:
		c.Set(fiber.HeaderRetryAfter, "1")
		return errorResponse(c, fiber.StatusServiceUnavailable, "Cargo publish capacity is temporarily exhausted")
	}

	reader := publishBodyReader(c)
	metadata, crateLength, err := readPublishHeader(reader, int64(c.Request().Header.ContentLength()))
	if err != nil {
		return errorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if err := validatePackage(metadata.Name, metadata.Version); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if err := validatePublishMetadata(&metadata); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	dependencies, err := indexDependencies(metadata.Deps)
	if err != nil {
		return errorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if !status.CanAllocateDiskSpace(state, uint64(crateLength)+(1<<20)) {
		return errorResponse(c, fiber.StatusInsufficientStorage, "Insufficient disk space to publish crate")
	}

	normalizedName := normalizeCrateName(metadata.Name)
	lockPath := cargoPackageLockPath(storagePath, repo, normalizedName)
	if !utils.IsSubPath(storagePath, lockPath) {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo package path")
	}
	release, err := acquireIndexLock(state, lockPath)
	if err != nil {
		return errorResponse(c, fiber.StatusServiceUnavailable, "Cargo registry state is unavailable")
	}
	succeeded := false
	defer func() { release(succeeded) }()

	// Re-read ownership only after acquiring the package-index lock. This
	// prevents an in-flight publish from restoring a package that was archived,
	// deleted, or removed from the publisher's team while it waited.
	packageRecord, err := db.GetCargoPackage(repo.Name, normalizedName)
	if err != nil {
		return cargoError(c, err)
	}
	packageName := metadata.Name
	if packageRecord == nil {
		reservedName, reserved, reservationErr := pendingPublicationReservation(
			state, repo.Name, normalizedName, user.Username)
		if reservationErr != nil {
			return cargoError(c, reservationErr)
		}
		if reserved {
			if reservedName != metadata.Name {
				return errorResponse(c, fiber.StatusConflict, "Cargo crate name collides with a pending package")
			}
			packageName = reservedName
		}
		if !auth.CurrentCredentialHasScopeTarget(c, core.APITokenScopePackageCreate, repo.Name) &&
			!auth.CurrentCredentialHasScope(c, core.APITokenScopePackageManage) {
			return cargoError(c, core.ErrCargoPermissionDenied)
		}
		if !user.CheckUpdatePermission(repo.Name) {
			return cargoError(c, core.ErrCargoPermissionDenied)
		}
	} else {
		details, err := db.GetCargoPackageDetails(repo.Name, normalizedName, user.Username)
		if err != nil {
			return cargoError(c, err)
		}
		if details == nil || details.Package == nil || details.Package.PermissionLevel < core.CargoPermissionPublish {
			return cargoError(c, core.ErrCargoPermissionDenied)
		}
		if details.Package.Archived {
			return cargoError(c, core.ErrCargoPackageArchived)
		}
		if details.Package.Name != metadata.Name {
			return errorResponse(c, fiber.StatusConflict, "Cargo crate name collides with an existing package")
		}
		packageName = details.Package.Name
	}
	indexFilePath := cargoIndexPath(storagePath, repo, packageName)
	if packageRecord == nil {
		indexExists, existsErr := h.Store.Exists(indexFilePath)
		if existsErr != nil {
			return errorResponse(c, fiber.StatusInternalServerError, "Failed to inspect Cargo index")
		}
		if indexExists {
			return errorResponse(c, fiber.StatusConflict, "A mirrored or unmanaged crate already uses this name")
		}
		if len(repo.Mirrors) > 0 {
			if h.UpstreamIndexExists == nil {
				return errorResponse(c, fiber.StatusServiceUnavailable, "Cargo upstream name verification is unavailable")
			}
			probeCtx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
			upstreamExists, probeErr := h.UpstreamIndexExists(probeCtx, state, repo, indexPath(packageName))
			cancel()
			if probeErr != nil {
				return errorResponse(c, fiber.StatusServiceUnavailable, "Failed to verify the Cargo crate name against upstream mirrors")
			}
			if upstreamExists {
				return errorResponse(c, fiber.StatusConflict, "An upstream crate already uses this name")
			}
		}
	}
	cratePath, crateRelativePath, validCratePath := cargoReviewCratePath(
		storagePath, repo, packageName, metadata.Version)
	if !validCratePath || !utils.IsSubPath(storagePath, indexFilePath) {
		return errorResponse(c, fiber.StatusBadRequest, "Invalid Cargo package path")
	}

	crateExists, err := h.Store.Exists(cratePath)
	if err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to inspect Cargo storage")
	}
	if crateExists {
		return errorResponse(c, fiber.StatusConflict, errVersionExists.Error())
	}

	crateStage, err := h.Store.Stage(cratePath)
	if err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to prepare Cargo storage")
	}
	defer crateStage.Discard()
	digest := sha256.New()
	if err := streamCrate(reader, crateStage, digest, crateLength); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	archiveReader, err := crateStage.Open()
	if err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to validate Cargo crate")
	}
	manifest, archiveErr := validateArchive(archiveReader, metadata.Name, metadata.Version)
	closeErr := archiveReader.Close()
	if archiveErr != nil {
		return errorResponse(c, fiber.StatusBadRequest, archiveErr.Error())
	}
	if closeErr != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to validate Cargo crate")
	}
	readme := ""
	if manifest != nil {
		readme = manifest.readmeContent
		readmePath, readmeEnabled := cargoReadmePath(manifest.Readme)
		if readme == "" && readmeEnabled && readmePath != "" && defaultCargoReadmeRank(readmePath) < 0 {
			readmeReader, openErr := crateStage.Open()
			if openErr != nil {
				return errorResponse(c, fiber.StatusInternalServerError, "Failed to read Cargo crate README")
			}
			readme, openErr = readDeclaredCargoReadme(readmeReader, metadata.Name, metadata.Version, readmePath)
			readmeCloseErr := readmeReader.Close()
			if openErr != nil || readmeCloseErr != nil {
				return errorResponse(c, fiber.StatusBadRequest, "Cargo crate README could not be read")
			}
		}
	}

	license := ""
	homepageURL := ""
	repoURL := ""
	docURL := ""
	rustVersion := ""
	if metadata.RustVersion != nil {
		rustVersion = *metadata.RustVersion
	}
	if manifest != nil {
		if manifest.License != "" {
			license = manifest.License
		} else if manifest.LicenseFile != "" {
			license = manifest.LicenseFile
		}
		if manifest.Documentation != "" {
			docURL = manifest.Documentation
		}
		if manifest.Homepage != "" {
			homepageURL = manifest.Homepage
		}
		if manifest.Repository != "" {
			repoURL = manifest.Repository
		}
		if rustVersion == "" && manifest.RustVersion != "" {
			rustVersion = manifest.RustVersion
		}
	}
	var rustVerPtr *string
	if rustVersion != "" {
		rustVerPtr = &rustVersion
	}

	entry := IndexEntry{
		Name: packageName, Version: metadata.Version, Deps: dependencies,
		Checksum: hex.EncodeToString(digest.Sum(nil)), Features: metadata.Features,
		Yanked: false, Links: metadata.Links, Features2: metadata.Features2,
		RustVersion: rustVerPtr,
	}
	if entry.Features == nil {
		entry.Features = map[string][]string{}
	}
	if len(entry.Features2) > 0 {
		entry.Schema = 2
	}

	indexReader, indexExisted, err := h.Store.Open(indexFilePath)
	if err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to read Cargo index")
	}
	if packageRecord == nil && indexExisted {
		_ = indexReader.Close()
		return errorResponse(c, fiber.StatusConflict, "A mirrored or unmanaged crate already uses this name")
	}
	indexStage, err := h.Store.Stage(indexFilePath)
	if err != nil {
		if indexReader != nil {
			_ = indexReader.Close()
		}
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to prepare Cargo index")
	}
	defer indexStage.Discard()
	rewriteErr := rewriteIndex(indexReader, indexStage, entry)
	if indexReader != nil {
		if err := indexReader.Close(); rewriteErr == nil {
			rewriteErr = err
		}
	}
	if errors.Is(rewriteErr, errVersionExists) {
		return errorResponse(c, fiber.StatusConflict, rewriteErr.Error())
	}
	if rewriteErr != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to update Cargo index")
	}
	if err := indexStage.Close(); err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to stage Cargo index")
	}

	now := time.Now().UnixMilli()
	packageMetadata := &core.CargoPackage{
		Repository: repo.Name, Name: packageName, NormalizedName: normalizedName,
		Description: metadata.Description, Readme: readme, RepositoryURL: repoURL, Homepage: homepageURL,
		Documentation: docURL, CreatedAt: now, UpdatedAt: now,
	}
	versionMetadata := &core.CargoVersion{
		Repository: repo.Name, Package: normalizedName, Version: metadata.Version,
		Description: metadata.Description, Publisher: user.Username, Size: crateLength,
		Checksum: entry.Checksum, RustVersion: rustVersion, License: license,
		Documentation: docURL, Homepage: homepageURL, RepositoryURL: repoURL,
		CreatedAt: now,
	}
	reviewPolicy := repo.PublicationReviewPolicy()
	publishedVersions := false
	reviewRequired := reviewPolicy == config.PublicationReviewEveryVersion
	if reviewPolicy == config.PublicationReviewNewPackages {
		publishedVersions, err = db.CargoHasPublishedVersions(repo.Name, normalizedName)
		if err != nil {
			return cargoError(c, err)
		}
		reviewRequired = !publishedVersions
	}
	if reviewRequired {
		state.Inner.FileIndex.BlockFile(cratePath)
		state.InvalidateFileCache(cratePath)
	}
	quotaTeamPrefix := ""
	if packageRecord != nil {
		quotaTeamPrefix = packageRecord.SuperTeamPrefix
	}
	quota, err := publicationquota.Reserve(state, user.Username, quotaTeamPrefix, core.PublicationQuotaDelta{
		Files: 1, Bytes: crateLength, Publications: 1,
	})
	if err != nil {
		if reviewRequired {
			state.Inner.FileIndex.UnblockFile(cratePath)
			state.InvalidateFileCache(cratePath)
		}
		c.Set("X-Renop-Error-Code", publicationquota.ErrorCode(err))
		return errorResponse(c, fiber.StatusTooManyRequests, "Cargo publication quota exceeded")
	}
	defer quota.Release()
	if err := quota.Commit(); err != nil {
		if reviewRequired {
			state.Inner.FileIndex.UnblockFile(cratePath)
			state.InvalidateFileCache(cratePath)
		}
		return errorResponse(c, fiber.StatusServiceUnavailable, "Cargo publication quota is unavailable")
	}

	if err := crateStage.Commit(state); err != nil {
		if reviewRequired {
			state.Inner.FileIndex.UnblockFile(cratePath)
			state.InvalidateFileCache(cratePath)
		}
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to store crate")
	}
	if reviewRequired {
		review, reviewErr := QueuePublicationReview(state, repo, packageMetadata, versionMetadata,
			entry, crateRelativePath, publishedVersions)
		if reviewErr != nil || review == nil || !review.Pending {
			if cleanupErr := h.Store.Delete(state, cratePath); cleanupErr != nil {
				state.Inner.FailuresCount.Add(1)
			}
			state.Inner.FileIndex.UnblockFile(cratePath)
			state.InvalidateFileCache(cratePath)
			return errorResponse(c, fiber.StatusInternalServerError, "Failed to create Cargo publication review")
		}
		succeeded = true
		c.Set("X-RenoP-Review-ID", review.TaskID)
		logCargoAudit(c, state, audit.ActionUploadQueuedReview,
			"Repository: "+repo.Name+", crate: "+packageName+", version: "+metadata.Version)
		c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
		return c.Status(fiber.StatusAccepted).JSON(PublishResponse{
			Warnings: Warnings{InvalidCategories: []string{}, InvalidBadges: []string{}, Other: []string{}},
		})
	}
	if err := indexStage.Commit(state); err != nil {
		if cleanupErr := h.Store.Delete(state, cratePath); cleanupErr != nil {
			log.Printf("failed to clean Cargo crate after index commit failure %s/%s@%s: %v", repo.Name, packageName, metadata.Version, cleanupErr)
		}
		return errorResponse(c, fiber.StatusInternalServerError, "Failed to store Cargo index")
	}
	if err := db.RecordCargoPublication(packageMetadata, versionMetadata, user.Username); err != nil {
		if rollbackErr := h.rollbackPublication(state, indexFilePath, cratePath, metadata.Version, indexExisted); rollbackErr != nil {
			log.Printf("failed to roll back Cargo publication %s/%s@%s: %v", repo.Name, packageName, metadata.Version, rollbackErr)
		}
		return cargoError(c, err)
	}
	succeeded = true
	logCargoAudit(c, state, audit.ActionCargoPublish, "Repository: "+repo.Name+", crate: "+packageName+", version: "+metadata.Version)
	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.JSON(PublishResponse{
		Warnings: Warnings{InvalidCategories: []string{}, InvalidBadges: []string{}, Other: []string{}},
	})
}

func (h Handler) rollbackPublication(state *core.AppState, indexPath, cratePath, version string, indexExisted bool) error {
	var rollbackErr error
	if err := h.Store.Delete(state, cratePath); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	return errors.Join(rollbackErr, rollbackCargoIndexVersion(state, h.Store, indexPath, version, indexExisted))
}

func rollbackCargoIndexVersion(state *core.AppState, store Store, indexPath, version string, indexExisted bool) error {
	if !indexExisted {
		return store.Delete(state, indexPath)
	}
	reader, found, err := store.Open(indexPath)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("Cargo index is missing during publication rollback")
	}
	stage, err := store.Stage(indexPath)
	if err != nil {
		_ = reader.Close()
		return err
	}
	defer stage.Discard()
	removed, _, rewriteErr := rewriteRemoveVersion(reader, stage, version)
	closeErr := reader.Close()
	if rewriteErr != nil || closeErr != nil {
		return errors.Join(rewriteErr, closeErr)
	}
	if !removed {
		return errors.New("Cargo version is missing during publication rollback")
	}
	if err := stage.Close(); err != nil {
		return err
	}
	return stage.Commit(state)
}

func publishBodyReader(c fiber.Ctx) io.Reader {
	if stream := c.Request().BodyStream(); stream != nil {
		return stream
	}
	return bytes.NewReader(c.Request().Body())
}

func readPublishHeader(reader io.Reader, contentLength int64) (PublishMetadata, int64, error) {
	var metadata PublishMetadata
	var lengthBuffer [4]byte
	if _, err := io.ReadFull(reader, lengthBuffer[:]); err != nil {
		return metadata, 0, errors.New("invalid Cargo publish body")
	}
	metadataLength := int64(binary.LittleEndian.Uint32(lengthBuffer[:]))
	if metadataLength <= 0 || metadataLength > maxMetadataSize {
		return metadata, 0, errors.New("invalid Cargo publish metadata length")
	}
	metadataReader := &io.LimitedReader{R: reader, N: metadataLength}
	decoder := json.NewDecoder(metadataReader)
	if err := decoder.Decode(&metadata); err != nil {
		return metadata, 0, errors.New("invalid Cargo publish metadata")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) || metadataReader.N != 0 {
		return metadata, 0, errors.New("invalid Cargo publish metadata")
	}
	if _, err := io.ReadFull(reader, lengthBuffer[:]); err != nil {
		return metadata, 0, errors.New("invalid Cargo publish body")
	}
	crateLength := int64(binary.LittleEndian.Uint32(lengthBuffer[:]))
	if crateLength <= 0 || crateLength > maxCrateSize {
		return metadata, 0, errors.New("cargo crate exceeds the size limit")
	}
	if expected := metadataLength + crateLength + 8; contentLength > 0 && contentLength != expected {
		return metadata, 0, errors.New("invalid Cargo publish body length")
	}
	return metadata, crateLength, nil
}

func streamCrate(reader io.Reader, destination io.WriteCloser, digest hash.Hash, crateLength int64) error {
	limited := &io.LimitedReader{R: reader, N: crateLength}
	bufferPointer := publishBuffers.Get().(*[]byte)
	_, copyErr := io.CopyBuffer(io.MultiWriter(destination, digest), limited, *bufferPointer)
	publishBuffers.Put(bufferPointer)
	if copyErr != nil || limited.N != 0 {
		_ = destination.Close()
		return errors.New("invalid Cargo crate length")
	}
	if err := destination.Close(); err != nil {
		return errors.New("failed to stage Cargo crate")
	}
	var trailing [1]byte
	if n, err := io.ReadFull(reader, trailing[:]); err == nil || n != 0 {
		return errors.New("invalid trailing Cargo publish data")
	} else if !errors.Is(err, io.EOF) {
		return errors.New("invalid Cargo publish body")
	}
	return nil
}
