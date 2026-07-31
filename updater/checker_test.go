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
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"renop/config"
	"renop/core"
	"renop/utils"
)

func largeBodyHandler(size int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(size))
		w.WriteHeader(http.StatusOK)
		chunk := bytes.Repeat([]byte("A"), 64*1024)
		left := size
		for left > 0 {
			n := min(len(chunk), left)
			_, _ = w.Write(chunk[:n])
			left -= n
		}
	})
}

func TestDoJSONGetRejectsOversizedContentLength(t *testing.T) {
	const size = 40 << 20
	ts := httptest.NewServer(largeBodyHandler(size))
	t.Cleanup(ts.Close)

	var dst map[string]any
	_, err := doJSONGet(context.Background(), ts.URL, "application/json", &dst)
	if err == nil {
		t.Fatal("expected error for oversized Content-Length")
	}
	if !strings.Contains(err.Error(), "too large") && !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size-limit error, got: %v", err)
	}
}

func TestDoGitHubJSONOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"tag_name":"v1.2.3","name":"rel"}`)
	}))
	t.Cleanup(ts.Close)

	var rel GithubReleaseResponse
	status, err := doGitHubJSON(context.Background(), ts.URL, &rel)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if status != 200 {
		t.Fatalf("status=%d", status)
	}
	if rel.TagName != "v1.2.3" {
		t.Fatalf("tag=%q", rel.TagName)
	}
}

func TestClipStringDoesNotRetainFullBacking(t *testing.T) {
	big := strings.Repeat("n", 4<<20)
	clipped := clipString(big, 64)
	if len(clipped) != 64 {
		t.Fatalf("len=%d", len(clipped))
	}
	big = ""
	runtime.GC()
	if clipped != strings.Repeat("n", 64) {
		t.Fatal("clip content mismatch")
	}
}

func TestClipStringSmallDoesNotRetainFullBacking(t *testing.T) {
	big := strings.Repeat("a", 1<<20)
	sub := big[:10]
	clipped := clipString(sub, 32768)
	if clipped != "aaaaaaaaaa" {
		t.Fatalf("got %q", clipped)
	}
	big = ""
	sub = ""
	runtime.GC()
	if clipped != "aaaaaaaaaa" {
		t.Fatal("clip content mismatch")
	}
}

func TestDoJSONGetDoesNotRetainResponseBuffer(t *testing.T) {
	padding := strings.Repeat("x", 500<<10)
	payload := `{"version":"1.2.3","commit":"` + padding + `"}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, payload)
	}))
	t.Cleanup(ts.Close)

	var info ChannelInfo
	status, err := doJSONGet(context.Background(), ts.URL, "application/json", &info)
	if err != nil || status != 200 {
		t.Fatalf("doJSONGet failed: status=%d, err=%v", status, err)
	}
	if info.Version != "1.2.3" {
		t.Fatalf("version=%q", info.Version)
	}

	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
}

