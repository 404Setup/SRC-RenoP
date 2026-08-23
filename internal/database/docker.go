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

	"github.com/goccy/go-json"
	"github.com/google/uuid"

	"renop/internal/core"
)

func sanitizeDockerKey(repository, imageName string) (string, string) {
	repository = strings.ToLower(SanitizeInputString(strings.TrimSpace(repository), 64))
	imageName = strings.ToLower(SanitizeInputString(strings.TrimSpace(imageName), 255))
	return repository, imageName
}

func sanitizeDockerUsername(username string) string {
	return strings.ToLower(SanitizeInputString(strings.TrimSpace(username), 255))
}

func (db *DB) GetDockerImage(repository, imageName string) (*core.DockerRepositoryImage, error) {
	if db == nil || db.SqlDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	img := &core.DockerRepositoryImage{}
	err := db.QueryRow(
		`SELECT repository, image_name, description, publisher, pull_count, created_at, updated_at FROM docker_images WHERE repository = ? AND image_name = ?`,
		repository, imageName,
	).Scan(&img.Repository, &img.ImageName, &img.Description, &img.Publisher, &img.PullCount, &img.CreatedAt, &img.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get Docker image: %w", err)
	}

	if img.Publisher == "" {
		var tagPub string
		_ = db.QueryRow(`SELECT publisher FROM docker_tags WHERE repository = ? AND image_name = ? AND publisher != '' ORDER BY updated_at DESC LIMIT 1`, repository, imageName).Scan(&tagPub)
		if tagPub != "" {
			img.Publisher = tagPub
		} else {
			var manPub string
			_ = db.QueryRow(`SELECT publisher FROM docker_manifests WHERE repository = ? AND image_name = ? AND publisher != '' ORDER BY created_at DESC LIMIT 1`, repository, imageName).Scan(&manPub)
			img.Publisher = manPub
		}
	}

	var ownerUser string
	_ = db.QueryRow(`SELECT username FROM docker_members WHERE repository = ? AND image_name = ? AND permission_level = ? LIMIT 1`, repository, imageName, core.DockerPermissionOwner).Scan(&ownerUser)
	if ownerUser != "" {
		img.Publisher = ownerUser
	}

	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM docker_tags WHERE repository = ? AND image_name = ?`, repository, imageName).Scan(&count)
	img.TagCount = count

	var latestTag string
	_ = db.QueryRow(`SELECT tag FROM docker_tags WHERE repository = ? AND image_name = ? ORDER BY updated_at DESC LIMIT 1`, repository, imageName).Scan(&latestTag)
	img.LatestTag = latestTag

	return img, nil
}

func (db *DB) IncrementDockerPullCount(repository, imageName string) error {
	if db == nil || db.SqlDB == nil {
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
	if db == nil || db.SqlDB == nil || delta <= 0 {
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
	if db == nil || db.SqlDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	description = strings.TrimSpace(description)
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
	if db == nil || db.SqlDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeDockerKey(repository, "")
	last = strings.ToLower(SanitizeInputString(strings.TrimSpace(last), 255))
	if limit < 1 || limit > 100 {
		limit = 50
	}

	query := `SELECT repository, image_name, description, publisher, pull_count, created_at, updated_at FROM docker_images WHERE repository = ?`
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
		if err := rows.Scan(&img.Repository, &img.ImageName, &img.Description, &img.Publisher, &img.PullCount, &img.CreatedAt, &img.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan Docker image: %w", err)
		}
		images = append(images, img)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Docker images: %w", err)
	}

	for _, img := range images {
		if img.Publisher == "" {
			var tagPub string
			_ = db.QueryRow(`SELECT publisher FROM docker_tags WHERE repository = ? AND image_name = ? AND publisher != '' ORDER BY updated_at DESC LIMIT 1`, img.Repository, img.ImageName).Scan(&tagPub)
			if tagPub != "" {
				img.Publisher = tagPub
			} else {
				var manPub string
				_ = db.QueryRow(`SELECT publisher FROM docker_manifests WHERE repository = ? AND image_name = ? AND publisher != '' ORDER BY created_at DESC LIMIT 1`, img.Repository, img.ImageName).Scan(&manPub)
				img.Publisher = manPub
			}
		}

		var ownerUser string
		_ = db.QueryRow(`SELECT username FROM docker_members WHERE repository = ? AND image_name = ? AND permission_level = ? LIMIT 1`, img.Repository, img.ImageName, core.DockerPermissionOwner).Scan(&ownerUser)
		if ownerUser != "" {
			img.Publisher = ownerUser
		}

		var count int
		_ = db.QueryRow(`SELECT COUNT(*) FROM docker_tags WHERE repository = ? AND image_name = ?`, img.Repository, img.ImageName).Scan(&count)
		img.TagCount = count

		var latestTag string
		_ = db.QueryRow(`SELECT tag FROM docker_tags WHERE repository = ? AND image_name = ? ORDER BY updated_at DESC LIMIT 1`, img.Repository, img.ImageName).Scan(&latestTag)
		img.LatestTag = latestTag
	}

	return images, nil
}

func (db *DB) SearchDockerImages(repository, query string, limit, offset int) ([]*core.DockerRepositoryImage, int, error) {
	if db == nil || db.SqlDB == nil {
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
		`SELECT repository, image_name, description, publisher, pull_count, created_at, updated_at FROM docker_images WHERE `+where+
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
		if err := rows.Scan(&img.Repository, &img.ImageName, &img.Description, &img.Publisher, &img.PullCount, &img.CreatedAt, &img.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan Docker search result: %w", err)
		}
		images = append(images, img)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate Docker search results: %w", err)
	}

	for _, img := range images {
		if img.Publisher == "" {
			var tagPub string
			_ = db.QueryRow(`SELECT publisher FROM docker_tags WHERE repository = ? AND image_name = ? AND publisher != '' ORDER BY updated_at DESC LIMIT 1`, img.Repository, img.ImageName).Scan(&tagPub)
			if tagPub != "" {
				img.Publisher = tagPub
			}
		}

		var ownerUser string
		_ = db.QueryRow(`SELECT username FROM docker_members WHERE repository = ? AND image_name = ? AND permission_level = ? LIMIT 1`, img.Repository, img.ImageName, core.DockerPermissionOwner).Scan(&ownerUser)
		if ownerUser != "" {
			img.Publisher = ownerUser
		}

		var count int
		_ = db.QueryRow(`SELECT COUNT(*) FROM docker_tags WHERE repository = ? AND image_name = ?`, img.Repository, img.ImageName).Scan(&count)
		img.TagCount = count

		var latestTag string
		_ = db.QueryRow(`SELECT tag FROM docker_tags WHERE repository = ? AND image_name = ? ORDER BY updated_at DESC LIMIT 1`, img.Repository, img.ImageName).Scan(&latestTag)
		img.LatestTag = latestTag
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
	if db == nil || db.SqlDB == nil {
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
	if db == nil || db.SqlDB == nil {
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
	if db == nil || db.SqlDB == nil {
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
	if db == nil || db.SqlDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if manifest == nil {
		return errors.New("missing Docker manifest")
	}
	repository, imageName := sanitizeDockerKey(manifest.Repository, manifest.ImageName)
	digest := strings.ToLower(SanitizeInputString(strings.TrimSpace(manifest.Digest), 128))
	mediaType := SanitizeInputString(strings.TrimSpace(manifest.MediaType), 128)
	configDigest := strings.ToLower(SanitizeInputString(strings.TrimSpace(manifest.ConfigDigest), 128))
	tag = SanitizeInputString(strings.TrimSpace(tag), 128)
	username = sanitizeDockerUsername(username)
	now := time.Now().UnixMilli()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Docker manifest transaction: %w", err)
	}
	defer tx.Rollback()

	// Ensure docker_images record exists
	var existingImage int
	err = tx.QueryRow(`SELECT 1 FROM docker_images WHERE repository = ? AND image_name = ?`, repository, imageName).Scan(&existingImage)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err = tx.Exec(
			`INSERT INTO docker_images (repository, image_name, description, publisher, pull_count, created_at, updated_at) VALUES (?, ?, '', ?, 0, ?, ?)`,
			repository, imageName, username, now, now,
		); err != nil {
			return fmt.Errorf("insert Docker image: %w", err)
		}
		if username != "" && username != "guest" {
			if _, err = tx.Exec(
				`INSERT INTO docker_members (repository, image_name, username, permission_level, added_at) VALUES (?, ?, ?, ?, ?)`,
				repository, imageName, username, core.DockerPermissionFull, now,
			); err != nil {
				return fmt.Errorf("insert Docker owner: %w", err)
			}
		}
	} else if err != nil {
		return fmt.Errorf("check Docker image: %w", err)
	} else {
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

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Docker manifest: %w", err)
	}
	return nil
}

func (db *DB) DeleteDockerTag(repository, imageName, tag string) error {
	if db == nil || db.SqlDB == nil {
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
	if db == nil || db.SqlDB == nil {
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
	if db == nil || db.SqlDB == nil {
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
	if db == nil || db.SqlDB == nil {
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

	for _, table := range []string{"docker_members", "docker_tags", "docker_manifests", "docker_images", "docker_blobs"} {
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
	if db == nil || db.SqlDB == nil {
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

func (db *DB) HasDockerBlob(repository, digest string) (bool, int64, error) {
	if db == nil || db.SqlDB == nil {
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

func (db *DB) DeleteDockerBlob(repository, digest string) error {
	if db == nil || db.SqlDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeDockerKey(repository, "")
	digest = strings.ToLower(SanitizeInputString(strings.TrimSpace(digest), 128))
	_, err := db.Exec(`DELETE FROM docker_blobs WHERE repository = ? AND digest = ?`, repository, digest)
	if err != nil {
		return fmt.Errorf("delete Docker blob: %w", err)
	}
	return nil
}

func (db *DB) GetDockerRepositoryStats(repository string) (totalImages int64, totalTags int64, totalSize int64, err error) {
	if db == nil || db.SqlDB == nil {
		return 0, 0, 0, core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeDockerKey(repository, "")
	_ = db.QueryRow(`SELECT COUNT(*) FROM docker_images WHERE repository = ?`, repository).Scan(&totalImages)
	_ = db.QueryRow(`SELECT COUNT(*) FROM docker_tags WHERE repository = ?`, repository).Scan(&totalTags)
	_ = db.QueryRow(`SELECT COALESCE(SUM(size), 0) FROM docker_blobs WHERE repository = ?`, repository).Scan(&totalSize)
	return totalImages, totalTags, totalSize, nil
}

func (db *DB) HasDockerMembership(repository, username string) (bool, error) {
	if db == nil || db.SqlDB == nil {
		return false, core.ErrDatabaseUnavailable
	}
	repository, _ = sanitizeDockerKey(repository, "")
	username = sanitizeDockerUsername(username)
	if username == "" {
		return false, nil
	}
	var exists int
	err := db.QueryRow(`SELECT 1 FROM docker_members WHERE repository = ? AND username = ? LIMIT 1`, repository, username).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check Docker membership: %w", err)
	}
	return true, nil
}

func (db *DB) GetDockerMemberLevel(repository, imageName, username string) (int, error) {
	if db == nil || db.SqlDB == nil {
		return 0, core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	username = sanitizeDockerUsername(username)
	if username == "" {
		return core.DockerPermissionRead, nil
	}
	var level int
	err := db.QueryRow(
		`SELECT permission_level FROM docker_members WHERE repository = ? AND image_name = ? AND username = ?`,
		repository, imageName, username,
	).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) {
		var pub string
		_ = db.QueryRow(`SELECT publisher FROM docker_images WHERE repository = ? AND image_name = ?`, repository, imageName).Scan(&pub)
		if pub != "" && pub == username {
			return core.DockerPermissionFull, nil
		}
		return core.DockerPermissionRead, nil
	}
	if err != nil {
		return core.DockerPermissionRead, fmt.Errorf("inspect Docker member level: %w", err)
	}
	return level, nil
}

func (db *DB) ListDockerMembers(repository, imageName string) ([]*core.DockerMember, error) {
	if db == nil || db.SqlDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	rows, err := db.Query(
		`SELECT username, permission_level, added_at FROM docker_members
		 WHERE repository = ? AND image_name = ? ORDER BY permission_level DESC, username ASC`,
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
		if err := rows.Scan(&m.Username, &m.Level, &m.AddedAt); err != nil {
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
		_, _ = db.Exec(`INSERT INTO docker_members (repository, image_name, username, permission_level, added_at) VALUES (?, ?, ?, ?, ?)`,
			repository, imageName, pub, core.DockerPermissionFull, createdAt)
		members = append([]*core.DockerMember{{
			Username: pub,
			Level:    core.DockerPermissionFull,
			AddedAt:  createdAt,
		}}, members...)
	}

	return members, nil
}

func (db *DB) CreateDockerInvitations(invitations []*core.DockerInvitation, messages []*core.UserMessage) error {
	if db == nil || db.SqlDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(invitations) == 0 || len(invitations) != len(messages) || len(invitations) > 20 {
		return errors.New("Docker invitation is missing")
	}
	for i, invitation := range invitations {
		message := messages[i]
		if invitation == nil || message == nil {
			return errors.New("Docker invitation is missing")
		}
		invitation.Repository, invitation.ImageName = sanitizeDockerKey(invitation.Repository, invitation.ImageName)
		invitation.Inviter = sanitizeDockerUsername(invitation.Inviter)
		invitation.Recipient = sanitizeDockerUsername(invitation.Recipient)
		if invitation.Level < core.DockerPermissionPublish || invitation.Level > core.DockerPermissionOwner {
			return errors.New("Docker invitation permission level is invalid")
		}
		if invitation.ID == "" || invitation.ID != message.ID || invitation.Recipient != strings.ToLower(strings.TrimSpace(message.Recipient)) {
			return errors.New("Docker invitation message does not match its workflow record")
		}
		if err := normalizeMessage(message); err != nil {
			return err
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Docker invitation: %w", err)
	}
	defer tx.Rollback()

	first := invitations[0]
	var inviterLevel int
	err = tx.QueryRow(`SELECT permission_level FROM docker_members
		WHERE repository = ? AND image_name = ? AND username = ?`,
		first.Repository, first.ImageName, first.Inviter).Scan(&inviterLevel)
	if errors.Is(err, sql.ErrNoRows) {
		var pub string
		var createdAt int64
		_ = tx.QueryRow(`SELECT publisher, created_at FROM docker_images WHERE repository = ? AND image_name = ?`, first.Repository, first.ImageName).Scan(&pub, &createdAt)
		if pub != "" && pub == first.Inviter {
			inviterLevel = core.DockerPermissionOwner
			if createdAt == 0 {
				createdAt = first.CreatedAt
			}
			_, _ = tx.Exec(`INSERT INTO docker_members (repository, image_name, username, permission_level, added_at) VALUES (?, ?, ?, ?, ?)`,
				first.Repository, first.ImageName, first.Inviter, core.DockerPermissionOwner, createdAt)
		} else {
			return core.ErrDockerPermissionDenied
		}
	} else if err != nil {
		return fmt.Errorf("inspect Docker inviter permission: %w", err)
	} else if inviterLevel < core.DockerPermissionTeam {
		return core.ErrDockerPermissionDenied
	}

	for i, invitation := range invitations {
		message := messages[i]
		if invitation.Repository != first.Repository || invitation.ImageName != first.ImageName || invitation.Inviter != first.Inviter {
			return errors.New("Docker invitation batch targets multiple images")
		}
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM docker_members WHERE repository = ? AND image_name = ? AND username = ?`,
			invitation.Repository, invitation.ImageName, invitation.Recipient).Scan(&exists); err == nil {
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
			LEFT JOIN docker_members inviter ON inviter.repository = i.repository
				AND inviter.image_name = i.image_name AND inviter.username = i.inviter
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
	if db == nil || db.SqlDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(usernames) == 0 || len(usernames) > 20 {
		return errors.New("Docker member addition batch is invalid")
	}
	if level < core.DockerPermissionPublish || level > core.DockerPermissionOwner {
		return errors.New("Docker permission level is invalid")
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	actor = sanitizeDockerUsername(actor)
	now := time.Now().UnixMilli()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Docker force member addition: %w", err)
	}
	defer tx.Rollback()

	for _, candidate := range usernames {
		username := sanitizeDockerUsername(candidate)
		if username == "" {
			continue
		}
		if level == core.DockerPermissionOwner {
			if _, err := tx.Exec(`UPDATE docker_members SET permission_level = ?
				WHERE repository = ? AND image_name = ? AND permission_level = ? AND username != ?`,
				core.DockerPermissionTeam, repository, imageName, core.DockerPermissionOwner, username); err != nil {
				return fmt.Errorf("demote previous Docker L4 owner: %w", err)
			}
			_, _ = tx.Exec(`UPDATE docker_images SET publisher = ? WHERE repository = ? AND image_name = ?`, username, repository, imageName)
		}

		var existingLevel int
		err := tx.QueryRow(`SELECT permission_level FROM docker_members WHERE repository = ? AND image_name = ? AND username = ?`,
			repository, imageName, username).Scan(&existingLevel)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.Exec(`INSERT INTO docker_members (repository, image_name, username, permission_level, added_at) VALUES (?, ?, ?, ?, ?)`,
				repository, imageName, username, level, now); err != nil {
				return fmt.Errorf("insert Docker member: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("check Docker member: %w", err)
		} else {
			if _, err := tx.Exec(`UPDATE docker_members SET permission_level = ? WHERE repository = ? AND image_name = ? AND username = ?`,
				level, repository, imageName, username); err != nil {
				return fmt.Errorf("update Docker member: %w", err)
			}
		}

		_ = cancelDockerInvitations(tx, `repository = ? AND image_name = ? AND recipient = ?`, []any{repository, imageName, username}, now)

		msgID := uuid.NewString()
		payloadBytes, _ := json.Marshal(map[string]any{
			"repository": repository,
			"image":      imageName,
			"inviter":    actor,
			"level":      level,
		})
		_, _ = tx.Exec(`INSERT INTO user_messages (`+messageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			msgID, username, actor, "docker_image_invite", "info",
			"Docker container image membership added",
			fmt.Sprintf("%s added you to collaborate on %s with L%d permission.", actor, imageName, level),
			string(payloadBytes), "docker_image_invite", core.MessageActionAccepted,
			now, 0, now, now+7*24*3600*1000, nil)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Docker force member addition: %w", err)
	}
	return nil
}

func (db *DB) RespondDockerInvitation(id, recipient, repository string, accept bool, actedAt int64) error {
	if db == nil || db.SqlDB == nil {
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

	if accept {
		var inviterLevel int
		if err := tx.QueryRow(`SELECT permission_level FROM docker_members
			WHERE repository = ? AND image_name = ? AND username = ?`,
			invitation.Repository, invitation.ImageName, invitation.Inviter).Scan(&inviterLevel); errors.Is(err, sql.ErrNoRows) || inviterLevel < core.DockerPermissionTeam {
			return core.ErrDockerInvitationInvalid
		} else if err != nil {
			return fmt.Errorf("validate Docker inviter: %w", err)
		}

		if invitation.Level == core.DockerPermissionOwner {
			if _, err := tx.Exec(`UPDATE docker_members SET permission_level = ?
				WHERE repository = ? AND image_name = ? AND permission_level = ? AND username != ?`,
				core.DockerPermissionTeam, invitation.Repository, invitation.ImageName, core.DockerPermissionOwner, recipient); err != nil {
				return fmt.Errorf("demote previous Docker L4 owner: %w", err)
			}
			_, _ = tx.Exec(`UPDATE docker_images SET publisher = ? WHERE repository = ? AND image_name = ?`, recipient, invitation.Repository, invitation.ImageName)
		}

		var memberLevel int
		err := tx.QueryRow(`SELECT permission_level FROM docker_members
			WHERE repository = ? AND image_name = ? AND username = ?`,
			invitation.Repository, invitation.ImageName, recipient).Scan(&memberLevel)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.Exec(`INSERT INTO docker_members
				(repository, image_name, username, permission_level, added_at) VALUES (?, ?, ?, ?, ?)`,
				invitation.Repository, invitation.ImageName, recipient, invitation.Level, actedAt); err != nil {
				return fmt.Errorf("accept Docker membership: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("inspect Docker invitation membership: %w", err)
		} else {
			if _, err := tx.Exec(`UPDATE docker_members SET permission_level = ? WHERE repository = ? AND image_name = ? AND username = ?`,
				invitation.Level, invitation.Repository, invitation.ImageName, recipient); err != nil {
				return fmt.Errorf("update Docker membership: %w", err)
			}
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
	if db == nil || db.SqlDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if level < core.DockerPermissionPublish || level > core.DockerPermissionOwner {
		return errors.New("Docker permission level is invalid")
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	actor = sanitizeDockerUsername(actor)
	username = sanitizeDockerUsername(username)
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Docker member update: %w", err)
	}
	defer tx.Rollback()

	if actor != "" {
		if err := requireDockerMemberPermission(tx, repository, imageName, actor, core.DockerPermissionTeam); err != nil {
			return err
		}
	}
	var current int
	if err := tx.QueryRow(`SELECT permission_level FROM docker_members
		WHERE repository = ? AND image_name = ? AND username = ?`, repository, imageName, username).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return core.ErrDockerImageNotFound
	} else if err != nil {
		return fmt.Errorf("inspect Docker member: %w", err)
	}
	if current == core.DockerPermissionOwner && level < core.DockerPermissionOwner {
		if err := requireAnotherFullDockerMember(tx, repository, imageName, username); err != nil {
			return err
		}
	}
	if level == core.DockerPermissionOwner {
		if _, err := tx.Exec(`UPDATE docker_members SET permission_level = ?
			WHERE repository = ? AND image_name = ? AND permission_level = ? AND username != ?`,
			core.DockerPermissionTeam, repository, imageName, core.DockerPermissionOwner, username); err != nil {
			return fmt.Errorf("demote previous Docker L4 owner: %w", err)
		}
		_, _ = tx.Exec(`UPDATE docker_images SET publisher = ? WHERE repository = ? AND image_name = ?`, username, repository, imageName)
	}
	if _, err := tx.Exec(`UPDATE docker_members SET permission_level = ?
		WHERE repository = ? AND image_name = ? AND username = ?`, level, repository, imageName, username); err != nil {
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
	if db == nil || db.SqlDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(usernames) == 0 || len(usernames) > 20 {
		return errors.New("Docker member removal batch is invalid")
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	actor = sanitizeDockerUsername(actor)
	unique := make([]string, 0, len(usernames))
	seen := make(map[string]struct{}, len(usernames))
	for _, candidate := range usernames {
		username := sanitizeDockerUsername(candidate)
		if username == "" {
			return errors.New("Docker member name is invalid")
		}
		if _, exists := seen[username]; exists {
			continue
		}
		seen[username] = struct{}{}
		unique = append(unique, username)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin Docker member removal: %w", err)
	}
	defer tx.Rollback()

	if actor != "" {
		if err := requireDockerMemberPermission(tx, repository, imageName, actor, core.DockerPermissionTeam); err != nil {
			return err
		}
	}
	fullRemoved := 0
	for _, username := range unique {
		var current int
		if err := tx.QueryRow(`SELECT permission_level FROM docker_members
			WHERE repository = ? AND image_name = ? AND username = ?`, repository, imageName, username).Scan(&current); errors.Is(err, sql.ErrNoRows) {
			return core.ErrDockerImageNotFound
		} else if err != nil {
			return fmt.Errorf("inspect Docker member removal: %w", err)
		}
		if current == core.DockerPermissionOwner {
			fullRemoved++
		}
	}
	if fullRemoved > 0 {
		var fullCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM docker_members WHERE repository = ? AND image_name = ? AND permission_level = ?`,
			repository, imageName, core.DockerPermissionOwner).Scan(&fullCount); err != nil {
			return fmt.Errorf("count Docker L4 members: %w", err)
		}
		if fullRemoved >= fullCount {
			return core.ErrDockerLastFullMember
		}
	}
	for _, username := range unique {
		if _, err := tx.Exec(`DELETE FROM docker_members WHERE repository = ? AND image_name = ? AND username = ?`,
			repository, imageName, username); err != nil {
			return fmt.Errorf("remove Docker member: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Docker member removal: %w", err)
	}
	return nil
}

func requireDockerMemberPermission(tx *Tx, repository, imageName, username string, required int) error {
	if tx == nil || username == "" {
		return core.ErrDockerPermissionDenied
	}
	var level int
	err := tx.QueryRow(`SELECT permission_level FROM docker_members
		WHERE repository = ? AND image_name = ? AND username = ?`,
		repository, imageName, username).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) {
		var pub string
		_ = tx.QueryRow(`SELECT publisher FROM docker_images WHERE repository = ? AND image_name = ?`, repository, imageName).Scan(&pub)
		if pub != "" && pub == username {
			return nil
		}
		return core.ErrDockerPermissionDenied
	}
	if err != nil {
		return fmt.Errorf("inspect Docker member permission: %w", err)
	}
	if level < required {
		return core.ErrDockerPermissionDenied
	}
	return nil
}

func requireAnotherFullDockerMember(tx *Tx, repository, imageName, excludedUsername string) error {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM docker_members WHERE repository = ? AND image_name = ?
		AND permission_level = ? AND username <> ?`, repository, imageName, core.DockerPermissionOwner, excludedUsername).Scan(&count); err != nil {
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
