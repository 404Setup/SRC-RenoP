/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package frontend

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	syncv2 "sync/v2"

	"github.com/gofiber/fiber/v3"

	"renop/internal/utils"
)

var (
	embeddedFileCache syncv2.Map[string, *embeddedFile]
)

type embeddedFile struct {
	data        []byte
	etag        string
	contentType string
}

// cacheEmbeddedFile stores an embed payload under its public URL path so later
// ServeEmbeddedFile hits avoid a second embed.FS.ReadFile of large bundles.
func cacheEmbeddedFile(publicPath string, data []byte) *embeddedFile {
	hasher := sha256.New()
	_, _ = hasher.Write(data)
	candidate := &embeddedFile{
		data:        data,
		etag:        `W/"` + hex.EncodeToString(hasher.Sum(nil))[:16] + `"`,
		contentType: utils.ContentTypeByExt(filepath.Ext(publicPath)),
	}
	actual, _ := embeddedFileCache.LoadOrStore(publicPath, candidate)
	return actual
}

func ServeEmbeddedFile(c fiber.Ctx, path string) error {
	var file *embeddedFile
	if cached, ok := embeddedFileCache.Load(path); ok {
		file = cached
	} else {
		data, err := readAsset(path)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		file = cacheEmbeddedFile(path, data)
	}

	if file == nil {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	c.Set(fiber.HeaderContentType, file.contentType)
	c.Set(fiber.HeaderETag, file.etag)

	if clientETag := c.Get(fiber.HeaderIfNoneMatch); clientETag != "" && clientETag == file.etag {
		return c.SendStatus(fiber.StatusNotModified)
	}

	c.Set(fiber.HeaderCacheControl, "no-cache, must-revalidate")

	return c.Send(file.data)
}
