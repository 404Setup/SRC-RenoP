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

const (
	ReviewKindSuperTeamTransfer = "super_team_transfer"
	ReviewKindPublication       = "publication"

	ReviewResourceDockerImage   = "docker_image"
	ReviewResourceNPMPackage    = "npm_package"
	ReviewResourceCargoPackage  = "cargo_package"
	ReviewResourceMavenArtifact = "maven_artifact"
	ReviewResourceMavenDomain   = "maven_domain"

	ReviewStatusPending   = "pending"
	ReviewStatusApproved  = "approved"
	ReviewStatusRejected  = "rejected"
	ReviewStatusCancelled = "cancelled"

	PublicationReviewSettleMillis = 5000
	ReviewVersionPackageCreation  = "@create"
)

var (
	ErrReviewTaskNotFound       = errors.New("review task was not found")
	ErrReviewTaskConflict       = errors.New("review task is no longer pending")
	ErrReviewTaskExists         = errors.New("an equivalent review task is already pending")
	ErrReviewInvalidRequest     = errors.New("review request is invalid")
	ErrReviewPermissionDenied   = errors.New("review permission denied")
	ErrReviewResourceConflict   = errors.New("review resource ownership changed")
	ErrReviewTransferRestricted = errors.New("this namespaced resource cannot return to personal ownership")
	ErrReviewPublicationSealed  = errors.New("the reviewed publication is sealed")
	ErrReviewPublicationActive  = errors.New("the publication is still receiving files")
	ErrReviewFileNotFound       = errors.New("review file was not found")
	ErrReviewFileLimit          = errors.New("review file limit reached")
)

// ReviewFile is one repository-relative file attached to a publication review.
type ReviewFile struct {
	Repository string `json:"-"`
	ID         string `json:"id"`
	Path       string `json:"path"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Critical   bool   `json:"critical"`
	AddedAt    int64  `json:"added_at"`
	Virtual    bool   `json:"-"`
}

// PublicationReviewRequest identifies one immutable package version and its committed files.
type PublicationReviewRequest struct {
	ResourceType     string
	Repository       string
	ResourceKey      string
	ResourceName     string
	Version          string
	RequestedBy      string
	Policy           string
	PackageExists    bool
	ReviewTeamPrefix string
	TargetTeamPrefix string
	Files            []*ReviewFile
	Payload          []byte
	CreatedAt        int64
}

// PublicationReviewResult describes whether a committed publication remains hidden for review.
type PublicationReviewResult struct {
	Pending bool
	TaskID  string
}

// ReviewTask is one independently paginated, single-decision workflow record.
type ReviewTask struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	ResourceType     string `json:"resource_type"`
	Repository       string `json:"repository,omitempty"`
	ResourceKey      string `json:"resource_key"`
	ResourceName     string `json:"resource_name"`
	ResourceVersion  string `json:"resource_version,omitempty"`
	SourceTeamPrefix string `json:"source_team_prefix,omitempty"`
	TargetTeamPrefix string `json:"target_team_prefix,omitempty"`
	ReviewTeamPrefix string `json:"review_team_prefix"`
	RequestedByID    string `json:"-"`
	RequestedBy      string `json:"requested_by"`
	Status           string `json:"status"`
	DecisionReason   string `json:"decision_reason,omitempty"`
	DecidedByID      string `json:"-"`
	DecidedBy        string `json:"decided_by,omitempty"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
	DecidedAt        int64  `json:"decided_at,omitempty"`
	FileCount        int    `json:"file_count,omitempty"`
	TotalSize        int64  `json:"total_size,omitempty"`
	ActiveKey        string `json:"-"`
}

// SuperTeamTransferRequest identifies a project or publishing domain and its requested owner.
type SuperTeamTransferRequest struct {
	ResourceType     string `json:"resource_type"`
	Repository       string `json:"repository"`
	ResourceKey      string `json:"resource_key"`
	TargetTeamPrefix string `json:"target_team_prefix"`
}

// ReviewTaskListOptions controls bounded reviewer or requester task pages.
type ReviewTaskListOptions struct {
	Username              string
	RequestedView         bool
	Administrator         bool
	ModerateAll           bool
	ModeratedRepositories []string
	ResourceTypes         []string
	Status                string
	Limit                 int
	Offset                int
}
