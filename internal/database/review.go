/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/goccy/go-json"
	"github.com/google/uuid"

	"renop/internal/config"
	"renop/internal/core"
)

const maxReviewResourceKey = 1024

var reviewTaskMutationLock sync.Mutex

const reviewTaskSelectColumns = `r.id, r.kind, r.resource_type, r.repository, r.resource_key,
	r.resource_name, r.source_team_prefix, r.target_team_prefix, r.review_team_prefix,
	COALESCE(requester.username, r.requested_by_name), r.requested_by_id, r.status,
	r.decision_reason, COALESCE(decider.username, r.decided_by_name), r.decided_by_id,
	r.created_at, r.decided_at`

const reviewTaskProfileJoins = ` LEFT JOIN user_profiles requester ON requester.user_id = r.requested_by_id
	LEFT JOIN user_profiles decider ON decider.user_id = r.decided_by_id`

func scanReviewTask(scanner row) (*core.ReviewTask, error) {
	task := &core.ReviewTask{}
	storedResourceKey := ""
	if err := scanner.Scan(&task.ID, &task.Kind, &task.ResourceType, &task.Repository, &storedResourceKey,
		&task.ResourceName, &task.SourceTeamPrefix, &task.TargetTeamPrefix, &task.ReviewTeamPrefix,
		&task.RequestedBy, &task.RequestedByID, &task.Status, &task.DecisionReason,
		&task.DecidedBy, &task.DecidedByID, &task.CreatedAt, &task.DecidedAt); err != nil {
		return nil, err
	}
	task.ResourceKey = storedResourceKey
	if task.Kind == core.ReviewKindPublication {
		resourceKey, version, valid := decodePublicationReviewKey(storedResourceKey)
		if !valid {
			return nil, errors.New("publication review key is invalid")
		}
		task.ResourceKey = resourceKey
		task.ResourceVersion = version
	}
	task.UpdatedAt = task.CreatedAt
	return task, nil
}

func normalizeReviewResourceType(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case core.ReviewResourceDockerImage, core.ReviewResourceNPMPackage, core.ReviewResourceCargoPackage,
		core.ReviewResourceMavenArtifact, core.ReviewResourceMavenDomain:
		return value, true
	default:
		return "", false
	}
}

func splitMavenArtifactReviewKey(value string) (string, string, bool) {
	value = SanitizeInputString(strings.TrimSpace(value), maxReviewResourceKey)
	separator := strings.LastIndexByte(value, ':')
	if separator <= 0 || separator == len(value)-1 {
		return "", "", false
	}
	groupID := sanitizeMavenDomain(value[:separator])
	artifactID := SanitizeInputString(strings.TrimSpace(value[separator+1:]), 255)
	return groupID, artifactID, groupID != "" && artifactID != ""
}

type reviewResourceState struct {
	binding string
	name    string
	domain  string
	local   bool
}

