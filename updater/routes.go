/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package updater

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/panjf2000/ants/v2"

	"renop/auth"
	"renop/config"
	"renop/core"
	"renop/utils"
	"renop/utils/protohttp"
	"renop/version"
)

var (
	lastAutoCheck       time.Time
	autoCheckLock       sync.Mutex
	autoCheckInterval   = 6 * time.Hour
	autoCheckWorkerOnce sync.Once
)

// resolveConfiguredUpdater returns the channel and mode from live config.
func resolveConfiguredUpdater(state *core.AppState) (Channel, UpdateMode) {
	if state == nil || state.Inner == nil || state.Inner.Config == nil {
		return ChannelRelease, ModeManual
	}
	cfgVal := state.Inner.Config.Load()
	if cfgVal == nil {
		return ChannelRelease, ModeManual
	}
	cfg, ok := cfgVal.(*config.Config)
	if !ok || cfg == nil {
		return ChannelRelease, ModeManual
	}
	return ParseChannel(cfg.Updater.Channel), ParseUpdateMode(cfg.Updater.Mode)
}

func resolveCheckChannel(query string, state *core.AppState) Channel {
	q := strings.TrimSpace(query)
	if q != "" {
		switch Channel(strings.ToLower(q)) {
		case ChannelNightly:
			return ChannelNightly
		case ChannelRelease:
			return ChannelRelease
		}
	}
	ch, _ := resolveConfiguredUpdater(state)
	return ch
}

func StartAutoCheckTicker(state *core.AppState) {
	autoCheckWorkerOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(autoCheckInterval)
			defer ticker.Stop()
			time.Sleep(30 * time.Second)
			ch, mode := resolveConfiguredUpdater(state)
			TriggerAutoCheck(ch, mode)
			for range ticker.C {
				ch, mode := resolveConfiguredUpdater(state)
				TriggerAutoCheck(ch, mode)
			}
		}()
	})
}

func TriggerAutoCheck(channel Channel, mode UpdateMode) {
	if mode == ModeManual {
		return
	}
	autoCheckLock.Lock()
	if time.Since(lastAutoCheck) < autoCheckInterval {
		autoCheckLock.Unlock()
		return
	}
	lastAutoCheck = time.Now()
	autoCheckLock.Unlock()

	_ = ants.Submit(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		res, err := CheckUpdate(ctx, channel)
		if err != nil || !res.HasUpdate {
			return
		}

		updateStateFields(func(s *UpdateState) {
			s.Status = "available"
			s.LatestVersion = strings.Clone(res.LatestVersion)
			s.DownloadUrl = strings.Clone(res.DownloadUrl)
			s.Size = res.Size
			s.EstimatedDiskSpace = res.EstimatedDiskSpace
			s.ReleaseDate = strings.Clone(res.ReleaseDate)
			s.ReleaseNotes = strings.Clone(res.ReleaseNotes)
			s.CommitSha = strings.Clone(res.CommitSha)
			s.IsRelease = res.IsRelease
		})

		utils.ReleaseMemoryToOS()

		if mode != ModeAutoInstall {
			return
		}
		reqSpace := res.EstimatedDiskSpace
		if reqSpace <= 0 {
			reqSpace = 100 * 1024 * 1024
		}
		if CanAllocateDiskSpace != nil && !CanAllocateDiskSpace(uint64(reqSpace)) {
			return
		}
		targetPath, err := DownloadAndExtract(ctx, res.DownloadUrl)
		if err != nil {
			return
		}
		if applyErr := ApplyUpdateAndRestart(targetPath); applyErr != nil {
			_ = os.Remove(targetPath)
		}
	})
}

