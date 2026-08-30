/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/goccy/go-json"
	"golang.org/x/mod/semver"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/reviewnotify"
	"renop/internal/utils"
)

type cargoPublicationReviewPayload struct {
	Package *core.CargoPackage `json:"package"`
	Version *core.CargoVersion `json:"version"`
	Entry   IndexEntry         `json:"entry"`
}

func pendingPublicationReservation(state *core.AppState, repository, normalizedName,
	username string,
) (string, bool, error) {
	if state == nil || state.GetDB() == nil {
		return "", false, core.ErrDatabaseUnavailable
	}
	tasks, err := state.GetDB().ListPublicationReviews(
		repository, core.ReviewResourceCargoPackage, normalizedName)
	if err != nil {
		return "", false, err
	}
	pending := false
	for _, task := range tasks {
		if task != nil && task.Status == core.ReviewStatusPending {
			pending = true
			break
		}
	}
	if !pending {
		return "", false, nil
	}
	profile, err := state.GetDB().GetUserProfile(username)
	if err != nil || profile == nil || profile.UserID == "" {
		return "", true, errors.Join(core.ErrCargoPermissionDenied, err)
	}
	packageName := ""
	for _, task := range tasks {
		if task == nil || task.Status != core.ReviewStatusPending {
			continue
		}
		if task.RequestedByID != profile.UserID {
			return "", true, core.ErrCargoPermissionDenied
		}
		payload, err := cargoReviewPayload(state, task)
		if err != nil {
			return "", true, err
		}
		if packageName == "" {
			packageName = payload.Package.Name
		} else if packageName != payload.Package.Name {
			return "", true, core.ErrReviewResourceConflict
		}
	}
	return packageName, packageName != "", nil
}

func cargoReviewPayload(state *core.AppState, task *core.ReviewTask) (*cargoPublicationReviewPayload, error) {
	if state == nil || state.GetDB() == nil || task == nil || task.ResourceType != core.ReviewResourceCargoPackage {
		return nil, core.ErrReviewInvalidRequest
	}
	raw, err := state.GetDB().GetReviewTaskPayload(task.ID)
	if err != nil {
		return nil, err
	}
	var payload cargoPublicationReviewPayload
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Package == nil || payload.Version == nil {
		return nil, core.ErrReviewInvalidRequest
	}
	normalizedName, valid := NormalizeCrateName(payload.Package.Name)
	if !valid || normalizedName != task.ResourceKey || payload.Package.Repository != task.Repository ||
		payload.Version.Version != task.ResourceVersion || payload.Entry.Version != task.ResourceVersion ||
		!strings.EqualFold(payload.Entry.Name, payload.Package.Name) ||
		!strings.EqualFold(payload.Version.Publisher, task.RequestedBy) ||
		payload.Entry.Checksum == "" || payload.Entry.Checksum != payload.Version.Checksum ||
		payload.Version.Size < 0 || payload.Version.CreatedAt <= 0 {
		return nil, core.ErrReviewInvalidRequest
	}
	payload.Package.NormalizedName = normalizedName
	payload.Version.Repository = task.Repository
	payload.Version.Package = normalizedName
	return &payload, nil
}

// QueuePublicationReview creates one hidden Cargo version task after the crate archive commits.
func QueuePublicationReview(state *core.AppState, repo *config.Repository, pkg *core.CargoPackage,
	version *core.CargoVersion, entry IndexEntry, cratePath string, packageExists bool,
) (*core.PublicationReviewResult, error) {
	if state == nil || state.GetDB() == nil || repo == nil || pkg == nil || version == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	payload, err := json.Marshal(&cargoPublicationReviewPayload{Package: pkg, Version: version, Entry: entry})
	if err != nil {
		return nil, err
	}
	result, err := state.GetDB().CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceCargoPackage, Repository: repo.Name,
		ResourceKey: pkg.NormalizedName, ResourceName: pkg.Name, Version: version.Version,
		RequestedBy: version.Publisher, Policy: repo.PublicationReviewPolicy(), PackageExists: packageExists,
		Files:   []*core.ReviewFile{{Path: cratePath, Size: version.Size, Critical: true}},
		Payload: payload, CreatedAt: version.CreatedAt,
	})
	if err == nil {
		reviewnotify.DeliverPending(state, result)
	}
	return result, err
}