func loadReviewResourceTx(tx *Tx, request *core.SuperTeamTransferRequest) (reviewResourceState, error) {
	var state reviewResourceState
	request.Repository = strings.ToLower(SanitizeInputString(strings.TrimSpace(request.Repository), 64))
	request.ResourceKey = SanitizeInputString(strings.TrimSpace(request.ResourceKey), maxReviewResourceKey)
	switch request.ResourceType {
	case core.ReviewResourceDockerImage:
		request.Repository, request.ResourceKey = sanitizeDockerKey(request.Repository, request.ResourceKey)
		var pushEnabled int
		err := tx.QueryRow(`SELECT super_team_prefix, image_name, push_enabled FROM docker_images
			WHERE repository = ? AND image_name = ?`, request.Repository, request.ResourceKey).Scan(
			&state.binding, &state.name, &pushEnabled)
		state.local = pushEnabled != 0
		return state, reviewResourceError(err)
	case core.ReviewResourceNPMPackage:
		request.Repository, request.ResourceKey = sanitizeNPMKey(request.Repository, request.ResourceKey)
		var mirrored int
		err := tx.QueryRow(`SELECT super_team_prefix, package_name, mirrored FROM npm_packages
			WHERE repository = ? AND package_name = ?`, request.Repository, request.ResourceKey).Scan(
			&state.binding, &state.name, &mirrored)
		state.local = mirrored == 0
		return state, reviewResourceError(err)
	case core.ReviewResourceCargoPackage:
		request.Repository, request.ResourceKey = sanitizeCargoKey(request.Repository, request.ResourceKey)
		var mirrored int
		err := tx.QueryRow(`SELECT super_team_prefix, package_name, mirrored FROM cargo_packages
			WHERE repository = ? AND normalized_name = ?`, request.Repository, request.ResourceKey).Scan(
			&state.binding, &state.name, &mirrored)
		state.local = mirrored == 0
		return state, reviewResourceError(err)
	case core.ReviewResourceMavenArtifact:
		groupID, artifactID, valid := splitMavenArtifactReviewKey(request.ResourceKey)
		if !valid {
			return state, core.ErrReviewResourceConflict
		}
		request.ResourceKey = groupID + ":" + artifactID
		var mirrored int
		err := tx.QueryRow(`SELECT super_team_prefix, domain, mirrored FROM maven_artifacts
			WHERE repository = ? AND group_id = ? AND artifact_id = ?`, request.Repository, groupID, artifactID).Scan(
			&state.binding, &state.domain, &mirrored)
		state.name = request.ResourceKey
		state.local = mirrored == 0
		return state, reviewResourceError(err)
	case core.ReviewResourceMavenDomain:
		request.Repository = ""
		request.ResourceKey = sanitizeMavenDomain(request.ResourceKey)
		err := tx.QueryRow(`SELECT super_team_prefix, domain FROM maven_domains
			WHERE repository = ? AND domain = ?`, globalMavenRepository, request.ResourceKey).Scan(
			&state.binding, &state.name)
		state.local = true
		return state, reviewResourceError(err)
	default:
		return state, core.ErrReviewResourceConflict
	}
}

func reviewResourceError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrReviewResourceConflict
	}
	if err != nil {
		return fmt.Errorf("load review resource: %w", err)
	}
	return nil
}

func mavenArtifactTeamPermissionTx(tx *Tx, repository, resourceKey, userID string) (int, bool, error) {
	groupID, artifactID, valid := splitMavenArtifactReviewKey(resourceKey)
	if !valid {
		return 0, false, core.ErrMavenArtifactNotFound
	}
	var role, member int
	err := tx.QueryRow(`SELECT COALESCE(stm.role_level, 0), CASE WHEN stm.user_id IS NULL THEN 0 ELSE 1 END
		FROM maven_artifacts a LEFT JOIN super_team_members stm
		ON stm.team_prefix = a.super_team_prefix AND stm.user_id = ?
		WHERE a.repository = ? AND a.group_id = ? AND a.artifact_id = ?`,
		userID, repository, groupID, artifactID).Scan(&role, &member)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, core.ErrMavenArtifactNotFound
	}
	if err != nil {
		return 0, false, fmt.Errorf("inspect Maven artifact transfer permission: %w", err)
	}
	level, effectiveMember := effectiveBoundPermission(0, false, role, member != 0)
	return level, effectiveMember, nil
}

func requireTransferRequesterPermissionTx(tx *Tx, request core.SuperTeamTransferRequest,
	resource reviewResourceState, userID string, administrator bool,
) error {
	if administrator {
		return nil
	}
	var level int
	var member bool
	var err error
	switch request.ResourceType {
	case core.ReviewResourceDockerImage:
		level, member, err = dockerEffectivePermissionTx(tx, request.Repository, request.ResourceKey, userID)
	case core.ReviewResourceNPMPackage:
		level, member, err = npmEffectivePermissionTx(tx, request.Repository, request.ResourceKey, userID)
	case core.ReviewResourceCargoPackage:
		level, member, err = cargoEffectivePermissionTx(tx, request.Repository, request.ResourceKey, userID)
	case core.ReviewResourceMavenDomain:
		level, member, err = mavenDomainEffectivePermissionTx(tx, request.ResourceKey, userID)
	case core.ReviewResourceMavenArtifact:
		level, member, err = mavenArtifactTeamPermissionTx(tx, request.Repository, request.ResourceKey, userID)
		if err == nil && member && level >= core.MavenPermissionOwner {
			return nil
		}
		level, member, err = mavenDomainEffectivePermissionTx(tx, resource.domain, userID)
	}
	if err != nil {
		return err
	}
	if !member || level < 4 {
		return core.ErrReviewPermissionDenied
	}
	return nil
}

