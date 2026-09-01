/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package api

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/docker"
	"renop/internal/utils"
)

const (
	dockerAPIErrorCodeHeader = "X-Renop-Error-Code"
	maxDockerReadmeBytes     = 512 << 10
)

func dockerAPIError(c fiber.Ctx, status int, code, message string) error {
	c.Set(dockerAPIErrorCodeHeader, code)
	return c.Status(status).SendString(message)
}

func ensureDockerImageMutable(state *core.AppState, repository, image string) error {
	if state == nil || state.GetDB() == nil {
		return core.ErrDatabaseUnavailable
	}
	return state.GetDB().EnsurePackageMutable(config.RepositoryFormatDocker, repository, image)
}

func dockerImageMutationError(c fiber.Ctx, err error) error {
	if errors.Is(err, core.ErrPackageDeprecated) {
		return dockerAPIError(c, fiber.StatusConflict, "package_deprecated",
			"Docker image is permanently deprecated and pull-only")
	}
	if errors.Is(err, core.ErrDatabaseUnavailable) {
		return dockerAPIError(c, fiber.StatusServiceUnavailable, "service_unavailable", "Docker package state is unavailable")
	}
	return dockerAPIError(c, fiber.StatusInternalServerError, "internal_error", "Failed to inspect Docker image state")
}

func withDockerAPIErrorCode(handler fiber.Handler) fiber.Handler {
	return func(c fiber.Ctx) error {
		err := handler(c)
		if (err != nil || c.Response().StatusCode() >= fiber.StatusBadRequest) &&
			len(c.Response().Header.Peek(dockerAPIErrorCodeHeader)) == 0 {
			c.Set(dockerAPIErrorCodeHeader, "request_failed")
		}
		return err
	}
}

func logDockerAudit(c fiber.Ctx, state *core.AppState, action, details string) {
	username, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username:   username,
		Operator:   operator,
		AuthMethod: authMethod,
		SessionID:  sessionID,
		IP:         ip,
		Action:     action,
		Details:    details,
		CreatedAt:  time.Now().UnixMilli(),
	})
}

// ListDockerImagesAPI handles GET /api/docker/repositories/:repo_name/images
func ListDockerImagesAPI(c fiber.Ctx, state *core.AppState) error {
	if c.Query("image") != "" {
		return GetDockerImageDetailsAPI(c, state)
	}
	repoName := c.Params("repo_name")
	if !utils.IsValidRepositoryName(repoName) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid repository name")
	}

	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}

	user := auth.GetUser(c)
	if !docker.CanReadDocker(state, user, repo, repoName) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}

	images, err := db.ListDockerImages(repoName, "", 100)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to list images")
	}

	images, err = docker.FilterReadableDockerImages(state, user, repo, images)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}

	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"repository": repoName,
		"images":     images,
	})
}

type createDockerImageRequest struct {
	Image           string `json:"image"`
	SuperTeamPrefix string `json:"super_team_prefix"`
	Private         bool   `json:"private"`
}

