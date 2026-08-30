/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"golang.org/x/mod/semver"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/reviewnotify"
)

type npmPublicationReviewPayload struct {
	Repository   string            `json:"repository"`
	PackageName  string            `json:"package_name"`
	Description  string            `json:"description"`
	Version      string            `json:"version"`
	ManifestJSON string            `json:"manifest_json"`
	Publisher    string            `json:"publisher"`
	TarballPath  string            `json:"tarball_path"`
	Shasum       string            `json:"shasum"`
	Integrity    string            `json:"integrity"`
	Size         int64             `json:"size"`
	Deprecated   string            `json:"deprecated,omitempty"`
	CreatedAt    int64             `json:"created_at"`
	UpdatedAt    int64             `json:"updated_at"`
	Tags         map[string]string `json:"tags"`
}

type npmPackageCreationReviewPayload struct {
	Repository      string `json:"repository"`
	PackageName     string `json:"package_name"`
	SuperTeamPrefix string `json:"super_team_prefix,omitempty"`
	Private         bool   `json:"private"`
	CreatedAt       int64  `json:"created_at"`
}

func npmCreationReviewFilePath(repository, packageName string) string {
	digest := sha256.Sum256([]byte(repository + "\x00" + packageName + "\x00create"))
	return "review-requests/npm/" + hex.EncodeToString(digest[:]) + ".json"
}

func npmCreationReviewPayload(state *core.AppState,
	task *core.ReviewTask,
) (*npmPackageCreationReviewPayload, []byte, error) {
	if state == nil || state.GetDB() == nil || task == nil || task.Kind != core.ReviewKindPublication ||
		task.ResourceType != core.ReviewResourceNPMPackage ||
		task.ResourceVersion != core.ReviewVersionPackageCreation {
		return nil, nil, core.ErrReviewInvalidRequest
	}
	raw, err := state.GetDB().GetReviewTaskPayload(task.ID)
	if err != nil {
		return nil, nil, err
	}
	var payload npmPackageCreationReviewPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, core.ErrReviewInvalidRequest
	}
	packageName, valid := NormalizePackageName(payload.PackageName)
	teamPrefix := payload.SuperTeamPrefix
	if teamPrefix != "" {
		var teamValid bool
		teamPrefix, teamValid = core.NormalizeSuperTeamPrefix(teamPrefix)
		if !teamValid {
			return nil, nil, core.ErrReviewInvalidRequest
		}
	}
	if !valid || payload.Repository != task.Repository || packageName != task.ResourceKey || payload.CreatedAt <= 0 ||
		payload.Private && !strings.HasPrefix(packageName, "@") {
		return nil, nil, core.ErrReviewInvalidRequest
	}
	requiredPrefix, scoped := core.NPMPackageSuperTeamPrefix(packageName)
	if scoped && teamPrefix != requiredPrefix {
		return nil, nil, core.ErrReviewInvalidRequest
	}
	payload.PackageName = packageName
	payload.SuperTeamPrefix = teamPrefix
	return &payload, raw, nil
}

// QueuePackageCreationReview creates a moderator task without reserving the npm package name.
func QueuePackageCreationReview(state *core.AppState, repo *config.Repository, packageName,
	superTeamPrefix, reviewTeamPrefix, publisher string, private bool, createdAt int64,
) (*core.PublicationReviewResult, error) {
	if state == nil || state.GetDB() == nil || repo == nil || createdAt <= 0 {
		return nil, core.ErrDatabaseUnavailable
	}
	payload, err := json.Marshal(&npmPackageCreationReviewPayload{
		Repository: repo.Name, PackageName: packageName, SuperTeamPrefix: superTeamPrefix,
		Private: private, CreatedAt: createdAt,
	})
	if err != nil {
		return nil, err
	}
	policy := repo.PublicationReviewPolicy()
	if reviewTeamPrefix != "" && policy == config.PublicationReviewOff {
		policy = config.PublicationReviewNewPackages
	}
	result, err := state.GetDB().CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceNPMPackage, Repository: repo.Name,
		ResourceKey: packageName, ResourceName: packageName, Version: core.ReviewVersionPackageCreation,
		RequestedBy: publisher, Policy: policy, ReviewTeamPrefix: reviewTeamPrefix,
		TargetTeamPrefix: reviewTeamPrefix, Files: []*core.ReviewFile{{
			Path: npmCreationReviewFilePath(repo.Name, packageName), Size: int64(len(payload)), Critical: true,
		}}, Payload: payload, CreatedAt: createdAt,
	})
	if err == nil {
		reviewnotify.DeliverPending(state, result)
	}
	return result, err
}

