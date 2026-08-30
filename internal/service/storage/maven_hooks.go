/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"renop/internal/config"
	"renop/internal/core"
)

// MavenMutationAuthorizer is wired by the Maven service to avoid a package cycle.
var MavenMutationAuthorizer func(state *core.AppState, user *config.User, repo *config.Repository, path string, requiredLevel int) error

// MavenPublicationQuotaOwner resolves the global-team quota owner for an authorized Maven path.
var MavenPublicationQuotaOwner func(state *core.AppState, username string, repo *config.Repository, path string) (string, error)

// MavenReadAuthorizer is wired by the Maven service for private-domain membership reads.
var MavenReadAuthorizer func(state *core.AppState, user *config.User, repo *config.Repository, path string, isRoot bool) (bool, error)

// MavenPublicationReviewCandidate classifies paths that may require pre-commit review hiding.
var MavenPublicationReviewCandidate func(path string) bool

// MavenPublicationProcessor records visible catalog metadata or creates one hidden publication review.
var MavenPublicationProcessor func(state *core.AppState, repo *config.Repository, username string,
	files []*core.ReviewFile) (*core.PublicationReviewResult, error)

// MavenMirrorRecorder is wired by the Maven service to retain mirror provenance in its catalog.
var MavenMirrorRecorder func(state *core.AppState, repository, path string, size, modTime int64) error
