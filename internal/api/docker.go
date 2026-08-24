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

	for _, img := range images {
		tags, _ := db.ListDockerTags(repoName, img.ImageName, "", 100)
		img.TagCount = len(tags)
		if len(tags) > 0 {
			img.LatestTag = tags[0].Tag
		}
	}

	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"repository": repoName,
		"images":     images,
	})
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
	if !docker.CanReadDocker(state, user, repo, repoName) {
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

	if user.IsManager() || user.CheckUpdatePermission(repoName) {
		details.Administrator = true
		details.PermissionLevel = core.DockerPermissionFull
	}

	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(fiber.StatusOK).JSON(details)
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

	user := auth.GetUser(c)
	canManage := user.IsManager() || user.CheckUpdatePermission(repoName)
	if !canManage {
		lvl, _ := db.GetDockerMemberLevel(repoName, imageName, user.Username)
		canManage = lvl >= core.DockerPermissionManage
	}
	if !canManage {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	var req UpdateDockerImageDescriptionRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid request body")
	}

	if err := db.UpdateDockerImageDescription(repoName, imageName, req.Description); err != nil {
		if errors.Is(err, core.ErrDockerImageNotFound) {
			return c.Status(fiber.StatusNotFound).SendString("Image not found")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update description")
	}

	logDockerAudit(c, state, "DOCKER_IMAGE_UPDATE", fmt.Sprintf("Repository: %s, image: %s", repoName, imageName))

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

	logDockerAudit(c, state, "DOCKER_IMAGE_DELETE", fmt.Sprintf("Repository: %s, image: %s", repoName, imageName))

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

	logDockerAudit(c, state, "DOCKER_TAG_DELETE", fmt.Sprintf("Repository: %s, image: %s, tag: %s", repoName, imageName, tag))

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
	if !docker.CanReadDocker(state, user, repo, repoName) {
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
	if !docker.CanReadDocker(state, user, repo, repoName) {
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

	var req dockerInviteRequest
	if err := c.Bind().Body(&req); err != nil || len(req.Users) == 0 || len(req.Users) > 20 {
		return c.Status(fiber.StatusBadRequest).SendString("Choose between 1 and 20 members to invite")
	}

	if req.Level < core.DockerPermissionPublish || req.Level > core.DockerPermissionOwner {
		req.Level = core.DockerPermissionPublish
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
				return c.Status(fiber.StatusBadRequest).SendString(fmt.Sprintf("User %s does not exist", recipient))
			}
			validUsers = append(validUsers, recipient)
		}

		if len(validUsers) == 0 {
			return c.Status(fiber.StatusBadRequest).SendString("No valid recipients provided")
		}

		if err := db.ForceAddDockerMembers(repoName, imageName, user.Username, validUsers, req.Level); err != nil {
			if errors.Is(err, core.ErrDockerMemberExists) {
				return c.Status(fiber.StatusConflict).SendString("User is already a member of this image team")
			}
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to add members directly")
		}

		logDockerAudit(c, state, "DOCKER_TEAM_ADD", fmt.Sprintf("Repository: %s, image: %s, members: %d, level: L%d", repoName, imageName, len(validUsers), req.Level))

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
			return c.Status(fiber.StatusBadRequest).SendString("Cannot invite yourself")
		}
		if _, dup := seen[recipient]; dup {
			continue
		}
		seen[recipient] = struct{}{}

		token, err := db.GetTokenByName(recipient)
		if err != nil || token == nil {
			return c.Status(fiber.StatusBadRequest).SendString(fmt.Sprintf("User %s does not exist", recipient))
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
		return c.Status(fiber.StatusBadRequest).SendString("No valid recipients provided")
	}

	if err := db.CreateDockerInvitations(invitations, messages); err != nil {
		if errors.Is(err, core.ErrDockerPermissionDenied) {
			return c.Status(fiber.StatusForbidden).SendString("Permission denied")
		}
		if errors.Is(err, core.ErrDockerMemberExists) {
			return c.Status(fiber.StatusConflict).SendString("User is already a member of this image team")
		}
		if errors.Is(err, core.ErrDockerInvitationExists) {
			return c.Status(fiber.StatusConflict).SendString("An active invitation is already pending for this user")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to create invitations")
	}

	logDockerAudit(c, state, "DOCKER_TEAM_INVITE", fmt.Sprintf("Repository: %s, image: %s, recipients: %d, level: L%d", repoName, imageName, len(invitations), req.Level))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"ok":      true,
		"message": fmt.Sprintf("Invited %d user(s) to %s", len(invitations), imageName),
	})
}

type dockerLevelRequest struct {
	Level int `json:"level"`
}

// SetDockerOwnerLevelAPI handles PUT /api/docker/repositories/:repo_name/owners/:username?image=...
func SetDockerOwnerLevelAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	targetUsername := strings.TrimSpace(c.Params("username"))
	imageName := strings.Trim(c.Query("image"), "/")

	if !utils.IsValidRepositoryName(repoName) || imageName == "" || targetUsername == "" {
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
	if err := c.Bind().Body(&req); err != nil || req.Level < core.DockerPermissionPublish || req.Level > core.DockerPermissionOwner {
		return c.Status(fiber.StatusBadRequest).SendString("Permission level must be between 1 and 4")
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

	logDockerAudit(c, state, "DOCKER_TEAM_LEVEL", fmt.Sprintf("Repository: %s, image: %s, member: %s, level: L%d", repoName, imageName, targetUsername, req.Level))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"ok":      true,
		"message": "Permission level updated",
	})
}

// RemoveDockerOwnerAPI handles DELETE /api/docker/repositories/:repo_name/owners/:username?image=...
func RemoveDockerOwnerAPI(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	targetUsername := strings.TrimSpace(c.Params("username"))
	imageName := strings.Trim(c.Query("image"), "/")

	if !utils.IsValidRepositoryName(repoName) || imageName == "" || targetUsername == "" {
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

	user := auth.GetUser(c)
	isAdmin := user.IsManager() || user.CheckUpdatePermission(repoName)
	memberLevel, err := db.GetDockerMemberLevel(repoName, imageName, user.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to inspect member permission")
	}
	isSelf := strings.EqualFold(targetUsername, user.Username)
	canManage := isAdmin || memberLevel >= core.DockerPermissionTeam || (isSelf && memberLevel > 0)
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

	logDockerAudit(c, state, "DOCKER_TEAM_REMOVE", fmt.Sprintf("Repository: %s, image: %s, member: %s", repoName, imageName, targetUsername))

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
		return c.Status(fiber.StatusBadRequest).SendString("Invalid parameters")
	}

	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return c.Status(fiber.StatusUnauthorized).SendString("Authentication required")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}

	accept := decision == "accept"
	now := time.Now().UnixMilli()
	if err := db.RespondDockerInvitation(invitationID, user.Username, repoName, accept, now); err != nil {
		if errors.Is(err, core.ErrDockerInvitationInvalid) {
			return c.Status(fiber.StatusBadRequest).SendString("Invitation is invalid or has expired")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to process invitation response")
	}

	logDockerAudit(c, state, "DOCKER_INVITE_"+strings.ToUpper(decision), fmt.Sprintf("Repository: %s, invitation: %s", repoName, invitationID))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"ok":      true,
		"message": fmt.Sprintf("Invitation %sed", decision),
	})
}
