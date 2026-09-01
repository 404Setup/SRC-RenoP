/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package maven owns verified publishing domains, domain teams, and the Maven catalog.
package maven

import (
	"errors"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/service/storage"
	"renop/internal/utils"
)

const (
	maxManagementRequestSize   = 16 << 10
	maxMavenReadmeBytes        = 512 << 10
	maxMavenReadmeRequestBytes = maxMavenReadmeBytes + (16 << 10)
	verificationInterval       = 5 * time.Second
)

type createDomainRequest struct {
	Domain          string `json:"domain"`
	SuperTeamPrefix string `json:"super_team_prefix"`
}

type updateArtifactRequest struct {
	Description *string `json:"description"`
	Readme      *string `json:"readme"`
}

func wireStorageHooks() {
	storage.MavenMutationAuthorizer = func(state *core.AppState, user *config.User, repo *config.Repository, path string, requiredLevel int) error {
		_, err := AuthorizeMutation(state, user, repo, path, requiredLevel)
		return err
	}
	storage.MavenPublicationQuotaOwner = func(state *core.AppState, username string, repo *config.Repository, path string) (string, error) {
		domain, err := AuthorizeMutation(state, &config.User{Username: username}, repo, path, core.MavenPermissionPublish)
		if err != nil {
			return "", err
		}
		if coordinate, valid := ParseArtifactPath(path); valid {
			prefix, _, _, accessErr := state.GetDB().GetMavenArtifactTeamAccess(
				repo.Name, coordinate.GroupID, coordinate.ArtifactID, username)
			if accessErr == nil && prefix != "" {
				return prefix, nil
			}
			if accessErr != nil && !errors.Is(accessErr, core.ErrMavenArtifactNotFound) {
				return "", accessErr
			}
		}
		return domain.SuperTeamPrefix, nil
	}
	storage.MavenReadAuthorizer = CanReadRepository
	storage.MavenPublicationReviewCandidate = IsPublicationReviewCandidate
	storage.MavenPublicationProcessor = ProcessPublishedFiles
	storage.MavenMirrorRecorder = RecordMirroredPath
}

// SetupRoutes registers Maven domain and catalog management APIs.
func SetupRoutes(router fiber.Router, state *core.AppState) {
	wireStorageHooks()
	global := router.Group("/maven")
	registerDomainRoutes(global, state)
	global.Get("/domains/:domain/packages", func(c fiber.Ctx) error { return listDomainArtifacts(c, state) })
	base := router.Group("/maven/repositories/:repo_name")
	base.Use(func(c fiber.Ctx) error {
		if _, err := repository(c, state); err != nil {
			return apiError(c, err)
		}
		return c.Next()
	})
	registerDomainRoutes(base, state)
	base.Get("/packages", func(c fiber.Ctx) error { return listArtifacts(c, state) })
	base.Get("/package", func(c fiber.Ctx) error { return getArtifact(c, state) })
	base.Put("/package", func(c fiber.Ctx) error { return updateArtifact(c, state) })
	base.Delete("/versions", func(c fiber.Ctx) error { return deleteVersion(c, state) })
}

func registerDomainRoutes(base fiber.Router, state *core.AppState) {
	base.Get("/domains", func(c fiber.Ctx) error { return listDomains(c, state) })
	base.Post("/domains", func(c fiber.Ctx) error { return createDomain(c, state) })
	base.Get("/domains/:domain", func(c fiber.Ctx) error { return getDomain(c, state) })
	base.Post("/domains/:domain/verify", func(c fiber.Ctx) error { return verifyDomain(c, state) })
	base.Post("/domains/:domain/verify/force", func(c fiber.Ctx) error { return forceVerifyDomain(c, state) })
	base.Delete("/domains/:domain", func(c fiber.Ctx) error { return deleteDomain(c, state) })
	base.Get("/domains/:domain/members", func(c fiber.Ctx) error { return listMembers(c, state) })
	base.Post("/domains/:domain/members", func(c fiber.Ctx) error { return inviteMembers(c, state) })
	base.Put("/domains/:domain/members/:username", func(c fiber.Ctx) error { return setMemberLevel(c, state) })
	base.Delete("/domains/:domain/members/:username", func(c fiber.Ctx) error { return removeMember(c, state) })
	base.Post("/invitations/:id/:decision", func(c fiber.Ctx) error { return respondInvitation(c, state) })
}

