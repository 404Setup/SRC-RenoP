/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package updater

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"renop/utils"
	"renop/version"
)

const maxRemoteJSONBody = 1 << 20

var checkHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			ClientSessionCache: nil,
		},
		TLSNextProto:          make(map[string]func(string, *tls.Conn) http.RoundTripper),
		DisableCompression:    true,
		DisableKeepAlives:     true,
		MaxIdleConns:          0,
		MaxIdleConnsPerHost:   0,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
	Timeout: 15 * time.Second,
}

func CheckUpdate(ctx context.Context, channel Channel) (*CheckResult, error) {
	defer checkHTTPClient.CloseIdleConnections()
	if channel == ChannelNightly {
		return checkNightly(ctx)
	}
	return checkRelease(ctx)
}

func doJSONGet(ctx context.Context, url string, accept string, dst any) (statusCode int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "RenoP-Updater")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Close = true

	resp, err := checkHTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer utils.DrainAndClose(resp.Body)

	statusCode = resp.StatusCode
	if statusCode != http.StatusOK {
		return statusCode, fmt.Errorf("status %d", statusCode)
	}

	if resp.ContentLength > maxRemoteJSONBody {
		return statusCode, fmt.Errorf("response too large: Content-Length=%d", resp.ContentLength)
	}

	data, err := utils.ReadAllLimited(resp.Body, maxRemoteJSONBody)
	if err != nil {
		if errors.Is(err, utils.ErrResponseTooLarge) {
			return statusCode, fmt.Errorf("response exceeds %d bytes", maxRemoteJSONBody)
		}
		return statusCode, err
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return statusCode, err
	}
	return statusCode, nil
}

func doGitHubJSON(ctx context.Context, url string, dst any) (statusCode int, err error) {
	return doJSONGet(ctx, url, "application/vnd.github.v3+json", dst)
}

func clipString(s string, max int) string {
	if max > 0 && len(s) > max {
		s = s[:max]
	}
	return strings.Clone(s)
}

func commitSubject(message string) string {
	msg := strings.TrimSpace(message)
	if before, _, ok := strings.Cut(msg, "\n"); ok {
		return strings.TrimSpace(before)
	}
	return msg
}

func isWebOnlyCommit(subject string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(subject)), "[web]")
}

