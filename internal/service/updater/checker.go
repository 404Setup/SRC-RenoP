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
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/klauspost/cpuid/v2"

	"renop/internal/utils"
	"renop/internal/version"
)

const maxRemoteJSONBody = 1 << 20

var sharedCheckClient lazyHTTPClient

func newCheckHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: checkHTTPSRedirect,
		Transport:     newCheckTransport(),
		Timeout:       15 * time.Second,
	}
}

// checkHTTPClient returns the process-wide bounded client used by update checks.
// The transport is initialized only when an update operation first needs it.
func checkHTTPClient() *http.Client {
	sharedCheckClient.Do(func() {
		sharedCheckClient.client = newCheckHTTPClient()
	})
	return sharedCheckClient.client
}

func checkHTTPSRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if req.URL.Scheme != "https" || req.URL.Host == "" || req.URL.User != nil {
		return errors.New("update redirect must be an HTTPS URL without user info")
	}
	return nil
}

func newCheckTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		ForceAttemptHTTP2:      false,
		DisableCompression:     true,
		DisableKeepAlives:      false,
		MaxIdleConns:           2,
		MaxIdleConnsPerHost:    1,
		MaxConnsPerHost:        2,
		IdleConnTimeout:        15 * time.Second,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  15 * time.Second,
		ExpectContinueTimeout:  1 * time.Second,
		MaxResponseHeaderBytes: 256 << 10,
	}
}

func CheckUpdate(ctx context.Context, channel Channel) (*CheckResult, error) {
	client := checkHTTPClient()
	defer utils.ScheduleNetworkWorkingSetTrim()

	if channel == ChannelNightly {
		return checkNightly(ctx, client)
	}
	return checkRelease(ctx, client)
}

func doJSONGet(ctx context.Context, client *http.Client, url string, accept string, dst any) (statusCode int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "RenoP-Updater")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if client == nil {
		client = checkHTTPClient()
	}

	resp, err := client.Do(req)
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

	if err := decodeJSONLimited(resp.Body, resp.ContentLength, maxRemoteJSONBody, dst); err != nil {
		if errors.Is(err, utils.ErrResponseTooLarge) {
			return statusCode, fmt.Errorf("response exceeds %d bytes", maxRemoteJSONBody)
		}
		return statusCode, err
	}
	return statusCode, nil
}

func decodeJSONLimited(r io.Reader, contentLength, max int64, dst any) error {
	if max < 0 || contentLength > max {
		return utils.ErrResponseTooLarge
	}

	lr := &io.LimitedReader{R: r, N: max + 1}
	dec := json.NewDecoder(lr)
	if err := dec.Decode(dst); err != nil {
		if lr.N <= 0 {
			return utils.ErrResponseTooLarge
		}
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if lr.N <= 0 {
			return utils.ErrResponseTooLarge
		}
		if err == nil {
			return errors.New("unexpected trailing data after JSON object")
		}
		if _, readErr := io.Copy(io.Discard, lr); readErr != nil || lr.N <= 0 {
			return utils.ErrResponseTooLarge
		}
		return err
	}
	if _, readErr := io.Copy(io.Discard, lr); readErr != nil || lr.N <= 0 {
		return utils.ErrResponseTooLarge
	}
	return nil
}

func doGitHubJSON(ctx context.Context, client *http.Client, url string, dst any) (statusCode int, err error) {
	return doJSONGet(ctx, client, url, "application/vnd.github.v3+json", dst)
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

const githubRepoAPI = "https://api.github.com/repos/404Setup/SRC-RenoP"

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

func githubCommitExists(ctx context.Context, client *http.Client, sha string) (exists bool, checked bool) {
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return false, false
	}
	url := githubRepoAPI + "/commits/" + sha
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, false
	}
	req.Header.Set("User-Agent", "RenoP-Updater")
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if client == nil {
		client = checkHTTPClient()
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, false
	}
	defer utils.DrainAndClose(resp.Body)

	return mapGithubCommitStatus(resp.StatusCode)
}

func mapGithubCommitStatus(statusCode int) (exists bool, checked bool) {
	switch statusCode {
	case http.StatusOK:
		return true, true
	case http.StatusNotFound, http.StatusUnprocessableEntity:
		return false, true
	default:
		return false, false
	}
}