// CreateDockerImageAPI handles POST /api/docker/repositories/:repo_name/images.
func CreateDockerImageAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	if !utils.IsValidRepositoryName(repoName) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid repository name")
	}
	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return c.Status(fiber.StatusUnauthorized).SendString("Authentication required")
	}
	if !user.IsManager() && !user.CheckUpdatePermission(repoName) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	var request createDockerImageRequest
	if err := utils.ReadJSONLimited(c, &request, 2048); err != nil {
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) {
			return err
		}
		return c.Status(fiber.StatusBadRequest).SendString("Invalid request body")
	}
	imageName, valid := docker.NormalizeImageName(request.Image)
	if !valid {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid Docker image name")
	}
	teamPrefix := strings.ToLower(strings.TrimSpace(request.SuperTeamPrefix))
	if teamPrefix != "" {
		var teamPrefixValid bool
		teamPrefix, teamPrefixValid = core.NormalizeSuperTeamPrefix(teamPrefix)
		if !teamPrefixValid {
			return dockerAPIError(c, fiber.StatusBadRequest, "super_team_mismatch", "Invalid global team prefix")
		}
	}
	requiredPrefix, namespaced := core.DockerImageSuperTeamPrefix(imageName)
	if strings.Contains(imageName, "/") && !namespaced {
		return dockerAPIError(c, fiber.StatusBadRequest, "super_team_mismatch", "Invalid global team namespace")
	}
	if namespaced && teamPrefix == "" {
		return dockerAPIError(c, fiber.StatusBadRequest, "super_team_required", "Namespaced images require a global team")
	}
	if namespaced && teamPrefix != requiredPrefix {
		return dockerAPIError(c, fiber.StatusBadRequest, "super_team_mismatch", "Image namespace must match the global team")
	}
	if teamPrefix != "" && !auth.CurrentCredentialHasScopeTarget(
		c, core.APITokenScopeTeamManage, "global/"+teamPrefix) {
		c.Set("X-Renop-Required-Scope", core.APITokenScopeTeamManage)
		return dockerAPIError(c, fiber.StatusForbidden, "super_team_permission", "API token cannot use this global team")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	reviewTeamPrefix := ""
	if teamPrefix != "" {
		teamRole, roleErr := db.GetSuperTeamRole(teamPrefix, user.Username)
		if roleErr != nil {
			if errors.Is(roleErr, core.ErrSuperTeamPermissionDenied) {
				return dockerAPIError(c, fiber.StatusForbidden, "super_team_permission",
					"T2 or higher global team permission is required")
			}
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to inspect global team permission")
		}
		if teamRole < core.SuperTeamRoleWrite {
			return dockerAPIError(c, fiber.StatusForbidden, "super_team_permission",
				"T2 or higher global team permission is required")
		}
		if teamRole == core.SuperTeamRoleWrite {
			reviewTeamPrefix = teamPrefix
		}
	}
	currentConfig := state.Inner.Config.Load()
	if currentConfig == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Configuration unavailable")
	}
	repo = currentConfig.Maven.Repositories[repoName]
	if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}
	imageExists, _, _, _, _, err := db.GetDockerImageAccess(repoName, imageName, "")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to inspect Docker image name")
	}
	if imageExists {
		return c.Status(fiber.StatusConflict).SendString("Docker image name is already in use")
	}
	if len(repo.Mirrors) > 0 {
		probeCtx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
		upstreamExists, probeErr := docker.UpstreamImageExists(probeCtx, state, repo, imageName)
		cancel()
		if probeErr != nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("Failed to verify Docker image name against upstream mirrors")
		}
		if upstreamExists {
			return c.Status(fiber.StatusConflict).SendString("Docker image name is already in use by an upstream mirror")
		}
	}
	createdAt := time.Now().UnixMilli()
	if repo.PublicationReviewPolicy() != config.PublicationReviewOff || reviewTeamPrefix != "" {
		review, reviewErr := docker.QueueImageCreationReview(
			state, repo, imageName, teamPrefix, reviewTeamPrefix, user.Username, request.Private, createdAt)
		if errors.Is(reviewErr, core.ErrReviewPermissionDenied) {
			return dockerAPIError(c, fiber.StatusConflict, "review_pending",
				"Another account already requested this Docker image name")
		}
		if reviewErr != nil || review == nil || !review.Pending {
			return dockerAPIError(c, fiber.StatusInternalServerError, "review_unavailable",
				"Failed to create Docker image review")
		}
		c.Set("X-RenoP-Review-ID", review.TaskID)
		logDockerAudit(c, state, audit.ActionUploadQueuedReview,
			fmt.Sprintf("Repository: %s, image creation: %s, global team: %s",
				repoName, imageName, teamPrefix))
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"pending": true, "review_id": review.TaskID, "image_name": imageName,
		})
	}
	image, err := db.CreateDockerImageForTeam(
		repoName, imageName, user.Username, teamPrefix, request.Private, createdAt)
	if errors.Is(err, core.ErrDockerImageExists) {
		return c.Status(fiber.StatusConflict).SendString("Docker image already exists")
	}
	if errors.Is(err, core.ErrSuperTeamBindingRequired) {
		return dockerAPIError(c, fiber.StatusBadRequest, "super_team_required", err.Error())
	}
	if errors.Is(err, core.ErrSuperTeamBindingMismatch) {
		return dockerAPIError(c, fiber.StatusBadRequest, "super_team_mismatch", err.Error())
	}
	if errors.Is(err, core.ErrSuperTeamBindingPermission) {
		return dockerAPIError(c, fiber.StatusForbidden, "super_team_permission", err.Error())
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to create Docker image")
	}
	logDockerAudit(c, state, audit.ActionDockerImageCreate,
		fmt.Sprintf("Repository: %s, image: %s, private: %t, global team: %s",
			repoName, imageName, request.Private, teamPrefix))
	return c.Status(fiber.StatusCreated).JSON(image)
}

