/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

// Package review exposes independent, single-decision review workflows.
package review

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/cargo"
	"renop/internal/service/docker"
	"renop/internal/service/maven"
	"renop/internal/service/npm"
	"renop/internal/service/repositorygate"
	"renop/internal/service/storage"
	"renop/internal/utils"
)

const maxReviewRequestBytes = 16 << 10

type decisionRequest struct {
	Decision   string `json:"decision"`
	ReasonCode string `json:"reason_code"`
	Reason     string `json:"reason"`
}

func reviewError(c fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	code := "review_failed"
	switch {
	case errors.Is(err, fiber.ErrBadRequest):
		status, code = fiber.StatusBadRequest, "invalid_request"
	case errors.Is(err, fiber.ErrUnauthorized):
		status, code = fiber.StatusUnauthorized, "authentication_required"
	case errors.Is(err, core.ErrReviewTaskNotFound):
		status, code = fiber.StatusNotFound, "review_not_found"
	case errors.Is(err, core.ErrReviewTaskExists):
		status, code = fiber.StatusConflict, "review_exists"
	case errors.Is(err, core.ErrReviewTaskConflict):
		status, code = fiber.StatusConflict, "review_decided"
	case errors.Is(err, core.ErrReviewPublicationActive):
		status, code = fiber.StatusConflict, "publication_active"
	case errors.Is(err, core.ErrReviewPublicationSealed):
		status, code = fiber.StatusConflict, "publication_sealed"
	case errors.Is(err, core.ErrReviewFileNotFound):
		status, code = fiber.StatusNotFound, "review_file_not_found"
	case errors.Is(err, core.ErrReviewFileLimit):
		status, code = fiber.StatusTooManyRequests, "review_limit"
	case errors.Is(err, core.ErrReviewInvalidRequest):
		status, code = fiber.StatusBadRequest, "invalid_request"
	case errors.Is(err, core.ErrReviewPermissionDenied), errors.Is(err, core.ErrSuperTeamBindingPermission):
		status, code = fiber.StatusForbidden, "review_permission"
	case errors.Is(err, core.ErrReviewTransferRestricted):
		status, code = fiber.StatusConflict, "transfer_restricted"
	case errors.Is(err, core.ErrReviewResourceConflict):
		status, code = fiber.StatusConflict, "resource_changed"
	case errors.Is(err, core.ErrSuperTeamBindingMismatch):
		status, code = fiber.StatusBadRequest, "super_team_mismatch"
	case errors.Is(err, docker.ErrUpstreamImageProbeUnavailable):
		status, code = fiber.StatusServiceUnavailable, "service_unavailable"
	case errors.Is(err, core.ErrDatabaseUnavailable):
		status, code = fiber.StatusServiceUnavailable, "service_unavailable"
	}
	c.Set("X-Renop-Error-Code", code)
	return c.Status(status).SendString(code)
}

func currentUser(c fiber.Ctx) (string, bool, error) {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return "", false, fiber.ErrUnauthorized
	}
	if auth.CurrentCredentialKind(c) != "session" {
		return "", false, core.ErrReviewPermissionDenied
	}
	return user.Username, user.IsManager(), nil
}

func normalizeTransferRequest(request *core.SuperTeamTransferRequest) bool {
	if request == nil {
		return false
	}
	request.ResourceType = strings.ToLower(strings.TrimSpace(request.ResourceType))
	request.Repository = strings.ToLower(strings.TrimSpace(request.Repository))
	request.ResourceKey = strings.TrimSpace(request.ResourceKey)
	switch request.ResourceType {
	case core.ReviewResourceDockerImage:
		value, valid := docker.NormalizeImageName(request.ResourceKey)
		request.ResourceKey = value
		return valid && utils.IsValidRepositoryName(request.Repository)
	case core.ReviewResourceNPMPackage:
		value, valid := npm.NormalizePackageName(request.ResourceKey)
		request.ResourceKey = value
		return valid && utils.IsValidRepositoryName(request.Repository)
	case core.ReviewResourceCargoPackage:
		value, valid := cargo.NormalizeCrateName(request.ResourceKey)
		request.ResourceKey = value
		return valid && utils.IsValidRepositoryName(request.Repository)
	case core.ReviewResourceMavenDomain:
		value, err := maven.NormalizeDomain(request.ResourceKey)
		request.ResourceKey = value
		request.Repository = ""
		return err == nil
	case core.ReviewResourceMavenArtifact:
		separator := strings.LastIndexByte(request.ResourceKey, ':')
		if separator <= 0 || separator == len(request.ResourceKey)-1 ||
			!utils.IsValidRepositoryName(request.Repository) {
			return false
		}
		groupID, err := maven.NormalizeDomain(request.ResourceKey[:separator])
		artifactID := strings.TrimSpace(request.ResourceKey[separator+1:])
		if err != nil || artifactID == "" || len(artifactID) > 255 || strings.ContainsAny(artifactID, "\x00\r\n/:\\") {
			return false
		}
		request.ResourceKey = groupID + ":" + artifactID
		return true
	default:
		return false
	}
}

