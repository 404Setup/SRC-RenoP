/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
)

func TestProcessUploadedFileAppliesReviewForChunkedAndDirectCallers(t *testing.T) {
	state, _, repo, storagePath := setupGPGUploadState(t)
	repo.RequireGPGSignature = false
	repo.PublicationReview = config.PublicationReviewEveryVersion
	previousCandidate := MavenPublicationReviewCandidate
	previousProcessor := MavenPublicationProcessor
	t.Cleanup(func() {
		MavenPublicationReviewCandidate = previousCandidate
		MavenPublicationProcessor = previousProcessor
	})
	MavenPublicationReviewCandidate = func(path string) bool {
		return path == "com/example/demo/1.0.0/demo-1.0.0.jar"
	}
	MavenPublicationProcessor = func(_ *core.AppState, received *config.Repository, username string,
		files []*core.ReviewFile,
	) (*core.PublicationReviewResult, error) {
		assert.Equal(t, repo, received)
		assert.Equal(t, "alice", username)
		require.Len(t, files, 1)
		assert.Equal(t, "com/example/demo/1.0.0/demo-1.0.0.jar", files[0].Path)
		return &core.PublicationReviewResult{Pending: true, TaskID: "review-id"}, nil
	}
	tempPath := filepath.Join(storagePath, "chunk.tmp")
	require.NoError(t, os.WriteFile(tempPath, []byte("artifact"), 0600))
	target := filepath.Join(storagePath, "releases", "com", "example", "demo", "1.0.0", "demo-1.0.0.jar")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0755))
	result, err := ProcessUploadedFile(context.Background(), state, repo, &PreparedUpload{
		LocalFilePath: target, TempPath: tempPath, Username: "alice", FileSize: 8,
		ModTime: time.Now().UnixNano(),
	})
	require.NoError(t, err)
	assert.True(t, result.Pending)
	assert.True(t, result.ReviewPending)
	assert.Equal(t, "review-id", result.ReviewID)
	assert.True(t, state.Inner.FileIndex.IsBlocked(target))
	_, err = os.Stat(target)
	require.NoError(t, err)
}

func TestPublicationReviewBlocksRestoreApprovalAndRejection(t *testing.T) {
	state, db, _, storagePath := setupGPGUploadState(t)
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", CreatedAt: time.Now().Format(time.RFC3339)}))
	now := time.Now().UnixMilli()
	relative := "com/example/demo/1.0.0/demo-1.0.0.jar"
	absolute := filepath.Join(storagePath, "releases", filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(absolute), 0755))
	require.NoError(t, os.WriteFile(absolute, []byte("artifact"), 0644))
	state.Inner.FileIndex.InsertFile(absolute, index.FileInfo{Size: 8, ModTime: time.Now().UnixNano()})
	result, err := db.CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceMavenArtifact, Repository: "releases",
		ResourceKey: "com.example:demo", ResourceName: "com.example:demo", Version: "1.0.0",
		RequestedBy: "alice", Policy: config.PublicationReviewEveryVersion, CreatedAt: now,
		Files: []*core.ReviewFile{{Path: relative, Size: 8}},
	})
	require.NoError(t, err)
	require.True(t, result.Pending)
	require.NoError(t, RestorePublicationReviewState(state))
	assert.True(t, state.Inner.FileIndex.IsBlocked(absolute))
	assert.False(t, state.Inner.FileIndex.HasFile(absolute))

	files, err := db.ListReviewTaskFiles(result.TaskID)
	require.NoError(t, err)
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "moderator", CreatedAt: time.Now().Format(time.RFC3339),
		Permissions: []string{"base", "canmoderate:releases"},
	}))
	_, err = db.DecideReviewTask(result.TaskID, "moderator", core.ReviewStatusApproved, "",
		now+core.PublicationReviewSettleMillis+1)
	require.NoError(t, err)
	require.NoError(t, UnblockPublicationReviewFiles(state, files))
	assert.False(t, state.Inner.FileIndex.IsBlocked(absolute))
	assert.True(t, state.Inner.FileIndex.HasFile(absolute))

	state.Inner.FileIndex.BlockFile(absolute)
	require.NoError(t, DeletePublicationReviewFiles(state, files))
	_, err = os.Stat(absolute)
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.False(t, state.Inner.FileIndex.IsBlocked(absolute))
}

func TestGPGCleanupPreservesPendingPublicationReviewBlock(t *testing.T) {
	state, db, _, storagePath := setupGPGUploadState(t)
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", CreatedAt: time.Now().Format(time.RFC3339)}))
	now := time.Now().UnixMilli()
	relative := "com/example/demo/2.0.0/demo-2.0.0.jar"
	absolute := filepath.Join(storagePath, "releases", filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(absolute), 0755))
	require.NoError(t, os.WriteFile(absolute, []byte("artifact"), 0644))
	_, err := db.CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceMavenArtifact, Repository: "releases",
		ResourceKey: "com.example:demo", ResourceName: "com.example:demo", Version: "2.0.0",
		RequestedBy: "alice", Policy: config.PublicationReviewEveryVersion, CreatedAt: now,
		Files: []*core.ReviewFile{{Path: relative, Size: 8}},
	})
	require.NoError(t, err)
	state.Inner.FileIndex.BlockFile(absolute)
	unblockReleasePaths(state, &core.GPGRelease{Repository: "releases", ArtifactPath: relative})
	assert.True(t, state.Inner.FileIndex.IsBlocked(absolute))
}

func TestMavenPublicationQuotaCountsFilesAndPOMPublicationsSeparately(t *testing.T) {
	state, db, repo, storagePath := setupGPGUploadState(t)
	next := state.Inner.Config.Load().DeepCopy()
	next.PublicationQuota = config.PublicationQuotaConfig{
		FileLimit: 10, ByteLimit: 1024, PublicationLimit: 10, Period: core.PublicationQuotaPeriodMonth,
	}
	state.Inner.Config.Store(next)
	for _, filename := range []string{"demo-1.0.0.jar", "demo-1.0.0.pom"} {
		reservation, err := reserveUploadedFileQuota(state, repo, &PreparedUpload{
			LocalFilePath: filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0.0", filename),
			Username:      "alice", FileSize: 100,
		})
		require.NoError(t, err)
		require.NoError(t, reservation.Commit())
		reservation.Release()
	}
	quota := next.PublicationQuota
	status, err := db.GetPublicationQuotaStatus(core.PublicationQuotaSubject{
		OwnerType: core.PublicationQuotaOwnerUser, OwnerKey: "alice",
	}, core.PublicationQuotaLimits{
		FileLimit: quota.FileLimit, ByteLimit: quota.ByteLimit,
		PublicationLimit: quota.PublicationLimit, Period: quota.Period,
	}, time.Now().UnixMilli())
	require.NoError(t, err)
	assert.EqualValues(t, 2, status.FilesUsed)
	assert.EqualValues(t, 200, status.BytesUsed)
	assert.EqualValues(t, 1, status.PublicationsUsed)
}