// GetDockerImageDetailsAPI handles GET /api/docker/repositories/:repo_name/images/*
func GetDockerImageDetailsAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	imageName := c.Query("image")
	if imageName == "" {
		imageName = strings.Trim(c.Params("*"), "/")
	}
	if unescaped, err := url.PathUnescape(imageName); err == nil && unescaped != "" {
		imageName = unescaped
	}
	imageName = strings.Trim(imageName, "/")

	if !utils.IsValidRepositoryName(repoName) || imageName == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid parameters")
	}

	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}

	user := auth.GetUser(c)
	if !docker.CanReadDocker(state, user, repo, repoName+"/"+imageName) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}

	details, err := db.GetDockerImageDetails(repoName, imageName, user.Username)
	if err != nil || details == nil {
		return c.Status(fiber.StatusNotFound).SendString("Image not found")
	}
	deprecated, err := db.IsPackageDeprecated(config.RepositoryFormatDocker, repoName, imageName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to inspect image state")
	}
	details.Image.Deprecated = deprecated

	if user.IsManager() || user.CheckUpdatePermission(repoName) {
		details.Administrator = true
		details.PermissionLevel = core.DockerPermissionFull
	}
	if details.Administrator || user.CheckModeratePermission(repoName) ||
		details.PermissionLevel >= core.DockerPermissionPublish {
		if err := docker.AddPendingPublicationTags(state, details); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load pending Docker publications")
		}
	}

	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(fiber.StatusOK).JSON(details)
}

// DeprecateDockerImageAPI permanently freezes one image for L3/L4 managers.
func DeprecateDockerImageAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	imageName := strings.Trim(c.Query("image"), "/")
	if !utils.IsValidRepositoryName(repoName) || imageName == "" {
		return dockerAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid Docker image")
	}
	db := state.GetDB()
	if db == nil {
		return dockerAPIError(c, fiber.StatusServiceUnavailable, "service_unavailable", "Database unavailable")
	}
	details, err := db.GetDockerImageDetails(repoName, imageName, auth.GetUser(c).Username)
	if err != nil || details == nil || details.Image == nil {
		return dockerAPIError(c, fiber.StatusNotFound, "image_not_found", "Docker image was not found")
	}
	user := auth.GetUser(c)
	administrator := user.IsManager() || user.CheckUpdatePermission(repoName)
	if !administrator && details.PermissionLevel < core.DockerPermissionTeam {
		return dockerAPIError(c, fiber.StatusForbidden, "permission_denied", "L3 or L4 image permission is required")
	}
	if details.Image.Mirrored {
		return dockerAPIError(c, fiber.StatusForbidden, "permission_denied", "Mirrored images cannot be deprecated")
	}
	if err := db.DeprecatePackage(config.RepositoryFormatDocker, repoName, imageName,
		time.Now().UnixMilli()); err != nil {
		switch {
		case errors.Is(err, core.ErrPackageDeprecated):
			return dockerAPIError(c, fiber.StatusConflict, "package_deprecated", "Docker image is already deprecated")
		case errors.Is(err, core.ErrPackageDeprecationPending):
			return dockerAPIError(c, fiber.StatusConflict, "review_pending", "Resolve pending reviews before deprecating this image")
		default:
			return dockerAPIError(c, fiber.StatusInternalServerError, "internal_error", "Failed to deprecate Docker image")
		}
	}
	logDockerAudit(c, state, audit.ActionPackageDeprecate,
		"Format: docker, repository: "+repoName+", package: "+imageName)
	return c.JSON(fiber.Map{"ok": true})
}

// UpdateDockerImageDescriptionRequest represents the payload for updating an image description / README.
type UpdateDockerImageDescriptionRequest struct {
	Description string `json:"description"`
}

