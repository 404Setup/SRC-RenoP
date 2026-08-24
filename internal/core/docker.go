/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import "errors"

// Permission levels for Docker container images, mirrored after the Cargo permission system.
const (
	DockerPermissionRead    = 0
	DockerPermissionPublish = 1
	DockerPermissionManage  = 2
	DockerPermissionTeam    = 3
	DockerPermissionOwner   = 4
	DockerPermissionFull    = 4
)

// DockerRepositoryImage represents a container image within a Docker repository.
type DockerRepositoryImage struct {
	Repository      string `json:"repository"`
	ImageName       string `json:"image_name"`
	Description     string `json:"description"`
	Publisher       string `json:"publisher"`
	TagCount        int    `json:"tag_count"`
	LatestTag       string `json:"latest_tag"`
	PullCount       int64  `json:"pull_count"`
	PermissionLevel int    `json:"permission_level,omitempty"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

// DockerTag represents a named image tag pointing to a specific manifest digest.
type DockerTag struct {
	Repository   string `json:"repository"`
	ImageName    string `json:"image_name"`
	Tag          string `json:"tag"`
	Digest       string `json:"digest"`
	MediaType    string `json:"media_type"`
	Size         int64  `json:"size"`
	ConfigDigest string `json:"config_digest"`
	Publisher    string `json:"publisher"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

// DockerManifest represents a stored OCI or Docker v2 image manifest.
type DockerManifest struct {
	Repository   string `json:"repository"`
	ImageName    string `json:"image_name"`
	Digest       string `json:"digest"`
	MediaType    string `json:"media_type"`
	Size         int64  `json:"size"`
	ConfigDigest string `json:"config_digest"`
	Publisher    string `json:"publisher"`
	RawJSON      []byte `json:"-"`
	CreatedAt    int64  `json:"created_at"`
}

// DockerMember represents an authorized collaborator for a container image.
type DockerMember struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Level    int    `json:"level"`
	AddedAt  int64  `json:"added_at"`
}

// DockerInvitation represents a pending invitation to collaborate on a container image.
type DockerInvitation struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	ImageName  string `json:"image_name"`
	Inviter    string `json:"inviter"`
	Recipient  string `json:"recipient"`
	Level      int    `json:"level"`
	CreatedAt  int64  `json:"created_at"`
}

// DockerBlob represents an immutable content-addressed layer or configuration blob.
type DockerBlob struct {
	Repository string `json:"repository"`
	Digest     string `json:"digest"`
	Size       int64  `json:"size"`
	CreatedAt  int64  `json:"created_at"`
}

// DockerImageDetails holds full inspection details for an image in the repository.
type DockerImageDetails struct {
	Image           *DockerRepositoryImage `json:"image"`
	Tags            []*DockerTag           `json:"tags"`
	Manifest        *DockerManifest        `json:"manifest,omitempty"`
	Members         []*DockerMember        `json:"members,omitempty"`
	PermissionLevel int                    `json:"permission_level"`
	Administrator   bool                   `json:"administrator"`
	TotalSize       int64                  `json:"total_size"`
	LayersCount     int                    `json:"layers_count"`
}

var (
	ErrDockerImageNotFound      = errors.New("Docker image not found")
	ErrDockerTagNotFound        = errors.New("Docker tag not found")
	ErrDockerManifestNotFound   = errors.New("Docker manifest not found")
	ErrDockerBlobNotFound       = errors.New("Docker blob not found")
	ErrDockerBlobUploadNotFound = errors.New("Docker blob upload session not found")
	ErrDockerInvalidDigest      = errors.New("invalid digest")
	ErrDockerInvalidName        = errors.New("invalid repository name")
	ErrDockerInvalidTag         = errors.New("invalid tag")
	ErrDockerPermissionDenied   = errors.New("Docker permission denied")
	ErrDockerManifestInvalid    = errors.New("invalid manifest format")
	ErrDockerBlobUploadRange    = errors.New("invalid upload range")
	ErrDockerMemberExists       = errors.New("user is already a member of this Docker image")
	ErrDockerInvitationExists   = errors.New("invitation already pending for this user")
	ErrDockerInvitationInvalid  = errors.New("invitation is invalid or has expired")
	ErrDockerLastFullMember     = errors.New("cannot remove or demote the last L4 owner of this Docker image")
	ErrDockerOwnerCannotLeave   = errors.New("Docker L4 owner must transfer ownership before leaving")
)
