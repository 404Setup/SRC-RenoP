/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/token"
)

type userProfileUpdateRequest struct {
	Username *string `json:"username"`
	Nickname *string `json:"nickname"`
}

type userProfileResponse struct {
	UserID                       string               `json:"user_id"`
	Username                     string               `json:"username"`
	Nickname                     string               `json:"nickname"`
	CreatedAt                    string               `json:"created_at"`
	OwnProfile                   bool                 `json:"own_profile"`
	UsernameChangesRemaining     int                  `json:"username_changes_remaining"`
	UsernameChangeWindowResetsAt int64                `json:"username_change_window_resets_at,omitempty"`
	MavenDomainCount             int                  `json:"maven_domain_count"`
	CargoPackageCount            int                  `json:"cargo_package_count"`
	DockerImageCount             int                  `json:"docker_image_count"`
	NPMPackageCount              int                  `json:"npm_package_count"`
	GitHub                       *githubProfileStatus `json:"github,omitempty"`
}

func publicUserProfile(c fiber.Ctx, state *core.AppState) error {
	username := strings.ToLower(strings.TrimSpace(c.Params("username")))
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	profile, err := db.GetUserProfile(username)
	if errors.Is(err, core.ErrUserProfileNotFound) {
		return c.Status(fiber.StatusNotFound).SendString("User profile not found")
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load user profile")
	}
	current := GetUser(c)
	own := current != nil && strings.EqualFold(current.Username, profile.Username)
	mavenMemberships, err := visibleUserPackageMemberships(c, state, profile, config.RepositoryFormatMaven)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load Maven memberships")
	}
	cargoMemberships, err := visibleUserPackageMemberships(c, state, profile, config.RepositoryFormatCargo)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load Cargo memberships")
	}
	dockerMemberships, err := visibleUserPackageMemberships(c, state, profile, config.RepositoryFormatDocker)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load Docker memberships")
	}
	npmMemberships, err := visibleUserPackageMemberships(c, state, profile, config.RepositoryFormatNPM)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load npm memberships")
	}
	response, err := profileResponseWithConnections(state, profile, own, time.Now().UnixMilli())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load account connections")
	}
	response.MavenDomainCount = len(mavenMemberships)
	response.CargoPackageCount = len(cargoMemberships)
	response.DockerImageCount = len(dockerMemberships)
	response.NPMPackageCount = len(npmMemberships)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(response)
}

func publicUserMemberships(c fiber.Ctx, state *core.AppState) error {
	username := strings.ToLower(strings.TrimSpace(c.Params("username")))
	format := strings.ToLower(strings.TrimSpace(c.Query("format")))
	if format != config.RepositoryFormatMaven && format != config.RepositoryFormatCargo &&
		format != config.RepositoryFormatDocker && format != config.RepositoryFormatNPM {
		return c.Status(fiber.StatusBadRequest).SendString("Package format must be maven, cargo, docker, or npm")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	profile, err := db.GetUserProfile(username)
	if errors.Is(err, core.ErrUserProfileNotFound) {
		return c.Status(fiber.StatusNotFound).SendString("User profile not found")
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load user profile")
	}
	memberships, err := visibleUserPackageMemberships(c, state, profile, format)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load package memberships")
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"memberships": memberships})
}