// UpdateDockerImageDescriptionAPI handles PUT /api/docker/repositories/:repo_name/images/*
func UpdateDockerImageDescriptionAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	imageName := c.Query("image")
	if imageName == "" {
		imageName = strings.Trim(c.Params("*"), "/")
	}
	if unescaped, err := url.PathUnescape(imageName); err == nil && unescaped != "" {
		imageName = unescaped
	}
	imageName = strings.Trim(imageName, "/")

	if !utils.IsValidRepositoryName(repoName) || imageName == "" {
		return dockerAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid parameters")
	}

	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return dockerAPIError(c, fiber.StatusNotFound, "repository_not_found", "Repository not found")
	}

	db := state.GetDB()
	if db == nil {
		return dockerAPIError(c, fiber.StatusServiceUnavailable, "service_unavailable", "Database unavailable")
	}
	if err := ensureDockerImageMutable(state, repoName, imageName); err != nil {
		return dockerImageMutationError(c, err)
	}

	user := auth.GetUser(c)
	canManage := user.IsManager() || user.CheckUpdatePermission(repoName)
	if !canManage {
		lvl, _ := db.GetDockerMemberLevel(repoName, imageName, user.Username)
		canManage = lvl >= core.DockerPermissionManage
	}
	if !canManage {
		return dockerAPIError(c, fiber.StatusForbidden, "permission_denied", "Forbidden")
	}

	var req UpdateDockerImageDescriptionRequest
	if err := utils.ReadJSONLimited(c, &req, maxDockerReadmeBytes+(16<<10)); errors.Is(err, fiber.ErrRequestEntityTooLarge) {
		return dockerAPIError(c, fiber.StatusRequestEntityTooLarge, "readme_too_large", "README exceeds the size limit")
	} else if err != nil {
		return dockerAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid request body")
	}
	if len(req.Description) > maxDockerReadmeBytes {
		return dockerAPIError(c, fiber.StatusRequestEntityTooLarge, "readme_too_large", "README exceeds the size limit")
	}

	if err := db.UpdateDockerImageDescription(repoName, imageName, req.Description); err != nil {
		if errors.Is(err, core.ErrDockerImageNotFound) {
			return dockerAPIError(c, fiber.StatusNotFound, "image_not_found", "Image not found")
		}
		return dockerAPIError(c, fiber.StatusInternalServerError, "internal_error", "Failed to update README")
	}

	logDockerAudit(c, state, audit.ActionDockerImageUpdate, fmt.Sprintf("Repository: %s, image: %s", repoName, imageName))

	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":      "updated",
		"description": req.Description,
	})
}

// DeleteDockerImageAPI handles DELETE /api/docker/repositories/:repo_name/images/*
func DeleteDockerImageAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	imageName := c.Query("image")
	if imageName == "" {
		imageName = strings.Trim(c.Params("*"), "/")
	}
	if unescaped, err := url.PathUnescape(imageName); err == nil && unescaped != "" {
		imageName = unescaped
	}
	imageName = strings.Trim(imageName, "/")

	if !utils.IsValidRepositoryName(repoName) || imageName == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid parameters")
	}

	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	if err := ensureDockerImageMutable(state, repoName, imageName); err != nil {
		return dockerImageMutationError(c, err)
	}

	user := auth.GetUser(c)
	canDelete := user.IsManager() || user.CheckUpdatePermission(repoName)
	if !canDelete {
		lvl, _ := db.GetDockerMemberLevel(repoName, imageName, user.Username)
		canDelete = lvl >= core.DockerPermissionFull
	}
	if !canDelete {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	if err := db.DeleteDockerImage(repoName, imageName); err != nil {
		if errors.Is(err, core.ErrDockerImageNotFound) {
			return c.Status(fiber.StatusNotFound).SendString("Image not found")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete image")
	}

	logDockerAudit(c, state, audit.ActionDockerImageDelete, fmt.Sprintf("Repository: %s, image: %s", repoName, imageName))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "deleted"})
}

