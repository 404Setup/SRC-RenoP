/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"os"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/proxy"
	"renop/internal/utils"
)

func handleProxy(c fiber.Ctx, state *core.AppState, repo *config.Repository, localFilePath, path, pathStr, storagePath, contentDisposition string) (bool, error) {
	if len(repo.Mirrors) == 0 {
		return false, nil
	}

	for range 3 {
		dl, loaded := state.Inner.InFlightDownloads.LockPath(pathStr)
		if loaded {
			state.Inner.InFlightDownloads.Wait(dl)

			if state.Inner.FileIndex.IsNotFound(pathStr) {
				return true, c.Status(fiber.StatusNotFound).SendString("Not found")
			}

			hasFile := false
			if IsS3Enabled(localFilePath) {
				hasFile = state.Inner.FileIndex.HasFile(pathStr)
			} else {
				_, err := os.Stat(localFilePath)
				hasFile = err == nil && state.Inner.FileIndex.HasFile(pathStr)
			}

			if hasFile {
				c.Set("Content-Security-Policy", "default-src 'none'; sandbox")
				if contentDisposition != "" {
					c.Set(fiber.HeaderContentDisposition, contentDisposition)
				}
				if IsS3Enabled(localFilePath) {
					s3Key := utils.GetS3Key(localFilePath)
					rc, _, err := DownloadFromS3(s3Key)
					if err != nil {
						return true, c.Status(fiber.StatusNotFound).SendString("File not found on S3")
					}
					info, ok := state.Inner.FileIndex.GetFileInfo(pathStr)
					size := -1
					if ok {
						size = int(info.Size)
					}
					return true, c.SendStream(rc, size)
				}
				return true, c.SendFile(localFilePath)
			}
			if !dl.Success {
				return true, c.Status(fiber.StatusNotFound).SendString("Not found")
			}
			continue
		}

		stream, err := proxy.ProxyArtifact(state, repo, path, storagePath, pathStr, dl)
		if err == nil {
			c.Set("Content-Security-Policy", "default-src 'none'; sandbox")
			return true, c.SendStream(stream)
		}
		break
	}
	return false, nil
}
