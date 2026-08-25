/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/publicsuffix"

	"renop/internal/core"
)

const (
	maxMavenDomainLength = 253
	maxMavenPathParts    = 32
)

var (
	dnsLabelPattern       = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	githubAccountPattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,37}[a-z0-9])?$`)
	gitlabAccountPattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_-]{0,61}[a-z0-9])?$`)
	coordinatePartPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)
)

// NormalizeDomain validates and canonicalizes a Maven namespace.
func NormalizeDomain(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > maxMavenDomainLength || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
		return "", errors.New("Maven domain must be a valid reverse-domain namespace")
	}
	parts := strings.Split(value, ".")
	if len(parts) < 2 {
		return "", errors.New("Maven domain must contain at least two labels")
	}
	if len(parts) >= 3 && parts[0] == "io" && parts[1] == "github" {
		if !githubAccountPattern.MatchString(parts[2]) {
			return "", errors.New("GitHub Maven domains require a valid account name")
		}
		for _, part := range parts[3:] {
			if !dnsLabelPattern.MatchString(part) {
				return "", errors.New("Maven domain contains an invalid label")
			}
		}
		return value, nil
	}
	if len(parts) >= 3 && parts[0] == "io" && parts[1] == "gitlab" {
		if !gitlabAccountPattern.MatchString(parts[2]) {
			return "", errors.New("GitLab Maven domains require a valid account name")
		}
		for _, part := range parts[3:] {
			if !dnsLabelPattern.MatchString(part) {
				return "", errors.New("Maven domain contains an invalid label")
			}
		}
		return value, nil
	}
	for _, part := range parts {
		if !dnsLabelPattern.MatchString(part) {
			return "", errors.New("Maven domain contains an invalid DNS label")
		}
	}
	return value, nil
}

// VerificationTarget returns the verification mechanism and fixed target for a namespace.
func VerificationTarget(domain string) (verificationType, target string, err error) {
	domain, err = NormalizeDomain(domain)
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(domain, ".")
	if len(parts) >= 3 && parts[0] == "io" && parts[1] == "github" {
		return core.MavenVerificationGitHub, parts[2], nil
	}
	if len(parts) >= 3 && parts[0] == "io" && parts[1] == "gitlab" {
		return core.MavenVerificationGitLab, parts[2], nil
	}
	reversed := append([]string(nil), parts...)
	slices.Reverse(reversed)
	host := strings.Join(reversed, ".")
	if _, icann := publicsuffix.PublicSuffix(host); !icann {
		return "", "", errors.New("Maven domain must use a recognized public DNS suffix")
	}
	root, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil || root == "" {
		return "", "", errors.New("Maven domain does not map to a registrable DNS domain")
	}
	return core.MavenVerificationDNS, strings.ToLower(root), nil
}

// NewVerificationCode creates a high-entropy, non-secret proof string.
func NewVerificationCode() (string, error) {
	var random [18]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "renop-verification=" + base64.RawURLEncoding.EncodeToString(random[:]), nil
}

// MavenCoordinate identifies one standard Maven artifact path.
type MavenCoordinate struct {
	GroupID    string
	ArtifactID string
	Version    string
}

// ParseArtifactPath extracts groupId, artifactId, and version from a repository path.
func ParseArtifactPath(path string) (MavenCoordinate, bool) {
	parts := strings.Split(strings.Trim(strings.ReplaceAll(path, `\`, "/"), "/"), "/")
	if len(parts) < 4 || len(parts) > maxMavenPathParts {
		return MavenCoordinate{}, false
	}
	filename := parts[len(parts)-1]
	if filename == "" || filename == "maven-metadata.xml" || strings.HasPrefix(filename, "maven-metadata.xml.") {
		return MavenCoordinate{}, false
	}
	artifactID := parts[len(parts)-3]
	version := parts[len(parts)-2]
	groupParts := parts[:len(parts)-3]
	if len(groupParts) < 2 || !coordinatePartPattern.MatchString(artifactID) || !coordinatePartPattern.MatchString(version) {
		return MavenCoordinate{}, false
	}
	for _, part := range groupParts {
		if !coordinatePartPattern.MatchString(part) {
			return MavenCoordinate{}, false
		}
	}
	if !strings.HasPrefix(filename, artifactID+"-") {
		return MavenCoordinate{}, false
	}
	return MavenCoordinate{GroupID: strings.Join(groupParts, "."), ArtifactID: artifactID, Version: version}, true
}

func isMavenPublicationPath(path string) bool {
	if _, ok := ParseArtifactPath(path); ok {
		return true
	}
	parts := strings.Split(strings.Trim(strings.ReplaceAll(path, `\`, "/"), "/"), "/")
	if len(parts) < 3 || len(parts) > maxMavenPathParts {
		return false
	}
	filename := parts[len(parts)-1]
	allowedMetadata := filename == "maven-metadata.xml" || filename == "maven-metadata.xml.asc" ||
		filename == "maven-metadata.xml.md5" || filename == "maven-metadata.xml.sha1" ||
		filename == "maven-metadata.xml.sha256" || filename == "maven-metadata.xml.sha512"
	if !allowedMetadata {
		return false
	}
	for _, part := range parts[:len(parts)-1] {
		if !coordinatePartPattern.MatchString(part) {
			return false
		}
	}
	return true
}

func pathNamespaceCandidate(path string) string {
	parts := strings.Split(strings.Trim(strings.ReplaceAll(path, `\`, "/"), "/"), "/")
	if len(parts) < 2 || len(parts) > maxMavenPathParts {
		return ""
	}
	directories := parts[:len(parts)-1]
	for _, part := range directories {
		if !coordinatePartPattern.MatchString(part) {
			return ""
		}
	}
	return strings.ToLower(strings.Join(directories, "."))
}

func domainContainsGroup(domain, groupID string) bool {
	return groupID == domain || strings.HasPrefix(groupID, domain+".")
}