func validReviewResourceType(value string) bool {
	switch value {
	case core.ReviewResourceDockerImage, core.ReviewResourceNPMPackage, core.ReviewResourceCargoPackage,
		core.ReviewResourceMavenArtifact, core.ReviewResourceMavenDomain:
		return true
	default:
		return false
	}
}

func validReviewStatus(value string) bool {
	switch value {
	case "all", core.ReviewStatusPending, core.ReviewStatusApproved,
		core.ReviewStatusRejected, core.ReviewStatusCancelled:
		return true
	default:
		return false
	}
}

func logReviewAudit(c fiber.Ctx, state *core.AppState, action, details string) {
	username, operator, method, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, AuthMethod: method, SessionID: sessionID,
		IP: ip, Action: action, Details: details, CreatedAt: time.Now().UnixMilli(),
	})
}

func createTransfer(c fiber.Ctx, state *core.AppState) error {
	username, administrator, err := currentUser(c)
	if err != nil {
		return reviewError(c, err)
	}
	var request core.SuperTeamTransferRequest
	if err := utils.ReadJSONLimited(c, &request, maxReviewRequestBytes); err != nil ||
		!normalizeTransferRequest(&request) {
		return reviewError(c, fiber.ErrBadRequest)
	}
	if !administrator && request.Repository != "" {
		administrator = auth.GetUser(c).CheckUpdatePermission(request.Repository)
	}
	task, err := state.GetDB().CreateSuperTeamTransferReview(
		request, username, administrator, time.Now().UnixMilli())
	if err != nil {
		return reviewError(c, err)
	}
	logReviewAudit(c, state, audit.ActionReviewRequest,
		fmt.Sprintf("Type: %s, repository: %s, resource: %s, team: %s",
			task.ResourceType, task.Repository, task.ResourceName, task.ReviewTeamPrefix))
	c.Set(fiber.HeaderLocation, "/api/reviews/"+task.ID)
	return c.Status(fiber.StatusCreated).JSON(task)
}

func listTasks(c fiber.Ctx, state *core.AppState) error {
	username, administrator, err := currentUser(c)
	if err != nil {
		return reviewError(c, err)
	}
	limit, err := strconv.Atoi(c.Query("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		return reviewError(c, fiber.ErrBadRequest)
	}
	offset, err := strconv.Atoi(c.Query("offset", "0"))
	if err != nil || offset < 0 {
		return reviewError(c, fiber.ErrBadRequest)
	}
	view := strings.ToLower(strings.TrimSpace(c.Query("view", "reviewer")))
	if view != "reviewer" && view != "requested" {
		return reviewError(c, fiber.ErrBadRequest)
	}
	status := strings.ToLower(strings.TrimSpace(c.Query("status", core.ReviewStatusPending)))
	if !validReviewStatus(status) {
		return reviewError(c, fiber.ErrBadRequest)
	}
	types := make([]string, 0, 5)
	for _, value := range strings.Split(c.Query("types"), ",") {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			if !validReviewResourceType(value) {
				return reviewError(c, fiber.ErrBadRequest)
			}
			types = append(types, value)
		}
	}
	moderatedRepositories := make([]string, 0)
	moderateAll := false
	if !administrator {
		user := auth.GetUser(c)
		if user != nil {
			moderateAll, moderatedRepositories = user.ModerationScope()
		}
	}
	tasks, total, err := state.GetDB().ListReviewTasks(core.ReviewTaskListOptions{
		Username: username, RequestedView: view == "requested", Administrator: administrator, ResourceTypes: types,
		ModerateAll: moderateAll, ModeratedRepositories: moderatedRepositories,
		Status: status, Limit: limit, Offset: offset,
	})
	if err != nil {
		return reviewError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{
		"tasks": tasks, "total": total, "limit": limit, "offset": offset, "view": view,
	})
}

func publicationDecisionReason(request decisionRequest) (string, bool) {
	code := strings.ToLower(strings.TrimSpace(request.ReasonCode))
	switch code {
	case "malware", "policy_violation", "invalid_metadata", "copyright", "quality":
		return "preset:" + code, true
	case "custom":
		reason, valid := core.NormalizeSuperTeamText(request.Reason, 505, false)
		if !valid || reason == "" {
			return "", false
		}
		return "custom:" + reason, true
	default:
		return "", false
	}
}

