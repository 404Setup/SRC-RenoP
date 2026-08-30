/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
)

var dockerTagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

type dockerPublicationReviewPayload struct {
	Repository   string   `json:"repository"`
	ImageName    string   `json:"image_name"`
	Reference    string   `json:"reference"`
	Digest       string   `json:"digest"`
	MediaType    string   `json:"media_type"`
	ConfigDigest string   `json:"config_digest,omitempty"`
	BlobDigests  []string `json:"blob_digests,omitempty"`
	RawJSON      []byte   `json:"raw_json"`
	Size         int64    `json:"size"`
	CreatedAt    int64    `json:"created_at"`
}

type dockerImageCreationReviewPayload struct {
	Repository      string `json:"repository"`
	ImageName       string `json:"image_name"`
	SuperTeamPrefix string `json:"super_team_prefix,omitempty"`
	Private         bool   `json:"private"`
	CreatedAt       int64  `json:"created_at"`
}

func normalizeManifestReference(reference, digest string) (string, string, bool) {
	reference = strings.TrimSpace(reference)
	if strings.HasPrefix(reference, "sha256:") {
		if reference != digest || len(reference) != 71 {
			return "", "", false
		}
		if _, err := hex.DecodeString(reference[len("sha256:"):]); err != nil {
			return "", "", false
		}
		return "", reference, true
	}
	if !dockerTagPattern.MatchString(reference) {
		return "", "", false
	}
	return reference, reference, true
}

