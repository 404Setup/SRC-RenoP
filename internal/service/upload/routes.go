/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package upload

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/status"
	"renop/internal/service/storage"
	"renop/internal/service/updater"
	"renop/internal/utils"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

// SetupChunkedUploadRoutes registers multi-part upload endpoints under /api/upload/chunked.
//
// Init and complete use application/x-protobuf. Part bodies are raw octets.
// Original single-shot PUT (repo paths) and POST /api/updater/upload remain unchanged.
func SetupChunkedUploadRoutes(router fiber.Router, state *core.AppState) {
	cfg := state.Inner.Config.Load()
	StartBackgroundCleanup(cfg.StoragePath)

	mgr := DefaultManager()
	api := router.Group("/upload/chunked")

	api.Post("/", func(c fiber.Ctx) error {
		return handleInit(c, state, mgr)
	})
	api.Put("/:id/:index", func(c fiber.Ctx) error {
		return handlePutChunk(c, mgr)
	})
	api.Post("/:id/complete", func(c fiber.Ctx) error {
		return handleComplete(c, state, mgr)
	})
	api.Delete("/:id", func(c fiber.Ctx) error {
		return handleAbort(c, mgr)
	})
}

func jsonErr(c fiber.Ctx, status int, msg string) error {
	return c.Status(status).JSON(fiber.Map{"error": msg})
}

