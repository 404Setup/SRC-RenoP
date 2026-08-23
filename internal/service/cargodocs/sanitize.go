/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargodocs

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zip"
)

var (
	errDocNotForCrate       = errors.New("documentation archive does not belong to the target crate")
	errDocContainsTracker   = errors.New("documentation archive contains tracker or suspicious tracking script")
	errDocInvalidFileFormat = errors.New("documentation archive contains invalid or forbidden file")
)

var allowedDocExtensions = map[string]bool{
	".html":  true,
	".htm":   true,
	".css":   true,
	".js":    true,
	".rs":    true,
	".txt":   true,
	".md":    true,
	".png":   true,
	".jpg":   true,
	".jpeg":  true,
	".gif":   true,
	".svg":   true,
	".ico":   true,
	".webp":  true,
	".woff":  true,
	".woff2": true,
	".ttf":   true,
	".eot":   true,
	".otf":   true,
	".json":  true,
	".map":   true,
}

var forbiddenDocExtensions = map[string]bool{
	".exe":    true,
	".dll":    true,
	".so":     true,
	".dylib":  true,
	".elf":    true,
	".bin":    true,
	".com":    true,
	".msi":    true,
	".jar":    true,
	".class":  true,
	".pyc":    true,
	".sh":     true,
	".bat":    true,
	".cmd":    true,
	".ps1":    true,
	".vbs":    true,
	".wsf":    true,
	".php":    true,
	".asp":    true,
	".aspx":   true,
	".jsp":    true,
	".cgi":    true,
	".pl":     true,
	".py":     true,
	".rb":     true,
	".zip":    true,
	".tar.gz": true,
	".tgz":    true,
	".tar":    true,
	".rar":    true,
	".7z":     true,
	".gz":     true,
	".bz2":    true,
	".xz":     true,
}

var trackerDomains = []string{
	"google-analytics.com",
	"googletagmanager.com",
	"analytics.google.com",
	"cloudflareinsights.com",
	"static.cloudflareinsights.com",
	"hm.baidu.com",
	"cnzz.com",
	"umeng.com",
	"matomo.js",
	"piwik.js",
	"matomo.php",
	"piwik.php",
	"plausible.io",
	"usefathom.com",
	"umami.is",
	"umami.js",
	"static.hotjar.com",
	"hotjar.com",
	"mixpanel.com",
	"cdn.mxpnl.com",
	"cdn.segment.com",
	"mc.yandex.ru",
	"amplitude.com",
	"sentry-cdn.com",
	"connect.facebook.net",
	"clarity.ms",
	"ads-twitter.com",
	"bat.bing.com",
	"posthog.com",
	"posthog.js",
	"fullstory.com",
	"logrocket.io",
	"logrocket.com",
}

var trackerPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<script[^>]+src\s*=\s*["']\s*(?:https?:)?//`),
	regexp.MustCompile(`(?i)<a[^>]+ping\s*=`),
	regexp.MustCompile(`navigator\.sendBeacon\s*\(`),
	regexp.MustCompile(`_hmt\.push\s*\(`),
	regexp.MustCompile(`gtag\s*\(`),
	regexp.MustCompile(`ga\s*\(\s*['"](?:create|send|set|require)`),
	regexp.MustCompile(`_gaq\.push\s*\(`),
	regexp.MustCompile(`_paq\.push\s*\(`),
	regexp.MustCompile(`posthog\.init\s*\(`),
	regexp.MustCompile(`mixpanel\.init\s*\(`),
	regexp.MustCompile(`mixpanel\.track\s*\(`),
	regexp.MustCompile(`_hjSettings`),
	regexp.MustCompile(`ym\s*\(\s*\d+`),
	regexp.MustCompile(`analytics\.load\s*\(`),
	regexp.MustCompile(`analytics\.track\s*\(`),
	regexp.MustCompile(`(?i)<img[^>]+src\s*=\s*["']\s*(?:https?:)?//[^>]+(?:width|height)\s*=\s*["']\s*[01]\s*["']`),
	regexp.MustCompile(`(?i)<img[^>]+(?:width|height)\s*=\s*["']\s*[01]\s*["'][^>]+src\s*=\s*["']\s*(?:https?:)?//`),
	regexp.MustCompile(`(?i)<img[^>]+style\s*=\s*["'][^"']*(?:display\s*:\s*none|visibility\s*:\s*hidden)[^"']*["'][^>]+src\s*=\s*["']\s*(?:https?:)?//`),
	regexp.MustCompile(`(?i)<img[^>]+src\s*=\s*["']\s*(?:https?:)?//[^>]+style\s*=\s*["'][^"']*(?:display\s*:\s*none|visibility\s*:\s*hidden)`),
	regexp.MustCompile(`(?i)<iframe[^>]+(?:width|height)\s*=\s*["']\s*[01]\s*["']`),
	regexp.MustCompile(`(?i)<iframe[^>]+style\s*=\s*["'][^"']*(?:display\s*:\s*none|visibility\s*:\s*hidden)`),
	regexp.MustCompile(`(?i)new\s+Image\s*\(\s*\)\.src\s*=\s*["']\s*(?:https?:)?//`),
}

