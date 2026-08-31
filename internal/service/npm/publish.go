/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
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
	maxPublishBodySize       = 96 << 20
	maxTarballSize           = 64 << 20
	maxUnpackedTarballSize   = 512 << 20
	maxTarballFiles          = 100_000
	maxPackageManifestSize   = 2 << 20
	maxStoredManifestJSON    = 4 << 20
	maxPublishAttachmentData = ((maxTarballSize + 2) / 3) * 4
	tarTypeRegularAlt        = byte(0)
)

var (
	publishSlots = make(chan struct{}, 2)
	copyBuffers  = sync.Pool{New: func() any {
		buffer := make([]byte, 128<<10)
		return &buffer
	}}
)

func publishBodyReader(c fiber.Ctx) io.Reader {
	if stream := c.Request().BodyStream(); stream != nil {
		return stream
	}
	return bytes.NewReader(c.Request().Body())
}

func decodePublishDocument(c fiber.Ctx) (*publishDocument, error) {
	if length := int64(c.Request().Header.ContentLength()); length > maxPublishBodySize {
		return nil, fiber.ErrRequestEntityTooLarge
	}
	reader := &io.LimitedReader{R: publishBodyReader(c), N: maxPublishBodySize + 1}
	decoder := json.NewDecoder(reader)
	document := &publishDocument{}
	if err := decoder.Decode(document); err != nil {
		return nil, errors.New("invalid npm publish document")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("npm publish document must contain one JSON value")
	}
	if reader.N <= 0 {
		return nil, fiber.ErrRequestEntityTooLarge
	}
	return document, nil
}

func publishVersion(document *publishDocument, packageName string) (string, map[string]any, error) {
	if document == nil || len(document.Versions) != 1 {
		return "", nil, errors.New("npm publish must contain exactly one version")
	}
	for version, raw := range document.Versions {
		if !validNPMVersion(version) || len(raw) == 0 || len(raw) > maxStoredManifestJSON {
			return "", nil, errors.New("npm package version is invalid")
		}
		manifest := make(map[string]any)
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return "", nil, errors.New("npm version manifest is invalid")
		}
		manifestName, _ := manifest["name"].(string)
		manifestVersion, _ := manifest["version"].(string)
		normalizedName, valid := NormalizePackageName(manifestName)
		if !valid || normalizedName != packageName || strings.TrimSpace(manifestVersion) != version {
			return "", nil, errors.New("npm version manifest does not match the package path")
		}
		return version, manifest, nil
	}
	return "", nil, errors.New("npm package version is missing")
}

func tarballAttachment(document *publishDocument) (publishAttachment, error) {
	if document == nil {
		return publishAttachment{}, errors.New("npm tarball attachment is missing")
	}
	var selected publishAttachment
	found := false
	for name, attachment := range document.Attachments {
		if !strings.HasSuffix(strings.ToLower(name), ".tgz") {
			continue
		}
		if found {
			return publishAttachment{}, errors.New("npm publish contains multiple tarballs")
		}
		if attachment.Length <= 0 || attachment.Length > maxTarballSize ||
			len(attachment.Data) == 0 || len(attachment.Data) > maxPublishAttachmentData+8 {
			return publishAttachment{}, errors.New("npm tarball exceeds the size limit")
		}
		selected = attachment
		found = true
	}
	if !found {
		return publishAttachment{}, errors.New("npm tarball attachment is missing")
	}
	return selected, nil
}

func stageAttachment(staged StagedFile, attachment publishAttachment) (int64, string, string, error) {
	sha1Digest := sha1.New()
	sha512Digest := sha512.New()
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(attachment.Data))
	limited := &io.LimitedReader{R: decoder, N: maxTarballSize + 1}
	buffer := copyBuffers.Get().(*[]byte)
	written, copyErr := io.CopyBuffer(io.MultiWriter(staged, sha1Digest, sha512Digest), limited, *buffer)
	copyBuffers.Put(buffer)
	if copyErr != nil || limited.N <= 0 || written != attachment.Length {
		_ = staged.Close()
		return 0, "", "", errors.New("npm tarball attachment is invalid")
	}
	if err := staged.Close(); err != nil {
		return 0, "", "", errors.New("failed to stage npm tarball")
	}
	return written, hex.EncodeToString(sha1Digest.Sum(nil)),
		"sha512-" + base64.StdEncoding.EncodeToString(sha512Digest.Sum(nil)), nil
}

