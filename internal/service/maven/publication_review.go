/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/utils"
)

func publicationReviewCoordinate(files []*core.ReviewFile) (MavenCoordinate, bool) {
	for _, file := range files {
		if file == nil {
			continue
		}
		if coordinate, valid := ParseArtifactPath(file.Path); valid {
			return coordinate, true
		}
	}
	return MavenCoordinate{}, false
}

type mavenMetadataCandidate struct {
	resourceKey string
	version     string
}

func publicationMetadataCandidates(path string) []mavenMetadataCandidate {
	parts := strings.Split(strings.Trim(strings.ReplaceAll(path, `\`, "/"), "/"), "/")
	if len(parts) < 4 {
		return nil
	}
	name := strings.ToLower(parts[len(parts)-1])
	if name != "maven-metadata.xml" && !strings.HasPrefix(name, "maven-metadata.xml.") {
		return nil
	}
	for _, part := range parts[:len(parts)-1] {
		if !coordinatePartPattern.MatchString(part) {
			return nil
		}
	}
	result := make([]mavenMetadataCandidate, 0, 2)
	if len(parts) >= 5 {
		groupParts := parts[:len(parts)-3]
		if len(groupParts) >= 2 {
			result = append(result, mavenMetadataCandidate{
				resourceKey: strings.Join(groupParts, ".") + ":" + parts[len(parts)-3],
				version:     parts[len(parts)-2],
			})
		}
	}
	groupParts := parts[:len(parts)-2]
	if len(groupParts) >= 2 {
		result = append(result, mavenMetadataCandidate{
			resourceKey: strings.Join(groupParts, ".") + ":" + parts[len(parts)-2],
		})
	}
	return result
}

func pendingMetadataReview(state *core.AppState, repository, path string) (*core.ReviewTask, error) {
	var latest *core.ReviewTask
	for _, candidate := range publicationMetadataCandidates(path) {
		tasks, err := state.GetDB().ListPublicationReviews(repository,
			core.ReviewResourceMavenArtifact, candidate.resourceKey)
		if err != nil {
			return nil, err
		}
		for _, task := range tasks {
			if task == nil || task.Status != core.ReviewStatusPending ||
				candidate.version != "" && task.ResourceVersion != candidate.version {
				continue
			}
			if candidate.version != "" {
				return task, nil
			}
			if latest == nil || task.UpdatedAt > latest.UpdatedAt ||
				task.UpdatedAt == latest.UpdatedAt && task.CreatedAt > latest.CreatedAt {
				latest = task
			}
		}
	}
	return latest, nil
}

func mavenReviewFileCritical(path string, coordinate MavenCoordinate) bool {
	name := strings.ToLower(filepath.Base(path))
	for _, suffix := range []string{".md5", ".sha1", ".sha256", ".sha512", ".asc"} {
		if strings.HasSuffix(name, suffix) {
			return false
		}
	}
	prefix := strings.ToLower(coordinate.ArtifactID + "-" + coordinate.Version)
	return name == prefix+".pom" || name == prefix+".module" || name == prefix+".jar"
}

func primaryMavenReviewFile(files []*core.ReviewFile, coordinate MavenCoordinate) *core.ReviewFile {
	var fallback *core.ReviewFile
	for _, file := range files {
		if file == nil {
			continue
		}
		if _, valid := ParseArtifactPath(file.Path); !valid {
			continue
		}
		if fallback == nil || file.Size > fallback.Size {
			fallback = file
		}
		name := strings.ToLower(filepath.Base(file.Path))
		companion := false
		for _, suffix := range []string{".md5", ".sha1", ".sha256", ".sha512", ".asc"} {
			if strings.HasSuffix(name, suffix) {
				companion = true
				break
			}
		}
		if !companion {
			return file
		}
	}
	return fallback
}

// IsPublicationReviewCandidate reports whether a repository path belongs to a Maven package version.
func IsPublicationReviewCandidate(path string) bool {
	if _, valid := ParseArtifactPath(path); valid {
		return true
	}
	return len(publicationMetadataCandidates(path)) > 0
}

// ProcessPublishedFiles records visible Maven metadata or creates one hidden review task.
func ProcessPublishedFiles(state *core.AppState, repo *config.Repository, username string,
	files []*core.ReviewFile,
) (*core.PublicationReviewResult, error) {
	if state == nil || state.GetDB() == nil || repo == nil || len(files) == 0 {
		return nil, core.ErrDatabaseUnavailable
	}
	coordinate, valid := publicationReviewCoordinate(files)
	policy := repo.PublicationReviewPolicy()
	if policy == config.PublicationReviewOff {
		if !valid {
			return &core.PublicationReviewResult{}, nil
		}
		primary := primaryMavenReviewFile(files, coordinate)
		if primary == nil {
			return &core.PublicationReviewResult{}, nil
		}
		return &core.PublicationReviewResult{}, RecordPublishedPath(
			state, repo.Name, primary.Path, username, primary.Size, primary.AddedAt)
	}
	if !valid {
		pending, err := pendingMetadataReview(state, repo.Name, files[0].Path)
		if err != nil || pending == nil {
			return &core.PublicationReviewResult{}, err
		}
		return state.GetDB().CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
			ResourceType: pending.ResourceType, Repository: pending.Repository,
			ResourceKey: pending.ResourceKey, ResourceName: pending.ResourceName,
			Version: pending.ResourceVersion, RequestedBy: username, Policy: policy,
			PackageExists: true, Files: files, CreatedAt: time.Now().UnixMilli(),
		})
	}
	for _, file := range files {
		if file != nil {
			file.Critical = mavenReviewFileCritical(file.Path, coordinate)
		}
	}
	db := state.GetDB()
	packageExists, err := db.MavenArtifactExists(repo.Name, coordinate.GroupID, coordinate.ArtifactID)
	if err != nil {
		return nil, err
	}
	createdAt := time.Now().UnixMilli()
	for _, file := range files {
		if file != nil && file.AddedAt > 0 && file.AddedAt < createdAt {
			createdAt = file.AddedAt
		}
	}
	result, err := db.CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceMavenArtifact, Repository: repo.Name,
		ResourceKey:  coordinate.GroupID + ":" + coordinate.ArtifactID,
		ResourceName: coordinate.GroupID + ":" + coordinate.ArtifactID,
		Version:      coordinate.Version, RequestedBy: username, Policy: policy,
		PackageExists: packageExists, Files: files, CreatedAt: createdAt,
	})
	if err != nil || result.Pending {
		return result, err
	}
	primary := primaryMavenReviewFile(files, coordinate)
	if primary == nil {
		return result, nil
	}
	return result, RecordPublishedPath(state, repo.Name, primary.Path, username, primary.Size, primary.AddedAt)
}

// ApprovePublicationReview records Maven catalog metadata before a pending version becomes visible.
func ApprovePublicationReview(state *core.AppState, task *core.ReviewTask) error {
	if state == nil || state.GetDB() == nil || task == nil || task.Kind != core.ReviewKindPublication ||
		task.ResourceType != core.ReviewResourceMavenArtifact || task.ResourceVersion == "" {
		return core.ErrReviewInvalidRequest
	}
	separator := strings.LastIndexByte(task.ResourceKey, ':')
	if separator <= 0 || separator == len(task.ResourceKey)-1 {
		return core.ErrReviewInvalidRequest
	}
	groupID, artifactID := task.ResourceKey[:separator], task.ResourceKey[separator+1:]
	domains, err := state.GetDB().ListMavenDomains("", true)
	if err != nil {
		return err
	}
	domain := matchingDomain(domains, strings.ToLower(groupID))
	if domain == nil || !domain.Verified {
		return core.ErrMavenDomainUnverified
	}
	createdAt := task.CreatedAt
	if createdAt <= 0 {
		createdAt = time.Now().UnixMilli()
	}
	files, err := state.GetDB().ListReviewTaskFiles(task.ID)
	if err != nil {
		return err
	}
	versionSize := int64(0)
	for _, file := range files {
		if file == nil {
			continue
		}
		coordinate, valid := ParseArtifactPath(file.Path)
		if valid && coordinate.GroupID == groupID && coordinate.ArtifactID == artifactID &&
			coordinate.Version == task.ResourceVersion {
			versionSize += file.Size
		}
	}
	return state.GetDB().RecordMavenPublication(&core.MavenArtifact{
		Repository: task.Repository, Domain: domain.Domain, GroupID: groupID, ArtifactID: artifactID,
		Publisher: task.RequestedBy, LatestVersion: task.ResourceVersion,
		CreatedAt: createdAt, UpdatedAt: max(task.UpdatedAt, createdAt),
	}, &core.MavenVersion{
		Repository: task.Repository, GroupID: groupID, ArtifactID: artifactID,
		Version: task.ResourceVersion, Publisher: task.RequestedBy,
		Size: versionSize, CreatedAt: createdAt,
	})
}

// RemoveApprovedPublicationMetadata rolls back catalog exposure after a failed review decision.
func RemoveApprovedPublicationMetadata(state *core.AppState, task *core.ReviewTask) error {
	if state == nil || state.GetDB() == nil || task == nil || task.ResourceType != core.ReviewResourceMavenArtifact {
		return core.ErrReviewInvalidRequest
	}
	separator := strings.LastIndexByte(task.ResourceKey, ':')
	if separator <= 0 || separator == len(task.ResourceKey)-1 {
		return core.ErrReviewInvalidRequest
	}
	err := state.GetDB().DeleteMavenVersionMetadata(task.Repository, task.ResourceKey[:separator],
		task.ResourceKey[separator+1:], task.ResourceVersion)
	if errors.Is(err, core.ErrMavenVersionNotFound) {
		return nil
	}
	return err
}

// AddPendingPublicationVersions appends visible-to-member synthetic versions without exposing them to public catalog reads.
func AddPendingPublicationVersions(state *core.AppState, details *core.MavenArtifactDetails) error {
	if state == nil || state.GetDB() == nil || details == nil || details.Artifact == nil {
		return core.ErrReviewInvalidRequest
	}
	resourceName := details.Artifact.GroupID + ":" + details.Artifact.ArtifactID
	tasks, err := state.GetDB().ListPublicationReviews(details.Artifact.Repository,
		core.ReviewResourceMavenArtifact, resourceName)
	if err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(details.Versions))
	for _, version := range details.Versions {
		if version != nil {
			existing[version.Version] = struct{}{}
		}
	}
	pendingCount := 0
	pendingFiles := 0
	pendingSize := int64(0)
	for _, task := range tasks {
		if task == nil || task.Status != core.ReviewStatusPending {
			continue
		}
		if _, exists := existing[task.ResourceVersion]; exists {
			continue
		}
		existing[task.ResourceVersion] = struct{}{}
		pendingCount++
		pendingFiles += task.FileCount
		pendingSize += task.TotalSize
		details.Versions = append(details.Versions, &core.MavenVersion{
			Repository: details.Artifact.Repository, GroupID: details.Artifact.GroupID,
			ArtifactID: details.Artifact.ArtifactID, Version: task.ResourceVersion,
			Publisher: task.RequestedBy, Size: task.TotalSize, CreatedAt: task.CreatedAt,
			FileCount: task.FileCount, TotalFileSize: task.TotalSize,
			ReviewStatus: task.Status, ReviewID: task.ID,
		})
	}
	details.Artifact.VersionCount += pendingCount
	details.Artifact.TotalSize += pendingSize
	details.FileCount += pendingFiles
	details.TotalFileSize += pendingSize
	slices.SortFunc(details.Versions, func(left, right *core.MavenVersion) int {
		return -utils.CompareVersions(left.Version, right.Version)
	})
	return nil
}
