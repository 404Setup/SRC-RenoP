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
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"

	"renop/internal/api"
	"renop/internal/bootstrap"
	"renop/internal/daemon"
	"renop/internal/middleware"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/cargodocs"
	"renop/internal/service/docker"
	"renop/internal/service/frontend"
	"renop/internal/service/gpg"
	"renop/internal/service/javadocs"
	"renop/internal/service/message"
	"renop/internal/service/settings"
	"renop/internal/service/status"
	"renop/internal/service/storage"
	"renop/internal/service/token"
	"renop/internal/service/updater"
	"renop/internal/service/upload"
	"renop/internal/utils"
)

func init() {
	fasthttp.SetBodySizePoolLimit(64*1024, 64*1024)
}

func main() {
	utils.InitMemoryTuning()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--install", "-install":
			if err := daemon.Install(); err != nil {
				log.Fatalf("Failed to install RenoP service: %v", err)
			}
			return
		case "--uninstall", "-uninstall", "--remove", "-remove":
			if err := daemon.Uninstall(); err != nil {
				log.Fatalf("Failed to uninstall RenoP service: %v", err)
			}
			return
		case "--help", "-help", "-h", "/?":
			fmt.Println("RenoP - High-performance self-hosted package repository server")
			fmt.Println("\nUsage:")
			fmt.Println("  renop                 Start RenoP server")
			fmt.Println("  renop --install       Install and start RenoP as a system service")
			fmt.Println("  renop --uninstall     Stop and remove the RenoP system service")
			fmt.Println("  renop --help          Show help information")
			return
		}
	}

	if daemon.IsWindowsService() {
		if err := daemon.RunWindowsService(startServer); err != nil {
			log.Fatalf("Windows service error: %v", err)
		}
		return
	}

	startServer()
}

func startServer() {
	state, context := bootstrap.Initialize()
	bootstrap.StartServices(state, context)
	cfg := state.Inner.Config.Load()
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

	app.Use(middleware.APINoCacheMiddleware())
	app.Use(middleware.CorsMiddleware(state))
	app.Use(middleware.AnomalyMiddleware(state))

	opChan := make(chan token.TokenOp, 100)
	go token.StartTokenConsumer(state, opChan)
	if err := token.AutoRegisterAdmin(state, opChan); err != nil {
		log.Fatalf("Failed to auto-register administrator: %v", err)
	}

	go audit.StartAuditLogConsumer(state)

	app.Use(auth.AuthMiddleware(state))

	apiGroup := app.Group("/api")
	auth.SetupAuthRoutes(apiGroup, state, opChan)
	gpg.SetupProfileRoutes(apiGroup, state)
	audit.SetupAuditRoutes(apiGroup.Group("/auth"), state)
	token.SetupTokenRoutes(apiGroup, state, opChan)
	status.SetupRoutes(apiGroup, state)
	status.SetupDebugRoutes(apiGroup)
	api.SetupApiRoutes(apiGroup, state)
	message.SetupRoutes(apiGroup, state)
	upload.SetupChunkedUploadRoutes(apiGroup, state)
	settings.SetupSettingsRoutes(apiGroup.Group("/settings"), state)
	updater.SetupUpdaterRoutes(apiGroup, state)

	storage.HTMLFallback = frontend.ServeIndex
	frontend.SetupFrontendRoutes(app, state)
	javadocs.SetupJavadocRoutes(app, state)
	cargodocs.SetupCargodocRoutes(app, state)
	docker.SetupDockerRoutes(app, state, storage.NewDockerStore(cfg.StoragePath))
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
