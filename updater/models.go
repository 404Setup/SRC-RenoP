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

import "sync/atomic"

type UpdateMode string

const (
	ModeManual      UpdateMode = "manual"
	ModeAutoInstall UpdateMode = "auto_install"
)

type Channel string

const (
	ChannelRelease Channel = "release"
	ChannelNightly Channel = "nightly"
)

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

type GithubReleaseAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadUrl string `json:"browser_download_url"`
}

type GithubReleaseResponse struct {
	TagName         string               `json:"tag_name"`
	Name            string               `json:"name"`
	PublishedAt     string               `json:"published_at"`
	CreatedAt       string               `json:"created_at"`
	Body            string               `json:"body"`
	TargetCommitish string               `json:"target_commitish"`
	Assets          []GithubReleaseAsset `json:"assets"`
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

type GithubReleaseItem struct {
	TagName         string `json:"tag_name"`
	TargetCommitish string `json:"target_commitish"`
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
