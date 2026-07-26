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

import (
	"bytes"
	"encoding/xml"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/3JoB/unsafeConvert"
)

func SafeRename(oldpath, newpath string) error {
	// Unix can atomically replace the destination. Windows cannot, so only
	// fall back to removing the destination after the atomic attempt fails.
	renameErr := os.Rename(oldpath, newpath)
	if renameErr == nil {
		return nil
	}
	if runtime.GOOS != "windows" {
		return renameErr
	}
	if err := os.Remove(newpath); err != nil {
		return renameErr
	}
	return os.Rename(oldpath, newpath)
}

func EscapeXML(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, unsafeConvert.BytePointer(s))
	return buf.String()
}

func GetS3Key(path string) string {
	s3Key := filepath.ToSlash(path)
	s3Key = strings.TrimPrefix(s3Key, "./")
	s3Key = strings.TrimPrefix(s3Key, "/")
	return s3Key
}

func isReservedDeviceName(part string) bool {
	before, _, ok := strings.Cut(part, ".")
	base := part
	if ok {
		base = before
	}
	baseLower := strings.ToLower(base)
	switch baseLower {
	case "con", "prn", "aux", "nul",
		"com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9",
		"lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9":
		return true
	}
	return false
}

func SanitizePath(path string) (string, bool) {
	if path == "" {
		return "", true
	}
	if !utf8.ValidString(path) {
		return "", false
	}

	var builder strings.Builder
	builder.Grow(len(path))

	start := 0
	n := len(path)
	for start < n {
		end := strings.IndexByte(path[start:], '/')
		var part string
		if end == -1 {
			part = path[start:]
			start = n
		} else {
			part = path[start : start+end]
			start = start + end + 1
		}

		if part == "" || part == "." {
			continue
		}

		decoded, err := url.PathUnescape(part)
		if err != nil || !utf8.ValidString(decoded) {
			return "", false
		}
		part = decoded

		if part == "" || part == "." {
			continue
		}
		if part == ".." || strings.ContainsAny(part, `/\`) ||
			strings.HasSuffix(part, " ") || strings.HasSuffix(part, ".") ||
			isReservedDeviceName(part) {
			return "", false
		}
		for _, r := range part {
			if unicode.IsControl(r) || strings.ContainsRune(`\:%*?#"<>|`, r) {
				return "", false
			}
		}
		if builder.Len() > 0 {
			builder.WriteByte('/')
		}
		builder.WriteString(part)
	}

	return builder.String(), true
}

func IsValidRepositoryName(name string) bool {
	if name == "" || strings.ContainsRune(name, '/') {
		return false
	}
	sanitized, ok := SanitizePath(name)
	return ok && sanitized == name
}

func IsImageFile(path string) bool {
	idx := strings.LastIndexByte(path, '.')
	if idx == -1 {
		return false
	}
	ext := path[idx+1:]
	l := len(ext)
	if l != 3 && l != 4 {
		return false
	}

	if l == 3 {
		c0 := ext[0] | 0x20
		c1 := ext[1] | 0x20
		c2 := ext[2] | 0x20

		if c0 == 'p' && c1 == 'n' && c2 == 'g' {
			return true
		}
		if c0 == 'j' && c1 == 'p' && c2 == 'g' {
			return true
		}
		if c0 == 'g' && c1 == 'i' && c2 == 'f' {
			return true
		}
		if c0 == 'b' && c1 == 'm' && c2 == 'p' {
			return true
		}
		if c0 == 's' && c1 == 'v' && c2 == 'g' {
			return true
		}
		if c0 == 'i' && c1 == 'c' && c2 == 'o' {
			return true
		}
		return false
	}

	c0 := ext[0] | 0x20
	c1 := ext[1] | 0x20
	c2 := ext[2] | 0x20
	c3 := ext[3] | 0x20

	if c0 == 'j' && c1 == 'p' && c2 == 'e' && c3 == 'g' {
		return true
	}
	if c0 == 'w' && c1 == 'e' && c2 == 'b' && c3 == 'p' {
		return true
	}

	return false
}

func IsPreviewableTextFile(path string) bool {
	idx := strings.LastIndexByte(path, '.')
	if idx == -1 {
		return false
	}
	ext := strings.ToLower(path[idx:])
	switch ext {
	case ".pom", ".xml", ".json", ".txt", ".md", ".yml", ".yaml":
		return true
	default:
		return false
	}
}

func IsSubPath(basePath, targetPath string) bool {
	cleanBase := filepath.Clean(basePath)
	cleanTarget := filepath.Clean(targetPath)
	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return false
	}
	return true
}
