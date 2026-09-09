/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/goccy/go-json"
	"renop/internal/core"
)

func dockerLockTarget(repository, image, digest string) core.ResourceLockTarget {
	return core.ResourceLockTarget{Format: "docker", Repository: repository, Name: image, Version: digest}
}

// Capture immutable content references with the lock, including children not yet cached locally.
func setDockerLockVersionsTx(tx *Tx, target core.ResourceLockTarget, source string) error {
	if target.Format != "docker" {
		return nil
	}
	if err := lockDockerImageRow(tx, target.Repository, target.Name); err != nil {
		return err
	}
	id := resourceLockID(target)
	if _, err := tx.Exec(`DELETE FROM resource_lock_versions WHERE lock_id = ? AND source = ?`, id, source); err != nil {
		return err
	}
	if target.Version == "" {
		return nil
	}
	const maxReferences = 8192
	seen := map[string]bool{target.Version: true}
	queued := map[string]bool{target.Version: true}
	pending := []string{target.Version}
	remainingBytes := 64 << 20
	for len(pending) > 0 {
		count := min(len(pending), 8)
		batch := pending[:count]
		pending = pending[count:]
		args := []any{target.Repository, target.Name}
		for _, digest := range batch {
			args = append(args, digest)
		}
		rows, err := tx.Query(`SELECT raw_json FROM docker_manifests WHERE repository = ? AND image_name = ?
			AND digest IN (`+strings.TrimSuffix(strings.Repeat("?,", count), ",")+`)`, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				_ = rows.Close()
				return err
			}
			remainingBytes -= len(raw)
			if len(raw) > 4<<20 || remainingBytes < 0 {
				_ = rows.Close()
				return core.ErrResourceLockInvalid
			}
			type descriptor struct {
				Digest string `json:"digest"`
			}
			var manifest struct {
				Config    descriptor   `json:"config"`
				Layers    []descriptor `json:"layers"`
				Manifests []descriptor `json:"manifests"`
			}
			if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
				_ = rows.Close()
				return core.ErrDockerManifestInvalid
			}
			for _, ref := range append(append(manifest.Layers, manifest.Manifests...), manifest.Config) {
				if ref.Digest == "" || seen[ref.Digest] {
					continue
				}
				if len(ref.Digest) != 71 || !strings.HasPrefix(ref.Digest, "sha256:") || strings.ToLower(ref.Digest) != ref.Digest {
					_ = rows.Close()
					return core.ErrDockerManifestInvalid
				}
				if _, err := hex.DecodeString(ref.Digest[7:]); err != nil || len(seen) >= maxReferences {
					_ = rows.Close()
					return core.ErrResourceLockInvalid
				}
				seen[ref.Digest] = true
			}
			for _, child := range manifest.Manifests {
				if child.Digest != "" && !queued[child.Digest] {
					queued[child.Digest] = true
					pending = append(pending, child.Digest)
				}
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	for digest := range seen {
		if digest == target.Version {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO resource_lock_versions (lock_id, source, version) VALUES (?, ?, ?)`, id, source, digest); err != nil {
			return err
		}
	}
	return nil
}

func ensureDockerTagMutableQuery(queryRow func(string, ...any) row, repository, image, tag string) error {
	var digest string
	err := queryRow(`SELECT digest FROM docker_tags WHERE repository = ? AND image_name = ? AND tag = ?`, repository, image, tag).Scan(&digest)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return ensureResourceMutableQuery(queryRow, dockerLockTarget(repository, image, digest), false)
}

// EnsureDockerManifestMutable checks both an immutable digest and the tag that would be replaced.
func (db *DB) EnsureDockerManifestMutable(repository, image, digest, tag string) error {
	if err := db.EnsureResourceMutable(dockerLockTarget(repository, image, digest), false); err != nil {
		return err
	}
	if tag != "" {
		return ensureDockerTagMutableQuery(db.QueryRow, repository, image, tag)
	}
	return nil
}

// EnsureDockerBlobMutable protects shared bytes referenced by any locked image or version.
func (db *DB) EnsureDockerBlobMutable(repository, digest string) error {
	return ensureDockerBlobMutableQuery(db.QueryRow, repository, digest)
}

func ensureDockerBlobMutableQuery(queryRow func(string, ...any) row, repository, digest string) error {
	var exists int
	err := queryRow(`SELECT 1 FROM `+resourceLocksQuery("docker")+` l
		WHERE l.format = 'docker' AND l.repository = ? AND (l.version = ? OR EXISTS (
		SELECT 1 FROM docker_image_blobs b WHERE b.repository = l.repository AND b.image_name = l.resource_name
		AND b.blob_digest = ? AND (l.version = '' OR l.version = b.manifest_digest))) LIMIT 1`, repository, digest, digest).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return core.ErrResourceLocked
}

// FilterDockerImageVersions recalculates visible tag counts and latest tags in one bounded batch.
func (db *DB) FilterDockerImageVersions(images []*core.DockerRepositoryImage, username string, moderator bool) error {
	if len(images) == 0 || moderator {
		return nil
	}
	if len(images) > 128 {
		return core.ErrResourceLockInvalid
	}
	userID := ""
	if username != "" && !strings.EqualFold(username, "guest") {
		var err error
		userID, err = db.userIDForUsername(username)
		if err != nil && !errors.Is(err, core.ErrUserProfileNotFound) {
			return err
		}
	}
	repository := images[0].Repository
	byName := make(map[string]*core.DockerRepositoryImage, len(images))
	args := []any{repository}
	for _, image := range images {
		if image.Repository != repository {
			return core.ErrResourceLockInvalid
		}
		byName[image.ImageName] = image
		args = append(args, image.ImageName)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(images)), ",")
	rows, err := db.Query(`SELECT DISTINCT resource_name FROM resource_locks
		WHERE format = 'docker' AND repository = ? AND mode = 'read' AND version <> ''
		AND resource_name IN (`+placeholders+`)`, args...)
	if err != nil {
		return err
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil || len(names) == 0 {
		return err
	}
	args = []any{userID, userID, repository}
	for _, name := range names {
		args = append(args, name)
		byName[name].TagCount, byName[name].LatestTag = 0, ""
	}
	placeholders = strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	rows, err = db.Query(`SELECT image_name, tag, tag_count FROM (
		SELECT t.image_name AS image_name, t.tag AS tag, COUNT(*) OVER (PARTITION BY t.image_name) AS tag_count,
		ROW_NUMBER() OVER (PARTITION BY t.image_name ORDER BY t.updated_at DESC, t.tag ASC) AS tag_rank
		FROM docker_tags t JOIN docker_images p ON p.repository = t.repository AND p.image_name = t.image_name
		LEFT JOIN docker_members m ON m.repository = p.repository AND m.image_name = p.image_name AND m.user_id = ?
		LEFT JOIN super_team_members stm ON stm.team_prefix = p.super_team_prefix AND stm.user_id = ?
		WHERE t.repository = ? AND t.image_name IN (`+placeholders+`) AND (m.user_id IS NOT NULL OR stm.user_id IS NOT NULL
		OR NOT EXISTS (SELECT 1 FROM `+resourceLocksQuery("docker")+` l WHERE l.format = 'docker' AND l.mode = 'read'
		AND l.repository = t.repository AND l.resource_name = t.image_name AND l.version = t.digest))) ranked
		WHERE tag_rank = 1`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, tag string
		var count int
		if err := rows.Scan(&name, &tag, &count); err != nil {
			return err
		}
		byName[name].TagCount, byName[name].LatestTag = count, tag
	}
	return rows.Err()
}