func shouldOmitNightlyNote(subject string) bool {
	lower := strings.ToLower(strings.TrimSpace(subject))
	if lower == "" {
		return true
	}
	if isWebOnlyCommit(lower) {
		return true
	}
	if strings.HasPrefix(lower, "[skip") || strings.HasPrefix(lower, "[ci skip]") || strings.HasPrefix(lower, "skip:") ||
		strings.Contains(lower, "[skip ci]") || strings.Contains(lower, "[ci skip]") {
		return true
	}
	if strings.HasPrefix(lower, "[release]") || strings.HasPrefix(lower, "[releases]") ||
		strings.HasPrefix(lower, "release:") || strings.HasPrefix(lower, "releases:") {
		return true
	}
	return false
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// commitSHAMatches reports whether a commit SHA matches a target identity
// (full SHA, short SHA, or either side being a prefix of the other when long enough).
func commitSHAMatches(sha, target string) bool {
	sha = strings.TrimSpace(sha)
	target = strings.TrimSpace(target)
	if sha == "" || target == "" {
		return false
	}
	if sha == target {
		return true
	}
	short := shortSHA(sha)
	targetShort := shortSHA(target)
	if short == target || short == targetShort {
		return true
	}
	if strings.HasPrefix(sha, target) || strings.HasPrefix(target, sha) {
		return true
	}
	if len(target) >= 7 && strings.HasPrefix(target, short) {
		return true
	}
	return strings.HasPrefix(sha, targetShort)
}

// findCommitIndex returns the index of the first commit matching id in a
// newest-first list, or -1 if not found.
func findCommitIndex(commits []GithubCommitResponse, id string) int {
	id = strings.TrimSpace(id)
	if id == "" {
		return -1
	}
	for i, c := range commits {
		if commitSHAMatches(c.Sha, id) {
			return i
		}
	}
	return -1
}

// commitDateAt returns the committer/author date for commits[i], if present.
func commitDateAt(commits []GithubCommitResponse, i int) string {
	if i < 0 || i >= len(commits) {
		return ""
	}
	c := commits[i]
	if c.Commit.Committer.Date != "" {
		return c.Commit.Committer.Date
	}
	return c.Commit.Author.Date
}

// remoteIsStrictlyNewer reports whether remote identity is strictly newer than
// currentVersion in a newest-first commit list. ok is false when either side
// cannot be located, so the caller should fall back to another strategy.
func remoteIsStrictlyNewer(commits []GithubCommitResponse, currentVersion, remoteCommit string) (newer, ok bool) {
	remoteIdx := findCommitIndex(commits, strings.TrimSpace(remoteCommit))
	if remoteIdx < 0 {
		return false, false
	}
	curr := normalizeVersionID(currentVersion)
	if curr == "" || curr == "dev" {
		return false, false
	}
	currIdx := findCommitIndex(commits, curr)
	if currIdx < 0 {
		return false, false
	}
	return remoteIdx < currIdx, true
}

// collectNightlyReleaseNotes builds nightly release notes from commits ordered
// newest-first. Notes cover the range from the latest packaged commit (inclusive)
// down to the running version (exclusive). When the running commit is at or
// ahead of the package, notes are empty (no downgrade changelog).
func collectNightlyReleaseNotes(commits []GithubCommitResponse, currentVersion, latestCommit string) (notes string, latestDate string) {
	curr := normalizeVersionID(currentVersion)
	commitSha := strings.TrimSpace(latestCommit)

	start := 0
	startFound := false
	if commitSha != "" {
		if idx := findCommitIndex(commits, commitSha); idx >= 0 {
			start = idx
			startFound = true
		}
	}

	currIdx := -1
	if curr != "" && curr != "dev" {
		currIdx = findCommitIndex(commits, curr)
	}

	latestDate = commitDateAt(commits, start)

	if currIdx >= 0 && currIdx <= start {
		return "", latestDate
	}

	end := len(commits)
	switch {
	case currIdx > start:
		end = currIdx
	case currIdx < 0 && curr != "" && curr != "dev" && startFound:
		return "", latestDate
	case !startFound && currIdx >= 0:
		start = 0
		end = currIdx
		latestDate = commitDateAt(commits, start)
	}

	var noteLines []string
	for i := start; i < end; i++ {
		firstLine := commitSubject(commits[i].Commit.Message)
		if shouldOmitNightlyNote(firstLine) {
			continue
		}
		noteLines = append(noteLines, firstLine)
	}
	return strings.Join(noteLines, "\n"), latestDate
}

func normalizeVersionID(v string) string {
	curr := strings.TrimSpace(v)
	curr = strings.TrimPrefix(curr, "v")
	curr = strings.TrimPrefix(curr, "nightly-")
	return strings.TrimSpace(curr)
}

// looksLikeCommitID reports whether s looks like a git commit identity
// rather than a semver or other label.
func looksLikeCommitID(s string) bool {
	s = normalizeVersionID(s)
	if s == "" || s == "dev" || looksLikeSemver(s) {
		return false
	}
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// versionsMatch reports whether the running binary identity matches a remote version/commit.
func versionsMatch(curr, remoteVersion, remoteCommit string) bool {
	curr = normalizeVersionID(curr)
	if curr == "" {
		return false
	}
	rv := normalizeVersionID(remoteVersion)
	rc := strings.TrimSpace(remoteCommit)
	if rv != "" && (curr == rv || curr == "nightly-"+rv) {
		return true
	}
	if rc != "" {
		if curr == rc {
			return true
		}
		if len(curr) >= 7 && len(curr) < len(rc) && strings.HasPrefix(rc, curr) {
			return true
		}
		if len(rc) >= 7 && len(rc) < len(curr) && strings.HasPrefix(curr, rc) {
			return true
		}
	}
	return false
}

// decideHasUpdate decides whether remote is a real upgrade over current.
// commits may be nil; when provided (newest-first), they resolve commit-order
// cases such as nightly-ahead-of-stable or current-ahead-of-nightly-package.
func decideHasUpdate(current, remoteVersion, remoteCommit string, target *ChannelInfoTarget, commits []GithubCommitResponse) bool {
	if target == nil {
		return false
	}
	if versionsMatch(current, remoteVersion, remoteCommit) {
		return false
	}

	currN := normalizeVersionID(current)
	remoteN := normalizeVersionID(remoteVersion)

	if looksLikeSemver(currN) && looksLikeSemver(remoteN) {
		return utils.CompareVersions(remoteN, currN) > 0
	}

	remoteID := strings.TrimSpace(remoteCommit)
	if remoteID == "" {
		remoteID = remoteN
	}
	if len(commits) > 0 {
		if newer, ok := remoteIsStrictlyNewer(commits, current, remoteID); ok {
			return newer
		}
	}

	if looksLikeCommitID(currN) {
		return false
	}

	return true
}

func infoJSONURL(ch Channel) string {
	return OfficialUpdateBase + "/" + OfficialChannelPath(ch) + "/info.json"
}

func packageURL(ch Channel, version, file string) string {
	version = strings.Trim(version, "/")
	file = strings.Trim(file, "/")
	return OfficialUpdateBase + "/" + OfficialChannelPath(ch) + "/" + version + "/" + file
}

func fetchChannelInfo(ctx context.Context, ch Channel) (*ChannelInfo, error) {
	var info ChannelInfo
	status, err := doJSONGet(ctx, infoJSONURL(ch), "application/json", &info)
	if err != nil {
		if status == http.StatusNotFound {
			return nil, fmt.Errorf("official %s channel has no info.json yet", OfficialChannelPath(ch))
		}
		if status != 0 && status != http.StatusOK {
			return nil, fmt.Errorf("official update host returned %d", status)
		}
		return nil, fmt.Errorf("official update check failed: %w", err)
	}
	if strings.TrimSpace(info.Version) == "" {
		return nil, errors.New("official info.json missing version")
	}
	return &info, nil
}

func findTarget(info *ChannelInfo, goos, goarch string) *ChannelInfoTarget {
	if info == nil {
		return nil
	}
	goos = strings.ToLower(goos)
	goarch = strings.ToLower(goarch)
	for i := range info.Targets {
		t := &info.Targets[i]
		if strings.EqualFold(t.OS, goos) && strings.EqualFold(t.Arch, goarch) {
			return t
		}
	}
	marker := goos + "-" + goarch
	for i := range info.Targets {
		t := &info.Targets[i]
		name := strings.ToLower(t.File)
		if strings.Contains(name, marker) {
			return t
		}
	}
	return nil
}

func checkRelease(ctx context.Context) (*CheckResult, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	info, err := fetchChannelInfo(checkCtx, ChannelRelease)
	if err != nil {
		return nil, err
	}

	target := findTarget(info, runtime.GOOS, runtime.GOARCH)

	downloadURL := ""
	var size int64
	if target != nil {
		downloadURL = packageURL(ChannelRelease, info.Version, target.File)
		size = target.Size
	}

	relNotes := ""
	relDate := info.PublishedAt
	commitSha := strings.TrimSpace(info.Commit)
	var rel GithubReleaseResponse
	tagCandidates := []string{info.Version, "v" + strings.TrimPrefix(info.Version, "v")}
	for _, tag := range tagCandidates {
		url := "https://api.github.com/repos/404Setup/SRC-RenoP/releases/tags/" + tag
		if _, err := doGitHubJSON(checkCtx, url, &rel); err == nil {
			relNotes = rel.Body
			if relNotes == "" {
				relNotes = rel.Name
			}
			if rel.PublishedAt != "" {
				relDate = rel.PublishedAt
			} else if rel.CreatedAt != "" {
				relDate = rel.CreatedAt
			}
			if rel.TargetCommitish != "" {
				commitSha = rel.TargetCommitish
			}
			break
		}
	}
	if relNotes == "" {
		if _, err := doGitHubJSON(checkCtx, "https://api.github.com/repos/404Setup/SRC-RenoP/releases/latest", &rel); err == nil {
			if normalizeVersionID(rel.TagName) == normalizeVersionID(info.Version) {
				relNotes = rel.Body
				if relNotes == "" {
					relNotes = rel.Name
				}
			}
		}
	}
	const maxNotes = 4 << 10
	relNotes = clipString(relNotes, maxNotes)

	var commits []GithubCommitResponse
	currN := normalizeVersionID(version.Version)
	remoteN := normalizeVersionID(info.Version)
	needCommitOrder := target != nil &&
		!versionsMatch(version.Version, info.Version, commitSha) &&
		!(looksLikeSemver(currN) && looksLikeSemver(remoteN))
	if needCommitOrder {
		_, _ = doGitHubJSON(checkCtx, "https://api.github.com/repos/404Setup/SRC-RenoP/commits?sha=main&per_page=30", &commits)
	}
	hasUpdate := decideHasUpdate(version.Version, info.Version, commitSha, target, commits)

	latestVersion := info.Version
	if !strings.HasPrefix(latestVersion, "v") && looksLikeSemver(latestVersion) {
		latestVersion = "v" + latestVersion
	}

	return &CheckResult{
		HasUpdate:          hasUpdate,
		CurrentVersion:     strings.Clone(version.Version),
		LatestVersion:      strings.Clone(latestVersion),
		DownloadUrl:        strings.Clone(downloadURL),
		Channel:            string(ChannelRelease),
		Size:               size,
		EstimatedDiskSpace: size * 3,
		ReleaseDate:        strings.Clone(relDate),
		ReleaseNotes:       strings.Clone(relNotes),
		CommitSha:          strings.Clone(commitSha),
		IsRelease:          true,
	}, nil
}

func looksLikeSemver(s string) bool {
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return false
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for i := 0; i < len(p); i++ {
			c := p[i]
			if c < '0' || c > '9' {
				if i > 0 {
					break
				}
				return false
			}
		}
	}
	return true
}

func checkNightly(ctx context.Context) (*CheckResult, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	info, err := fetchChannelInfo(checkCtx, ChannelNightly)
	if err != nil {
		return nil, err
	}

	target := findTarget(info, runtime.GOOS, runtime.GOARCH)

	downloadURL := ""
	var size int64
	if target != nil {
		downloadURL = packageURL(ChannelNightly, info.Version, target.File)
		size = target.Size
	}

	commitSha := strings.TrimSpace(info.Commit)

	var commits []GithubCommitResponse
	releaseNotes := ""
	commitDate := info.PublishedAt
	if _, err := doGitHubJSON(checkCtx, "https://api.github.com/repos/404Setup/SRC-RenoP/commits?sha=main&per_page=30", &commits); err == nil && len(commits) > 0 {
		var noteDate string
		releaseNotes, noteDate = collectNightlyReleaseNotes(commits, version.Version, commitSha)
		if noteDate != "" {
			commitDate = noteDate
		}
	}
	const maxNotes = 4 << 10
	releaseNotes = clipString(releaseNotes, maxNotes)

	hasUpdate := decideHasUpdate(version.Version, info.Version, commitSha, target, commits)

	latestVersion := info.Version
	if !strings.HasPrefix(latestVersion, "nightly-") {
		latestVersion = "nightly-" + normalizeVersionID(latestVersion)
	}

	return &CheckResult{
		HasUpdate:          hasUpdate,
		CurrentVersion:     strings.Clone(version.Version),
		LatestVersion:      strings.Clone(latestVersion),
		DownloadUrl:        strings.Clone(downloadURL),
		Channel:            string(ChannelNightly),
		Size:               size,
		EstimatedDiskSpace: size * 3,
		ReleaseDate:        strings.Clone(commitDate),
		ReleaseNotes:       strings.Clone(releaseNotes),
		CommitSha:          strings.Clone(commitSha),
		IsRelease:          false,
	}, nil
}
