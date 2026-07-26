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
	"renop/utils/protohttp"
	"renop/version"
)

var (
	lastAutoCheck       time.Time
	autoCheckLock       sync.Mutex
	autoCheckInterval   = 6 * time.Hour
	autoCheckWorkerOnce sync.Once
)

func StartAutoCheckTicker() {
	autoCheckWorkerOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(6 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				TriggerAutoCheck(ChannelRelease, ModeManual)
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
			s.LatestVersion = res.LatestVersion
			s.DownloadUrl = res.DownloadUrl
			s.Size = res.Size
			s.EstimatedDiskSpace = res.EstimatedDiskSpace
			s.ReleaseDate = res.ReleaseDate
			s.ReleaseNotes = res.ReleaseNotes
			s.CommitSha = res.CommitSha
			s.IsRelease = res.IsRelease
		})

		if mode == ModeAutoInstall {
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
		}
	})
}

func SetupUpdaterRoutes(router fiber.Router) {
	CleanOldExecutables()
	StartAutoCheckTicker()

	api := router.Group("/updater")

	api.Get("/status", func(c fiber.Ctx) error {
		return protohttp.Write(c, ToPbUpdateState(GetUpdateState()))
	})

	api.Post("/check", func(c fiber.Ctx) error {
		user := auth.GetUser(c)
		if !user.IsManager() {
			return c.Status(fiber.StatusForbidden).SendString("Forbidden")
		}

		channel := Channel(c.Query("channel", string(ChannelRelease)))
		res, err := CheckUpdate(c.Context(), channel)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		updateStateFields(func(s *UpdateState) {
			s.Size = res.Size
			s.EstimatedDiskSpace = res.EstimatedDiskSpace
			s.ReleaseDate = res.ReleaseDate
			s.ReleaseNotes = res.ReleaseNotes
			s.CommitSha = res.CommitSha
			s.IsRelease = res.IsRelease
			if res.HasUpdate {
				s.Status = "available"
				s.LatestVersion = res.LatestVersion
				s.DownloadUrl = res.DownloadUrl
			} else {
				s.Status = "idle"
				s.LatestVersion = version.Version
			}
		})

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
				downloadUrl = "https://nightly.link/404Setup/SRC-RenoP/workflows/build/main/renop-nightly.zip"
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
		if pending == "" {
			return c.Status(fiber.StatusBadRequest).SendString("No update ready to install")
		}

		log.Print("[Updater] Restarting application to apply update...")
		err := ApplyUpdateAndRestart(pending)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		return c.JSON(fiber.Map{"status": "restarting"})
	})
}
