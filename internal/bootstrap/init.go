/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package bootstrap initializes application state and service lifecycles.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/middleware"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/cargodocs"
	"renop/internal/service/index"
	"renop/internal/service/javadocs"
	"renop/internal/service/statistics"
	"renop/internal/service/status"
	"renop/internal/service/storage"
	"renop/internal/service/tasks"
	"renop/internal/service/updater"
	"renop/internal/service/upload"
	"renop/internal/utils"
)

const (
	statusSnapshotInterval          = 20 * time.Second
	indexSaveInterval               = 10 * time.Second
	sessionCleanupInterval          = time.Minute
	securityCleanupInterval         = time.Minute
	fidoCleanupInterval             = 5 * time.Minute
	ipLimiterCleanupInterval        = 5 * time.Minute
	databaseCacheInterval           = 2 * time.Minute
	auditCleanupInterval            = 10 * time.Minute
	downloadStatisticsFlushInterval = 2 * time.Second
	gpgQueuePollInterval            = time.Second
	uploadCleanupInterval           = time.Minute
)

// ServiceRuntime owns the shared periodic scheduler and shutdown finalizers.
type ServiceRuntime struct {
	scheduler       *tasks.Scheduler
	state           *core.AppState
	indexSave       func(context.Context)
	downloadCounter *statistics.Counter
	closeOnce       sync.Once
	closeErr        error
}

// Close stops periodic work, persists pending index and download-statistics state, and
// releases the file watcher.
func (runtime *ServiceRuntime) Close() error {
	if runtime == nil {
		return nil
	}
	runtime.closeOnce.Do(func() {
		if runtime.scheduler != nil {
			runtime.scheduler.Close()
		}
		var closeErrors []error
		if runtime.state != nil && runtime.state.Inner != nil {
			runtime.state.Inner.IndexWatcherMutex.Lock()
			watcher := runtime.state.Inner.IndexWatcher
			runtime.state.Inner.IndexWatcher = nil
			runtime.state.Inner.IndexWatcherMutex.Unlock()
			if watcher != nil {
				if err := watcher.Close(); err != nil {
					closeErrors = append(closeErrors, err)
				}
			}
		}
		if runtime.downloadCounter != nil {
			if err := runtime.downloadCounter.Flush(); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		if runtime.indexSave != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			runtime.indexSave(ctx)
			cancel()
		}
		runtime.closeErr = errors.Join(closeErrors...)
	})
	return runtime.closeErr
}

func Initialize() (*core.AppState, BootstrapContext) {
	configPath := os.Getenv("RENOP_CONFIG")
	if configPath == "" {
		configPath = "config.yaml"
	}
	cfg := LoadConfig(configPath)

	repositoriesPath := os.Getenv("RENOP_REPOSITORIES")
	if repositoriesPath == "" {
		repositoriesPath = "repositories.yaml"
	}

	if _, err := os.Stat(repositoriesPath); err == nil {
		cfg.Maven = LoadMaven(repositoriesPath)
	} else {
		yamlData, err := yaml.Marshal(&cfg.Maven)
		if err == nil {
			_ = utils.WritePrivateFile(repositoriesPath, yamlData)
		}
	}

	dbInstance, dbErr := database.InitDB(cfg.Database)
	if dbErr != nil {
		log.Fatalf("Database initialization failed: %v", dbErr)
	}
	if dbInstance == nil {
		log.Fatal("Database initialization returned nil — check your database configuration.")
	}

	indexPath := os.Getenv("RENOP_INDEX")
	if indexPath == "" {
		indexPath = "index.json"
	}
	fileIndex := LoadFileIndex(indexPath)

	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.DB = dbInstance
	storage.InitS3(cfg)
	javadocs.InitJavadocs(cfg)
	cargodocs.InitCargodocs(cfg)

	concurrencyLimit := int(cfg.Server.MaxActiveRequests)
	if concurrencyLimit <= 0 {
		concurrencyLimit = 512
	}
	if concurrencyLimit > 262144 {
		concurrencyLimit = 262144
	}
	state.Inner.ProxyClientSemaphore = make(chan struct{}, concurrencyLimit)

	count, err := dbInstance.CountTokens()
	if err == nil && count > 0 {
		state.Inner.TokensCount.Store(count)
	}

	activeSessions, err := dbInstance.GetActiveSessions(time.Now().UnixMilli() - core.SessionIdleTimeoutMillis)
	if err == nil {
		for _, sessionDto := range activeSessions {
			lm := sessionDto.LoginMethod
			if lm == "" {
				lm = "password"
			}
			session := &core.Session{
				PublicID:    sessionDto.PublicID,
				Username:    sessionDto.Username,
				IP:          sessionDto.IP,
				UserAgent:   sessionDto.UserAgent,
				CreatedAt:   sessionDto.CreatedAt,
				LoginMethod: lm,
			}
			session.LastActive.Store(sessionDto.LastActive)
			state.Inner.Sessions.Store(sessionDto.SessionToken, session)
		}
	}

	state.Inner.FileIndex = fileIndex
	if err := storage.RestoreGPGReleaseState(state); err != nil {
		log.Fatalf("Failed to restore GPG publication queue: %v", err)
	}

	state.Inner.FileCache = core.NewFileByteCache(int(cfg.Server.FileCacheSizeMb) << 20)

	bootstrapCtx := BootstrapContext{
		ConfigPath: configPath,
		IndexPath:  indexPath,
	}

	return state, bootstrapCtx
}

