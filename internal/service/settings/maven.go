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
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.yaml.in/yaml/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	mavenservice "renop/internal/service/maven"
	"renop/internal/service/outboundproxy"
	"renop/internal/service/repositorygate"
	"renop/internal/service/statistics"
	"renop/internal/service/storage"
	"renop/internal/utils"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

const repositoryMigrationErrorHeader = "X-Renop-Error-Code"

var errRepositoryMigrationConflict = errors.New("repository engine changed concurrently")

func repositoryMigrationError(c fiber.Ctx, status int, code, message string) error {
	c.Set(repositoryMigrationErrorHeader, code)
	return c.Status(status).SendString(message)
}

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
	releaseMigration := repositorygate.AcquireMigration(repoName)
	defer releaseMigration()
	cfg := state.Inner.Config.Load()
	existing := cfg.Maven.Repositories[repoName]
	for configuredName := range cfg.Maven.Repositories {
		if configuredName != repoName && strings.EqualFold(configuredName, repoName) {
			return c.Status(fiber.StatusConflict).SendString("Repository name conflicts with an existing repository")
		}
	}
	creating := existing == nil
	if existing != nil && state.GetDB() != nil {
		if pending, pendingErr := state.GetDB().HasPendingPublicationReviews(repoName); pendingErr != nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("Repository review state is unavailable")
		} else if pending {
			c.Set("X-Renop-Error-Code", "repository_pending_review")
			return c.Status(fiber.StatusConflict).SendString("Repository has pending publication reviews")
		}
	}
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
	if existing != nil && existing.DownloadStatistics != nil {
		enabled := *existing.DownloadStatistics
		repo.DownloadStatistics = &enabled
	}
	if existing != nil {
		repo.PublicationReview = existing.PublicationReviewPolicy()
	}
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
	if existing != nil && existing.NormalizedFormat() == config.RepositoryFormatFiles &&
		repo.NormalizedFormat() == config.RepositoryFormatFiles {
		repo.MavenRestore = existing.MavenRestore.DeepCopy()
	} else if repo.NormalizedFormat() != config.RepositoryFormatFiles {
		repo.MavenRestore = nil
	}
	if repo.Format == config.RepositoryFormatCargo || repo.Format == config.RepositoryFormatNPM {
		repo.AllowRedeployment = false
		repo.RequireGPGSignature = false
	}
	if repo.Format == config.RepositoryFormatFiles {
		repo.AllowRedeployment = true
		repo.RequireGPGSignature = false
	}
	if repo.PublicationReviewPolicy() != config.PublicationReviewOff {
		repo.AllowRedeployment = false
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
	state.Inner.ConfigWriteLock.Lock()
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
	state.Inner.ConfigWriteLock.Unlock()

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
		Action:     audit.ActionSettingsUpdate,
		Details:    "Updated repository settings for (" + repoName + ")",
		AuthMethod: authMethod,
		SessionID:  sessionID,
		IP:         ip,
	})

	return c.Status(fiber.StatusOK).SendString("")
}

func repositoryWithMigratedEngine(repo *config.Repository, target string) *config.Repository {
	migrated := repo.DeepCopy()
	if migrated.DownloadStatistics == nil {
		enabled := repo.DownloadStatisticsEnabled()
		migrated.DownloadStatistics = &enabled
	}
	if target == config.RepositoryFormatFiles {
		migrated.MavenRestore = &config.MavenRestoreSettings{
			Format: repo.ConfiguredFormat(), AllowRedeployment: repo.AllowRedeployment,
			RequireGPGSignature: repo.RequireGPGSignature, PublicationReview: repo.PublicationReviewPolicy(),
		}
		migrated.Format = config.RepositoryFormatFiles
		migrated.AllowRedeployment = true
		migrated.RequireGPGSignature = false
		migrated.PublicationReview = config.PublicationReviewOff
		return migrated
	}
	migrated.Format = config.RepositoryFormatMaven
	migrated.AllowRedeployment = false
	migrated.RequireGPGSignature = false
	migrated.PublicationReview = config.PublicationReviewOff
	if restore := repo.MavenRestore; restore != nil {
		if restore.Format == config.RepositoryFormatMaven || restore.Format == config.RepositoryFormatMavenClassic {
			migrated.Format = restore.Format
		}
		migrated.AllowRedeployment = restore.AllowRedeployment
		migrated.RequireGPGSignature = restore.RequireGPGSignature
		migrated.PublicationReview = restore.PublicationReview
	}
	migrated.MavenRestore = nil
	return migrated
}

