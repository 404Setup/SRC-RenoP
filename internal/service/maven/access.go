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
		return nil, core.ErrMavenPermissionDenied
	}
	return domain, nil
}

// CanReadRepository applies repository visibility and Maven domain membership access.
func CanReadRepository(state *core.AppState, user *config.User, repo *config.Repository, path string, isRoot bool) (bool, error) {
	if repo == nil {
		return false, nil
	}
	if strings.EqualFold(repo.Visibility, "PUBLIC") || strings.EqualFold(repo.Visibility, "HIDDEN") {
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