func validateTarball(staged StagedFile, packageName, version string) error {
	reader, err := staged.Open()
	if err != nil {
		return errors.New("failed to validate npm tarball")
	}
	defer reader.Close()
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return errors.New("npm tarball is not valid gzip data")
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	fileCount := 0
	var unpackedSize int64
	manifestFound := false
	for {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return errors.New("npm tarball is invalid")
		}
		fileCount++
		if fileCount > maxTarballFiles || header.Size < 0 || header.Size > maxUnpackedTarballSize-unpackedSize {
			return errors.New("npm tarball exceeds unpacked limits")
		}
		unpackedSize += header.Size
		cleanName := path.Clean(strings.ReplaceAll(header.Name, `\`, "/"))
		if cleanName == "." || strings.HasPrefix(cleanName, "/") || cleanName == ".." ||
			strings.HasPrefix(cleanName, "../") || cleanName != "package" && !strings.HasPrefix(cleanName, "package/") {
			return errors.New("npm tarball contains an unsafe path")
		}
		if cleanName != "package/package.json" || header.Typeflag != tar.TypeReg && header.Typeflag != tarTypeRegularAlt {
			continue
		}
		if header.Size > maxPackageManifestSize {
			return errors.New("npm package.json exceeds the size limit")
		}
		contents, readErr := io.ReadAll(io.LimitReader(tarReader, maxPackageManifestSize+1))
		if readErr != nil || int64(len(contents)) != header.Size {
			return errors.New("npm package.json is invalid")
		}
		var manifest struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal(contents, &manifest); err != nil {
			return errors.New("npm package.json is invalid")
		}
		normalizedName, valid := NormalizePackageName(manifest.Name)
		if !valid || normalizedName != packageName || manifest.Version != version {
			return errors.New("npm tarball package.json does not match the published package")
		}
		manifestFound = true
	}
	if !manifestFound {
		return errors.New("npm tarball does not contain package/package.json")
	}
	return nil
}

func manifestDescription(manifest map[string]any, fallback string) string {
	if description, ok := manifest["description"].(string); ok {
		return strings.TrimSpace(description)
	}
	return strings.TrimSpace(fallback)
}

func manifestDeprecated(manifest map[string]any) string {
	deprecated, _ := manifest["deprecated"].(string)
	return strings.TrimSpace(deprecated)
}

func setCanonicalDist(manifest map[string]any, baseURL, repository, packageName, version,
	tarballPath, shasum, integrity, publisher string, size int64) {
	dist, _ := manifest["dist"].(map[string]any)
	if dist == nil {
		dist = make(map[string]any)
	}
	dist["tarball"] = strings.TrimRight(baseURL, "/") + "/" + repository + "/" + tarballPath
	dist["shasum"] = shasum
	dist["integrity"] = integrity
	dist["size"] = size
	manifest["dist"] = dist
	manifest["_id"] = packageName + "@" + version
	manifest["_npmUser"] = map[string]any{"name": publisher}
}

func packageLockPath(storagePath string, repo *config.Repository, packageName string) string {
	return filepath.Join(storagePath, repo.Name, ".renop.tmp.npm", filepath.FromSlash(packageName))
}

func acquirePackageLock(state *core.AppState, lockPath string) (func(bool), error) {
	if state == nil || state.Inner == nil || state.Inner.InFlightDownloads == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	key := filepath.ToSlash(lockPath)
	publication := state.Inner.InFlightDownloads.AcquirePath(key)
	return func(succeeded bool) {
		state.Inner.InFlightDownloads.UnlockPath(key, publication, succeeded)
	}, nil
}

func logNPMAudit(c fiber.Ctx, state *core.AppState, action, details string) {
	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, AuthMethod: authMethod, SessionID: sessionID,
		IP: ip, Action: action, Details: details, CreatedAt: time.Now().UnixMilli(),
	})
}

func publish(c fiber.Ctx, state *core.AppState, repo *config.Repository, store Store,
	storagePath, packageName string) error {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return npmError(c, fiber.StatusUnauthorized, "authentication required", "npm publish requires authentication")
	}
	if !canWriteRepository(user, repo) {
		return npmError(c, fiber.StatusForbidden, "forbidden", "npm repository publish permission is required")
	}
	if state.GetDB() == nil {
		return npmError(c, fiber.StatusServiceUnavailable, "database unavailable", "npm metadata is unavailable")
	}
	exists, _, publishEnabled, member, level, err := state.GetDB().GetNPMPackageAccess(repo.Name, packageName, user.Username)
	if err != nil {
		return npmError(c, fiber.StatusServiceUnavailable, "database unavailable", "npm metadata is unavailable")
	}
	if !exists {
		return npmError(c, fiber.StatusNotFound, "package not reserved", "create the npm package in RenoP before publishing")
	}
	if !member || level < core.NPMPermissionPublish {
		return npmError(c, fiber.StatusForbidden, "forbidden", "npm package publish permission is required")
	}
	if !publishEnabled {
		return npmError(c, fiber.StatusForbidden, "package is pull-only", "mirrored npm packages cannot be published locally")
	}
	select {
	case publishSlots <- struct{}{}:
		defer func() { <-publishSlots }()
	default:
		c.Set(fiber.HeaderRetryAfter, "1")
		return npmError(c, fiber.StatusServiceUnavailable, "publish capacity exhausted", "retry npm publish shortly")
	}
	document, err := decodePublishDocument(c)
	if err != nil {
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) {
			return npmError(c, fiber.StatusRequestEntityTooLarge, "publish body too large", "npm publish exceeds the server limit")
		}
		return npmError(c, fiber.StatusBadRequest, "invalid publish document", err.Error())
	}
	documentName, valid := NormalizePackageName(document.Name)
	if !valid || documentName != packageName || document.ID != "" && strings.ToLower(document.ID) != packageName {
		return npmError(c, fiber.StatusBadRequest, "invalid package name", "npm publish name does not match the request path")
	}
	if len(document.Attachments) == 0 {
		return updatePackument(c, state, repo, packageName, document, 0)
	}
	if !auth.CurrentCredentialHasScopeTarget(c, core.APITokenScopeRepositoryPublish, repo.Name) {
		return npmError(c, fiber.StatusForbidden, "forbidden", "API token repository publish scope is required")
	}
	version, manifest, err := publishVersion(document, packageName)
	if err != nil {
		return npmError(c, fiber.StatusBadRequest, "invalid package version", err.Error())
	}
	for tag, taggedVersion := range document.DistTags {
		if !validNPMTag(tag) || taggedVersion != version {
			return npmError(c, fiber.StatusBadRequest, "invalid dist-tag", "npm publish dist-tags must target the published version")
		}
	}
	if len(document.DistTags) == 0 {
		document.DistTags = map[string]string{"latest": version}
	}
	attachment, err := tarballAttachment(document)
	if err != nil {
		return npmError(c, fiber.StatusBadRequest, "invalid tarball", err.Error())
	}
	if !status.CanAllocateDiskSpace(state, uint64(attachment.Length)+(1<<20)) {
		return npmError(c, fiber.StatusInsufficientStorage, "insufficient storage", "not enough storage for npm package")
	}
	lockPath := packageLockPath(storagePath, repo, packageName)
	if !utils.IsSubPath(storagePath, lockPath) {
		return npmError(c, fiber.StatusBadRequest, "invalid package path", "npm package path is invalid")
	}
	release, err := acquirePackageLock(state, lockPath)
	if err != nil {
		return npmError(c, fiber.StatusServiceUnavailable, "registry unavailable", "npm registry state is unavailable")
	}
	succeeded := false
	defer func() { release(succeeded) }()
	exists, _, publishEnabled, member, level, err = state.GetDB().GetNPMPackageAccess(repo.Name, packageName, user.Username)
	if err != nil || !exists || !publishEnabled || !member || level < core.NPMPermissionPublish {
		return npmError(c, fiber.StatusForbidden, "forbidden", "npm package publication state changed")
	}
	tarballPath := canonicalTarballPath(packageName, version)
	targetPath := filepath.Join(storagePath, repo.Name, filepath.FromSlash(tarballPath))
	if !utils.IsSubPath(storagePath, targetPath) {
		return npmError(c, fiber.StatusBadRequest, "invalid tarball path", "npm tarball path is invalid")
	}
	if exists, inspectErr := store.Exists(targetPath); inspectErr != nil {
		return npmError(c, fiber.StatusInternalServerError, "storage failure", "failed to inspect npm tarball storage")
	} else if exists {
		return npmError(c, fiber.StatusConflict, "version already exists", "npm package versions are immutable")
	}
	staged, err := store.Stage(targetPath)
	if err != nil {
		return npmError(c, fiber.StatusInternalServerError, "storage failure", "failed to stage npm tarball")
	}
	defer staged.Discard()
	size, shasum, integrity, err := stageAttachment(staged, attachment)
	if err != nil {
		return npmError(c, fiber.StatusBadRequest, "invalid tarball", err.Error())
	}
	if err := validateTarball(staged, packageName, version); err != nil {
		return npmError(c, fiber.StatusBadRequest, "invalid tarball", err.Error())
	}
	setCanonicalDist(manifest, "", repo.Name, packageName, version, tarballPath,
		shasum, integrity, user.Username, size)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil || len(manifestJSON) > maxStoredManifestJSON {
		return npmError(c, fiber.StatusBadRequest, "manifest too large", "npm version metadata exceeds the server limit")
	}
	now := time.Now().UnixMilli()
	packageMetadata := &core.NPMPackage{
		Repository: repo.Name, Name: packageName, Description: manifestDescription(manifest, document.Description),
		UpdatedAt: now,
	}
	versionMetadata := &core.NPMVersion{
		Repository: repo.Name, Package: packageName, Version: version, ManifestJSON: string(manifestJSON),
		Publisher: user.Username, TarballPath: tarballPath, Shasum: shasum, Integrity: integrity,
		Size: size, Deprecated: manifestDeprecated(manifest), CreatedAt: now,
	}
	reviewPolicy := repo.PublicationReviewPolicy()
	reviewRequired := reviewPolicy == config.PublicationReviewEveryVersion
	if reviewRequired {
		state.Inner.FileIndex.BlockFile(targetPath)
		state.InvalidateFileCache(targetPath)
	}
	quotaPackage, err := state.GetDB().GetNPMPackage(repo.Name, packageName)
	if err != nil || quotaPackage == nil {
		if reviewRequired {
			state.Inner.FileIndex.UnblockFile(targetPath)
			state.InvalidateFileCache(targetPath)
		}
		return npmError(c, fiber.StatusServiceUnavailable, "database unavailable", "npm metadata is unavailable")
	}
	quota, err := publicationquota.Reserve(state, user.Username, quotaPackage.SuperTeamPrefix,
		core.PublicationQuotaDelta{Files: 1, Bytes: size, Publications: 1})
	if err != nil {
		if reviewRequired {
			state.Inner.FileIndex.UnblockFile(targetPath)
			state.InvalidateFileCache(targetPath)
		}
		c.Set("X-Renop-Error-Code", publicationquota.ErrorCode(err))
		return npmError(c, fiber.StatusTooManyRequests, publicationquota.ErrorCode(err), "npm publication quota exceeded")
	}
	defer quota.Release()
	if err := quota.Commit(); err != nil {
		if reviewRequired {
			state.Inner.FileIndex.UnblockFile(targetPath)
			state.InvalidateFileCache(targetPath)
		}
		return npmError(c, fiber.StatusServiceUnavailable, "publication quota unavailable", "npm publication quota is unavailable")
	}
	if err := staged.Commit(state); err != nil {
		if reviewRequired {
			state.Inner.FileIndex.UnblockFile(targetPath)
			state.InvalidateFileCache(targetPath)
		}
		return npmError(c, fiber.StatusInternalServerError, "storage failure", "failed to store npm tarball")
	}
	if reviewRequired {
		review, reviewErr := QueuePublicationReview(state, repo, packageMetadata, versionMetadata,
			document.DistTags, true)
		if reviewErr != nil || review == nil || !review.Pending {
			if cleanupErr := store.Delete(state, targetPath); cleanupErr != nil {
				state.Inner.FailuresCount.Add(1)
			}
			state.Inner.FileIndex.UnblockFile(targetPath)
			state.InvalidateFileCache(targetPath)
			return npmError(c, fiber.StatusInternalServerError, "review failure", "failed to create npm publication review")
		}
		succeeded = true
		c.Set("X-RenoP-Review-ID", review.TaskID)
		logNPMAudit(c, state, audit.ActionUploadQueuedReview,
			fmt.Sprintf("Repository: %s, package: %s, version: %s", repo.Name, packageName, version))
		return c.Status(fiber.StatusAccepted).JSON(operationResponse{OK: true, ID: packageName})
	}
	if err := state.GetDB().RecordNPMPublication(packageMetadata, versionMetadata,
		document.DistTags, user.Username); err != nil {
		if cleanupErr := store.Delete(state, targetPath); cleanupErr != nil {
			state.Inner.FailuresCount.Add(1)
		}
		switch {
		case errors.Is(err, core.ErrNPMVersionExists):
			return npmError(c, fiber.StatusConflict, "version already exists", "npm package versions are immutable")
		case errors.Is(err, core.ErrNPMPermissionDenied):
			return npmError(c, fiber.StatusForbidden, "forbidden", "npm package publish permission is required")
		case errors.Is(err, core.ErrNPMPackageArchived), errors.Is(err, core.ErrNPMPackageMirrored):
			return npmError(c, fiber.StatusConflict, "package unavailable", err.Error())
		case errors.Is(err, core.ErrNPMPackageLimit):
			return npmError(c, fiber.StatusRequestEntityTooLarge, "package metadata limit", "npm package metadata limit reached")
		default:
			return npmError(c, fiber.StatusInternalServerError, "metadata failure", "failed to record npm package metadata")
		}
	}
	succeeded = true
	logNPMAudit(c, state, audit.ActionNPMPublish,
		fmt.Sprintf("Repository: %s, package: %s, version: %s", repo.Name, packageName, version))
	return c.Status(fiber.StatusCreated).JSON(operationResponse{OK: true, ID: packageName})
}