func replaceRepositoryConfig(state *core.AppState, repository, expectedFormat string, replacement *config.Repository) error {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil || replacement == nil {
		return core.ErrDatabaseUnavailable
	}
	state.Inner.ConfigWriteLock.Lock()
	defer state.Inner.ConfigWriteLock.Unlock()
	return state.Inner.FileIndex.UpdateMetadataCallback(func() error {
		currentConfig := state.Inner.Config.Load()
		if currentConfig == nil {
			return core.ErrDatabaseUnavailable
		}
		current := currentConfig.Maven.Repositories[repository]
		if current == nil || current.NormalizedFormat() != expectedFormat {
			return errRepositoryMigrationConflict
		}
		updatedConfig := currentConfig.DeepCopy()
		updatedConfig.Maven.Repositories[repository] = replacement.DeepCopy()
		if err := saveRepositories(updatedConfig); err != nil {
			return err
		}
		state.Inner.Config.Store(updatedConfig)
		config.ClearRepoCacheConfigs()
		return nil
	})
}

func repositoryHasPendingGPG(state *core.AppState, repository string) (bool, error) {
	db := state.GetDB()
	if db == nil {
		return false, core.ErrDatabaseUnavailable
	}
	releases, err := db.ListPendingGPGReleases()
	if err != nil {
		return false, err
	}
	for _, release := range releases {
		if release != nil && strings.EqualFold(release.Repository, repository) {
			return true, nil
		}
	}
	return false, nil
}

