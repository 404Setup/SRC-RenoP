/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
)

func TestMavenPublicationReviewDefersCatalogUntilApproval(t *testing.T) {
	state, _ := newMavenRouteState(t)
	now := time.Now().UnixMilli()
	require.NoError(t, state.GetDB().CreateMavenDomain(&core.MavenDomain{
		Domain: "com.example", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "example.com", VerificationCode: "review-code",
		Verified: true, CreatedAt: now, VerifiedAt: now,
	}, "alice"))
	repo := state.Inner.Config.Load().Maven.Repositories["releases"]
	repo.PublicationReview = config.PublicationReviewEveryVersion
	result, err := ProcessPublishedFiles(state, repo, "alice", []*core.ReviewFile{{
		Path: "com/example/demo/1.0.0/demo-1.0.0.jar", Size: 128, AddedAt: now,
	}, {
		Path: "com/example/demo/1.0.0/demo-1.0.0.pom", Size: 32, AddedAt: now,
	}})
	require.NoError(t, err)
	require.True(t, result.Pending)
	metadataResult, err := ProcessPublishedFiles(state, repo, "alice", []*core.ReviewFile{{
		Path: "com/example/demo/maven-metadata.xml", Size: 16, AddedAt: now + 1,
	}})
	require.NoError(t, err)
	require.True(t, metadataResult.Pending)
	assert.Equal(t, result.TaskID, metadataResult.TaskID)
	exists, err := state.GetDB().MavenArtifactExists("releases", "com.example", "demo")
	require.NoError(t, err)
	assert.False(t, exists)

	task, err := state.GetDB().GetReviewTask(result.TaskID)
	require.NoError(t, err)
	assert.Equal(t, 3, task.FileCount)
	assert.EqualValues(t, 176, task.TotalSize)
	require.NoError(t, ApprovePublicationReview(state, task))
	details, err := state.GetDB().GetMavenArtifactDetails("releases", "com.example", "demo")
	require.NoError(t, err)
	require.Len(t, details.Versions, 1)
	assert.Equal(t, "1.0.0", details.Versions[0].Version)
	assert.EqualValues(t, 160, details.Versions[0].Size)
}

func TestMavenArtifactDetailsIncludePendingVersionForAuthorizedViewer(t *testing.T) {
	state, _ := newMavenRouteState(t)
	now := time.Now().UnixMilli()
	require.NoError(t, state.GetDB().CreateMavenDomain(&core.MavenDomain{
		Domain: "org.example", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "example.org", VerificationCode: "review-code",
		Verified: true, CreatedAt: now, VerifiedAt: now,
	}, "alice"))
	require.NoError(t, state.GetDB().RecordMavenPublication(&core.MavenArtifact{
		Repository: "releases", Domain: "org.example", GroupID: "org.example", ArtifactID: "demo",
		CreatedAt: now, UpdatedAt: now,
	}, &core.MavenVersion{Version: "1.0.0", Publisher: "alice", CreatedAt: now}))
	repo := state.Inner.Config.Load().Maven.Repositories["releases"]
	repo.PublicationReview = config.PublicationReviewEveryVersion
	result, err := ProcessPublishedFiles(state, repo, "alice", []*core.ReviewFile{{
		Path: "org/example/demo/2.0.0/demo-2.0.0.jar", Size: 256, AddedAt: now + 1,
	}})
	require.NoError(t, err)
	require.True(t, result.Pending)
	details, err := state.GetDB().GetMavenArtifactDetails("releases", "org.example", "demo")
	require.NoError(t, err)
	require.NoError(t, AddPendingPublicationVersions(state, details))
	require.Len(t, details.Versions, 2)
	assert.Equal(t, 2, details.Artifact.VersionCount)
	assert.EqualValues(t, 256, details.Artifact.TotalSize)
	assert.Equal(t, 1, details.FileCount)
	assert.EqualValues(t, 256, details.TotalFileSize)
	assert.Equal(t, "2.0.0", details.Versions[0].Version)
	assert.Equal(t, core.ReviewStatusPending, details.Versions[0].ReviewStatus)
	assert.Equal(t, result.TaskID, details.Versions[0].ReviewID)
}
