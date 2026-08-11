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
	"embed"
	"encoding/hex"
	"html"
	"io"
	"strings"
	"sync"

	"github.com/3JoB/unsafeConvert"

	"renop/internal/config"
	"renop/internal/core"
)

//go:generate pnpm --dir renop-html install --frozen-lockfile
//go:generate pnpm --dir renop-html run build
//go:embed renop-html/index.html
//go:embed renop-html/svg
//go:embed renop-html/dist
var Asset embed.FS

var (
	indexHtml  string
	assetsHash string
	onceIndex  sync.Once
	onceHash   sync.Once
)

const assetRoot = "renop-html"

// resolveAssetPath maps public URL paths to embed.FS paths.
func resolveAssetPath(path string) string {
	path = strings.TrimPrefix(path, "/")
	switch {
	case path == "index.html":
		return assetRoot + "/index.html"
	case strings.HasPrefix(path, "svg/"):
		return assetRoot + "/" + path
	default:
		return assetRoot + "/dist/" + path
	}
}

func readAsset(path string) ([]byte, error) {
	return Asset.ReadFile(resolveAssetPath(path))
}

func GetAssetsHash() string {
	onceHash.Do(func() {
		// Hash cache-busting entrypoints via Open+Copy (streaming). Warm the
		// HTTP embed cache for js/css in the same pass so ServeJs does not
		// re-read the bundle — one buffer per served asset, not two.
		hasher := sha256.New()
		type entry struct {
			embedPath  string
			publicPath string // empty = hash only (not cached for HTTP)
		}
		for _, e := range []entry{
			{embedPath: assetRoot + "/index.html"},
			{embedPath: assetRoot + "/dist/js/main.js", publicPath: "js/main.js"},
			{embedPath: assetRoot + "/dist/css/style.css", publicPath: "css/style.css"},
		} {
			f, err := Asset.Open(e.embedPath)
			if err != nil {
				continue
			}
			hasher.Write(unsafeConvert.BytePointer(e.embedPath))
			if e.publicPath != "" {
				// Need body for HTTP cache: tee into a single buffer while hashing.
				data, err := io.ReadAll(f)
				_ = f.Close()
				if err != nil {
					continue
				}
				hasher.Write(data)
				cacheEmbeddedFile(e.publicPath, data)
			} else {
				_, _ = io.Copy(hasher, f)
				_ = f.Close()
			}
		}
		// Include svg names so logo changes still bust the index cache without
		// reading full image payloads when unnecessary.
		if entries, err := Asset.ReadDir(assetRoot + "/svg"); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				hasher.Write(unsafeConvert.BytePointer(name))
			}
		}
		assetsHash = hex.EncodeToString(hasher.Sum(nil))[:16]
	})
	return assetsHash
}

func GenerateIndexHtmlFromConfig(cfg *config.FrontendConfig) []byte {
	onceIndex.Do(func() {
		data, err := readAsset("index.html")
		if err == nil {
			indexHtml = unsafeConvert.StringPointer(data)
		} else {
			indexHtml = CreateFallbackIndex()
		}
	})

	replacer := strings.NewReplacer(
		"{{RENOP.TITLE}}", html.EscapeString(cfg.Title),
		"{{RENOP.DESCRIPTION}}", html.EscapeString(cfg.Description),
		"{{RENOP.BASE_PATH}}", "",
		"{{RENOP.ID}}", html.EscapeString(cfg.Id),
		"{{RENOP.ORGANIZATION_WEBSITE}}", html.EscapeString(cfg.OrganizationWebsite),
		"{{RENOP.ORGANIZATION_LOGO}}", html.EscapeString(cfg.OrganizationLogo),
		"{{RENOP.BACKGROUND_URL}}", html.EscapeString(cfg.BackgroundUrl),
		"{{RENOP.ICP_LICENSE}}", html.EscapeString(cfg.IcpLicense),
		"{{RENOP.LEGAL_NOTICE_URL}}", html.EscapeString(cfg.LegalNoticeUrl),
		"{{RENOP.HASH}}", GetAssetsHash(),
	)
	htmlStr := replacer.Replace(indexHtml)

	return unsafeConvert.BytePointer(htmlStr)
}

func GenerateIndexHtml(state *core.AppState) []byte {
	cfg := state.Inner.Config.Load()
	if len(cfg.Frontend.CachedIndexHtml) > 0 {
		return cfg.Frontend.CachedIndexHtml
	}
	return GenerateIndexHtmlFromConfig(&cfg.Frontend)
}

func CreateFallbackIndex() string {
	return `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <title>{{RENOP.TITLE}}</title>
    <meta name="description" content="{{RENOP.DESCRIPTION}}">
  </head>
  <body>
    <div id="app">Welcome to {{RENOP.TITLE}}</div>
  </body>
</html>`
}