func repository(c fiber.Ctx, state *core.AppState) (*config.Repository, error) {
	if state == nil || state.Inner == nil {
		return nil, fiber.ErrServiceUnavailable
	}
	name := c.Params("repo_name")
	if !utils.IsValidRepositoryName(name) {
		return nil, fiber.ErrBadRequest
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return nil, fiber.ErrServiceUnavailable
	}
	repo := cfg.Maven.Repositories[name]
	if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatMaven {
		return nil, fiber.ErrNotFound
	}
	return repo, nil
}

func authenticated(c fiber.Ctx) (*config.User, error) {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return nil, fiber.ErrUnauthorized
	}
	return user, nil
}

func apiError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, core.ErrSuperTeamBindingRequired):
		c.Set("X-Renop-Error-Code", "super_team_required")
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	case errors.Is(err, core.ErrSuperTeamBindingMismatch):
		c.Set("X-Renop-Error-Code", "super_team_mismatch")
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	case errors.Is(err, core.ErrSuperTeamBindingPermission):
		c.Set("X-Renop-Error-Code", "super_team_permission")
		return c.Status(fiber.StatusForbidden).SendString(err.Error())
	case errors.Is(err, fiber.ErrBadRequest):
		return c.Status(fiber.StatusBadRequest).SendString("Invalid Maven request")
	case errors.Is(err, core.ErrUserProfileNotFound):
		c.Set("X-RenoP-Error-Code", "MAVEN_USER_NOT_FOUND")
		return c.Status(fiber.StatusBadRequest).SendString("Maven user does not exist")
	case errors.Is(err, fiber.ErrUnauthorized):
		return c.Status(fiber.StatusUnauthorized).SendString("Authentication required")
	case errors.Is(err, fiber.ErrNotFound), errors.Is(err, core.ErrMavenDomainNotFound), errors.Is(err, core.ErrMavenArtifactNotFound), errors.Is(err, core.ErrMavenVersionNotFound):
		return c.Status(fiber.StatusNotFound).SendString("Maven resource not found")
	case errors.Is(err, core.ErrMavenDomainExists), errors.Is(err, core.ErrMavenMemberExists), errors.Is(err, core.ErrMavenInvitationExists):
		return c.Status(fiber.StatusConflict).SendString(err.Error())
	case errors.Is(err, core.ErrMavenDomainUnverified), errors.Is(err, core.ErrMavenVerificationFailed),
		errors.Is(err, core.ErrMavenDomainNotEmpty),
		errors.Is(err, core.ErrMavenLastFullMember), errors.Is(err, core.ErrMavenOwnerCannotLeave),
		errors.Is(err, core.ErrMavenInvitationInvalid):
		return c.Status(fiber.StatusConflict).SendString(err.Error())
	case errors.Is(err, core.ErrMavenVerificationRateLimit):
		c.Set(fiber.HeaderRetryAfter, strconv.Itoa(int(verificationInterval/time.Second)))
		return c.Status(fiber.StatusTooManyRequests).SendString(err.Error())
	case errors.Is(err, core.ErrMavenPermissionDenied):
		return c.Status(fiber.StatusForbidden).SendString("Maven domain permission denied")
	case errors.Is(err, core.ErrDatabaseUnavailable), errors.Is(err, fiber.ErrServiceUnavailable):
		return c.Status(fiber.StatusServiceUnavailable).SendString("Maven metadata is unavailable")
	default:
		return c.Status(fiber.StatusInternalServerError).SendString("Maven operation failed")
	}
}

