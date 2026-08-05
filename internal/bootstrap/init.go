/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package bootstrap

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/index"
	"renop/internal/service/javadocs"
	"renop/internal/service/status"
	"renop/internal/service/storage"
	"renop/internal/service/tasks"
)

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
			_ = os.WriteFile(repositoriesPath, yamlData, 0644)
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
				PublicId:    sessionDto.PublicId,
				Username:    sessionDto.Username,
				Ip:          sessionDto.Ip,
				UserAgent:   sessionDto.UserAgent,
				CreatedAt:   sessionDto.CreatedAt,
				LoginMethod: lm,
			}
			session.LastActive.Store(sessionDto.LastActive)
			state.Inner.Sessions.Store(sessionDto.SessionToken, session)
		}
	}

	state.Inner.FileIndex = fileIndex

	state.Inner.FileCache = core.NewFileByteCache(int(cfg.Server.FileCacheSizeMb) << 20)

	bootstrapCtx := BootstrapContext{
		ConfigPath: configPath,
		IndexPath:  indexPath,
	}

	return state, bootstrapCtx
}

func StartServices(state *core.AppState, context BootstrapContext) {
	status.StartStatusSnapshotScheduler(state, 20*time.Second)
	tasks.StartIndexSaver(state, context.IndexPath)
	tasks.StartSessionCleaner(state)

	go func() {
		for {
			time.Sleep(1 * time.Minute)
			now := time.Now().UnixMilli()
			state.Inner.AuthCache.Range(func(key string, val core.AuthCacheEntry) bool {
				if now > val.ExpiredAt {
					state.DeleteAuthCache(key)
				}
				return true
			})
		}
	}()

	cfg := state.Inner.Config.Load().(*config.Config)
	storagePath := cfg.StoragePath

	watcher, err := index.StartFileWatcher(storagePath, state.Inner.FileIndex)
	if err == nil {
		state.Inner.IndexWatcherMutex.Lock()
		state.Inner.IndexWatcher = watcher
		state.Inner.IndexWatcherMutex.Unlock()
	}

	index.RebuildIndexAsync(storagePath, state.Inner.FileIndex)

	for repoName := range cfg.Maven.Repositories {
		repoDir := filepath.Join(storagePath, repoName)
		_ = os.MkdirAll(repoDir, 0755)
	}
}