func TestCheckUpdateClosesIdleConnsAndFreesMemory(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"version":"1.0.0","commit":"abc1234567890"}`)
	}))
	t.Cleanup(ts.Close)

	before := currentCheckHTTPClient()
	var info ChannelInfo
	status, err := doJSONGet(context.Background(), ts.URL, "application/json", &info)
	if err != nil || status != 200 {
		t.Fatalf("status=%d err=%v", status, err)
	}

	recycleCheckHTTPClient()
	after := currentCheckHTTPClient()
	if after == nil {
		t.Fatal("expected a live check HTTP client after recycle")
	}
	if before != nil && after == before {
		t.Fatal("expected check HTTP client to be replaced after recycle")
	}
	utils.ReleaseMemoryToOS()
}

func TestCommitSubject(t *testing.T) {
	if got := commitSubject("fix\n\nbody"); got != "fix" {
		t.Fatalf("got %q", got)
	}
	if got := commitSubject("  single  "); got != "single" {
		t.Fatalf("got %q", got)
	}
}

func TestIsWebOnlyCommit(t *testing.T) {
	cases := []struct {
		subject string
		want    bool
	}{
		{subject: "[web] update homepage", want: true},
		{subject: "[Web] docs", want: true},
		{subject: "[WEB] i18n", want: true},
		{subject: "fix: not web", want: false},
		{subject: "prefix [web] middle", want: false},
		{subject: "", want: false},
	}
	for _, tc := range cases {
		if got := isWebOnlyCommit(tc.subject); got != tc.want {
			t.Fatalf("isWebOnlyCommit(%q)=%v want %v", tc.subject, got, tc.want)
		}
	}
}

func TestShouldOmitNightlyNote(t *testing.T) {
	omit := []string{
		"[web] site only",
		"[skip ci] no build",
		"[ci skip] no build",
		"feat: [skip ci] mid",
		"[release] v1.0.0",
		"release: 1.0.0",
		"",
	}
	for _, s := range omit {
		if !shouldOmitNightlyNote(s) {
			t.Fatalf("expected omit for %q", s)
		}
	}
	keep := []string{"fix: real change", "chore: bump deps"}
	for _, s := range keep {
		if shouldOmitNightlyNote(s) {
			t.Fatalf("expected keep for %q", s)
		}
	}
}

func ghCommit(sha, message, date string) GithubCommitResponse {
	return GithubCommitResponse{
		Sha: sha,
		Commit: GithubCommitDetail{
			Message:   message,
			Committer: GithubCommitPerson{Date: date},
			Author:    GithubCommitPerson{Date: date},
		},
	}
}

func TestCollectNightlyReleaseNotesFromCheckedLatestToCurrent(t *testing.T) {
	const (
		afterPkg   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		latestFull = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		webOnly    = "cccccccccccccccccccccccccccccccccccccccc"
		midFull    = "c111111111111111111111111111111111111111"
		currFull   = "dddddddddddddddddddddddddddddddddddddddd"
		olderFull  = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	)
	commits := []GithubCommitResponse{
		ghCommit(afterPkg, "chore: not in package yet", "2026-07-27T10:00:00Z"),
		ghCommit(latestFull, "fix: packaged latest\n\nbody", "2026-07-26T10:00:00Z"),
		ghCommit(webOnly, "[web] docs only", "2026-07-25T10:00:00Z"),
		ghCommit(midFull, "feat: middle change", "2026-07-24T10:00:00Z"),
		ghCommit(currFull, "feat: currently installed", "2026-07-23T10:00:00Z"),
		ghCommit(olderFull, "chore: older history", "2026-07-22T10:00:00Z"),
	}

	notes, date := collectNightlyReleaseNotes(commits, "nightly-"+currFull[:7], latestFull, false)
	if date != "2026-07-26T10:00:00Z" {
		t.Fatalf("latest package date: got %q", date)
	}
	want := "fix: packaged latest\nfeat: middle change"
	if notes != want {
		t.Fatalf("notes=\n%q\nwant\n%q", notes, want)
	}
}

func TestCollectNightlyReleaseNotesStopsAtCurrentWithoutIncludingIt(t *testing.T) {
	const (
		latest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		curr   = "dddddddddddddddddddddddddddddddddddddddd"
	)
	commits := []GithubCommitResponse{
		ghCommit(latest, "fix: latest", "2026-07-26T00:00:00Z"),
		ghCommit(curr, "feat: current", "2026-07-23T00:00:00Z"),
	}
	notes, _ := collectNightlyReleaseNotes(commits, curr[:7], latest, false)
	if notes != "fix: latest" {
		t.Fatalf("got %q", notes)
	}
}

func TestCollectNightlyReleaseNotesEmptyWhenUpToDate(t *testing.T) {
	const sha = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	commits := []GithubCommitResponse{
		ghCommit(sha, "fix: same", "2026-07-26T00:00:00Z"),
		ghCommit("cccccccccccccccccccccccccccccccccccccccc", "older", "2026-07-20T00:00:00Z"),
	}
	notes, _ := collectNightlyReleaseNotes(commits, "nightly-"+sha[:7], sha, false)
	if notes != "" {
		t.Fatalf("expected empty notes when current==latest, got %q", notes)
	}
}

func TestCollectNightlyReleaseNotesFallbackWhenLatestMissing(t *testing.T) {
	const curr = "dddddddddddddddddddddddddddddddddddddddd"
	commits := []GithubCommitResponse{
		ghCommit("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "feat: a", "2026-07-26T00:00:00Z"),
		ghCommit("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "feat: b", "2026-07-25T00:00:00Z"),
		ghCommit(curr, "feat: current", "2026-07-23T00:00:00Z"),
	}
	notes, _ := collectNightlyReleaseNotes(commits, curr[:7], "ffffffffffffffffffffffffffffffffffffffff", false)
	want := "feat: a\nfeat: b"
	if notes != want {
		t.Fatalf("got %q want %q", notes, want)
	}
}

func TestCollectNightlyReleaseNotesEmptyWhenCurrentAheadOfPackage(t *testing.T) {
	const (
		currFull   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		midFull    = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		latestFull = "cccccccccccccccccccccccccccccccccccccccc"
		olderFull  = "dddddddddddddddddddddddddddddddddddddddd"
	)
	commits := []GithubCommitResponse{
		ghCommit(currFull, "feat: local ahead of package", "2026-07-28T10:00:00Z"),
		ghCommit(midFull, "feat: between", "2026-07-27T10:00:00Z"),
		ghCommit(latestFull, "fix: packaged latest", "2026-07-26T10:00:00Z"),
		ghCommit(olderFull, "chore: older history", "2026-07-22T10:00:00Z"),
	}

	notes, date := collectNightlyReleaseNotes(commits, "nightly-"+currFull[:7], latestFull, false)
	if notes != "" {
		t.Fatalf("expected empty notes when current is ahead of package, got %q", notes)
	}
	if date != "2026-07-26T10:00:00Z" {
		t.Fatalf("package date: got %q", date)
	}
}

func TestCollectNightlyReleaseNotesPartialWhenCurrentNotInWindow(t *testing.T) {
	const latestFull = "cccccccccccccccccccccccccccccccccccccccc"
	commits := []GithubCommitResponse{
		ghCommit(latestFull, "fix: packaged latest", "2026-07-26T10:00:00Z"),
		ghCommit("dddddddddddddddddddddddddddddddddddddddd", "feat: still in window", "2026-07-22T10:00:00Z"),
	}
	notes, _ := collectNightlyReleaseNotes(commits, "eeeeeee", latestFull, false)
	if notes != "" {
		t.Fatalf("expected empty notes without existsOutside, got %q", notes)
	}
	notes, date := collectNightlyReleaseNotes(commits, "eeeeeee", latestFull, true)
	if date != "2026-07-26T10:00:00Z" {
		t.Fatalf("package date: got %q", date)
	}
	want := "fix: packaged latest\nfeat: still in window"
	if notes != want {
		t.Fatalf("notes=\n%q\nwant\n%q", notes, want)
	}
}

func TestCommitSHAMatches(t *testing.T) {
	const full = "f306a3851931578435b2b214cf89b0c7c0a0a39d"
	const short = "f306a38"
	if !commitSHAMatches(full, short) {
		t.Fatal("full vs short")
	}
	if !commitSHAMatches(full, full) {
		t.Fatal("full vs full")
	}
	if !commitSHAMatches(full, full[:12]) {
		t.Fatal("full vs abbrev12")
	}
	if commitSHAMatches(full, "deadbee") {
		t.Fatal("unrelated must not match")
	}
	if commitSHAMatches("", short) {
		t.Fatal("empty sha")
	}
}

func TestVersionsMatch(t *testing.T) {
	const short = "f306a38"
	const full = "f306a3851931578435b2b214cf89b0c7c0a0a39d"

	if versionsMatch("0.0.1", short, full) {
		t.Fatal("stable must not match nightly sha")
	}
	if !versionsMatch(short, short, full) {
		t.Fatal("short sha should match")
	}
	if !versionsMatch("nightly-"+short, short, full) {
		t.Fatal("nightly- prefix should match")
	}
	if !versionsMatch(full, short, full) {
		t.Fatal("full sha should match")
	}
	if !versionsMatch(full[:12], short, full) {
		t.Fatal("abbrev of full should match")
	}
	if versionsMatch("deadbee", short, full) {
		t.Fatal("other sha must not match")
	}
}

func TestFindTarget(t *testing.T) {
	info := &ChannelInfo{
		Version: "abc1234",
		Targets: []ChannelInfoTarget{
			{OS: "linux", Arch: "amd64", File: "renop-abc1234-linux-amd64.zip", Size: 10},
			{OS: "windows", Arch: "amd64", File: "renop-abc1234-windows-amd64.zip", Size: 20},
		},
	}
	got := findTarget(info, "linux", "amd64")
	if got == nil || got.Size != 10 {
		t.Fatalf("linux amd64: %+v", got)
	}
	if findTarget(info, "openbsd", "arm64") != nil {
		t.Fatal("missing platform should be nil")
	}
	info2 := &ChannelInfo{Targets: []ChannelInfoTarget{
		{File: "renop-x-freebsd-arm64.zip", Size: 3},
	}}
	if findTarget(info2, "freebsd", "arm64") == nil {
		t.Fatal("filename fallback failed")
	}
	info3 := &ChannelInfo{
		Version: "abc1234",
		Releases: []ChannelInfoRelease{
			{
				Version: "abc1234",
				Targets: []ChannelInfoTarget{
					{OS: "darwin", Arch: "arm64", File: "renop-abc1234-darwin-arm64.zip", Size: 50},
				},
			},
		},
	}
	got3 := findTarget(info3, "darwin", "arm64")
	if got3 == nil || got3.Size != 50 {
		t.Fatalf("releases target fallback failed: %+v", got3)
	}
}

func TestPackageURL(t *testing.T) {
	u := packageURL(ChannelNightly, "abc1234", "renop-abc1234-linux-amd64.zip")
	want := OfficialUpdateBase + "/nightly/abc1234/renop-abc1234-linux-amd64.zip"
	if u != want {
		t.Fatalf("got %s want %s", u, want)
	}
	u2 := packageURL(ChannelRelease, "v0.0.1", "renop-v0.0.1-windows-amd64.zip")
	want2 := OfficialUpdateBase + "/stable/v0.0.1/renop-v0.0.1-windows-amd64.zip"
	if u2 != want2 {
		t.Fatalf("got %s want %s", u2, want2)
	}
}

func TestParseChannelAndMode(t *testing.T) {
	if ParseChannel("nightly") != ChannelNightly {
		t.Fatal("nightly")
	}
	if ParseChannel("") != ChannelRelease {
		t.Fatal("default release")
	}
	if ParseUpdateMode("auto_check") != ModeAutoCheck {
		t.Fatal("auto_check")
	}
	if ParseUpdateMode("") != ModeManual {
		t.Fatal("default manual")
	}
}

func TestResolveCheckChannelFromConfig(t *testing.T) {
	var cfgAtomic atomic.Value
	cfgAtomic.Store(&config.Config{
		Updater: config.UpdaterConfig{Channel: "nightly", Mode: "manual"},
	})
	state := &core.AppState{Inner: &core.AppStateInner{Config: &cfgAtomic}}

	if got := resolveCheckChannel("", state); got != ChannelNightly {
		t.Fatalf("config channel nightly: got %s", got)
	}
	if got := resolveCheckChannel("release", state); got != ChannelRelease {
		t.Fatalf("query override: got %s", got)
	}
}

func TestHasUpdateRequiresPlatformPackage(t *testing.T) {
	info := &ChannelInfo{
		Version: "1.2.3",
		Commit:  "newsha1full00000000000000000000000000000",
		Targets: []ChannelInfoTarget{
			{OS: "linux", Arch: "mips64", File: "only-mips.zip", Size: 1},
		},
	}
	target := findTarget(info, "windows", "amd64")
	if decideHasUpdate("0.0.1", info.Version, info.Commit, target, nil, nil, false) {
		t.Fatal("must not report update when platform package is missing")
	}
	info.Targets = append(info.Targets, ChannelInfoTarget{
		OS: "windows", Arch: "amd64", File: "win.zip", Size: 2,
	})
	target = findTarget(info, "windows", "amd64")
	if !decideHasUpdate("0.0.1", info.Version, info.Commit, target, nil, nil, false) {
		t.Fatal("must report update when platform package exists and version differs")
	}
}

func TestDecideHasUpdateSemver(t *testing.T) {
	target := &ChannelInfoTarget{OS: "linux", Arch: "amd64", File: "x.zip", Size: 1}

	if decideHasUpdate("1.2.3", "1.2.3", "", target, nil, nil, false) {
		t.Fatal("same semver is not an update")
	}
	if !decideHasUpdate("1.2.3", "1.3.0", "", target, nil, nil, false) {
		t.Fatal("newer semver is an update")
	}
	if decideHasUpdate("1.3.0", "1.2.3", "", target, nil, nil, false) {
		t.Fatal("older semver must not be treated as an update")
	}
	if decideHasUpdate("v1.3.0", "1.2.9", "", target, nil, nil, false) {
		t.Fatal("older semver with v-prefix must not be an update")
	}
}

func TestDecideHasUpdateNightlyAheadOfStable(t *testing.T) {
	const (
		nightlyAhead = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		stableCommit = "cccccccccccccccccccccccccccccccccccccccc"
		older        = "dddddddddddddddddddddddddddddddddddddddd"
	)
	commits := []GithubCommitResponse{
		ghCommit(nightlyAhead, "feat: after release", "2026-07-28T00:00:00Z"),
		ghCommit(stableCommit, "release: v0.0.1", "2026-07-20T00:00:00Z"),
		ghCommit(older, "chore: old", "2026-07-10T00:00:00Z"),
	}
	target := &ChannelInfoTarget{OS: "linux", Arch: "amd64", File: "x.zip", Size: 1}

	if decideHasUpdate("nightly-"+nightlyAhead[:7], "0.0.1", stableCommit, target, nil, commits, false) {
		t.Fatal("older stable must not be an update over a newer nightly")
	}
	if decideHasUpdate(nightlyAhead[:7], "v0.0.1", stableCommit, target, nil, commits, false) {
		t.Fatal("short sha nightly ahead of stable must not report update")
	}

	const olderNightly = "dddddddddddddddddddddddddddddddddddddddd"
	if !decideHasUpdate("nightly-"+olderNightly[:7], "0.0.2", stableCommit, target, nil, commits, false) {
		t.Fatal("stable newer than running nightly should be an update")
	}
}

func TestDecideHasUpdateNightlyAheadOfPackage(t *testing.T) {
	const (
		curr  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		pkg   = "cccccccccccccccccccccccccccccccccccccccc"
		older = "dddddddddddddddddddddddddddddddddddddddd"
	)
	commits := []GithubCommitResponse{
		ghCommit(curr, "feat: ahead", "2026-07-28T00:00:00Z"),
		ghCommit(pkg, "fix: packaged", "2026-07-26T00:00:00Z"),
		ghCommit(older, "chore: old", "2026-07-20T00:00:00Z"),
	}
	target := &ChannelInfoTarget{OS: "linux", Arch: "amd64", File: "x.zip", Size: 1}

	if decideHasUpdate("nightly-"+curr[:7], pkg[:7], pkg, target, nil, commits, false) {
		t.Fatal("current ahead of nightly package must not report update")
	}
	if !decideHasUpdate("nightly-"+older[:7], pkg[:7], pkg, target, nil, commits, false) {
		t.Fatal("package ahead of current must report update")
	}
	if decideHasUpdate("nightly-"+pkg[:7], pkg[:7], pkg, target, nil, commits, false) {
		t.Fatal("same package commit is not an update")
	}
}

func TestDecideHasUpdateCommitLikeWithoutGraph(t *testing.T) {
	target := &ChannelInfoTarget{OS: "linux", Arch: "amd64", File: "x.zip", Size: 1}
	if decideHasUpdate("f306a38", "0.0.1", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", target, nil, nil, false) {
		t.Fatal("commit-like current without graph must not assume stable is newer")
	}
}

func TestDecideHasUpdateWithInfoReleases(t *testing.T) {
	target := &ChannelInfoTarget{OS: "linux", Arch: "amd64", File: "x.zip", Size: 1}
	releases := []ChannelInfoRelease{
		{Version: "nightly-b2", Commit: "sha2"},
		{Version: "nightly-b1", Commit: "sha1"},
	}

	if !decideHasUpdate("nightly-b1", "nightly-b2", "sha2", target, releases, nil, false) {
		t.Fatal("older release in infoReleases must report hasUpdate=true without GitHub API")
	}
	if decideHasUpdate("nightly-b2", "nightly-b2", "sha2", target, releases, nil, false) {
		t.Fatal("same latest release in infoReleases must report hasUpdate=false")
	}
}

func TestLooksLikeCommitID(t *testing.T) {
	if !looksLikeCommitID("f306a38") {
		t.Fatal("short sha")
	}
	if !looksLikeCommitID("nightly-f306a38") {
		t.Fatal("nightly-prefixed sha")
	}
	if looksLikeCommitID("0.0.1") {
		t.Fatal("semver is not a commit id")
	}
	if looksLikeCommitID("dev") {
		t.Fatal("dev is not a commit id")
	}
	if looksLikeCommitID("v1.2.3") {
		t.Fatal("v-semver is not a commit id")
	}
}

func TestRemoteIsStrictlyNewer(t *testing.T) {
	const (
		newC = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		midC = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		oldC = "cccccccccccccccccccccccccccccccccccccccc"
	)
	commits := []GithubCommitResponse{
		ghCommit(newC, "n", "2026-07-28T00:00:00Z"),
		ghCommit(midC, "m", "2026-07-27T00:00:00Z"),
		ghCommit(oldC, "o", "2026-07-26T00:00:00Z"),
	}
	newer, ok := remoteIsStrictlyNewer(commits, oldC[:7], newC)
	if !ok || !newer {
		t.Fatalf("remote newer: newer=%v ok=%v", newer, ok)
	}
	newer, ok = remoteIsStrictlyNewer(commits, newC[:7], midC)
	if !ok || newer {
		t.Fatalf("remote older: newer=%v ok=%v", newer, ok)
	}
	newer, ok = remoteIsStrictlyNewer(commits, midC[:7], midC)
	if !ok || newer {
		t.Fatalf("equal: newer=%v ok=%v", newer, ok)
	}
	_, ok = remoteIsStrictlyNewer(commits, "deadbee", newC)
	if ok {
		t.Fatal("missing current must not be ok without existence proof")
	}
	_, ok = remoteIsStrictlyNewer(commits, oldC[:7], "ffffffffffffffffffff")
	if ok {
		t.Fatal("missing remote must not be ok")
	}
}

func TestDecideHasUpdateCurrentOutsideCommitWindow(t *testing.T) {
	const (
		pkg = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		mid = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	commits := []GithubCommitResponse{
		ghCommit(pkg, "fix: packaged", "2026-07-28T00:00:00Z"),
		ghCommit(mid, "feat: mid", "2026-07-20T00:00:00Z"),
	}
	target := &ChannelInfoTarget{OS: "linux", Arch: "amd64", File: "x.zip", Size: 1}

	if decideHasUpdate("nightly-eeeeeee", pkg[:7], pkg, target, nil, commits, false) {
		t.Fatal("outside window without existence proof must not report update")
	}
	if !decideHasUpdate("nightly-eeeeeee", pkg[:7], pkg, target, nil, commits, true) {
		t.Fatal("stale nightly confirmed on GitHub must report update")
	}
	if !decideHasUpdate("eeeeeee", "0.0.2", pkg, target, nil, commits, true) {
		t.Fatal("stale short-sha confirmed on GitHub must report update vs release package")
	}
	if decideHasUpdate("eeeeeee", "0.0.2", pkg, target, nil, nil, true) {
		t.Fatal("commit-like current without graph must not assume update")
	}
}

func TestGithubCommitExistsStatusMapping(t *testing.T) {
	cases := []struct {
		code        int
		wantExists  bool
		wantChecked bool
	}{
		{code: http.StatusOK, wantExists: true, wantChecked: true},
		{code: http.StatusNotFound, wantExists: false, wantChecked: true},
		{code: http.StatusUnprocessableEntity, wantExists: false, wantChecked: true},
		{code: http.StatusInternalServerError, wantExists: false, wantChecked: false},
		{code: http.StatusForbidden, wantExists: false, wantChecked: false},
	}
	for _, tc := range cases {
		exists, checked := mapGithubCommitStatus(tc.code)
		if exists != tc.wantExists || checked != tc.wantChecked {
			t.Fatalf("status %d: exists=%v checked=%v want exists=%v checked=%v",
				tc.code, exists, checked, tc.wantExists, tc.wantChecked)
		}
	}
	if exists, checked := githubCommitExists(context.Background(), ""); exists || checked {
		t.Fatalf("empty sha must not probe: exists=%v checked=%v", exists, checked)
	}
}