func logAudit(c fiber.Ctx, state *core.AppState, action, details string) {
	username, operator, method, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, AuthMethod: method, SessionID: sessionID,
		IP: ip, Action: action, Details: details, CreatedAt: time.Now().UnixMilli(),
	})
}

func redactMavenDomainForViewer(domain *core.MavenDomain, administrator, globalArtifactCount bool) {
	if domain == nil || administrator {
		return
	}
	if domain.PermissionLevel < core.MavenPermissionManage {
		domain.VerificationCode = ""
	}
	if !domain.Member {
		domain.RepositoryCount = 0
		if globalArtifactCount {
			domain.ArtifactCount = 0
		}
	}
}

func managedDomainListOptions(c fiber.Ctx, user *config.User) (core.MavenDomainListOptions, error) {
	limit, err := strconv.Atoi(c.Query("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		return core.MavenDomainListOptions{}, fiber.ErrBadRequest
	}
	offset, err := strconv.Atoi(c.Query("offset", "0"))
	if err != nil || offset < 0 {
		return core.MavenDomainListOptions{}, fiber.ErrBadRequest
	}
	administrator := user.IsManager()
	options := core.MavenDomainListOptions{
		Username: user.Username, Administrator: administrator, Limit: limit, Offset: offset,
	}
	levelsValue := strings.TrimSpace(c.Query("levels"))
	statesValue := strings.TrimSpace(c.Query("states"))
	options.Filtered = levelsValue != "" || statesValue != ""
	seenLevels := make(map[int]struct{}, 5)
	if levelsValue != "" {
		for _, value := range strings.Split(levelsValue, ",") {
			level, parseErr := strconv.Atoi(strings.TrimSpace(value))
			if parseErr != nil || level < core.MavenPermissionRead || level > core.MavenPermissionOwner ||
				(administrator && level == core.MavenPermissionRead) {
				return core.MavenDomainListOptions{}, fiber.ErrBadRequest
			}
			if _, exists := seenLevels[level]; exists {
				continue
			}
			seenLevels[level] = struct{}{}
			options.PermissionLevels = append(options.PermissionLevels, level)
		}
	}
	if statesValue != "" {
		if !administrator {
			return core.MavenDomainListOptions{}, core.ErrMavenPermissionDenied
		}
		seenStates := make(map[string]struct{}, 2)
		for _, value := range strings.Split(statesValue, ",") {
			state := strings.ToLower(strings.TrimSpace(value))
			if _, exists := seenStates[state]; exists {
				continue
			}
			seenStates[state] = struct{}{}
			switch state {
			case "unverified":
				options.IncludeUnverified = true
			case "mirror":
				options.IncludeMirrored = true
			default:
				return core.MavenDomainListOptions{}, fiber.ErrBadRequest
			}
		}
	}
	return options, nil
}

func listManagedDomains(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	options, err := managedDomainListOptions(c, user)
	if err != nil {
		return apiError(c, err)
	}
	domains, total, err := state.GetDB().ListManagedMavenDomains(options)
	if err != nil {
		return apiError(c, err)
	}
	for _, domain := range domains {
		redactMavenDomainForViewer(domain, options.Administrator, true)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{
		"domains": domains, "total": total, "limit": options.Limit, "offset": options.Offset,
		"administrator": options.Administrator,
	})
}

func listDomains(c fiber.Ctx, state *core.AppState) error {
	user := auth.GetUser(c)
	username := ""
	administrator := false
	if user != nil {
		username = user.Username
		administrator = user.IsManager()
	}
	if c.Params("repo_name") != "" {
		repo, err := repository(c, state)
		if err != nil {
			return apiError(c, err)
		}
		if err := authorizeRepositoryRead(c, state, repo); err != nil {
			return apiError(c, err)
		}
		if err := UpgradeLegacyRepository(state, repo.Name); err != nil {
			return apiError(c, err)
		}
		domains, err := state.GetDB().ListMavenRepositoryDomains(repo.Name, username)
		if err != nil {
			return apiError(c, err)
		}
		for _, domain := range domains {
			redactMavenDomainForViewer(domain, administrator, false)
		}
		c.Set(fiber.HeaderCacheControl, "no-store")
		return c.JSON(fiber.Map{"repository": repo.Name, "domains": domains})
	}
	view := strings.TrimSpace(c.Query("view"))
	if view == "managed" {
		return listManagedDomains(c, state)
	}
	if view != "" {
		return apiError(c, fiber.ErrBadRequest)
	}
	domains, err := state.GetDB().ListMavenDomains(username, administrator)
	if err != nil {
		return apiError(c, err)
	}
	for _, domain := range domains {
		redactMavenDomainForViewer(domain, administrator, true)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"domains": domains})
}

