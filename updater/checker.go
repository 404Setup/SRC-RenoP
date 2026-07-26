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

const maxGitHubAPIBody = 1 << 20

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

func doGitHubJSON(ctx context.Context, url string, dst any) (statusCode int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "RenoP-Updater")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

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

	if resp.ContentLength > maxGitHubAPIBody {
		utils.AbortHTTPResponse(resp, capConn.Conn())
		return statusCode, fmt.Errorf("response too large: Content-Length=%d", resp.ContentLength)
	}

	data, err := utils.ReadAllLimited(resp.Body, maxGitHubAPIBody)
	utils.AbortHTTPResponse(resp, capConn.Conn())
	if err != nil {
		if errors.Is(err, utils.ErrResponseTooLarge) {
			return statusCode, fmt.Errorf("response exceeds %d bytes", maxGitHubAPIBody)
		}
		return statusCode, err
	}
	if err := sonic.ConfigFastest.Unmarshal(data, dst); err != nil {
		return statusCode, err
	}
	return statusCode, nil
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

func checkRelease(ctx context.Context) (*CheckResult, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var rel GithubReleaseResponse
	if status, err := doGitHubJSON(checkCtx, "https://api.github.com/repos/404Setup/SRC-RenoP/releases/latest", &rel); err != nil {
		if status != 0 && status != http.StatusOK {
			return nil, fmt.Errorf("GitHub release check failed: %d", status)
		}
		return nil, fmt.Errorf("GitHub release check failed: %w", err)
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	curr := strings.TrimPrefix(version.Version, "v")
	hasUpdate := latest != curr && latest != ""

	targetAsset := ""
	var assetSize int64
	osName := runtime.GOOS
	archName := runtime.GOARCH

	for i := range rel.Assets {
		asset := &rel.Assets[i]
		nameLower := strings.ToLower(asset.Name)
		if strings.Contains(nameLower, osName) && strings.Contains(nameLower, archName) {
			targetAsset = asset.BrowserDownloadUrl
			assetSize = asset.Size
			break
		}
	}

	relDate := rel.PublishedAt
	if relDate == "" {
		relDate = rel.CreatedAt
	}

	relNotes := rel.Body
	if relNotes == "" {
		relNotes = rel.Name
	}
	const maxNotes = 32 << 10
	relNotes = clipString(relNotes, maxNotes)

	return &CheckResult{
		HasUpdate:          hasUpdate,
		CurrentVersion:     version.Version,
		LatestVersion:      rel.TagName,
		DownloadUrl:        targetAsset,
		Channel:            string(ChannelRelease),
		Size:               assetSize,
		EstimatedDiskSpace: assetSize * 3,
		ReleaseDate:        relDate,
		ReleaseNotes:       relNotes,
		CommitSha:          rel.TargetCommitish,
		IsRelease:          true,
	}, nil
}

func checkNightly(ctx context.Context) (*CheckResult, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var commits []GithubCommitResponse
	if status, err := doGitHubJSON(checkCtx, "https://api.github.com/repos/404Setup/SRC-RenoP/commits?sha=main&per_page=100", &commits); err != nil {
		if status != 0 && status != http.StatusOK {
			return nil, fmt.Errorf("GitHub commit check failed: %d", status)
		}
		return nil, fmt.Errorf("GitHub commit check failed: %w", err)
	}
	if len(commits) == 0 {
		return nil, errors.New("GitHub returned no commits")
	}

	var latestCommit *GithubCommitResponse
	for i := range commits {
		subject := commitSubject(commits[i].Commit.Message)
		if isWebOnlyCommit(subject) {
			continue
		}
		latestCommit = &commits[i]
		break
	}
	if latestCommit == nil {
		return nil, errors.New("GitHub returned no product commits (all [web]?)")
	}

	commitSha := latestCommit.Sha
	shortSha := commitSha
	if len(shortSha) > 7 {
		shortSha = shortSha[:7]
	}

	commitDate := latestCommit.Commit.Committer.Date
	if commitDate == "" {
		commitDate = latestCommit.Commit.Author.Date
	}

	curr := strings.TrimPrefix(version.Version, "v")
	curr = strings.TrimPrefix(curr, "nightly-")
	curr = strings.TrimSpace(curr)

	hasUpdate := true
	if curr == shortSha || curr == commitSha || curr == "nightly-"+shortSha {
		hasUpdate = false
	}

	var notes []string
	for _, c := range commits {
		sha := c.Sha
		short := sha
		if len(short) > 7 {
			short = short[:7]
		}

		if curr != "" && curr != "dev" {
			if curr == sha || curr == short || strings.HasPrefix(sha, curr) || strings.HasPrefix(curr, short) {
				break
			}
		}

		firstLine := commitSubject(c.Commit.Message)
		if shouldOmitNightlyNote(firstLine) {
			continue
		}
		notes = append(notes, firstLine)
	}
	releaseNotes := strings.Join(notes, "\n")
	const maxNotes = 32 << 10
	releaseNotes = clipString(releaseNotes, maxNotes)

	isRelease := false
	var releases []GithubReleaseItem
	if _, err := doGitHubJSON(checkCtx, "https://api.github.com/repos/404Setup/SRC-RenoP/releases?per_page=5", &releases); err == nil {
		for i := range releases {
			r := &releases[i]
			if r.TargetCommitish == commitSha || strings.HasPrefix(commitSha, strings.TrimPrefix(r.TagName, "v")) {
				isRelease = true
				break
			}
		}
	}

	downloadUrl := "https://nightly.link/404Setup/SRC-RenoP/workflows/build/main/renop-nightly.zip"
	var size int64

	latestVersion := "nightly-" + shortSha

	return &CheckResult{
		HasUpdate:          hasUpdate,
		CurrentVersion:     version.Version,
		LatestVersion:      latestVersion,
		DownloadUrl:        downloadUrl,
		Channel:            string(ChannelNightly),
		Size:               size,
		EstimatedDiskSpace: size * 3,
		ReleaseDate:        commitDate,
		ReleaseNotes:       releaseNotes,
		CommitSha:          commitSha,
		IsRelease:          isRelease,
	}, nil
}