func resolveCurrentCommit(ctx context.Context, client *http.Client, current string, commits []GithubCommitResponse) bool {
	curr := normalizeVersionID(current)
	if curr == "" || curr == "dev" {
		return false
	}
	if findCommitIndex(commits, curr) >= 0 {
		return false
	}
	if !looksLikeCommitID(curr) {
		return false
	}
	exists, _ := githubCommitExists(ctx, client, curr)
	return exists
}

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

func collectNightlyReleaseNotes(commits []GithubCommitResponse, currentVersion, latestCommit string, currentExistsOutside bool) (notes string, latestDate string) {
	return collectNightlyReleaseNotesForBuild(commits, currentVersion, "", latestCommit, currentExistsOutside)
}

func collectNightlyReleaseNotesForBuild(commits []GithubCommitResponse, currentVersion, currentCommit, latestCommit string, currentExistsOutside bool) (notes string, latestDate string) {
	curr := normalizeVersionID(currentVersion)
	if looksLikeCommitID(currentCommit) {
		curr = normalizeVersionID(currentCommit)
	}
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
	case currIdx < 0 && currentExistsOutside:
		if !startFound {
			start = 0
			latestDate = commitDateAt(commits, start)
		}
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

func commitIdentitiesMatch(left, right string) bool {
	left = normalizeVersionID(left)
	right = normalizeVersionID(right)
	if len(left) < 7 || len(right) < 7 {
		return false
	}
	return left == right || strings.HasPrefix(left, right) || strings.HasPrefix(right, left)
}

func releaseMatchesBuild(release ChannelInfoRelease, currentVersion, currentCommit string) bool {
	return versionsMatch(currentVersion, release.Version, release.Commit) ||
		(commitIdentitiesMatch(currentCommit, release.Commit) && looksLikeCommitID(currentCommit))
}

func findReleaseIndexForBuild(releases []ChannelInfoRelease, currentVersion, currentCommit string) int {
	for i, r := range releases {
		if releaseMatchesBuild(r, currentVersion, currentCommit) {
			return i
		}
	}
	return -1
}

func releaseRangeBoundary(releases []ChannelInfoRelease, currentVersion, currentCommit string) (int, bool) {
	if index := findReleaseIndexForBuild(releases, currentVersion, currentCommit); index >= 0 {
		return index, true
	}
	if looksLikeCommitID(currentCommit) {
		for index, release := range releases {
			if commitIdentitiesMatch(currentCommit, release.PreviousCommit) {
				return index + 1, true
			}
		}
	}
	currentNormalized := normalizeVersionID(currentVersion)
	if looksLikeSemver(currentNormalized) {
		for index, release := range releases {
			releaseVersion := normalizeVersionID(release.Version)
			if looksLikeSemver(releaseVersion) && utils.CompareVersions(releaseVersion, currentNormalized) <= 0 {
				return index, true
			}
		}
	}
	return len(releases), false
}

func collectAvailableReleaseNotes(releases []ChannelInfoRelease, currentVersion, currentCommit string) string {
	end, _ := releaseRangeBoundary(releases, currentVersion, currentCommit)
	end = min(max(end, 0), len(releases))
	if end == 0 {
		return ""
	}
	var notesBuilder strings.Builder
	notesBuilder.Grow(min(maxRemoteJSONBody, end*256))
	for index := end - 1; index >= 0; index-- {
		release := releases[index]
		if notesBuilder.Len() > 0 {
			notesBuilder.WriteString("\n\n")
		}
		notesBuilder.WriteString("## ")
		notesBuilder.WriteString(strings.TrimSpace(release.Version))
		if notes := strings.TrimSpace(release.Changelog); notes != "" {
			notesBuilder.WriteByte('\n')
			notesBuilder.WriteString(notes)
		}
	}
	return clipString(notesBuilder.String(), maxRemoteJSONBody)
}

func decideHasUpdate(current, remoteVersion, remoteCommit string, target *ChannelInfoTarget, infoReleases []ChannelInfoRelease, commits []GithubCommitResponse, currentExistsOutside bool) bool {
	return decideHasUpdateForBuild(current, "", remoteVersion, remoteCommit, target, infoReleases, commits, currentExistsOutside)
}

func decideHasUpdateForBuild(current, currentCommit, remoteVersion, remoteCommit string, target *ChannelInfoTarget, infoReleases []ChannelInfoRelease, commits []GithubCommitResponse, currentExistsOutside bool) bool {
	if target == nil {
		return false
	}
	if versionsMatch(current, remoteVersion, remoteCommit) ||
		(looksLikeCommitID(currentCommit) && commitIdentitiesMatch(currentCommit, remoteCommit)) {
		return false
	}

	currN := normalizeVersionID(current)
	remoteN := normalizeVersionID(remoteVersion)

	if looksLikeSemver(currN) && looksLikeSemver(remoteN) {
		return utils.CompareVersions(remoteN, currN) > 0
	}

	if len(infoReleases) > 0 {
		if boundary, found := releaseRangeBoundary(infoReleases, current, currentCommit); found {
			return boundary > 0
		}
	}

	remoteID := strings.TrimSpace(remoteCommit)
	if remoteID == "" {
		remoteID = remoteN
	}
	if len(commits) > 0 {
		graphCurrent := current
		if looksLikeCommitID(currentCommit) {
			graphCurrent = currentCommit
		}
		if newer, ok := remoteIsStrictlyNewer(commits, graphCurrent, remoteID); ok {
			return newer
		}
		if currentExistsOutside && findCommitIndex(commits, remoteID) >= 0 {
			return true
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

func fetchChannelInfo(ctx context.Context, client *http.Client, ch Channel) (*ChannelInfo, error) {
	var info ChannelInfo
	status, err := doJSONGet(ctx, client, infoJSONURL(ch), "application/json", &info)
	if err != nil {
		if status == http.StatusNotFound {
			return nil, fmt.Errorf("official %s channel has no info.json yet", OfficialChannelPath(ch))
		}
		if status != 0 && status != http.StatusOK {
			return nil, fmt.Errorf("official update host returned %d", status)
		}
		return nil, fmt.Errorf("official update check failed: %w", err)
	}
	if len(info.Releases) == 0 || strings.TrimSpace(info.Releases[0].Version) == "" {
		return nil, errors.New("official info.json missing releases")
	}
	return &info, nil
}

func getHostAMD64Level() int {
	if runtime.GOARCH != "amd64" {
		return 0
	}
	lvl := cpuid.CPU.X64Level()
	if lvl < 1 {
		return 1
	}
	if lvl > 4 {
		return 4
	}
	return lvl
}

func matchAMD64TargetForLevel(targets []ChannelInfoTarget, goos string, level int) *ChannelInfoTarget {
	targetArch := "amd64"
	if level > 1 {
		targetArch = fmt.Sprintf("amd64v%d", level)
	}

	for i := range targets {
		t := &targets[i]
		if strings.EqualFold(t.OS, goos) {
			if level == 1 {
				if strings.EqualFold(t.Arch, "amd64") || strings.EqualFold(t.Arch, "amd64v1") {
					return t
				}
			} else {
				if strings.EqualFold(t.Arch, targetArch) {
					return t
				}
			}
		}
	}

	for i := range targets {
		t := &targets[i]
		if !strings.EqualFold(t.OS, goos) {
			continue
		}
		name := strings.ToLower(t.File)
		if level == 1 {
			if strings.Contains(name, goos+"-amd64v1") ||
				(strings.Contains(name, goos+"-amd64") &&
					!strings.Contains(name, "amd64v2") &&
					!strings.Contains(name, "amd64v3") &&
					!strings.Contains(name, "amd64v4")) {
				return t
			}
		} else {
			if strings.Contains(name, goos+"-"+targetArch) {
				return t
			}
		}
	}

	return nil
}

func findTarget(info *ChannelInfo, goos, goarch string) *ChannelInfoTarget {
	if info == nil || len(info.Releases) == 0 {
		return nil
	}
	targets := info.Releases[0].Targets
	goos = strings.ToLower(goos)
	goarch = strings.ToLower(goarch)

	if goarch == "amd64" {
		level := getHostAMD64Level()
		for l := level; l >= 1; l-- {
			if t := matchAMD64TargetForLevel(targets, goos, l); t != nil {
				return t
			}
		}
		return nil
	}

	for i := range targets {
		t := &targets[i]
		if strings.EqualFold(t.OS, goos) && strings.EqualFold(t.Arch, goarch) {
			return t
		}
	}
	marker := goos + "-" + goarch
	for i := range targets {
		t := &targets[i]
		name := strings.ToLower(t.File)
		if strings.Contains(name, marker) {
			return t
		}
	}
	return nil
}

func estimateTargetDiskSpace(target *ChannelInfoTarget) int64 {
	if target == nil || target.Size <= 0 {
		return 0
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	if target.UncompressedSize > 0 {
		if target.UncompressedSize > (maxInt64-target.Size)/2 {
			return maxInt64
		}
		return target.Size + target.UncompressedSize*2
	}
	if target.Size > maxInt64/3 {
		return maxInt64
	}
	return target.Size * 3
}

func checkRelease(ctx context.Context, client *http.Client) (*CheckResult, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	info, err := fetchChannelInfo(checkCtx, client, ChannelRelease)
	if err != nil {
		return nil, err
	}

	latestRel := &info.Releases[0]
	target := findTarget(info, runtime.GOOS, runtime.GOARCH)

	downloadURL := ""
	var size int64
	var sha256Digest string
	if target != nil {
		if target.DownloadURL != "" {
			downloadURL = target.DownloadURL
		} else {
			downloadURL = packageURL(ChannelRelease, latestRel.Version, target.File)
		}
		size = target.Size
		sha256Digest = strings.TrimSpace(target.SHA256)
	}

	relNotes := latestRel.Changelog
	relDate := latestRel.PublishedAt
	commitSha := strings.TrimSpace(latestRel.Commit)

	if relNotes == "" {
		var rel GithubReleaseResponse
		tagCandidates := []string{latestRel.Version, "v" + strings.TrimPrefix(latestRel.Version, "v")}
		for _, tag := range tagCandidates {
			url := "https://api.github.com/repos/404Setup/SRC-RenoP/releases/tags/" + tag
			if _, err := doGitHubJSON(checkCtx, client, url, &rel); err == nil {
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
			if _, err := doGitHubJSON(checkCtx, client, "https://api.github.com/repos/404Setup/SRC-RenoP/releases/latest", &rel); err == nil {
				if normalizeVersionID(rel.TagName) == normalizeVersionID(latestRel.Version) {
					relNotes = rel.Body
					if relNotes == "" {
						relNotes = rel.Name
					}
				}
			}
		}
	}
	latestRel.Changelog = strings.Clone(relNotes)

	var commits []GithubCommitResponse
	currentCommit := strings.TrimSpace(version.Commit)
	if !looksLikeCommitID(currentCommit) {
		currentCommit = ""
	}
	currN := normalizeVersionID(version.Version)
	remoteN := normalizeVersionID(latestRel.Version)
	needCommitOrder := target != nil &&
		!releaseMatchesBuild(*latestRel, version.Version, currentCommit) &&
		!(looksLikeSemver(currN) && looksLikeSemver(remoteN)) &&
		findReleaseIndexForBuild(info.Releases, version.Version, currentCommit) < 0

	currentExistsOutside := false
	if needCommitOrder {
		_, _ = doGitHubJSON(checkCtx, client, githubRepoAPI+"/commits?sha=main&per_page=30", &commits)
		if len(commits) > 0 {
			currentIdentity := version.Version
			if currentCommit != "" {
				currentIdentity = currentCommit
			}
			currentExistsOutside = resolveCurrentCommit(checkCtx, client, currentIdentity, commits)
		}
	}
	hasUpdate := decideHasUpdateForBuild(version.Version, currentCommit, latestRel.Version, commitSha, target, info.Releases, commits, currentExistsOutside)
	if hasUpdate {
		if combined := collectAvailableReleaseNotes(info.Releases, version.Version, currentCommit); combined != "" {
			relNotes = combined
		}
	} else {
		relNotes = ""
	}
	relNotes = clipString(relNotes, maxRemoteJSONBody)

	latestVersion := latestRel.Version
	if !strings.HasPrefix(latestVersion, "v") && looksLikeSemver(latestVersion) {
		latestVersion = "v" + latestVersion
	}

	return &CheckResult{
		HasUpdate:          hasUpdate,
		CurrentVersion:     strings.Clone(version.Version),
		LatestVersion:      strings.Clone(latestVersion),
		DownloadURL:        strings.Clone(downloadURL),
		Channel:            string(ChannelRelease),
		Size:               size,
		EstimatedDiskSpace: estimateTargetDiskSpace(target),
		ReleaseDate:        strings.Clone(relDate),
		ReleaseNotes:       strings.Clone(relNotes),
		CommitSha:          strings.Clone(commitSha),
		SHA256:             strings.Clone(sha256Digest),
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

func checkNightly(ctx context.Context, client *http.Client) (*CheckResult, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	info, err := fetchChannelInfo(checkCtx, client, ChannelNightly)
	if err != nil {
		return nil, err
	}

	latestRel := &info.Releases[0]
	target := findTarget(info, runtime.GOOS, runtime.GOARCH)

	downloadURL := ""
	var size int64
	var sha256Digest string
	if target != nil {
		if target.DownloadURL != "" {
			downloadURL = target.DownloadURL
		} else {
			downloadURL = packageURL(ChannelNightly, latestRel.Version, target.File)
		}
		size = target.Size
		sha256Digest = strings.TrimSpace(target.SHA256)
	}

	commitSha := strings.TrimSpace(latestRel.Commit)
	currentCommit := strings.TrimSpace(version.Commit)
	if !looksLikeCommitID(currentCommit) {
		currentCommit = ""
	}

	var commits []GithubCommitResponse
	releaseNotes := collectAvailableReleaseNotes(info.Releases, version.Version, currentCommit)
	commitDate := latestRel.PublishedAt
	currentExistsOutside := false
	_, releaseBoundaryFound := releaseRangeBoundary(info.Releases, version.Version, currentCommit)

	needCommitOrder := target != nil &&
		!releaseMatchesBuild(*latestRel, version.Version, currentCommit) &&
		!releaseBoundaryFound

	if releaseNotes == "" || needCommitOrder {
		if _, err := doGitHubJSON(checkCtx, client, githubRepoAPI+"/commits?sha=main&per_page=30", &commits); err == nil && len(commits) > 0 {
			currentIdentity := version.Version
			if currentCommit != "" {
				currentIdentity = currentCommit
			}
			currentExistsOutside = resolveCurrentCommit(checkCtx, client, currentIdentity, commits)
			if releaseNotes == "" {
				var noteDate string
				releaseNotes, noteDate = collectNightlyReleaseNotesForBuild(commits, version.Version, currentCommit, commitSha, currentExistsOutside)
				if noteDate != "" {
					commitDate = noteDate
				}
			}
		}
	}

	hasUpdate := decideHasUpdateForBuild(version.Version, currentCommit, latestRel.Version, commitSha, target, info.Releases, commits, currentExistsOutside)
	if !hasUpdate {
		releaseNotes = ""
	}
	releaseNotes = clipString(releaseNotes, maxRemoteJSONBody)

	latestVersion := latestRel.Version
	if !strings.HasPrefix(latestVersion, "nightly-") {
		latestVersion = "nightly-" + normalizeVersionID(latestVersion)
	}

	return &CheckResult{
		HasUpdate:          hasUpdate,
		CurrentVersion:     strings.Clone(version.Version),
		LatestVersion:      strings.Clone(latestVersion),
		DownloadURL:        strings.Clone(downloadURL),
		Channel:            string(ChannelNightly),
		Size:               size,
		EstimatedDiskSpace: estimateTargetDiskSpace(target),
		ReleaseDate:        strings.Clone(commitDate),
		ReleaseNotes:       strings.Clone(releaseNotes),
		CommitSha:          strings.Clone(commitSha),
		SHA256:             strings.Clone(sha256Digest),
		IsRelease:          false,
	}, nil
}
