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
	"net/http"
	"strconv"
	"strings"

	"github.com/3JoB/unsafeConvert"
	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/index"
	"renop/internal/utils"
)

func CheckNotModified(c fiber.Ctx, etag *string, lastModified *string) bool {
	if reqEtag := c.Get(fiber.HeaderIfNoneMatch); reqEtag != "" {
		if etag == nil {
			return false
		}

		for candidate := range strings.SplitSeq(reqEtag, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "*" || candidate == *etag || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(*etag, "W/") {
				return true
			}
		}
		return false
	}

	if reqModified := c.Get(fiber.HeaderIfModifiedSince); reqModified != "" {
		if lastModified != nil {
			requestTime, requestErr := http.ParseTime(reqModified)
			modifiedTime, modifiedErr := http.ParseTime(*lastModified)
			if requestErr == nil && modifiedErr == nil && !modifiedTime.After(requestTime) {
				return true
			}
		}
	}

	return false
}

func encodeCacheEntry(etag string, lastModified string, data []byte) []byte {
	etagLen := len(etag)
	lmLen := len(lastModified)

	buf := make([]byte, 1+etagLen+1+lmLen+len(data))
	buf[0] = byte(etagLen)
	copy(buf[1:], etag)
	buf[1+etagLen] = byte(lmLen)
	copy(buf[1+etagLen+1:], lastModified)
	copy(buf[1+etagLen+1+lmLen:], data)
	return buf
}

func decodeCacheEntry(buf []byte) (string, string, []byte, bool) {
	if len(buf) < 2 {
		return "", "", nil, false
	}
	etagLen := int(buf[0])
	if len(buf) < 1+etagLen+1 {
		return "", "", nil, false
	}
	etag := unsafeConvert.StringPointer(buf[1 : 1+etagLen])

	lmLen := int(buf[1+etagLen])
	if len(buf) < 1+etagLen+1+lmLen {
		return "", "", nil, false
	}
	lastModified := unsafeConvert.StringPointer(buf[1+etagLen+1 : 1+etagLen+1+lmLen])
	data := buf[1+etagLen+1+lmLen:]
	return etag, lastModified, data, true
}

func serveFromCache(c fiber.Ctx, state *core.AppState, pathStr string, contentDisposition string, info index.FileInfo) (bool, error) {
	if state.Inner.FileCache == nil {
		return false, nil
	}

	cachedBuf, err := state.Inner.FileCache.GetReadOnlyView(pathStr)
	if err != nil {
		return false, nil
	}

	etagVal, lastModifiedVal, cachedData, ok := decodeCacheEntry(cachedBuf)
	if !ok {
		state.Inner.FileCache.Delete(pathStr)
		return false, nil
	}
	expectedEtag := "W/\"" + strconv.FormatInt(info.Size, 16) + "-" + strconv.FormatInt(info.ModTime, 16) + "\""
	if etagVal != expectedEtag {
		state.Inner.FileCache.Delete(pathStr)
		return false, nil
	}

	var etagPtr, lmPtr *string
	if etagVal != "" {
		etagPtr = &etagVal
	}
	if lastModifiedVal != "" {
		lmPtr = &lastModifiedVal
	}

	if CheckNotModified(c, etagPtr, lmPtr) {
		return true, c.Status(fiber.StatusNotModified).SendString("")
	}

	if etagPtr != nil {
		c.Set(fiber.HeaderETag, *etagPtr)
	}
	if lmPtr != nil {
		c.Set(fiber.HeaderLastModified, *lmPtr)
	}
	setArtifactContentType(c, pathStr)

	fileSize := uint64(len(cachedData))

	reqRange := c.Get(fiber.HeaderRange)
	if reqRange != "" {
		start, end, ok := utils.ParseRange(reqRange, fileSize)
		if !ok {
			c.Set(fiber.HeaderContentRange, "bytes */"+strconv.FormatUint(fileSize, 10))
			return true, c.Status(fiber.StatusRequestedRangeNotSatisfiable).SendString("")
		}

		c.Set(fiber.HeaderContentRange, "bytes "+strconv.FormatUint(start, 10)+"-"+strconv.FormatUint(end, 10)+"/"+strconv.FormatUint(fileSize, 10))
		c.Set(fiber.HeaderAcceptRanges, "bytes")
		c.Set(fiber.HeaderContentDisposition, contentDisposition)
		c.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "SAMEORIGIN")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Status(fiber.StatusPartialContent)
		return true, c.Send(cachedData[start : end+1])
	}

	c.Set(fiber.HeaderContentDisposition, contentDisposition)
	c.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("X-Frame-Options", "SAMEORIGIN")
	c.Set("X-XSS-Protection", "1; mode=block")
	return true, c.Send(cachedData)
}
