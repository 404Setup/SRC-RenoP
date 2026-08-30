/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package statistics

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
)

func requireStatisticsAPIToken(c fiber.Ctx, scope string) error {
	if !auth.CurrentCredentialIsAPIToken(c) {
		c.Set(fiber.HeaderWWWAuthenticate, `Bearer realm="RenoP statistics"`)
		return c.Status(fiber.StatusUnauthorized).SendString("An API token is required for statistics queries")
	}
	if !auth.CurrentCredentialHasScope(c, scope) {
		return c.Status(fiber.StatusForbidden).SendString("API token scope is insufficient")
	}
	return nil
}

func statisticsQuery(c fiber.Ctx, groupDefault string) (core.DownloadStatisticsQuery, error) {
	limit, err := strconv.Atoi(c.Query("limit", "50"))
	if err != nil || limit < 1 || limit > 100 {
		return core.DownloadStatisticsQuery{}, fiber.ErrBadRequest
	}
	offset, err := strconv.Atoi(c.Query("offset", "0"))
	if err != nil || offset < 0 || offset > core.MaxDownloadStatisticsOffset {
		return core.DownloadStatisticsQuery{}, fiber.ErrBadRequest
	}
	query := core.DownloadStatisticsQuery{
		Repository: strings.ToLower(strings.TrimSpace(c.Query("repository"))),
		Format:     strings.ToLower(strings.TrimSpace(c.Query("format"))),
		Namespace:  strings.ToLower(strings.TrimSpace(c.Query("namespace"))),
		Package:    strings.TrimSpace(c.Query("package")),
		Version:    strings.TrimSpace(c.Query("version")),
		GroupBy:    strings.ToLower(strings.TrimSpace(c.Query("group_by", groupDefault))),
		Limit:      limit, Offset: offset,
	}
	for _, filter := range []struct {
		value   string
		maximum int
	}{
		{value: query.Repository, maximum: 64},
		{value: query.Format, maximum: 32},
		{value: query.Namespace, maximum: 253},
		{value: query.Package, maximum: 512},
		{value: query.Version, maximum: 255},
	} {
		if len(filter.value) > filter.maximum {
			return core.DownloadStatisticsQuery{}, fiber.ErrBadRequest
		}
	}
	allowedGroup := query.GroupBy == "user" || query.GroupBy == "repository" || query.GroupBy == "namespace" ||
		query.GroupBy == "package" || query.GroupBy == "version"
	if !allowedGroup {
		return core.DownloadStatisticsQuery{}, fiber.ErrBadRequest
	}
	if query.Format != "" && query.Format != config.RepositoryFormatMaven &&
		query.Format != config.RepositoryFormatFiles && query.Format != config.RepositoryFormatCargo &&
		query.Format != config.RepositoryFormatDocker {
		return core.DownloadStatisticsQuery{}, fiber.ErrBadRequest
	}
	return query, nil
}

func flushStatistics(state *core.AppState) error {
	counter := GetCounter(state)
	if counter == nil {
		return core.ErrDatabaseUnavailable
	}
	return counter.Flush()
}

func writeStatisticsPage(c fiber.Ctx, state *core.AppState, query core.DownloadStatisticsQuery) error {
	if err := flushStatistics(state); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Download statistics are temporarily unavailable")
	}
	page, err := state.GetDB().QueryDownloadStatistics(query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to query download statistics")
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(page)
}

func currentUserStatistics(c fiber.Ctx, state *core.AppState) error {
	if err := requireStatisticsAPIToken(c, core.APITokenScopeStatisticsRead); err != nil {
		return err
	}
	user := auth.GetUser(c)
	if user == nil || strings.EqualFold(user.Username, "guest") {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	profile, err := state.GetDB().GetUserProfile(user.Username)
	if err != nil || profile == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("User profile is unavailable")
	}
	query, err := statisticsQuery(c, "repository")
	if err != nil || query.GroupBy == "user" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid statistics query")
	}
	query.UserID = profile.UserID
	return writeStatisticsPage(c, state, query)
}

func userStatistics(c fiber.Ctx, state *core.AppState) error {
	if err := requireStatisticsAPIToken(c, core.APITokenScopeStatisticsRead); err != nil {
		return err
	}
	requested := strings.ToLower(strings.TrimSpace(c.Params("username")))
	current := auth.GetUser(c)
	if requested == "" || current == nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid statistics user")
	}
	if !strings.EqualFold(requested, current.Username) &&
		(!current.IsManager() || !auth.CurrentCredentialHasScope(c, core.APITokenScopeAdminStatistics)) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	profile, err := state.GetDB().GetUserProfile(requested)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("User profile is unavailable")
	}
	if profile == nil {
		return c.Status(fiber.StatusNotFound).SendString("User not found")
	}
	query, err := statisticsQuery(c, "repository")
	if err != nil || query.GroupBy == "user" {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid statistics query")
	}
	query.UserID = profile.UserID
	return writeStatisticsPage(c, state, query)
}

func systemStatistics(c fiber.Ctx, state *core.AppState) error {
	if err := requireStatisticsAPIToken(c, core.APITokenScopeAdminStatistics); err != nil {
		return err
	}
	user := auth.GetUser(c)
	if user == nil || !user.IsManager() || !auth.CurrentCredentialHasScope(c, core.APITokenScopeAdminStatistics) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	query, err := statisticsQuery(c, "repository")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid statistics query")
	}
	requestedUser := strings.ToLower(strings.TrimSpace(c.Query("username")))
	if requestedUser != "" {
		if len(requestedUser) > 255 {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid statistics user")
		}
		profile, profileErr := state.GetDB().GetUserProfile(requestedUser)
		if profileErr != nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("User profile is unavailable")
		}
		if profile == nil {
			return c.Status(fiber.StatusNotFound).SendString("User not found")
		}
		query.UserID = profile.UserID
	}
	return writeStatisticsPage(c, state, query)
}

// SetupRoutes registers API-token-only download-statistics query routes.
func SetupRoutes(router fiber.Router, state *core.AppState) {
	router.Get("/statistics", func(c fiber.Ctx) error { return currentUserStatistics(c, state) })
	router.Get("/statistics/users/:username", func(c fiber.Ctx) error { return userStatistics(c, state) })
	router.Get("/statistics/system", func(c fiber.Ctx) error { return systemStatistics(c, state) })
}
