/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func setupReviewTeam(t *testing.T) (*core.SuperTeam, *database.DB, int64) {
	t.Helper()
	db := newMavenDB(t)
	for _, username := range []string{"charlie", "dana"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{
			Name: username, CreatedAt: time.Now().Format(time.RFC3339),
		}))
	}
	now := time.Now().UnixMilli()
	team := &core.SuperTeam{Prefix: "platform", Name: "Platform", CreatedAt: now}
	require.NoError(t, db.CreateSuperTeam(team, "alice", 5, 10))
	require.NoError(t, db.ForceAddSuperTeamMembers("platform", "admin", []string{"bob"},
		core.SuperTeamRoleManage, 5, 10, now+1))
	require.NoError(t, db.ForceAddSuperTeamMembers("platform", "admin", []string{"charlie"},
		core.SuperTeamRoleRead, 5, 10, now+2))
	return team, db, now
}

func TestPublicationReviewFilesAreBoundedScopedAndSingleDecision(t *testing.T) {
	_, db, now := setupReviewTeam(t)
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "moderator", CreatedAt: time.Now().Format(time.RFC3339),
		Permissions: []string{"base", "canmoderate:releases"},
	}))
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "outsider", CreatedAt: time.Now().Format(time.RFC3339), Permissions: []string{"base"},
	}))
	request := core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceMavenArtifact, Repository: "releases",
		ResourceKey: "com.example:demo", ResourceName: "com.example:demo", Version: "1.0.0",
		RequestedBy: "charlie", Policy: config.PublicationReviewEveryVersion, CreatedAt: now,
		Payload: []byte(`{"engine":"test"}`),
		Files:   []*core.ReviewFile{{Path: "com/example/demo/1.0.0/demo-1.0.0.jar", Size: 42, Critical: true}},
	}
	result, err := db.CreateOrUpdatePublicationReview(request)
	require.NoError(t, err)
	require.True(t, result.Pending)
	require.NotEmpty(t, result.TaskID)

	request.Files[0].Size = 84
	request.Files[0].AddedAt = now + 100
	repeated, err := db.CreateOrUpdatePublicationReview(request)
	require.NoError(t, err)
	assert.Equal(t, result.TaskID, repeated.TaskID)
	task, err := db.GetReviewTask(result.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "com.example:demo", task.ResourceKey)
	assert.Equal(t, "1.0.0", task.ResourceVersion)
	assert.Equal(t, 1, task.FileCount)
	assert.EqualValues(t, 84, task.TotalSize)
	assert.Equal(t, now+100, task.UpdatedAt)
	payload, err := db.GetReviewTaskPayload(task.ID)
	require.NoError(t, err)
	assert.JSONEq(t, `{"engine":"test"}`, string(payload))
	files, err := db.ListReviewTaskFiles(task.ID)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.True(t, files[0].Critical)
	assert.Equal(t, "demo-1.0.0.jar", files[0].Name)
	pending, err := db.IsPublicationReviewPathPending("releases", files[0].Path)
	require.NoError(t, err)
	assert.True(t, pending)

	moderatorTasks, total, err := db.ListReviewTasks(core.ReviewTaskListOptions{
		Username: "moderator", ModeratedRepositories: []string{"releases"},
		Status: core.ReviewStatusPending, Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, moderatorTasks, 1)
	assert.Equal(t, task.ID, moderatorTasks[0].ID)
	_, err = db.DecideReviewTask(task.ID, "outsider", core.ReviewStatusApproved, "", now+6000)
	require.ErrorIs(t, err, core.ErrReviewPermissionDenied)
	_, err = db.DecideReviewTask(task.ID, "moderator", core.ReviewStatusApproved, "", now+200)
	require.ErrorIs(t, err, core.ErrReviewPublicationActive)
	approved, err := db.DecideReviewTask(task.ID, "moderator", core.ReviewStatusApproved, "", now+6000)
	require.NoError(t, err)
	assert.Equal(t, core.ReviewStatusApproved, approved.Status)
	_, err = db.GetReviewTaskPayload(task.ID)
	require.ErrorIs(t, err, core.ErrReviewTaskNotFound)
	pending, err = db.IsPublicationReviewPathPending("releases", files[0].Path)
	require.NoError(t, err)
	assert.False(t, pending)
	_, err = db.CreateOrUpdatePublicationReview(request)
	require.ErrorIs(t, err, core.ErrReviewPublicationSealed)
	_, err = db.CancelReviewTask(task.ID, "charlie", now+7000)
	require.ErrorIs(t, err, core.ErrReviewTaskConflict)

	retainedRequest := request
	retainedRequest.Version = "3.0.0"
	retainedRequest.CreatedAt = now + 7001
	retainedRequest.Files = []*core.ReviewFile{{Path: "com/example/demo/3.0.0/demo-3.0.0.jar", Size: 32}}
	retained, err := db.CreateOrUpdatePublicationReview(retainedRequest)
	require.NoError(t, err)
	require.NoError(t, db.DeleteToken("charlie"))
	retainedTask, err := db.GetReviewTask(retained.TaskID)
	require.NoError(t, err)
	assert.Equal(t, core.ReviewStatusPending, retainedTask.Status)
	assert.Equal(t, "charlie", retainedTask.RequestedBy)

	newPackageOnly := request
	newPackageOnly.Version = "2.0.0"
	newPackageOnly.Policy = config.PublicationReviewNewPackages
	newPackageOnly.PackageExists = true
	newPackageOnly.Files = []*core.ReviewFile{{Path: "com/example/demo/2.0.0/demo-2.0.0.jar", Size: 24}}
	notPending, err := db.CreateOrUpdatePublicationReview(newPackageOnly)
	require.NoError(t, err)
	assert.False(t, notPending.Pending)
}

