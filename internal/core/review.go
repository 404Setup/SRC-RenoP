/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package core

import "errors"

const (
	ReviewKindSuperTeamTransfer = "super_team_transfer"

	ReviewResourceDockerImage   = "docker_image"
	ReviewResourceNPMPackage    = "npm_package"
	ReviewResourceCargoPackage  = "cargo_package"
	ReviewResourceMavenArtifact = "maven_artifact"
	ReviewResourceMavenDomain   = "maven_domain"

	ReviewStatusPending   = "pending"
	ReviewStatusApproved  = "approved"
	ReviewStatusRejected  = "rejected"
	ReviewStatusCancelled = "cancelled"
)

var (
	ErrReviewTaskNotFound       = errors.New("review task was not found")
	ErrReviewTaskConflict       = errors.New("review task is no longer pending")
	ErrReviewTaskExists         = errors.New("an equivalent review task is already pending")
	ErrReviewInvalidRequest     = errors.New("review request is invalid")
	ErrReviewPermissionDenied   = errors.New("review permission denied")
	ErrReviewResourceConflict   = errors.New("review resource ownership changed")
	ErrReviewTransferRestricted = errors.New("this namespaced resource cannot return to personal ownership")
)

// ReviewTask is one independently paginated, single-decision workflow record.
type ReviewTask struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	ResourceType     string `json:"resource_type"`
	Repository       string `json:"repository,omitempty"`
	ResourceKey      string `json:"resource_key"`
	ResourceName     string `json:"resource_name"`
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
	DecidedAt        int64  `json:"decided_at,omitempty"`
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
	Username      string
	RequestedView bool
	Administrator bool
	ResourceTypes []string
	Status        string
	Limit         int
	Offset        int
}
