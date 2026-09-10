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
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	syncv2 "sync/v2"

	"github.com/gofiber/fiber/v3"

	"renop/internal/cache"
	"renop/internal/core"
	"renop/internal/utils"
)

var (
	embeddedFileCache   syncv2.Map[string, *embeddedFile]
	embeddedRemoteCache *cache.Remote
)

// UseRemoteCache configures rendered HTML caching before requests begin; static assets stay in embed.FS.
func UseRemoteCache(remote *cache.Remote) {
	indexHTMLCache = core.NewFileByteCache(1 << 20)
	indexHTMLCache.UseRemote(remote)
	embeddedFileCache.Clear()
	embeddedRemoteCache = remote
}

const frontendAssetCacheControl = "no-cache, must-revalidate, max-age=0"

type embeddedFile struct {
	assetPath       string
	size            int
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

// loadEmbeddedFile caches only immutable metadata; payload bytes are streamed from the executable.
func loadEmbeddedFile(cacheKey, publicPath, encoding string) (*embeddedFile, error) {
	if cached, ok := embeddedFileCache.Load(cacheKey); ok {
		return cached, nil
	}
	assetPath := resolveAssetPath(cacheKey)
	reader, err := Asset.Open(assetPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, reader)
	if err != nil {
		return nil, err
	}
	candidate := &embeddedFile{
		assetPath: assetPath, size: int(size),
		etag:        `W/"` + hex.EncodeToString(hasher.Sum(nil))[:16] + `"`,
		contentType: utils.ContentTypeByExt(filepath.Ext(publicPath)), contentEncoding: encoding,
	}
	actual, _ := embeddedFileCache.LoadOrStore(cacheKey, candidate)
	return actual, nil
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
	return loadEmbeddedFile(path+encoding.suffix, path, encoding.name)
}

func loadEmbeddedIdentity(path string) (*embeddedFile, error) {
	return loadEmbeddedFile(path, path, "")
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

	c.Response().Header.SetContentLength(file.size)
	if c.Method() == fiber.MethodHead {
		return nil
	}
	reader, err := Asset.Open(file.assetPath)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to read asset")
	}
	return c.SendStream(reader, file.size)
}