func canInspectPublicationTask(c fiber.Ctx, state *core.AppState, task *core.ReviewTask) (bool, error) {
	user := auth.GetUser(c)
	if user == nil || task == nil || state == nil || state.GetDB() == nil {
		return false, nil
	}
	if user.IsManager() || user.CheckModeratePermission(task.Repository) {
		return true, nil
	}
	profile, err := state.GetDB().GetUserProfile(user.Username)
	if err != nil {
		return false, err
	}
	return profile.UserID == task.RequestedByID, nil
}

func reviewFiles(c fiber.Ctx, state *core.AppState) error {
	if _, _, err := currentUser(c); err != nil {
		return reviewError(c, err)
	}
	task, err := state.GetDB().GetReviewTask(c.Params("id"))
	if err != nil {
		return reviewError(c, err)
	}
	allowed, err := canInspectPublicationTask(c, state, task)
	if err != nil {
		return reviewError(c, err)
	}
	if task.Kind != core.ReviewKindPublication || !allowed {
		return reviewError(c, core.ErrReviewPermissionDenied)
	}
	files, err := state.GetDB().ListReviewTaskFiles(task.ID)
	if err != nil {
		return reviewError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"task_id": task.ID, "files": files, "total": len(files)})
}

func downloadReviewFile(c fiber.Ctx, state *core.AppState) error {
	if _, _, err := currentUser(c); err != nil {
		return reviewError(c, err)
	}
	task, err := state.GetDB().GetReviewTask(c.Params("id"))
	if err != nil {
		return reviewError(c, err)
	}
	allowed, err := canInspectPublicationTask(c, state, task)
	if err != nil {
		return reviewError(c, err)
	}
	if task.Kind != core.ReviewKindPublication || task.Status != core.ReviewStatusPending || !allowed {
		return reviewError(c, core.ErrReviewPermissionDenied)
	}
	files, err := state.GetDB().ListReviewTaskFiles(task.ID)
	if err != nil {
		return reviewError(c, err)
	}
	fileID := strings.TrimSpace(c.Params("file_id"))
	for _, file := range files {
		if file != nil && file.ID == fileID {
			c.Set(fiber.HeaderCacheControl, "private, no-store")
			if task.ResourceType == core.ReviewResourceNPMPackage &&
				task.ResourceVersion == core.ReviewVersionPackageCreation {
				if err := npm.ServePackageCreationReview(c, state, task, file); err != nil {
					return reviewError(c, err)
				}
				return nil
			}
			if task.ResourceType == core.ReviewResourceDockerImage {
				if err := docker.ServePublicationReviewManifest(c, state, task, file); err != nil {
					return reviewError(c, err)
				}
				return nil
			}
			if err := storage.ServePublicationReviewFile(c, state, file); err != nil {
				return reviewError(c, err)
			}
			return nil
		}
	}
	return reviewError(c, core.ErrReviewFileNotFound)
}

