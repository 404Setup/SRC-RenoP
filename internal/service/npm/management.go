/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/repositorygate"
	"renop/internal/utils"
)

const npmAPIErrorCodeHeader = "X-Renop-Error-Code"

func npmAPIError(c fiber.Ctx, status int, code, message string) error {
	c.Set(npmAPIErrorCodeHeader, code)
	return c.Status(status).SendString(message)
}

func npmRepository(c fiber.Ctx, state *core.AppState) (*config.Repository, error) {
	repository := strings.ToLower(strings.TrimSpace(c.Params("repo_name")))
	if !utils.IsValidRepositoryName(repository) {
		return nil, fiber.ErrBadRequest
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil || cfg.Maven.Repositories[repository] == nil ||
		cfg.Maven.Repositories[repository].NormalizedFormat() != config.RepositoryFormatNPM {
		return nil, fiber.ErrNotFound
	}
	return cfg.Maven.Repositories[repository], nil
}

func npmPackageQuery(c fiber.Ctx) (string, bool) {
	value := c.Query("package")
	if value == "" {
		value = c.Query("name")
	}
	if decoded, ok := decodeRegistryPath(value); ok {
		value = decoded
	}
	return NormalizePackageName(value)
}

func listPackagesAPI(c fiber.Ctx, state *core.AppState) error {
	repo, err := npmRepository(c, state)
	if errors.Is(err, fiber.ErrBadRequest) {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid repository")
	}
	if err != nil {
		return npmAPIError(c, fiber.StatusNotFound, "repository_not_found", "Repository not found")
	}
	if c.Query("package") != "" || c.Query("name") != "" {
		packageName, valid := npmPackageQuery(c)
		if !valid {
			return npmAPIError(c, fiber.StatusBadRequest, "invalid_package_name", "Invalid npm package name")
		}
		return packageDetailsAPI(c, state, repo, packageName)
	}
	user := auth.GetUser(c)
	allowed, accessErr := CanReadRepository(state, user, repo, "", true)
	if accessErr != nil || !allowed {
		return npmAPIError(c, fiber.StatusForbidden, "permission_denied", "Forbidden")
	}
	limit, parseErr := strconv.Atoi(c.Query("limit", "50"))
	if parseErr != nil || limit < 1 || limit > 100 {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Limit must be between 1 and 100")
	}
	offset, parseErr := strconv.Atoi(c.Query("offset", "0"))
	if parseErr != nil || offset < 0 || offset > 1_000_000 {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Offset is out of range")
	}
	query := strings.TrimSpace(c.Query("query"))
	administrator := user.IsManager() || user.CheckUpdatePermission(repo.Name)
	packages, total, err := state.GetDB().SearchNPMPackages(repo.Name, query, user.Username,
		administrator, limit, offset)
	if err != nil {
		return npmAPIError(c, fiber.StatusInternalServerError, "internal_error", "Failed to list npm packages")
	}
	return c.JSON(fiber.Map{
		"repository": repo.Name, "packages": packages, "total": total, "limit": limit, "offset": offset,
	})
}

func removeNPMTarballs(state *core.AppState, store Store, repository string, paths []string) {
	if state == nil || state.Inner == nil || store == nil {
		log.Printf("Failed to remove npm tarballs for repository %s: storage state is unavailable", repository)
		return
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		log.Printf("Failed to remove npm tarballs for repository %s: configuration is unavailable", repository)
		return
	}
	repositoryRoot := filepath.Join(cfg.StoragePath, repository)
	for _, tarballPath := range paths {
		target := filepath.Join(repositoryRoot, filepath.FromSlash(tarballPath))
		if !utils.IsSubPath(repositoryRoot, target) {
			state.Inner.FailuresCount.Add(1)
			log.Printf("Refused to remove npm tarball outside repository %s: %s", repository, tarballPath)
			continue
		}
		if err := store.Delete(state, target); err != nil {
			state.Inner.FailuresCount.Add(1)
			log.Printf("Failed to remove npm tarball %s from repository %s: %v", tarballPath, repository, err)
		}
	}
}

func packageDetailsAPI(c fiber.Ctx, state *core.AppState, repo *config.Repository, packageName string) error {
	user := auth.GetUser(c)
	allowed, err := CanReadPackage(state, user, repo, packageName)
	if err != nil || !allowed {
		return npmAPIError(c, fiber.StatusNotFound, "package_not_found", "Package not found")
	}
	details, err := state.GetDB().GetNPMPackageDetails(repo.Name, packageName, user.Username)
	if err != nil || details == nil || details.Package == nil {
		return npmAPIError(c, fiber.StatusNotFound, "package_not_found", "Package not found")
	}
	if details.Member || user.CheckModeratePermission(repo.Name) {
		if err := AddPendingPublicationVersions(state, details); err != nil {
			return npmAPIError(c, fiber.StatusInternalServerError, "review_unavailable", "Publication review is unavailable")
		}
	}
	enrichNPMProjectMetadata(details)
	details.Administrator = user.IsManager() || user.CheckUpdatePermission(repo.Name)
	if !details.Administrator && details.Package.PermissionLevel < core.NPMPermissionTeam {
		details.Members = nil
	}
	return c.JSON(details)
}

type createPackageRequest struct {
	Name            string `json:"name"`
	SuperTeamPrefix string `json:"super_team_prefix"`
	Private         bool   `json:"private"`
}

func createPackageAPI(c fiber.Ctx, state *core.AppState) error {
	repo, err := npmRepository(c, state)
	if err != nil {
		return npmAPIError(c, fiber.StatusNotFound, "repository_not_found", "Repository not found")
	}
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return npmAPIError(c, fiber.StatusUnauthorized, "authentication_required", "Authentication required")
	}
	if !user.IsManager() && !user.CheckUpdatePermission(repo.Name) {
		return npmAPIError(c, fiber.StatusForbidden, "permission_denied", "Repository update permission is required")
	}
	var request createPackageRequest
	if err := utils.ReadJSONLimited(c, &request, 4096); err != nil {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid request body")
	}
	packageName, valid := NormalizePackageName(request.Name)
	if !valid {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_package_name", "Invalid npm package name")
	}
	if request.Private && !strings.HasPrefix(packageName, "@") {
		return npmAPIError(c, fiber.StatusBadRequest, "private_requires_scope", "Private npm packages must be scoped")
	}
	teamPrefix := strings.ToLower(strings.TrimSpace(request.SuperTeamPrefix))
	if teamPrefix != "" {
		var teamPrefixValid bool
		teamPrefix, teamPrefixValid = core.NormalizeSuperTeamPrefix(teamPrefix)
		if !teamPrefixValid {
			return npmAPIError(c, fiber.StatusBadRequest, "super_team_mismatch", "Invalid global team prefix")
		}
	}
	requiredPrefix, scoped := core.NPMPackageSuperTeamPrefix(packageName)
	if scoped && teamPrefix == "" {
		return npmAPIError(c, fiber.StatusBadRequest, "super_team_required", "Scoped npm packages require a global team")
	}
	if scoped && teamPrefix != requiredPrefix {
		return npmAPIError(c, fiber.StatusBadRequest, "super_team_mismatch", "npm scope must match the global team")
	}
	if teamPrefix != "" && !auth.CurrentCredentialHasScopeTarget(
		c, core.APITokenScopeTeamManage, "global/"+teamPrefix) {
		c.Set("X-Renop-Required-Scope", core.APITokenScopeTeamManage)
		return npmAPIError(c, fiber.StatusForbidden, "super_team_permission", "API token cannot use this global team")
	}
	db := state.GetDB()
	if db == nil {
		return npmAPIError(c, fiber.StatusServiceUnavailable, "internal_error", "Database unavailable")
	}
	reviewTeamPrefix := ""
	if teamPrefix != "" {
		teamRole, roleErr := db.GetSuperTeamRole(teamPrefix, user.Username)
		if roleErr != nil {
			if errors.Is(roleErr, core.ErrSuperTeamPermissionDenied) {
				return npmAPIError(c, fiber.StatusForbidden, "super_team_permission",
					"T2 or higher global team permission is required")
			}
			return npmAPIError(c, fiber.StatusInternalServerError, "internal_error",
				"Failed to inspect global team permission")
		}
		if teamRole < core.SuperTeamRoleWrite {
			return npmAPIError(c, fiber.StatusForbidden, "super_team_permission",
				"T2 or higher global team permission is required")
		}
		if teamRole == core.SuperTeamRoleWrite {
			reviewTeamPrefix = teamPrefix
		}
	}
	release := repositorygate.AcquireMutation(repo.Name)
	defer release()
	currentConfig := state.Inner.Config.Load()
	if currentConfig == nil {
		return npmAPIError(c, fiber.StatusServiceUnavailable, "internal_error", "Configuration unavailable")
	}
	repo = currentConfig.Maven.Repositories[repo.Name]
	if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatNPM {
		return npmAPIError(c, fiber.StatusNotFound, "repository_not_found", "Repository not found")
	}
	existing, err := db.GetNPMPackage(repo.Name, packageName)
	if err != nil {
		return npmAPIError(c, fiber.StatusInternalServerError, "internal_error", "Failed to inspect npm package")
	}
	if existing != nil {
		return npmAPIError(c, fiber.StatusConflict, "package_exists", "npm package already exists")
	}
	createdAt := time.Now().UnixMilli()
	if repo.PublicationReviewPolicy() != config.PublicationReviewOff || reviewTeamPrefix != "" {
		review, reviewErr := QueuePackageCreationReview(
			state, repo, packageName, teamPrefix, reviewTeamPrefix, user.Username, request.Private, createdAt)
		if errors.Is(reviewErr, core.ErrReviewPermissionDenied) {
			return npmAPIError(c, fiber.StatusConflict, "review_pending",
				"Another account already requested this npm package name")
		}
		if reviewErr != nil || review == nil || !review.Pending {
			return npmAPIError(c, fiber.StatusInternalServerError, "review_unavailable",
				"Failed to create npm package review")
		}
		c.Set("X-RenoP-Review-ID", review.TaskID)
		logNPMAudit(c, state, audit.ActionUploadQueuedReview,
			fmt.Sprintf("Repository: %s, package creation: %s, global team: %s",
				repo.Name, packageName, teamPrefix))
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"pending": true, "review_id": review.TaskID, "name": packageName,
		})
	}
	pkg, err := db.CreateNPMPackageForTeam(repo.Name, packageName, user.Username,
		teamPrefix, request.Private, createdAt)
	if errors.Is(err, core.ErrNPMPackageExists) {
		return npmAPIError(c, fiber.StatusConflict, "package_exists", "npm package already exists")
	}
	if errors.Is(err, core.ErrSuperTeamBindingRequired) {
		return npmAPIError(c, fiber.StatusBadRequest, "super_team_required", err.Error())
	}
	if errors.Is(err, core.ErrSuperTeamBindingMismatch) {
		return npmAPIError(c, fiber.StatusBadRequest, "super_team_mismatch", err.Error())
	}
	if errors.Is(err, core.ErrSuperTeamBindingPermission) {
		return npmAPIError(c, fiber.StatusForbidden, "super_team_permission", err.Error())
	}
	if err != nil {
		return npmAPIError(c, fiber.StatusInternalServerError, "internal_error", "Failed to create npm package")
	}
	logNPMAudit(c, state, audit.ActionNPMPackageCreate,
		fmt.Sprintf("Repository: %s, package: %s, private: %t, global team: %s",
			repo.Name, packageName, request.Private, teamPrefix))
	return c.Status(fiber.StatusCreated).JSON(pkg)
}