// ApprovePublicationReview writes the sparse index and Cargo catalog before exposing the crate archive.
func ApprovePublicationReview(state *core.AppState, task *core.ReviewTask, store Store) (func() error, error) {
	if state == nil || state.Inner == nil || state.GetDB() == nil || store == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	payload, err := cargoReviewPayload(state, task)
	if err != nil {
		return nil, err
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repo := cfg.Maven.Repositories[task.Repository]
	if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatCargo {
		return nil, core.ErrReviewResourceConflict
	}
	previous, err := state.GetDB().GetCargoPackage(task.Repository, task.ResourceKey)
	if err != nil {
		return nil, err
	}
	indexFilePath := cargoIndexPath(cfg.StoragePath, repo, payload.Package.Name)
	if !utils.IsSubPath(cfg.StoragePath, indexFilePath) {
		return nil, core.ErrReviewInvalidRequest
	}
	reader, indexExisted, err := store.Open(indexFilePath)
	if err != nil {
		return nil, err
	}
	stage, err := store.Stage(indexFilePath)
	if err != nil {
		if reader != nil {
			_ = reader.Close()
		}
		return nil, err
	}
	defer stage.Discard()
	rewriteErr := rewriteIndex(reader, stage, payload.Entry)
	if reader != nil {
		if closeErr := reader.Close(); rewriteErr == nil {
			rewriteErr = closeErr
		}
	}
	if rewriteErr != nil {
		return nil, rewriteErr
	}
	if err := stage.Close(); err != nil {
		return nil, err
	}
	if err := stage.Commit(state); err != nil {
		return nil, err
	}
	if err := state.GetDB().RecordCargoPublication(payload.Package, payload.Version,
		payload.Version.Publisher); err != nil {
		return nil, errors.Join(err, rollbackCargoIndexVersion(
			state, store, indexFilePath, task.ResourceVersion, indexExisted))
	}
	rollback := func() error {
		metadataErr := state.GetDB().RollbackCargoPublicationReview(
			task.Repository, task.ResourceKey, task.ResourceVersion, previous)
		indexErr := rollbackCargoIndexVersion(state, store, indexFilePath, task.ResourceVersion, indexExisted)
		return errors.Join(metadataErr, indexErr)
	}
	return rollback, nil
}

// AddPendingPublicationVersions adds review-only versions to an authorized Cargo package response.
func AddPendingPublicationVersions(state *core.AppState, details *core.CargoPackageDetails) error {
	if state == nil || state.GetDB() == nil || details == nil || details.Package == nil {
		return core.ErrReviewInvalidRequest
	}
	tasks, err := state.GetDB().ListPublicationReviews(details.Package.Repository,
		core.ReviewResourceCargoPackage, details.Package.NormalizedName)
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
		payload, err := cargoReviewPayload(state, task)
		if err != nil {
			return err
		}
		version := *payload.Version
		version.ReviewStatus = task.Status
		version.ReviewID = task.ID
		details.Versions = append(details.Versions, &version)
		existing[version.Version] = struct{}{}
	}
	slices.SortFunc(details.Versions, func(left, right *core.CargoVersion) int {
		return -semver.Compare("v"+left.Version, "v"+right.Version)
	})
	return nil
}

// PendingPublicationPackageDetails returns a pending-only package to its requester or an authorized moderator.
func PendingPublicationPackageDetails(state *core.AppState, repository, normalizedName,
	username string, canModerate bool,
) (*core.CargoPackageDetails, error) {
	if state == nil || state.GetDB() == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	userID := ""
	if !canModerate {
		profile, err := state.GetDB().GetUserProfile(username)
		if err != nil || profile == nil || profile.UserID == "" {
			return nil, errors.Join(core.ErrCargoPackageNotFound, err)
		}
		userID = profile.UserID
	}
	tasks, err := state.GetDB().ListPublicationReviews(
		repository, core.ReviewResourceCargoPackage, normalizedName)
	if err != nil {
		return nil, err
	}
	details := &core.CargoPackageDetails{Members: []*core.CargoMember{}, Versions: []*core.CargoVersion{}}
	for _, task := range tasks {
		if task == nil || task.Status != core.ReviewStatusPending || !canModerate && task.RequestedByID != userID {
			continue
		}
		payload, err := cargoReviewPayload(state, task)
		if err != nil {
			return nil, err
		}
		if details.Package == nil {
			pkg := *payload.Package
			if task.RequestedByID == userID {
				pkg.PermissionLevel = core.CargoPermissionOwner
			}
			details.Package = &pkg
		}
		version := *payload.Version
		version.ReviewStatus = task.Status
		version.ReviewID = task.ID
		details.Versions = append(details.Versions, &version)
	}
	if details.Package == nil {
		return nil, core.ErrCargoPackageNotFound
	}
	slices.SortFunc(details.Versions, func(left, right *core.CargoVersion) int {
		return -semver.Compare("v"+left.Version, "v"+right.Version)
	})
	return details, nil
}

func cargoReviewCratePath(storagePath string, repo *config.Repository, packageName, version string) (string, string, bool) {
	absolute := filepath.Join(storagePath, repo.Name, "api", "v1", "crates", packageName, version, "download")
	root := filepath.Join(storagePath, repo.Name)
	relative, err := filepath.Rel(root, absolute)
	if err != nil || !utils.IsSubPath(storagePath, absolute) {
		return "", "", false
	}
	return absolute, filepath.ToSlash(relative), true
}
