/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package docker

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/repositorygate"
	"renop/internal/service/statistics"
)

// Handler handles all /v2 Docker/OCI registry endpoints.
type Handler struct {
	Store Store
}

func referencedBlobDigests(manifest *ParsedManifest) []string {
	if manifest == nil || manifest.IsIndex {
		return nil
	}
	digests := make([]string, 0, len(manifest.Layers)+1)
	if manifest.ConfigDigest != "" {
		digests = append(digests, manifest.ConfigDigest)
	}
	for _, layer := range manifest.Layers {
		if layer.Digest != "" {
			digests = append(digests, layer.Digest)
		}
	}
	return digests
}

func getParam(c fiber.Ctx, key string) string {
	if val, ok := c.Locals(key).(string); ok && val != "" {
		return val
	}
	return c.Params(key)
}

func (h *Handler) getRepo(state *core.AppState, repoName string) (*config.Repository, bool) {
	cfg := state.Inner.Config.Load()
	if cfg == nil || cfg.Maven.Repositories == nil {
		return nil, false
	}
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return nil, false
	}
	return repo, true
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

func (h *Handler) authenticateAndAuthorize(c fiber.Ctx, state *core.AppState, repo *config.Repository, repoFullName string, isWrite bool) (*config.User, error) {
	user := auth.GetUser(c)
	actionRequired := "pull"
	requiredCredentialScope := core.APITokenScopeRepositoryRead
	if isWrite {
		actionRequired = "push"
		requiredCredentialScope = core.APITokenScopeRepositoryPublish
		if c.Method() == fiber.MethodDelete && !strings.Contains(c.Path(), "/blobs/uploads/") {
			actionRequired = "delete"
			requiredCredentialScope = core.APITokenScopeRepositoryDelete
		}
	}

	authHeader := c.Get(fiber.HeaderAuthorization)
	if after, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
		tokenStr := after
		signingKey := state.GetDockerSecret()
		claims, err := ValidateDockerToken(signingKey, tokenStr)
		if err != nil || claims == nil {
			_ = SendAuthChallenge(c, c.Host(), "")
			return nil, errors.New("unauthorized")
		}
		user = &config.User{Username: claims.Subject}
		if tokenObj := state.GetTokenByName(claims.Subject); tokenObj != nil {
			user.Roles = tokenObj.Permissions
		}
		hasAccess := false
		for _, entry := range claims.Access {
			if entry.Name == repoFullName || entry.Name == "*" {
				for _, action := range entry.Actions {
					if action == actionRequired || action == "*" {
						hasAccess = true
						break
					}
				}
			}
		}
		c.Locals("user", user)
		if hasAccess && ((isWrite && CanWriteDocker(state, user, repo, repoFullName)) ||
			(!isWrite && CanReadDocker(state, user, repo, repoFullName))) {
			return user, nil
		}
		_ = RespondError(c, fiber.StatusForbidden, ErrCodeDenied, "access denied", nil)
		return nil, errors.New("denied")
	}
	if !auth.CurrentCredentialHasScopeTarget(c, requiredCredentialScope, repo.Name) {
		_ = RespondError(c, fiber.StatusForbidden, ErrCodeDenied, "API token scope is insufficient", nil)
		return nil, errors.New("insufficient API token scope")
	}

	if isWrite {
		if !CanWriteDocker(state, user, repo, repoFullName) {
			if user.Username == "guest" {
				requestedActions := "pull,push"
				if actionRequired == "delete" {
					requestedActions = "delete"
				}
				scope := fmt.Sprintf("repository:%s:%s", repoFullName, requestedActions)
				_ = SendAuthChallenge(c, c.Host(), scope)
				return nil, errors.New("unauthorized")
			}
			_ = RespondError(c, fiber.StatusForbidden, ErrCodeDenied, "access denied", nil)
			return nil, errors.New("denied")
		}
	} else {
		if !CanReadDocker(state, user, repo, repoFullName) {
			if user.Username == "guest" {
				scope := fmt.Sprintf("repository:%s:pull", repoFullName)
				_ = SendAuthChallenge(c, c.Host(), scope)
				return nil, errors.New("unauthorized")
			}
			_ = RespondError(c, fiber.StatusForbidden, ErrCodeDenied, "access denied", nil)
			return nil, errors.New("denied")
		}
	}

	c.Locals("user", user)
	return user, nil
}

