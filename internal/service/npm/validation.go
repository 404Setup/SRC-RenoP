/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"net/url"
	"path"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

const (
	maxPackageNameLength = 214
	maxVersionLength     = 128
	maxTagLength         = 128
)

func validNPMSegment(value string) bool {
	if value == "" || !((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= '0' && value[0] <= '9')) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			character == '-' || character == '_' || character == '.' || character == '~' {
			continue
		}
		return false
	}
	return true
}

// NormalizePackageName validates one canonical lowercase npm package name.
func NormalizePackageName(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > maxPackageNameLength || value == "node_modules" || value == "favicon.ico" {
		return "", false
	}
	if strings.HasPrefix(value, "@") {
		parts := strings.Split(value, "/")
		if len(parts) != 2 || len(parts[0]) < 2 || !validNPMSegment(parts[0][1:]) || !validNPMSegment(parts[1]) {
			return "", false
		}
		return value, true
	}
	if strings.Contains(value, "/") || !validNPMSegment(value) {
		return "", false
	}
	return value, true
}

func validNPMVersion(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= maxVersionLength && semver.IsValid("v"+value)
}

func npmTagConflictsWithVersionRange(value string) bool {
	if value == "" {
		return false
	}
	if semver.IsValid("v" + value) {
		return true
	}
	lower := strings.ToLower(value)
	if lower == "*" || strings.HasPrefix(lower, "v") && len(lower) > 1 && lower[1] >= '0' && lower[1] <= '9' {
		return true
	}
	if strings.ContainsAny(lower[:1], "<>=~^") {
		return true
	}
	hasDigit := false
	for index := 0; index < len(lower); index++ {
		character := lower[index]
		if character >= '0' && character <= '9' {
			hasDigit = true
			continue
		}
		if character == '.' || character == 'x' || character == '*' || character == '-' ||
			character == '|' || character == ' ' {
			continue
		}
		return false
	}
	return hasDigit
}

func validNPMTag(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > maxTagLength || npmTagConflictsWithVersionRange(value) {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			character == '-' || character == '_' || character == '.' || character == '~' {
			continue
		}
		return false
	}
	return true
}

func decodeRegistryPath(value string) (string, bool) {
	decoded, err := url.PathUnescape(strings.Trim(value, "/"))
	if err != nil || strings.ContainsRune(decoded, '\x00') {
		return "", false
	}
	return strings.Trim(decoded, "/"), true
}

func packageFromMetadataPath(value string) (string, bool) {
	decoded, ok := decodeRegistryPath(value)
	if !ok {
		return "", false
	}
	return NormalizePackageName(decoded)
}

func packageFromTarballPath(value string) (string, bool) {
	decoded, ok := decodeRegistryPath(value)
	if !ok {
		return "", false
	}
	parts := strings.Split(decoded, "/")
	if len(parts) == 3 && parts[1] == "-" {
		return NormalizePackageName(parts[0])
	}
	if len(parts) == 4 && strings.HasPrefix(parts[0], "@") && parts[2] == "-" {
		return NormalizePackageName(parts[0] + "/" + parts[1])
	}
	return "", false
}

func canonicalTarballPath(packageName, version string) string {
	baseName := packageName
	if separator := strings.LastIndexByte(baseName, '/'); separator >= 0 {
		baseName = baseName[separator+1:]
	}
	return path.Join(packageName, "-", baseName+"-"+version+".tgz")
}

// ClassifyTarballPath returns the npm package and version encoded by a canonical tarball path.
func ClassifyTarballPath(value string) (packageName, version string, ok bool) {
	packageName, ok = packageFromTarballPath(value)
	if !ok {
		return "", "", false
	}
	decoded, _ := decodeRegistryPath(value)
	filename := path.Base(decoded)
	baseName := packageName
	if separator := strings.LastIndexByte(baseName, '/'); separator >= 0 {
		baseName = baseName[separator+1:]
	}
	prefix := baseName + "-"
	if !strings.HasPrefix(filename, prefix) || !strings.HasSuffix(filename, ".tgz") {
		return "", "", false
	}
	version = strings.TrimSuffix(strings.TrimPrefix(filename, prefix), ".tgz")
	if !validNPMVersion(version) {
		return "", "", false
	}
	return packageName, version, true
}

func escapedPackageName(packageName string) string {
	return url.PathEscape(packageName)
}

func parseRevision(value string) int64 {
	value = strings.TrimSpace(value)
	if before, _, ok := strings.Cut(value, "-"); ok {
		value = before
	}
	revision, _ := strconv.ParseInt(value, 10, 64)
	return max(revision, 0)
}