func createDomain(c fiber.Ctx, state *core.AppState) error {
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	if len(c.Body()) > maxManagementRequestSize {
		return c.SendStatus(fiber.StatusRequestEntityTooLarge)
	}
	var request createDomainRequest
	if err := c.Bind().Body(&request); err != nil {
		return apiError(c, fiber.ErrBadRequest)
	}
	domain, err := NormalizeDomain(request.Domain)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}
	if !auth.CurrentCredentialHasScopeTarget(c, core.APITokenScopeDomainCreate, domain) &&
		!auth.CurrentCredentialHasScope(c, core.APITokenScopeDomainManage) {
		c.Set("X-Renop-Required-Scope", core.APITokenScopeDomainCreate)
		return c.Status(fiber.StatusForbidden).SendString("API token target is not permitted")
	}
	verificationType, verificationHost, err := VerificationTarget(domain)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}
	code, err := NewVerificationCode()
	if err != nil {
		return apiError(c, err)
	}
	now := time.Now().UnixMilli()
	record := &core.MavenDomain{
		Domain: domain, VerificationType: verificationType,
		VerificationHost: verificationHost, VerificationCode: code,
		SuperTeamPrefix: strings.ToLower(strings.TrimSpace(request.SuperTeamPrefix)), CreatedAt: now,
		PermissionLevel: core.MavenPermissionOwner, Member: true,
	}
	if record.SuperTeamPrefix != "" {
		var bindingValid bool
		record.SuperTeamPrefix, bindingValid = core.NormalizeSuperTeamPrefix(record.SuperTeamPrefix)
		if !bindingValid {
			return apiError(c, core.ErrSuperTeamBindingMismatch)
		}
	}
	if record.SuperTeamPrefix != "" && !auth.CurrentCredentialHasScopeTarget(
		c, core.APITokenScopeTeamManage, "global/"+record.SuperTeamPrefix) {
		c.Set("X-Renop-Required-Scope", core.APITokenScopeTeamManage)
		return apiError(c, core.ErrSuperTeamBindingPermission)
	}
	if verificationType == core.MavenVerificationGitHub {
		authorized, authErr := state.GetDB().HasRecentGitHubPrincipal(user.Username, verificationHost,
			now-core.GitHubPrincipalFreshnessMillis)
		if authErr != nil {
			return apiError(c, authErr)
		}
		if authorized {
			record.Verified = true
			record.VerifiedAt = now
		}
	}
	if err := state.GetDB().CreateMavenDomain(record, user.Username); err != nil {
		return apiError(c, err)
	}
	logAudit(c, state, audit.ActionMavenDomainCreate, fmt.Sprintf("Domain: %s", domain))
	return c.Status(fiber.StatusCreated).JSON(record)
}

func authorizedDomain(c fiber.Ctx, state *core.AppState, required int) (*config.User, *core.MavenDomainDetails, error) {
	user, err := authenticated(c)
	if err != nil {
		return nil, nil, err
	}
	domain, err := NormalizeDomain(c.Params("domain"))
	if err != nil {
		return nil, nil, fiber.ErrBadRequest
	}
	details, err := state.GetDB().GetMavenDomainDetails(domain, user.Username)
	if err != nil {
		return nil, nil, err
	}
	administrator := user.IsManager()
	if !administrator {
		if !details.Domain.Member || details.Domain.PermissionLevel < required {
			return nil, nil, core.ErrMavenPermissionDenied
		}
	} else {
		details.Administrator = true
	}
	return user, details, nil
}

