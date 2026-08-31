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
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	syncv2 "sync/v2"

	"github.com/gofiber/fiber/v3"

	"renop/internal/utils"
)

var (
	embeddedFileCache syncv2.Map[string, *embeddedFile]
)

const frontendAssetCacheControl = "no-cache, must-revalidate, max-age=0"

type embeddedFile struct {
	data            []byte
	etag            string
	contentType     string
	contentEncoding string
}

type embeddedAssetEncoding struct {
	name    string
	suffix  string
	quality float64
}

var embeddedAssetEncodings = [...]embeddedAssetEncoding{
	{name: "br", suffix: ".br"},
	{name: "zstd", suffix: ".zst"},
	{name: "gzip", suffix: ".gz"},
	{name: "deflate", suffix: ".deflate"},
}

// cacheEmbeddedFile stores an embed payload under its public URL path so later
// ServeEmbeddedFile hits avoid a second embed.FS.ReadFile of large bundles.
func cacheEmbeddedFile(publicPath string, data []byte) *embeddedFile {
	return cacheEmbeddedRepresentation(publicPath, publicPath, data, "")
}

func cacheEmbeddedRepresentation(cacheKey, publicPath string, data []byte, encoding string) *embeddedFile {
	hasher := sha256.New()
	_, _ = hasher.Write(data)
	candidate := &embeddedFile{
		data:            data,
		etag:            `W/"` + hex.EncodeToString(hasher.Sum(nil))[:16] + `"`,
		contentType:     utils.ContentTypeByExt(filepath.Ext(publicPath)),
		contentEncoding: encoding,
	}
	actual, _ := embeddedFileCache.LoadOrStore(cacheKey, candidate)
	return actual
}

func isPrecompressedAssetPath(path string) bool {
	lower := strings.ToLower(path)
	for _, encoding := range embeddedAssetEncodings {
		if strings.HasSuffix(lower, encoding.suffix) {
			return true
		}
	}
	return false
}

func preferredAssetEncodings(header string) ([]embeddedAssetEncoding, bool) {
	if strings.TrimSpace(header) == "" {
		return nil, true
	}
	qualities := make(map[string]float64, len(embeddedAssetEncodings)+2)
	wildcard := -1.0
	for token := range strings.SplitSeq(header, ",") {
		parts := strings.Split(token, ";")
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		if name == "" {
			continue
		}
		quality := 1.0
		for _, parameter := range parts[1:] {
			key, value, ok := strings.Cut(parameter, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "q") {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil || parsed < 0 || parsed > 1 {
				quality = 0
			} else {
				quality = parsed
			}
		}
		if name == "*" {
			wildcard = max(wildcard, quality)
		} else {
			qualities[name] = max(qualities[name], quality)
		}
	}

	identityQuality := 1.0
	if quality, exists := qualities["identity"]; exists {
		identityQuality = quality
	} else if wildcard == 0 {
		identityQuality = 0
	}
	accepted := make([]embeddedAssetEncoding, 0, len(embeddedAssetEncodings))
	for _, encoding := range embeddedAssetEncodings {
		quality, exists := qualities[encoding.name]
		if !exists {
			quality = wildcard
		}
		if quality > 0 && quality >= identityQuality {
			encoding.quality = quality
			accepted = append(accepted, encoding)
		}
	}
	sort.SliceStable(accepted, func(left, right int) bool {
		return accepted[left].quality > accepted[right].quality
	})
	return accepted, identityQuality > 0
}

func loadEmbeddedRepresentation(path string, encoding embeddedAssetEncoding) (*embeddedFile, error) {
	cacheKey := path + encoding.suffix
	if cached, ok := embeddedFileCache.Load(cacheKey); ok {
		return cached, nil
	}
	data, err := readAsset(cacheKey)
	if err != nil {
		return nil, err
	}
	return cacheEmbeddedRepresentation(cacheKey, path, data, encoding.name), nil
}

func loadEmbeddedIdentity(path string) (*embeddedFile, error) {
	if cached, ok := embeddedFileCache.Load(path); ok {
		return cached, nil
	}
	data, err := readAsset(path)
	if err != nil {
		return nil, err
	}
	return cacheEmbeddedFile(path, data), nil
}

func ServeEmbeddedFile(c fiber.Ctx, path string) error {
	if isPrecompressedAssetPath(path) {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}
	c.Vary(fiber.HeaderAcceptEncoding)
	encodings, identityAllowed := preferredAssetEncodings(c.Get(fiber.HeaderAcceptEncoding))
	var file *embeddedFile
	for _, encoding := range encodings {
		candidate, err := loadEmbeddedRepresentation(path, encoding)
		if err == nil {
			file = candidate
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to read asset")
		}
	}
	if file == nil && identityAllowed {
		var err error
		file, err = loadEmbeddedIdentity(path)
		if errors.Is(err, fs.ErrNotExist) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to read asset")
		}
	}
	if file == nil {
		asset, err := Asset.Open(resolveAssetPath(path))
		if errors.Is(err, fs.ErrNotExist) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to inspect asset")
		}
		_ = asset.Close()
		return c.SendStatus(fiber.StatusNotAcceptable)
	}

	if file.contentEncoding != "" {
		c.Set(fiber.HeaderContentEncoding, file.contentEncoding)
	}

	c.Set(fiber.HeaderContentType, file.contentType)
	c.Set(fiber.HeaderETag, file.etag)
	c.Set(fiber.HeaderCacheControl, frontendAssetCacheControl)
	c.Set(fiber.HeaderPragma, "no-cache")
	c.Set(fiber.HeaderExpires, "0")

	if clientETag := c.Get(fiber.HeaderIfNoneMatch); clientETag != "" && clientETag == file.etag {
		return c.SendStatus(fiber.StatusNotModified)
	}

	return c.Send(file.data)
}
