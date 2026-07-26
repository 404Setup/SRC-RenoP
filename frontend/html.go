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
	"strings"
	"sync"

	"github.com/3JoB/unsafeConvert"

	"renop/config"
	"renop/core"
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
		hasher := sha256.New()
		var walk func(string)
		walk = func(path string) {
			entries, err := Asset.ReadDir(path)
			if err != nil {
				return
			}
			for _, entry := range entries {
				entryPath := path + "/" + entry.Name()
				if entry.IsDir() {
					walk(entryPath)
				} else {
					data, err := Asset.ReadFile(entryPath)
					if err == nil {
						hasher.Write(unsafeConvert.BytePointer(entryPath))
						hasher.Write(data)
					}
				}
			}
		}
		walk(assetRoot)
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
		"{{RENOP.HASH}}", GetAssetsHash(),
	)
	htmlStr := replacer.Replace(indexHtml)

	return unsafeConvert.BytePointer(htmlStr)
}

func GenerateIndexHtml(state *core.AppState) []byte {
	cfg := state.Inner.Config.Load().(*config.Config)
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