func visibleUserPackageMemberships(c fiber.Ctx, state *core.AppState, profile *core.UserProfile, format string) ([]*core.UserPackageMembership, error) {
	db := state.GetDB()
	memberships, err := db.ListUserPackageMemberships(profile.UserID, format)
	if err != nil {
		return nil, err
	}
	viewer := GetUser(c)
	if format == config.RepositoryFormatMaven {
		return memberships, nil
	}
	ownProfile := viewer != nil && strings.EqualFold(viewer.Username, profile.Username)
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return nil, errors.New("repository configuration is unavailable")
	}
	visible := make([]*core.UserPackageMembership, 0, len(memberships))
	for _, membership := range memberships {
		repository := cfg.Maven.Repositories[membership.Repository]
		if repository == nil || repository.NormalizedFormat() != format {
			continue
		}
		if strings.EqualFold(repository.Visibility, "HIDDEN") &&
			(viewer == nil || !viewer.IsManager() &&
				!viewer.CheckReadPermission(membership.Repository, "", repository.Visibility, true)) {
			continue
		}
		if ownProfile {
			visible = append(visible, membership)
			continue
		}
		if format == config.RepositoryFormatDocker {
			viewerName := ""
			if viewer != nil {
				viewerName = viewer.Username
			}
			exists, private, _, member, _, err := db.GetDockerImageAccess(
				membership.Repository, membership.Name, viewerName)
			if err != nil {
				return nil, err
			}
			if exists && private {
				if member || (viewer != nil && (viewer.IsManager() || viewer.CheckUpdatePermission(membership.Repository))) {
					visible = append(visible, membership)
				}
				continue
			}
		}
		if format == config.RepositoryFormatNPM {
			viewerName := ""
			if viewer != nil {
				viewerName = viewer.Username
			}
			exists, private, _, member, _, err := db.GetNPMPackageAccess(
				membership.Repository, membership.Name, viewerName)
			if err != nil {
				return nil, err
			}
			if exists && private {
				if member || (viewer != nil && (viewer.IsManager() || viewer.CheckUpdatePermission(membership.Repository))) {
					visible = append(visible, membership)
				}
				continue
			}
		}
		if viewer != nil && viewer.CheckReadPermission(membership.Repository, membership.Name, repository.Visibility, false) {
			visible = append(visible, membership)
		}
	}
	return visible, nil
}

func publicUserProfiles(c fiber.Ctx, state *core.AppState) error {
	rawNames := strings.TrimSpace(c.Query("names"))
	if rawNames == "" || len(rawNames) > 4096 {
		return c.Status(fiber.StatusBadRequest).SendString("Choose between 1 and 50 usernames")
	}
	usernames := strings.Split(rawNames, ",")
	if len(usernames) == 0 || len(usernames) > 50 {
		return c.Status(fiber.StatusBadRequest).SendString("Choose between 1 and 50 usernames")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	profiles, err := db.GetUserProfiles(usernames)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load user profiles")
	}
	current := GetUser(c)
	response := make([]userProfileResponse, 0, len(profiles))
	for _, username := range usernames {
		profile := profiles[strings.ToLower(strings.TrimSpace(username))]
		if profile == nil {
			continue
		}
		own := current != nil && strings.EqualFold(current.Username, profile.Username)
		response = append(response, profileResponse(profile, own, time.Now().UnixMilli()))
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"profiles": response})
}

func ownUserProfile(c fiber.Ctx, state *core.AppState) error {
	user := GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	profile, err := db.GetUserProfile(user.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load user profile")
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	response, err := profileResponseWithConnections(state, profile, true, time.Now().UnixMilli())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load account connections")
	}
	return c.JSON(response)
}