func dockerReviewFilePath(repository, imageName, reference string) string {
	digest := sha256.Sum256([]byte(repository + "\x00" + imageName + "\x00" + reference))
	name := strings.NewReplacer("/", "-", `\`, "-", ":", "-").Replace(imageName)
	if len(name) > 80 {
		name = name[:80]
	}
	return "review-manifests/" + name + "-" + hex.EncodeToString(digest[:8]) + ".json"
}

func dockerCreationReviewFilePath(repository, imageName string) string {
	digest := sha256.Sum256([]byte(repository + "\x00" + imageName + "\x00create"))
	return "review-requests/docker/" + hex.EncodeToString(digest[:]) + ".json"
}

func dockerCreationReviewPayload(state *core.AppState,
	task *core.ReviewTask,
) (*dockerImageCreationReviewPayload, []byte, error) {
	if state == nil || state.GetDB() == nil || task == nil || task.Kind != core.ReviewKindPublication ||
		task.ResourceType != core.ReviewResourceDockerImage ||
		task.ResourceVersion != core.ReviewVersionPackageCreation {
		return nil, nil, core.ErrReviewInvalidRequest
	}
	raw, err := state.GetDB().GetReviewTaskPayload(task.ID)
	if err != nil {
		return nil, nil, err
	}
	var payload dockerImageCreationReviewPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, core.ErrReviewInvalidRequest
	}
	imageName, valid := NormalizeImageName(payload.ImageName)
	teamPrefix := payload.SuperTeamPrefix
	if teamPrefix != "" {
		var teamValid bool
		teamPrefix, teamValid = core.NormalizeSuperTeamPrefix(teamPrefix)
		if !teamValid {
			return nil, nil, core.ErrReviewInvalidRequest
		}
	}
	if !valid || payload.Repository != task.Repository || imageName != task.ResourceKey || payload.CreatedAt <= 0 {
		return nil, nil, core.ErrReviewInvalidRequest
	}
	requiredPrefix, namespaced := core.DockerImageSuperTeamPrefix(imageName)
	if strings.Contains(imageName, "/") && !namespaced || namespaced && teamPrefix != requiredPrefix {
		return nil, nil, core.ErrReviewInvalidRequest
	}
	payload.ImageName = imageName
	payload.SuperTeamPrefix = teamPrefix
	return &payload, raw, nil
}

// QueueImageCreationReview creates a moderator task without reserving the Docker image name.
func QueueImageCreationReview(state *core.AppState, repo *config.Repository, imageName,
	superTeamPrefix, publisher string, private bool, createdAt int64,
) (*core.PublicationReviewResult, error) {
	if state == nil || state.GetDB() == nil || repo == nil || createdAt <= 0 {
		return nil, core.ErrDatabaseUnavailable
	}
	payload, err := json.Marshal(&dockerImageCreationReviewPayload{
		Repository: repo.Name, ImageName: imageName, SuperTeamPrefix: superTeamPrefix,
		Private: private, CreatedAt: createdAt,
	})
	if err != nil {
		return nil, err
	}
	return state.GetDB().CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: repo.Name,
		ResourceKey: imageName, ResourceName: imageName, Version: core.ReviewVersionPackageCreation,
		RequestedBy: publisher, Policy: repo.PublicationReviewPolicy(), Files: []*core.ReviewFile{{
			Path: dockerCreationReviewFilePath(repo.Name, imageName), Size: int64(len(payload)), Critical: true,
		}}, Payload: payload, CreatedAt: createdAt,
	})
}

func dockerReviewPayload(state *core.AppState, task *core.ReviewTask) (*dockerPublicationReviewPayload, error) {
	if state == nil || state.GetDB() == nil || task == nil || task.Kind != core.ReviewKindPublication ||
		task.ResourceType != core.ReviewResourceDockerImage {
		return nil, core.ErrReviewInvalidRequest
	}
	raw, err := state.GetDB().GetReviewTaskPayload(task.ID)
	if err != nil {
		return nil, err
	}
	var payload dockerPublicationReviewPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, core.ErrReviewInvalidRequest
	}
	imageName, valid := NormalizeImageName(payload.ImageName)
	parsed, parseErr := ParseManifest(payload.RawJSON, payload.MediaType)
	_, version, referenceValid := normalizeManifestReference(payload.Reference, payload.Digest)
	if !valid || parseErr != nil || !referenceValid || payload.Repository != task.Repository ||
		imageName != task.ResourceKey || version != task.ResourceVersion || payload.Digest != parsed.Digest ||
		payload.MediaType != parsed.MediaType || payload.ConfigDigest != parsed.ConfigDigest ||
		payload.Size != parsed.Size || payload.Size <= 0 || payload.CreatedAt <= 0 ||
		!slices.Equal(payload.BlobDigests, referencedBlobDigests(parsed)) {
		return nil, core.ErrReviewInvalidRequest
	}
	payload.ImageName = imageName
	return &payload, nil
}

// QueuePublicationReview stores one bounded virtual Docker manifest for moderator review.
func QueuePublicationReview(state *core.AppState, repo *config.Repository, imageName, reference string,
	parsed *ParsedManifest, publisher string, packageExists bool, createdAt int64,
) (*core.PublicationReviewResult, error) {
	if state == nil || state.GetDB() == nil || repo == nil || parsed == nil || createdAt <= 0 {
		return nil, core.ErrDatabaseUnavailable
	}
	_, version, valid := normalizeManifestReference(reference, parsed.Digest)
	if !valid {
		return nil, core.ErrReviewInvalidRequest
	}
	payload, err := json.Marshal(&dockerPublicationReviewPayload{
		Repository: repo.Name, ImageName: imageName, Reference: reference, Digest: parsed.Digest,
		MediaType: parsed.MediaType, ConfigDigest: parsed.ConfigDigest,
		BlobDigests: referencedBlobDigests(parsed), RawJSON: parsed.RawJSON,
		Size: parsed.Size, CreatedAt: createdAt,
	})
	if err != nil {
		return nil, err
	}
	return state.GetDB().CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceDockerImage, Repository: repo.Name,
		ResourceKey: imageName, ResourceName: imageName, Version: version,
		RequestedBy: publisher, Policy: repo.PublicationReviewPolicy(), PackageExists: packageExists,
		Files: []*core.ReviewFile{{
			Path: dockerReviewFilePath(repo.Name, imageName, reference), Size: parsed.Size, Critical: true,
		}},
		Payload: payload, CreatedAt: createdAt,
	})
}

// ServePublicationReviewManifest returns a validated virtual manifest to an authorized reviewer.
func ServePublicationReviewManifest(c fiber.Ctx, state *core.AppState, task *core.ReviewTask,
	file *core.ReviewFile,
) error {
	if task != nil && task.ResourceVersion == core.ReviewVersionPackageCreation {
		payload, raw, err := dockerCreationReviewPayload(state, task)
		if err != nil {
			return err
		}
		if file == nil || !file.Virtual || file.Path != dockerCreationReviewFilePath(
			payload.Repository, payload.ImageName) {
			return core.ErrReviewFileNotFound
		}
		c.Set(fiber.HeaderCacheControl, "private, no-store")
		c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSONCharsetUTF8)
		c.Set(fiber.HeaderContentDisposition, `attachment; filename="image-request.json"`)
		c.Set(fiber.HeaderContentLength, strconv.Itoa(len(raw)))
		return c.Send(raw)
	}
	payload, err := dockerReviewPayload(state, task)
	if err != nil {
		return err
	}
	if file == nil || !file.Virtual || file.Path != dockerReviewFilePath(
		payload.Repository, payload.ImageName, payload.Reference) {
		return core.ErrReviewFileNotFound
	}
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	c.Set(fiber.HeaderContentType, payload.MediaType)
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="manifest.json"`)
	c.Set(fiber.HeaderContentLength, strconv.FormatInt(payload.Size, 10))
	return c.Send(payload.RawJSON)
}

