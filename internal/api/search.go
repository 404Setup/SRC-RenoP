/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package api

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/service/cargo"
	"renop/internal/service/index"
	"renop/internal/utils"
)

const (
	defaultRepositorySearchLimit = 20
	maxRepositorySearchLimit     = 50
	maxRepositorySearchScan      = 100000
	maxRepositorySearchMatches   = 500
)

type repositorySearchResult struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Type          string `json:"type"`
	Description   string `json:"description,omitempty"`
	LatestVersion string `json:"latest_version,omitempty"`
	Size          int64  `json:"size,omitempty"`
	ModifiedAt    int64  `json:"modified_at,omitempty"`
}

type repositorySearchResponse struct {
	Format  string                   `json:"format"`
	Results []repositorySearchResult `json:"results"`
	Total   int                      `json:"total"`
	HasMore bool                     `json:"has_more"`
}

// SearchRepository provides one bounded, format-aware search endpoint for the
// repository browser. Cargo searches package metadata; Maven searches the
// indexed repository tree without touching the filesystem on every request.
func SearchRepository(c fiber.Ctx, state *core.AppState) error {
	query := strings.TrimSpace(c.Query("q"))
	if query == "" || len(query) > 128 || strings.ContainsAny(query, "\x00\r\n") {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid repository search query")
	}
	limit, err := strconv.Atoi(c.Query("limit", strconv.Itoa(defaultRepositorySearchLimit)))
	if err != nil || limit < 1 || limit > maxRepositorySearchLimit {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid repository search limit")
	}

	repository := c.Params("repo_name")
	if !utils.IsValidRepositoryName(repository) || state == nil || state.Inner == nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid repository")
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Repository configuration is unavailable")
	}
	repo := cfg.Maven.Repositories[repository]
	if repo == nil {
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}
	user := auth.GetUser(c)
	canRead, err := cargo.CanReadRepository(state, user, repo, "", true)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Repository metadata is unavailable")
	}
	if !canRead {
		return c.Status(fiber.StatusNotFound).SendString("Repository not found")
	}

	var response repositorySearchResponse
	if repo.NormalizedFormat() == config.RepositoryFormatCargo {
		response, err = searchCargoRepository(state, repo, query, limit)
	} else {
		response, err = searchMavenRepository(state, cfg.StoragePath, repo, user, query, limit)
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Repository search failed")
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(response)
}

func searchCargoRepository(state *core.AppState, repo *config.Repository, query string, limit int) (repositorySearchResponse, error) {
	db := state.GetDB()
	if db == nil {
		return repositorySearchResponse{}, core.ErrDatabaseUnavailable
	}
	packages, total, err := db.SearchCargoPackages(repo.Name, query, limit, 0)
	if err != nil {
		return repositorySearchResponse{}, err
	}
	results := make([]repositorySearchResult, 0, len(packages))
	for _, pkg := range packages {
		if pkg == nil {
			continue
		}
		results = append(results, repositorySearchResult{
			Name: pkg.Name, Path: "packages/" + pkg.Name, Type: "PACKAGE",
			Description: pkg.Description, LatestVersion: pkg.MaxVersion,
		})
	}
	return repositorySearchResponse{
		Format: config.RepositoryFormatCargo, Results: results, Total: total, HasMore: total > len(results),
	}, nil
}

func searchMavenRepository(state *core.AppState, storagePath string, repo *config.Repository, user *config.User, query string, limit int) (repositorySearchResponse, error) {
	root := filepath.ToSlash(filepath.Clean(filepath.Join(storagePath, repo.Name)))
	rootPrefix := root + "/"
	needle := strings.ToLower(query)
	results := make([]repositorySearchResult, 0, min(limit*4, maxRepositorySearchMatches))
	total := 0
	visited := 0
	scanLimitReached := false

	state.Inner.FileIndex.Walk(root, func(indexedPath string, info index.FileInfo, isDir bool) bool {
		visited++
		if visited > maxRepositorySearchScan {
			scanLimitReached = true
			return false
		}
		if indexedPath == root || state.Inner.FileIndex.IsBlocked(indexedPath) {
			return true
		}
		var relative string
		if strings.HasPrefix(indexedPath, rootPrefix) {
			relative = indexedPath[len(rootPrefix):]
		} else {
			rel, err := filepath.Rel(root, indexedPath)
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
				return true
			}
			relative = filepath.ToSlash(rel)
		}
		if relative == "" || !containsFold(relative, needle) ||
			!user.CheckReadPermission(repo.Name, relative, repo.Visibility, isDir) {
			return true
		}
		total++
		if len(results) >= maxRepositorySearchMatches {
			return true
		}
		resultType := "FILE"
		if isDir {
			resultType = "DIRECTORY"
		}
		name := relative
		if idx := strings.LastIndexByte(relative, '/'); idx != -1 {
			name = relative[idx+1:]
		}
		result := repositorySearchResult{Name: name, Path: relative, Type: resultType}
		if !isDir {
			result.Size = info.Size
			result.ModifiedAt = time.Unix(0, info.ModTime).UnixMilli()
		}
		results = append(results, result)
		return true
	})

	sort.SliceStable(results, func(i, j int) bool {
		leftRank := repositorySearchRank(results[i].Name, needle)
		rightRank := repositorySearchRank(results[j].Name, needle)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if len(results[i].Path) != len(results[j].Path) {
			return len(results[i].Path) < len(results[j].Path)
		}
		return strings.ToLower(results[i].Path) < strings.ToLower(results[j].Path)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return repositorySearchResponse{
		Format: config.RepositoryFormatMaven, Results: results, Total: total,
		HasMore: scanLimitReached || total > len(results),
	}, nil
}

func containsFold(s, substrLower string) bool {
	if len(substrLower) == 0 {
		return true
	}
	if len(s) < len(substrLower) {
		return false
	}
	maxStart := len(s) - len(substrLower)
	for i := 0; i <= maxStart; i++ {
		match := true
		for j := 0; j < len(substrLower); j++ {
			c := s[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != substrLower[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func repositorySearchRank(name, queryLower string) int {
	if strings.EqualFold(name, queryLower) {
		return 0
	}
	if hasPrefixFold(name, queryLower) {
		return 1
	}
	return 2
}

func hasPrefixFold(s, prefixLower string) bool {
	if len(s) < len(prefixLower) {
		return false
	}
	for i := 0; i < len(prefixLower); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != prefixLower[i] {
			return false
		}
	}
	return true
}