func handleInit(c fiber.Ctx, state *core.AppState, mgr *Manager) error {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || user.Username == "guest" {
		return jsonErr(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	var req pb.ChunkedUploadInitRequest
	if err := protohttp.Read(c, &req); err != nil {
		return jsonErr(c, fiber.StatusBadRequest, "Invalid request body")
	}

	purpose := strings.ToLower(strings.TrimSpace(req.GetPurpose()))
	if purpose == "" {
		purpose = PurposeStorage
	}
	if req.GetSize() < 0 {
		return jsonErr(c, fiber.StatusBadRequest, "Invalid size")
	}
	filename := req.GetFilename()
	if filename == "" && purpose == PurposeUpdater {
		return jsonErr(c, fiber.StatusBadRequest, "filename is required")
	}

	chunkSize := NormalizeChunkSize(req.GetSize(), req.GetChunkSize())

	var localFilePath, repoName string
	var generateChecksums bool

	switch purpose {
	case PurposeStorage:
		path := strings.TrimPrefix(filepath.ToSlash(req.GetPath()), "/")
		if path == "" {
			return jsonErr(c, fiber.StatusBadRequest, "path is required for storage uploads")
		}
		parts := strings.SplitN(path, "/", 2)
		repoName = parts[0]
		if !utils.IsValidRepositoryName(repoName) {
			return jsonErr(c, fiber.StatusBadRequest, "Invalid repository")
		}
		cfg := state.Inner.Config.Load()
		repo, exists := cfg.Maven.Repositories[repoName]
		if !exists {
			return jsonErr(c, fiber.StatusNotFound, "Repository not found")
		}
		if !user.CheckUpdatePermission(repoName) {
			return jsonErr(c, fiber.StatusForbidden, "Forbidden")
		}

		rel := ""
		if len(parts) > 1 {
			rel = parts[1]
		}
		if rel == "" {
			return jsonErr(c, fiber.StatusBadRequest, "file path is required")
		}
		sanitized, ok := utils.SanitizePath(rel)
		if !ok {
			return jsonErr(c, fiber.StatusBadRequest, "Invalid path")
		}
		localFilePath = filepath.Join(cfg.StoragePath, repoName, sanitized)
		if !utils.IsSubPath(cfg.StoragePath, localFilePath) {
			return jsonErr(c, fiber.StatusBadRequest, "Invalid path")
		}

		if storage.PathExistsForUpload(state, localFilePath) && !repo.AllowRedeployment {
			return jsonErr(c, fiber.StatusConflict, "Conflict")
		}

		estimated := storage.EstimateUploadDiskSpace(localFilePath, req.GetSize())
		if !status.CanAllocateDiskSpace(state, uint64(estimated)) {
			return jsonErr(c, fiber.StatusInsufficientStorage, "Insufficient disk space to upload file")
		}
		generateChecksums = req.GetGenerateChecksums()
		if filename == "" {
			filename = filepath.Base(sanitized)
		}

	case PurposeUpdater:
		if !user.IsManager() {
			return jsonErr(c, fiber.StatusForbidden, "Forbidden")
		}
		if !strings.HasSuffix(strings.ToLower(filename), ".zip") {
			return jsonErr(c, fiber.StatusBadRequest, "Uploaded file must be a .zip package")
		}
		reqSpace := req.GetSize() * 3
		if reqSpace <= 0 {
			reqSpace = 100 * 1024 * 1024
		}
		if updater.CanAllocateDiskSpace != nil && !updater.CanAllocateDiskSpace(uint64(reqSpace)) {
			return jsonErr(c, fiber.StatusInsufficientStorage, "Insufficient disk space to upload update package")
		}

	default:
		return jsonErr(c, fiber.StatusBadRequest, "Invalid purpose")
	}

	sess, err := mgr.CreateSession(
		purpose,
		filename,
		user.Username,
		req.GetSize(),
		chunkSize,
		generateChecksums,
		localFilePath,
		repoName,
	)
	if err != nil {
		return jsonErr(c, fiber.StatusInternalServerError, err.Error())
	}

	return protohttp.Write(c, &pb.ChunkedUploadInitResponse{
		UploadId:   sess.ID,
		ChunkSize:  sess.ChunkSize,
		ChunkCount: int32(sess.ChunkCount),
		Purpose:    sess.Purpose,
	})
}

func handlePutChunk(c fiber.Ctx, mgr *Manager) error {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || user.Username == "guest" {
		return jsonErr(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	id := c.Params("id")
	indexStr := c.Params("index")
	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 0 {
		return jsonErr(c, fiber.StatusBadRequest, "Invalid chunk index")
	}

	sess, ok := mgr.Get(id)
	if !ok {
		return jsonErr(c, fiber.StatusNotFound, "Upload session not found")
	}
	if !sess.OwnedBy(user.Username) {
		return jsonErr(c, fiber.StatusForbidden, "Forbidden")
	}

	contentLength := int64(c.Request().Header.ContentLength())

	if stream := c.Request().BodyStream(); stream != nil {
		if err := sess.WriteChunk(index, stream, contentLength); err != nil {
			return jsonErr(c, fiber.StatusBadRequest, err.Error())
		}
	} else {
		body := c.Body()
		if contentLength < 0 {
			contentLength = int64(len(body))
		}
		if contentLength == int64(len(body)) {
			if err := sess.WriteChunkBytes(index, body); err != nil {
				return jsonErr(c, fiber.StatusBadRequest, err.Error())
			}
		} else if err := sess.WriteChunk(index, bytes.NewReader(body), contentLength); err != nil {
			return jsonErr(c, fiber.StatusBadRequest, err.Error())
		}
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func handleComplete(c fiber.Ctx, state *core.AppState, mgr *Manager) error {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || user.Username == "guest" {
		return jsonErr(c, fiber.StatusUnauthorized, "Unauthorized")
	}

	sess, err := mgr.BeginSessionCompletion(c.Params("id"), user.Username)
	if err != nil {
		if errors.Is(err, errSessionNotFound) {
			return jsonErr(c, fiber.StatusNotFound, "Upload session not found")
		}
		if errors.Is(err, errSessionNotOwned) {
			return jsonErr(c, fiber.StatusForbidden, "Forbidden")
		}
		if errors.Is(err, errChunksIncomplete) {
			return jsonErr(c, fiber.StatusBadRequest, "Not all chunks received")
		}
		return jsonErr(c, fiber.StatusConflict, "Upload session is already being finalized")
	}

	if err := sess.CloseFile(); err != nil {
		sess.Abort()
		return jsonErr(c, fiber.StatusInternalServerError, "Failed to finalize upload")
	}

	switch sess.Purpose {
	case PurposeStorage:
		return completeStorage(c, state, sess)
	case PurposeUpdater:
		return completeUpdater(c, sess)
	default:
		sess.Abort()
		return jsonErr(c, fiber.StatusBadRequest, "Invalid purpose")
	}
}

func completeStorage(c fiber.Ctx, state *core.AppState, sess *Session) error {
	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[sess.RepoName]
	if !exists {
		sess.Abort()
		return jsonErr(c, fiber.StatusNotFound, "Repository not found")
	}
	if user := auth.GetUser(c); user == nil || !user.CheckUpdatePermission(sess.RepoName) {
		sess.Abort()
		return jsonErr(c, fiber.StatusForbidden, "Forbidden")
	}

	localFilePath := sess.LocalFilePath
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

	existed := storage.PathExistsForUpload(state, localFilePath)
	if existed && !repo.AllowRedeployment {
		sess.Abort()
		return jsonErr(c, fiber.StatusConflict, "Conflict")
	}

	estimated := storage.EstimateUploadDiskSpace(localFilePath, sess.TotalSize)
	if !status.CanAllocateDiskSpace(state, uint64(estimated)) {
		sess.Abort()
		return jsonErr(c, fiber.StatusInsufficientStorage, "Insufficient disk space to upload file")
	}

	var digests *storage.ContentDigests
	fileSize := sess.TotalSize
	modTime := time.Now().UnixNano()

	if sess.GenerateChecksums {
		d, n, err := storage.HashFile(sess.TempPath)
		if err != nil {
			sess.Abort()
			return jsonErr(c, fiber.StatusInternalServerError, "Failed to hash upload")
		}
		digests = d
		fileSize = n
	} else if st, err := os.Stat(sess.TempPath); err == nil {
		fileSize = st.Size()
		modTime = st.ModTime().UnixNano()
	}

	tmpPath := sess.TempPath
	if err := storage.CommitUploadedFile(state, localFilePath, tmpPath, fileSize, modTime, existed, sess.GenerateChecksums, digests); err != nil {
		sess.Abort()
		msg := "Internal Server Error"
		if strings.HasPrefix(err.Error(), "Failed to upload to S3:") {
			msg = err.Error()
		}
		return jsonErr(c, fiber.StatusInternalServerError, msg)
	}

	sess.TempPath = ""
	sess.MarkCompleted()
	uploadSucceeded = true

	rel := filepath.ToSlash(localFilePath)
	storageSlash := filepath.ToSlash(cfg.StoragePath)
	rel = strings.TrimPrefix(rel, storageSlash)
	rel = strings.TrimPrefix(rel, "/")

	username, op, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	details := fmt.Sprintf("Repository: %s, File: %s, Size: %d bytes", sess.RepoName, rel, fileSize)
	audit.Log(state, &core.AuditLogEntry{
		Username:   username,
		Operator:   op,
		Action:     "UPLOAD",
		Details:    details,
		AuthMethod: authMethod,
		SessionID:  sessionID,
		IP:         ip,
	})

	return protohttp.WriteStatus(c, fiber.StatusCreated, &pb.ChunkedUploadCompleteResponse{
		Status: "created",
		Path:   rel,
	})
}

func completeUpdater(c fiber.Ctx, sess *Session) error {
	user := auth.GetUser(c)
	if user == nil || !user.IsManager() {
		sess.Abort()
		return jsonErr(c, fiber.StatusForbidden, "Forbidden")
	}

	reqSpace := sess.TotalSize * 3
	if reqSpace <= 0 {
		reqSpace = 100 * 1024 * 1024
	}
	if updater.CanAllocateDiskSpace != nil && !updater.CanAllocateDiskSpace(uint64(reqSpace)) {
		sess.Abort()
		return jsonErr(c, fiber.StatusInsufficientStorage, "Insufficient disk space to upload update package")
	}

	if !updater.TryBeginInstall() {
		sess.Abort()
		return c.Status(fiber.StatusConflict).SendString("Installation already in progress")
	}
	defer updater.EndInstall()

	updater.SetDownloadingProgress(50)

	zipPath := sess.TempPath
	sess.TempPath = ""

	targetPath, err := updater.ExtractExecutableFromZipPath(zipPath)
	_ = os.Remove(zipPath)
	sess.MarkCompleted()

	if err != nil {
		updater.SetError(err.Error())
		return jsonErr(c, fiber.StatusBadRequest, err.Error())
	}

	updater.SetReadyToRestart(targetPath, "offline")
	return protohttp.Write(c, &pb.ChunkedUploadCompleteResponse{
		Status:  "ready_to_restart",
		Message: "Offline update installed successfully",
	})
}

func handleAbort(c fiber.Ctx, mgr *Manager) error {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || user.Username == "guest" {
		return jsonErr(c, fiber.StatusUnauthorized, "Unauthorized")
	}
	found, owned := mgr.AbortOwned(c.Params("id"), user.Username)
	if !found {
		return c.SendStatus(fiber.StatusNoContent)
	}
	if !owned {
		return jsonErr(c, fiber.StatusForbidden, "Forbidden")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
