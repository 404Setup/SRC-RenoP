/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package updater checks, validates, and applies RenoP application updates.
package updater

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/utils/protohttp"
	"renop/internal/version"
)

const (
	// AutoCheckInterval controls scheduled update checks.
	AutoCheckInterval = 6 * time.Hour
	// AutoCheckInitialDelay lets startup activity settle before the first check.
	AutoCheckInitialDelay = 30 * time.Second
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
	return ParseChannel(cfgVal.Updater.Channel), ParseUpdateMode(cfgVal.Updater.Mode)
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

// RunScheduledCheck performs one configured update check and optional install.
func RunScheduledCheck(ctx context.Context, state *core.AppState) error {
	channel, mode := resolveConfiguredUpdater(state)
	if mode == ModeManual {
		return nil
	}
	checkContext, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	res, err := CheckUpdate(checkContext, channel)
	if err != nil || !res.HasUpdate {
		return err
	}

	updateStateFields(func(s *UpdateState) {
		s.Status = "available"
		s.LatestVersion = strings.Clone(res.LatestVersion)
		s.DownloadURL = strings.Clone(res.DownloadURL)
		s.Size = res.Size
		s.EstimatedDiskSpace = res.EstimatedDiskSpace
		s.ReleaseDate = strings.Clone(res.ReleaseDate)
		s.ReleaseNotes = strings.Clone(res.ReleaseNotes)
		s.CommitSha = strings.Clone(res.CommitSha)
		s.SHA256 = strings.Clone(res.SHA256)
		s.IsRelease = res.IsRelease
	})
	if mode != ModeAutoInstall {
		return nil
	}
	reqSpace := res.EstimatedDiskSpace
	if reqSpace <= 0 {
		reqSpace = 100 * 1024 * 1024
	}
	if CanAllocateDiskSpace != nil && !CanAllocateDiskSpace(uint64(reqSpace)) {
		return nil
	}
	targetPath, err := DownloadAndExtract(checkContext, res.DownloadURL, res.SHA256)
	if err != nil {
		return err
	}
	if err := ApplyUpdateAndRestart(targetPath); err != nil {
		_ = os.Remove(targetPath)
		return err
	}
	return nil
}

func SetupUpdaterRoutes(router fiber.Router, state *core.AppState) {
	CleanOldExecutables()

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
				s.DownloadURL = strings.Clone(res.DownloadURL)
				s.ReleaseNotes = strings.Clone(res.ReleaseNotes)
				s.CommitSha = strings.Clone(res.CommitSha)
				s.SHA256 = strings.Clone(res.SHA256)
			} else {
				s.Status = "idle"
				s.LatestVersion = strings.Clone(version.Version)
				s.DownloadURL = ""
				s.ReleaseNotes = ""
				s.CommitSha = ""
				s.SHA256 = ""
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

		go func() {
			defer isInstalling.Store(false)

			st := GetUpdateState()
			downloadURL := st.DownloadURL
			if downloadURL == "" {
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

			targetPath, err := DownloadAndExtract(context.Background(), downloadURL, st.SHA256)
			if err != nil {
				updateStateFields(func(s *UpdateState) {
					s.Status = "error"
					s.ErrorMessage = err.Error()
				})
			} else {
				SetReadyToRestart(targetPath, st.LatestVersion)
			}
		}()

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
