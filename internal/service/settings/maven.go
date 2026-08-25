/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.yaml.in/yaml/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/outboundproxy"
	"renop/internal/service/storage"
	"renop/internal/utils"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func GetMavenRepositories(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	cfg := state.Inner.Config.Load()
	return protohttp.Write(c, pb.FromMavenRepositories(cfg.Maven.Repositories))
}

func PutMavenRepository(c fiber.Ctx, state *core.AppState) error {
	repoName := strings.Clone(c.Params("name"))

	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	cfg := state.Inner.Config.Load()
	existing := cfg.Maven.Repositories[repoName]
	for configuredName := range cfg.Maven.Repositories {
		if configuredName != repoName && strings.EqualFold(configuredName, repoName) {
			return c.Status(fiber.StatusConflict).SendString("Repository name conflicts with an existing repository")
		}
	}
	creating := existing == nil
	if creating {
		if !utils.IsValidRepositorySlug(repoName) {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid repository name")
		}
	} else if !utils.IsValidRepositoryName(repoName) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid repository name")
	}

	var msg pb.Repository
	if err := protohttp.Read(c, &msg); err != nil {
		if err == fiber.ErrRequestEntityTooLarge {
			return err
		}
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}

	repo := pb.ToRepository(&msg)
	if repo == nil {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}
	repo.Name = repoName
	requestedFormat := strings.ToLower(strings.TrimSpace(repo.Format))
	if existing == nil && requestedFormat == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Repository format is required when creating a repository")
	}
	if existing != nil && requestedFormat == "" {
		requestedFormat = existing.ConfiguredFormat()
	}
	if !config.IsSupportedRepositoryFormat(requestedFormat) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid repository format")
	}
	repo.Format = requestedFormat
	if existing != nil && existing.NormalizedFormat() != repo.NormalizedFormat() {
		return c.Status(fiber.StatusConflict).SendString("Repository format cannot be changed after creation")
	}
	if repo.Format == config.RepositoryFormatCargo {
		repo.AllowRedeployment = false
		repo.RequireGPGSignature = false
	}
	if repo.Format == config.RepositoryFormatFiles {
		repo.AllowRedeployment = true
		repo.RequireGPGSignature = false
	}
	if repo.Mirrors == nil {
		repo.Mirrors = []config.Mirror{}
	}
	vis := strings.ToUpper(strings.TrimSpace(repo.Visibility))
	if vis != "PUBLIC" && vis != "HIDDEN" && vis != "PRIVATE" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid visibility. Expected PUBLIC, HIDDEN, or PRIVATE")
	}
	repo.Visibility = vis
	for i := range repo.Mirrors {
		mirror := &repo.Mirrors[i]
		if _, err := outboundproxy.ResolveMirrorSelection(mirror.ProxyMode, cfg.Proxy); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid mirror proxy selection: " + err.Error())
		}
		mirror.Proxy = nil
		if err := repo.Mirrors[i].Authorization.Validate(); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid mirror authentication: " + err.Error())
		}
		if err := mirror.ValidateArtifactURL(repo.Format); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid mirror artifact URL: " + err.Error())
		}
		if repo.Format != config.RepositoryFormatCargo {
			mirror.ArtifactURL = ""
		}
	}
	if repo.S3 != nil {
		keyPrefix, err := storage.NormalizeS3KeyPrefix(repo.S3.KeyPrefix)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid S3 key prefix")
		}
		repo.S3.KeyPrefix = keyPrefix
	}

	errRepositoryCreated := errors.New("repository was created concurrently")
	errRepositoryRemoved := errors.New("repository was removed concurrently")
	errFormatChanged := errors.New("repository format changed concurrently")
	err := state.Inner.FileIndex.UpdateMetadataCallback(func() error {
		oldConfig := state.Inner.Config.Load()
		current := oldConfig.Maven.Repositories[repoName]
		if creating {
			for configuredName := range oldConfig.Maven.Repositories {
				if strings.EqualFold(configuredName, repoName) {
					return errRepositoryCreated
				}
			}
			if err := ensureRepositoryStorageDir(state, repoName); err != nil {
				return err
			}
		} else if current == nil {
			return errRepositoryRemoved
		}
		if current != nil && current.NormalizedFormat() != repo.NormalizedFormat() {
			return errFormatChanged
		}
		newConfig := oldConfig.DeepCopy()

		newConfig.Maven.Repositories[repoName] = repo.DeepCopy()

		if err := saveRepositories(newConfig); err != nil {
			return err
		}
		state.Inner.Config.Store(newConfig)
		config.ClearRepoCacheConfigs()
		return nil
	})

	if err != nil {
		if errors.Is(err, errRepositoryCreated) {
			return c.Status(fiber.StatusConflict).SendString("Repository already exists")
		}
		if errors.Is(err, errRepositoryRemoved) {
			return c.Status(fiber.StatusConflict).SendString("Repository was removed while it was being updated")
		}
		if errors.Is(err, errFormatChanged) {
			return c.Status(fiber.StatusConflict).SendString("Repository format cannot be changed after creation")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	if cfg := state.Inner.Config.Load(); cfg != nil {
		storage.InitS3(cfg)
	}

	user, op, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username:   user,
		Operator:   op,
		Action:     "SETTINGS_UPDATE",
		Details:    "Updated repository settings for (" + repoName + ")",
		AuthMethod: authMethod,
		SessionID:  sessionID,
		IP:         ip,
	})

	return c.Status(fiber.StatusOK).SendString("")
}

