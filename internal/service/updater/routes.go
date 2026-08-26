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
	if err != nil {
		if notifyErr := deliverUpdateNotificationToManagers(state, updateNoticeCheckFailed, nil); notifyErr != nil {
			log.Printf("[Updater] Failed to deliver scheduled check notification: %v", notifyErr)
		}
		return err
	}
	if !res.HasUpdate {
		return nil
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
	if err := deliverUpdateNotificationToManagers(state, updateNoticeAvailable, res); err != nil {
		return err
	}
	if mode != ModeAutoInstall {
		return nil
	}
	reqSpace := res.EstimatedDiskSpace
	if reqSpace <= 0 {
		reqSpace = 100 * 1024 * 1024
	}
	if CanAllocateDiskSpace != nil && !CanAllocateDiskSpace(uint64(reqSpace)) {
		if notifyErr := deliverUpdateNotificationToManagers(state, updateNoticeInsufficientSpace, res); notifyErr != nil {
			log.Printf("[Updater] Failed to deliver insufficient-space notification: %v", notifyErr)
		}
		return nil
	}
	targetPath, err := DownloadAndExtract(checkContext, res.DownloadURL, res.SHA256)
	if err != nil {
		if notifyErr := deliverUpdateNotificationToManagers(state, updateNoticeInstallFailed, res); notifyErr != nil {
			log.Printf("[Updater] Failed to deliver update failure notification: %v", notifyErr)
		}
		return err
	}
	if err := ApplyUpdateAndRestart(targetPath); err != nil {
		_ = os.Remove(targetPath)
		if notifyErr := deliverUpdateNotificationToManagers(state, updateNoticeRestartFailed, res); notifyErr != nil {
			log.Printf("[Updater] Failed to deliver restart failure notification: %v", notifyErr)
		}
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
			return WriteAPIError(c, fiber.StatusForbidden, APIErrorForbidden, "Updater access is forbidden")
		}

		channel := resolveCheckChannel(c.Query("channel"), state)
		res, err := CheckUpdate(c.Context(), channel)
		if err != nil {
			log.Printf("[Updater] Manual update check failed: %v", err)
			if notifyErr := deliverUpdateNotification(state, user.Username, updateNoticeCheckFailed, nil); notifyErr != nil {
				log.Printf("[Updater] Failed to deliver check failure notification: %v", notifyErr)
			}
			return WriteAPIError(c, fiber.StatusInternalServerError, APIErrorCheckFailed, "Update check failed")
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
		event := updateNoticeCurrent
		if res.HasUpdate {
			event = updateNoticeAvailable
		}
		if err := deliverUpdateNotification(state, user.Username, event, res); err != nil {
			log.Printf("[Updater] Failed to deliver manual update notification: %v", err)
			return WriteAPIError(c, fiber.StatusInternalServerError, APIErrorNotificationFailed,
				"Failed to deliver update notification")
		}

		return c.JSON(res)
	})

	api.Post("/install", func(c fiber.Ctx) error {
		user := auth.GetUser(c)
		if !user.IsManager() {
			return WriteAPIError(c, fiber.StatusForbidden, APIErrorForbidden, "Updater access is forbidden")
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
			if notifyErr := deliverUpdateNotification(state, user.Username, updateNoticeInsufficientSpace, nil); notifyErr != nil {
				log.Printf("[Updater] Failed to deliver insufficient-space notification: %v", notifyErr)
			}
			return WriteAPIError(c, fiber.StatusInsufficientStorage, APIErrorInsufficientSpace,
				"Insufficient disk space to download update package")
		}

		if !isInstalling.CompareAndSwap(false, true) {
			return WriteAPIError(c, fiber.StatusConflict, APIErrorInstallBusy, "Installation already in progress")
		}
		recipient := strings.Clone(user.Username)

		go func() {
			defer isInstalling.Store(false)

			st := GetUpdateState()
			downloadURL := st.DownloadURL
			if downloadURL == "" {
				updateStateFields(func(s *UpdateState) {
					s.Status = "error"
					s.ErrorMessage = "No download URL for the current platform"
				})
				if notifyErr := deliverUpdateNotification(state, recipient, updateNoticeInstallFailed, nil); notifyErr != nil {
					log.Printf("[Updater] Failed to deliver update failure notification: %v", notifyErr)
				}
				return
			}

			updateStateFields(func(s *UpdateState) {
				s.Status = "downloading"
				s.Progress = 10
			})

			targetPath, err := DownloadAndExtract(context.Background(), downloadURL, st.SHA256)
			if err != nil {
				_, _, publicError := PackageAPIError(err)
				updateStateFields(func(s *UpdateState) {
					s.Status = "error"
					s.ErrorMessage = publicError
				})
				if notifyErr := deliverUpdateNotification(state, recipient, updateNoticeInstallFailed, nil); notifyErr != nil {
					log.Printf("[Updater] Failed to deliver update failure notification: %v", notifyErr)
				}
			} else {
				SetReadyToRestart(targetPath, st.LatestVersion)
			}
		}()

		return c.JSON(fiber.Map{"status": "started"})
	})

	api.Post("/upload", func(c fiber.Ctx) error {
		user := auth.GetUser(c)
		if !user.IsManager() {
			return WriteAPIError(c, fiber.StatusForbidden, APIErrorForbidden, "Updater access is forbidden")
		}

		contentLength := c.Request().Header.ContentLength()
		if contentLength > 0 {
			reqSpace := uint64(EstimateUploadedPackageDiskSpace("package.br", int64(contentLength)))
			if CanAllocateDiskSpace != nil && !CanAllocateDiskSpace(reqSpace) {
				updateStateFields(func(s *UpdateState) {
					s.Status = "error"
					s.ErrorMessage = "Insufficient disk space to upload update package"
				})
				return WriteAPIError(c, fiber.StatusInsufficientStorage, APIErrorInsufficientSpace,
					"Insufficient disk space to upload update package")
			}
		}

		file, err := c.FormFile("file")
		if err != nil {
			file, err = c.FormFile("package")
			if err != nil {
				return WriteAPIError(c, fiber.StatusBadRequest, APIErrorMissingFile, "No update package file uploaded")
			}
		}

		reqSpace := EstimateUploadedPackageDiskSpace(file.Filename, file.Size)
		if CanAllocateDiskSpace != nil && !CanAllocateDiskSpace(uint64(reqSpace)) {
			updateStateFields(func(s *UpdateState) {
				s.Status = "error"
				s.ErrorMessage = "Insufficient disk space to upload update package"
			})
			return WriteAPIError(c, fiber.StatusInsufficientStorage, APIErrorInsufficientSpace,
				"Insufficient disk space to upload update package")
		}

		if !isInstalling.CompareAndSwap(false, true) {
			return WriteAPIError(c, fiber.StatusConflict, APIErrorInstallBusy, "Installation already in progress")
		}
		defer isInstalling.Store(false)

		if !IsSupportedUpdatePackageName(file.Filename) {
			return WriteAPIError(c, fiber.StatusBadRequest, APIErrorInvalidPackage,
				"Uploaded file must be a .br or .zip package")
		}

		updateStateFields(func(s *UpdateState) {
			s.Status = "downloading"
			s.Progress = 50
		})

		targetPath, err := SaveAndExtractUploadedPackage(file)
		if err != nil {
			_, _, publicError := PackageAPIError(err)
			updateStateFields(func(s *UpdateState) {
				s.Status = "error"
				s.ErrorMessage = publicError
			})
			log.Printf("[Updater] Failed to process offline update package: %v", err)
			return WritePackageAPIError(c, err)
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
			return WriteAPIError(c, fiber.StatusForbidden, APIErrorForbidden, "Updater access is forbidden")
		}

		pending := pendingBinary.get()
		if pending != "" {
			log.Print("[Updater] Restarting application to apply update...")
			if err := ApplyUpdateAndRestart(pending); err != nil {
				log.Printf("[Updater] Failed to apply update and restart: %v", err)
				if notifyErr := deliverUpdateNotification(state, user.Username, updateNoticeRestartFailed, nil); notifyErr != nil {
					log.Printf("[Updater] Failed to deliver restart failure notification: %v", notifyErr)
				}
				return WriteAPIError(c, fiber.StatusInternalServerError, APIErrorRestartFailed,
					"Failed to restart the service")
			}
			return c.JSON(fiber.Map{"status": "restarting"})
		}

		log.Print("[Updater] Restarting application...")
		if err := RestartProcess(); err != nil {
			log.Printf("[Updater] Failed to restart process: %v", err)
			if notifyErr := deliverUpdateNotification(state, user.Username, updateNoticeRestartFailed, nil); notifyErr != nil {
				log.Printf("[Updater] Failed to deliver restart failure notification: %v", notifyErr)
			}
			return WriteAPIError(c, fiber.StatusInternalServerError, APIErrorRestartFailed,
				"Failed to restart the service")
		}
		return c.JSON(fiber.Map{"status": "restarting"})
	})
}