// HandleBase handles GET /v2/ and HEAD /v2/ version check endpoint.
func (h *Handler) HandleBase(c fiber.Ctx) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	user := auth.GetUser(c)
	if user == nil || user.Username == "guest" {
		_ = SendAuthChallenge(c, c.Host(), "")
		return nil
	}
	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(fiber.StatusOK).SendString("{}")
}

// HandleCatalog handles GET /v2/_catalog
func (h *Handler) HandleCatalog(c fiber.Ctx, state *core.AppState) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	user := auth.GetUser(c)
	cfg := state.Inner.Config.Load()
	db := state.GetDB()
	if db == nil {
		return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
	}

	n, _ := strconv.Atoi(c.Query("n", "50"))
	last := c.Query("last")

	var allRepos []string
	for repoName, repo := range cfg.Maven.Repositories {
		if repo.NormalizedFormat() != config.RepositoryFormatDocker {
			continue
		}
		if !CanReadDocker(state, user, repo, repoName) {
			continue
		}
		images, err := db.ListDockerImages(repoName, last, n)
		if err == nil {
			for _, img := range images {
				if !CanReadDocker(state, user, repo, repoName+"/"+img.ImageName) {
					continue
				}
				allRepos = append(allRepos, fmt.Sprintf("%s/%s", repoName, img.ImageName))
			}
		}
	}

	if n > 0 && len(allRepos) >= n {
		lastItem := allRepos[len(allRepos)-1]
		c.Set("Link", fmt.Sprintf(`</v2/_catalog?last=%s&n=%d>; rel="next"`, url.QueryEscape(lastItem), n))
	}

	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(fiber.StatusOK).JSON(CatalogList{Repositories: allRepos})
}

// HandleTagsList handles GET /v2/<name>/tags/list
func (h *Handler) HandleTagsList(c fiber.Ctx, state *core.AppState) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	name := getParam(c, "name")
	repoName, imageName := ParseRepositoryAndImage(name)
	repo, ok := h.getRepo(state, repoName)
	if !ok {
		return RespondError(c, fiber.StatusNotFound, ErrCodeNameUnknown, "repository not found", map[string]string{"name": name})
	}

	if _, err := h.authenticateAndAuthorize(c, state, repo, name, false); err != nil {
		return nil
	}

	db := state.GetDB()
	if db == nil {
		return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
	}

	n, _ := strconv.Atoi(c.Query("n", "50"))
	last := c.Query("last")

	tagObjects, err := db.ListDockerTags(repoName, imageName, last, n)
	if err != nil {
		return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported, "failed to list tags", nil)
	}

	tagNames := make([]string, 0, len(tagObjects))
	for _, t := range tagObjects {
		tagNames = append(tagNames, t.Tag)
	}

	if n > 0 && len(tagNames) >= n {
		lastTag := tagNames[len(tagNames)-1]
		c.Set("Link", fmt.Sprintf(`</v2/%s/tags/list?last=%s&n=%d>; rel="next"`, name, url.QueryEscape(lastTag), n))
	}

	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(fiber.StatusOK).JSON(TagList{
		Name: name,
		Tags: tagNames,
	})
}