type updatePackageRequest struct {
	Description *string `json:"description"`
	Private     *bool   `json:"private"`
	Archived    *bool   `json:"archived"`
}

func updatePackageAPI(c fiber.Ctx, state *core.AppState) error {
	repo, err := npmRepository(c, state)
	if err != nil {
		return npmAPIError(c, fiber.StatusNotFound, "repository_not_found", "Repository not found")
	}
	packageName, valid := npmPackageQuery(c)
	if !valid {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_package_name", "Invalid npm package name")
	}
	var request updatePackageRequest
	if err := utils.ReadJSONLimited(c, &request, 8192); err != nil {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid request body")
	}
	changes := 0
	if request.Description != nil {
		changes++
	}
	if request.Private != nil {
		changes++
	}
	if request.Archived != nil {
		changes++
	}
	if changes != 1 {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Change exactly one package setting per request")
	}
	user := auth.GetUser(c)
	actor := npmMutationActor(user, repo.Name)
	if request.Description != nil {
		err = state.GetDB().UpdateNPMPackageDescription(repo.Name, packageName, *request.Description, actor)
	} else if request.Private != nil {
		if *request.Private && !strings.HasPrefix(packageName, "@") {
			return npmAPIError(c, fiber.StatusBadRequest, "private_requires_scope", "Private npm packages must be scoped")
		}
		err = state.GetDB().SetNPMPackagePrivate(repo.Name, packageName, actor, *request.Private)
	} else {
		err = state.GetDB().SetNPMPackageArchived(repo.Name, packageName, actor, *request.Archived)
	}
	if err != nil {
		return npmManagementError(c, err)
	}
	action := audit.ActionNPMMetadataUpdate
	if request.Archived != nil {
		action = audit.ActionNPMPackageRestore
		if *request.Archived {
			action = audit.ActionNPMPackageArchive
		}
	}
	logNPMAudit(c, state, action, "Repository: "+repo.Name+", package: "+packageName)
	return c.JSON(operationResponse{OK: true, ID: packageName})
}

