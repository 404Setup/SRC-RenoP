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
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"
	"go.yaml.in/yaml/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/proxy"
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

	reservedName := strings.ToLower(repoName)
	if !utils.IsValidRepositoryName(repoName) || reservedName == "css" || reservedName == "js" || reservedName == "svg" || reservedName == "api" || reservedName == "javadocs" || reservedName == "assets" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid repository name")
	}

	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
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
	if repo.Mirrors == nil {
		repo.Mirrors = []config.Mirror{}
	}
	vis := strings.ToUpper(strings.TrimSpace(repo.Visibility))
	if vis != "PUBLIC" && vis != "HIDDEN" && vis != "PRIVATE" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid visibility. Expected PUBLIC, HIDDEN, or PRIVATE")
	}
	repo.Visibility = vis
	for i := range repo.Mirrors {
		if err := proxy.ValidateMirrorProxy(repo.Mirrors[i].Proxy); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid mirror proxy: " + err.Error())
		}
	}
	if repo.S3 != nil {
		keyPrefix, err := storage.NormalizeS3KeyPrefix(repo.S3.KeyPrefix)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid S3 key prefix")
		}
		repo.S3.KeyPrefix = keyPrefix
	}

	err := state.Inner.FileIndex.UpdateMetadataCallback(func() error {
		oldConfig := state.Inner.Config.Load()
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
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	if cfg := state.Inner.Config.Load(); cfg != nil {
		storage.InitS3(cfg)
	}

	ensureRepositoryStorageDir(state, repoName)

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

func ensureRepositoryStorageDir(state *core.AppState, repoName string) {
	cfgVal := state.Inner.Config.Load()
	if cfgVal == nil || cfgVal.StoragePath == "" {
		return
	}

	repoDir := filepath.Join(cfgVal.StoragePath, repoName)
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return
	}

	pathNorm := filepath.ToSlash(filepath.Clean(repoDir))
	state.Inner.FileIndex.InsertDir(pathNorm)

	state.Inner.IndexWatcherMutex.Lock()
	if state.Inner.IndexWatcher != nil {
		_ = state.Inner.IndexWatcher.Add(pathNorm)
	}
	state.Inner.IndexWatcherMutex.Unlock()
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
		notFound    bool
		storagePath string
		s3Cfg       *config.S3Config
	)
	err := state.Inner.FileIndex.UpdateMetadataCallback(func() error {
		oldConfig := state.Inner.Config.Load()
		repo, ok := oldConfig.Maven.Repositories[repoName]
		if !ok {
			notFound = true
			return nil
		}
		storagePath = oldConfig.StoragePath
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
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}

	storage.RemoveRepositoryStorage(state, storagePath, repoName, s3Cfg)

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