func decidePublicationTask(c fiber.Ctx, state *core.AppState, username string,
	task *core.ReviewTask, request decisionRequest,
) (*core.ReviewTask, error) {
	user := auth.GetUser(c)
	if user == nil || !user.CheckModeratePermission(task.Repository) {
		return nil, core.ErrReviewPermissionDenied
	}
	decision := strings.ToLower(strings.TrimSpace(request.Decision))
	reason := ""
	if decision == core.ReviewStatusRejected {
		var valid bool
		reason, valid = publicationDecisionReason(request)
		if !valid {
			return nil, core.ErrReviewInvalidRequest
		}
	}
	release := repositorygate.AcquireMutation(task.Repository)
	defer release()
	current, err := state.GetDB().GetReviewTask(task.ID)
	if err != nil {
		return nil, err
	}
	if current.Status != core.ReviewStatusPending {
		return nil, core.ErrReviewTaskConflict
	}
	now := time.Now().UnixMilli()
	if now-current.UpdatedAt < core.PublicationReviewSettleMillis {
		return nil, core.ErrReviewPublicationActive
	}
	files, err := state.GetDB().ListReviewTaskFiles(current.ID)
	if err != nil {
		return nil, err
	}
	if decision == core.ReviewStatusRejected {
		if err := storage.DeletePublicationReviewFiles(state, files); err != nil {
			return nil, err
		}
		return state.GetDB().DecideReviewTask(current.ID, username, decision, reason, now)
	}
	if decision != core.ReviewStatusApproved {
		return nil, core.ErrReviewInvalidRequest
	}
	var rollback func() error
	switch current.ResourceType {
	case core.ReviewResourceMavenArtifact:
		if err := maven.ApprovePublicationReview(state, current); err != nil {
			return nil, err
		}
		rollback = func() error { return maven.RemoveApprovedPublicationMetadata(state, current) }
	case core.ReviewResourceNPMPackage:
		if current.ResourceVersion == core.ReviewVersionPackageCreation {
			decided, err := npm.ApprovePackageCreationReview(state, current, username, now)
			if err != nil {
				return nil, err
			}
			if err := storage.UnblockPublicationReviewFiles(state, files); err != nil {
				return nil, err
			}
			return decided, nil
		}
		previousDetails, err := state.GetDB().GetNPMPackageDetails(
			current.Repository, current.ResourceKey, current.RequestedBy)
		if err != nil || previousDetails == nil || previousDetails.Package == nil {
			return nil, errors.Join(core.ErrReviewResourceConflict, err)
		}
		previousTags := make(map[string]string, len(previousDetails.DistTags))
		for tag, version := range previousDetails.DistTags {
			previousTags[tag] = version
		}
		if err := npm.ApprovePublicationReview(state, current); err != nil {
			return nil, err
		}
		rollback = func() error {
			return npm.RemoveApprovedPublicationMetadata(state, current, previousDetails.Package, previousTags)
		}
	case core.ReviewResourceCargoPackage:
		cargoRollback, err := cargo.ApprovePublicationReview(state, current, storage.NewPackageStore())
		if err != nil {
			return nil, err
		}
		rollback = cargoRollback
	case core.ReviewResourceDockerImage:
		if current.ResourceVersion == core.ReviewVersionPackageCreation {
			decided, err := docker.ApproveImageCreationReview(
				c.Context(), state, current, username, now)
			if err != nil {
				return nil, err
			}
			if err := storage.UnblockPublicationReviewFiles(state, files); err != nil {
				return nil, err
			}
			return decided, nil
		}
		cfg := state.Inner.Config.Load()
		if cfg == nil {
			return nil, core.ErrDatabaseUnavailable
		}
		decided, err := docker.ApprovePublicationReview(
			state, current, storage.NewDockerStore(cfg.StoragePath), username, now)
		if err != nil {
			return nil, err
		}
		if err := storage.UnblockPublicationReviewFiles(state, files); err != nil {
			return nil, err
		}
		return decided, nil
	default:
		return nil, core.ErrReviewInvalidRequest
	}
	decided, err := state.GetDB().DecideReviewTask(current.ID, username, decision, "", now)
	if err != nil {
		return nil, errors.Join(err, rollback())
	}
	if err := storage.UnblockPublicationReviewFiles(state, files); err != nil {
		return nil, err
	}
	return decided, nil
}

func decideTask(c fiber.Ctx, state *core.AppState) error {
	username, _, err := currentUser(c)
	if err != nil {
		return reviewError(c, err)
	}
	var request decisionRequest
	if err := utils.ReadJSONLimited(c, &request, maxReviewRequestBytes); err != nil {
		return reviewError(c, fiber.ErrBadRequest)
	}
	existing, err := state.GetDB().GetReviewTask(c.Params("id"))
	if err != nil {
		return reviewError(c, err)
	}
	var task *core.ReviewTask
	if existing.Kind == core.ReviewKindPublication {
		task, err = decidePublicationTask(c, state, username, existing, request)
	} else {
		task, err = state.GetDB().DecideReviewTask(
			existing.ID, username, request.Decision, request.Reason, time.Now().UnixMilli())
	}
	if err != nil {
		if task != nil && errors.Is(err, core.ErrReviewResourceConflict) {
			c.Set("X-Renop-Error-Code", "resource_changed")
			return c.Status(fiber.StatusConflict).JSON(task)
		}
		return reviewError(c, err)
	}
	logReviewAudit(c, state, audit.ActionReviewDecision,
		fmt.Sprintf("Review: %s, decision: %s, type: %s", task.ID, task.Status, task.ResourceType))
	return c.JSON(task)
}

func cancelTask(c fiber.Ctx, state *core.AppState) error {
	username, _, err := currentUser(c)
	if err != nil {
		return reviewError(c, err)
	}
	task, err := state.GetDB().CancelReviewTask(c.Params("id"), username, time.Now().UnixMilli())
	if err != nil {
		return reviewError(c, err)
	}
	logReviewAudit(c, state, audit.ActionReviewCancel, "Review: "+task.ID)
	return c.JSON(task)
}

// SetupRoutes registers session-only review and transfer APIs.
func SetupRoutes(router fiber.Router, state *core.AppState) {
	base := router.Group("/reviews")
	base.Get("", func(c fiber.Ctx) error { return listTasks(c, state) })
	base.Post("/super-team-transfers", func(c fiber.Ctx) error { return createTransfer(c, state) })
	base.Get("/:id/files", func(c fiber.Ctx) error { return reviewFiles(c, state) })
	base.Get("/:id/files/:file_id", func(c fiber.Ctx) error { return downloadReviewFile(c, state) })
	base.Post("/:id/decision", func(c fiber.Ctx) error { return decideTask(c, state) })
	base.Delete("/:id", func(c fiber.Ctx) error { return cancelTask(c, state) })
}