// DeleteDockerTagAPI handles DELETE /api/docker/repositories/:repo_name/tags/*
func DeleteDockerTagAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	imageName := c.Query("image")
	tag := c.Query("tag")

	if imageName == "" || tag == "" {
		wildcard := strings.Trim(c.Params("*"), "/")
		if unescaped, err := url.PathUnescape(wildcard); err == nil && unescaped != "" {
			wildcard = unescaped
		}
		if wildcard != "" {
			idx := strings.LastIndex(wildcard, "/")
			if idx != -1 {
				imageName = wildcard[:idx]
				tag = wildcard[idx+1:]
			}
		}
	}

	if unescaped, err := url.PathUnescape(imageName); err == nil && unescaped != "" {
		imageName = unescaped
	}
	if unescaped, err := url.PathUnescape(tag); err == nil && unescaped != "" {
		tag = unescaped
	}
	imageName = strings.Trim(imageName, "/")
	tag = strings.TrimSpace(tag)

	if !utils.IsValidRepositoryName(repoName) || imageName == "" || tag == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid parameters")
	}

	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	if err := ensureDockerImageMutable(state, repoName, imageName); err != nil {
		return dockerImageMutationError(c, err)
	}

	user := auth.GetUser(c)
	canManage := user.IsManager() || user.CheckUpdatePermission(repoName)
	if !canManage {
		lvl, _ := db.GetDockerMemberLevel(repoName, imageName, user.Username)
		canManage = lvl >= core.DockerPermissionManage
	}
	if !canManage {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	if err := db.DeleteDockerTag(repoName, imageName, tag); err != nil {
		if errors.Is(err, core.ErrDockerTagNotFound) {
			return c.Status(fiber.StatusNotFound).SendString("Tag not found")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete tag")
	}

	logDockerAudit(c, state, audit.ActionDockerTagDelete, fmt.Sprintf("Repository: %s, image: %s, tag: %s", repoName, imageName, tag))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "deleted"})
}

// GetDockerManifestAPI handles GET /api/docker/repositories/:repo_name/manifests/*
func GetDockerManifestAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	imageName := c.Query("image")
	ref := c.Query("ref")
	if ref == "" {
		ref = c.Query("tag")
	}
	if ref == "" {
		ref = c.Query("digest")
	}

	if imageName == "" || ref == "" {
		wildcard := strings.Trim(c.Params("*"), "/")
		if unescaped, err := url.PathUnescape(wildcard); err == nil && unescaped != "" {
			wildcard = unescaped
		}
		if wildcard != "" {
			idx := strings.LastIndex(wildcard, "/")
			if idx != -1 {
				imageName = wildcard[:idx]
				ref = wildcard[idx+1:]
			}
		}
	}

	if unescaped, err := url.PathUnescape(imageName); err == nil && unescaped != "" {
		imageName = unescaped
	}
	if unescaped, err := url.PathUnescape(ref); err == nil && unescaped != "" {
		ref = unescaped
	}
	imageName = strings.Trim(imageName, "/")
	ref = strings.TrimSpace(ref)

	if !utils.IsValidRepositoryName(repoName) || imageName == "" || ref == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid parameters")
	}

	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}

	user := auth.GetUser(c)
	if !docker.CanReadDocker(state, user, repo, repoName+"/"+imageName) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}

	digest := ref
	if !strings.HasPrefix(ref, "sha256:") {
		t, err := db.GetDockerTag(repoName, imageName, ref)
		if err == nil && t != nil && t.Digest != "" {
			digest = t.Digest
		}
	}

	manifest, err := db.GetDockerManifest(repoName, imageName, digest)
	if (err != nil || manifest == nil) && digest != ref {
		manifest, err = db.GetDockerManifest(repoName, imageName, ref)
	}
	if err != nil || manifest == nil {
		return c.Status(fiber.StatusNotFound).SendString("Manifest not found")
	}

	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"repository":    manifest.Repository,
		"image_name":    manifest.ImageName,
		"digest":        manifest.Digest,
		"media_type":    manifest.MediaType,
		"size":          manifest.Size,
		"config_digest": manifest.ConfigDigest,
		"publisher":     manifest.Publisher,
		"created_at":    manifest.CreatedAt,
		"raw_json":      string(manifest.RawJSON),
	})
}

// ListDockerOwnersAPI handles GET /api/docker/repositories/:repo_name/owners?image=...
func ListDockerOwnersAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	imageName := strings.Trim(c.Query("image"), "/")
	if !utils.IsValidRepositoryName(repoName) || imageName == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid parameters")
	}

	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}

	user := auth.GetUser(c)
	if !docker.CanReadDocker(state, user, repo, repoName+"/"+imageName) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}

	members, err := db.ListDockerMembers(repoName, imageName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to list members")
	}

	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"users": members,
	})
}

type dockerInviteRequest struct {
	Users []string `json:"users"`
	Level int      `json:"level"`
}

