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
	indexHTML  string
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

type assetHashEntry struct {
	embedPath  string
	publicPath string // empty = hash only (not cached for HTTP)
}

func GetAssetsHash() string {
	onceHash.Do(func() {
		// Hash cache-busting entrypoints via Open+Copy (streaming). Warm the
		// HTTP embed cache for js/css in the same pass so ServeJs does not
		// re-read the bundle — one buffer per served asset, not two.
		hasher := sha256.New()
		for _, e := range []assetHashEntry{
			{embedPath: assetRoot + "/index.html"},
			{embedPath: assetRoot + "/dist/js/main.js", publicPath: "js/main.js"},
			{embedPath: assetRoot + "/dist/css/style.css", publicPath: "css/style.css"},
		} {
			f, err := Asset.Open(e.embedPath)
			if err != nil {
				continue
			}
			_, _ = io.WriteString(hasher, e.embedPath)
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
				_, _ = io.WriteString(hasher, name)
			}
		}
		assetsHash = hex.EncodeToString(hasher.Sum(nil))[:16]
	})
	return assetsHash
}

// GenerateIndexHTMLFromConfig renders the embedded index template for cfg.
func GenerateIndexHTMLFromConfig(cfg *config.FrontendConfig) []byte {
	onceIndex.Do(func() {
		data, err := readAsset("index.html")
		if err == nil {
			indexHTML = string(data)
		} else {
			indexHTML = CreateFallbackIndex()
		}
	})

	fontPreset, valid := config.NormalizeFrontendFontPreset(cfg.FontPreset)
	if !valid {
		fontPreset = config.FrontendFontSystem
	}
	fontURL := ""
	if fontPreset == config.FrontendFontCustom {
		fontURL = strings.TrimSpace(cfg.FontURL)
	}
	replacer := strings.NewReplacer(
		"{{RENOP.TITLE}}", html.EscapeString(cfg.Title),
		"{{RENOP.DESCRIPTION}}", html.EscapeString(cfg.Description),
		"{{RENOP.BASE_PATH}}", "",
		"{{RENOP.ID}}", html.EscapeString(cfg.ID),
		"{{RENOP.ORGANIZATION_WEBSITE}}", html.EscapeString(cfg.OrganizationWebsite),
		"{{RENOP.ORGANIZATION_LOGO}}", html.EscapeString(cfg.OrganizationLogo),
		"{{RENOP.BACKGROUND_URL}}", html.EscapeString(cfg.BackgroundURL),
		"{{RENOP.ICP_LICENSE}}", html.EscapeString(cfg.IcpLicense),
		"{{RENOP.PUBLIC_SECURITY_FILING}}", html.EscapeString(cfg.PublicSecurityFiling),
		"{{RENOP.LEGAL_NOTICE_URL}}", html.EscapeString(cfg.LegalNoticeURL),
		"{{RENOP.FONT_PRESET}}", html.EscapeString(fontPreset),
		"{{RENOP.FONT_URL}}", html.EscapeString(fontURL),
		"{{RENOP.HASH}}", GetAssetsHash(),
	)
	htmlStr := replacer.Replace(indexHTML)

	return []byte(htmlStr)
}

// RefreshIndexHTMLCache rebuilds the immutable H5 shell after frontend configuration changes.
func RefreshIndexHTMLCache(cfg *config.FrontendConfig) {
	if cfg == nil {
		return
	}
	cfg.CachedIndexHTML = GenerateIndexHTMLFromConfig(cfg)
}

// GenerateIndexHTML renders the embedded index template from live state.
func GenerateIndexHTML(state *core.AppState) []byte {
	cfg := state.Inner.Config.Load()
	if len(cfg.Frontend.CachedIndexHTML) > 0 {
		return cfg.Frontend.CachedIndexHTML
	}
	return GenerateIndexHTMLFromConfig(&cfg.Frontend)
}

func CreateFallbackIndex() string {
	return `<!DOCTYPE html>
<html lang="en" data-font-preset="{{RENOP.FONT_PRESET}}">
  <head>
    <meta charset="UTF-8" />
    <title>{{RENOP.TITLE}}</title>
    <meta name="description" content="{{RENOP.DESCRIPTION}}">
    <meta name="renop-font-url" content="{{RENOP.FONT_URL}}">
  </head>
  <body>
    <div id="app">Welcome to {{RENOP.TITLE}}</div>
  </body>
</html>`
}
