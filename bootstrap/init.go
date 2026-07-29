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
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"renop/config"
	"renop/core"
	"renop/index"
	"renop/javadocs"
	"renop/status"
	"renop/storage"
	"renop/tasks"
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

	tokensPath := os.Getenv("RENOP_TOKENS")
	if tokensPath == "" {
		tokensPath = "tokens.yaml"
	}

	tokenMap := LoadTokens(tokensPath)
	usersList := make([]*core.AccessToken, 0, len(tokenMap))
	for k, v := range tokenMap {
		v.Name = strings.ToLower(k)
		usersList = append(usersList, v)
	}

	indexPath := os.Getenv("RENOP_INDEX")
	if indexPath == "" {
		indexPath = "index.json"
	}
	fileIndex := LoadFileIndex(indexPath)

	sessionsPath := os.Getenv("RENOP_SESSIONS")
	if sessionsPath == "" {
		sessionsPath = "sessions.bin"
	}

	sessionsDb, migrateSessions := LoadSessions(sessionsPath)

	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
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

	for _, v := range usersList {
		if v == nil {
			continue
		}
		v.Name = strings.ToLower(v.Name)
		state.Inner.TokenRepository.Store(v.Name, v)
		for _, t := range v.Tokens {
			state.Inner.TokenIndex.Store(t, v)
		}
	}
	state.Inner.TokensCount.Store(uint64(len(usersList)))

	for _, sessionDto := range sessionsDb {
		session := &core.Session{
			PublicId:  sessionDto.PublicId,
			Username:  strings.ToLower(sessionDto.Username),
			Ip:        sessionDto.Ip,
			UserAgent: sessionDto.UserAgent,
			CreatedAt: sessionDto.CreatedAt,
		}
		session.LastActive.Store(sessionDto.LastActive)
		state.Inner.Sessions.Store(sessionDto.SessionToken, session)
	}
	if migrateSessions {
		state.Inner.SessionsIsDirty.Store(true)
	}

	state.Inner.FileIndex = fileIndex

	// Lightweight size-capped map (starts empty — no bigcache shard preallocation).
	state.Inner.FileCache = core.NewFileByteCache(int(cfg.Server.FileCacheSizeMb) << 20)

	bootstrapCtx := BootstrapContext{
		ConfigPath:   configPath,
		IndexPath:    indexPath,
		SessionsPath: sessionsPath,
	}

	return state, bootstrapCtx
}

func StartServices(state *core.AppState, context BootstrapContext) {
	status.StartStatusSnapshotScheduler(state, 20*time.Second)
	tasks.StartIndexSaver(state, context.IndexPath)
	tasks.StartSessionSaver(state, context.SessionsPath)

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
