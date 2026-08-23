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

func sanitizeDockerKey(repository, imageName string) (string, string) {
	repository = strings.ToLower(SanitizeInputString(strings.TrimSpace(repository), 64))
	imageName = strings.ToLower(SanitizeInputString(strings.TrimSpace(imageName), 255))
	return repository, imageName
}

func (db *DB) GetDockerImage(repository, imageName string) (*core.DockerRepositoryImage, error) {
	if db == nil || db.SqlDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	repository, imageName = sanitizeDockerKey(repository, imageName)
	img := &core.DockerRepositoryImage{}
	err := db.QueryRow(
		`SELECT repository, image_name, description, publisher, created_at, updated_at FROM docker_images WHERE repository = ? AND image_name = ?`,
		repository, imageName,
	).Scan(&img.Repository, &img.ImageName, &img.Description, &img.Publisher, &img.CreatedAt, &img.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get Docker image: %w", err)
	}

	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM docker_tags WHERE repository = ? AND image_name = ?`, repository, imageName).Scan(&count)
	img.TagCount = count

	var latestTag string
	_ = db.QueryRow(`SELECT tag FROM docker_tags WHERE repository = ? AND image_name = ? ORDER BY updated_at DESC LIMIT 1`, repository, imageName).Scan(&latestTag)
	img.LatestTag = latestTag

	return img, nil
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

	query := `SELECT repository, image_name, description, publisher, created_at, updated_at FROM docker_images WHERE repository = ?`
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
		if err := rows.Scan(&img.Repository, &img.ImageName, &img.Description, &img.Publisher, &img.CreatedAt, &img.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan Docker image: %w", err)
		}
		images = append(images, img)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Docker images: %w", err)
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
		`SELECT repository, image_name, description, publisher, created_at, updated_at FROM docker_images WHERE `+where+
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
		if err := rows.Scan(&img.Repository, &img.ImageName, &img.Description, &img.Publisher, &img.CreatedAt, &img.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan Docker search result: %w", err)
		}
		images = append(images, img)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate Docker search results: %w", err)
	}

	// Populate tag counts and latest tag
	for _, img := range images {
		var count int
		_ = db.QueryRow(`SELECT COUNT(*) FROM docker_tags WHERE repository = ? AND image_name = ?`, img.Repository, img.ImageName).Scan(&count)
		img.TagCount = count

		var latestTag string
		_ = db.QueryRow(`SELECT tag FROM docker_tags WHERE repository = ? AND image_name = ? ORDER BY updated_at DESC LIMIT 1`, img.Repository, img.ImageName).Scan(&latestTag)
		img.LatestTag = latestTag
	}

	return images, total, nil
}

func (db *DB) GetDockerImageDetails(repository, imageName string) (*core.DockerImageDetails, error) {
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
			mRows.Close()
		}
	}

	details := &core.DockerImageDetails{
		Image: img,
		Tags:  tags,
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
	username = SanitizeInputString(strings.TrimSpace(username), 255)
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
			`INSERT INTO docker_images (repository, image_name, description, publisher, created_at, updated_at) VALUES (?, ?, '', ?, ?, ?)`,
			repository, imageName, username, now, now,
		); err != nil {
			return fmt.Errorf("insert Docker image: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("check Docker image: %w", err)
	} else {
		if username != "" {
			if _, err = tx.Exec(`UPDATE docker_images SET publisher = ?, updated_at = ? WHERE repository = ? AND image_name = ?`, username, now, repository, imageName); err != nil {
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
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete Docker image: %w", err)
	}
	defer tx.Rollback()

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
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete Docker repo: %w", err)
	}
	defer tx.Rollback()

	for _, table := range []string{"docker_tags", "docker_manifests", "docker_images", "docker_blobs"} {
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