func normalizeCrateVariants(crateName string) []string {
	clean := strings.TrimSpace(crateName)
	if clean == "" {
		return nil
	}
	under := strings.ReplaceAll(clean, "-", "_")
	hyphen := strings.ReplaceAll(clean, "_", "-")

	set := make(map[string]struct{})
	for _, v := range []string{clean, under, hyphen, strings.ToLower(clean), strings.ToLower(under), strings.ToLower(hyphen)} {
		if v != "" {
			set[v] = struct{}{}
		}
	}
	variants := make([]string, 0, len(set))
	for v := range set {
		variants = append(variants, v)
	}
	return variants
}

func stripArchivePrefix(cleanPath string) string {
	cleanPath = strings.TrimPrefix(cleanPath, "./")
	cleanPath = strings.TrimPrefix(cleanPath, "/")

	for {
		if after, ok := strings.CutPrefix(cleanPath, "target/doc/"); ok {
			cleanPath = after
			continue
		}
		if after, ok := strings.CutPrefix(cleanPath, "doc/"); ok {
			cleanPath = after
			continue
		}
		break
	}
	return cleanPath
}

// IsTargetCrateDocRoot reports whether a file path points to the documentation entry point of the crate.
func IsTargetCrateDocRoot(cleanPath string, crateName string) bool {
	norm := stripArchivePrefix(cleanPath)
	variants := normalizeCrateVariants(crateName)

	base := path.Base(norm)
	baseLower := strings.ToLower(base)
	dir := path.Dir(norm)

	if baseLower == "index.html" || baseLower == "all.html" {
		if dir == "." || dir == "" {
			return true
		}
		dirBase := strings.ToLower(path.Base(dir))
		for _, v := range variants {
			if strings.EqualFold(dirBase, v) {
				return true
			}
		}
	}

	for _, v := range variants {
		if strings.EqualFold(norm, v+"/index.html") || strings.EqualFold(norm, v+"/all.html") {
			return true
		}
	}
	return false
}