// InviteDockerOwnersAPI handles POST /api/docker/repositories/:repo_name/owners?image=...
func InviteDockerOwnersAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	imageName := strings.Trim(c.Query("image"), "/")
	if !utils.IsValidRepositoryName(repoName) || imageName == "" {
		return dockerAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid parameters")
	}

	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return dockerAPIError(c, fiber.StatusNotFound, "repository_not_found", "Repository not found")
	}

	db := state.GetDB()
	if db == nil {
		return dockerAPIError(c, fiber.StatusServiceUnavailable, "service_unavailable", "Database unavailable")
	}
	if err := ensureDockerImageMutable(state, repoName, imageName); err != nil {
		return dockerImageMutationError(c, err)
	}

	user := auth.GetUser(c)
	isAdmin := user.IsManager() || user.CheckUpdatePermission(repoName)
	memberLevel, err := db.GetDockerMemberLevel(repoName, imageName, user.Username)
	if err != nil {
		return dockerAPIError(c, fiber.StatusInternalServerError, "internal_error", "Failed to inspect member permission")
	}
	canManage := isAdmin || memberLevel >= core.DockerPermissionTeam
	if !canManage {
		return dockerAPIError(c, fiber.StatusForbidden, "permission_denied", "Forbidden")
	}

	var req dockerInviteRequest
	if err := c.Bind().Body(&req); err != nil || len(req.Users) == 0 || len(req.Users) > 20 {
		return dockerAPIError(c, fiber.StatusBadRequest, "invalid_request", "Choose between 1 and 20 members to invite")
	}

	if req.Level < core.DockerPermissionRead || req.Level > core.DockerPermissionOwner {
		return dockerAPIError(c, fiber.StatusBadRequest, "invalid_permission_level", "Permission level must be between 0 and 4")
	}

	if isAdmin {
		validUsers := make([]string, 0, len(req.Users))
		seen := make(map[string]struct{}, len(req.Users))
		for _, candidate := range req.Users {
			recipient := strings.ToLower(strings.TrimSpace(candidate))
			if recipient == "" || len(recipient) > 255 {
				continue
			}
			if _, dup := seen[recipient]; dup {
				continue
			}
			seen[recipient] = struct{}{}

			token, err := db.GetTokenByName(recipient)
			if err != nil || token == nil {
				return dockerAPIError(c, fiber.StatusBadRequest, "user_not_found", fmt.Sprintf("User %s does not exist", recipient))
			}
			validUsers = append(validUsers, recipient)
		}

		if len(validUsers) == 0 {
			return dockerAPIError(c, fiber.StatusBadRequest, "invalid_request", "No valid recipients provided")
		}

		if err := db.ForceAddDockerMembers(repoName, imageName, user.Username, validUsers, req.Level); err != nil {
			if errors.Is(err, core.ErrDockerMemberExists) {
				return dockerAPIError(c, fiber.StatusConflict, "member_exists", "User is already a member of this image team")
			}
			return dockerAPIError(c, fiber.StatusInternalServerError, "internal_error", "Failed to add members directly")
		}

		logDockerAudit(c, state, audit.ActionDockerTeamAdd, fmt.Sprintf("Repository: %s, image: %s, members: %d, level: L%d", repoName, imageName, len(validUsers), req.Level))

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"ok":      true,
			"message": fmt.Sprintf("Added %d user(s) to %s", len(validUsers), imageName),
		})
	}

	now := time.Now().UnixMilli()
	invitations := make([]*core.DockerInvitation, 0, len(req.Users))
	messages := make([]*core.UserMessage, 0, len(req.Users))
	seen := make(map[string]struct{}, len(req.Users))

	for _, candidate := range req.Users {
		recipient := strings.ToLower(strings.TrimSpace(candidate))
		if recipient == "" || len(recipient) > 255 {
			continue
		}
		if recipient == strings.ToLower(user.Username) {
			return dockerAPIError(c, fiber.StatusBadRequest, "cannot_invite_self", "Cannot invite yourself")
		}
		if _, dup := seen[recipient]; dup {
			continue
		}
		seen[recipient] = struct{}{}

		token, err := db.GetTokenByName(recipient)
		if err != nil || token == nil {
			return dockerAPIError(c, fiber.StatusBadRequest, "user_not_found", fmt.Sprintf("User %s does not exist", recipient))
		}

		invID := uuid.NewString()
		payloadBytes, _ := json.Marshal(fiber.Map{
			"repository": repoName,
			"image":      imageName,
			"inviter":    user.Username,
			"level":      req.Level,
		})

		invitations = append(invitations, &core.DockerInvitation{
			ID:         invID,
			Repository: repoName,
			ImageName:  imageName,
			Inviter:    user.Username,
			Recipient:  recipient,
			Level:      req.Level,
			CreatedAt:  now,
		})

		messages = append(messages, &core.UserMessage{
			ID:           invID,
			Recipient:    recipient,
			Sender:       strings.ToLower(user.Username),
			Kind:         "docker_image_invite",
			Severity:     "info",
			Title:        "Docker container image invitation",
			Body:         fmt.Sprintf("%s invited you to collaborate on %s with L%d permission.", user.Username, imageName, req.Level),
			Payload:      payloadBytes,
			ActionKind:   "docker_image_invite",
			ActionStatus: core.MessageActionPending,
			CreatedAt:    now,
			ExpiresAt:    now + 7*24*3600*1000,
		})
	}

	if len(invitations) == 0 {
		return dockerAPIError(c, fiber.StatusBadRequest, "invalid_request", "No valid recipients provided")
	}

	if err := db.CreateDockerInvitations(invitations, messages); err != nil {
		if errors.Is(err, core.ErrDockerPermissionDenied) {
			return dockerAPIError(c, fiber.StatusForbidden, "permission_denied", "Permission denied")
		}
		if errors.Is(err, core.ErrDockerMemberExists) {
			return dockerAPIError(c, fiber.StatusConflict, "member_exists", "User is already a member of this image team")
		}
		if errors.Is(err, core.ErrDockerInvitationExists) {
			return dockerAPIError(c, fiber.StatusConflict, "invitation_pending", "An active invitation is already pending for this user")
		}
		return dockerAPIError(c, fiber.StatusInternalServerError, "internal_error", "Failed to create invitations")
	}

	logDockerAudit(c, state, audit.ActionDockerTeamInvite, fmt.Sprintf("Repository: %s, image: %s, recipients: %d, level: L%d", repoName, imageName, len(invitations), req.Level))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"ok":      true,
		"message": fmt.Sprintf("Invited %d user(s) to %s", len(invitations), imageName),
	})
}

