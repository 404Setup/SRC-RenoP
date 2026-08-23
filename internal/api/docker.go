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
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/service/docker"
	"renop/internal/utils"
)

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

	details, err := db.GetDockerImageDetails(repoName, imageName)
	if err != nil || details == nil {
		return c.Status(fiber.StatusNotFound).SendString("Image not found")
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

	user := auth.GetUser(c)
	if !docker.CanWriteDocker(state, user, repo, repoName) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
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

	user := auth.GetUser(c)
	if !docker.CanWriteDocker(state, user, repo, repoName) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}

	if err := db.DeleteDockerImage(repoName, imageName); err != nil {
		if errors.Is(err, core.ErrDockerImageNotFound) {
			return c.Status(fiber.StatusNotFound).SendString("Image not found")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete image")
	}

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

	user := auth.GetUser(c)
	if !docker.CanWriteDocker(state, user, repo, repoName) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}

	if err := db.DeleteDockerTag(repoName, imageName, tag); err != nil {
		if errors.Is(err, core.ErrDockerTagNotFound) {
			return c.Status(fiber.StatusNotFound).SendString("Tag not found")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete tag")
	}

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