func visibleDomain(c fiber.Ctx, state *core.AppState) (*core.MavenDomainDetails, error) {
	user := auth.GetUser(c)
	username := ""
	administrator := false
	if user != nil {
		username = user.Username
		administrator = user.IsManager()
	}
	domain, err := NormalizeDomain(c.Params("domain"))
	if err != nil {
		return nil, fiber.ErrBadRequest
	}
	details, err := state.GetDB().GetMavenDomainDetails(domain, username)
	if err != nil {
		return nil, err
	}
	if !details.Domain.Verified && !administrator && !details.Domain.Member {
		return nil, core.ErrMavenDomainNotFound
	}
	if administrator {
		details.Administrator = true
	}
	redactMavenDomainForViewer(details.Domain, administrator, true)
	if !administrator && details.Domain.PermissionLevel < core.MavenPermissionManage {
		details.Members = nil
	}
	return details, nil
}

func getDomain(c fiber.Ctx, state *core.AppState) error {
	details, err := visibleDomain(c, state)
	if err != nil {
		return apiError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(details)
}

func listDomainArtifacts(c fiber.Ctx, state *core.AppState) error {
	if _, err := visibleDomain(c, state); err != nil {
		return apiError(c, err)
	}
	limit, err := strconv.Atoi(c.Query("limit", "30"))
	if err != nil {
		return apiError(c, fiber.ErrBadRequest)
	}
	offset, err := strconv.Atoi(c.Query("offset", "0"))
	if err != nil {
		return apiError(c, fiber.ErrBadRequest)
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return apiError(c, fiber.ErrServiceUnavailable)
	}
	repositories := make([]string, 0, len(cfg.Maven.Repositories))
	for name, repo := range cfg.Maven.Repositories {
		if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatMaven {
			continue
		}
		allowed, readErr := CanReadRepository(state, auth.GetUser(c), repo, "", true)
		if readErr != nil {
			return apiError(c, readErr)
		}
		if allowed {
			repositories = append(repositories, name)
		}
	}
	slices.Sort(repositories)
	artifacts, total, err := state.GetDB().ListMavenDomainArtifacts(repositories, c.Params("domain"), limit, offset)
	if err != nil {
		return apiError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"artifacts": artifacts, "total": total})
}

func verifyDomain(c fiber.Ctx, state *core.AppState) error {
	user, details, err := authorizedDomain(c, state, core.MavenPermissionOwner)
	if err != nil {
		return apiError(c, err)
	}
	if details.Domain.Verified {
		return c.JSON(details.Domain)
	}
	now := time.Now()
	if err := state.GetDB().ReserveMavenVerificationAttempt(details.Domain.Domain, user.Username,
		details.Administrator, now.UnixMilli(), now.Add(-verificationInterval).UnixMilli()); err != nil {
		return apiError(c, err)
	}
	if err := VerifyDomainProof(c.Context(), state.Inner.Config.Load(), details.Domain); err != nil {
		if errors.Is(err, core.ErrMavenVerificationFailed) {
			return apiError(c, err)
		}
		log.Printf("Maven domain verification failed for %s: %v", details.Domain.Domain, err)
		return c.Status(fiber.StatusBadGateway).SendString("Maven verification provider is unavailable")
	}
	if err := state.GetDB().MarkMavenDomainVerified(details.Domain.Domain,
		details.Domain.VerificationCode, now.UnixMilli()); err != nil {
		return apiError(c, err)
	}
	details.Domain.Verified = true
	details.Domain.VerifiedAt = now.UnixMilli()
	if err := ReconcileGlobalDomainCatalog(state, details.Domain.Domain, user.Username); err != nil {
		log.Printf("failed to reconcile Maven catalog for %s: %v", details.Domain.Domain, err)
	}
	logAudit(c, state, audit.ActionMavenDomainVerify, fmt.Sprintf("Domain: %s", details.Domain.Domain))
	return c.JSON(details.Domain)
}