// HandleGetManifest handles GET /v2/<name>/manifests/<reference> and HEAD /v2/<name>/manifests/<reference>
func (h *Handler) HandleGetManifest(c fiber.Ctx, state *core.AppState) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	name := getParam(c, "name")
	reference := getParam(c, "reference")
	if name == "" || reference == "" {
		return RespondError(c, fiber.StatusBadRequest, ErrCodeManifestInvalid, "invalid parameters", nil)
	}

	repoName, imageName := ParseRepositoryAndImage(name)
	repo, ok := h.getRepo(state, repoName)
	if !ok {
		return RespondError(c, fiber.StatusNotFound, ErrCodeNameUnknown, "repository not found", map[string]string{"name": name})
	}

	if _, err := h.authenticateAndAuthorize(c, state, repo, name, false); err != nil {
		return nil
	}

	db := state.GetDB()
	if db == nil {
		return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
	}
	imageExists, _, pushEnabled, _, _, err := db.GetDockerImageAccess(repoName, imageName, "")
	if err != nil {
		return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
	}

	digest := reference
	var mediaType string

	if !strings.HasPrefix(reference, "sha256:") {
		tag, tagErr := db.GetDockerTag(repoName, imageName, reference)
		if tagErr != nil {
			return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
		}
		if tag != nil {
			digest = tag.Digest
			mediaType = tag.MediaType
		} else {
			if imageExists && pushEnabled {
				return RespondError(c, fiber.StatusNotFound, ErrCodeManifestUnknown, "manifest not found", map[string]string{"reference": reference})
			}
			upstreamData, uMediaType, uDigest, uErr := FetchUpstreamManifest(c.Context(), state, repo, imageName, reference)
			if uErr == nil && len(upstreamData) > 0 {
				parsed, parseErr := ParseManifest(upstreamData, uMediaType)
				if parseErr != nil {
					return RespondError(c, fiber.StatusBadGateway, ErrCodeManifestInvalid, "upstream manifest is invalid", nil)
				}
				if err := db.CacheDockerManifest(&core.DockerManifest{
					Repository: repoName, ImageName: imageName, Digest: uDigest, MediaType: uMediaType,
					Size: parsed.Size, ConfigDigest: parsed.ConfigDigest,
					BlobDigests: referencedBlobDigests(parsed), RawJSON: upstreamData,
				}, reference); err != nil {
					if errors.Is(err, core.ErrDockerImageExists) {
						return RespondError(c, fiber.StatusNotFound, ErrCodeManifestUnknown, "manifest unknown", nil)
					}
					return RespondError(c, fiber.StatusBadGateway, ErrCodeUnsupported, "failed to cache upstream manifest metadata", nil)
				}
				mirrorPersist, _ := repo.GetCacheConfig()
				if mirrorPersist {
					if persistErr := h.Store.PutManifest(state, repoName, imageName, uDigest, upstreamData); persistErr != nil {
						log.Printf("failed to persist mirrored Docker manifest %s/%s@%s: %v", repoName, imageName, reference, persistErr)
					}
				}
				c.Set(fiber.HeaderContentType, uMediaType)
				c.Set(DockerDigestHeader, uDigest)
				c.Set(fiber.HeaderETag, fmt.Sprintf(`"%s"`, uDigest))
				c.Set(fiber.HeaderContentLength, strconv.Itoa(len(upstreamData)))
				if c.Method() == fiber.MethodHead {
					return c.SendStatus(fiber.StatusOK)
				}
				statistics.RecordDockerPull(c, state, repo, imageName, reference, parsed.Size)
				return c.Status(fiber.StatusOK).Send(upstreamData)
			}
			return RespondError(c, fiber.StatusNotFound, ErrCodeManifestUnknown, "manifest not found", map[string]string{"reference": reference})
		}
	}

	manifest, err := db.GetDockerManifest(repoName, imageName, digest)
	if err != nil {
		return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
	}
	var rawJSON []byte
	if manifest != nil {
		if mediaType == "" {
			mediaType = manifest.MediaType
		}
		rawJSON = manifest.RawJSON
		if len(rawJSON) == 0 {
			var openErr error
			rawJSON, _, openErr = h.Store.OpenManifest(repoName, imageName, digest)
			if openErr != nil {
				return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported, "failed to read manifest", nil)
			}
		}
	} else {
		if imageExists && pushEnabled {
			return RespondError(c, fiber.StatusNotFound, ErrCodeManifestUnknown, "manifest not found", map[string]string{"reference": reference})
		}
		var openErr error
		rawJSON, ok, openErr = h.Store.OpenManifest(repoName, imageName, digest)
		if openErr != nil {
			return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported, "failed to read manifest", nil)
		}
		if !ok {
			upstreamData, uMediaType, uDigest, uErr := FetchUpstreamManifest(c.Context(), state, repo, imageName, digest)
			if uErr == nil && len(upstreamData) > 0 {
				rawJSON = upstreamData
				mediaType = uMediaType
				digest = uDigest
			} else {
				return RespondError(c, fiber.StatusNotFound, ErrCodeManifestUnknown, "manifest not found", map[string]string{"reference": reference})
			}
		}
	}

	if mediaType == "" {
		mediaType = MediaTypeDockerManifest2
	}

	c.Set(fiber.HeaderContentType, mediaType)
	c.Set(DockerDigestHeader, digest)
	c.Set(fiber.HeaderETag, fmt.Sprintf(`"%s"`, digest))
	c.Set(fiber.HeaderContentLength, strconv.Itoa(len(rawJSON)))

	if c.Method() == fiber.MethodHead {
		return c.SendStatus(fiber.StatusOK)
	}

	pullSize := int64(len(rawJSON))
	if manifest != nil && manifest.Size > 0 {
		pullSize = manifest.Size
	}
	statistics.RecordDockerPull(c, state, repo, imageName, reference, pullSize)
	return c.Status(fiber.StatusOK).Send(rawJSON)
}

