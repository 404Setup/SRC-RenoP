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
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/core"
	"renop/utils"
)

func serveLocalFile(c fiber.Ctx, state *core.AppState, localFilePath, pathStr, contentDisposition string, fileSize int64, isDir bool, etagHeader, lastModifiedHeader *string) error {
	if isDir {
		var files strings.Builder
		children := state.Inner.FileIndex.GetChildren(localFilePath)
		if len(children) == 0 && !IsS3Enabled(localFilePath) {
			entries, err := os.ReadDir(localFilePath)
			if err == nil {
				for _, entry := range entries {
					files.WriteString(entry.Name())
					files.WriteByte('\n')
				}
			}
		} else {
			for _, child := range children {
				files.WriteString(child)
				files.WriteByte('\n')
			}
		}
		c.Set(fiber.HeaderContentType, "text/plain")
		c.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "SAMEORIGIN")
		c.Set("X-XSS-Protection", "1; mode=block")
		return c.SendString(files.String())
	}

	c.Set(fiber.HeaderContentDisposition, contentDisposition)
	setArtifactContentType(c, localFilePath)
	c.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("X-Frame-Options", "SAMEORIGIN")
	c.Set("X-XSS-Protection", "1; mode=block")
	if etagHeader != nil {
		c.Set(fiber.HeaderETag, *etagHeader)
	}
	if lastModifiedHeader != nil {
		c.Set(fiber.HeaderLastModified, *lastModifiedHeader)
	}

	if IsS3Enabled(localFilePath) {
		s3Key := utils.GetS3Key(localFilePath)
		s3Cfg := GetS3ConfigForPath(localFilePath)
		if s3Cfg != nil && s3Cfg.RedirectDownloads {
			presignedUrl, err := GetS3PresignedURL(s3Key, 15*time.Minute)
			if err == nil {
				return c.Redirect().To(presignedUrl)
			}
		}
		if reqRange := c.Get(fiber.HeaderRange); reqRange != "" {
			start, end, ok := utils.ParseRange(reqRange, uint64(fileSize))
			if !ok {
				c.Set(fiber.HeaderContentRange, "bytes */"+strconv.FormatInt(fileSize, 10))
				return c.Status(fiber.StatusRequestedRangeNotSatisfiable).SendString("")
			}
			rc, err := DownloadRangeFromS3(s3Key, int64(start), int64(end))
			if err != nil {
				return c.Status(fiber.StatusNotFound).SendString("Not found")
			}
			c.Set(fiber.HeaderContentRange, "bytes "+strconv.FormatUint(start, 10)+"-"+strconv.FormatUint(end, 10)+"/"+strconv.FormatInt(fileSize, 10))
			c.Set(fiber.HeaderAcceptRanges, "bytes")
			c.Status(fiber.StatusPartialContent)
			return c.SendStream(rc, int(end-start+1))
		}
		if fileSize > 0 && fileSize <= 128*1024 {
			rc, _, err := DownloadFromS3(s3Key)
			if err == nil {
				defer rc.Close()
				data, readErr := io.ReadAll(rc)
				if readErr == nil {
					if state.Inner.FileCache != nil && fileSize <= 32*1024 && isCacheableMetadata(pathStr) {
						var etagVal, lmVal string
						if etagHeader != nil {
							etagVal = *etagHeader
						}
						if lastModifiedHeader != nil {
							lmVal = *lastModifiedHeader
						}
						state.Inner.FileCache.Set(pathStr, encodeCacheEntry(etagVal, lmVal, data))
					}
					reqRange := c.Get(fiber.HeaderRange)
					if reqRange == "" {
						return c.Send(data)
					}
					start, end, ok := utils.ParseRange(reqRange, uint64(fileSize))
					if !ok {
						c.Set(fiber.HeaderContentRange, "bytes */"+strconv.FormatInt(fileSize, 10))
						return c.Status(fiber.StatusRequestedRangeNotSatisfiable).SendString("")
					}
					c.Set(fiber.HeaderContentRange, "bytes "+strconv.FormatUint(start, 10)+"-"+strconv.FormatUint(end, 10)+"/"+strconv.FormatInt(fileSize, 10))
					c.Set(fiber.HeaderAcceptRanges, "bytes")
					c.Status(fiber.StatusPartialContent)
					return c.Send(data[start : end+1])
				}
			}
		}
		rc, _, err := DownloadFromS3(s3Key)
		if err == nil {
			return c.SendStream(rc, int(fileSize))
		}
	}

	if fileSize > 0 && fileSize <= 128*1024 {
		data, err := os.ReadFile(localFilePath)
		if err == nil {
			if state.Inner.FileCache != nil && fileSize <= 32*1024 && isCacheableMetadata(pathStr) {
				var etagVal, lmVal string
				if etagHeader != nil {
					etagVal = *etagHeader
				}
				if lastModifiedHeader != nil {
					lmVal = *lastModifiedHeader
				}
				state.Inner.FileCache.Set(pathStr, encodeCacheEntry(etagVal, lmVal, data))
			}
			reqRange := c.Get(fiber.HeaderRange)
			if reqRange == "" {
				return c.Send(data)
			}
			start, end, ok := utils.ParseRange(reqRange, uint64(fileSize))
			if !ok {
				c.Set(fiber.HeaderContentRange, "bytes */"+strconv.FormatInt(fileSize, 10))
				return c.Status(fiber.StatusRequestedRangeNotSatisfiable).SendString("")
			}
			c.Set(fiber.HeaderContentRange, "bytes "+strconv.FormatUint(start, 10)+"-"+strconv.FormatUint(end, 10)+"/"+strconv.FormatInt(fileSize, 10))
			c.Set(fiber.HeaderAcceptRanges, "bytes")
			c.Status(fiber.StatusPartialContent)
			return c.Send(data[start : end+1])
		}
	}

	return c.SendFile(localFilePath)
}

func isCacheableMetadata(pathStr string) bool {
	ext := strings.ToLower(filepath.Ext(pathStr))
	return ext == ".pom" || ext == ".xml" || ext == ".json" || ext == ".sha1" || ext == ".md5"
}
