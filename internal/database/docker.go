/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"renop/internal/core"
)

var dockerTeamRemovalSpec = &teamRemovalSpec{
	format:              "Docker",
	table:               "docker_members",
	resourceColumn:      "image_name",
	manageLevel:         core.DockerPermissionTeam,
	ownerLevel:          core.DockerPermissionOwner,
	invalidBatch:        errors.New("docker member removal batch is invalid"),
	invalidName:         errors.New("docker member name is invalid"),
	resourceNotFound:    core.ErrDockerImageNotFound,
	permissionDenied:    core.ErrDockerPermissionDenied,
	ownerCannotLeave:    core.ErrDockerOwnerCannotLeave,
	lastOwner:           core.ErrDockerLastFullMember,
	lock:                lockDockerImageTeam,
	effectivePermission: dockerEffectivePermissionTx,
}

func sanitizeDockerKey(repository, imageName string) (string, string) {
	repository = strings.ToLower(SanitizeInputString(strings.TrimSpace(repository), 64))
	imageName = strings.ToLower(SanitizeInputString(strings.TrimSpace(imageName), 255))
	return repository, imageName
}

func sanitizeDockerUsername(username string) string {
	return strings.ToLower(SanitizeInputString(strings.TrimSpace(username), 255))
}

func setDockerImageFlags(image *core.DockerRepositoryImage, privateValue, pushEnabledValue int) {
	image.Private = privateValue != 0
	image.PushEnabled = pushEnabledValue != 0
	image.Mirrored = !image.PushEnabled
}

func boundDockerImageDescription(image *core.DockerRepositoryImage, catalog bool) {
	if image == nil {
		return
	}
	if catalog {
		image.Description = strings.TrimSpace(SanitizeInputString(image.Description, 4000))
		return
	}
	image.Description = sanitizePackageReadme(image.Description)
}

// CreateDockerImage reserves an empty image and assigns its initial L4 owner.
func (db *DB) CreateDockerImage(repository, imageName, owner string, private bool, createdAt int64) (*core.DockerRepositoryImage, error) {
	return db.createDockerImage(repository, imageName, owner, "", private, createdAt, false)
}

// CreateDockerImageForTeam reserves an image with an optional global-team binding.
func (db *DB) CreateDockerImageForTeam(repository, imageName, owner, superTeamPrefix string,
	private bool, createdAt int64,
) (*core.DockerRepositoryImage, error) {
	return db.createDockerImage(repository, imageName, owner, superTeamPrefix, private, createdAt, true)
}

func (db *DB) createDockerImage(repository, imageName, owner, superTeamPrefix string,
	private bool, createdAt int64, enforceNamespace bool,
) (*core.DockerRepositoryImage, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	owner = sanitizeDockerUsername(owner)
	var err error
	superTeamPrefix, err = normalizeOptionalSuperTeamPrefix(superTeamPrefix)
	if err != nil {
		return nil, err
	}
	if enforceNamespace {
		requiredPrefix, namespaced := core.DockerImageSuperTeamPrefix(imageName)
		if strings.Contains(imageName, "/") && !namespaced {
			return nil, core.ErrSuperTeamBindingMismatch
		}
		if namespaced && superTeamPrefix == "" {
			return nil, core.ErrSuperTeamBindingRequired
		}
		if namespaced && requiredPrefix != superTeamPrefix {
			return nil, core.ErrSuperTeamBindingMismatch
		}
	}
	if repository == "" || imageName == "" || owner == "" || owner == "guest" || createdAt <= 0 {
		return nil, core.ErrDockerInvalidName
	}
	ownerID, err := db.ensureUserProfile(owner)
	if err != nil {
		return nil, core.ErrDockerPermissionDenied
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin Docker image creation: %w", err)
	}
	defer tx.Rollback()
	image, err := createDockerImageTx(
		tx, repository, imageName, owner, ownerID, superTeamPrefix, private, createdAt,
		core.SuperTeamRoleManage)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit Docker image creation: %w", err)
	}
	return image, nil
}

