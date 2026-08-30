/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"errors"
	"strings"

	"renop/internal/config"
	"renop/internal/core"
)

func matchingDomain(domains []*core.MavenDomain, candidate string) *core.MavenDomain {
	var matched *core.MavenDomain
	for _, domain := range domains {
		if domain == nil || !domainContainsGroup(domain.Domain, candidate) {
			continue
		}
		if matched == nil || len(domain.Domain) > len(matched.Domain) {
			matched = domain
		}
	}
	return matched
}

// AuthorizeGroup checks a verified namespace team permission for one groupId.
func AuthorizeGroup(state *core.AppState, user *config.User, repo *config.Repository, groupID string, requiredLevel int, administratorAllowed bool) (*core.MavenDomain, error) {
	if state == nil || repo == nil || repo.NormalizedFormat() != config.RepositoryFormatMaven {
		return nil, core.ErrMavenDomainNotFound
	}
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return nil, core.ErrMavenPermissionDenied
	}
	if state.GetDB() == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	if administratorAllowed && (user.IsManager() || user.CheckUpdatePermission(repo.Name)) {
		domains, err := state.GetDB().ListMavenDomains(user.Username, true)
		if err != nil {
			return nil, err
		}
		domain := matchingDomain(domains, strings.ToLower(strings.TrimSpace(groupID)))
		if domain == nil {
			return nil, core.ErrMavenDomainNotFound
		}
		return domain, nil
	}
	domains, err := state.GetDB().ListMavenDomains(user.Username, false)
	if err != nil {
		return nil, err
	}
	domain := matchingDomain(domains, strings.ToLower(strings.TrimSpace(groupID)))
	if domain == nil || !domain.Member || domain.PermissionLevel < requiredLevel {
		return nil, core.ErrMavenPermissionDenied
	}
	if !domain.Verified {
		return nil, core.ErrMavenDomainUnverified
	}
	return domain, nil
}

// AuthorizeArtifact combines verified domain permission with an optional global-team artifact binding.
func AuthorizeArtifact(state *core.AppState, user *config.User, repo *config.Repository,
	groupID, artifactID string, requiredLevel int, administratorAllowed bool,
) (*core.MavenDomain, error) {
	domain, err := AuthorizeGroup(state, user, repo, groupID, requiredLevel, administratorAllowed)
	if err == nil {
		return domain, nil
	}
	if state == nil || user == nil || repo == nil || state.GetDB() == nil ||
		errors.Is(err, core.ErrMavenDomainNotFound) || errors.Is(err, core.ErrMavenDomainUnverified) {
		return nil, err
	}
	domains, listErr := state.GetDB().ListMavenDomains(user.Username, false)
	if listErr != nil {
		return nil, listErr
	}
	domain = matchingDomain(domains, strings.ToLower(strings.TrimSpace(groupID)))
	if domain == nil || !domain.Verified {
		return nil, core.ErrMavenPermissionDenied
	}
	_, member, level, accessErr := state.GetDB().GetMavenArtifactTeamAccess(
		repo.Name, groupID, artifactID, user.Username)
	if accessErr != nil {
		return nil, accessErr
	}
	if !member || level < requiredLevel {
		return nil, core.ErrMavenPermissionDenied
	}
	domain.Member = true
	domain.PermissionLevel = level
	return domain, nil
}

func pathArtifactCandidate(path string) (groupID, artifactID string, ok bool) {
	if coordinate, valid := ParseArtifactPath(path); valid {
		return coordinate.GroupID, coordinate.ArtifactID, true
	}
	parts := strings.Split(strings.Trim(strings.ReplaceAll(path, `\`, "/"), "/"), "/")
	if len(parts) < 3 || !strings.HasPrefix(parts[len(parts)-1], "maven-metadata.xml") {
		return "", "", false
	}
	artifactID = parts[len(parts)-2]
	groupParts := parts[:len(parts)-2]
	if len(groupParts) == 0 {
		return "", "", false
	}
	return strings.Join(groupParts, "."), artifactID, true
}

// AuthorizeMutation checks verified domain ownership for an upload or deletion path.
func AuthorizeMutation(state *core.AppState, user *config.User, repo *config.Repository, path string, requiredLevel int) (*core.MavenDomain, error) {
	if state == nil || repo == nil || repo.NormalizedFormat() != config.RepositoryFormatMaven {
		return nil, core.ErrMavenDomainNotFound
	}
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return nil, core.ErrMavenPermissionDenied
	}
	if !isMavenPublicationPath(path) {
		return nil, core.ErrMavenPermissionDenied
	}
	candidate := pathNamespaceCandidate(path)
	if candidate == "" {
		return nil, core.ErrMavenPermissionDenied
	}
	db := state.GetDB()
	if db == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	domains, err := db.ListMavenDomains(user.Username, false)
	if err != nil {
		return nil, err
	}
	domain := matchingDomain(domains, candidate)
	if domain == nil {
		return nil, core.ErrMavenPermissionDenied
	}
	if !domain.Verified {
		return nil, core.ErrMavenDomainUnverified
	}
	if domain.PermissionLevel < requiredLevel {
		groupID, artifactID, artifactPath := pathArtifactCandidate(path)
		if !artifactPath {
			return nil, core.ErrMavenPermissionDenied
		}
		_, member, level, accessErr := db.GetMavenArtifactTeamAccess(
			repo.Name, groupID, artifactID, user.Username)
		if errors.Is(accessErr, core.ErrMavenArtifactNotFound) {
			return nil, core.ErrMavenPermissionDenied
		}
		if accessErr != nil {
			return nil, accessErr
		}
		if !member || level < requiredLevel {
			return nil, core.ErrMavenPermissionDenied
		}
		domain.Member = true
		domain.PermissionLevel = level
	}
	return domain, nil
}

// CanReadRepository applies repository visibility and Maven domain membership access.
func CanReadRepository(state *core.AppState, user *config.User, repo *config.Repository, path string, isRoot bool) (bool, error) {
	if repo == nil {
		return false, nil
	}
	if strings.EqualFold(repo.Visibility, "PUBLIC") {
		return true, nil
	}
	if user != nil && user.CheckReadPermission(repo.Name, path, repo.Visibility, isRoot) {
		return true, nil
	}
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return false, nil
	}
	if state == nil || state.GetDB() == nil {
		return false, core.ErrDatabaseUnavailable
	}
	allowed, err := state.GetDB().HasMavenMembership(user.Username)
	if err != nil {
		return false, errors.Join(core.ErrDatabaseUnavailable, err)
	}
	return allowed, nil
}