func deletePackageAPI(c fiber.Ctx, state *core.AppState, store Store) error {
	repo, err := npmRepository(c, state)
	if err != nil {
		return npmAPIError(c, fiber.StatusNotFound, "repository_not_found", "Repository not found")
	}
	packageName, valid := npmPackageQuery(c)
	if !valid {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_package_name", "Invalid npm package name")
	}
	user := auth.GetUser(c)
	paths, err := state.GetDB().DeleteNPMPackage(repo.Name, packageName, npmMutationActor(user, repo.Name), 0)
	if err != nil {
		return npmManagementError(c, err)
	}
	removeNPMTarballs(state, store, repo.Name, paths)
	logNPMAudit(c, state, audit.ActionNPMPackageDelete,
		"Repository: "+repo.Name+", package: "+packageName)
	return c.JSON(operationResponse{OK: true, ID: packageName})
}

func deleteVersionAPI(c fiber.Ctx, state *core.AppState, store Store) error {
	repo, err := npmRepository(c, state)
	if err != nil {
		return npmAPIError(c, fiber.StatusNotFound, "repository_not_found", "Repository not found")
	}
	packageName, valid := npmPackageQuery(c)
	version := strings.TrimSpace(c.Query("version"))
	if !valid || !validNPMVersion(version) {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid npm package or version")
	}
	user := auth.GetUser(c)
	path, err := state.GetDB().UnpublishNPMVersion(repo.Name, packageName, version,
		npmMutationActor(user, repo.Name), 0)
	if err != nil {
		return npmManagementError(c, err)
	}
	removeNPMTarballs(state, store, repo.Name, []string{path})
	logNPMAudit(c, state, audit.ActionNPMVersionDelete,
		"Repository: "+repo.Name+", package: "+packageName+", version: "+version)
	return c.JSON(operationResponse{OK: true, ID: packageName})
}

