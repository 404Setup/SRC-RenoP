/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"renop/internal/config"
	"renop/internal/core"
)

// DriverCheckResult records one completed database contract phase.
type DriverCheckResult struct {
	Name     string        `json:"name"`
	Duration time.Duration `json:"duration"`
}

// RunDriverCheck exercises the cross-driver account, transaction, package,
// review, and statistics contract against an isolated database.
func RunDriverCheck(ctx context.Context, db *DB) ([]DriverCheckResult, error) {
	if db == nil || db.Dialect == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	suffix := uuid.NewString()[:8]
	username := "dbcheck-" + suffix
	now := time.Now().UnixMilli()
	results := make([]DriverCheckResult, 0, 7)
	run := func(name string, check func() error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		started := time.Now()
		if err := check(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		results = append(results, DriverCheckResult{Name: name, Duration: time.Since(started)})
		return nil
	}
	if err := run("account and session", func() error {
		if err := db.SaveToken(&core.AccessToken{
			Name: username, EncryptedSecret: "driver-check-password", CreatedAt: time.Now().UTC().Format(time.RFC3339),
			Permissions: []string{"base"},
		}); err != nil {
			return err
		}
		account, err := db.GetTokenByName(username)
		if err != nil || account == nil {
			return errorsOrMissing(err, "account")
		}
		session := &core.Session{PublicID: suffix, Username: username, IP: "127.0.0.1", CreatedAt: now, LoginMethod: "password"}
		session.LastActive.Store(now)
		if err := db.SaveSession(session, "driver-check-session-"+suffix); err != nil {
			return err
		}
		stored, err := db.GetSession("driver-check-session-" + suffix)
		if err != nil || stored == nil || stored.Username != username {
			return errorsOrMissing(err, "session")
		}
		return nil
	}); err != nil {
		return results, err
	}
	if err := run("transaction rollback", func() error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		rollbackUsername := username + "-rollback"
		if _, err := tx.Exec(`INSERT INTO user_profiles
			(user_id, username, nickname, rename_window_started_at, rename_count, updated_at)
			VALUES (?, ?, '', 0, 0, 0)`, uuid.NewString(), rollbackUsername); err != nil {
			return err
		}
		if err := tx.Rollback(); err != nil {
			return err
		}
		profile, err := db.GetUserProfile(rollbackUsername)
		if err == nil || profile != nil {
			return fmt.Errorf("rolled-back profile remained visible")
		}
		return nil
	}); err != nil {
		return results, err
	}
	if err := run("message deduplication", func() error {
		message := &core.UserMessage{
			ID: uuid.NewString(), Recipient: username, Sender: "system", Kind: "driver_check", Severity: "info",
			Title: "Driver check", Body: "Database message", DedupeKey: "driver-check-" + suffix, CreatedAt: now,
		}
		inserted, err := db.SaveMessageIfAbsent(message)
		if err != nil || !inserted {
			return errorsOrMissing(err, "first message insert")
		}
		inserted, err = db.SaveMessageIfAbsent(message)
		if err != nil || inserted {
			return errorsOrMissing(err, "message deduplication")
		}
		return nil
	}); err != nil {
		return results, err
	}
	globalTeamPrefix := "dbcheck-" + suffix
	memberUsername := "dbmember-" + suffix
	if err := run("global teams", func() error {
		if err := db.SaveToken(&core.AccessToken{
			Name: memberUsername, CreatedAt: time.Now().UTC().Format(time.RFC3339), Permissions: []string{"base"},
		}); err != nil {
			return err
		}
		team := &core.SuperTeam{
			Prefix: globalTeamPrefix, Name: "Driver Check Team", Description: "Database contract",
			CreatedAt: now,
		}
		if err := db.CreateSuperTeam(team, username, 5, 20); err != nil {
			return err
		}
		details, err := db.GetSuperTeamDetails(team.Prefix, username, false)
		if err != nil || details == nil || details.Team == nil ||
			details.Team.RoleLevel != core.SuperTeamRoleOwner || len(details.Members) != 1 {
			return errorsOrMissing(err, "global team")
		}
		teams, total, err := db.ListSuperTeams(username, false, 10, 0)
		if err != nil || total != 1 || len(teams) != 1 || teams[0].Prefix != team.Prefix {
			return errorsOrMissing(err, "global team listing")
		}
		invitationID := uuid.NewString()
		expiresAt := now + int64((time.Hour / time.Millisecond))
		invitation := &core.SuperTeamInvitation{
			ID: invitationID, TeamPrefix: team.Prefix, Inviter: username, Recipient: memberUsername,
			Level: core.SuperTeamRoleWrite, CreatedAt: now, ExpiresAt: expiresAt,
		}
		message := &core.UserMessage{
			ID: invitationID, Recipient: memberUsername, Sender: username, Kind: "super_team_invite", Severity: "info",
			Title: "Global team invitation", Body: "Driver check invitation", Payload: []byte(`{"prefix":"` + team.Prefix + `"}`),
			ActionKind: "super_team_invite", ActionStatus: core.MessageActionPending, CreatedAt: now, ExpiresAt: expiresAt,
		}
		if err := db.CreateSuperTeamInvitations(
			[]*core.SuperTeamInvitation{invitation}, []*core.UserMessage{message}); err != nil {
			return err
		}
		if err := db.RespondSuperTeamInvitation(invitationID, memberUsername, true, 20, now+1); err != nil {
			return err
		}
		if err := db.SetSuperTeamMemberLevel(team.Prefix, username, memberUsername,
			core.SuperTeamRoleManage, false); err != nil {
			return err
		}
		if err := db.RemoveSuperTeamMember(team.Prefix, username, memberUsername, false, now+2); err != nil {
			return err
		}
		memberTeams, memberTotal, err := db.ListSuperTeams(memberUsername, false, 10, 0)
		if err != nil || memberTotal != 0 || len(memberTeams) != 0 {
			return errorsOrMissing(err, "global team removal visibility")
		}
		return nil
	}); err != nil {
		return results, err
	}
	cargoRepository := "cargo-" + suffix
	dockerRepository := "docker-" + suffix
	mavenRepository := "maven-" + suffix
	mavenDomain := "io.renop." + suffix
	npmRepository := "npm-" + suffix
	if err := run("package catalogs", func() error {
		if err := db.RecordCargoPublication(&core.CargoPackage{
			Repository: cargoRepository, Name: "demo", NormalizedName: "demo", Description: "Driver check",
			CreatedAt: now, UpdatedAt: now,
		}, &core.CargoVersion{
			Repository: cargoRepository, Package: "demo", Version: "1.0.0", Publisher: username, CreatedAt: now,
		}, username); err != nil {
			return err
		}
		previousCargo, err := db.GetCargoPackage(cargoRepository, "demo")
		if err != nil || previousCargo == nil {
			return errorsOrMissing(err, "Cargo package snapshot")
		}
		if err := db.RecordCargoPublication(&core.CargoPackage{
			Repository: cargoRepository, Name: "demo", NormalizedName: "demo", Description: "Pending review",
			CreatedAt: now, UpdatedAt: now + 1,
		}, &core.CargoVersion{
			Repository: cargoRepository, Package: "demo", Version: "1.1.0", Publisher: username, CreatedAt: now + 1,
		}, username); err != nil {
			return err
		}
		if err := db.RollbackCargoPublicationReview(cargoRepository, "demo", "1.1.0", previousCargo); err != nil {
			return err
		}
		published, err := db.CargoHasPublishedVersions(cargoRepository, "demo")
		if err != nil || !published {
			return errorsOrMissing(err, "Cargo publication review rollback")
		}
		cargoDetails, err := db.GetCargoPackageDetails(cargoRepository, "demo", username)
		if err != nil || cargoDetails == nil || cargoDetails.Package == nil || len(cargoDetails.Versions) != 1 ||
			cargoDetails.Package.Description != previousCargo.Description {
			return errorsOrMissing(err, "Cargo publication review metadata restoration")
		}
		dockerImage := globalTeamPrefix + "/demo"
		if _, err := db.CreateDockerImageForTeam(
			dockerRepository, dockerImage, username, globalTeamPrefix, false, now); err != nil {
			return err
		}
		domain := &core.MavenDomain{
			Domain: mavenDomain, VerificationType: core.MavenVerificationDNS,
			VerificationHost: "example.test", VerificationCode: "driver-check-" + suffix,
			SuperTeamPrefix: globalTeamPrefix, CreatedAt: now,
		}
		if err := db.CreateMavenDomain(domain, username); err != nil {
			return err
		}
		if err := db.MarkMavenDomainVerified(domain.Domain, domain.VerificationCode, now); err != nil {
			return err
		}
		if err := db.RecordMavenPublication(&core.MavenArtifact{
			Repository: mavenRepository, Domain: domain.Domain, GroupID: domain.Domain, ArtifactID: "demo",
			SuperTeamPrefix: globalTeamPrefix, CreatedAt: now, UpdatedAt: now,
		}, &core.MavenVersion{
			Repository: mavenRepository, GroupID: domain.Domain, ArtifactID: "demo", Version: "1.0.0",
			Publisher: username, CreatedAt: now,
		}); err != nil {
			return err
		}
		npmPackage := "@" + globalTeamPrefix + "/demo"
		if _, err := db.CreateNPMPackageForTeam(
			npmRepository, npmPackage, username, globalTeamPrefix, true, now); err != nil {
			return err
		}
		return db.RecordNPMPublication(&core.NPMPackage{
			Repository: npmRepository, Name: npmPackage, Description: "Driver check", UpdatedAt: now,
		}, &core.NPMVersion{
			Repository: npmRepository, Package: npmPackage, Version: "1.0.0",
			ManifestJSON: `{"name":"` + npmPackage + `","version":"1.0.0"}`, Publisher: username,
			TarballPath: npmPackage + "/-/demo-1.0.0.tgz", CreatedAt: now,
		}, map[string]string{"latest": "1.0.0"}, username)
	}); err != nil {
		return results, err
	}
	if err := run("ownership review", func() error {
		if err := db.ForceAddSuperTeamMembers(globalTeamPrefix, username, []string{memberUsername},
			core.SuperTeamRoleManage, 5, 20, now+3); err != nil {
			return err
		}
		const reviewImage = "review-demo"
		if _, err := db.CreateDockerImage(dockerRepository, reviewImage, memberUsername, false, now+4); err != nil {
			return err
		}
		task, err := db.CreateSuperTeamTransferReview(core.SuperTeamTransferRequest{
			ResourceType: core.ReviewResourceDockerImage, Repository: dockerRepository,
			ResourceKey: reviewImage, TargetTeamPrefix: globalTeamPrefix,
		}, memberUsername, false, now+5)
		if err != nil {
			return err
		}
		reviewerTasks, reviewerTotal, err := db.ListReviewTasks(core.ReviewTaskListOptions{
			Username: username, ResourceTypes: []string{core.ReviewResourceDockerImage},
			Status: core.ReviewStatusPending, Limit: 10,
		})
		if err != nil || reviewerTotal != 1 || len(reviewerTasks) != 1 || reviewerTasks[0].ID != task.ID {
			return errorsOrMissing(err, "reviewer task listing")
		}
		requestedTasks, requestedTotal, err := db.ListReviewTasks(core.ReviewTaskListOptions{
			Username: memberUsername, RequestedView: true, Status: "all", Limit: 10,
		})
		if err != nil || requestedTotal != 1 || len(requestedTasks) != 1 || requestedTasks[0].ID != task.ID {
			return errorsOrMissing(err, "requester task listing")
		}
		if _, err := db.DecideReviewTask(task.ID, memberUsername,
			core.ReviewStatusApproved, "", now+6); err != nil {
			return err
		}
		if _, err := db.DecideReviewTask(task.ID, username,
			core.ReviewStatusRejected, "already decided", now+7); !errors.Is(err, core.ErrReviewTaskConflict) {
			return errorsOrMissing(err, "single review decision")
		}
		image, err := db.GetDockerImage(dockerRepository, reviewImage)
		if err != nil || image == nil || image.SuperTeamPrefix != globalTeamPrefix {
			return errorsOrMissing(err, "review ownership transfer")
		}
		if err := db.SaveToken(&core.AccessToken{
			Name: username, CreatedAt: time.Now().UTC().Format(time.RFC3339),
			Permissions: []string{"base", "canmoderate:" + mavenRepository},
		}); err != nil {
			return err
		}
		publication, err := db.CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
			ResourceType: core.ReviewResourceMavenArtifact, Repository: mavenRepository,
			ResourceKey: mavenDomain + ":reviewed", ResourceName: mavenDomain + ":reviewed",
			Version: "1.0.0", RequestedBy: memberUsername, Policy: config.PublicationReviewEveryVersion,
			Payload:   []byte(`{"driver":"contract"}`),
			CreatedAt: now + 8, Files: []*core.ReviewFile{{
				Path: strings.ReplaceAll(mavenDomain, ".", "/") + "/reviewed/1.0.0/reviewed-1.0.0.jar",
				Size: 128, Critical: true,
			}},
		})
		if err != nil || publication == nil || !publication.Pending {
			return errorsOrMissing(err, "publication review creation")
		}
		payload, err := db.GetReviewTaskPayload(publication.TaskID)
		if err != nil || string(payload) != `{"driver":"contract"}` {
			return errorsOrMissing(err, "publication review payload")
		}
		files, err := db.ListReviewTaskFiles(publication.TaskID)
		if err != nil || len(files) != 1 || !files[0].Critical {
			return errorsOrMissing(err, "publication review files")
		}
		moderatedTasks, moderatedTotal, err := db.ListReviewTasks(core.ReviewTaskListOptions{
			Username: username, ModeratedRepositories: []string{mavenRepository},
			ResourceTypes: []string{core.ReviewResourceMavenArtifact},
			Status:        core.ReviewStatusPending, Limit: 10,
		})
		if err != nil || moderatedTotal != 1 || len(moderatedTasks) != 1 ||
			moderatedTasks[0].ID != publication.TaskID {
			return errorsOrMissing(err, "publication reviewer task listing")
		}
		if _, err := db.DecideReviewTask(publication.TaskID, username,
			core.ReviewStatusApproved, "", now+core.PublicationReviewSettleMillis+20); err != nil {
			return err
		}
		pending, err := db.IsPublicationReviewPathPending(mavenRepository, files[0].Path)
		if err != nil || pending {
			return errorsOrMissing(err, "publication review completion")
		}
		return nil
	}); err != nil {
		return results, err
	}
	if err := run("download statistics", func() error {
		if err := db.BatchIncrementDownloadStatistics([]*core.DownloadStatisticDelta{{
			Username: username, Repository: dockerRepository, Format: config.RepositoryFormatDocker,
			Package: globalTeamPrefix + "/demo", Version: "latest", Count: 2, Bytes: 256, UpdatedAt: now,
		}}); err != nil {
			return err
		}
		page, err := db.QueryDownloadStatistics(core.DownloadStatisticsQuery{
			Repository: dockerRepository, GroupBy: "version", Limit: 20,
		})
		if err != nil || page == nil || page.Count != 2 {
			return errorsOrMissing(err, "download statistics")
		}
		return nil
	}); err != nil {
		return results, err
	}
	return results, nil
}

func errorsOrMissing(err error, resource string) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("%s contract failed", resource)
}