func reviewUserTx(tx *Tx, userID string) (*config.User, error) {
	var username string
	if err := tx.QueryRow(`SELECT username FROM user_profiles WHERE user_id = ?`, userID).Scan(&username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, core.ErrReviewPermissionDenied
		}
		return nil, fmt.Errorf("load review account profile: %w", err)
	}
	var encoded string
	if err := tx.QueryRow(`SELECT permissions_json FROM tokens WHERE name = ?`, username).Scan(&encoded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, core.ErrReviewPermissionDenied
		}
		return nil, fmt.Errorf("load review account permissions: %w", err)
	}
	permissions := make([]string, 0)
	if encoded != "" {
		if err := json.Unmarshal([]byte(encoded), &permissions); err != nil {
			return nil, fmt.Errorf("decode review account permissions: %w", err)
		}
	}
	legacyManager := false
	for _, permission := range permissions {
		if permission == "m" || permission == "access-token:manager" {
			legacyManager = true
			break
		}
	}
	if legacyManager {
		permissions = append(permissions, "manager")
	}
	return &config.User{Username: username, Roles: permissions}, nil
}

func reviewRequesterAdministratorTx(tx *Tx, userID, repository string) (bool, error) {
	user, err := reviewUserTx(tx, userID)
	if err != nil {
		if errors.Is(err, core.ErrReviewPermissionDenied) {
			return false, nil
		}
		return false, err
	}
	return user.IsManager() || repository != "" && user.CheckUpdatePermission(repository), nil
}