func npmManagementError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, core.ErrNPMPackageNotFound), errors.Is(err, core.ErrNPMVersionNotFound):
		return npmAPIError(c, fiber.StatusNotFound, "package_not_found", "npm package or version not found")
	case errors.Is(err, core.ErrNPMPermissionDenied):
		return npmAPIError(c, fiber.StatusForbidden, "permission_denied", "Insufficient npm package permission")
	case errors.Is(err, core.ErrNPMLastFullMember), errors.Is(err, core.ErrNPMOwnerCannotLeave):
		return npmAPIError(c, fiber.StatusConflict, "last_owner", err.Error())
	case errors.Is(err, core.ErrNPMMemberExists):
		return npmAPIError(c, fiber.StatusConflict, "member_exists", "User is already a package member")
	default:
		return npmAPIError(c, fiber.StatusInternalServerError, "internal_error", "npm package operation failed")
	}
}

type inviteRequest struct {
	Users []string `json:"users"`
	Level int      `json:"level"`
}

func teamAccess(state *core.AppState, repo *config.Repository, packageName string, user *config.User) (
	administrator bool, member bool, level int, err error,
) {
	username := ""
	if user != nil {
		administrator = user.IsManager() || user.CheckUpdatePermission(repo.Name)
		username = user.Username
	}
	_, _, _, member, level, err = state.GetDB().GetNPMPackageAccess(repo.Name, packageName, username)
	return administrator, member, level, err
}