func SetupUpdaterRoutes(router fiber.Router, state *core.AppState) {
	CleanOldExecutables()
	StartAutoCheckTicker(state)

	api := router.Group("/updater")

	api.Get("/status", func(c fiber.Ctx) error {
		return protohttp.Write(c, ToPbUpdateState(GetUpdateState()))
	})

	api.Post("/check", func(c fiber.Ctx) error {
		user := auth.GetUser(c)
		if !user.IsManager() {
			return c.Status(fiber.StatusForbidden).SendString("Forbidden")
		}

		channel := resolveCheckChannel(c.Query("channel"), state)
		res, err := CheckUpdate(c.Context(), channel)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		updateStateFields(func(s *UpdateState) {
			s.Size = res.Size
			s.EstimatedDiskSpace = res.EstimatedDiskSpace
			s.ReleaseDate = strings.Clone(res.ReleaseDate)
			s.IsRelease = res.IsRelease
			if res.HasUpdate {
				s.Status = "available"
				s.LatestVersion = strings.Clone(res.LatestVersion)
				s.DownloadUrl = strings.Clone(res.DownloadUrl)
				s.ReleaseNotes = strings.Clone(res.ReleaseNotes)
				s.CommitSha = strings.Clone(res.CommitSha)
			} else {
				s.Status = "idle"
				s.LatestVersion = strings.Clone(version.Version)
				s.DownloadUrl = ""
				s.ReleaseNotes = ""
				s.CommitSha = ""
			}
		})

		utils.ReleaseMemoryToOS()

		return c.JSON(res)
	})

	api.Post("/install", func(c fiber.Ctx) error {
		user := auth.GetUser(c)
		if !user.IsManager() {
			return c.Status(fiber.StatusForbidden).SendString("Forbidden")
		}

		st := GetUpdateState()
		reqSpace := st.EstimatedDiskSpace
		if reqSpace <= 0 {
			reqSpace = 100 * 1024 * 1024
		}
		if CanAllocateDiskSpace != nil && !CanAllocateDiskSpace(uint64(reqSpace)) {
			updateStateFields(func(s *UpdateState) {
				s.Status = "error"
				s.ErrorMessage = "Insufficient disk space to download update package"
			})
			return c.Status(fiber.StatusInsufficientStorage).JSON(fiber.Map{"error": "Insufficient disk space to download update package"})
		}

		if !isInstalling.CompareAndSwap(false, true) {
			return c.Status(fiber.StatusConflict).SendString("Installation already in progress")
		}

		err := ants.Submit(func() {
			defer isInstalling.Store(false)

			st := GetUpdateState()
			downloadUrl := st.DownloadUrl
			if downloadUrl == "" {
				updateStateFields(func(s *UpdateState) {
					s.Status = "error"
					s.ErrorMessage = "No download URL for the current platform"
				})
				return
			}

			updateStateFields(func(s *UpdateState) {
				s.Status = "downloading"
				s.Progress = 10
			})

			targetPath, err := DownloadAndExtract(context.Background(), downloadUrl)
			if err != nil {
				updateStateFields(func(s *UpdateState) {
					s.Status = "error"
					s.ErrorMessage = err.Error()
				})
			} else {
				SetReadyToRestart(targetPath, st.LatestVersion)
			}
		})

		if err != nil {
			isInstalling.Store(false)
			return c.Status(fiber.StatusServiceUnavailable).SendString("Task submission failed")
		}

		return c.JSON(fiber.Map{"status": "started"})
	})

	api.Post("/upload", func(c fiber.Ctx) error {
		user := auth.GetUser(c)
		if !user.IsManager() {
			return c.Status(fiber.StatusForbidden).SendString("Forbidden")
		}

		contentLength := c.Request().Header.ContentLength()
		if contentLength > 0 {
			reqSpace := uint64(contentLength) * 3
			if CanAllocateDiskSpace != nil && !CanAllocateDiskSpace(reqSpace) {
				updateStateFields(func(s *UpdateState) {
					s.Status = "error"
					s.ErrorMessage = "Insufficient disk space to upload update package"
				})
				return c.Status(fiber.StatusInsufficientStorage).JSON(fiber.Map{"error": "Insufficient disk space to upload update package"})
			}
		}

		file, err := c.FormFile("file")
		if err != nil {
			file, err = c.FormFile("package")
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "No update package file uploaded"})
			}
		}

		reqSpace := file.Size * 3
		if reqSpace <= 0 {
			reqSpace = 100 * 1024 * 1024
		}
		if CanAllocateDiskSpace != nil && !CanAllocateDiskSpace(uint64(reqSpace)) {
			updateStateFields(func(s *UpdateState) {
				s.Status = "error"
				s.ErrorMessage = "Insufficient disk space to upload update package"
			})
			return c.Status(fiber.StatusInsufficientStorage).JSON(fiber.Map{"error": "Insufficient disk space to upload update package"})
		}

		if !isInstalling.CompareAndSwap(false, true) {
			return c.Status(fiber.StatusConflict).SendString("Installation already in progress")
		}
		defer isInstalling.Store(false)

		if !strings.HasSuffix(strings.ToLower(file.Filename), ".zip") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Uploaded file must be a .zip package"})
		}

		updateStateFields(func(s *UpdateState) {
			s.Status = "downloading"
			s.Progress = 50
		})

		targetPath, err := SaveAndExtractUploadedZip(file)
		if err != nil {
			updateStateFields(func(s *UpdateState) {
				s.Status = "error"
				s.ErrorMessage = err.Error()
			})
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		SetReadyToRestart(targetPath, "offline")

		return c.JSON(fiber.Map{
			"status":  "ready_to_restart",
			"message": "Offline update installed successfully",
		})
	})

	api.Post("/restart", func(c fiber.Ctx) error {
		user := auth.GetUser(c)
		if !user.IsManager() {
			return c.Status(fiber.StatusForbidden).SendString("Forbidden")
		}

		pending := pendingBinary.get()
		if pending != "" {
			log.Print("[Updater] Restarting application to apply update...")
			if err := ApplyUpdateAndRestart(pending); err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
			}
			return c.JSON(fiber.Map{"status": "restarting"})
		}

		log.Print("[Updater] Restarting application...")
		if err := RestartProcess(); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}
		return c.JSON(fiber.Map{"status": "restarting"})
	})
}
