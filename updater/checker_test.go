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
		Version: "newsha1",
		Commit:  "newsha1full00000000000000000000000000000",
		Targets: []ChannelInfoTarget{
			{OS: "linux", Arch: "mips64", File: "only-mips.zip", Size: 1},
		},
	}
	target := findTarget(info, "windows", "amd64")
	matched := versionsMatch("0.0.1", info.Version, info.Commit)
	hasUpdate := !matched && target != nil
	if hasUpdate {
		t.Fatal("must not report update when platform package is missing")
	}
	info.Targets = append(info.Targets, ChannelInfoTarget{
		OS: "windows", Arch: "amd64", File: "win.zip", Size: 2,
	})
	target = findTarget(info, "windows", "amd64")
	hasUpdate = !matched && target != nil
	if !hasUpdate {
		t.Fatal("must report update when platform package exists and version differs")
	}
}
