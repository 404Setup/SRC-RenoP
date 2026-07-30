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
		if state.Inner.FileCache != nil && fileSize > 0 && fileSize <= 32*1024 && isCacheableMetadata(pathStr) {
			rc, _, err := DownloadFromS3(s3Key)
			if err == nil {
				data, readErr := io.ReadAll(io.LimitReader(rc, fileSize+1))
				_ = rc.Close()
				if readErr == nil && int64(len(data)) == fileSize {
					var etagVal, lmVal string
					if etagHeader != nil {
						etagVal = *etagHeader
					}
					if lastModifiedHeader != nil {
						lmVal = *lastModifiedHeader
					}
					state.Inner.FileCache.Set(pathStr, encodeCacheEntry(etagVal, lmVal, data))
					return c.Send(data)
				}
			}
		}
		rc, _, err := DownloadFromS3(s3Key)
		if err == nil {
			if fileSize > 0 {
				return c.SendStream(rc, int(fileSize))
			}
			return c.SendStream(rc)
		}
	}

	if state.Inner.FileCache != nil && fileSize > 0 && fileSize <= 32*1024 && isCacheableMetadata(pathStr) {
		data, err := os.ReadFile(localFilePath)
		if err == nil && int64(len(data)) == fileSize {
			var etagVal, lmVal string
			if etagHeader != nil {
				etagVal = *etagHeader
			}
			if lastModifiedHeader != nil {
				lmVal = *lastModifiedHeader
			}
			state.Inner.FileCache.Set(pathStr, encodeCacheEntry(etagVal, lmVal, data))
			return c.Send(data)
		}
	}

	f, err := os.Open(localFilePath)
	if err != nil {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}
	if fileSize <= 0 {
		if st, stErr := f.Stat(); stErr == nil {
			fileSize = st.Size()
		}
	}
	if reqRange := c.Get(fiber.HeaderRange); reqRange != "" && fileSize > 0 {
		start, end, ok := utils.ParseRange(reqRange, uint64(fileSize))
		if !ok {
			_ = f.Close()
			c.Set(fiber.HeaderContentRange, "bytes */"+strconv.FormatInt(fileSize, 10))
			return c.Status(fiber.StatusRequestedRangeNotSatisfiable).SendString("")
		}
		c.Set(fiber.HeaderContentRange, "bytes "+strconv.FormatUint(start, 10)+"-"+strconv.FormatUint(end, 10)+"/"+strconv.FormatInt(fileSize, 10))
		c.Set(fiber.HeaderAcceptRanges, "bytes")
		c.Status(fiber.StatusPartialContent)
		length := int64(end - start + 1)
		return c.SendStream(&rangeReadCloser{
			Reader: io.NewSectionReader(f, int64(start), length),
			Closer: f,
		}, int(length))
	}
	if fileSize > 0 {
		return c.SendStream(f, int(fileSize))
	}
	return c.SendStream(f)
}

// rangeReadCloser pairs a SectionReader with the underlying *os.File closer.
type rangeReadCloser struct {
	io.Reader
	io.Closer
}

func isCacheableMetadata(pathStr string) bool {
	ext := strings.ToLower(filepath.Ext(pathStr))
	return ext == ".pom" || ext == ".xml" || ext == ".json" || ext == ".sha1" || ext == ".md5"
}