func forceVerifyDomain(c fiber.Ctx, state *core.AppState) error {
	user, details, err := authorizedDomain(c, state, core.MavenPermissionOwner)
	if err != nil {
		return apiError(c, err)
	}
	if !user.IsManager() {
		return apiError(c, core.ErrMavenPermissionDenied)
	}
	if details.Domain.Verified {
		return c.JSON(details.Domain)
	}
	now := time.Now().UnixMilli()
	if err := state.GetDB().MarkMavenDomainVerified(details.Domain.Domain,
		details.Domain.VerificationCode, now); err != nil {
		return apiError(c, err)
	}
	details.Domain.Verified = true
	details.Domain.VerifiedAt = now
	if err := ReconcileGlobalDomainCatalog(state, details.Domain.Domain, user.Username); err != nil {
		log.Printf("failed to reconcile force-verified Maven catalog for %s: %v", details.Domain.Domain, err)
	}
	logAudit(c, state, audit.ActionMavenDomainForceVerify,
		fmt.Sprintf("Domain: %s", details.Domain.Domain))
	return c.JSON(details.Domain)
}

func deleteDomain(c fiber.Ctx, state *core.AppState) error {
	user, details, err := authorizedDomain(c, state, core.MavenPermissionOwner)
	if err != nil {
		return apiError(c, err)
	}
	if err := state.GetDB().DeleteMavenDomain(details.Domain.Domain, user.Username,
		details.Administrator, time.Now().UnixMilli()); err != nil {
		return apiError(c, err)
	}
	logAudit(c, state, audit.ActionMavenDomainDelete, fmt.Sprintf("Domain: %s", details.Domain.Domain))
	return c.SendStatus(fiber.StatusNoContent)
}

func authorizeRepositoryRead(c fiber.Ctx, state *core.AppState, repo *config.Repository) error {
	allowed, err := CanReadRepository(state, auth.GetUser(c), repo, "", true)
	if err != nil {
		return err
	}
	if !allowed {
		return core.ErrMavenPermissionDenied
	}
	return nil
}

func listArtifacts(c fiber.Ctx, state *core.AppState) error {
	repo, err := repository(c, state)
	if err != nil {
		return apiError(c, err)
	}
	if err := authorizeRepositoryRead(c, state, repo); err != nil {
		return apiError(c, err)
	}
	if err := UpgradeLegacyRepository(state, repo.Name); err != nil {
		return apiError(c, err)
	}
	limit, err := strconv.Atoi(c.Query("limit", "30"))
	if err != nil {
		return apiError(c, fiber.ErrBadRequest)
	}
	offset, err := strconv.Atoi(c.Query("offset", "0"))
	if err != nil {
		return apiError(c, fiber.ErrBadRequest)
	}
	domain := strings.ToLower(strings.TrimSpace(c.Query("domain")))
	query := strings.TrimSpace(c.Query("q"))
	artifacts, total, err := state.GetDB().ListMavenArtifacts(repo.Name, domain, query, limit, offset)
	if err != nil {
		return apiError(c, err)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"repository": repo.Name, "artifacts": artifacts, "total": total})
}