// MigrateRepositoryEngine converts a Maven repository to files storage or restores it to Maven.
func MigrateRepositoryEngine(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return repositoryMigrationError(c, fiber.StatusForbidden, "repository_migration_forbidden", "Forbidden")
	}
	repository := strings.Clone(c.Params("name"))
	if !utils.IsValidRepositoryName(repository) {
		return repositoryMigrationError(c, fiber.StatusBadRequest, "repository_migration_invalid", "Invalid repository name")
	}
	target := strings.ToLower(strings.TrimSpace(c.Params("target")))
	if target != config.RepositoryFormatMaven && target != config.RepositoryFormatFiles {
		return repositoryMigrationError(c, fiber.StatusBadRequest, "repository_migration_invalid", "Invalid target repository engine")
	}
	releaseMigration := repositorygate.AcquireMigration(repository)
	defer releaseMigration()
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return repositoryMigrationError(c, fiber.StatusServiceUnavailable, "repository_migration_unavailable", "Repository configuration is unavailable")
	}
	current := cfg.Maven.Repositories[repository]
	if current == nil {
		return repositoryMigrationError(c, fiber.StatusNotFound, "repository_migration_not_found", "Repository not found")
	}
	source := current.NormalizedFormat()
	if source == target {
		return repositoryMigrationError(c, fiber.StatusConflict, "repository_migration_unchanged", "Repository already uses the requested engine")
	}
	if (source != config.RepositoryFormatMaven && source != config.RepositoryFormatFiles) ||
		(target != config.RepositoryFormatMaven && target != config.RepositoryFormatFiles) {
		return repositoryMigrationError(c, fiber.StatusConflict, "repository_migration_unsupported", "Only Maven and files repositories can be migrated")
	}
	if pending, err := repositoryHasPendingGPG(state, repository); err != nil {
		return repositoryMigrationError(c, fiber.StatusServiceUnavailable, "repository_migration_unavailable", "Repository publication state is unavailable")
	} else if pending {
		return repositoryMigrationError(c, fiber.StatusConflict, "repository_migration_pending_gpg", "Repository has pending GPG publications")
	}
	if pending, err := state.GetDB().HasPendingPublicationReviews(repository); err != nil {
		return repositoryMigrationError(c, fiber.StatusServiceUnavailable, "repository_migration_unavailable", "Repository review state is unavailable")
	} else if pending {
		return repositoryMigrationError(c, fiber.StatusConflict, "repository_migration_pending_review", "Repository has pending publication reviews")
	}

	original := current.DeepCopy()
	replacement := repositoryWithMigratedEngine(current, target)
	if target == config.RepositoryFormatMaven {
		if err := mavenservice.RebuildRepositoryCatalog(state, repository); err != nil {
			return repositoryMigrationError(c, fiber.StatusInternalServerError, "repository_migration_rebuild_failed", "Failed to rebuild Maven repository metadata")
		}
		if err := replaceRepositoryConfig(state, repository, source, replacement); err != nil {
			cleanupErr := state.GetDB().DeleteMavenRepository(repository)
			if cleanupErr != nil {
				log.Printf("failed to clean Maven metadata after repository migration error: %v", errors.Join(err, cleanupErr))
				return repositoryMigrationError(c, fiber.StatusInternalServerError, "repository_migration_cleanup_failed", "Failed to clean incomplete Maven repository metadata")
			}
			if errors.Is(err, errRepositoryMigrationConflict) {
				return repositoryMigrationError(c, fiber.StatusConflict, "repository_migration_conflict", "Repository settings changed during migration")
			}
			return repositoryMigrationError(c, fiber.StatusInternalServerError, "repository_migration_failed", "Failed to save migrated repository settings")
		}
	} else {
		if err := replaceRepositoryConfig(state, repository, source, replacement); err != nil {
			if errors.Is(err, errRepositoryMigrationConflict) {
				return repositoryMigrationError(c, fiber.StatusConflict, "repository_migration_conflict", "Repository settings changed during migration")
			}
			return repositoryMigrationError(c, fiber.StatusInternalServerError, "repository_migration_failed", "Failed to save migrated repository settings")
		}
		if err := state.GetDB().DeleteMavenRepository(repository); err != nil {
			rollbackErr := replaceRepositoryConfig(state, repository, target, original)
			if rollbackErr != nil {
				return repositoryMigrationError(c, fiber.StatusInternalServerError, "repository_migration_rollback_failed", "Repository migration rollback failed")
			}
			return repositoryMigrationError(c, fiber.StatusInternalServerError, "repository_migration_failed", "Failed to remove Maven repository metadata")
		}
	}
	if latest := state.Inner.Config.Load(); latest != nil {
		storage.InitS3(latest)
	}
	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionRepositoryMigrate,
		Details:    "Repository: " + repository + ", engine: " + source + " -> " + target,
		AuthMethod: authMethod, SessionID: sessionID, IP: ip,
	})
	return protohttp.Write(c, &pb.StatusOk{Status: "ok"})
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
	releaseMigration := repositorygate.AcquireMigration(repoName)
	defer releaseMigration()
	if db := state.GetDB(); db != nil {
		if pending, err := db.HasPendingPublicationReviews(repoName); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("Repository review state is unavailable")
		} else if pending {
			c.Set("X-Renop-Error-Code", "repository_pending_review")
			return c.Status(fiber.StatusConflict).SendString("Repository has pending publication reviews")
		}
	}

	var (
		notFound         bool
		storagePath      string
		repositoryFormat string
		s3Cfg            *config.S3Config
	)
	state.Inner.ConfigWriteLock.Lock()
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
	state.Inner.ConfigWriteLock.Unlock()

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}
	if notFound {
		if db := state.GetDB(); db != nil {
			actedAt := time.Now().UnixMilli()
			if err := errors.Join(db.DeleteMavenRepository(repoName),
				db.DeleteCargoRepository(repoName, actedAt), db.DeleteDockerRepository(repoName), db.DeleteNPMRepository(repoName),
				statistics.GetCounter(state).ResetRepository(repoName)); err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to remove repository package metadata")
			}
		}
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}
	var metadataErr error
	if db := state.GetDB(); db != nil {
		switch repositoryFormat {
		case config.RepositoryFormatMaven:
			metadataErr = db.DeleteMavenRepository(repoName)
		case config.RepositoryFormatCargo:
			metadataErr = db.DeleteCargoRepository(repoName, time.Now().UnixMilli())
		case config.RepositoryFormatDocker:
			metadataErr = db.DeleteDockerRepository(repoName)
		case config.RepositoryFormatNPM:
			metadataErr = db.DeleteNPMRepository(repoName)
		}
		metadataErr = errors.Join(metadataErr, statistics.GetCounter(state).ResetRepository(repoName))
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