func listMembersAPI(c fiber.Ctx, state *core.AppState) error {
	repo, err := npmRepository(c, state)
	packageName, valid := npmPackageQuery(c)
	if err != nil || !valid {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid npm package")
	}
	user := auth.GetUser(c)
	administrator, _, level, err := teamAccess(state, repo, packageName, user)
	if err != nil || !administrator && level < core.NPMPermissionTeam {
		return npmAPIError(c, fiber.StatusForbidden, "permission_denied", "Package team permission is required")
	}
	members, err := state.GetDB().ListNPMMembers(repo.Name, packageName)
	if err != nil {
		return npmAPIError(c, fiber.StatusInternalServerError, "internal_error", "Failed to list npm package members")
	}
	return c.JSON(fiber.Map{"users": members})
}

func inviteMembersAPI(c fiber.Ctx, state *core.AppState) error {
	repo, err := npmRepository(c, state)
	packageName, valid := npmPackageQuery(c)
	if err != nil || !valid {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid npm package")
	}
	var request inviteRequest
	if err := utils.ReadJSONLimited(c, &request, 8192); err != nil || len(request.Users) == 0 || len(request.Users) > 20 ||
		request.Level < core.NPMPermissionRead || request.Level > core.NPMPermissionOwner {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Choose 1 to 20 users and an L0-L4 permission")
	}
	user := auth.GetUser(c)
	administrator, _, level, err := teamAccess(state, repo, packageName, user)
	if err != nil || !administrator && level < core.NPMPermissionTeam {
		return npmAPIError(c, fiber.StatusForbidden, "permission_denied", "Package team permission is required")
	}
	validUsers := make([]string, 0, len(request.Users))
	seen := make(map[string]struct{}, len(request.Users))
	for _, candidate := range request.Users {
		recipient := strings.ToLower(strings.TrimSpace(candidate))
		if recipient == "" || len(recipient) > 255 {
			continue
		}
		if _, duplicate := seen[recipient]; duplicate {
			continue
		}
		seen[recipient] = struct{}{}
		if token, lookupErr := state.GetDB().GetTokenByName(recipient); lookupErr != nil || token == nil {
			return npmAPIError(c, fiber.StatusBadRequest, "user_not_found", "Invited user does not exist")
		}
		validUsers = append(validUsers, recipient)
	}
	if len(validUsers) == 0 {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "No valid users were provided")
	}
	if administrator {
		if err := state.GetDB().ForceAddNPMMembers(repo.Name, packageName, user.Username, validUsers, request.Level); err != nil {
			return npmManagementError(c, err)
		}
		logNPMAudit(c, state, audit.ActionNPMTeamAdd,
			fmt.Sprintf("Repository: %s, package: %s, members: %d, level: L%d", repo.Name, packageName, len(validUsers), request.Level))
		return c.JSON(operationResponse{OK: true, ID: packageName})
	}
	now := time.Now().UnixMilli()
	invitations := make([]*core.NPMInvitation, 0, len(validUsers))
	messages := make([]*core.UserMessage, 0, len(validUsers))
	for _, recipient := range validUsers {
		if strings.EqualFold(recipient, user.Username) {
			return npmAPIError(c, fiber.StatusBadRequest, "cannot_invite_self", "Cannot invite yourself")
		}
		id := uuid.NewString()
		payload, _ := json.Marshal(fiber.Map{
			"repository": repo.Name, "package": packageName, "inviter": user.Username, "level": request.Level,
		})
		invitations = append(invitations, &core.NPMInvitation{
			ID: id, Repository: repo.Name, Package: packageName, Inviter: user.Username,
			Recipient: recipient, Level: request.Level, CreatedAt: now,
		})
		messages = append(messages, &core.UserMessage{
			ID: id, Recipient: recipient, Sender: strings.ToLower(user.Username), Kind: "npm_package_invite",
			Severity: "info", Title: "npm package invitation",
			Body:    fmt.Sprintf("%s invited you to collaborate on %s with L%d permission.", user.Username, packageName, request.Level),
			Payload: payload, ActionKind: "npm_package_invite", ActionStatus: core.MessageActionPending,
			CreatedAt: now, ExpiresAt: now + 7*24*3600*1000,
		})
	}
	if err := state.GetDB().CreateNPMInvitations(invitations, messages); err != nil {
		if errors.Is(err, core.ErrNPMInvitationExists) {
			return npmAPIError(c, fiber.StatusConflict, "invitation_pending", "An invitation is already pending")
		}
		return npmManagementError(c, err)
	}
	logNPMAudit(c, state, audit.ActionNPMTeamInvite,
		fmt.Sprintf("Repository: %s, package: %s, recipients: %d, level: L%d", repo.Name, packageName, len(invitations), request.Level))
	return c.JSON(operationResponse{OK: true, ID: packageName})
}