type dockerLevelRequest struct {
	Level int `json:"level"`
}

func resolveDockerMemberReference(db core.StateDB, reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if _, err := uuid.Parse(reference); err != nil {
		return reference, nil
	}
	profile, err := db.GetUserProfileByID(reference)
	if err != nil {
		return "", err
	}
	return profile.Username, nil
}

// SetDockerOwnerLevelAPI handles PUT /api/docker/repositories/:repo_name/owners/:username?image=...
func SetDockerOwnerLevelAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	targetReference := strings.TrimSpace(c.Params("username"))
	imageName := strings.Trim(c.Query("image"), "/")

	if !utils.IsValidRepositoryName(repoName) || imageName == "" || targetReference == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid parameters")
	}

	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	if err := ensureDockerImageMutable(state, repoName, imageName); err != nil {
		return dockerImageMutationError(c, err)
	}
	targetUsername, err := resolveDockerMemberReference(db, targetReference)
	if err != nil {
		return c.Status(fiber.StatusNotFound).SendString("Member not found")
	}

	user := auth.GetUser(c)
	isAdmin := user.IsManager() || user.CheckUpdatePermission(repoName)
	memberLevel, err := db.GetDockerMemberLevel(repoName, imageName, user.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to inspect member permission")
	}
	canManage := isAdmin || memberLevel >= core.DockerPermissionTeam
	if !canManage {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	var req dockerLevelRequest
	if err := c.Bind().Body(&req); err != nil || req.Level < core.DockerPermissionRead || req.Level > core.DockerPermissionOwner {
		return c.Status(fiber.StatusBadRequest).SendString("Permission level must be between 0 and 4")
	}

	actor := user.Username
	if isAdmin && !(req.Level == core.DockerPermissionOwner && memberLevel == core.DockerPermissionOwner &&
		!strings.EqualFold(targetUsername, user.Username)) {
		actor = ""
	}

	if err := db.SetDockerMemberLevel(repoName, imageName, actor, targetUsername, req.Level); err != nil {
		if errors.Is(err, core.ErrDockerLastFullMember) || errors.Is(err, core.ErrDockerOwnerCannotLeave) {
			return c.Status(fiber.StatusBadRequest).SendString("Cannot demote the last L4 owner of this image")
		}
		if errors.Is(err, core.ErrDockerPermissionDenied) {
			return c.Status(fiber.StatusForbidden).SendString("Only the current L4 owner can transfer ownership")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update member level")
	}

	logDockerAudit(c, state, audit.ActionDockerTeamLevel, fmt.Sprintf("Repository: %s, image: %s, member: %s, level: L%d", repoName, imageName, targetUsername, req.Level))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"ok":      true,
		"message": "Permission level updated",
	})
}

