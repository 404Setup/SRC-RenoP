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
	"strings"
	"sync/atomic"
)

type UpdateMode string

const (
	ModeManual      UpdateMode = "manual"
	ModeAutoCheck   UpdateMode = "auto_check"
	ModeAutoInstall UpdateMode = "auto_install"
	ModeSafeInstall UpdateMode = "safe_install"
)

type Channel string

const (
	ChannelRelease Channel = "release"
	ChannelNightly Channel = "nightly"
)

const OfficialUpdateBase = "https://mvnc.pkg.one/update/renop"

// ParseChannel normalizes a channel string. Unknown values fall back to release.
func ParseChannel(s string) Channel {
	switch Channel(strings.ToLower(strings.TrimSpace(s))) {
	case ChannelNightly:
		return ChannelNightly
	default:
		return ChannelRelease
	}
}

// ParseUpdateMode normalizes an update mode string. Unknown values fall back to manual.
func ParseUpdateMode(s string) UpdateMode {
	switch UpdateMode(strings.ToLower(strings.TrimSpace(s))) {
	case ModeAutoCheck:
		return ModeAutoCheck
	case ModeAutoInstall:
		return ModeAutoInstall
	case ModeSafeInstall:
		return ModeSafeInstall
	default:
		return ModeManual
	}
}

// OfficialChannelPath is the path segment under OfficialUpdateBase for a channel.
func OfficialChannelPath(ch Channel) string {
	if ch == ChannelNightly {
		return "nightly"
	}
	return "stable"
}

type UpdateState struct {
	Status             string `json:"status"` // idle, checking, available, downloading, ready_to_restart, error
	LatestVersion      string `json:"latest_version"`
	DownloadUrl        string `json:"download_url"`
	Progress           int    `json:"progress"`
	ErrorMessage       string `json:"error_message,omitempty"`
	Size               int64  `json:"size,omitempty"`
	EstimatedDiskSpace int64  `json:"estimated_disk_space,omitempty"`
	ReleaseDate        string `json:"release_date,omitempty"`
	ReleaseNotes       string `json:"release_notes,omitempty"`
	CommitSha          string `json:"commit_sha,omitempty"`
	IsRelease          bool   `json:"is_release"`
}

type CheckResult struct {
	CurrentVersion     string `json:"current_version"`
	LatestVersion      string `json:"latest_version"`
	DownloadUrl        string `json:"download_url"`
	Channel            string `json:"channel"`
	ReleaseDate        string `json:"release_date"`
	ReleaseNotes       string `json:"release_notes"`
	CommitSha          string `json:"commit_sha"`
	Size               int64  `json:"size"`
	EstimatedDiskSpace int64  `json:"estimated_disk_space"`
	HasUpdate          bool   `json:"has_update"`
	IsRelease          bool   `json:"is_release"`
}

// ChannelInfoRelease represents information about a single release or preview build.
type ChannelInfoRelease struct {
	Version     string              `json:"version"`
	Commit      string              `json:"commit"`
	Channel     string              `json:"channel,omitempty"`
	Development bool                `json:"development,omitempty"`
	PublishedAt string              `json:"published_at"`
	Changelog   string              `json:"changelog,omitempty"`
	Targets     []ChannelInfoTarget `json:"targets"`
}

// ChannelInfo is the hosted update/renop/{channel}/info.json document.
type ChannelInfo struct {
	Releases []ChannelInfoRelease `json:"releases"`
}

type ChannelInfoTarget struct {
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	File        string `json:"file"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"download_url,omitempty"`
}

type GithubReleaseResponse struct {
	TagName         string `json:"tag_name"`
	Name            string `json:"name"`
	PublishedAt     string `json:"published_at"`
	CreatedAt       string `json:"created_at"`
	Body            string `json:"body"`
	TargetCommitish string `json:"target_commitish"`
}

type GithubCommitPerson struct {
	Date string `json:"date"`
}

type GithubCommitDetail struct {
	Author    GithubCommitPerson `json:"author"`
	Committer GithubCommitPerson `json:"committer"`
	Message   string             `json:"message"`
}

type GithubCommitResponse struct {
	Sha    string             `json:"sha"`
	Commit GithubCommitDetail `json:"commit"`
}

var currentStatePtr atomic.Pointer[UpdateState]

func init() {
	initialState := &UpdateState{Status: "idle"}
	currentStatePtr.Store(initialState)
}

func GetUpdateState() *UpdateState {
	s := currentStatePtr.Load()
	if s == nil {
		return &UpdateState{Status: "idle"}
	}
	return s
}

func updateStateFields(mutate func(s *UpdateState)) {
	for {
		curr := currentStatePtr.Load()
		var copyState UpdateState
		if curr != nil {
			copyState = *curr
		} else {
			copyState = UpdateState{Status: "idle"}
		}
		mutate(&copyState)
		if currentStatePtr.CompareAndSwap(curr, &copyState) {
			break
		}
	}
}