type levelRequest struct {
	Level int `json:"level"`
}

func resolveMemberReference(db core.StateDB, reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if _, err := uuid.Parse(reference); err != nil {
		return reference, nil
	}
	profile, err := db.GetUserProfileByID(reference)
	if err != nil {
		return "", err
	}
	return profile.Username, nil
}

func setMemberLevelAPI(c fiber.Ctx, state *core.AppState) error {
	repo, err := npmRepository(c, state)
	packageName, valid := npmPackageQuery(c)
	target, resolveErr := resolveMemberReference(state.GetDB(), c.Params("username"))
	if err != nil || !valid || resolveErr != nil {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid npm package member")
	}
	var request levelRequest
	if err := utils.ReadJSONLimited(c, &request, 1024); err != nil ||
		request.Level < core.NPMPermissionRead || request.Level > core.NPMPermissionOwner {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Permission must be between L0 and L4")
	}
	user := auth.GetUser(c)
	administrator, _, level, err := teamAccess(state, repo, packageName, user)
	if err != nil || !administrator && level < core.NPMPermissionTeam {
		return npmAPIError(c, fiber.StatusForbidden, "permission_denied", "Package team permission is required")
	}
	actor := user.Username
	if administrator {
		actor = ""
	}
	if err := state.GetDB().SetNPMMemberLevel(repo.Name, packageName, actor, target, request.Level); err != nil {
		return npmManagementError(c, err)
	}
	logNPMAudit(c, state, audit.ActionNPMTeamLevel,
		fmt.Sprintf("Repository: %s, package: %s, member: %s, level: L%d", repo.Name, packageName, target, request.Level))
	return c.JSON(operationResponse{OK: true, ID: packageName})
}

func removeMemberAPI(c fiber.Ctx, state *core.AppState) error {
	repo, err := npmRepository(c, state)
	packageName, valid := npmPackageQuery(c)
	target, resolveErr := resolveMemberReference(state.GetDB(), c.Params("username"))
	if err != nil || !valid || resolveErr != nil {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid npm package member")
	}
	user := auth.GetUser(c)
	administrator, member, level, err := teamAccess(state, repo, packageName, user)
	isSelf := strings.EqualFold(target, user.Username)
	if err != nil || !administrator && level < core.NPMPermissionTeam && !(isSelf && member) {
		return npmAPIError(c, fiber.StatusForbidden, "permission_denied", "Package team permission is required")
	}
	actor := user.Username
	if administrator && !isSelf {
		actor = ""
	}
	if err := state.GetDB().RemoveNPMMember(repo.Name, packageName, actor, target); err != nil {
		return npmManagementError(c, err)
	}
	logNPMAudit(c, state, audit.ActionNPMTeamRemove,
		"Repository: "+repo.Name+", package: "+packageName+", member: "+target)
	return c.JSON(operationResponse{OK: true, ID: packageName})
}