func updateOwnUserProfile(c fiber.Ctx, state *core.AppState, opChan chan<- token.TokenOp) error {
	user := GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	if len(c.Body()) > 1024 {
		return c.Status(fiber.StatusRequestEntityTooLarge).SendString("Profile update is too large")
	}
	var request userProfileUpdateRequest
	if err := c.Bind().Body(&request); err != nil || (request.Username == nil && request.Nickname == nil) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid profile update")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Database unavailable")
	}
	current, err := db.GetUserProfile(user.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load user profile")
	}

	newUsername := current.Username
	if request.Username != nil {
		requestedUsername := strings.ToLower(strings.TrimSpace(*request.Username))
		if requestedUsername != current.Username {
			newUsername, err = normalizeProfileUsername(requestedUsername)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).SendString(err.Error())
			}
		}
	}
	nickname := current.Nickname
	if request.Nickname != nil {
		var valid bool
		nickname, valid = core.NormalizeNickname(*request.Nickname)
		if !valid {
			err := errors.New("nickname must not exceed 36 characters or contain control characters")
			return c.Status(fiber.StatusBadRequest).SendString(err.Error())
		}
	}
	github, err := githubProfileStatusForAccount(state, current.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to load account connections")
	}
	if newUsername == current.Username && nickname == current.Nickname {
		response := profileResponse(current, true, time.Now().UnixMilli())
		response.GitHub = &github
		c.Set(fiber.HeaderCacheControl, "no-store")
		return c.JSON(response)
	}

	changedAt := time.Now().UnixMilli()
	if err := token.UpdateUserProfileSync(opChan, current.Username, newUsername, nickname, changedAt); err != nil {
		var rateError *core.UsernameChangeRateError
		switch {
		case errors.As(err, &rateError):
			retrySeconds := max((rateError.RetryAt-changedAt+999)/1000, 1)
			c.Set(fiber.HeaderRetryAfter, strconv.FormatInt(retrySeconds, 10))
			return c.Status(fiber.StatusTooManyRequests).SendString("Username can only be changed twice per 24-hour window")
		case errors.Is(err, core.ErrUsernameAlreadyExists):
			return c.Status(fiber.StatusConflict).SendString("Username is already in use")
		case errors.Is(err, core.ErrUserProfileNotFound):
			return c.Status(fiber.StatusNotFound).SendString("User profile not found")
		default:
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update user profile")
		}
	}
	updated, err := db.GetUserProfile(newUsername)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Profile updated but could not be reloaded")
	}
	logProfileUpdate(c, state, current, updated)
	c.Set(fiber.HeaderCacheControl, "no-store")
	response := profileResponse(updated, true, changedAt)
	response.GitHub = &github
	return c.JSON(response)
}

func profileResponseWithConnections(state *core.AppState, profile *core.UserProfile, own bool,
	now int64) (userProfileResponse, error) {
	response := profileResponse(profile, own, now)
	if !own {
		return response, nil
	}
	github, err := githubProfileStatusForAccount(state, profile.Username)
	if err != nil {
		return userProfileResponse{}, err
	}
	response.GitHub = &github
	return response, nil
}

func profileResponse(profile *core.UserProfile, own bool, now int64) userProfileResponse {
	response := userProfileResponse{
		UserID: profile.UserID, Username: profile.Username, Nickname: profile.Nickname,
		CreatedAt: profile.CreatedAt, OwnProfile: own,
		MavenDomainCount: profile.MavenDomainCount, CargoPackageCount: profile.CargoPackageCount,
		DockerImageCount: profile.DockerImageCount, NPMPackageCount: profile.NPMPackageCount,
	}
	if !own {
		return response
	}
	windowActive := profile.UsernameChangeWindowAt > 0 && now >= profile.UsernameChangeWindowAt &&
		now-profile.UsernameChangeWindowAt < core.UsernameChangeWindowMillis
	if windowActive {
		response.UsernameChangesRemaining = max(core.MaxUsernameChangesPerDay-profile.UsernameChangeCount, 0)
		response.UsernameChangeWindowResetsAt = profile.UsernameChangeWindowAt + core.UsernameChangeWindowMillis
	} else {
		response.UsernameChangesRemaining = core.MaxUsernameChangesPerDay
	}
	return response
}

func normalizeProfileUsername(username string) (string, error) {
	normalized, valid := core.NormalizeUsername(username)
	if !valid {
		return "", errors.New("username must contain 4 to 18 letters, numbers, or underscores")
	}
	return normalized, nil
}

func logProfileUpdate(c fiber.Ctx, state *core.AppState, before, after *core.UserProfile) {
	_, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	if strings.EqualFold(operator, before.Username) {
		operator = after.Username
	}
	details := "Account nickname updated"
	if before.Username != after.Username {
		details = "Account username changed from " + before.Username + " to " + after.Username
	}
	audit.Log(state, &core.AuditLogEntry{
		Username: after.Username, Operator: operator, Action: audit.ActionProfileUpdate, Details: details,
		AuthMethod: authMethod, SessionID: sessionID, IP: ip,
	})
}