// IsTargetCrateOrSharedAsset reports whether an entry should be kept for the target crate.
func IsTargetCrateOrSharedAsset(cleanPath string, crateName string) bool {
	norm := stripArchivePrefix(cleanPath)
	if norm == "" || norm == "." {
		return true
	}

	variants := normalizeCrateVariants(crateName)
	normLower := strings.ToLower(norm)
	base := path.Base(norm)
	baseLower := strings.ToLower(base)

	switch baseLower {
	case "index.html", "help.html", "settings.html", "all.html", "crates.js", "search-index.js", "source-files.js", "settings.js":
		if path.Dir(norm) == "." || path.Dir(norm) == "" {
			return true
		}
	}

	if strings.HasPrefix(normLower, "search-index-") && strings.HasSuffix(normLower, ".js") {
		return true
	}

	if strings.HasPrefix(normLower, "static.files/") || strings.HasPrefix(normLower, "implementors/") || strings.HasPrefix(normLower, "search.index/") {
		return true
	}

	if normLower == "src" || normLower == "src/" || normLower == "src/source-files.js" || normLower == "src/crates.js" {
		return true
	}
	if strings.HasPrefix(normLower, "src/") {
		sub := norm[4:]
		firstSub, _, _ := strings.Cut(sub, "/")
		for _, v := range variants {
			if strings.EqualFold(firstSub, v) {
				return true
			}
		}
		return false
	}

	if strings.HasPrefix(normLower, "trait.impl/") {
		sub := norm[11:]
		firstSub, _, _ := strings.Cut(sub, "/")
		for _, v := range variants {
			if strings.EqualFold(firstSub, v) {
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(normLower, "type.impl/") {
		sub := norm[10:]
		firstSub, _, _ := strings.Cut(sub, "/")
		for _, v := range variants {
			if strings.EqualFold(firstSub, v) {
				return true
			}
		}
		return false
	}

	firstPart, _, _ := strings.Cut(norm, "/")
	for _, v := range variants {
		if strings.EqualFold(firstPart, v) {
			return true
		}
	}

	if path.Dir(norm) == "." || path.Dir(norm) == "" {
		ext := strings.ToLower(path.Ext(norm))
		if allowedDocExtensions[ext] {
			return true
		}
	}

	return false
}

// ValidateDocFileEntry validates entry paths and checks for forbidden extensions.
func ValidateDocFileEntry(name string) error {
	if name == "" || strings.ContainsAny(name, "\\\x00:") {
		return fmt.Errorf("%w: unsafe path %q", errDocInvalidFileFormat, name)
	}
	clean := path.Clean(filepath.ToSlash(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return fmt.Errorf("%w: unsafe path %q", errDocInvalidFileFormat, name)
	}

	lower := strings.ToLower(clean)
	parts := strings.SplitSeq(lower, "/")
	for p := range parts {
		if p == ".git" || p == ".svn" || p == ".hg" || p == "__macosx" || p == ".ds_store" || p == "thumbs.db" || p == ".env" {
			return fmt.Errorf("%w: forbidden system entry %q", errDocInvalidFileFormat, name)
		}
	}

	base := path.Base(lower)
	if strings.HasPrefix(base, ".") && base != ".well-known" {
		return fmt.Errorf("%w: hidden entry %q", errDocInvalidFileFormat, name)
	}

	ext := strings.ToLower(path.Ext(clean))
	if ext != "" {
		if forbiddenDocExtensions[ext] {
			return fmt.Errorf("%w: forbidden file extension %q in %s", errDocInvalidFileFormat, ext, name)
		}
		if !allowedDocExtensions[ext] {
			return fmt.Errorf("%w: unsupported file extension %q in %s", errDocInvalidFileFormat, ext, name)
		}
	}

	return nil
}

func needsTrackerScan(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".html", ".htm", ".js", ".svg", ".css":
		return true
	default:
		return false
	}
}

func containsFoldASCII(b []byte, s string) bool {
	if len(s) == 0 {
		return true
	}
	if len(b) < len(s) {
		return false
	}
	firstLower := s[0]
	firstUpper := firstLower
	if firstLower >= 'a' && firstLower <= 'z' {
		firstUpper = firstLower - 32
	}
	for i := 0; i <= len(b)-len(s); i++ {
		c := b[i]
		if c == firstLower || c == firstUpper {
			match := true
			for j := 1; j < len(s); j++ {
				bc := b[i+j]
				if bc >= 'A' && bc <= 'Z' {
					bc += 32
				}
				if bc != s[j] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

// ScanDocContentForTrackers checks content for analytics scripts, beacons, and tracker patterns.
func ScanDocContentForTrackers(name string, content []byte) error {
	if !needsTrackerScan(name) {
		return nil
	}

	for _, domain := range trackerDomains {
		if containsFoldASCII(content, domain) {
			return fmt.Errorf("%w: tracker domain %q found in %s", errDocContainsTracker, domain, name)
		}
	}

	for _, pattern := range trackerPatterns {
		if loc := pattern.FindIndex(content); loc != nil {
			snippet := string(content[loc[0]:min(len(content), loc[1]+20)])
			return fmt.Errorf("%w: suspicious pattern %q found in %s", errDocContainsTracker, snippet, name)
		}
	}

	return nil
}

// SanitizeTarGzDocArchive processes a tar.gz archive, validating ownership, rejecting trackers,
// stripping non-target package contents, and writing a clean tar.gz stream.
func SanitizeTarGzDocArchive(reader io.Reader, crateName string, writer io.Writer) error {
	gz, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("%w: invalid gzip header", errDocInvalidFileFormat)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var hasTargetDoc bool
	var entriesCount int
	var totalSize uint64

	outGz := gzip.NewWriter(writer)
	defer outGz.Close()
	outTar := tar.NewWriter(outGz)
	defer outTar.Close()

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: invalid tar structure", errDocInvalidFileFormat)
		}

		entriesCount++
		if entriesCount > maxCargodocEntries {
			return fmt.Errorf("%w: too many entries", errUnsafeCargodocArchive)
		}

		cleanPath := path.Clean(filepath.ToSlash(hdr.Name))
		if err := ValidateDocFileEntry(cleanPath); err != nil {
			return err
		}

		if hdr.Typeflag == tar.TypeDir {
			if IsTargetCrateOrSharedAsset(cleanPath, crateName) {
				_ = outTar.WriteHeader(hdr)
			}
			continue
		}

		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}

		if hdr.Size < 0 || uint64(hdr.Size) > maxCargodocEntrySize || totalSize+uint64(hdr.Size) > maxCargodocTotalExtractedSize {
			return fmt.Errorf("%w: documentation exceeds size limit", errUnsafeCargodocArchive)
		}
		totalSize += uint64(hdr.Size)

		if IsTargetCrateDocRoot(cleanPath, crateName) {
			hasTargetDoc = true
		}

		targetAsset := IsTargetCrateOrSharedAsset(cleanPath, crateName)

		if !needsTrackerScan(cleanPath) {
			if targetAsset {
				if err := outTar.WriteHeader(hdr); err != nil {
					return err
				}
				written, copyErr := io.Copy(outTar, io.LimitReader(tr, hdr.Size))
				if copyErr != nil || written != hdr.Size {
					return fmt.Errorf("%w: truncated entry %s", errDocInvalidFileFormat, cleanPath)
				}
			} else {
				_, _ = io.Copy(io.Discard, io.LimitReader(tr, hdr.Size))
			}
			continue
		}

		content, readErr := io.ReadAll(io.LimitReader(tr, hdr.Size+1))
		if readErr != nil || int64(len(content)) != hdr.Size {
			return fmt.Errorf("%w: truncated entry %s", errDocInvalidFileFormat, cleanPath)
		}

		if err := ScanDocContentForTrackers(cleanPath, content); err != nil {
			return err
		}

		if targetAsset {
			hdrCopy := *hdr
			hdrCopy.Size = int64(len(content))
			if err := outTar.WriteHeader(&hdrCopy); err != nil {
				return err
			}
			if _, err := outTar.Write(content); err != nil {
				return err
			}
		}
	}

	if !hasTargetDoc {
		return fmt.Errorf("%w: archive does not contain documentation for crate %q", errDocNotForCrate, crateName)
	}

	return nil
}

// SanitizeZipDocArchive processes a zip archive, validating ownership, rejecting trackers,
// stripping non-target package contents, and writing a clean zip stream.
func SanitizeZipDocArchive(readerAt io.ReaderAt, size int64, crateName string, writer io.Writer) error {
	zr, err := zip.NewReader(readerAt, size)
	if err != nil {
		return fmt.Errorf("%w: invalid zip structure", errDocInvalidFileFormat)
	}

	if len(zr.File) > maxCargodocEntries {
		return fmt.Errorf("%w: too many entries", errUnsafeCargodocArchive)
	}

	var hasTargetDoc bool
	var totalSize uint64

	zw := zip.NewWriter(writer)
	defer zw.Close()

	for _, f := range zr.File {
		cleanPath := path.Clean(filepath.ToSlash(f.Name))
		if err := ValidateDocFileEntry(cleanPath); err != nil {
			return err
		}

		if f.FileInfo().IsDir() {
			if IsTargetCrateOrSharedAsset(cleanPath, crateName) {
				_, _ = zw.Create(cleanPath + "/")
			}
			continue
		}

		if f.UncompressedSize64 > maxCargodocEntrySize || totalSize+f.UncompressedSize64 > maxCargodocTotalExtractedSize {
			return fmt.Errorf("%w: documentation exceeds size limit", errUnsafeCargodocArchive)
		}
		totalSize += f.UncompressedSize64

		if IsTargetCrateDocRoot(cleanPath, crateName) {
			hasTargetDoc = true
		}

		targetAsset := IsTargetCrateOrSharedAsset(cleanPath, crateName)
		if !targetAsset && !needsTrackerScan(cleanPath) {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		if !needsTrackerScan(cleanPath) {
			w, createErr := zw.Create(cleanPath)
			if createErr != nil {
				_ = rc.Close()
				return createErr
			}
			written, copyErr := io.Copy(w, rc)
			_ = rc.Close()
			if copyErr != nil || uint64(written) != f.UncompressedSize64 {
				return fmt.Errorf("%w: truncated entry %s", errDocInvalidFileFormat, cleanPath)
			}
			continue
		}

		content, readErr := io.ReadAll(io.LimitReader(rc, int64(f.UncompressedSize64)+1))
		_ = rc.Close()
		if readErr != nil || uint64(len(content)) != f.UncompressedSize64 {
			return fmt.Errorf("%w: truncated entry %s", errDocInvalidFileFormat, cleanPath)
		}

		if err := ScanDocContentForTrackers(cleanPath, content); err != nil {
			return err
		}

		if targetAsset {
			w, err := zw.Create(cleanPath)
			if err != nil {
				return err
			}
			if _, err := w.Write(content); err != nil {
				return err
			}
		}
	}

	if !hasTargetDoc {
		return fmt.Errorf("%w: archive does not contain documentation for crate %q", errDocNotForCrate, crateName)
	}

	return nil
}

// SanitizeDocArchiveToBytes sanitizes either tar.gz or zip archive data and returns the cleaned archive bytes.
func SanitizeDocArchiveToBytes(data []byte, isZip bool, crateName string) ([]byte, error) {
	var outBuf bytes.Buffer
	if isZip {
		if err := SanitizeZipDocArchive(bytes.NewReader(data), int64(len(data)), crateName, &outBuf); err != nil {
			return nil, err
		}
	} else {
		if err := SanitizeTarGzDocArchive(bytes.NewReader(data), crateName, &outBuf); err != nil {
			return nil, err
		}
	}
	return outBuf.Bytes(), nil
}