func transferReviewActiveKey(request core.SuperTeamTransferRequest) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		core.ReviewKindSuperTeamTransfer, request.ResourceType, request.Repository,
		request.ResourceKey,
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// CreateSuperTeamTransferReview creates one pending transfer without sending a notification.
func (db *DB) CreateSuperTeamTransferReview(request core.SuperTeamTransferRequest, actor string,
	administrator bool, createdAt int64,
) (*core.ReviewTask, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	resourceType, valid := normalizeReviewResourceType(request.ResourceType)
	if !valid || createdAt <= 0 {
		return nil, core.ErrReviewResourceConflict
	}
	request.ResourceType = resourceType
	target, err := normalizeOptionalSuperTeamPrefix(request.TargetTeamPrefix)
	if err != nil {
		return nil, core.ErrReviewResourceConflict
	}
	request.TargetTeamPrefix = target
	actor = strings.ToLower(strings.TrimSpace(actor))
	actorID, err := db.userIDForExistingAccount(actor)
	if err != nil {
		return nil, core.ErrReviewPermissionDenied
	}
	superTeamMutationLock.Lock()
	defer superTeamMutationLock.Unlock()
	reviewTaskMutationLock.Lock()
	defer reviewTaskMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin global-team transfer review: %w", err)
	}
	defer tx.Rollback()
	resource, err := loadReviewResourceTx(tx, &request)
	if err != nil {
		return nil, err
	}
	if !resource.local {
		return nil, core.ErrReviewResourceConflict
	}
	if err := requireTransferRequesterPermissionTx(tx, request, resource, actorID, administrator); err != nil {
		return nil, err
	}
	reviewTeam := ""
	switch {
	case resource.binding == "" && target != "":
		reviewTeam = target
		if err := requireSuperTeamRoleTx(tx, target, actorID, core.SuperTeamRoleRead); err != nil {
			return nil, core.ErrReviewPermissionDenied
		}
		if err := validateTransferNamespace(request.ResourceType, request.ResourceKey, target, false); err != nil {
			return nil, err
		}
	case resource.binding != "" && target == "":
		reviewTeam = resource.binding
		if err := validateTransferNamespace(request.ResourceType, request.ResourceKey, resource.binding, true); err != nil {
			return nil, err
		}
	default:
		return nil, core.ErrReviewResourceConflict
	}
	activeKey := transferReviewActiveKey(request)
	var existing string
	if err := tx.QueryRow(`SELECT id FROM review_tasks WHERE active_key = ? AND status = ?`,
		activeKey, core.ReviewStatusPending).Scan(&existing); err == nil {
		return nil, core.ErrReviewTaskExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("inspect pending transfer review: %w", err)
	}
	task := &core.ReviewTask{
		ID: uuid.NewString(), Kind: core.ReviewKindSuperTeamTransfer,
		ResourceType: request.ResourceType, Repository: request.Repository, ResourceKey: request.ResourceKey,
		ResourceName: resource.name, SourceTeamPrefix: resource.binding, TargetTeamPrefix: target,
		ReviewTeamPrefix: reviewTeam, RequestedByID: actorID, RequestedBy: actor,
		Status: core.ReviewStatusPending, CreatedAt: createdAt, ActiveKey: activeKey,
	}
	if _, err := tx.Exec(`INSERT INTO review_tasks
		(id, kind, resource_type, repository, resource_key, resource_name, source_team_prefix,
		target_team_prefix, review_team_prefix, requested_by_id, requested_by_name, status,
		decision_reason, decided_by_id, decided_by_name, created_at, decided_at, active_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', '', ?, 0, ?)`, task.ID, task.Kind,
		task.ResourceType, task.Repository, task.ResourceKey, task.ResourceName, task.SourceTeamPrefix,
		task.TargetTeamPrefix, task.ReviewTeamPrefix, task.RequestedByID, task.RequestedBy,
		task.Status, task.CreatedAt, task.ActiveKey); err != nil {
		if lookupErr := tx.QueryRow(`SELECT id FROM review_tasks WHERE active_key = ? AND status = ?`,
			activeKey, core.ReviewStatusPending).Scan(&existing); lookupErr == nil {
			return nil, core.ErrReviewTaskExists
		}
		return nil, fmt.Errorf("create transfer review: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transfer review: %w", err)
	}
	return task, nil
}

func validateTransferNamespace(resourceType, resourceKey, teamPrefix string, transferOut bool) error {
	switch resourceType {
	case core.ReviewResourceDockerImage:
		required, namespaced := core.DockerImageSuperTeamPrefix(resourceKey)
		if transferOut && strings.Contains(resourceKey, "/") {
			return core.ErrReviewTransferRestricted
		}
		if !transferOut && strings.Contains(resourceKey, "/") && (!namespaced || required != teamPrefix) {
			return core.ErrSuperTeamBindingMismatch
		}
	case core.ReviewResourceNPMPackage:
		required, scoped := core.NPMPackageSuperTeamPrefix(resourceKey)
		if transferOut && scoped {
			return core.ErrReviewTransferRestricted
		}
		if !transferOut && scoped && required != teamPrefix {
			return core.ErrSuperTeamBindingMismatch
		}
	}
	return nil
}