// HandlePutManifest handles PUT /v2/<name>/manifests/<reference>
func (h *Handler) HandlePutManifest(c fiber.Ctx, state *core.AppState) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	name := getParam(c, "name")
	reference := getParam(c, "reference")
	if name == "" || reference == "" {
		return RespondError(c, fiber.StatusBadRequest, ErrCodeManifestInvalid, "invalid parameters", nil)
	}

	repoName, imageName := ParseRepositoryAndImage(name)
	repo, ok := h.getRepo(state, repoName)
	if !ok {
		return RespondError(c, fiber.StatusNotFound, ErrCodeNameUnknown, "repository not found", map[string]string{"name": name})
	}
	release := repositorygate.AcquireMutation(repoName)
	defer release()

	user, err := h.authenticateAndAuthorize(c, state, repo, name, true)
	if err != nil {
		return nil
	}

	body := c.Body()
	if len(body) == 0 {
		return RespondError(c, fiber.StatusBadRequest, ErrCodeManifestInvalid, "empty manifest", nil)
	}

	contentType := c.Get(fiber.HeaderContentType)
	parsed, err := ParseManifest(body, contentType)
	if err != nil {
		return RespondError(c, fiber.StatusBadRequest, ErrCodeManifestInvalid, err.Error(), nil)
	}

	if strings.HasPrefix(reference, "sha256:") && reference != parsed.Digest {
		return RespondError(c, fiber.StatusBadRequest, ErrCodeDigestInvalid,
			"provided digest does not match manifest content", nil)
	}
	tag, _, validReference := normalizeManifestReference(reference, parsed.Digest)
	if !validReference {
		return RespondError(c, fiber.StatusBadRequest, ErrCodeTagInvalid, "invalid manifest reference", nil)
	}
	db := state.GetDB()
	if db == nil {
		return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
	}

	reviewPolicy := repo.PublicationReviewPolicy()
	reviewRequired := reviewPolicy == config.PublicationReviewEveryVersion
	createdAt := time.Now().UnixMilli()
	if reviewRequired {
		review, reviewErr := QueuePublicationReview(
			state, repo, imageName, reference, parsed, user.Username, true, createdAt)
		if reviewErr != nil || review == nil || !review.Pending {
			status := fiber.StatusInternalServerError
			if errors.Is(reviewErr, core.ErrReviewFileLimit) {
				status = fiber.StatusTooManyRequests
			}
			return RespondError(c, status, ErrCodeUnsupported,
				"failed to create Docker publication review", nil)
		}
		c.Set("X-RenoP-Review-ID", review.TaskID)
		c.Set(DockerDigestHeader, parsed.Digest)
		c.Set("Location", fmt.Sprintf("/v2/%s/manifests/%s", name, parsed.Digest))
		logDockerAudit(c, state, audit.ActionUploadQueuedReview,
			fmt.Sprintf("Repository: %s, image: %s, reference: %s, digest: %s",
				repoName, imageName, reference, parsed.Digest))
		return c.SendStatus(fiber.StatusAccepted)
	}

	if err := h.Store.PutManifest(state, repoName, imageName, parsed.Digest, body); err != nil {
		return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported, "failed to save manifest", nil)
	}

	manifestRecord := &core.DockerManifest{
		Repository:   repoName,
		ImageName:    imageName,
		Digest:       parsed.Digest,
		MediaType:    parsed.MediaType,
		Size:         parsed.Size,
		ConfigDigest: parsed.ConfigDigest,
		BlobDigests:  referencedBlobDigests(parsed),
		RawJSON:      body,
		CreatedAt:    createdAt,
	}

	if err := db.PutDockerManifest(manifestRecord, tag, user.Username); err != nil {
		if errors.Is(err, core.ErrDockerImageNotFound) {
			_ = h.Store.DeleteManifest(state, repoName, imageName, parsed.Digest)
			return RespondError(c, fiber.StatusNotFound, ErrCodeNameUnknown, "image must be created before push", nil)
		}
		return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported, "failed to record manifest", nil)
	}

	logDockerAudit(c, state, audit.ActionDockerManifestPut, fmt.Sprintf("Repository: %s, image: %s, tag: %s, digest: %s", repoName, imageName, tag, parsed.Digest))

	c.Set(DockerDigestHeader, parsed.Digest)
	c.Set("Location", fmt.Sprintf("/v2/%s/manifests/%s", name, parsed.Digest))
	return c.SendStatus(fiber.StatusCreated)
}