// ApproveImageCreationReview rechecks local and upstream names before atomically reserving an image.
func ApproveImageCreationReview(ctx context.Context, state *core.AppState, task *core.ReviewTask,
	reviewer string, decidedAt int64,
) (*core.ReviewTask, error) {
	payload, _, err := dockerCreationReviewPayload(state, task)
	if err != nil {
		return nil, err
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repo := cfg.Maven.Repositories[payload.Repository]
	if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatDocker {
		return nil, core.ErrReviewResourceConflict
	}
	exists, _, _, _, _, err := state.GetDB().GetDockerImageAccess(
		payload.Repository, payload.ImageName, "")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, core.ErrReviewResourceConflict
	}
	if len(repo.Mirrors) > 0 {
		upstreamExists, probeErr := UpstreamImageExists(ctx, state, repo, payload.ImageName)
		if probeErr != nil {
			return nil, probeErr
		}
		if upstreamExists {
			return nil, core.ErrReviewResourceConflict
		}
	}
	return state.GetDB().ApproveDockerImageCreationReview(
		task.ID, reviewer, payload.Repository, payload.ImageName, payload.SuperTeamPrefix,
		payload.Private, payload.CreatedAt, decidedAt)
}

// ApprovePublicationReview persists one reviewed manifest and atomically completes its task.
func ApprovePublicationReview(state *core.AppState, task *core.ReviewTask, store Store,
	reviewer string, decidedAt int64,
) (*core.ReviewTask, error) {
	if state == nil || state.GetDB() == nil || store == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	payload, err := dockerReviewPayload(state, task)
	if err != nil {
		return nil, err
	}
	for _, digest := range payload.BlobDigests {
		recorded, _, recordErr := state.GetDB().HasDockerBlob(payload.Repository, digest)
		stored, _, storeErr := store.BlobExists(payload.Repository, digest)
		linked, linkErr := state.GetDB().DockerImageReferencesBlob(
			payload.Repository, payload.ImageName, digest)
		if recordErr != nil || storeErr != nil || linkErr != nil || !recorded || !stored || !linked {
			return nil, errors.Join(core.ErrReviewResourceConflict, recordErr, storeErr, linkErr)
		}
	}
	previous, existed, err := store.OpenManifest(payload.Repository, payload.ImageName, payload.Digest)
	if err != nil {
		return nil, err
	}
	if existed && CalculateDigest(previous) != payload.Digest {
		return nil, core.ErrReviewResourceConflict
	}
	if err := store.PutManifest(state, payload.Repository, payload.ImageName,
		payload.Digest, payload.RawJSON); err != nil {
		return nil, err
	}
	tag, _, _ := normalizeManifestReference(payload.Reference, payload.Digest)
	decided, err := state.GetDB().ApproveDockerPublicationReview(task.ID, reviewer, &core.DockerManifest{
		Repository: payload.Repository, ImageName: payload.ImageName, Digest: payload.Digest,
		MediaType: payload.MediaType, Size: payload.Size, ConfigDigest: payload.ConfigDigest,
		BlobDigests: payload.BlobDigests, RawJSON: payload.RawJSON, CreatedAt: payload.CreatedAt,
	}, tag, decidedAt)
	if err != nil && !existed {
		err = errors.Join(err, store.DeleteManifest(state, payload.Repository, payload.ImageName, payload.Digest))
	}
	return decided, err
}

// AddPendingPublicationTags appends pending tags to an authorized image-management response.
func AddPendingPublicationTags(state *core.AppState, details *core.DockerImageDetails) error {
	if state == nil || state.GetDB() == nil || details == nil || details.Image == nil {
		return core.ErrReviewInvalidRequest
	}
	tasks, err := state.GetDB().ListPublicationReviews(details.Image.Repository,
		core.ReviewResourceDockerImage, details.Image.ImageName)
	if err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(details.Tags))
	for _, tag := range details.Tags {
		if tag != nil {
			existing[tag.Tag] = struct{}{}
		}
	}
	for _, task := range tasks {
		if task == nil || task.Status != core.ReviewStatusPending {
			continue
		}
		payload, err := dockerReviewPayload(state, task)
		if err != nil {
			return err
		}
		if _, found := existing[payload.Reference]; found {
			continue
		}
		details.Tags = append(details.Tags, &core.DockerTag{
			Repository: payload.Repository, ImageName: payload.ImageName, Tag: payload.Reference,
			Digest: payload.Digest, MediaType: payload.MediaType, Size: payload.Size,
			ConfigDigest: payload.ConfigDigest, Publisher: task.RequestedBy,
			CreatedAt: payload.CreatedAt, UpdatedAt: task.UpdatedAt,
			ReviewStatus: task.Status, ReviewID: task.ID,
		})
		existing[payload.Reference] = struct{}{}
	}
	slices.SortFunc(details.Tags, func(left, right *core.DockerTag) int {
		if left.UpdatedAt != right.UpdatedAt {
			if left.UpdatedAt > right.UpdatedAt {
				return -1
			}
			return 1
		}
		return strings.Compare(left.Tag, right.Tag)
	})
	return nil
}