// RemoveDockerOwnerAPI handles DELETE /api/docker/repositories/:repo_name/owners/:username?image=...
func RemoveDockerOwnerAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	targetReference := strings.TrimSpace(c.Params("username"))
	imageName := strings.Trim(c.Query("image"), "/")

	if !utils.IsValidRepositoryName(repoName) || imageName == "" || targetReference == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid parameters")
	}

	cfg := state.Inner.Config.Load()
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	if err := ensureDockerImageMutable(state, repoName, imageName); err != nil {
		return dockerImageMutationError(c, err)
	}
	targetUsername, err := resolveDockerMemberReference(db, targetReference)
	if err != nil {
		return c.Status(fiber.StatusNotFound).SendString("Member not found")
	}

	user := auth.GetUser(c)
	isAdmin := user.IsManager() || user.CheckUpdatePermission(repoName)
	imageExists, _, _, member, memberLevel, err := db.GetDockerImageAccess(repoName, imageName, user.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to inspect member permission")
	}
	if !imageExists {
		return c.Status(fiber.StatusNotFound).SendString("Image not found")
	}
	isSelf := strings.EqualFold(targetUsername, user.Username)
	canManage := isAdmin || memberLevel >= core.DockerPermissionTeam || (isSelf && member)
	if !canManage {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	actor := user.Username
	if isAdmin && !isSelf {
		actor = ""
	}

	if err := db.RemoveDockerMember(repoName, imageName, actor, targetUsername); err != nil {
		if errors.Is(err, core.ErrDockerLastFullMember) {
			return c.Status(fiber.StatusBadRequest).SendString("Cannot remove the last L4 owner of this image")
		}
		if errors.Is(err, core.ErrDockerOwnerCannotLeave) {
			return c.Status(fiber.StatusBadRequest).SendString("L4 owners must transfer ownership before leaving")
		}
		if errors.Is(err, core.ErrDockerPermissionDenied) {
			return c.Status(fiber.StatusForbidden).SendString("Forbidden")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to remove member")
	}

	logDockerAudit(c, state, audit.ActionDockerTeamRemove, fmt.Sprintf("Repository: %s, image: %s, member: %s", repoName, imageName, targetUsername))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"ok":      true,
		"message": "Member removed",
	})
}

// SearchDockerUsersAPI handles GET /api/docker/repositories/:repo_name/users/search?q=...
func SearchDockerUsersAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	query := strings.TrimSpace(c.Query("q"))
	if !utils.IsValidRepositoryName(repoName) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid parameters")
	}

	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}

	users, err := db.SearchTokenNames(query, 10, time.Now().UnixMilli())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to search users")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"users": users,
	})
}

// RespondDockerInvitationAPI handles POST /api/docker/repositories/:repo_name/invitations/:id/:decision
func RespondDockerInvitationAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	invitationID := strings.TrimSpace(c.Params("id"))
	decision := strings.ToLower(strings.TrimSpace(c.Params("decision")))

	if !utils.IsValidRepositoryName(repoName) || invitationID == "" || (decision != "accept" && decision != "reject") {
		return dockerAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid parameters")
	}

	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return dockerAPIError(c, fiber.StatusUnauthorized, "authentication_required", "Authentication required")
	}

	db := state.GetDB()
	if db == nil {
		return dockerAPIError(c, fiber.StatusServiceUnavailable, "service_unavailable", "Database unavailable")
	}

	accept := decision == "accept"
	now := time.Now().UnixMilli()
	if err := db.RespondDockerInvitation(invitationID, user.Username, repoName, accept, now); err != nil {
		if errors.Is(err, core.ErrDockerInvitationInvalid) {
			return dockerAPIError(c, fiber.StatusBadRequest, "invitation_invalid", "Invitation is invalid or has expired")
		}
		return dockerAPIError(c, fiber.StatusInternalServerError, "internal_error", "Failed to process invitation response")
	}

	action := audit.ActionDockerInviteReject
	if decision == "accept" {
		action = audit.ActionDockerInviteAccept
	}
	logDockerAudit(c, state, action, fmt.Sprintf("Repository: %s, invitation: %s", repoName, invitationID))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"ok":      true,
		"message": fmt.Sprintf("Invitation %sed", decision),
	})
}