func ensureRepositoryStorageDir(state *core.AppState, repoName string) error {
	if state == nil || state.Inner == nil {
		return errors.New("application state is unavailable")
	}
	cfgVal := state.Inner.Config.Load()
	if cfgVal == nil || cfgVal.StoragePath == "" {
		return errors.New("repository storage path is unavailable")
	}

	repoDir := filepath.Join(cfgVal.StoragePath, repoName)
	_, statErr := os.Stat(repoDir)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect repository storage directory: %w", statErr)
	}
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return fmt.Errorf("create repository storage directory: %w", err)
	}

	pathNorm := filepath.ToSlash(filepath.Clean(repoDir))
	if state.Inner.FileIndex == nil {
		return errors.New("repository file index is unavailable")
	}
	state.Inner.FileIndex.InsertDir(pathNorm)

	state.Inner.IndexWatcherMutex.Lock()
	defer state.Inner.IndexWatcherMutex.Unlock()
	if state.Inner.IndexWatcher != nil {
		if err := state.Inner.IndexWatcher.Add(pathNorm); err != nil {
			state.Inner.FileIndex.RemoveDir(pathNorm)
			return fmt.Errorf("watch repository storage directory: %w", err)
		}
	}
	return nil
}

func DeleteMavenRepository(c fiber.Ctx, state *core.AppState) error {
	repoName := strings.Clone(c.Params("name"))

	if !utils.IsValidRepositoryName(repoName) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid repository name")
	}

	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	var (
		notFound         bool
		storagePath      string
		repositoryFormat string
		s3Cfg            *config.S3Config
	)
	err := state.Inner.FileIndex.UpdateMetadataCallback(func() error {
		oldConfig := state.Inner.Config.Load()
		repo, ok := oldConfig.Maven.Repositories[repoName]
		if !ok {
			notFound = true
			return nil
		}
		storagePath = oldConfig.StoragePath
		repositoryFormat = repo.NormalizedFormat()
		if repo.S3 != nil && repo.S3.Enabled {
			s3Cfg = repo.S3.DeepCopy()
		}

		newConfig := oldConfig.DeepCopy()
		delete(newConfig.Maven.Repositories, repoName)

		if err := saveRepositories(newConfig); err != nil {
			return err
		}
		state.Inner.Config.Store(newConfig)
		storage.InitS3(newConfig)
		config.ClearRepoCacheConfigs()
		return nil
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}
	if notFound {
		if db := state.GetDB(); db != nil {
			actedAt := time.Now().UnixMilli()
			if err := errors.Join(db.DeleteMavenRepository(repoName, actedAt),
				db.DeleteCargoRepository(repoName, actedAt), db.DeleteDockerRepository(repoName)); err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to remove repository package metadata")
			}
		}
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}
	var metadataErr error
	if db := state.GetDB(); db != nil {
		switch repositoryFormat {
		case config.RepositoryFormatMaven:
			metadataErr = db.DeleteMavenRepository(repoName, time.Now().UnixMilli())
		case config.RepositoryFormatCargo:
			metadataErr = db.DeleteCargoRepository(repoName, time.Now().UnixMilli())
		case config.RepositoryFormatDocker:
			metadataErr = db.DeleteDockerRepository(repoName)
		}
	}
	storage.RemoveRepositoryStorage(state, storagePath, repoName, s3Cfg)
	if metadataErr != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to remove repository package metadata")
	}

	return c.Status(fiber.StatusOK).SendString("")
}

func saveRepositories(cfg *config.Config) error {
	yamlData, err := yaml.Marshal(&cfg.Maven)
	if err != nil {
		return err
	}

	reposPath := os.Getenv("RENOP_REPOSITORIES")
	if reposPath == "" {
		reposPath = "repositories.yaml"
	}
	tmpPath := reposPath + ".tmp"
	if err := utils.WritePrivateFile(tmpPath, yamlData); err != nil {
		return err
	}
	return utils.SafeRename(tmpPath, reposPath)
}