// StartServices starts event-driven workers and registers coalescible periodic
// maintenance on one process-wide scheduler.
func StartServices(state *core.AppState, bootstrapContext BootstrapContext) (*ServiceRuntime, error) {
	if state == nil || state.Inner == nil {
		return nil, errors.New("application state is unavailable")
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return nil, errors.New("application configuration is unavailable")
	}
	storagePath := cfg.StoragePath
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return nil, fmt.Errorf("create storage directory: %w", err)
	}
	for repoName := range cfg.Maven.Repositories {
		if err := os.MkdirAll(filepath.Join(storagePath, repoName), 0755); err != nil {
			return nil, fmt.Errorf("create repository directory %q: %w", repoName, err)
		}
	}
	indexSave := tasks.NewIndexSaveTask(state, bootstrapContext.IndexPath)
	downloadCounter := statistics.GetCounter(state)
	uploadCleanup := upload.NewCleanupTask(storagePath)
	scheduler := tasks.NewScheduler()
	runtimeServices := &ServiceRuntime{
		scheduler:       scheduler,
		state:           state,
		indexSave:       indexSave,
		downloadCounter: downloadCounter,
	}
	schedule := func(name string, interval, initialDelay time.Duration, run func(context.Context)) error {
		if err := scheduler.Schedule(name, interval, initialDelay, run); err != nil {
			return errors.Join(err, runtimeServices.Close())
		}
		return nil
	}

	registrations := []struct {
		name         string
		interval     time.Duration
		initialDelay time.Duration
		run          func(context.Context)
	}{
		{"status-snapshot", statusSnapshotInterval, 0, func(context.Context) {
			status.UpdateStatusSnapshot(state)
		}},
		{"index-save", indexSaveInterval, indexSaveInterval, indexSave},
		{"session-cleanup", sessionCleanupInterval, sessionCleanupInterval, func(context.Context) {
			if err := tasks.CleanExpiredSessions(state, time.Now()); err != nil {
				state.Inner.FailuresCount.Add(1)
				log.Printf("Failed to clean expired sessions: %v", err)
			}
		}},
		{"security-cache-cleanup", securityCleanupInterval, securityCleanupInterval, func(context.Context) {
			tasks.PruneAuthCache(state, time.Now())
			state.Inner.AnomalyFailures.PruneExpired()
		}},
		{"fido-session-cleanup", fidoCleanupInterval, fidoCleanupInterval, func(context.Context) {
			auth.PruneExpiredFidoSessions(time.Now())
		}},
		{"ip-limiter-cleanup", ipLimiterCleanupInterval, ipLimiterCleanupInterval, func(context.Context) {
			middleware.PruneIPLimiters()
		}},
		{"database-cache-eviction", databaseCacheInterval, databaseCacheInterval, func(context.Context) {
			if evicter, ok := state.Inner.DB.(interface{ EvictExpiredCaches() }); ok {
				evicter.EvictExpiredCaches()
			}
		}},
		{"audit-log-cleanup", auditCleanupInterval, auditCleanupInterval, func(context.Context) {
			audit.CleanExpiredLogs(state)
		}},
		{"download-statistics-flush", downloadStatisticsFlushInterval, downloadStatisticsFlushInterval, func(context.Context) {
			if err := downloadCounter.Flush(); err != nil {
				state.Inner.FailuresCount.Add(1)
				log.Printf("Failed to flush download statistics: %v", err)
			}
		}},
		{"gpg-release-poll", gpgQueuePollInterval, gpgQueuePollInterval, func(context.Context) {
			storage.NotifyGPGReleaseWorker(state)
		}},
		{"upload-cleanup", uploadCleanupInterval, 0, func(context.Context) {
			uploadCleanup()
		}},
		{"updater-check", updater.AutoCheckInterval, updater.AutoCheckInitialDelay, func(ctx context.Context) {
			if err := updater.RunScheduledCheck(ctx, state); err != nil {
				state.Inner.FailuresCount.Add(1)
				log.Printf("Scheduled updater check failed: %v", err)
			}
		}},
	}
	for _, registration := range registrations {
		if err := schedule(registration.name, registration.interval, registration.initialDelay, registration.run); err != nil {
			return nil, err
		}
	}

	watcher, err := index.StartFileWatcher(storagePath, state.Inner.FileIndex)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("start storage watcher: %w", err), runtimeServices.Close())
	}
	state.Inner.IndexWatcherMutex.Lock()
	state.Inner.IndexWatcher = watcher
	state.Inner.IndexWatcherMutex.Unlock()

	storage.StartGPGReleaseWorker(state)
	index.RebuildIndexAsync(storagePath, state.Inner.FileIndex)
	return runtimeServices, nil
}