// HandleDeleteManifest handles DELETE /v2/<name>/manifests/<reference>
func (h *Handler) HandleDeleteManifest(c fiber.Ctx, state *core.AppState) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	name := getParam(c, "name")
	reference := getParam(c, "reference")

	repoName, imageName := ParseRepositoryAndImage(name)
	repo, ok := h.getRepo(state, repoName)
	if !ok {
		return RespondError(c, fiber.StatusNotFound, ErrCodeNameUnknown, "repository not found", nil)
	}

	if _, err := h.authenticateAndAuthorize(c, state, repo, name, true); err != nil {
		return nil
	}

	db := state.GetDB()
	if db == nil {
		return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
	}

	if !strings.HasPrefix(reference, "sha256:") {
		if err := db.DeleteDockerTag(repoName, imageName, reference); err != nil {
			if errors.Is(err, core.ErrDockerTagNotFound) {
				return RespondError(c, fiber.StatusNotFound, ErrCodeManifestUnknown, "tag not found", nil)
			}
			return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported, err.Error(), nil)
		}
	} else {
		if err := db.DeleteDockerManifest(repoName, imageName, reference); err != nil {
			if errors.Is(err, core.ErrDockerManifestNotFound) {
				return RespondError(c, fiber.StatusNotFound, ErrCodeManifestUnknown, "manifest not found", nil)
			}
			return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported, err.Error(), nil)
		}
		_ = h.Store.DeleteManifest(state, repoName, imageName, reference)
	}

	logDockerAudit(c, state, audit.ActionDockerManifestDelete, fmt.Sprintf("Repository: %s, image: %s, reference: %s", repoName, imageName, reference))

	return c.SendStatus(fiber.StatusAccepted)
}

// HandleGetBlob handles GET /v2/<name>/blobs/<digest> and HEAD /v2/<name>/blobs/<digest>
func (h *Handler) HandleGetBlob(c fiber.Ctx, state *core.AppState) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	name := getParam(c, "name")
	digest := getParam(c, "digest")

	repoName, imageName := ParseRepositoryAndImage(name)
	repo, ok := h.getRepo(state, repoName)
	if !ok {
		return RespondError(c, fiber.StatusNotFound, ErrCodeNameUnknown, "repository not found", nil)
	}

	if _, err := h.authenticateAndAuthorize(c, state, repo, name, false); err != nil {
		return nil
	}
	db := state.GetDB()
	if db == nil {
		return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
	}
	imageExists, _, _, _, _, err := db.GetDockerImageAccess(repoName, imageName, "")
	if err != nil {
		return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
	}
	allowLocalBlob := false
	if imageExists {
		allowLocalBlob, err = db.DockerImageReferencesBlob(repoName, imageName, digest)
		if err != nil {
			return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
		}
		if !allowLocalBlob {
			return RespondError(c, fiber.StatusNotFound, ErrCodeBlobUnknown, "blob is not referenced by image", nil)
		}
	}

	if allowLocalBlob {
		localPath, isLocalDisk := h.Store.BlobFilePath(repoName, digest)
		if isLocalDisk {
			c.Set(fiber.HeaderContentType, MediaTypeOctetStream)
			c.Set(DockerDigestHeader, digest)
			c.Set(fiber.HeaderETag, fmt.Sprintf(`"%s"`, digest))
			if c.Method() == fiber.MethodHead {
				exists, size, _ := h.Store.BlobExists(repoName, digest)
				if exists {
					c.Set(fiber.HeaderContentLength, strconv.FormatInt(size, 10))
					return c.SendStatus(fiber.StatusOK)
				}
			}
			return c.SendFile(localPath)
		}

		reader, size, exists, openErr := h.Store.OpenBlob(repoName, digest)
		if openErr == nil && exists && reader != nil {
			defer reader.Close()
			c.Set(fiber.HeaderContentType, MediaTypeOctetStream)
			c.Set(DockerDigestHeader, digest)
			c.Set(fiber.HeaderETag, fmt.Sprintf(`"%s"`, digest))
			c.Set(fiber.HeaderContentLength, strconv.FormatInt(size, 10))
			if c.Method() == fiber.MethodHead {
				return c.SendStatus(fiber.StatusOK)
			}
			return c.SendStream(reader, int(size))
		}
	}

	upstreamRc, uSize, uErr := FetchUpstreamBlob(c.Context(), state, repo, imageName, digest)
	if uErr == nil && upstreamRc != nil {
		defer upstreamRc.Close()
		c.Set(fiber.HeaderContentType, MediaTypeOctetStream)
		c.Set(DockerDigestHeader, digest)
		c.Set(fiber.HeaderETag, fmt.Sprintf(`"%s"`, digest))
		c.Set(fiber.HeaderContentLength, strconv.FormatInt(uSize, 10))

		if c.Method() == fiber.MethodHead {
			return c.SendStatus(fiber.StatusOK)
		}

		mirrorPersist, _ := repo.GetCacheConfig()
		if mirrorPersist {
			uploadUUID := uuid.NewString()
			staged, sErr := h.Store.StageBlob(repoName, uploadUUID)
			if sErr == nil && staged != nil {
				tee := io.TeeReader(upstreamRc, staged)
				err := c.SendStream(io.NopCloser(tee), int(uSize))
				_ = staged.Close()
				if err == nil {
					committedSize, cErr := h.Store.CommitBlob(state, repoName, uploadUUID, digest)
					if cErr == nil {
						_ = db.RecordDockerBlob(repoName, digest, committedSize)
					}
				} else {
					_ = staged.Discard()
				}
				return err
			}
		}

		return c.SendStream(upstreamRc, int(uSize))
	}

	return RespondError(c, fiber.StatusNotFound, ErrCodeBlobUnknown, "blob not found", map[string]string{"digest": digest})
}