func TestSuperTeamTransferReviewIsSingleDecisionAndReversible(t *testing.T) {
	_, stateDB, now := setupReviewTeam(t)
	db := stateDB
	_, err := db.CreateDockerImage("containers", "personal", "charlie", true, now+3)
	require.NoError(t, err)
	request := core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
		ResourceKey: "personal", TargetTeamPrefix: "platform",
	}
	task, err := db.CreateSuperTeamTransferReview(request, "charlie", false, now+4)
	require.NoError(t, err)
	assert.Equal(t, core.ReviewStatusPending, task.Status)
	assert.Empty(t, task.SourceTeamPrefix)
	assert.Equal(t, "platform", task.TargetTeamPrefix)
	_, err = db.CreateSuperTeamTransferReview(request, "charlie", false, now+5)
	require.ErrorIs(t, err, core.ErrReviewTaskExists)
	require.NoError(t, db.CreateSuperTeam(&core.SuperTeam{
		Prefix: "alternate", Name: "Alternate", CreatedAt: now + 6,
	}, "dana", 5, 10))
	require.NoError(t, db.ForceAddSuperTeamMembers("alternate", "dana", []string{"charlie"},
		core.SuperTeamRoleRead, 5, 10, now+7))
	_, err = db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
		ResourceKey: "personal", TargetTeamPrefix: "alternate",
	}, "charlie", false, now+8)
	require.ErrorIs(t, err, core.ErrReviewTaskExists)
	_, err = db.DecideReviewTask(task.ID, "charlie", core.ReviewStatusApproved, "", now+9)
	require.ErrorIs(t, err, core.ErrReviewPermissionDenied)

	reviewerTasks, total, err := db.ListReviewTasks(core.ReviewTaskListOptions{
		Username: "bob", Status: core.ReviewStatusPending, Limit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, reviewerTasks, 1)
	requestedTasks, requestedTotal, err := db.ListReviewTasks(core.ReviewTaskListOptions{
		Username: "charlie", RequestedView: true, Status: "all", Limit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, 1, requestedTotal)
	require.Len(t, requestedTasks, 1)
	_, err = db.DecideReviewTask(task.ID, "dana", core.ReviewStatusApproved, "", now+10)
	require.ErrorIs(t, err, core.ErrReviewPermissionDenied)

	type decisionResult struct {
		task *core.ReviewTask
		err  error
	}
	results := make(chan decisionResult, 2)
	var wait sync.WaitGroup
	for _, reviewer := range []string{"alice", "bob"} {
		wait.Add(1)
		go func(username string) {
			defer wait.Done()
			decided, decisionErr := db.DecideReviewTask(
				task.ID, username, core.ReviewStatusApproved, "", now+11)
			results <- decisionResult{task: decided, err: decisionErr}
		}(reviewer)
	}
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		if result.err == nil {
			successes++
			require.NotNil(t, result.task)
			assert.Equal(t, core.ReviewStatusApproved, result.task.Status)
		} else if errors.Is(result.err, core.ErrReviewTaskConflict) {
			conflicts++
		} else {
			require.NoError(t, result.err)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)
	image, err := db.GetDockerImage("containers", "personal")
	require.NoError(t, err)
	require.NotNil(t, image)
	assert.Equal(t, "platform", image.SuperTeamPrefix)

	outTask, err := db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers", ResourceKey: "personal",
	}, "charlie", false, now+12)
	require.NoError(t, err)
	_, err = db.DecideReviewTask(outTask.ID, "bob", core.ReviewStatusApproved, "", now+13)
	require.NoError(t, err)
	image, err = db.GetDockerImage("containers", "personal")
	require.NoError(t, err)
	assert.Empty(t, image.SuperTeamPrefix)
}

