/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package main

import (
	"log"
	"os"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"

	"renop/api"
	"renop/auth"
	"renop/bootstrap"
	"renop/config"
	"renop/frontend"
	"renop/javadocs"
	"renop/middleware"
	"renop/settings"
	"renop/status"
	"renop/storage"
	"renop/token"
	"renop/updater"
	"renop/upload"
	"renop/utils"
)

func init() {
	fasthttp.SetBodySizePoolLimit(64*1024, 64*1024)
}

func main() {
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(40)
	}
	utils.InitLinuxMemoryTuning()

	state, context := bootstrap.Initialize()
	bootstrap.StartServices(state, context)
	cfg := state.Inner.Config.Load().(*config.Config)
	status.InitDebugMode(cfg.Server.DebugMode)

	concurrency := int(cfg.Server.MaxActiveRequests)
	if concurrency <= 0 {
		concurrency = 512
	}
	if concurrency > 262144 {
		concurrency = 262144
	}

	app := fiber.New(fiber.Config{
		ServerHeader:                 "RenoP",
		AppName:                      "RenoP",
		BodyLimit:                    2 * 1024 * 1024 * 1024,
		StreamRequestBody:            true,
		JSONEncoder:                  json.Marshal,
		JSONDecoder:                  json.Unmarshal,
		Concurrency:                  concurrency,
		IdleTimeout:                  30 * time.Second,
		ReadTimeout:                  120 * time.Second,
		WriteTimeout:                 30 * time.Minute,
		DisablePreParseMultipartForm: true,
		UnescapePath:                 false,
		ReadBufferSize:               4 * 1024,
	})

	app.Use(middleware.CorsMiddleware(state))
	app.Use(middleware.AnomalyMiddleware(state))

	opChan := make(chan token.TokenOp, 100)
	go token.StartTokenConsumer(state, opChan)
	token.AutoRegisterAdmin(state, opChan)

	app.Use(auth.AuthMiddleware(state))

	apiGroup := app.Group("/api")
	auth.SetupAuthRoutes(apiGroup, state, opChan)
	token.SetupTokenRoutes(apiGroup, state, opChan)
	status.SetupRoutes(apiGroup, state)
	status.SetupDebugRoutes(apiGroup)
	api.SetupApiRoutes(apiGroup, state)
	upload.SetupChunkedUploadRoutes(apiGroup, state)
	settings.SetupSettingsRoutes(apiGroup.Group("/settings"), state)
	updater.SetupUpdaterRoutes(apiGroup, state)

	storage.HTMLFallback = frontend.ServeIndex
	frontend.SetupFrontendRoutes(app, state)
	javadocs.SetupJavadocRoutes(app, state)
	storage.SetupRoutes(app, state)

	listenAddr := cfg.Server.Host + ":" + strconv.Itoa(int(cfg.Server.Port))

	if cfg.Server.SslEnabled {
		log.Printf("Listening on https://%s", listenAddr)
		log.Fatal(app.Listen(listenAddr, fiber.ListenConfig{
			CertFile:    cfg.Server.SslCertPath,
			CertKeyFile: cfg.Server.SslKeyPath,
		}))
		return
	}

	log.Printf("Listening on http://%s", listenAddr)
	log.Fatal(app.Listen(listenAddr))
}