// HandleDeleteBlob handles DELETE /v2/<name>/blobs/<digest>
func (h *Handler) HandleDeleteBlob(c fiber.Ctx, state *core.AppState) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	name := getParam(c, "name")
	digest := getParam(c, "digest")

	repoName, _ := ParseRepositoryAndImage(name)
	repo, ok := h.getRepo(state, repoName)
	if !ok {
		return RespondError(c, fiber.StatusNotFound, ErrCodeNameUnknown, "repository not found", nil)
	}

	if _, err := h.authenticateAndAuthorize(c, state, repo, name, true); err != nil {
		return nil
	}

	db := state.GetDB()
	if db != nil {
		_ = db.DeleteDockerBlob(repoName, digest)
	}
	_ = h.Store.DeleteBlob(state, repoName, digest)

	logDockerAudit(c, state, audit.ActionDockerBlobDelete, fmt.Sprintf("Repository: %s, digest: %s", repoName, digest))

	return c.SendStatus(fiber.StatusAccepted)
}

// HandlePostUpload handles POST /v2/<name>/blobs/uploads/
func (h *Handler) HandlePostUpload(c fiber.Ctx, state *core.AppState) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	name := getParam(c, "name")
	repoName, imageName := ParseRepositoryAndImage(name)
	repo, ok := h.getRepo(state, repoName)
	if !ok {
		return RespondError(c, fiber.StatusNotFound, ErrCodeNameUnknown, "repository not found", nil)
	}

	user, err := h.authenticateAndAuthorize(c, state, repo, name, true)
	if err != nil {
		return nil
	}

	mountDigest := c.Query("mount")
	fromRepo := c.Query("from")
	if mountDigest != "" && fromRepo != "" {
		fromRepoName, fromImageName := ParseRepositoryAndImage(fromRepo)
		fromRepository, sourceExists := h.getRepo(state, fromRepoName)
		canReadSource := sourceExists && strings.Contains(strings.Trim(fromRepo, "/"), "/") &&
			CanReadDocker(state, user, fromRepository, fromRepo)
		db := state.GetDB()
		if db == nil {
			return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
		}
		sourceReferencesBlob := false
		if canReadSource {
			var referenceErr error
			sourceReferencesBlob, referenceErr = db.DockerImageReferencesBlob(fromRepoName, fromImageName, mountDigest)
			if referenceErr != nil {
				return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
			}
		}
		exists, _, _ := h.Store.BlobExists(fromRepoName, mountDigest)
		if canReadSource && sourceReferencesBlob && exists {
			_, size, _ := h.Store.BlobExists(fromRepoName, mountDigest)
			if err := db.RecordDockerBlob(repoName, mountDigest, size); err != nil {
				return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported, "failed to record mounted blob", nil)
			}
			if err := db.RecordDockerImageBlob(repoName, imageName, mountDigest); err != nil {
				return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported, "failed to link mounted blob", nil)
			}
			logDockerAudit(c, state, audit.ActionDockerBlobMount, fmt.Sprintf("Repository: %s, image: %s, digest: %s, from: %s", repoName, imageName, mountDigest, fromRepoName))
			c.Set(DockerDigestHeader, mountDigest)
			c.Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", name, mountDigest))
			return c.SendStatus(fiber.StatusCreated)
		}
	}

	uploadUUID := uuid.NewString()
	staged, err := h.Store.StageBlob(repoName, uploadUUID)
	if err != nil {
		return RespondError(c, fiber.StatusInternalServerError, ErrCodeBlobUploadInvalid, "failed to start upload", nil)
	}

	singleDigest := c.Query("digest")
	body := c.Body()
	if singleDigest != "" && len(body) > 0 {
		defer staged.Discard()
		calculated := CalculateDigest(body)
		if calculated != singleDigest {
			return RespondError(c, fiber.StatusBadRequest, ErrCodeDigestInvalid, "digest mismatch", nil)
		}
		if _, err := staged.Write(body); err != nil {
			return RespondError(c, fiber.StatusInternalServerError, ErrCodeBlobUploadInvalid, "write failed", nil)
		}
		_ = staged.Close()
		committedSize, err := h.Store.CommitBlob(state, repoName, uploadUUID, singleDigest)
		if err != nil {
			return RespondError(c, fiber.StatusInternalServerError, ErrCodeBlobUploadInvalid, "commit failed", nil)
		}
		db := state.GetDB()
		if db == nil {
			return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
		}
		if err := db.RecordDockerBlob(repoName, singleDigest, committedSize); err != nil {
			return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported, "failed to record blob", nil)
		}
		if err := db.RecordDockerImageBlob(repoName, imageName, singleDigest); err != nil {
			return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported, "failed to link blob", nil)
		}
		logDockerAudit(c, state, audit.ActionDockerBlobUpload, fmt.Sprintf("Repository: %s, image: %s, digest: %s, size: %d", repoName, imageName, singleDigest, committedSize))
		c.Set(DockerDigestHeader, singleDigest)
		c.Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", name, singleDigest))
		return c.SendStatus(fiber.StatusCreated)
	}

	_ = staged.Close()
	c.Set(DockerUploadUUID, uploadUUID)
	c.Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", name, uploadUUID))
	c.Set("Range", "0-0")
	return c.SendStatus(fiber.StatusAccepted)
}