func searchUsersAPI(c fiber.Ctx, state *core.AppState) error {
	repo, err := npmRepository(c, state)
	packageName, valid := npmPackageQuery(c)
	if err != nil || !valid {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid npm package")
	}
	user := auth.GetUser(c)
	administrator, _, level, err := teamAccess(state, repo, packageName, user)
	if err != nil || !administrator && level < core.NPMPermissionTeam {
		return npmAPIError(c, fiber.StatusForbidden, "permission_denied", "Package team permission is required")
	}
	users, err := state.GetDB().SearchTokenNames(strings.TrimSpace(c.Query("q")), 10, time.Now().UnixMilli())
	if err != nil {
		return npmAPIError(c, fiber.StatusInternalServerError, "internal_error", "Failed to search users")
	}
	return c.JSON(fiber.Map{"users": users})
}

func respondInvitationAPI(c fiber.Ctx, state *core.AppState) error {
	repo, err := npmRepository(c, state)
	decision := strings.ToLower(strings.TrimSpace(c.Params("decision")))
	id := strings.TrimSpace(c.Params("id"))
	if err != nil || id == "" || decision != "accept" && decision != "reject" {
		return npmAPIError(c, fiber.StatusBadRequest, "invalid_request", "Invalid invitation response")
	}
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return npmAPIError(c, fiber.StatusUnauthorized, "authentication_required", "Authentication required")
	}
	if err := state.GetDB().RespondNPMInvitation(id, user.Username, repo.Name,
		decision == "accept", time.Now().UnixMilli()); err != nil {
		if errors.Is(err, core.ErrNPMInvitationInvalid) {
			return npmAPIError(c, fiber.StatusBadRequest, "invitation_invalid", "Invitation is invalid or expired")
		}
		return npmManagementError(c, err)
	}
	action := audit.ActionNPMInviteReject
	if decision == "accept" {
		action = audit.ActionNPMInviteAccept
	}
	logNPMAudit(c, state, action, "Repository: "+repo.Name+", invitation: "+id)
	return c.JSON(operationResponse{OK: true})
}

// SetupRoutes registers browser-management APIs for npm repositories.
func SetupRoutes(router fiber.Router, state *core.AppState, store Store) {
	wrap := func(handler fiber.Handler) fiber.Handler {
		return func(c fiber.Ctx) error {
			err := handler(c)
			if (err != nil || c.Response().StatusCode() >= fiber.StatusBadRequest) &&
				len(c.Response().Header.Peek(npmAPIErrorCodeHeader)) == 0 {
				c.Set(npmAPIErrorCodeHeader, "request_failed")
			}
			return err
		}
	}
	base := router.Group("/npm/repositories/:repo_name")
	base.Get("/packages", wrap(func(c fiber.Ctx) error { return listPackagesAPI(c, state) }))
	base.Post("/packages", wrap(func(c fiber.Ctx) error { return createPackageAPI(c, state) }))
	base.Put("/packages", wrap(func(c fiber.Ctx) error { return updatePackageAPI(c, state) }))
	base.Delete("/packages", wrap(func(c fiber.Ctx) error { return deletePackageAPI(c, state, store) }))
	base.Delete("/versions", wrap(func(c fiber.Ctx) error { return deleteVersionAPI(c, state, store) }))
	base.Get("/owners", wrap(func(c fiber.Ctx) error { return listMembersAPI(c, state) }))
	base.Post("/owners", wrap(func(c fiber.Ctx) error { return inviteMembersAPI(c, state) }))
	base.Put("/owners/:username", wrap(func(c fiber.Ctx) error { return setMemberLevelAPI(c, state) }))
	base.Delete("/owners/:username", wrap(func(c fiber.Ctx) error { return removeMemberAPI(c, state) }))
	base.Get("/users/search", wrap(func(c fiber.Ctx) error { return searchUsersAPI(c, state) }))
	base.Post("/invitations/:id/:decision", wrap(func(c fiber.Ctx) error { return respondInvitationAPI(c, state) }))
}