// ServePackageCreationReview returns one validated virtual npm reservation request.
func ServePackageCreationReview(c fiber.Ctx, state *core.AppState, task *core.ReviewTask,
	file *core.ReviewFile,
) error {
	payload, raw, err := npmCreationReviewPayload(state, task)
	if err != nil {
		return err
	}
	if file == nil || !file.Virtual || file.Path != npmCreationReviewFilePath(
		payload.Repository, payload.PackageName) {
		return core.ErrReviewFileNotFound
	}
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSONCharsetUTF8)
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="package-request.json"`)
	c.Set(fiber.HeaderContentLength, strconv.Itoa(len(raw)))
	return c.Send(raw)
}

// ApprovePackageCreationReview atomically reserves an approved npm package.
func ApprovePackageCreationReview(state *core.AppState, task *core.ReviewTask,
	reviewer string, decidedAt int64,
) (*core.ReviewTask, error) {
	payload, _, err := npmCreationReviewPayload(state, task)
	if err != nil {
		return nil, err
	}
	existing, err := state.GetDB().GetNPMPackage(payload.Repository, payload.PackageName)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, core.ErrReviewResourceConflict
	}
	return state.GetDB().ApproveNPMPackageCreationReview(
		task.ID, reviewer, payload.Repository, payload.PackageName, payload.SuperTeamPrefix,
		payload.Private, payload.CreatedAt, decidedAt)
}

func validateNPMReviewPayload(task *core.ReviewTask, payload *npmPublicationReviewPayload) error {
	if task == nil || payload == nil || task.ResourceType != core.ReviewResourceNPMPackage {
		return core.ErrReviewInvalidRequest
	}
	packageName, valid := NormalizePackageName(payload.PackageName)
	if !valid || packageName != task.ResourceKey || payload.Repository != task.Repository ||
		payload.Version != task.ResourceVersion || !validNPMVersion(payload.Version) ||
		!strings.EqualFold(payload.Publisher, task.RequestedBy) || payload.Size < 0 || payload.CreatedAt <= 0 ||
		payload.TarballPath != canonicalTarballPath(packageName, payload.Version) ||
		len(payload.ManifestJSON) == 0 || len(payload.ManifestJSON) > maxStoredManifestJSON {
		return core.ErrReviewInvalidRequest
	}
	for tag, version := range payload.Tags {
		if !validNPMTag(tag) || version != task.ResourceVersion {
			return core.ErrReviewInvalidRequest
		}
	}
	return nil
}

func npmReviewPayload(state *core.AppState, task *core.ReviewTask) (*npmPublicationReviewPayload, error) {
	if state == nil || state.GetDB() == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	raw, err := state.GetDB().GetReviewTaskPayload(task.ID)
	if err != nil {
		return nil, err
	}
	var payload npmPublicationReviewPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, core.ErrReviewInvalidRequest
	}
	if err := validateNPMReviewPayload(task, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// QueuePublicationReview creates one hidden npm version task after its tarball commits.
func QueuePublicationReview(state *core.AppState, repo *config.Repository, pkg *core.NPMPackage,
	version *core.NPMVersion, tags map[string]string, packageExists bool,
) (*core.PublicationReviewResult, error) {
	if state == nil || state.GetDB() == nil || repo == nil || pkg == nil || version == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	payload, err := json.Marshal(&npmPublicationReviewPayload{
		Repository: repo.Name, PackageName: pkg.Name, Description: pkg.Description,
		Version: version.Version, ManifestJSON: version.ManifestJSON, Publisher: version.Publisher,
		TarballPath: version.TarballPath, Shasum: version.Shasum, Integrity: version.Integrity,
		Size: version.Size, Deprecated: version.Deprecated, CreatedAt: version.CreatedAt,
		UpdatedAt: pkg.UpdatedAt, Tags: tags,
	})
	if err != nil {
		return nil, err
	}
	result, err := state.GetDB().CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceNPMPackage, Repository: repo.Name,
		ResourceKey: pkg.Name, ResourceName: pkg.Name, Version: version.Version,
		RequestedBy: version.Publisher, Policy: repo.PublicationReviewPolicy(), PackageExists: packageExists,
		Files:   []*core.ReviewFile{{Path: version.TarballPath, Size: version.Size, Critical: true}},
		Payload: payload, CreatedAt: version.CreatedAt,
	})
	if err == nil {
		reviewnotify.DeliverPending(state, result)
	}
	return result, err
}

// ApprovePublicationReview records one reviewed npm version and its dist-tags.
func ApprovePublicationReview(state *core.AppState, task *core.ReviewTask) error {
	payload, err := npmReviewPayload(state, task)
	if err != nil {
		return err
	}
	return state.GetDB().RecordNPMPublication(&core.NPMPackage{
		Repository: payload.Repository, Name: payload.PackageName,
		Description: payload.Description, UpdatedAt: payload.UpdatedAt,
	}, &core.NPMVersion{
		Repository: payload.Repository, Package: payload.PackageName, Version: payload.Version,
		ManifestJSON: payload.ManifestJSON, Publisher: payload.Publisher, TarballPath: payload.TarballPath,
		Shasum: payload.Shasum, Integrity: payload.Integrity, Size: payload.Size,
		Deprecated: payload.Deprecated, CreatedAt: payload.CreatedAt,
	}, payload.Tags, payload.Publisher)
}

// RemoveApprovedPublicationMetadata removes npm metadata inserted before a failed task decision.
func RemoveApprovedPublicationMetadata(state *core.AppState, task *core.ReviewTask,
	previous *core.NPMPackage, previousTags map[string]string,
) error {
	if state == nil || state.GetDB() == nil || task == nil || previous == nil {
		return core.ErrReviewInvalidRequest
	}
	err := state.GetDB().RollbackNPMPublicationReview(task.Repository, task.ResourceKey,
		task.ResourceVersion, previous, previousTags)
	if errors.Is(err, core.ErrNPMVersionNotFound) {
		return nil
	}
	return err
}

// AddPendingPublicationVersions adds review-only versions to an authorized package-management response.
func AddPendingPublicationVersions(state *core.AppState, details *core.NPMPackageDetails) error {
	if state == nil || state.GetDB() == nil || details == nil || details.Package == nil {
		return core.ErrReviewInvalidRequest
	}
	tasks, err := state.GetDB().ListPublicationReviews(details.Package.Repository,
		core.ReviewResourceNPMPackage, details.Package.Name)
	if err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(details.Versions))
	for _, version := range details.Versions {
		if version != nil {
			existing[version.Version] = struct{}{}
		}
	}
	for _, task := range tasks {
		if task == nil || task.Status != core.ReviewStatusPending {
			continue
		}
		if _, exists := existing[task.ResourceVersion]; exists {
			continue
		}
		payload, err := npmReviewPayload(state, task)
		if err != nil {
			return err
		}
		version := &core.NPMVersion{
			Repository: payload.Repository, Package: payload.PackageName, Version: payload.Version,
			ManifestJSON: payload.ManifestJSON, Publisher: payload.Publisher, TarballPath: payload.TarballPath,
			Shasum: payload.Shasum, Integrity: payload.Integrity, Size: payload.Size,
			Deprecated: payload.Deprecated, CreatedAt: payload.CreatedAt,
			ReviewStatus: task.Status, ReviewID: task.ID,
		}
		details.Versions = append(details.Versions, version)
		existing[version.Version] = struct{}{}
		details.Package.VersionCount++
	}
	slices.SortFunc(details.Versions, func(left, right *core.NPMVersion) int {
		return -semver.Compare("v"+left.Version, "v"+right.Version)
	})
	return nil
}