// HandlePatchUpload handles PATCH /v2/<name>/blobs/uploads/<uuid>
func (h *Handler) HandlePatchUpload(c fiber.Ctx, state *core.AppState) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	name := getParam(c, "name")
	uploadUUID := getParam(c, "uuid")

	repoName, _ := ParseRepositoryAndImage(name)
	repo, ok := h.getRepo(state, repoName)
	if !ok {
		return RespondError(c, fiber.StatusNotFound, ErrCodeNameUnknown, "repository not found", nil)
	}

	if _, err := h.authenticateAndAuthorize(c, state, repo, name, true); err != nil {
		return nil
	}

	staged, err := h.Store.GetStagedBlob(repoName, uploadUUID)
	if err != nil {
		return RespondError(c, fiber.StatusNotFound, ErrCodeBlobUploadUnknown, "upload session not found", nil)
	}
	defer staged.Close()

	body := c.Body()
	currentSize, _ := staged.Size()

	if len(body) > 0 {
		if _, err := staged.WriteAt(body, currentSize); err != nil {
			return RespondError(c, fiber.StatusInternalServerError, ErrCodeBlobUploadInvalid, "failed to append chunk", nil)
		}
		currentSize += int64(len(body))
	}

	c.Set(DockerUploadUUID, uploadUUID)
	c.Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", name, uploadUUID))
	c.Set("Range", fmt.Sprintf("0-%d", currentSize))
	return c.SendStatus(fiber.StatusAccepted)
}

