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
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/bytedance/sonic"

	"renop/utils"
	"renop/version"
)

const maxRemoteJSONBody = 1 << 20

var checkHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return utils.DialContextLimited(utils.LimitedDialer(8*time.Second, 30*time.Second), ctx, network, addr)
		},
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   2,
		MaxConnsPerHost:       4,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
		ExpectContinueTimeout: time.Second,
		DisableKeepAlives:     true,
	},
}

func CheckUpdate(ctx context.Context, channel Channel) (*CheckResult, error) {
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
	var capConn utils.ConnCapture
	req = req.WithContext(utils.WithConnCapture(req.Context(), &capConn))

	resp, err := checkHTTPClient.Do(req)
	if err != nil {
		utils.ForceTCPAbort(capConn.Conn())
		return 0, err
	}

	statusCode = resp.StatusCode
	if statusCode != http.StatusOK {
		utils.AbortHTTPResponse(resp, capConn.Conn())
		return statusCode, fmt.Errorf("status %d", statusCode)
	}

	if resp.ContentLength > maxRemoteJSONBody {
		utils.AbortHTTPResponse(resp, capConn.Conn())
		return statusCode, fmt.Errorf("response too large: Content-Length=%d", resp.ContentLength)
	}

	data, err := utils.ReadAllLimited(resp.Body, maxRemoteJSONBody)
	utils.AbortHTTPResponse(resp, capConn.Conn())
	if err != nil {
		if errors.Is(err, utils.ErrResponseTooLarge) {
			return statusCode, fmt.Errorf("response exceeds %d bytes", maxRemoteJSONBody)
		}
		return statusCode, err
	}
	if err := sonic.ConfigFastest.Unmarshal(data, dst); err != nil {
		return statusCode, err
	}
	return statusCode, nil
}

func doGitHubJSON(ctx context.Context, url string, dst any) (statusCode int, err error) {
	return doJSONGet(ctx, url, "application/vnd.github.v3+json", dst)
}

func clipString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return strings.Clone(s[:max])
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

// collectNightlyReleaseNotes builds nightly release notes from commits ordered
// newest-first.
func collectNightlyReleaseNotes(commits []GithubCommitResponse, currentVersion, latestCommit string) (notes string, latestDate string) {
	curr := normalizeVersionID(currentVersion)
	commitSha := strings.TrimSpace(latestCommit)

	start := 0
	if commitSha != "" {
		for i, c := range commits {
			if commitSHAMatches(c.Sha, commitSha) {
				start = i
				break
			}
		}
	}

	end := len(commits)
	if curr != "" && curr != "dev" {
		for i := start; i < len(commits); i++ {
			if commitSHAMatches(commits[i].Sha, curr) {
				end = i
				break
			}
		}
	}

	if start < len(commits) {
		c := commits[start]
		if c.Commit.Committer.Date != "" {
			latestDate = c.Commit.Committer.Date
		} else if c.Commit.Author.Date != "" {
			latestDate = c.Commit.Author.Date
		}
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
	matched := versionsMatch(version.Version, info.Version, info.Commit)
	hasUpdate := !matched && target != nil

	downloadURL := ""
	var size int64
	if target != nil {
		downloadURL = packageURL(ChannelRelease, info.Version, target.File)
		size = target.Size
	}

	relNotes := ""
	relDate := info.PublishedAt
	commitSha := info.Commit
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
	const maxNotes = 32 << 10
	relNotes = clipString(relNotes, maxNotes)

	latestVersion := info.Version
	if !strings.HasPrefix(latestVersion, "v") && looksLikeSemver(latestVersion) {
		latestVersion = "v" + latestVersion
	}

	return &CheckResult{
		HasUpdate:          hasUpdate,
		CurrentVersion:     version.Version,
		LatestVersion:      latestVersion,
		DownloadUrl:        downloadURL,
		Channel:            string(ChannelRelease),
		Size:               size,
		EstimatedDiskSpace: size * 3,
		ReleaseDate:        relDate,
		ReleaseNotes:       relNotes,
		CommitSha:          commitSha,
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
	matched := versionsMatch(version.Version, info.Version, info.Commit)
	hasUpdate := !matched && target != nil

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
	if _, err := doGitHubJSON(checkCtx, "https://api.github.com/repos/404Setup/SRC-RenoP/commits?sha=main&per_page=100", &commits); err == nil && len(commits) > 0 {
		// From the update-check latest package commit down to the running version
		// (not from main HEAD / prefix-filtered tip down to current).
		var noteDate string
		releaseNotes, noteDate = collectNightlyReleaseNotes(commits, version.Version, commitSha)
		if noteDate != "" {
			commitDate = noteDate
		}
	}
	const maxNotes = 32 << 10
	releaseNotes = clipString(releaseNotes, maxNotes)

	latestVersion := info.Version
	if !strings.HasPrefix(latestVersion, "nightly-") {
		latestVersion = "nightly-" + normalizeVersionID(latestVersion)
	}

	return &CheckResult{
		HasUpdate:          hasUpdate,
		CurrentVersion:     version.Version,
		LatestVersion:      latestVersion,
		DownloadUrl:        downloadURL,
		Channel:            string(ChannelNightly),
		Size:               size,
		EstimatedDiskSpace: size * 3,
		ReleaseDate:        commitDate,
		ReleaseNotes:       releaseNotes,
		CommitSha:          commitSha,
		IsRelease:          false,
	}, nil
}