func TestSuperTeamTransferReviewCoversFormatsCancellationAndStaleResources(t *testing.T) {
	_, db, now := setupReviewTeam(t)
	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{
		Repository: "cargo", Name: "demo", NormalizedName: "demo", CreatedAt: now + 3, UpdatedAt: now + 3,
	}, &core.CargoVersion{Version: "1.0.0", CreatedAt: now + 3}, "charlie"))
	_, err := db.CreateNPMPackage("npm", "personal", "charlie", false, now+4)
	require.NoError(t, err)
	domain := &core.MavenDomain{
		Domain: "com.example", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "example.com", VerificationCode: "review-test", Verified: true,
		CreatedAt: now + 5, VerifiedAt: now + 5,
	}
	require.NoError(t, db.CreateMavenDomain(domain, "charlie"))
	require.NoError(t, db.RecordMavenPublication(&core.MavenArtifact{
		Repository: "maven", Domain: domain.Domain, GroupID: domain.Domain,
		ArtifactID: "demo", CreatedAt: now + 6, UpdatedAt: now + 6,
	}, &core.MavenVersion{Version: "1.0.0", Publisher: "charlie", CreatedAt: now + 6}))

	requests := []core.SuperTeamTransferRequest{
		{ResourceType: core.ReviewResourceCargoPackage, Repository: "cargo", ResourceKey: "demo", TargetTeamPrefix: "platform"},
		{ResourceType: core.ReviewResourceMavenDomain, ResourceKey: "com.example", TargetTeamPrefix: "platform"},
		{ResourceType: core.ReviewResourceMavenArtifact, Repository: "maven", ResourceKey: "com.example:demo", TargetTeamPrefix: "platform"},
	}
	for index, request := range requests {
		task, createErr := db.CreateSuperTeamTransferReview(request, "charlie", false, now+int64(10+index))
		require.NoError(t, createErr)
		_, decideErr := db.DecideReviewTask(task.ID, "bob", core.ReviewStatusApproved, "", now+int64(20+index))
		require.NoError(t, decideErr)
	}
	cargoDetails, err := db.GetCargoPackageDetails("cargo", "demo", "bob")
	require.NoError(t, err)
	assert.Equal(t, "platform", cargoDetails.Package.SuperTeamPrefix)
	domainDetails, err := db.GetMavenDomainDetails("com.example", "bob")
	require.NoError(t, err)
	assert.Equal(t, "platform", domainDetails.Domain.SuperTeamPrefix)
	artifactDetails, err := db.GetMavenArtifactDetails("maven", "com.example", "demo")
	require.NoError(t, err)
	assert.Equal(t, "platform", artifactDetails.Artifact.SuperTeamPrefix)

	cancelTask, err := db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceNPMPackage, Repository: "npm",
		ResourceKey: "personal", TargetTeamPrefix: "platform",
	}, "charlie", false, now+30)
	require.NoError(t, err)
	_, err = db.CancelReviewTask(cancelTask.ID, "dana", now+31)
	require.ErrorIs(t, err, core.ErrReviewPermissionDenied)
	cancelled, err := db.CancelReviewTask(cancelTask.ID, "charlie", now+32)
	require.NoError(t, err)
	assert.Equal(t, core.ReviewStatusCancelled, cancelled.Status)

	staleTask, err := db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceNPMPackage, Repository: "npm",
		ResourceKey: "personal", TargetTeamPrefix: "platform",
	}, "charlie", false, now+33)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE npm_packages SET super_team_prefix = ? WHERE repository = ? AND package_name = ?`,
		"platform", "npm", "personal")
	require.NoError(t, err)
	stale, err := db.DecideReviewTask(staleTask.ID, "bob", core.ReviewStatusApproved, "", now+34)
	require.ErrorIs(t, err, core.ErrReviewResourceConflict)
	require.NotNil(t, stale)
	assert.Equal(t, core.ReviewStatusCancelled, stale.Status)

	_, err = db.CreateDockerImageForTeam(
		"containers", "platform/namespaced", "alice", "platform", false, now+35)
	require.NoError(t, err)
	_, err = db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers", ResourceKey: "platform/namespaced",
	}, "alice", false, now+36)
	require.ErrorIs(t, err, core.ErrReviewTransferRestricted)
}

func TestSuperTeamTransferReviewAllowsRequesterWithManagerRole(t *testing.T) {
	_, db, now := setupReviewTeam(t)
	_, err := db.CreateDockerImage("containers", "owner-request", "alice", false, now+3)
	require.NoError(t, err)
	task, err := db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
		ResourceKey: "owner-request", TargetTeamPrefix: "platform",
	}, "alice", false, now+4)
	require.NoError(t, err)
	_, err = db.DecideReviewTask(task.ID, "bob", core.ReviewStatusRejected, "", now+5)
	require.ErrorIs(t, err, core.ErrReviewInvalidRequest)
	_, err = db.DecideReviewTask(task.ID, "alice", core.ReviewStatusApproved, "ignored", now+6)
	require.NoError(t, err)
}

func TestSystemAdministratorReviewsTransfersWithoutTeamMembership(t *testing.T) {
	_, db, now := setupReviewTeam(t)
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "admin", CreatedAt: time.Now().Format(time.RFC3339), Permissions: []string{"base", "manager"},
	}))
	_, err := db.CreateDockerImage("containers", "administrator-review", "charlie", false, now+3)
	require.NoError(t, err)
	task, err := db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
		ResourceKey: "administrator-review", TargetTeamPrefix: "platform",
	}, "charlie", false, now+4)
	require.NoError(t, err)

	tasks, total, err := db.ListReviewTasks(core.ReviewTaskListOptions{
		Username: "admin", Administrator: true, Status: core.ReviewStatusPending, Limit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, tasks, 1)
	assert.Equal(t, task.ID, tasks[0].ID)
	_, err = db.DecideReviewTask(task.ID, "admin", core.ReviewStatusApproved, "", now+5)
	require.NoError(t, err)
}

func TestPendingTransferProtectsTeamAndRequesterDeletionCancelsReview(t *testing.T) {
	_, db, now := setupReviewTeam(t)
	_, err := db.CreateDockerImage("containers", "deletion", "charlie", false, now+3)
	require.NoError(t, err)
	task, err := db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
		ResourceKey: "deletion", TargetTeamPrefix: "platform",
	}, "charlie", false, now+4)
	require.NoError(t, err)
	require.ErrorIs(t, db.DeleteSuperTeam("platform", "admin", true, now+5), core.ErrSuperTeamNotEmpty)
	require.NoError(t, db.ForceAddDockerMembers(
		"containers", "deletion", "admin", []string{"bob"}, core.DockerPermissionOwner))
	require.NoError(t, db.DeleteToken("charlie"))

	var status, reason string
	var activeKey *string
	require.NoError(t, db.QueryRow(`SELECT status, decision_reason, active_key FROM review_tasks WHERE id = ?`,
		task.ID).Scan(&status, &reason, &activeKey))
	assert.Equal(t, core.ReviewStatusCancelled, status)
	assert.Equal(t, "requester_deleted", reason)
	assert.Nil(t, activeKey)
	require.NoError(t, db.DeleteSuperTeam("platform", "admin", true, now+6))
}

func TestSuperTeamTransferReviewRechecksRequesterAuthority(t *testing.T) {
	_, db, now := setupReviewTeam(t)
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "repository-manager", CreatedAt: time.Now().Format(time.RFC3339),
		Permissions: []string{"base", "canupdate:containers"},
	}))
	require.NoError(t, db.ForceAddSuperTeamMembers("platform", "alice", []string{"repository-manager"},
		core.SuperTeamRoleRead, 5, 10, now+3))

	_, err := db.CreateDockerImage("containers", "admin-transfer", "dana", false, now+4)
	require.NoError(t, err)
	adminTask, err := db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
		ResourceKey: "admin-transfer", TargetTeamPrefix: "platform",
	}, "repository-manager", true, now+5)
	require.NoError(t, err)
	_, err = db.DecideReviewTask(adminTask.ID, "bob", core.ReviewStatusApproved, "", now+6)
	require.NoError(t, err)

	_, err = db.CreateDockerImage("containers", "revoked-manager", "dana", false, now+7)
	require.NoError(t, err)
	revokedTask, err := db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
		ResourceKey: "revoked-manager", TargetTeamPrefix: "platform",
	}, "repository-manager", true, now+8)
	require.NoError(t, err)
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "repository-manager", CreatedAt: time.Now().Format(time.RFC3339), Permissions: []string{"base"},
	}))
	cancelled, err := db.DecideReviewTask(
		revokedTask.ID, "bob", core.ReviewStatusApproved, "", now+9)
	require.ErrorIs(t, err, core.ErrReviewResourceConflict)
	require.NotNil(t, cancelled)
	assert.Equal(t, core.ReviewStatusCancelled, cancelled.Status)

	_, err = db.CreateDockerImage("containers", "left-team", "charlie", false, now+10)
	require.NoError(t, err)
	leftTask, err := db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
		ResourceKey: "left-team", TargetTeamPrefix: "platform",
	}, "charlie", false, now+11)
	require.NoError(t, err)
	require.NoError(t, db.RemoveSuperTeamMember("platform", "alice", "charlie", false, now+12))
	cancelled, err = db.DecideReviewTask(leftTask.ID, "bob", core.ReviewStatusApproved, "", now+13)
	require.ErrorIs(t, err, core.ErrReviewResourceConflict)
	require.NotNil(t, cancelled)
	assert.Equal(t, core.ReviewStatusCancelled, cancelled.Status)
}

func TestReviewDecisionKeepsTaskPendingAfterOperationalFailure(t *testing.T) {
	_, db, now := setupReviewTeam(t)
	_, err := db.CreateDockerImage("containers", "operational-failure", "charlie", false, now+3)
	require.NoError(t, err)
	task, err := db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: "containers",
		ResourceKey: "operational-failure", TargetTeamPrefix: "platform",
	}, "charlie", false, now+4)
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE docker_images`)
	require.NoError(t, err)
	_, err = db.DecideReviewTask(task.ID, "bob", core.ReviewStatusApproved, "", now+5)
	require.Error(t, err)
	require.NotErrorIs(t, err, core.ErrReviewResourceConflict)

	var status string
	var activeKey string
	require.NoError(t, db.QueryRow(`SELECT status, active_key FROM review_tasks WHERE id = ?`, task.ID).
		Scan(&status, &activeKey))
	assert.Equal(t, core.ReviewStatusPending, status)
	assert.NotEmpty(t, activeKey)
}
