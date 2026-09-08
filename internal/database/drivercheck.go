/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/mail"
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
	results := make([]DriverCheckResult, 0, 17)
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
		banUntil := now + int64(time.Hour/time.Millisecond)
		if err := db.SetAccountBan(username, &core.AccountBan{
			Reason: "Driver contract suspension", CreatedAt: now, ExpiresAt: &banUntil,
		}); err != nil {
			return err
		}
		account, err = db.GetTokenByName(username)
		if err != nil || account == nil || !account.Ban.IsActive(now) {
			return errorsOrMissing(err, "account suspension")
		}
		stored, err = db.GetSession("driver-check-session-" + suffix)
		if err != nil || stored != nil {
			return errorsOrMissing(err, "suspended account session revocation")
		}
		if err := db.SetAccountBan(username, nil); err != nil {
			return err
		}
		if err := db.UpdateToken(username, func(token *core.AccessToken) { token.Permissions = []string{"canmoderate:driver-check"} }); err != nil {
			return err
		}
		if err := db.SetAccountBan(username, &core.AccountBan{Reason: "Protected role", CreatedAt: now}); !errors.Is(err, core.ErrAccountBanProtected) {
			return errorsOrMissing(err, "protected account suspension")
		}
		if err := db.UpdateToken(username, func(token *core.AccessToken) { token.Permissions = []string{"base"} }); err != nil {
			return err
		}
		deprecationKey := "driver-check-" + suffix
		if err := db.DeprecatePackage(config.RepositoryFormatCargo, "driver-check", deprecationKey, now); err != nil {
			return err
		}
		if err := db.EnsurePackageMutable(config.RepositoryFormatCargo, "driver-check", deprecationKey); !errors.Is(err, core.ErrPackageDeprecated) {
			return errorsOrMissing(err, "permanent package deprecation")
		}
		retiredUsername := "retired_" + suffix
		if err := db.SaveToken(&core.AccessToken{
			Name: retiredUsername, EncryptedSecret: "retired-password",
			CreatedAt: time.Now().UTC().Format(time.RFC3339), Permissions: []string{"base"},
		}); err != nil {
			return err
		}
		if _, err := db.UpdateAccountEmail(retiredUsername, retiredUsername+"@example.test", now); err != nil {
			return err
		}
		retiredRepository := "retirement-" + suffix
		if _, err := db.CreateNPMPackage(retiredRepository, "frozen", retiredUsername, false, now); err != nil {
			return err
		}
		plan, err := db.GetAccountRetirementPlan(retiredUsername)
		if err != nil || plan == nil || plan.Eligible || plan.PackageOwnerCount != 1 {
			return errorsOrMissing(err, "account retirement ownership blocker")
		}
		if err := db.RetireAccount(retiredUsername, now+1); !errors.Is(err, core.ErrAccountRetirementBusy) {
			return errorsOrMissing(err, "retirement ownership enforcement")
		}
		if err := db.DeprecatePackage(config.RepositoryFormatNPM, retiredRepository, "frozen", now); err != nil {
			return err
		}
		if err := db.SaveAuditLog(&core.AuditLogEntry{
			Username: retiredUsername, Operator: retiredUsername, Action: "DRIVER_CHECK",
			CreatedAt: now - int64(40*24*time.Hour/time.Millisecond),
		}); err != nil {
			return err
		}
		if err := db.RetireAccount(retiredUsername, now+1); err != nil {
			return err
		}
		retired, err := db.GetTokenByName(retiredUsername)
		if err != nil || retired == nil || retired.DeletedAt != now+1 || retired.EncryptedSecret != "" {
			return errorsOrMissing(err, "account retirement tombstone")
		}
		if err := db.CreateToken(&core.AccessToken{Name: retiredUsername}, "", now+2); !errors.Is(err, core.ErrUsernameAlreadyExists) {
			return errorsOrMissing(err, "retired username reservation")
		}
		if err := db.CleanExpiredAuditLogs(1, 1); err != nil {
			return err
		}
		_, retained, err := db.GetAuditLogs(retiredUsername, 1, 0)
		if err != nil || retained != 1 {
			return errorsOrMissing(err, "retired activity retention")
		}
		if err := db.CleanupRetiredAccountData(now+1+core.AccountAuditRetentionMillis, 100); err != nil {
			return err
		}
		status, err := db.GetAccountRetirementStatus(retiredUsername)
		if err != nil || status == nil || status.EmailReleasedAt == 0 || status.AuditPurgedAt == 0 {
			return errorsOrMissing(err, "retired account retention cleanup")
		}
		if err := db.SaveAuditLog(&core.AuditLogEntry{
			Username: retiredUsername, Operator: retiredUsername, Action: "LATE_EVENT", CreatedAt: now,
		}); err != nil {
			return err
		}
		_, retained, err = db.GetAuditLogs(retiredUsername, 1, 0)
		if err != nil || retained != 0 {
			return errorsOrMissing(err, "retired activity cannot be restored")
		}
		return nil
	}); err != nil {
		return results, err
	}
	if err := run("global log filters", func() error {
		for _, entry := range []*core.AuditLogEntry{
			{Username: username, Operator: "operator-a", Action: "FILTER_CHECK", AuthMethod: "Web", CreatedAt: now},
			{Username: username, Operator: "operator-b", Action: "FILTER_CHECK", Trigger: "api", CreatedAt: now + 1},
			{Username: username, Operator: "system", Action: "FILTER_CHECK", Kind: "system", Trigger: "http", Severity: "error", CreatedAt: now + 2},
		} {
			if err := db.SaveAuditLog(entry); err != nil {
				return err
			}
		}
		filter := core.AuditLogFilter{Username: username, Action: "FILTER_CHECK", Kind: "audit", Trigger: "web", Operator: "operator-a", Initiator: "operator-a", From: now, Until: now}
		entries, total, err := db.FilterAuditLogs(filter, 1, 0)
		if err != nil || total != 1 || len(entries) != 1 || entries[0].Trigger != "web" || entries[0].Initiator != "operator-a" {
			return errorsOrMissing(err, "inclusive combined log filters")
		}
		filter = core.AuditLogFilter{Username: username, Action: "FILTER_CHECK", ExcludeOperator: "operator-a", ExcludeInitiator: "operator-a", From: now, Until: now + 2}
		entries, total, err = db.FilterAuditLogs(filter, 1, 1)
		if err != nil || total != 2 || len(entries) != 1 || entries[0].Operator != "operator-b" {
			return errorsOrMissing(err, "masked operator filter and pagination")
		}
		filter.Kind, filter.Severity, filter.Trigger = "system", "error", "http"
		entries, total, err = db.FilterAuditLogs(filter, 10, 0)
		if err != nil || total != 1 || len(entries) != 1 || entries[0].Kind != "system" {
			return errorsOrMissing(err, "system log filtering")
		}
		filter.Operator = "' OR 1=1 --"
		_, total, err = db.FilterAuditLogs(filter, 10, 0)
		if err != nil || total != 0 {
			return errorsOrMissing(err, "log filter parameter binding")
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
	if err := run("profile avatars", func() error {
		data := []byte("driver-check-sanitized-avatar")
		sum := sha256.Sum256(data)
		if err := db.PutUserAvatar(username, &core.UserAvatar{
			ContentType: "image/png", Data: data, Size: int64(len(data)),
			SHA256: hex.EncodeToString(sum[:]), UpdatedAt: now,
		}); err != nil {
			return err
		}
		avatar, err := db.GetUserAvatar(username)
		if err != nil || avatar == nil || !strings.EqualFold(avatar.SHA256, hex.EncodeToString(sum[:])) {
			return errorsOrMissing(err, "profile avatar")
		}
		if err := db.DeleteUserAvatar(username); err != nil {
			return err
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
		deleted, err := db.DeleteMessagesByDedupeKey(message.DedupeKey)
		if err != nil || deleted != 1 {
			return errorsOrMissing(err, "workflow notification cleanup")
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
		reviewers, err := db.ListSuperTeamReviewerNames(team.Prefix)
		reviewerSet := make(map[string]struct{}, len(reviewers))
		for _, reviewer := range reviewers {
			reviewerSet[reviewer] = struct{}{}
		}
		_, hasOwner := reviewerSet[username]
		_, hasManager := reviewerSet[memberUsername]
		if err != nil || len(reviewers) != 2 || !hasOwner || !hasManager {
			return errorsOrMissing(err, "global team reviewer listing")
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
	if err := run("publication quotas", func() error {
		limits := core.PublicationQuotaLimits{
			FileLimit: 3, ByteLimit: 1024, PublicationLimit: 2, Period: core.PublicationQuotaPeriodMonth,
		}
		subject := core.PublicationQuotaSubject{OwnerType: core.PublicationQuotaOwnerUser, OwnerKey: username}
		reservation, err := db.ReservePublicationQuota(subject, limits,
			core.PublicationQuotaDelta{Files: 2, Bytes: 512, Publications: 1}, now, now+60_000)
		if err != nil || reservation == nil || reservation.ID == "" {
			return errorsOrMissing(err, "publication quota reservation")
		}
		if err := db.CommitPublicationQuotaReservation(reservation.ID, now+1); err != nil {
			return err
		}
		status, err := db.GetPublicationQuotaStatus(subject, limits, now+2)
		if err != nil || status == nil || status.FilesUsed != 2 || status.BytesUsed != 512 ||
			status.PublicationsUsed != 1 {
			return errorsOrMissing(err, "publication quota usage")
		}
		if _, err := db.ReservePublicationQuota(subject, limits,
			core.PublicationQuotaDelta{Files: 2}, now+3, now+60_000); !errors.Is(err, core.ErrPublicationFileLimit) {
			return errorsOrMissing(err, "publication quota enforcement")
		}
		unlimited := true
		if err := db.SetPublicationQuotaOverride(core.PublicationQuotaSubject{
			OwnerType: core.PublicationQuotaOwnerSuperTeam, OwnerKey: globalTeamPrefix,
		}, core.PublicationQuotaOverride{Unlimited: &unlimited}, now+4); err != nil {
			return err
		}
		teamReservation, err := db.ReservePublicationQuota(core.PublicationQuotaSubject{
			OwnerType: core.PublicationQuotaOwnerSuperTeam, OwnerKey: globalTeamPrefix,
		}, limits, core.PublicationQuotaDelta{Files: 100, Bytes: 1 << 30, Publications: 100}, now+5, now+60_000)
		if err != nil || teamReservation == nil || !teamReservation.Unlimited {
			return errorsOrMissing(err, "unlimited global team quota")
		}
		return nil
	}); err != nil {
		return results, err
	}
	if err := run("long-term quotas", func() error {
		limits := core.PublicationQuotaLimits{
			FileLimit: 1, ByteLimit: 1024, PublicationLimit: 1, Period: core.PublicationQuotaPeriodLifetime,
		}
		subject := core.PublicationQuotaSubject{OwnerType: core.PublicationQuotaOwnerUser, OwnerKey: username}
		reservation, err := db.ReservePublicationQuota(subject, limits,
			core.PublicationQuotaDelta{Files: 1, Bytes: 512, Publications: 1}, now, now+60_000)
		if err != nil {
			return err
		}
		if err := db.CommitPublicationQuotaReservation(reservation.ID, now+1); err != nil {
			return err
		}
		later := now + (400 * 24 * time.Hour).Milliseconds()
		if err := db.CleanExpiredPublicationQuotaReservations(later); err != nil {
			return err
		}
		status, err := db.GetPublicationQuotaStatus(subject, limits, later)
		if err != nil || status == nil || status.PeriodStart != 0 || status.PeriodEnd != 0 ||
			status.FilesUsed != 1 || status.BytesUsed != 512 || status.PublicationsUsed != 1 {
			return errorsOrMissing(err, "long-term usage after periodic cleanup")
		}
		if _, err := db.ReservePublicationQuota(subject, limits, core.PublicationQuotaDelta{Files: 1},
			later, later+60_000); !errors.Is(err, core.ErrPublicationFileLimit) {
			return errorsOrMissing(err, "long-term quota enforcement")
		}
		limits.Period = core.PublicationQuotaPeriodMonth
		status, err = db.GetPublicationQuotaStatus(subject, limits, now)
		if err != nil || status == nil || status.FilesUsed != 0 {
			return errorsOrMissing(err, "expired periodic usage cleanup")
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
		if err := db.PutDockerManifest(&core.DockerManifest{
			Repository: dockerRepository, ImageName: dockerImage,
			Digest:    "sha256:1111111111111111111111111111111111111111111111111111111111111111",
			MediaType: "application/vnd.docker.distribution.manifest.v2+json",
			RawJSON:   []byte(`{"schemaVersion":2}`), CreatedAt: now,
		}, "latest", username); err != nil {
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
		lifecycleDomain := "released." + suffix + ".example"
		closedAt := now - core.MavenDomainReleaseLockMillis
		originalClaim := &core.MavenDomain{
			Domain: lifecycleDomain, VerificationType: core.MavenVerificationDNS,
			VerificationHost: "example.test", VerificationCode: "original-" + suffix,
			CreatedAt: closedAt - 1,
		}
		if err := db.CreateMavenDomain(originalClaim, username); err != nil {
			return err
		}
		if err := db.CloseMavenDomain(lifecycleDomain, username, false, closedAt); err != nil {
			return err
		}
		reclaimed := &core.MavenDomain{
			Domain: lifecycleDomain, VerificationType: core.MavenVerificationDNS,
			VerificationHost: "example.test", VerificationCode: "reclaimed-" + suffix,
			CreatedAt: now,
		}
		if err := db.CreateMavenDomain(reclaimed, memberUsername); err != nil {
			return err
		}
		if err := db.MarkMavenDomainVerified(lifecycleDomain, reclaimed.VerificationCode, now+1); err != nil {
			return err
		}
		if err := db.ReviewMavenDomainClaim(lifecycleDomain, core.ReviewStatusApproved, now+2); err != nil {
			return err
		}
		reclaimedDetails, err := db.GetMavenDomainDetails(lifecycleDomain, memberUsername)
		if err != nil || reclaimedDetails == nil || reclaimedDetails.Domain == nil ||
			!reclaimedDetails.Domain.Verified || reclaimedDetails.Domain.ClaimStatus != "" {
			return errorsOrMissing(err, "released Maven domain reviewed reclaim")
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
		images, err := db.ListDockerImages(dockerRepository, "", 100)
		if err != nil {
			return err
		}
		listed := false
		for _, candidate := range images {
			listed = listed || candidate != nil && candidate.ImageName == reviewImage
		}
		if !listed {
			return errors.New("docker batch list omitted reviewed image")
		}
		images, total, err := db.SearchDockerImages(dockerRepository, reviewImage, 10, 0)
		if err != nil || total != 1 || len(images) != 1 || images[0].ImageName != reviewImage {
			return errorsOrMissing(err, "Docker batch search")
		}
		memberLevels, err := db.DockerImageMemberLevels(
			dockerRepository, memberUsername, []string{reviewImage})
		if _, member := memberLevels[reviewImage]; err != nil || !member {
			return errorsOrMissing(err, "Docker batch membership")
		}
		if err := db.SaveToken(&core.AccessToken{
			Name: username, CreatedAt: time.Now().UTC().Format(time.RFC3339),
			Permissions: []string{
				"base", "canupdate:" + dockerRepository, "canupdate:" + npmRepository,
				"canmoderate:" + dockerRepository, "canmoderate:" + npmRepository,
				"canmoderate:" + mavenRepository,
			},
		}); err != nil {
			return err
		}
		dockerCreation, err := db.CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
			ResourceType: core.ReviewResourceDockerImage, Repository: dockerRepository,
			ResourceKey: "review-created", ResourceName: "review-created",
			Version: core.ReviewVersionPackageCreation, RequestedBy: username,
			Policy: config.PublicationReviewNewPackages, Payload: []byte(`{"driver":"docker-create"}`),
			CreatedAt: now + 8, Files: []*core.ReviewFile{{
				Path: "review-requests/docker/driver-contract.json", Size: 64, Critical: true,
			}},
		})
		if err != nil || dockerCreation == nil || !dockerCreation.Pending {
			return errorsOrMissing(err, "Docker creation review")
		}
		if _, err := db.ApproveDockerImageCreationReview(
			dockerCreation.TaskID, username, dockerRepository, "review-created", "", false,
			now+8, now+8+core.PublicationReviewSettleMillis+1); err != nil {
			return err
		}
		createdImage, err := db.GetDockerImage(dockerRepository, "review-created")
		if err != nil || createdImage == nil || createdImage.Publisher != username {
			return errorsOrMissing(err, "Docker creation review result")
		}
		npmCreation, err := db.CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
			ResourceType: core.ReviewResourceNPMPackage, Repository: npmRepository,
			ResourceKey: "review-created", ResourceName: "review-created",
			Version: core.ReviewVersionPackageCreation, RequestedBy: username,
			Policy: config.PublicationReviewNewPackages, Payload: []byte(`{"driver":"npm-create"}`),
			CreatedAt: now + 9, Files: []*core.ReviewFile{{
				Path: "review-requests/npm/driver-contract.json", Size: 64, Critical: true,
			}},
		})
		if err != nil || npmCreation == nil || !npmCreation.Pending {
			return errorsOrMissing(err, "npm creation review")
		}
		if _, err := db.ApproveNPMPackageCreationReview(
			npmCreation.TaskID, username, npmRepository, "review-created", "", false,
			now+9, now+9+core.PublicationReviewSettleMillis+1); err != nil {
			return err
		}
		createdPackage, err := db.GetNPMPackage(npmRepository, "review-created")
		if err != nil || createdPackage == nil || createdPackage.Publisher != username {
			return errorsOrMissing(err, "npm creation review result")
		}
		if err := db.SaveToken(&core.AccessToken{
			Name: memberUsername, CreatedAt: time.Now().UTC().Format(time.RFC3339),
			Permissions: []string{"base", "canupdate:" + npmRepository},
		}); err != nil {
			return err
		}
		if err := db.SetSuperTeamMemberLevel(globalTeamPrefix, username, memberUsername,
			core.SuperTeamRoleWrite, false); err != nil {
			return err
		}
		teamPackageName := "@" + globalTeamPrefix + "/review-created"
		teamCreation, err := db.CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
			ResourceType: core.ReviewResourceNPMPackage, Repository: npmRepository,
			ResourceKey: teamPackageName, ResourceName: teamPackageName,
			Version: core.ReviewVersionPackageCreation, RequestedBy: memberUsername,
			Policy: config.PublicationReviewNewPackages, ReviewTeamPrefix: globalTeamPrefix,
			TargetTeamPrefix: globalTeamPrefix, Payload: []byte(`{"driver":"team-npm-create"}`),
			CreatedAt: now + 10, Files: []*core.ReviewFile{{
				Path: "review-requests/npm/team-driver-contract.json", Size: 64, Critical: true,
			}},
		})
		if err != nil || teamCreation == nil || !teamCreation.Pending {
			return errorsOrMissing(err, "team npm creation review")
		}
		advanced, err := db.AdvancePackageCreationReview(
			teamCreation.TaskID, username, now+10+core.PublicationReviewSettleMillis+1)
		if err != nil || advanced == nil || advanced.Status != core.ReviewStatusPending ||
			advanced.ReviewTeamPrefix != "" || advanced.TargetTeamPrefix != globalTeamPrefix {
			return errorsOrMissing(err, "team npm creation review advancement")
		}
		if _, err := db.ApproveNPMPackageCreationReview(
			teamCreation.TaskID, username, npmRepository, teamPackageName, globalTeamPrefix, false,
			now+10, now+10+core.PublicationReviewSettleMillis+2); err != nil {
			return err
		}
		teamCreatedPackage, err := db.GetNPMPackage(npmRepository, teamPackageName)
		if err != nil || teamCreatedPackage == nil || teamCreatedPackage.Publisher != memberUsername ||
			teamCreatedPackage.SuperTeamPrefix != globalTeamPrefix {
			return errorsOrMissing(err, "team npm creation review result")
		}
		dockerPublication, err := db.CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
			ResourceType: core.ReviewResourceDockerImage, Repository: dockerRepository,
			ResourceKey: globalTeamPrefix + "/demo", ResourceName: globalTeamPrefix + "/demo",
			Version: "reviewed", RequestedBy: memberUsername, Policy: config.PublicationReviewEveryVersion,
			Payload: []byte(`{"driver":"docker-contract"}`), CreatedAt: now + 10,
			Files: []*core.ReviewFile{{Path: "review-manifests/driver-contract.json", Size: 64, Critical: true}},
		})
		if err != nil || dockerPublication == nil || !dockerPublication.Pending {
			return errorsOrMissing(err, "Docker publication review creation")
		}
		approvedDocker, err := db.ApproveDockerPublicationReview(
			dockerPublication.TaskID, username, &core.DockerManifest{
				Repository: dockerRepository, ImageName: globalTeamPrefix + "/demo",
				Digest: "sha256:" + strings.Repeat("d", 64), MediaType: "application/vnd.oci.image.manifest.v1+json",
				RawJSON: []byte(`{"schemaVersion":2}`), CreatedAt: now + 10,
			}, "reviewed", now+10+core.PublicationReviewSettleMillis+1)
		if err != nil || approvedDocker == nil || approvedDocker.Status != core.ReviewStatusApproved {
			return errorsOrMissing(err, "Docker publication review approval")
		}
		dockerTag, err := db.GetDockerTag(dockerRepository, globalTeamPrefix+"/demo", "reviewed")
		if err != nil || dockerTag == nil || dockerTag.Digest != "sha256:"+strings.Repeat("d", 64) {
			return errorsOrMissing(err, "Docker publication review catalog")
		}
		publication, err := db.CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
			ResourceType: core.ReviewResourceMavenArtifact, Repository: mavenRepository,
			ResourceKey: mavenDomain + ":reviewed", ResourceName: mavenDomain + ":reviewed",
			Version: "1.0.0", RequestedBy: memberUsername, Policy: config.PublicationReviewEveryVersion,
			Payload:   []byte(`{"driver":"contract"}`),
			CreatedAt: now + 11, Files: []*core.ReviewFile{{
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
			core.ReviewStatusApproved, "", now+core.PublicationReviewSettleMillis+23); err != nil {
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
	if err := run("durable email queue", func() error {
		cfg := mail.DefaultConfig()
		if err := cfg.EnsureKey(); err != nil {
			return err
		}
		profile, err := db.GetUserProfile(username)
		if err != nil || profile == nil {
			return errorsOrMissing(err, "mail owner")
		}
		makeJob := func(id string) *mail.Job {
			return &mail.Job{ID: id, AccountID: "driver-mail-" + suffix, UserID: profile.UserID, Actor: username, Scene: "test", CreatedAt: now, ExpiresAt: now + 600000, Message: mail.Message{ID: id, To: "receiver@example.test", Subject: "Private driver message", Text: "Private verification code", CreatedAt: now}}
		}
		job := makeJob("driver-mail-" + suffix)
		inserted, err := db.QueueMailJob(job, cfg.EncryptionKey, "192.0.2.99", cfg.ManualRate)
		if err != nil || !inserted {
			return errorsOrMissing(err, "mail queue insert")
		}
		inserted, err = db.QueueMailJob(job, cfg.EncryptionKey, "192.0.2.99", cfg.ManualRate)
		if err != nil || inserted {
			return errorsOrMissing(err, "mail queue deduplication")
		}
		if _, err = db.QueueMailJob(makeJob("driver-mail-limit-"+suffix), cfg.EncryptionKey, "192.0.2.99", cfg.ManualRate); !errors.Is(err, mail.ErrRateLimited) {
			return errorsOrMissing(err, "mail IP rate limit")
		}
		queued, err := db.NextMailJob(cfg.EncryptionKey, now)
		if err != nil || queued == nil || queued.ID != job.ID {
			return errorsOrMissing(err, "due mail selection")
		}
		var sealed string
		if err = db.QueryRow("SELECT payload FROM mail_jobs WHERE id = ?", job.ID).Scan(&sealed); err != nil {
			return err
		}
		if strings.Contains(sealed, job.Message.Text) || strings.Contains(sealed, job.Message.To) {
			return errors.New("mail payload was not encrypted")
		}
		if _, _, err = db.ListMailJobs(profile.UserID, "queued", cfg.EncryptionKey, 20, 0); err != nil {
			return err
		}
		owner := "mail-worker-" + suffix
		_, owned, err := db.AcquireMailLease(owner, now)
		if err != nil || !owned {
			return errorsOrMissing(err, "mail worker lease")
		}
		_, owned, err = db.AcquireMailLease("other-worker", now)
		if err != nil || owned {
			return errorsOrMissing(err, "mail lease exclusivity")
		}
		state := mail.AccountState{}
		account := mail.Account{ID: job.AccountID, Quota: mail.Quota{Limit: 1, Period: "day"}, Overage: mail.Quota{Limit: -1, Period: "day"}}
		reservation, reason := state.Reserve(account, cfg.AccountRate, time.UnixMilli(now))
		if reason != "" {
			return fmt.Errorf("mail reservation: %s", reason)
		}
		job.Status, job.UpdatedAt, job.Reservation = "sending", now, reservation
		if err = db.SaveMailAttempt(owner, cfg.EncryptionKey, job, &state, now+5000); err != nil {
			return err
		}
		restored, err := db.LoadMailAccount(account.ID, cfg.EncryptionKey)
		if err != nil || restored.Charged != 1 {
			return errorsOrMissing(err, "mail durable accounting")
		}
		if err = db.RecoverMailAttempts(owner, now+1); err != nil {
			return err
		}
		stored, err := db.GetMailJob(job.ID, cfg.EncryptionKey)
		if err != nil || stored == nil || stored.Status != "unknown" {
			return errorsOrMissing(err, "mail restart uncertainty")
		}
		queued, err = db.NextMailJob(cfg.EncryptionKey, now+10000)
		if err != nil || queued != nil {
			return errorsOrMissing(err, "mail duplicate-send prevention")
		}
		stored.Status, stored.UpdatedAt, stored.Result = "failed", now+2, mail.Result{Status: "failed", Code: "HTTP_401"}
		restored.Refund(reservation)
		if err = db.SaveMailAttempt(owner, cfg.EncryptionKey, stored, &restored, now+5000); err != nil {
			return err
		}
		restored.Charged = 999
		if err = db.SaveMailAttempt(owner, cfg.EncryptionKey, stored, &restored, 0); !errors.Is(err, mail.ErrFinalized) {
			return errorsOrMissing(err, "mail terminal replay protection")
		}
		if err = db.CleanMailData(now+int64(8*24*time.Hour/time.Millisecond), []string{account.ID}); err != nil {
			return err
		}
		stored, err = db.GetMailJob(job.ID, cfg.EncryptionKey)
		if err != nil || stored != nil {
			return errorsOrMissing(err, "mail retention")
		}
		restored, err = db.LoadMailAccount(account.ID, cfg.EncryptionKey)
		if err != nil || restored.Attempts != 1 || restored.Charged != 0 {
			return errorsOrMissing(err, "mail retained account counter")
		}
		return db.ReleaseMailLease(owner)
	}); err != nil {
		return results, err
	}
	if err := run("email password reset", func() error {
		username := "reset_" + suffix
		if err := db.SaveToken(&core.AccessToken{Name: username, EncryptedSecret: "old-password", Permissions: []string{"base"}}); err != nil {
			return err
		}
		cfg := mail.DefaultConfig()
		if err := cfg.EnsureKey(); err != nil {
			return err
		}
		email := username + "@example.test"
		if _, err := db.UpdateAccountEmail(username, email, now); err != nil {
			return err
		}
		hash, wrong := strings.Repeat("a", 64), strings.Repeat("b", 64)
		makeJob := func(id, address string) *mail.Job {
			return &mail.Job{ID: id, AccountID: "driver-reset", Scene: "password_reset", TicketHash: hash,
				CreatedAt: now, ExpiresAt: now + 600000,
				Message: mail.Message{ID: id, To: address, Subject: "Reset", Text: "Verification code", CreatedAt: now}}
		}
		job := makeJob("reset-"+suffix, email)
		created, err := db.QueueEmailPasswordReset(job, hash, cfg.EncryptionKey, "192.0.2.100", cfg.ManualRate)
		if err != nil || !created {
			return errorsOrMissing(err, "reset queue insert")
		}
		replacement := makeJob("reset-replace-"+suffix, email)
		if _, err = db.QueueEmailPasswordReset(replacement, wrong, cfg.EncryptionKey, "192.0.2.101", cfg.ManualRate); !errors.Is(err, mail.ErrRateLimited) {
			return errorsOrMissing(err, "reset email cooldown")
		}
		stored, err := db.GetMailJob(replacement.ID, cfg.EncryptionKey)
		if err != nil || stored != nil {
			return errorsOrMissing(err, "reset queue rollback")
		}
		if _, err = db.ResetPasswordWithEmailCode(email, wrong, "new-password", now+1); !errors.Is(err, core.ErrEmailCodeInvalid) {
			return errorsOrMissing(err, "reset invalid code")
		}
		session := &core.Session{PublicID: "reset-session-" + suffix, Username: username, CreatedAt: now, LoginMethod: "password"}
		session.LastActive.Store(now)
		if err = db.SaveSession(session, "reset-secret-"+suffix); err != nil {
			return err
		}
		mfa, err := db.GetMFAState(username)
		if err != nil {
			return err
		}
		if err = db.UpdateMFA(username, mfa.Snapshot, "encrypted-authenticator", false, 0, "reset-secret-"+suffix); err != nil {
			return err
		}
		recovered, err := db.ResetPasswordWithEmailCode(email, hash, "reset-password", now+2)
		if err != nil || recovered != username {
			return errorsOrMissing(err, "reset password consumption")
		}
		mfa, err = db.GetMFAState(username)
		if err != nil || mfa.Secret != "encrypted-authenticator" {
			return errorsOrMissing(err, "email reset preserves second factors")
		}
		previous, err := db.GetSession("reset-secret-" + suffix)
		if err != nil || previous != nil {
			return errorsOrMissing(err, "reset session revocation")
		}
		token, err := db.GetTokenByName(username)
		if err != nil || token == nil || token.EncryptedSecret != "reset-password" {
			return errorsOrMissing(err, "reset password persistence")
		}
		if _, err = db.ResetPasswordWithEmailCode(email, hash, "replayed-password", now+3); !errors.Is(err, core.ErrEmailCodeInvalid) {
			return errorsOrMissing(err, "reset replay rejection")
		}
		unknown := makeJob("reset-unknown-"+suffix, "unknown-"+email)
		created, err = db.QueueEmailPasswordReset(unknown, hash, cfg.EncryptionKey, "192.0.2.101", cfg.ManualRate)
		if err != nil || !created {
			return errorsOrMissing(err, "unknown email ownership verification")
		}
		if _, err = db.ResetPasswordWithEmailCode(unknown.Message.To, hash, "new-password", now+2); !errors.Is(err, core.ErrEmailCodeInvalid) {
			return errorsOrMissing(err, "unknown account reset rejection")
		}
		if err = db.CleanMailData(now+600000, nil); err != nil {
			return err
		}
		var count int
		if err = db.QueryRow(`SELECT COUNT(*) FROM user_password_resets WHERE email = ?`, unknown.Message.To).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("expired password proof retained")
		}
		return nil
	}); err != nil {
		return results, err
	}
	if err := run("second-factor policy and recovery", func() error {
		username, sessionID := "mfa_"+suffix, "mfa-session-"+suffix
		if err := db.SaveToken(&core.AccessToken{Name: username, EncryptedSecret: "password", Permissions: []string{"base"}}); err != nil {
			return err
		}
		session := &core.Session{PublicID: sessionID, Username: username, CreatedAt: now}
		session.LastActive.Store(now)
		if err := db.SaveSession(session, sessionID); err != nil {
			return err
		}
		if err := db.SaveFidoDevice(&core.FidoDevice{ID: "mfa-key-" + suffix, Username: username, CredentialID: []byte("mfa-credential-" + suffix), PublicKey: []byte("key"), CreatedAt: now}); err != nil {
			return err
		}
		mfa, err := db.GetMFAState(username)
		if err != nil {
			return err
		}
		before := mfa.Snapshot
		if err := db.UpdateMFA(username, before, "encrypted-totp", true, 0, sessionID); err != nil {
			return err
		}
		security, err := db.GetAccountSecurity(username)
		if err != nil || !security.TOTPEnabled || !security.PasskeySecondFactor || security.CanDisablePasswordLogin {
			return errorsOrMissing(err, "second-factor policy visibility")
		}
		if _, err := db.SetPasswordLoginEnabled(username, false, now); !errors.Is(err, core.ErrLastLoginMethod) {
			return errorsOrMissing(err, "secondary passkey is not a primary login")
		}
		if err := db.DeleteFidoDevice(username, "mfa-key-"+suffix); !errors.Is(err, core.ErrLastLoginMethod) {
			return errorsOrMissing(err, "last secondary passkey protection")
		}
		session.AuthenticationSnapshot = before
		if err := db.SaveSession(session, "stale-"+sessionID); !errors.Is(err, core.ErrMFAInvalid) {
			return errorsOrMissing(err, "stale login policy rejection")
		}
		mfa, err = db.GetMFAState(username)
		if err != nil {
			return err
		}
		if err := db.ConsumeMFACode(username, mfa.Revision, 100, now); err != nil {
			return err
		}
		for range 5 {
			if err := db.ConsumeMFACode(username, mfa.Revision, 100, now); !errors.Is(err, core.ErrMFAInvalid) {
				return errorsOrMissing(err, "one-time code replay rejection")
			}
		}
		if err := db.ConsumeMFACode(username, mfa.Revision, 101, now); !errors.Is(err, core.ErrMFAInvalid) {
			return errorsOrMissing(err, "account-wide code attempt limit")
		}
		if err := db.ConsumeMFACode(username, mfa.Revision, 101, now+300000); err != nil {
			return err
		}
		hashes := make([]core.RecoveryCodeHash, core.RecoveryCodeCount)
		selectors := make([]string, core.RecoveryCodesRequired)
		for index := range hashes {
			hashes[index] = core.RecoveryCodeHash{SelectorHash: fmt.Sprintf("%064x", index+1), PasswordHash: "recovery-hash", CreatedAt: now}
			if index < len(selectors) {
				selectors[index] = hashes[index].SelectorHash
			}
		}
		if err := db.ReplaceRecoveryCodes(username, hashes); err != nil {
			return err
		}
		if _, err := db.ResetPasswordWithRecoveryCodes(username, selectors, "recovered-password", now+300001); err != nil {
			return err
		}
		mfa, err = db.GetMFAState(username)
		if err != nil || mfa.Enabled() {
			return errorsOrMissing(err, "offline recovery clears second factors")
		}
		stored, err := db.GetSession(sessionID)
		if err != nil || stored != nil {
			return errorsOrMissing(err, "offline recovery revokes sessions")
		}
		return nil
	}); err != nil {
		return results, err
	}
	if err := run("security email verification", func() error {
		username, sessionID := "email_"+suffix, "email-session-"+suffix
		if err := db.SaveToken(&core.AccessToken{Name: username, EncryptedSecret: "password", Permissions: []string{"base"}}); err != nil {
			return err
		}
		oldEmail, newEmail := username+"@example.test", "new-"+username+"@example.test"
		if _, err := db.UpdateAccountEmail(username, oldEmail, now); err != nil {
			return err
		}
		session := &core.Session{PublicID: sessionID, Username: username, CreatedAt: now}
		session.LastActive.Store(now)
		if err := db.SaveSession(session, sessionID); err != nil {
			return err
		}
		cfg := mail.DefaultConfig()
		if err := cfg.EnsureKey(); err != nil {
			return err
		}
		cfg.ManualRate.Limit = 100
		hash := strings.Repeat("a", 64)
		job := &mail.Job{ID: "email-job-" + suffix, AccountID: "test", Scene: "email_verify", TicketHash: hash,
			CreatedAt: now, ExpiresAt: now + 600000,
			Message: mail.Message{ID: "email-job-" + suffix, To: newEmail, Subject: "Verify email", Text: "Code", CreatedAt: now}}
		if err := db.QueueAccountEmailChange(username, sessionID, job, hash, cfg.EncryptionKey, "192.0.2.88", cfg.ManualRate); err != nil {
			return err
		}
		security, err := db.GetAccountSecurity(username)
		if err != nil || security.Email != oldEmail {
			return errorsOrMissing(err, "unverified email preserves the current address")
		}
		if _, err := db.ConfirmAccountEmailChange(username, sessionID, newEmail, strings.Repeat("b", 64), now+1); !errors.Is(err, core.ErrEmailCodeInvalid) {
			return errorsOrMissing(err, "email code denial")
		}
		security, err = db.ConfirmAccountEmailChange(username, sessionID, newEmail, hash, now+2)
		if err != nil || security.Email != newEmail {
			return errorsOrMissing(err, "confirmed email change")
		}
		if _, err := db.ConfirmAccountEmailChange(username, sessionID, newEmail, hash, now+3); !errors.Is(err, core.ErrEmailCodeInvalid) {
			return errorsOrMissing(err, "email code replay denial")
		}
		account, err := db.GetMFAState(username)
		if err != nil {
			return err
		}
		if _, err = db.UpdateAccountEmailFromSession(username, sessionID, oldEmail, account.Snapshot, now+4); err != nil {
			return err
		}
		if _, err = db.UpdateAccountEmailFromSession(username, sessionID, newEmail, account.Snapshot, now+4); !errors.Is(err, core.ErrEmailCodeInvalid) {
			return errorsOrMissing(err, "stale external email proof denial")
		}
		return nil
	}); err != nil {
		return results, err
	}
	if err := run("account registration", func() error {
		cfg := config.DefaultRegistrationConfig()
		cfg.Enabled = true
		mailCfg := mail.DefaultConfig()
		if err := mailCfg.EnsureKey(); err != nil {
			return err
		}
		password, err := bcrypt.GenerateFromPassword([]byte("DriverCheckPassword!"), bcrypt.MinCost)
		if err != nil {
			return err
		}
		hash := func(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value+suffix))) }
		request := core.AccountRegistration{Username: "reg_" + suffix, Email: "reg_" + suffix + "@example.com", PasswordHash: string(password),
			IDHash: hash("registration"), CodeHash: hash("registration-code"), IPHash: hash("registration-ip"), RequireEmail: true}
		pending := &core.PendingRegistration{IDHash: request.IDHash, IPHash: request.IPHash, Email: request.Email, CodeHash: request.CodeHash,
			CreatedAt: now, ExpiresAt: now + 600000, CooldownUntil: now + 600000}
		job := &mail.Job{ID: "registration-" + suffix, AccountID: "driver-account", Scene: "registration_verify", CreatedAt: now,
			ExpiresAt: pending.ExpiresAt, TicketHash: hash("ticket"), Message: mail.Message{ID: "registration-" + suffix, To: request.Email, Subject: "Verify", Text: "Code", CreatedAt: now}}
		if err := db.BeginRegistration(pending, job, mailCfg.EncryptionKey, "192.0.2.254", mailCfg.ManualRate, cfg); err != nil {
			return err
		}
		wrong := request
		wrong.CodeHash = hash("wrong")
		if _, err := db.RegisterAccount(wrong, cfg, now+1); !errors.Is(err, core.ErrRegistrationInvalid) {
			return errorsOrMissing(err, "registration code denial")
		}
		if _, err := db.RegisterAccount(request, cfg, now+2); err != nil {
			return err
		}
		account, err := db.GetTokenByEmail(request.Email)
		if err != nil || account == nil || account.Name != request.Username {
			return errorsOrMissing(err, "registered email and account")
		}
		if _, err := db.GetPendingRegistration(request.IDHash, now+3); !errors.Is(err, core.ErrRegistrationInvalid) {
			return errorsOrMissing(err, "registration challenge consumption")
		}
		if _, err := db.RegisterAccount(request, cfg, now+3); !errors.Is(err, core.ErrRegistrationRateLimited) {
			return errorsOrMissing(err, "registration IP limit")
		}
		if err := db.RetireAccount(request.Username, now+3); err != nil {
			return err
		}
		if err := db.CheckRegistrationIP(request.IPHash, cfg, now+4); !errors.Is(err, core.ErrRegistrationRateLimited) {
			return errorsOrMissing(err, "retirement retains registration allowance")
		}
		pending.IDHash, pending.IPHash, pending.CodeHash = hash("provider"), hash("provider-ip"), ""
		pending.ProviderKey = "github:42000001"
		pending.ProfileJSON = `{"github_id":42000001,"github_login":"external","principals":[{"type":"user","github_id":42000001,"login":"external"}]}`
		pending.CooldownUntil = pending.ExpiresAt + cfg.ProviderCooldown.Duration().Milliseconds()
		if err := db.BeginRegistration(pending, nil, "", "", mailCfg.ManualRate, cfg); err != nil {
			return err
		}
		if err := db.CleanRegistrations(pending.ExpiresAt); err != nil {
			return err
		}
		var email, profile string
		if err := db.QueryRow(`SELECT email, profile_json FROM account_registrations WHERE id_hash = ?`, pending.IDHash).Scan(&email, &profile); err != nil {
			return err
		}
		if email != "" || profile != "" {
			return errorsOrMissing(nil, "expired provider profile cleanup")
		}
		next := *pending
		next.IDHash = hash("provider-retry")
		next.CreatedAt = pending.ExpiresAt
		next.ExpiresAt = next.CreatedAt + 600000
		next.CooldownUntil = next.ExpiresAt + cfg.ProviderCooldown.Duration().Milliseconds()
		if err := db.BeginRegistration(&next, nil, "", "", mailCfg.ManualRate, cfg); !errors.Is(err, core.ErrRegistrationCooldown) {
			return errorsOrMissing(err, "provider cooldown")
		}
		next.CreatedAt = pending.CooldownUntil
		next.ExpiresAt = next.CreatedAt + 600000
		next.CooldownUntil = next.ExpiresAt + cfg.ProviderCooldown.Duration().Milliseconds()
		next.Email = "provider_" + suffix + "@example.com"
		if err := db.BeginRegistration(&next, nil, "", "", mailCfg.ManualRate, cfg); err != nil {
			return err
		}
		request.Provider, request.Username, request.Email = "github", "ext_"+suffix, next.Email
		request.IDHash, request.IPHash, request.CodeHash = next.IDHash, next.IPHash, ""
		if _, err := db.RegisterAccount(request, cfg, next.CreatedAt+1); err != nil {
			return err
		}
		identity, err := db.GetGitHubIdentityByProviderID(42000001)
		if err != nil || identity == nil || identity.Username != request.Username {
			return errorsOrMissing(err, "registered provider identity")
		}
		return nil
	}); err != nil {
		return results, err
	}
	if err := run("OAuth account lifecycle", func() error {
		cfg := config.DefaultRegistrationConfig()
		cfg.Enabled = true
		mailCfg := mail.DefaultConfig()
		if err := mailCfg.EnsureKey(); err != nil {
			return err
		}
		hash := func(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value+suffix))) }
		identity := core.OAuthIdentity{ProviderID: "gitlab", Subject: "subject-" + suffix, Authority: hash("authority"), Login: "external",
			Namespaces: []string{"external", "owned-group"}}
		profile, err := json.Marshal(core.RegistrationProfile{OAuth: &identity})
		if err != nil {
			return err
		}
		pending := &core.PendingRegistration{IDHash: hash("oauth"), IPHash: hash("oauth-ip"),
			ProviderKey: identity.ProviderID + ":" + identity.Key(), ProfileJSON: string(profile), CreatedAt: now,
			ExpiresAt: now + 600000, CooldownUntil: now + 600000 + cfg.ProviderCooldown.Duration().Milliseconds()}
		if err := db.BeginRegistration(pending, nil, "", "", mailCfg.ManualRate, cfg); err != nil {
			return err
		}
		password, err := bcrypt.GenerateFromPassword([]byte("OAuthPassword2026!"), bcrypt.MinCost)
		if err != nil {
			return err
		}
		request := core.AccountRegistration{Provider: identity.ProviderID, Username: "oauth_" + suffix, PasswordHash: string(password),
			IDHash: pending.IDHash, IPHash: pending.IPHash, Email: "oauth_" + suffix + "@example.test", CodeHash: hash("oauth-code")}
		if _, err := db.RegisterAccount(request, cfg, now+1); !errors.Is(err, core.ErrRegistrationInvalid) {
			return errorsOrMissing(err, "unverified OAuth email denial")
		}
		job := &mail.Job{ID: "oauth-" + suffix, AccountID: "driver-account", Scene: "registration_verify", TicketHash: hash("oauth-ticket"),
			CreatedAt: now + 2, ExpiresAt: pending.ExpiresAt, Message: mail.Message{ID: "oauth-" + suffix, To: request.Email, Subject: "Verify", Text: "Code", CreatedAt: now + 2}}
		if err := db.QueueProviderRegistrationEmail(pending.IDHash, pending.IPHash, request.CodeHash, job, mailCfg.EncryptionKey, "192.0.2.201", mailCfg.ManualRate, cfg); err != nil {
			return err
		}
		if _, err := db.RegisterAccount(request, cfg, now+3); err != nil {
			return err
		}
		linked, err := db.GetOAuthIdentity(identity)
		if err != nil || linked == nil || linked.Username != request.Username || len(linked.Namespaces) != 2 {
			return errorsOrMissing(err, "OAuth identity and namespace persistence")
		}
		identity.Namespaces = []string{"external"}
		if err := db.RefreshOAuthIdentity(linked.UserID, identity, now+4); err != nil {
			return err
		}
		identities, err := db.GetOAuthIdentities(request.Username)
		if err != nil || len(identities) != 1 || len(identities[0].Namespaces) != 1 || identities[0].AuthorizedAt != now+4 {
			return errorsOrMissing(err, "OAuth proof refresh")
		}
		sessionID := "oauth-session-" + suffix
		session := &core.Session{PublicID: sessionID, Username: request.Username, CreatedAt: now, LoginMethod: "oauth:gitlab"}
		session.LastActive.Store(now)
		if err := db.SaveSession(session, sessionID); err != nil {
			return err
		}
		security, err := db.SetPasswordLoginEnabled(request.Username, false, now+5)
		if err != nil || security.OAuthIdentityCount != 1 || !security.CanDisablePasswordLogin {
			return errorsOrMissing(err, "OAuth primary login alternative")
		}
		if err := db.DeleteOAuthIdentity(request.Username, sessionID, identity.ProviderID, now+6); !errors.Is(err, core.ErrLastLoginMethod) {
			return errorsOrMissing(err, "last OAuth login preservation")
		}
		if err := db.RetireAccount(request.Username, now+7); err != nil {
			return err
		}
		identities, err = db.GetOAuthIdentities(request.Username)
		if err != nil || len(identities) != 0 {
			return errorsOrMissing(err, "OAuth retirement release")
		}
		if err := db.RefreshOAuthIdentity(linked.UserID, identity, now+8); !errors.Is(err, core.ErrAccountDeleted) {
			return errorsOrMissing(err, "retired OAuth refresh denial")
		}
		receiver, receiverSession := "oauthnew_"+suffix, "oauth-new-session-"+suffix
		if err := db.SaveToken(&core.AccessToken{Name: receiver, EncryptedSecret: string(password), Permissions: []string{"base"}}); err != nil {
			return err
		}
		session.PublicID, session.Username = receiverSession, receiver
		if err := db.SaveSession(session, receiverSession); err != nil {
			return err
		}
		mfa, err := db.GetMFAState(receiver)
		if err != nil {
			return err
		}
		if err := db.LinkOAuthIdentity(receiver, receiverSession, mfa.Snapshot, identity, now+9); err != nil {
			return err
		}
		if _, err := db.UpdateAccountEmail(receiver, request.Email, now+10); !errors.Is(err, core.ErrEmailAlreadyExists) {
			return errorsOrMissing(err, "OAuth release preserves retired email hold")
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
