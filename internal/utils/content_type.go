/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package utils

import "strings"

var contentTypes = map[string]string{
	".html":       "text/html; charset=utf-8",
	".htm":        "text/html; charset=utf-8",
	".css":        "text/css; charset=utf-8",
	".js":         "text/javascript; charset=utf-8",
	".mjs":        "text/javascript; charset=utf-8",
	".json":       "application/json; charset=utf-8",
	".map":        "application/json; charset=utf-8",
	".xml":        "text/xml; charset=utf-8",
	".pom":        "text/xml; charset=utf-8",
	".svg":        "image/svg+xml",
	".png":        "image/png",
	".jpg":        "image/jpeg",
	".jpeg":       "image/jpeg",
	".gif":        "image/gif",
	".webp":       "image/webp",
	".ico":        "image/x-icon",
	".woff":       "font/woff",
	".woff2":      "font/woff2",
	".ttf":        "font/ttf",
	".otf":        "font/otf",
	".txt":        "text/plain; charset=utf-8",
	".md":         "text/markdown; charset=utf-8",
	".csv":        "text/csv; charset=utf-8",
	".jar":        "application/java-archive",
	".war":        "application/java-archive",
	".ear":        "application/java-archive",
	".zip":        "application/zip",
	".br":         "application/x-brotli",
	".gz":         "application/gzip",
	".tgz":        "application/gzip",
	".tar":        "application/x-tar",
	".xz":         "application/x-xz",
	".txz":        "application/x-xz",
	".bz2":        "application/x-bzip2",
	".tbz2":       "application/x-bzip2",
	".7z":         "application/x-7z-compressed",
	".rar":        "application/vnd.rar",
	".zst":        "application/zstd",
	".zstd":       "application/zstd",
	".lz4":        "application/x-lz4",
	".sz":         "application/x-snappy-framed",
	".cab":        "application/vnd.ms-cab-compressed",
	".wim":        "application/x-ms-wim",
	".pdf":        "application/pdf",
	".wasm":       "application/wasm",
	".sha1":       "text/plain; charset=utf-8",
	".sha256":     "text/plain; charset=utf-8",
	".sha512":     "text/plain; charset=utf-8",
	".md5":        "text/plain; charset=utf-8",
	".asc":        "text/plain; charset=utf-8",
	".module":     "application/json; charset=utf-8",
	".toml":       "application/toml; charset=utf-8",
	".yaml":       "application/yaml; charset=utf-8",
	".yml":        "application/yaml; charset=utf-8",
	".properties": "text/plain; charset=utf-8",
	".proto":      "text/plain; charset=utf-8",
	".bin":        "application/octet-stream",
	".exe":        "application/octet-stream",
	".dll":        "application/octet-stream",
	".so":         "application/octet-stream",
	".dylib":      "application/octet-stream",
}

func ContentTypeByExt(ext string) string {
	if ext == "" {
		return "application/octet-stream"
	}
	if ext[0] != '.' {
		ext = "." + ext
	}
	if ct, ok := contentTypes[strings.ToLower(ext)]; ok {
		return ct
	}
	return "application/octet-stream"
}