func createDockerImageTx(tx *Tx, repository, imageName, owner, ownerID, superTeamPrefix string,
	private bool, createdAt int64, requiredTeamRole int,
) (*core.DockerRepositoryImage, error) {
	if tx == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	if err := requireSuperTeamRoleTx(tx, superTeamPrefix, ownerID, requiredTeamRole); err != nil {
		return nil, err
	}
	var existing int
	if err := tx.QueryRow(`SELECT 1 FROM docker_images WHERE repository = ? AND image_name = ?`,
		repository, imageName).Scan(&existing); err == nil {
		return nil, core.ErrDockerImageExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("inspect Docker image creation: %w", err)
	}
	privateValue := 0
	if private {
		privateValue = 1
	}
	if _, err := tx.Exec(`INSERT INTO docker_images
		(repository, image_name, description, publisher, pull_count, super_team_prefix, private, push_enabled, created_at, updated_at)
		VALUES (?, ?, '', ?, 0, ?, ?, 1, ?, ?)`, repository, imageName, owner, superTeamPrefix,
		privateValue, createdAt, createdAt); err != nil {
		return nil, fmt.Errorf("create Docker image: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO docker_members
		(repository, image_name, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
		repository, imageName, owner, ownerID, core.DockerPermissionOwner, createdAt); err != nil {
		return nil, fmt.Errorf("create Docker image owner: %w", err)
	}
	return &core.DockerRepositoryImage{
		Repository: repository, ImageName: imageName, Publisher: owner, Private: private, PushEnabled: true,
		SuperTeamPrefix: superTeamPrefix,
		PermissionLevel: core.DockerPermissionOwner, CreatedAt: createdAt, UpdatedAt: createdAt,
	}, nil
}

// GetDockerImageAccess returns current image visibility and exact membership in one query.
func (db *DB) GetDockerImageAccess(repository, imageName, username string) (exists, private, pushEnabled, member bool, level int, err error) {
	if db == nil || db.SQLDB == nil {
		return false, false, false, false, 0, core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	username = sanitizeDockerUsername(username)
	userID := ""
	if username != "" && username != "guest" {
		userID, err = db.userIDForUsername(username)
		if errors.Is(err, core.ErrUserProfileNotFound) {
			err = nil
			userID = ""
		}
		if err != nil {
			return false, false, false, false, 0, err
		}
	}
	var privateValue, pushEnabledValue, explicitLevel, explicitMember, superRole, superMember int
	err = db.QueryRow(`SELECT i.private, i.push_enabled, COALESCE(m.permission_level, 0),
		CASE WHEN m.user_id IS NULL THEN 0 ELSE 1 END, COALESCE(stm.role_level, 0),
		CASE WHEN stm.user_id IS NULL THEN 0 ELSE 1 END
		FROM docker_images i LEFT JOIN docker_members m ON m.repository = i.repository
		AND m.image_name = i.image_name AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = i.super_team_prefix AND stm.user_id = ?
		WHERE i.repository = ? AND i.image_name = ?`, userID, userID, repository, imageName).Scan(
		&privateValue, &pushEnabledValue, &explicitLevel, &explicitMember, &superRole, &superMember)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, false, false, 0, nil
	}
	if err != nil {
		return false, false, false, false, 0, fmt.Errorf("inspect Docker image access: %w", err)
	}
	level, member = effectiveBoundPermission(explicitLevel, explicitMember != 0, superRole, superMember != 0)
	return true, privateValue != 0, pushEnabledValue != 0, member, level, nil
}

// DockerImageMemberLevels returns effective memberships for a bounded image batch.
func (db *DB) DockerImageMemberLevels(repository, username string, imageNames []string) (map[string]int, error) {
	levels := make(map[string]int, len(imageNames))
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	if len(imageNames) == 0 {
		return levels, nil
	}
	if len(imageNames) > 100 {
		return nil, core.ErrDockerInvalidName
	}
	repository, _ = sanitizeDockerKey(repository, "")
	username = sanitizeDockerUsername(username)
	if repository == "" || username == "" || username == "guest" {
		return levels, nil
	}
	userID, err := db.userIDForUsername(username)
	if errors.Is(err, core.ErrUserProfileNotFound) {
		return levels, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(imageNames))
	for _, imageName := range imageNames {
		_, imageName = sanitizeDockerKey("", imageName)
		if imageName == "" {
			return nil, core.ErrDockerInvalidName
		}
		names = append(names, imageName)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	arguments := make([]any, 0, len(names)+3)
	arguments = append(arguments, userID, userID, repository)
	for _, imageName := range names {
		arguments = append(arguments, imageName)
	}
	rows, err := db.Query(`SELECT i.image_name, COALESCE(m.permission_level, 0),
		CASE WHEN m.user_id IS NULL THEN 0 ELSE 1 END, COALESCE(stm.role_level, 0),
		CASE WHEN stm.user_id IS NULL THEN 0 ELSE 1 END
		FROM docker_images i LEFT JOIN docker_members m ON m.repository = i.repository
		AND m.image_name = i.image_name AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = i.super_team_prefix AND stm.user_id = ?
		WHERE i.repository = ? AND i.image_name IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list Docker image memberships: %w", err)
	}
	for rows.Next() {
		var imageName string
		var explicitLevel, explicitMember, superRole, superMember int
		if err := rows.Scan(&imageName, &explicitLevel, &explicitMember, &superRole, &superMember); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan Docker image membership: %w", err)
		}
		level, member := effectiveBoundPermission(explicitLevel, explicitMember != 0, superRole, superMember != 0)
		if member {
			levels[imageName] = level
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate Docker image memberships: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close Docker image memberships: %w", err)
	}
	return levels, nil
}

func (db *DB) GetDockerImage(repository, imageName string) (*core.DockerRepositoryImage, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	img := &core.DockerRepositoryImage{}
	var privateValue, pushEnabledValue int
	err := db.QueryRow(
		`SELECT repository, image_name, SUBSTR(description, 1, 524288), publisher, pull_count, super_team_prefix,
		private, push_enabled, created_at, updated_at FROM docker_images WHERE repository = ? AND image_name = ?`,
		repository, imageName,
	).Scan(&img.Repository, &img.ImageName, &img.Description, &img.Publisher, &img.PullCount,
		&img.SuperTeamPrefix, &privateValue, &pushEnabledValue, &img.CreatedAt, &img.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get Docker image: %w", err)
	}
	setDockerImageFlags(img, privateValue, pushEnabledValue)
	boundDockerImageDescription(img, false)
	if err := db.hydrateDockerImageMetadata([]*core.DockerRepositoryImage{img}); err != nil {
		return nil, err
	}
	return img, nil
}

func (db *DB) hydrateDockerImageMetadata(images []*core.DockerRepositoryImage) error {
	if len(images) == 0 {
		return nil
	}
	repository := ""
	byName := make(map[string]*core.DockerRepositoryImage, len(images))
	names := make([]string, 0, len(images))
	for _, image := range images {
		if image == nil {
			continue
		}
		if repository == "" {
			repository = image.Repository
		} else if image.Repository != repository {
			return errors.New("docker image metadata batch spans repositories")
		}
		names = append(names, image.ImageName)
		byName[image.ImageName] = image
	}
	if len(names) == 0 {
		return nil
	}
	queryArguments := func(batch []string, prefix ...any) (string, []any) {
		arguments := make([]any, 0, len(prefix)+len(batch))
		arguments = append(arguments, prefix...)
		for _, name := range batch {
			arguments = append(arguments, name)
		}
		return strings.TrimSuffix(strings.Repeat("?,", len(batch)), ","), arguments
	}

	placeholders, arguments := queryArguments(names, repository, core.DockerPermissionOwner)
	ownerRows, err := db.Query(`SELECT m.image_name, COALESCE(p.username, m.username) FROM docker_members m
		LEFT JOIN user_profiles p ON p.user_id = m.user_id
		WHERE m.repository = ? AND m.permission_level = ? AND m.image_name IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return fmt.Errorf("list Docker image owners: %w", err)
	}
	for ownerRows.Next() {
		var imageName, owner string
		if err := ownerRows.Scan(&imageName, &owner); err != nil {
			_ = ownerRows.Close()
			return fmt.Errorf("scan Docker image owner: %w", err)
		}
		if image := byName[imageName]; image != nil && owner != "" {
			image.Publisher = owner
		}
	}
	if err := ownerRows.Err(); err != nil {
		_ = ownerRows.Close()
		return fmt.Errorf("iterate Docker image owners: %w", err)
	}
	if err := ownerRows.Close(); err != nil {
		return fmt.Errorf("close Docker image owners: %w", err)
	}

	placeholders, arguments = queryArguments(names, repository)
	tagRows, err := db.Query(`SELECT image_name, tag, publisher, tag_count FROM (
		SELECT image_name, tag, publisher, COUNT(*) OVER (PARTITION BY image_name) AS tag_count,
			ROW_NUMBER() OVER (PARTITION BY image_name ORDER BY updated_at DESC, tag ASC) AS tag_rank
		FROM docker_tags WHERE repository = ? AND image_name IN (`+placeholders+`)) ranked
		WHERE tag_rank = 1`, arguments...)
	if err != nil {
		return fmt.Errorf("list Docker image tag metadata: %w", err)
	}
	for tagRows.Next() {
		var imageName, tag, publisher string
		var tagCount int
		if err := tagRows.Scan(&imageName, &tag, &publisher, &tagCount); err != nil {
			_ = tagRows.Close()
			return fmt.Errorf("scan Docker image tag metadata: %w", err)
		}
		if image := byName[imageName]; image != nil {
			image.TagCount = tagCount
			image.LatestTag = tag
			if image.Publisher == "" {
				image.Publisher = publisher
			}
		}
	}
	if err := tagRows.Err(); err != nil {
		_ = tagRows.Close()
		return fmt.Errorf("iterate Docker image tag metadata: %w", err)
	}
	if err := tagRows.Close(); err != nil {
		return fmt.Errorf("close Docker image tag metadata: %w", err)
	}

	missingPublishers := make([]string, 0, len(names))
	for _, name := range names {
		if byName[name].Publisher == "" {
			missingPublishers = append(missingPublishers, name)
		}
	}
	if len(missingPublishers) == 0 {
		return nil
	}
	placeholders, arguments = queryArguments(missingPublishers, repository)
	manifestRows, err := db.Query(`SELECT image_name, publisher FROM (
		SELECT image_name, publisher, ROW_NUMBER() OVER (
			PARTITION BY image_name ORDER BY created_at DESC, digest ASC) AS manifest_rank
		FROM docker_manifests WHERE repository = ? AND publisher != '' AND image_name IN (`+placeholders+`)) ranked
		WHERE manifest_rank = 1`, arguments...)
	if err != nil {
		return fmt.Errorf("list Docker image manifest publishers: %w", err)
	}
	for manifestRows.Next() {
		var imageName, publisher string
		if err := manifestRows.Scan(&imageName, &publisher); err != nil {
			_ = manifestRows.Close()
			return fmt.Errorf("scan Docker image manifest publisher: %w", err)
		}
		if image := byName[imageName]; image != nil {
			image.Publisher = publisher
		}
	}
	if err := manifestRows.Err(); err != nil {
		_ = manifestRows.Close()
		return fmt.Errorf("iterate Docker image manifest publishers: %w", err)
	}
	if err := manifestRows.Close(); err != nil {
		return fmt.Errorf("close Docker image manifest publishers: %w", err)
	}
	return nil
}

func (db *DB) IncrementDockerPullCount(repository, imageName string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	_, err := db.Exec(`UPDATE docker_images SET pull_count = pull_count + 1 WHERE repository = ? AND image_name = ?`, repository, imageName)
	if err != nil {
		return fmt.Errorf("increment Docker pull count: %w", err)
	}
	return nil
}

func (db *DB) BatchIncrementDockerPullCount(repository, imageName string, delta int64) error {
	if db == nil || db.SQLDB == nil || delta <= 0 {
		return nil
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	_, err := db.Exec(`UPDATE docker_images SET pull_count = pull_count + ? WHERE repository = ? AND image_name = ?`, delta, repository, imageName)
	if err != nil {
		return fmt.Errorf("batch increment Docker pull count: %w", err)
	}
	return nil
}

func (db *DB) UpdateDockerImageDescription(repository, imageName, description string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	description = sanitizePackageReadme(description)
	now := time.Now().UnixMilli()

	res, err := db.Exec(
		`UPDATE docker_images SET description = ?, updated_at = ? WHERE repository = ? AND image_name = ?`,
		description, now, repository, imageName,
	)
	if err != nil {
		return fmt.Errorf("update Docker image description: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return core.ErrDockerImageNotFound
	}
	return nil
}

func (db *DB) ListDockerImages(repository, last string, limit int) ([]*core.DockerRepositoryImage, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeDockerKey(repository, "")
	last = strings.ToLower(SanitizeInputString(strings.TrimSpace(last), 255))
	if limit < 1 || limit > 100 {
		limit = 50
	}

	query := `SELECT repository, image_name, SUBSTR(description, 1, 4000), publisher, pull_count,
		super_team_prefix, private, push_enabled, created_at, updated_at FROM docker_images WHERE repository = ?`
	args := []any{repository}
	if last != "" {
		query += ` AND image_name > ?`
		args = append(args, last)
	}
	query += ` ORDER BY image_name LIMIT ?`
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list Docker images: %w", err)
	}
	defer rows.Close()

	images := make([]*core.DockerRepositoryImage, 0, limit)
	for rows.Next() {
		img := &core.DockerRepositoryImage{}
		var privateValue, pushEnabledValue int
		if err := rows.Scan(&img.Repository, &img.ImageName, &img.Description, &img.Publisher, &img.PullCount,
			&img.SuperTeamPrefix, &privateValue, &pushEnabledValue, &img.CreatedAt, &img.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan Docker image: %w", err)
		}
		setDockerImageFlags(img, privateValue, pushEnabledValue)
		boundDockerImageDescription(img, true)
		images = append(images, img)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Docker images: %w", err)
	}

	if err := db.hydrateDockerImageMetadata(images); err != nil {
		return nil, err
	}
	return images, nil
}

func (db *DB) SearchDockerImages(repository, query string, limit, offset int) ([]*core.DockerRepositoryImage, int, error) {
	if db == nil || db.SQLDB == nil {
		return nil, 0, core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeDockerKey(repository, "")
	query = strings.ToLower(SanitizeInputString(strings.TrimSpace(query), 128))
	query = strings.NewReplacer("%", "", "_", "").Replace(query)
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 || offset > 1000000 {
		offset = 0
	}

	pattern := "%" + query + "%"
	where := `repository = ? AND (image_name LIKE ? OR LOWER(description) LIKE ?)`
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM docker_images WHERE `+where, repository, pattern, pattern).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count Docker search results: %w", err)
	}

	rows, err := db.Query(
		`SELECT repository, image_name, SUBSTR(description, 1, 4000), publisher, pull_count,
			super_team_prefix, private, push_enabled, created_at, updated_at FROM docker_images WHERE `+where+
			` ORDER BY CASE WHEN image_name = ? THEN 0 ELSE 1 END, image_name LIMIT ? OFFSET ?`,
		repository, pattern, pattern, query, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("search Docker images: %w", err)
	}
	defer rows.Close()

	images := make([]*core.DockerRepositoryImage, 0, limit)
	for rows.Next() {
		img := &core.DockerRepositoryImage{}
		var privateValue, pushEnabledValue int
		if err := rows.Scan(&img.Repository, &img.ImageName, &img.Description, &img.Publisher, &img.PullCount,
			&img.SuperTeamPrefix, &privateValue, &pushEnabledValue, &img.CreatedAt, &img.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan Docker search result: %w", err)
		}
		setDockerImageFlags(img, privateValue, pushEnabledValue)
		boundDockerImageDescription(img, true)
		images = append(images, img)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate Docker search results: %w", err)
	}

	if err := db.hydrateDockerImageMetadata(images); err != nil {
		return nil, 0, err
	}
	return images, total, nil
}

func (db *DB) GetDockerImageDetails(repository, imageName string, username ...string) (*core.DockerImageDetails, error) {
	img, err := db.GetDockerImage(repository, imageName)
	if err != nil || img == nil {
		return nil, err
	}

	tags, err := db.ListDockerTags(repository, imageName, "", 100)
	if err != nil {
		return nil, err
	}

	if len(tags) == 0 {
		mRows, mErr := db.Query(
			`SELECT repository, image_name, digest, media_type, size, config_digest, publisher, created_at
			 FROM docker_manifests WHERE repository = ? AND image_name = ? ORDER BY created_at DESC LIMIT 50`,
			repository, imageName,
		)
		if mErr == nil {
			defer mRows.Close()
			for mRows.Next() {
				m := &core.DockerTag{}
				if scanErr := mRows.Scan(&m.Repository, &m.ImageName, &m.Digest, &m.MediaType, &m.Size, &m.ConfigDigest, &m.Publisher, &m.CreatedAt); scanErr == nil {
					m.Tag = m.Digest
					if len(m.Digest) > 19 {
						m.Tag = m.Digest[:19]
					}
					m.UpdatedAt = m.CreatedAt
					tags = append(tags, m)
				}
			}
		}
	}

	members, _ := db.ListDockerMembers(repository, imageName)
	for _, m := range members {
		if m.Level == core.DockerPermissionOwner {
			img.Publisher = m.Username
			break
		}
	}

	var permissionLevel int
	if len(username) > 0 && username[0] != "" {
		u := sanitizeDockerUsername(username[0])
		if u != "" && u != "guest" {
			permissionLevel, _ = db.GetDockerMemberLevel(repository, imageName, u)
		}
	}

	details := &core.DockerImageDetails{
		Image:           img,
		Tags:            tags,
		Members:         members,
		PermissionLevel: permissionLevel,
	}

	if len(tags) > 0 {
		var totalSize int64
		for _, t := range tags {
			if t.Size > totalSize {
				totalSize = t.Size
			}
		}
		details.TotalSize = totalSize

		latestManifest, _ := db.GetDockerManifest(repository, imageName, tags[0].Digest)
		if latestManifest != nil {
			details.Manifest = latestManifest
		}
	}

	return details, nil
}

func (db *DB) GetDockerTag(repository, imageName, tag string) (*core.DockerTag, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	tag = SanitizeInputString(strings.TrimSpace(tag), 128)
	t := &core.DockerTag{}
	err := db.QueryRow(
		`SELECT repository, image_name, tag, digest, media_type, size, config_digest, publisher, created_at, updated_at
		 FROM docker_tags WHERE repository = ? AND image_name = ? AND tag = ?`,
		repository, imageName, tag,
	).Scan(&t.Repository, &t.ImageName, &t.Tag, &t.Digest, &t.MediaType, &t.Size, &t.ConfigDigest, &t.Publisher, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get Docker tag: %w", err)
	}
	return t, nil
}

func (db *DB) ListDockerTags(repository, imageName, last string, limit int) ([]*core.DockerTag, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	last = SanitizeInputString(strings.TrimSpace(last), 128)
	if limit < 1 || limit > 100 {
		limit = 50
	}

	query := `SELECT repository, image_name, tag, digest, media_type, size, config_digest, publisher, created_at, updated_at
	          FROM docker_tags WHERE repository = ? AND image_name = ?`
	args := []any{repository, imageName}
	if last != "" {
		query += ` AND tag > ?`
		args = append(args, last)
	}
	query += ` ORDER BY tag LIMIT ?`
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list Docker tags: %w", err)
	}
	defer rows.Close()

	tags := make([]*core.DockerTag, 0, limit)
	for rows.Next() {
		t := &core.DockerTag{}
		if err := rows.Scan(&t.Repository, &t.ImageName, &t.Tag, &t.Digest, &t.MediaType, &t.Size, &t.ConfigDigest, &t.Publisher, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan Docker tag: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Docker tags: %w", err)
	}

	for _, t := range tags {
		if t.Publisher == "" {
			var manPub string
			_ = db.QueryRow(`SELECT publisher FROM docker_manifests WHERE repository = ? AND image_name = ? AND digest = ? AND publisher != '' LIMIT 1`, t.Repository, t.ImageName, t.Digest).Scan(&manPub)
			t.Publisher = manPub
		}
	}

	return tags, nil
}

func (db *DB) GetDockerManifest(repository, imageName, digest string) (*core.DockerManifest, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	digest = strings.ToLower(SanitizeInputString(strings.TrimSpace(digest), 128))
	m := &core.DockerManifest{}
	var rawJSON string
	err := db.QueryRow(
		`SELECT repository, image_name, digest, media_type, size, config_digest, publisher, raw_json, created_at
		 FROM docker_manifests WHERE repository = ? AND image_name = ? AND digest = ?`,
		repository, imageName, digest,
	).Scan(&m.Repository, &m.ImageName, &m.Digest, &m.MediaType, &m.Size, &m.ConfigDigest, &m.Publisher, &rawJSON, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get Docker manifest: %w", err)
	}
	m.RawJSON = []byte(rawJSON)
	return m, nil
}

func (db *DB) PutDockerManifest(manifest *core.DockerManifest, tag string, username string) error {
	return db.putDockerManifest(manifest, tag, username, false)
}

// CacheDockerManifest records a manifest fetched through a configured mirror.
// Unlike client publication, mirror caching may create an unowned public image.
func (db *DB) CacheDockerManifest(manifest *core.DockerManifest, tag string) error {
	return db.putDockerManifest(manifest, tag, "mirror", true)
}

type dockerManifestWrite struct {
	manifest          *core.DockerManifest
	repository        string
	imageName         string
	digest            string
	mediaType         string
	configDigest      string
	tag               string
	username          string
	now               int64
	allowMirrorCreate bool
}

func normalizeDockerManifestWrite(manifest *core.DockerManifest, tag, username string,
	allowMirrorCreate bool, now int64,
) (*dockerManifestWrite, error) {
	if manifest == nil {
		return nil, errors.New("missing Docker manifest")
	}
	repository, imageName := sanitizeDockerKey(manifest.Repository, manifest.ImageName)
	write := &dockerManifestWrite{
		manifest: manifest, repository: repository, imageName: imageName,
		digest:       strings.ToLower(SanitizeInputString(strings.TrimSpace(manifest.Digest), 128)),
		mediaType:    SanitizeInputString(strings.TrimSpace(manifest.MediaType), 128),
		configDigest: strings.ToLower(SanitizeInputString(strings.TrimSpace(manifest.ConfigDigest), 128)),
		tag:          SanitizeInputString(strings.TrimSpace(tag), 128), username: sanitizeDockerUsername(username),
		now: now, allowMirrorCreate: allowMirrorCreate,
	}
	if write.repository == "" || write.imageName == "" || write.digest == "" ||
		write.mediaType == "" || write.username == "" || now <= 0 {
		return nil, core.ErrDockerManifestInvalid
	}
	return write, nil
}

func (db *DB) putDockerManifest(manifest *core.DockerManifest, tag string, username string, allowMirrorCreate bool) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	write, err := normalizeDockerManifestWrite(manifest, tag, username, allowMirrorCreate, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Docker manifest transaction: %w", err)
	}
	defer tx.Rollback()
	if err := putDockerManifestTx(tx, write); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Docker manifest: %w", err)
	}
	return nil
}

func putDockerManifestTx(tx *Tx, write *dockerManifestWrite) error {
	if tx == nil || write == nil || write.manifest == nil {
		return core.ErrDatabaseUnavailable
	}
	manifest := write.manifest
	repository, imageName := write.repository, write.imageName
	digest, mediaType, configDigest := write.digest, write.mediaType, write.configDigest
	tag, username, now := write.tag, write.username, write.now
	allowMirrorCreate := write.allowMirrorCreate
	var err error

	var pushEnabled int
	err = tx.QueryRow(`SELECT push_enabled FROM docker_images WHERE repository = ? AND image_name = ?`, repository, imageName).Scan(&pushEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		if !allowMirrorCreate {
			return core.ErrDockerImageNotFound
		}
		if _, err = tx.Exec(
			`INSERT INTO docker_images (repository, image_name, description, publisher, pull_count, private, push_enabled, created_at, updated_at) VALUES (?, ?, '', ?, 0, 0, 0, ?, ?)`,
			repository, imageName, username, now, now,
		); err != nil {
			return fmt.Errorf("insert Docker image: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("check Docker image: %w", err)
	} else {
		if allowMirrorCreate && pushEnabled != 0 {
			return core.ErrDockerImageExists
		}
		if !allowMirrorCreate && pushEnabled == 0 {
			return core.ErrDockerImageNotFound
		}
		if username != "" {
			if _, err = tx.Exec(`UPDATE docker_images SET publisher = CASE WHEN publisher = '' THEN ? ELSE publisher END, updated_at = ? WHERE repository = ? AND image_name = ?`, username, now, repository, imageName); err != nil {
				return fmt.Errorf("update Docker image timestamp: %w", err)
			}
		} else {
			if _, err = tx.Exec(`UPDATE docker_images SET updated_at = ? WHERE repository = ? AND image_name = ?`, now, repository, imageName); err != nil {
				return fmt.Errorf("update Docker image timestamp: %w", err)
			}
		}
	}

	// Upsert docker_manifests record
	var existingManifest int
	err = tx.QueryRow(`SELECT 1 FROM docker_manifests WHERE repository = ? AND image_name = ? AND digest = ?`, repository, imageName, digest).Scan(&existingManifest)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err = tx.Exec(
			`INSERT INTO docker_manifests (repository, image_name, digest, media_type, size, config_digest, publisher, raw_json, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			repository, imageName, digest, mediaType, manifest.Size, configDigest, username, string(manifest.RawJSON), now,
		); err != nil {
			return fmt.Errorf("insert Docker manifest: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("check Docker manifest: %w", err)
	} else {
		if username != "" {
			if _, err = tx.Exec(
				`UPDATE docker_manifests SET media_type = ?, size = ?, config_digest = ?, publisher = ?, raw_json = ?
				 WHERE repository = ? AND image_name = ? AND digest = ?`,
				mediaType, manifest.Size, configDigest, username, string(manifest.RawJSON), repository, imageName, digest,
			); err != nil {
				return fmt.Errorf("update Docker manifest: %w", err)
			}
		} else {
			if _, err = tx.Exec(
				`UPDATE docker_manifests SET media_type = ?, size = ?, config_digest = ?, raw_json = ?
				 WHERE repository = ? AND image_name = ? AND digest = ?`,
				mediaType, manifest.Size, configDigest, string(manifest.RawJSON), repository, imageName, digest,
			); err != nil {
				return fmt.Errorf("update Docker manifest: %w", err)
			}
		}
	}

	// If tag is supplied and not a digest, upsert docker_tags
	if tag != "" && !strings.HasPrefix(tag, "sha256:") {
		var existingTag int
		err = tx.QueryRow(`SELECT 1 FROM docker_tags WHERE repository = ? AND image_name = ? AND tag = ?`, repository, imageName, tag).Scan(&existingTag)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err = tx.Exec(
				`INSERT INTO docker_tags (repository, image_name, tag, digest, media_type, size, config_digest, publisher, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				repository, imageName, tag, digest, mediaType, manifest.Size, configDigest, username, now, now,
			); err != nil {
				return fmt.Errorf("insert Docker tag: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("check Docker tag: %w", err)
		} else {
			if username != "" {
				if _, err = tx.Exec(
					`UPDATE docker_tags SET digest = ?, media_type = ?, size = ?, config_digest = ?, publisher = ?, updated_at = ?
					 WHERE repository = ? AND image_name = ? AND tag = ?`,
					digest, mediaType, manifest.Size, configDigest, username, now, repository, imageName, tag,
				); err != nil {
					return fmt.Errorf("update Docker tag: %w", err)
				}
			} else {
				if _, err = tx.Exec(
					`UPDATE docker_tags SET digest = ?, media_type = ?, size = ?, config_digest = ?, updated_at = ?
					 WHERE repository = ? AND image_name = ? AND tag = ?`,
					digest, mediaType, manifest.Size, configDigest, now, repository, imageName, tag,
				); err != nil {
					return fmt.Errorf("update Docker tag: %w", err)
				}
			}
		}
	}
	if _, err := tx.Exec(`DELETE FROM docker_image_blobs
		WHERE repository = ? AND image_name = ? AND manifest_digest = ?`, repository, imageName, digest); err != nil {
		return fmt.Errorf("replace Docker manifest blob links: %w", err)
	}
	seenBlobs := make(map[string]struct{}, len(manifest.BlobDigests))
	for _, candidate := range manifest.BlobDigests {
		blobDigest := strings.ToLower(SanitizeInputString(strings.TrimSpace(candidate), 128))
		if blobDigest == "" {
			continue
		}
		if _, duplicate := seenBlobs[blobDigest]; duplicate {
			continue
		}
		seenBlobs[blobDigest] = struct{}{}
		if _, err := tx.Exec(`DELETE FROM docker_image_blobs
			WHERE repository = ? AND image_name = ? AND manifest_digest = '' AND blob_digest = ?`,
			repository, imageName, blobDigest); err != nil {
			return fmt.Errorf("promote Docker upload blob link: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO docker_image_blobs
			(repository, image_name, manifest_digest, blob_digest) VALUES (?, ?, ?, ?)`,
			repository, imageName, digest, blobDigest); err != nil {
			return fmt.Errorf("record Docker manifest blob link: %w", err)
		}
	}

	return nil
}

func (db *DB) DeleteDockerTag(repository, imageName, tag string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	tag = SanitizeInputString(strings.TrimSpace(tag), 128)
	res, err := db.Exec(`DELETE FROM docker_tags WHERE repository = ? AND image_name = ? AND tag = ?`, repository, imageName, tag)
	if err != nil {
		return fmt.Errorf("delete Docker tag: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return core.ErrDockerTagNotFound
	}
	return nil
}

func (db *DB) DeleteDockerManifest(repository, imageName, digest string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	digest = strings.ToLower(SanitizeInputString(strings.TrimSpace(digest), 128))
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete Docker manifest: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM docker_tags WHERE repository = ? AND image_name = ? AND digest = ?`, repository, imageName, digest); err != nil {
		return fmt.Errorf("delete Docker tags for manifest: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM docker_image_blobs WHERE repository = ? AND image_name = ? AND manifest_digest = ?`,
		repository, imageName, digest); err != nil {
		return fmt.Errorf("delete Docker manifest blob links: %w", err)
	}

	res, err := tx.Exec(`DELETE FROM docker_manifests WHERE repository = ? AND image_name = ? AND digest = ?`, repository, imageName, digest)
	if err != nil {
		return fmt.Errorf("delete Docker manifest: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return core.ErrDockerManifestNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete Docker manifest: %w", err)
	}
	return nil
}

func (db *DB) DeleteDockerImage(repository, imageName string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	now := time.Now().UnixMilli()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete Docker image: %w", err)
	}
	defer tx.Rollback()

	_ = cancelDockerInvitations(tx, `repository = ? AND image_name = ?`, []any{repository, imageName}, now)

	if _, err := tx.Exec(`DELETE FROM docker_members WHERE repository = ? AND image_name = ?`, repository, imageName); err != nil {
		return fmt.Errorf("delete Docker members for image: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM docker_image_blobs WHERE repository = ? AND image_name = ?`, repository, imageName); err != nil {
		return fmt.Errorf("delete Docker blob links for image: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM docker_tags WHERE repository = ? AND image_name = ?`, repository, imageName); err != nil {
		return fmt.Errorf("delete Docker tags for image: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM docker_manifests WHERE repository = ? AND image_name = ?`, repository, imageName); err != nil {
		return fmt.Errorf("delete Docker manifests for image: %w", err)
	}
	res, err := tx.Exec(`DELETE FROM docker_images WHERE repository = ? AND image_name = ?`, repository, imageName)
	if err != nil {
		return fmt.Errorf("delete Docker image: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return core.ErrDockerImageNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete Docker image: %w", err)
	}
	return nil
}

func (db *DB) DeleteDockerRepository(repository string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeDockerKey(repository, "")
	now := time.Now().UnixMilli()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete Docker repo: %w", err)
	}
	defer tx.Rollback()

	_ = cancelDockerInvitations(tx, `repository = ?`, []any{repository}, now)

	for _, table := range []string{"docker_image_blobs", "docker_members", "docker_tags", "docker_manifests", "docker_images", "docker_blobs"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE repository = ?`, repository); err != nil {
			return fmt.Errorf("clean Docker repository from %s: %w", table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete Docker repo: %w", err)
	}
	return nil
}

func (db *DB) RecordDockerBlob(repository, digest string, size int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeDockerKey(repository, "")
	digest = strings.ToLower(SanitizeInputString(strings.TrimSpace(digest), 128))
	now := time.Now().UnixMilli()

	var existing int
	err := db.QueryRow(`SELECT 1 FROM docker_blobs WHERE repository = ? AND digest = ?`, repository, digest).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = db.Exec(`INSERT INTO docker_blobs (repository, digest, size, created_at) VALUES (?, ?, ?, ?)`, repository, digest, size, now)
		if err != nil {
			return fmt.Errorf("insert Docker blob: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("check Docker blob: %w", err)
	}
	return nil
}

// RecordDockerImageBlob links an uploaded or mounted blob to its target image before manifest publication.
func (db *DB) RecordDockerImageBlob(repository, imageName, digest string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	digest = strings.ToLower(SanitizeInputString(strings.TrimSpace(digest), 128))
	if repository == "" || imageName == "" || digest == "" {
		return core.ErrDockerInvalidDigest
	}
	var exists int
	err := db.QueryRow(`SELECT 1 FROM docker_image_blobs
		WHERE repository = ? AND image_name = ? AND manifest_digest = '' AND blob_digest = ?`,
		repository, imageName, digest).Scan(&exists)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect Docker upload blob link: %w", err)
	}
	if _, err := db.Exec(`INSERT INTO docker_image_blobs
		(repository, image_name, manifest_digest, blob_digest) VALUES (?, ?, '', ?)`,
		repository, imageName, digest); err != nil {
		return fmt.Errorf("record Docker upload blob link: %w", err)
	}
	return nil
}

func (db *DB) HasDockerBlob(repository, digest string) (bool, int64, error) {
	if db == nil || db.SQLDB == nil {
		return false, 0, core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeDockerKey(repository, "")
	digest = strings.ToLower(SanitizeInputString(strings.TrimSpace(digest), 128))
	var size int64
	err := db.QueryRow(`SELECT size FROM docker_blobs WHERE repository = ? AND digest = ?`, repository, digest).Scan(&size)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("check Docker blob existence: %w", err)
	}
	return true, size, nil
}

// DockerImageReferencesBlob reports whether a manifest in an image uses a blob.
func (db *DB) DockerImageReferencesBlob(repository, imageName, digest string) (bool, error) {
	if db == nil || db.SQLDB == nil {
		return false, core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	digest = strings.ToLower(SanitizeInputString(strings.TrimSpace(digest), 128))
	var exists int
	err := db.QueryRow(`SELECT 1 FROM docker_image_blobs
		WHERE repository = ? AND image_name = ? AND blob_digest = ? LIMIT 1`,
		repository, imageName, digest).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect Docker image blob reference: %w", err)
	}
	return true, nil
}

func (db *DB) DeleteDockerBlob(repository, digest string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeDockerKey(repository, "")
	digest = strings.ToLower(SanitizeInputString(strings.TrimSpace(digest), 128))
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete Docker blob: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM docker_image_blobs WHERE repository = ? AND blob_digest = ?`, repository, digest); err != nil {
		return fmt.Errorf("delete Docker blob references: %w", err)
	}
	_, err = tx.Exec(`DELETE FROM docker_blobs WHERE repository = ? AND digest = ?`, repository, digest)
	if err != nil {
		return fmt.Errorf("delete Docker blob: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete Docker blob: %w", err)
	}
	return nil
}

func (db *DB) GetDockerRepositoryStats(repository string) (totalImages int64, totalTags int64, totalSize int64, err error) {
	if db == nil || db.SQLDB == nil {
		return 0, 0, 0, core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeDockerKey(repository, "")
	_ = db.QueryRow(`SELECT COUNT(*) FROM docker_images WHERE repository = ?`, repository).Scan(&totalImages)
	_ = db.QueryRow(`SELECT COUNT(*) FROM docker_tags WHERE repository = ?`, repository).Scan(&totalTags)
	_ = db.QueryRow(`SELECT COALESCE(SUM(size), 0) FROM docker_blobs WHERE repository = ?`, repository).Scan(&totalSize)
	return totalImages, totalTags, totalSize, nil
}

func (db *DB) HasDockerMembership(repository, username string) (bool, error) {
	if db == nil || db.SQLDB == nil {
		return false, core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeDockerKey(repository, "")
	username = sanitizeDockerUsername(username)
	if username == "" {
		return false, nil
	}
	userID, err := db.userIDForUsername(username)
	if errors.Is(err, core.ErrUserProfileNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var exists int
	err = db.QueryRow(`SELECT 1 FROM docker_images i
		LEFT JOIN docker_members m ON m.repository = i.repository AND m.image_name = i.image_name AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = i.super_team_prefix AND stm.user_id = ?
		WHERE i.repository = ? AND (m.user_id IS NOT NULL OR stm.user_id IS NOT NULL) LIMIT 1`,
		userID, userID, repository).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check Docker membership: %w", err)
	}
	return true, nil
}

func (db *DB) GetDockerMemberLevel(repository, imageName, username string) (int, error) {
	if db == nil || db.SQLDB == nil {
		return 0, core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	username = sanitizeDockerUsername(username)
	if username == "" {
		return core.DockerPermissionRead, nil
	}
	userID, identityErr := db.userIDForUsername(username)
	if errors.Is(identityErr, core.ErrUserProfileNotFound) {
		return core.DockerPermissionRead, nil
	}
	if identityErr != nil {
		return core.DockerPermissionRead, identityErr
	}
	var publisher string
	var explicitLevel, explicitMember, superRole, superMember int
	err := db.QueryRow(`SELECT i.publisher, COALESCE(m.permission_level, 0),
		CASE WHEN m.user_id IS NULL THEN 0 ELSE 1 END, COALESCE(stm.role_level, 0),
		CASE WHEN stm.user_id IS NULL THEN 0 ELSE 1 END
		FROM docker_images i LEFT JOIN docker_members m ON m.repository = i.repository
		AND m.image_name = i.image_name AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = i.super_team_prefix AND stm.user_id = ?
		WHERE i.repository = ? AND i.image_name = ?`, userID, userID, repository, imageName).Scan(
		&publisher, &explicitLevel, &explicitMember, &superRole, &superMember)
	if errors.Is(err, sql.ErrNoRows) {
		return core.DockerPermissionRead, nil
	}
	if err != nil {
		return core.DockerPermissionRead, fmt.Errorf("inspect Docker member level: %w", err)
	}
	level, member := effectiveBoundPermission(explicitLevel, explicitMember != 0, superRole, superMember != 0)
	if !member && strings.EqualFold(publisher, username) {
		return core.DockerPermissionFull, nil
	}
	return level, nil
}

func (db *DB) ListDockerMembers(repository, imageName string) ([]*core.DockerMember, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	rows, err := db.Query(
		`SELECT m.user_id, COALESCE(p.username, m.username), m.permission_level, m.added_at FROM docker_members m
		 LEFT JOIN user_profiles p ON p.user_id = m.user_id
		 WHERE m.repository = ? AND m.image_name = ? ORDER BY m.permission_level DESC, COALESCE(p.username, m.username) ASC`,
		repository, imageName,
	)
	if err != nil {
		return nil, fmt.Errorf("list Docker members: %w", err)
	}
	defer rows.Close()

	members := make([]*core.DockerMember, 0)
	hasPublisher := false
	var pub string
	var createdAt int64
	_ = db.QueryRow(`SELECT publisher, created_at FROM docker_images WHERE repository = ? AND image_name = ?`, repository, imageName).Scan(&pub, &createdAt)

	for rows.Next() {
		m := &core.DockerMember{}
		if err := rows.Scan(&m.UserID, &m.Username, &m.Level, &m.AddedAt); err != nil {
			return nil, fmt.Errorf("scan Docker member: %w", err)
		}
		if pub != "" && m.Username == pub {
			hasPublisher = true
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Docker members: %w", err)
	}

	if pub != "" && !hasPublisher {
		if createdAt == 0 {
			createdAt = time.Now().UnixMilli()
		}
		userID, identityErr := db.ensureUserProfile(pub)
		if identityErr != nil {
			return nil, fmt.Errorf("resolve Docker publisher identity: %w", identityErr)
		}
		if _, err := db.Exec(`INSERT INTO docker_members (repository, image_name, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
			repository, imageName, pub, userID, core.DockerPermissionFull, createdAt); err != nil {
			return nil, fmt.Errorf("restore Docker publisher membership: %w", err)
		}
		members = append([]*core.DockerMember{{
			UserID:   userID,
			Username: pub,
			Level:    core.DockerPermissionFull,
			AddedAt:  createdAt,
		}}, members...)
	}

	return members, nil
}

func (db *DB) CreateDockerInvitations(invitations []*core.DockerInvitation, messages []*core.UserMessage) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(invitations) == 0 || len(invitations) != len(messages) || len(invitations) > 20 {
		return errors.New("docker invitation is missing")
	}
	for i, invitation := range invitations {
		message := messages[i]
		if invitation == nil || message == nil {
			return errors.New("docker invitation is missing")
		}
		invitation.Repository, invitation.ImageName = sanitizeDockerKey(invitation.Repository, invitation.ImageName)
		invitation.Inviter = sanitizeDockerUsername(invitation.Inviter)
		invitation.Recipient = sanitizeDockerUsername(invitation.Recipient)
		if invitation.Level < core.DockerPermissionRead || invitation.Level > core.DockerPermissionOwner {
			return errors.New("docker invitation permission level is invalid")
		}
		if invitation.ID == "" || invitation.ID != message.ID || invitation.Recipient != strings.ToLower(strings.TrimSpace(message.Recipient)) {
			return errors.New("docker invitation message does not match its workflow record")
		}
		if err := normalizeMessage(message); err != nil {
			return err
		}
	}
	first := invitations[0]
	inviterID, identityErr := db.ensureUserProfile(first.Inviter)
	if identityErr != nil {
		return core.ErrDockerPermissionDenied
	}
	recipientIDs := make(map[string]string, len(invitations))
	for _, invitation := range invitations {
		recipientID, err := db.ensureUserProfile(invitation.Recipient)
		if err != nil {
			return core.ErrDockerPermissionDenied
		}
		recipientIDs[invitation.Recipient] = recipientID
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Docker invitation: %w", err)
	}
	defer tx.Rollback()

	if err := lockDockerImageTeam(tx, first.Repository, first.ImageName); err != nil {
		return err
	}
	inviterLevel, inviterMember, err := dockerEffectivePermissionTx(tx, first.Repository, first.ImageName, inviterID)
	if err != nil {
		return err
	}
	if !inviterMember {
		var pub string
		var createdAt int64
		_ = tx.QueryRow(`SELECT publisher, created_at FROM docker_images WHERE repository = ? AND image_name = ?`, first.Repository, first.ImageName).Scan(&pub, &createdAt)
		if pub != "" && pub == first.Inviter {
			inviterLevel = core.DockerPermissionOwner
			if createdAt == 0 {
				createdAt = first.CreatedAt
			}
			if _, err := tx.Exec(`INSERT INTO docker_members (repository, image_name, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
				first.Repository, first.ImageName, first.Inviter, inviterID, core.DockerPermissionOwner, createdAt); err != nil {
				return fmt.Errorf("restore Docker inviter membership: %w", err)
			}
		} else {
			return core.ErrDockerPermissionDenied
		}
	} else if inviterLevel < core.DockerPermissionTeam {
		return core.ErrDockerPermissionDenied
	}

	for i, invitation := range invitations {
		message := messages[i]
		if invitation.Repository != first.Repository || invitation.ImageName != first.ImageName || invitation.Inviter != first.Inviter {
			return errors.New("docker invitation batch targets multiple images")
		}
		if invitation.Level == core.DockerPermissionOwner && inviterLevel < core.DockerPermissionOwner {
			return core.ErrDockerPermissionDenied
		}
		recipientID := recipientIDs[invitation.Recipient]
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM docker_members WHERE repository = ? AND image_name = ? AND user_id = ?`,
			invitation.Repository, invitation.ImageName, recipientID).Scan(&exists); err == nil {
			return core.ErrDockerMemberExists
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect Docker invitation recipient: %w", err)
		}

		var existingID, existingStatus string
		var existingExpiry int64
		var existingInviterLevel int
		err := tx.QueryRow(`SELECT i.id, COALESCE(m.action_status, ''), COALESCE(m.expires_at, 0),
			COALESCE(inviter.permission_level, 0) FROM docker_invitations i
			LEFT JOIN user_messages m ON m.id = i.id AND m.recipient = i.recipient
			LEFT JOIN user_profiles inviter_profile ON inviter_profile.username = i.inviter
			LEFT JOIN docker_members inviter ON inviter.repository = i.repository
				AND inviter.image_name = i.image_name AND inviter.user_id = inviter_profile.user_id
			WHERE i.repository = ? AND i.image_name = ? AND i.recipient = ?`,
			invitation.Repository, invitation.ImageName, invitation.Recipient).Scan(
			&existingID, &existingStatus, &existingExpiry, &existingInviterLevel,
		)
		if err == nil && existingStatus == core.MessageActionPending &&
			existingExpiry > invitation.CreatedAt && existingInviterLevel >= core.DockerPermissionTeam {
			return core.ErrDockerInvitationExists
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect pending Docker invitation: %w", err)
		}
		if err == nil {
			if err := cancelDockerInvitations(tx,
				`repository = ? AND image_name = ? AND recipient = ?`,
				[]any{invitation.Repository, invitation.ImageName, invitation.Recipient}, invitation.CreatedAt); err != nil {
				return err
			}
		}

		var dedupeKey any
		if message.DedupeKey != "" {
			dedupeKey = message.DedupeKey
		}
		if _, err := tx.Exec(`INSERT INTO user_messages (`+messageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			message.ID, message.Recipient, message.Sender, message.Kind, message.Severity,
			message.Title, message.Body, string(message.Payload), message.ActionKind, message.ActionStatus,
			message.CreatedAt, message.ReadAt, message.ActedAt, message.ExpiresAt, dedupeKey); err != nil {
			return fmt.Errorf("create Docker invitation message: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO docker_invitations
			(id, repository, image_name, inviter, recipient, permission_level, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, invitation.ID, invitation.Repository, invitation.ImageName,
			invitation.Inviter, invitation.Recipient, invitation.Level, invitation.CreatedAt); err != nil {
			return fmt.Errorf("create Docker invitation: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Docker invitation: %w", err)
	}
	return nil
}

func (db *DB) ForceAddDockerMembers(repository, imageName, actor string, usernames []string, level int) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(usernames) == 0 || len(usernames) > 20 {
		return errors.New("docker member addition batch is invalid")
	}
	if level < core.DockerPermissionRead || level > core.DockerPermissionOwner {
		return errors.New("docker permission level is invalid")
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	actor = sanitizeDockerUsername(actor)
	normalizedUsers := make([]string, 0, len(usernames))
	userIDs := make(map[string]string, len(usernames))
	seen := make(map[string]struct{}, len(usernames))
	for _, candidate := range usernames {
		username := sanitizeDockerUsername(candidate)
		if username == "" {
			return errors.New("docker member name is invalid")
		}
		if _, duplicate := seen[username]; duplicate {
			continue
		}
		seen[username] = struct{}{}
		userID, err := db.ensureUserProfile(username)
		if err != nil {
			return core.ErrUserProfileNotFound
		}
		userIDs[username] = userID
		normalizedUsers = append(normalizedUsers, username)
	}
	if len(normalizedUsers) == 0 || (level == core.DockerPermissionOwner && len(normalizedUsers) != 1) {
		return errors.New("docker L4 ownership can only be assigned to one member at a time")
	}
	now := time.Now().UnixMilli()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Docker force member addition: %w", err)
	}
	defer tx.Rollback()

	if err := lockDockerImageTeam(tx, repository, imageName); err != nil {
		return err
	}
	for _, username := range normalizedUsers {
		var existingLevel int
		err := tx.QueryRow(`SELECT permission_level FROM docker_members WHERE repository = ? AND image_name = ? AND user_id = ?`,
			repository, imageName, userIDs[username]).Scan(&existingLevel)
		if err == nil {
			return core.ErrDockerMemberExists
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check Docker member: %w", err)
		}
	}
	if level == core.DockerPermissionOwner {
		if _, err := tx.Exec(`UPDATE docker_members SET permission_level = ?
			WHERE repository = ? AND image_name = ? AND permission_level = ?`,
			core.DockerPermissionTeam, repository, imageName, core.DockerPermissionOwner); err != nil {
			return fmt.Errorf("demote previous Docker L4 owner: %w", err)
		}
		if _, err := tx.Exec(`UPDATE docker_images SET publisher = ? WHERE repository = ? AND image_name = ?`,
			normalizedUsers[0], repository, imageName); err != nil {
			return fmt.Errorf("update Docker image owner: %w", err)
		}
	}
	for _, username := range normalizedUsers {
		if _, err := tx.Exec(`INSERT INTO docker_members (repository, image_name, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
			repository, imageName, username, userIDs[username], level, now); err != nil {
			return fmt.Errorf("insert Docker member: %w", err)
		}

		if err := cancelDockerInvitations(tx, `repository = ? AND image_name = ? AND recipient = ?`, []any{repository, imageName, username}, now); err != nil {
			return err
		}

		if err := insertAcceptedMembershipMessage(tx, username, actor, "docker_image_invite",
			"Docker container image membership added",
			fmt.Sprintf("%s added you to collaborate on %s with L%d permission.", actor, imageName, level), map[string]any{
				"repository": repository,
				"image":      imageName,
				"inviter":    actor,
				"level":      level,
			}, now); err != nil {
			return fmt.Errorf("create Docker membership message: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Docker force member addition: %w", err)
	}
	return nil
}

func (db *DB) RespondDockerInvitation(id, recipient, repository string, accept bool, actedAt int64) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	id = SanitizeInputString(strings.TrimSpace(id), 64)
	recipient = sanitizeDockerUsername(recipient)
	repository, _ = sanitizeDockerKey(repository, "")
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Docker invitation response: %w", err)
	}
	defer tx.Rollback()

	invitation := &core.DockerInvitation{ID: id, Recipient: recipient}
	err = tx.QueryRow(`SELECT repository, image_name, inviter, permission_level, created_at
		FROM docker_invitations WHERE id = ? AND recipient = ? AND repository = ?`, id, recipient, repository).Scan(
		&invitation.Repository, &invitation.ImageName, &invitation.Inviter, &invitation.Level, &invitation.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return core.ErrDockerInvitationInvalid
	}
	if err != nil {
		return fmt.Errorf("load Docker invitation: %w", err)
	}
	if err := lockDockerImageTeam(tx, invitation.Repository, invitation.ImageName); err != nil {
		return err
	}

	if accept {
		inviterID, identityErr := userIDForUsernameTx(tx, invitation.Inviter)
		if identityErr != nil {
			return core.ErrDockerInvitationInvalid
		}
		recipientID, identityErr := userIDForUsernameTx(tx, recipient)
		if identityErr != nil {
			return core.ErrDockerInvitationInvalid
		}
		inviterLevel, inviterMember, permissionErr := dockerEffectivePermissionTx(
			tx, invitation.Repository, invitation.ImageName, inviterID)
		if permissionErr != nil {
			return permissionErr
		}
		if !inviterMember || inviterLevel < core.DockerPermissionTeam {
			return core.ErrDockerInvitationInvalid
		}
		if invitation.Level == core.DockerPermissionOwner && inviterLevel < core.DockerPermissionOwner {
			return core.ErrDockerInvitationInvalid
		}

		var memberLevel int
		err := tx.QueryRow(`SELECT permission_level FROM docker_members
			WHERE repository = ? AND image_name = ? AND user_id = ?`,
			invitation.Repository, invitation.ImageName, recipientID).Scan(&memberLevel)
		if err == nil {
			return core.ErrDockerMemberExists
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect Docker invitation membership: %w", err)
		}

		if invitation.Level == core.DockerPermissionOwner {
			if _, err := tx.Exec(`UPDATE docker_members SET permission_level = ?
				WHERE repository = ? AND image_name = ? AND permission_level = ? AND user_id != ?`,
				core.DockerPermissionTeam, invitation.Repository, invitation.ImageName, core.DockerPermissionOwner, recipientID); err != nil {
				return fmt.Errorf("demote previous Docker L4 owner: %w", err)
			}
			if _, err := tx.Exec(`UPDATE docker_images SET publisher = ? WHERE repository = ? AND image_name = ?`,
				recipient, invitation.Repository, invitation.ImageName); err != nil {
				return fmt.Errorf("update Docker image owner: %w", err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO docker_members
			(repository, image_name, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
			invitation.Repository, invitation.ImageName, recipient, recipientID, invitation.Level, actedAt); err != nil {
			return fmt.Errorf("accept Docker membership: %w", err)
		}
	}

	status := core.MessageActionRejected
	if accept {
		status = core.MessageActionAccepted
	}
	result, err := tx.Exec(`UPDATE user_messages SET action_status = ?, acted_at = ?,
		read_at = CASE WHEN read_at = 0 THEN ? ELSE read_at END
		WHERE id = ? AND recipient = ? AND action_kind = 'docker_image_invite'
		AND action_status = ? AND (expires_at = 0 OR expires_at > ?)`,
		status, actedAt, actedAt, id, recipient, core.MessageActionPending, actedAt)
	if err != nil {
		return fmt.Errorf("update Docker invitation message: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count Docker invitation response: %w", err)
	}
	if changed != 1 {
		return core.ErrDockerInvitationInvalid
	}
	if _, err := tx.Exec(`DELETE FROM docker_invitations WHERE id = ? AND recipient = ?`, id, recipient); err != nil {
		return fmt.Errorf("complete Docker invitation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Docker invitation response: %w", err)
	}
	return nil
}

func (db *DB) SetDockerMemberLevel(repository, imageName, actor, username string, level int) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if level < core.DockerPermissionRead || level > core.DockerPermissionOwner {
		return errors.New("docker permission level is invalid")
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	actor = sanitizeDockerUsername(actor)
	username = sanitizeDockerUsername(username)
	targetID, err := db.userIDForUsername(username)
	if err != nil {
		return core.ErrDockerImageNotFound
	}
	actorID := ""
	if actor != "" {
		actorID, err = db.userIDForUsername(actor)
		if err != nil {
			return core.ErrDockerPermissionDenied
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Docker member update: %w", err)
	}
	defer tx.Rollback()

	if err := lockDockerImageTeam(tx, repository, imageName); err != nil {
		return err
	}
	actorLevel := 0
	if actor != "" {
		if err := requireDockerMemberPermission(tx, repository, imageName, actorID, core.DockerPermissionTeam); err != nil {
			return err
		}
		var actorMember bool
		actorLevel, actorMember, err = dockerEffectivePermissionTx(tx, repository, imageName, actorID)
		if err != nil {
			return err
		}
		if !actorMember {
			return core.ErrDockerPermissionDenied
		}
	}
	var current int
	if err := tx.QueryRow(`SELECT permission_level FROM docker_members
		WHERE repository = ? AND image_name = ? AND user_id = ?`, repository, imageName, targetID).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return core.ErrDockerImageNotFound
	} else if err != nil {
		return fmt.Errorf("inspect Docker member: %w", err)
	}
	if current == core.DockerPermissionOwner && level < core.DockerPermissionOwner {
		if err := requireAnotherFullDockerMember(tx, repository, imageName, targetID); err != nil {
			return err
		}
	}
	if level == core.DockerPermissionOwner && current != core.DockerPermissionOwner {
		if actor != "" {
			if actorLevel != core.DockerPermissionOwner || actor == username {
				return core.ErrDockerPermissionDenied
			}
			if _, err := tx.Exec(`UPDATE docker_members SET permission_level = ?
				WHERE repository = ? AND image_name = ? AND user_id = ?`,
				current, repository, imageName, actorID); err != nil {
				return fmt.Errorf("exchange previous Docker L4 owner permission: %w", err)
			}
		} else if _, err := tx.Exec(`UPDATE docker_members SET permission_level = ?
			WHERE repository = ? AND image_name = ? AND permission_level = ? AND user_id != ?`,
			core.DockerPermissionTeam, repository, imageName, core.DockerPermissionOwner, targetID); err != nil {
			return fmt.Errorf("demote previous Docker L4 owner: %w", err)
		}
		if _, err := tx.Exec(`UPDATE docker_images SET publisher = ? WHERE repository = ? AND image_name = ?`,
			username, repository, imageName); err != nil {
			return fmt.Errorf("update Docker image owner: %w", err)
		}
	}
	if _, err := tx.Exec(`UPDATE docker_members SET permission_level = ?
		WHERE repository = ? AND image_name = ? AND user_id = ?`, level, repository, imageName, targetID); err != nil {
		return fmt.Errorf("update Docker member: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Docker member update: %w", err)
	}
	return nil
}

func (db *DB) RemoveDockerMember(repository, imageName, actor, username string) error {
	return db.RemoveDockerMembers(repository, imageName, actor, []string{username})
}

func (db *DB) RemoveDockerMembers(repository, imageName, actor string, usernames []string) error {
	repository, imageName = sanitizeDockerKey(repository, imageName)
	actor = sanitizeDockerUsername(actor)
	return db.removeTeamMembers(repository, imageName, actor, usernames, sanitizeDockerUsername, dockerTeamRemovalSpec)
}

func dockerEffectivePermissionTx(tx *Tx, repository, imageName, userID string) (int, bool, error) {
	if tx == nil || userID == "" {
		return 0, false, core.ErrDockerPermissionDenied
	}
	var explicitLevel, explicitMember, superRole, superMember int
	err := tx.QueryRow(`SELECT COALESCE(m.permission_level, 0),
		CASE WHEN m.user_id IS NULL THEN 0 ELSE 1 END, COALESCE(stm.role_level, 0),
		CASE WHEN stm.user_id IS NULL THEN 0 ELSE 1 END
		FROM docker_images i LEFT JOIN docker_members m ON m.repository = i.repository
		AND m.image_name = i.image_name AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = i.super_team_prefix AND stm.user_id = ?
		WHERE i.repository = ? AND i.image_name = ?`,
		userID, userID, repository, imageName).Scan(&explicitLevel, &explicitMember, &superRole, &superMember)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, core.ErrDockerImageNotFound
	}
	if err != nil {
		return 0, false, fmt.Errorf("inspect Docker member permission: %w", err)
	}
	level, member := effectiveBoundPermission(explicitLevel, explicitMember != 0, superRole, superMember != 0)
	return level, member, nil
}

func requireDockerMemberPermission(tx *Tx, repository, imageName, userID string, required int) error {
	level, member, err := dockerEffectivePermissionTx(tx, repository, imageName, userID)
	if err != nil {
		if errors.Is(err, core.ErrDockerImageNotFound) {
			return core.ErrDockerPermissionDenied
		}
		return err
	}
	if !member || level < required {
		return core.ErrDockerPermissionDenied
	}
	return nil
}

func lockDockerImageTeam(tx *Tx, repository, imageName string) error {
	if _, err := tx.Exec(`UPDATE docker_images SET updated_at = updated_at
		WHERE repository = ? AND image_name = ?`, repository, imageName); err != nil {
		return fmt.Errorf("lock Docker image team: %w", err)
	}
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM docker_images WHERE repository = ? AND image_name = ?`,
		repository, imageName).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return core.ErrDockerImageNotFound
	} else if err != nil {
		return fmt.Errorf("inspect Docker image team lock: %w", err)
	}
	return nil
}

func requireAnotherFullDockerMember(tx *Tx, repository, imageName, excludedUserID string) error {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM docker_members WHERE repository = ? AND image_name = ?
		AND permission_level = ? AND user_id <> ?`, repository, imageName, core.DockerPermissionOwner, excludedUserID).Scan(&count); err != nil {
		return fmt.Errorf("count Docker L4 members: %w", err)
	}
	if count == 0 {
		return core.ErrDockerLastFullMember
	}
	return nil
}

func cancelDockerInvitations(tx *Tx, whereClause string, args []any, actedAt int64) error {
	if tx == nil {
		return errors.New("transaction unavailable")
	}
	rows, err := tx.Query(`SELECT id, recipient FROM docker_invitations WHERE `+whereClause, args...)
	if err != nil {
		return fmt.Errorf("find Docker invitations to cancel: %w", err)
	}
	defer rows.Close()

	type item struct{ id, recipient string }
	var toCancel []item
	for rows.Next() {
		var id, recipient string
		if err := rows.Scan(&id, &recipient); err == nil {
			toCancel = append(toCancel, item{id, recipient})
		}
	}
	_ = rows.Close()

	for _, it := range toCancel {
		_, _ = tx.Exec(`UPDATE user_messages SET action_status = ?, acted_at = ?
			WHERE id = ? AND recipient = ? AND action_kind = 'docker_image_invite' AND action_status = ?`,
			core.MessageActionCancelled, actedAt, it.id, it.recipient, core.MessageActionPending)
		_, _ = tx.Exec(`DELETE FROM docker_invitations WHERE id = ? AND recipient = ?`, it.id, it.recipient)
	}
	return nil
}