// ListReviewTasks returns one requester or T3+ team-reviewer page.
func (db *DB) ListReviewTasks(options core.ReviewTaskListOptions) ([]*core.ReviewTask, int, error) {
	if db == nil || db.SQLDB == nil {
		return nil, 0, core.ErrDatabaseUnavailable
	}
	if options.Limit < 1 || options.Limit > 100 || options.Offset < 0 {
		return nil, 0, errors.New("review task page is invalid")
	}
	userID, err := db.userIDForExistingAccount(options.Username)
	if err != nil {
		return nil, 0, core.ErrReviewPermissionDenied
	}
	status := strings.ToLower(strings.TrimSpace(options.Status))
	if status == "" {
		status = core.ReviewStatusPending
	}
	if status != "all" && status != core.ReviewStatusPending && status != core.ReviewStatusApproved &&
		status != core.ReviewStatusRejected && status != core.ReviewStatusCancelled {
		return nil, 0, errors.New("review task status is invalid")
	}
	resourceTypes := make([]string, 0, len(options.ResourceTypes))
	seen := make(map[string]struct{}, len(options.ResourceTypes))
	for _, candidate := range options.ResourceTypes {
		resourceType, valid := normalizeReviewResourceType(candidate)
		if !valid {
			continue
		}
		if _, duplicate := seen[resourceType]; duplicate {
			continue
		}
		seen[resourceType] = struct{}{}
		resourceTypes = append(resourceTypes, resourceType)
	}
	from := ` FROM review_tasks r` + reviewTaskProfileJoins
	where := make([]string, 0, 3)
	args := make([]any, 0, len(resourceTypes)+4)
	if options.RequestedView {
		where = append(where, "r.requested_by_id = ?")
		args = append(args, userID)
	} else if !options.Administrator {
		from += ` LEFT JOIN super_team_members reviewer ON reviewer.team_prefix = r.review_team_prefix
			AND reviewer.user_id = ? AND reviewer.role_level >= ?`
		args = append(args, userID, core.SuperTeamRoleManage)
		reviewerClauses := []string{"(r.kind = ? AND reviewer.user_id IS NOT NULL)"}
		args = append(args, core.ReviewKindSuperTeamTransfer)
		moderated := make([]string, 0, len(options.ModeratedRepositories))
		seenRepositories := make(map[string]struct{}, len(options.ModeratedRepositories))
		for _, candidate := range options.ModeratedRepositories {
			if len(moderated) >= 256 {
				break
			}
			repository := sanitizeMavenRepository(candidate)
			if repository == "" {
				continue
			}
			if _, exists := seenRepositories[repository]; exists {
				continue
			}
			seenRepositories[repository] = struct{}{}
			moderated = append(moderated, repository)
		}
		if options.ModerateAll {
			reviewerClauses = append(reviewerClauses, "r.kind = ?")
			args = append(args, core.ReviewKindPublication)
		} else if len(moderated) > 0 {
			reviewerClauses = append(reviewerClauses, "(r.kind = ? AND r.repository IN ("+
				strings.TrimSuffix(strings.Repeat("?,", len(moderated)), ",")+"))")
			args = append(args, core.ReviewKindPublication)
			for _, repository := range moderated {
				args = append(args, repository)
			}
		}
		where = append(where, "("+strings.Join(reviewerClauses, " OR ")+")")
	}
	if status != "all" {
		where = append(where, "r.status = ?")
		args = append(args, status)
	}
	if len(resourceTypes) > 0 {
		where = append(where, "r.resource_type IN ("+strings.TrimSuffix(strings.Repeat("?,", len(resourceTypes)), ",")+")")
		for _, resourceType := range resourceTypes {
			args = append(args, resourceType)
		}
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := db.QueryRow(`SELECT COUNT(*)`+from+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count review tasks: %w", err)
	}
	pageArgs := append(append([]any(nil), args...), options.Limit, options.Offset)
	rows, err := db.Query(`SELECT `+reviewTaskSelectColumns+from+whereSQL+`
		ORDER BY r.created_at DESC, r.id DESC LIMIT ? OFFSET ?`, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list review tasks: %w", err)
	}
	defer rows.Close()
	tasks := make([]*core.ReviewTask, 0, min(total, options.Limit))
	for rows.Next() {
		task, scanErr := scanReviewTask(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan review task: %w", scanErr)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate review tasks: %w", err)
	}
	if err := db.hydrateReviewTaskSummaries(tasks); err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func (db *DB) hydrateReviewTaskSummaries(tasks []*core.ReviewTask) error {
	if len(tasks) == 0 {
		return nil
	}
	byID := make(map[string]*core.ReviewTask, len(tasks))
	ids := make([]any, 0, len(tasks))
	for _, task := range tasks {
		if task == nil || task.ID == "" {
			continue
		}
		byID[task.ID] = task
		ids = append(ids, task.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	query := `SELECT task_id, COUNT(*), COALESCE(SUM(size), 0), COALESCE(MAX(added_at), 0)
		FROM review_task_files WHERE task_id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + `)
		GROUP BY task_id`
	rows, err := db.Query(query, ids...)
	if err != nil {
		return fmt.Errorf("summarize review files: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var taskID string
		var count int
		var size, updatedAt int64
		if err := rows.Scan(&taskID, &count, &size, &updatedAt); err != nil {
			return fmt.Errorf("scan review file summary: %w", err)
		}
		if task := byID[taskID]; task != nil {
			task.FileCount = count
			task.TotalSize = size
			if updatedAt > task.UpdatedAt {
				task.UpdatedAt = updatedAt
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate review file summaries: %w", err)
	}
	return nil
}

// GetReviewTask returns one task with its bounded file summary.
func (db *DB) GetReviewTask(id string) (*core.ReviewTask, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	id = SanitizeInputString(strings.TrimSpace(id), 64)
	if id == "" {
		return nil, core.ErrReviewTaskNotFound
	}
	task, err := scanReviewTask(db.QueryRow(`SELECT `+reviewTaskSelectColumns+`
		FROM review_tasks r`+reviewTaskProfileJoins+` WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrReviewTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load review task: %w", err)
	}
	if err := db.hydrateReviewTaskSummaries([]*core.ReviewTask{task}); err != nil {
		return nil, err
	}
	return task, nil
}

func loadReviewTaskTx(tx *Tx, id string) (*core.ReviewTask, error) {
	task, err := scanReviewTask(tx.QueryRow(`SELECT `+reviewTaskSelectColumns+`
		FROM review_tasks r`+reviewTaskProfileJoins+` WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrReviewTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load review task: %w", err)
	}
	return task, nil
}

func applySuperTeamTransferTx(tx *Tx, task *core.ReviewTask) error {
	request := core.SuperTeamTransferRequest{
		ResourceType: task.ResourceType, Repository: task.Repository,
		ResourceKey: task.ResourceKey, TargetTeamPrefix: task.TargetTeamPrefix,
	}
	resource, err := loadReviewResourceTx(tx, &request)
	if err != nil {
		return err
	}
	if !resource.local || resource.binding != task.SourceTeamPrefix {
		return core.ErrReviewResourceConflict
	}
	administrator, err := reviewRequesterAdministratorTx(tx, task.RequestedByID, task.Repository)
	if err != nil {
		return err
	}
	if err := requireTransferRequesterPermissionTx(tx, request, resource,
		task.RequestedByID, administrator); err != nil {
		if reviewResourceStateChanged(err) {
			return errors.Join(core.ErrReviewResourceConflict, err)
		}
		return err
	}
	if task.TargetTeamPrefix != "" {
		if err := requireSuperTeamRoleTx(tx, task.TargetTeamPrefix,
			task.RequestedByID, core.SuperTeamRoleRead); err != nil {
			if errors.Is(err, core.ErrSuperTeamBindingPermission) {
				return errors.Join(core.ErrReviewResourceConflict, err)
			}
			return err
		}
	}
	if err := validateTransferNamespace(task.ResourceType, task.ResourceKey,
		task.ReviewTeamPrefix, task.TargetTeamPrefix == ""); err != nil {
		return err
	}
	var result result
	switch task.ResourceType {
	case core.ReviewResourceDockerImage:
		result, err = tx.Exec(`UPDATE docker_images SET super_team_prefix = ?, updated_at = ?
			WHERE repository = ? AND image_name = ? AND super_team_prefix = ?`, task.TargetTeamPrefix,
			task.DecidedAt, task.Repository, task.ResourceKey, task.SourceTeamPrefix)
	case core.ReviewResourceNPMPackage:
		result, err = tx.Exec(`UPDATE npm_packages SET super_team_prefix = ?, updated_at = ?, revision = revision + 1
			WHERE repository = ? AND package_name = ? AND super_team_prefix = ?`, task.TargetTeamPrefix,
			task.DecidedAt, task.Repository, task.ResourceKey, task.SourceTeamPrefix)
	case core.ReviewResourceCargoPackage:
		result, err = tx.Exec(`UPDATE cargo_packages SET super_team_prefix = ?, updated_at = ?
			WHERE repository = ? AND normalized_name = ? AND super_team_prefix = ?`, task.TargetTeamPrefix,
			task.DecidedAt, task.Repository, task.ResourceKey, task.SourceTeamPrefix)
	case core.ReviewResourceMavenArtifact:
		groupID, artifactID, valid := splitMavenArtifactReviewKey(task.ResourceKey)
		if !valid {
			return core.ErrReviewResourceConflict
		}
		result, err = tx.Exec(`UPDATE maven_artifacts SET super_team_prefix = ?, updated_at = ?
			WHERE repository = ? AND group_id = ? AND artifact_id = ? AND super_team_prefix = ?`,
			task.TargetTeamPrefix, task.DecidedAt, task.Repository, groupID, artifactID, task.SourceTeamPrefix)
	case core.ReviewResourceMavenDomain:
		result, err = tx.Exec(`UPDATE maven_domains SET super_team_prefix = ?
			WHERE repository = ? AND domain = ? AND super_team_prefix = ?`, task.TargetTeamPrefix,
			globalMavenRepository, task.ResourceKey, task.SourceTeamPrefix)
	default:
		return core.ErrReviewResourceConflict
	}
	if err != nil {
		return fmt.Errorf("apply global-team transfer: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count global-team transfer: %w", err)
	}
	if changed != 1 {
		return core.ErrReviewResourceConflict
	}
	return nil
}

func reviewResourceStateChanged(err error) bool {
	return errors.Is(err, core.ErrReviewResourceConflict) ||
		errors.Is(err, core.ErrReviewPermissionDenied) ||
		errors.Is(err, core.ErrReviewTransferRestricted) ||
		errors.Is(err, core.ErrSuperTeamBindingPermission) ||
		errors.Is(err, core.ErrSuperTeamBindingMismatch) ||
		errors.Is(err, core.ErrDockerImageNotFound) ||
		errors.Is(err, core.ErrNPMPackageNotFound) ||
		errors.Is(err, core.ErrCargoPackageNotFound) ||
		errors.Is(err, core.ErrMavenArtifactNotFound) ||
		errors.Is(err, core.ErrMavenDomainNotFound)
}

// DecideReviewTask applies one approval or rejection with a pending-state compare-and-set.
func (db *DB) DecideReviewTask(id, actor, decision, reason string, decidedAt int64) (*core.ReviewTask, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	id = SanitizeInputString(strings.TrimSpace(id), 64)
	decision = strings.ToLower(strings.TrimSpace(decision))
	reason, reasonValid := core.NormalizeSuperTeamText(reason, 512, true)
	if id == "" || decidedAt <= 0 || !reasonValid ||
		(decision != core.ReviewStatusApproved && decision != core.ReviewStatusRejected) ||
		(decision == core.ReviewStatusRejected && reason == "") {
		return nil, core.ErrReviewInvalidRequest
	}
	if decision == core.ReviewStatusApproved {
		reason = ""
	}
	actor = strings.ToLower(strings.TrimSpace(actor))
	actorID, err := db.userIDForExistingAccount(actor)
	if err != nil {
		return nil, core.ErrReviewPermissionDenied
	}
	superTeamMutationLock.Lock()
	defer superTeamMutationLock.Unlock()
	reviewTaskMutationLock.Lock()
	defer reviewTaskMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin review decision: %w", err)
	}
	defer tx.Rollback()
	task, err := loadReviewTaskTx(tx, id)
	if err != nil {
		return nil, err
	}
	if task.Status != core.ReviewStatusPending {
		return nil, core.ErrReviewTaskConflict
	}
	reviewer, err := reviewUserTx(tx, actorID)
	if err != nil {
		return nil, err
	}
	if task.Kind == core.ReviewKindPublication {
		if !reviewer.CheckModeratePermission(task.Repository) {
			return nil, core.ErrReviewPermissionDenied
		}
		var updatedAt int64
		if err := tx.QueryRow(`SELECT COALESCE(MAX(added_at), ?) FROM review_task_files WHERE task_id = ?`,
			task.CreatedAt, task.ID).Scan(&updatedAt); err != nil {
			return nil, fmt.Errorf("inspect publication review activity: %w", err)
		}
		if decidedAt-updatedAt < core.PublicationReviewSettleMillis {
			return nil, core.ErrReviewPublicationActive
		}
	} else if !reviewer.IsManager() {
		if err := requireSuperTeamRoleTx(tx, task.ReviewTeamPrefix, actorID, core.SuperTeamRoleManage); err != nil {
			return nil, core.ErrReviewPermissionDenied
		}
	}
	task.DecidedAt = decidedAt
	task.DecidedByID = actorID
	task.DecidedBy = actor
	task.DecisionReason = reason
	status := decision
	applyErr := error(nil)
	if decision == core.ReviewStatusApproved && task.Kind == core.ReviewKindSuperTeamTransfer {
		applyErr = applySuperTeamTransferTx(tx, task)
		if applyErr != nil {
			if !reviewResourceStateChanged(applyErr) {
				return nil, applyErr
			}
			status = core.ReviewStatusCancelled
			task.DecisionReason = "resource_changed"
		}
	}
	result, err := tx.Exec(`UPDATE review_tasks SET status = ?, decision_reason = ?, decided_by_id = ?,
		decided_by_name = ?, decided_at = ?, active_key = NULL WHERE id = ? AND status = ?`,
		status, task.DecisionReason, actorID, actor, decidedAt, task.ID, core.ReviewStatusPending)
	if err != nil {
		return nil, fmt.Errorf("complete review task: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return nil, core.ErrReviewTaskConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit review decision: %w", err)
	}
	task.Status = status
	if applyErr != nil {
		return task, errors.Join(core.ErrReviewResourceConflict, applyErr)
	}
	return task, nil
}

// CancelReviewTask cancels one pending request owned by the caller.
func (db *DB) CancelReviewTask(id, actor string, cancelledAt int64) (*core.ReviewTask, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	id = SanitizeInputString(strings.TrimSpace(id), 64)
	actorID, err := db.userIDForExistingAccount(actor)
	if err != nil || id == "" || cancelledAt <= 0 {
		return nil, core.ErrReviewPermissionDenied
	}
	reviewTaskMutationLock.Lock()
	defer reviewTaskMutationLock.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin review cancellation: %w", err)
	}
	defer tx.Rollback()
	task, err := loadReviewTaskTx(tx, id)
	if err != nil {
		return nil, err
	}
	if task.Status != core.ReviewStatusPending {
		return nil, core.ErrReviewTaskConflict
	}
	if task.Kind == core.ReviewKindPublication {
		return nil, core.ErrReviewInvalidRequest
	}
	if task.RequestedByID != actorID {
		return nil, core.ErrReviewPermissionDenied
	}
	result, err := tx.Exec(`UPDATE review_tasks SET status = ?, decision_reason = 'request_cancelled',
		decided_by_id = ?, decided_by_name = ?, decided_at = ?, active_key = NULL
		WHERE id = ? AND status = ? AND requested_by_id = ?`, core.ReviewStatusCancelled,
		actorID, strings.ToLower(strings.TrimSpace(actor)), cancelledAt, task.ID,
		core.ReviewStatusPending, actorID)
	if err != nil {
		return nil, fmt.Errorf("cancel review task: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return nil, core.ErrReviewTaskConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit review cancellation: %w", err)
	}
	task.Status = core.ReviewStatusCancelled
	task.DecisionReason = "request_cancelled"
	task.DecidedAt = cancelledAt
	task.DecidedByID = actorID
	task.DecidedBy = strings.ToLower(strings.TrimSpace(actor))
	return task, nil
}