// HandlePutUpload handles PUT /v2/<name>/blobs/uploads/<uuid>?digest=sha256:...
func (h *Handler) HandlePutUpload(c fiber.Ctx, state *core.AppState) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	name := getParam(c, "name")
	uploadUUID := getParam(c, "uuid")
	digest := c.Query("digest")

	if digest == "" {
		return RespondError(c, fiber.StatusBadRequest, ErrCodeDigestInvalid, "missing digest query parameter", nil)
	}

	repoName, imageName := ParseRepositoryAndImage(name)
	repo, ok := h.getRepo(state, repoName)
	if !ok {
		return RespondError(c, fiber.StatusNotFound, ErrCodeNameUnknown, "repository not found", nil)
	}

	if _, err := h.authenticateAndAuthorize(c, state, repo, name, true); err != nil {
		return nil
	}

	staged, err := h.Store.GetStagedBlob(repoName, uploadUUID)
	if err != nil {
		return RespondError(c, fiber.StatusNotFound, ErrCodeBlobUploadUnknown, "upload session not found", nil)
	}

	body := c.Body()
	if len(body) > 0 {
		currentSize, _ := staged.Size()
		if _, err := staged.WriteAt(body, currentSize); err != nil {
			_ = staged.Discard()
			return RespondError(c, fiber.StatusInternalServerError, ErrCodeBlobUploadInvalid, "failed to write final chunk", nil)
		}
	}

	_ = staged.Close()

	calculatedDigest, err := staged.Digest()
	if err != nil {
		_ = staged.Discard()
		return RespondError(c, fiber.StatusInternalServerError, ErrCodeBlobUploadInvalid, "failed to compute digest", nil)
	}

	if calculatedDigest != digest {
		_ = staged.Discard()
		return RespondError(c, fiber.StatusBadRequest, ErrCodeDigestInvalid, "computed digest does not match expected", nil)
	}

	committedSize, err := h.Store.CommitBlob(state, repoName, uploadUUID, digest)
	if err != nil {
		return RespondError(c, fiber.StatusInternalServerError, ErrCodeBlobUploadInvalid, "failed to commit blob", nil)
	}

	db := state.GetDB()
	if db == nil {
		return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "database unavailable", nil)
	}
	if err := db.RecordDockerBlob(repoName, digest, committedSize); err != nil {
		return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported, "failed to record blob", nil)
	}
	if err := db.RecordDockerImageBlob(repoName, imageName, digest); err != nil {
		return RespondError(c, fiber.StatusInternalServerError, ErrCodeUnsupported, "failed to link blob", nil)
	}

	logDockerAudit(c, state, audit.ActionDockerBlobUpload, fmt.Sprintf("Repository: %s, image: %s, digest: %s, size: %d", repoName, imageName, digest, committedSize))

	c.Set(DockerDigestHeader, digest)
	c.Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", name, digest))
	return c.SendStatus(fiber.StatusCreated)
}

// HandleGetUploadStatus handles GET /v2/<name>/blobs/uploads/<uuid>
func (h *Handler) HandleGetUploadStatus(c fiber.Ctx, state *core.AppState) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	name := getParam(c, "name")
	uploadUUID := getParam(c, "uuid")

	repoName, _ := ParseRepositoryAndImage(name)
	repo, ok := h.getRepo(state, repoName)
	if !ok {
		return RespondError(c, fiber.StatusNotFound, ErrCodeNameUnknown, "repository not found", nil)
	}

	if _, err := h.authenticateAndAuthorize(c, state, repo, name, true); err != nil {
		return nil
	}

	staged, err := h.Store.GetStagedBlob(repoName, uploadUUID)
	if err != nil {
		return RespondError(c, fiber.StatusNotFound, ErrCodeBlobUploadUnknown, "upload session not found", nil)
	}
	defer staged.Close()

	size, _ := staged.Size()
	c.Set(DockerUploadUUID, uploadUUID)
	c.Set("Range", fmt.Sprintf("0-%d", size))
	return c.SendStatus(fiber.StatusNoContent)
}

// HandleDeleteUpload handles DELETE /v2/<name>/blobs/uploads/<uuid>
func (h *Handler) HandleDeleteUpload(c fiber.Ctx, state *core.AppState) error {
	c.Set(DockerHeaderVersion, DockerVersionValue)
	name := getParam(c, "name")
	uploadUUID := getParam(c, "uuid")

	repoName, _ := ParseRepositoryAndImage(name)
	repo, ok := h.getRepo(state, repoName)
	if !ok {
		return RespondError(c, fiber.StatusNotFound, ErrCodeNameUnknown, "repository not found", nil)
	}

	if _, err := h.authenticateAndAuthorize(c, state, repo, name, true); err != nil {
		return nil
	}

	staged, err := h.Store.GetStagedBlob(repoName, uploadUUID)
	if err == nil && staged != nil {
		_ = staged.Discard()
	}

	return c.SendStatus(fiber.StatusNoContent)
}