func getArtifact(c fiber.Ctx, state *core.AppState) error {
	repo, err := repository(c, state)
	if err != nil {
		return apiError(c, err)
	}
	if err := authorizeRepositoryRead(c, state, repo); err != nil {
		return apiError(c, err)
	}
	groupID, artifactID := strings.TrimSpace(c.Query("group")), strings.TrimSpace(c.Query("artifact"))
	if groupID == "" || artifactID == "" {
		return apiError(c, fiber.ErrBadRequest)
	}
	details, err := state.GetDB().GetMavenArtifactDetails(repo.Name, groupID, artifactID)
	if err != nil {
		return apiError(c, err)
	}
	if err := enrichMavenArtifactDetails(state, repo.Name, details); err != nil {
		log.Printf("failed to enrich Maven artifact %s:%s in %s: %v", groupID, artifactID, repo.Name, err)
	}
	user := auth.GetUser(c)
	if user != nil && !strings.EqualFold(user.Username, "guest") {
		if domain, authErr := AuthorizeArtifact(
			state, user, repo, groupID, artifactID, core.MavenPermissionRead, true); authErr == nil {
			details.Artifact.PermissionLevel = domain.PermissionLevel
			details.Administrator = user.IsManager() || user.CheckUpdatePermission(repo.Name)
			if err := AddPendingPublicationVersions(state, details); err != nil {
				return apiError(c, err)
			}
		} else if user.CheckModeratePermission(repo.Name) {
			if err := AddPendingPublicationVersions(state, details); err != nil {
				return apiError(c, err)
			}
		}
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(details)
}

func updateArtifact(c fiber.Ctx, state *core.AppState) error {
	repo, err := repository(c, state)
	if err != nil {
		return apiError(c, err)
	}
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	groupID, artifactID := strings.TrimSpace(c.Query("group")), strings.TrimSpace(c.Query("artifact"))
	if _, err := AuthorizeArtifact(
		state, user, repo, groupID, artifactID, core.MavenPermissionVersion, true); err != nil {
		return apiError(c, err)
	}
	var request updateArtifactRequest
	if err := utils.ReadJSONLimited(c, &request, maxMavenReadmeRequestBytes); errors.Is(err, fiber.ErrRequestEntityTooLarge) {
		return c.SendStatus(fiber.StatusRequestEntityTooLarge)
	} else if err != nil || (request.Description == nil) == (request.Readme == nil) ||
		request.Description != nil && len(*request.Description) > 4000 ||
		request.Readme != nil && len(*request.Readme) > maxMavenReadmeBytes {
		return apiError(c, fiber.ErrBadRequest)
	}
	if request.Description != nil {
		err = state.GetDB().UpdateMavenArtifactDescription(repo.Name, groupID, artifactID, *request.Description)
	} else {
		err = state.GetDB().UpdateMavenArtifactReadme(repo.Name, groupID, artifactID, *request.Readme)
	}
	if err != nil {
		return apiError(c, err)
	}
	logAudit(c, state, audit.ActionMavenArtifactUpdate, fmt.Sprintf("Repository: %s, artifact: %s:%s", repo.Name, groupID, artifactID))
	return c.JSON(fiber.Map{"ok": true})
}

func deleteVersion(c fiber.Ctx, state *core.AppState) error {
	repo, err := repository(c, state)
	if err != nil {
		return apiError(c, err)
	}
	user, err := authenticated(c)
	if err != nil {
		return apiError(c, err)
	}
	groupID := strings.TrimSpace(c.Query("group"))
	artifactID := strings.TrimSpace(c.Query("artifact"))
	version := strings.TrimSpace(c.Query("version"))
	if groupID == "" || artifactID == "" || version == "" {
		return apiError(c, fiber.ErrBadRequest)
	}
	if _, err := AuthorizeArtifact(
		state, user, repo, groupID, artifactID, core.MavenPermissionVersion, true); err != nil {
		return apiError(c, err)
	}
	if err := storage.RemoveMavenVersion(state, repo.Name, groupID, artifactID, version); err != nil {
		return apiError(c, err)
	}
	if err := state.GetDB().DeleteMavenVersionMetadata(repo.Name, groupID, artifactID, version); err != nil {
		return apiError(c, err)
	}
	logAudit(c, state, audit.ActionMavenVersionDelete, fmt.Sprintf("Repository: %s, artifact: %s:%s, version: %s", repo.Name, groupID, artifactID, version))
	return c.SendStatus(fiber.StatusNoContent)
}
