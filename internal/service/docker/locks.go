/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package docker

import (
	"errors"

	"renop/internal/config"
	"renop/internal/core"

	"github.com/gofiber/fiber/v3"
)

func dockerLockTarget(repository, image, digest string) core.ResourceLockTarget {
	return core.ResourceLockTarget{Format: "docker", Repository: repository, Name: image, Version: digest}
}

// ApplyImageLocks limits image inspection to metadata visible to the current account.
func ApplyImageLocks(state *core.AppState, user *config.User, details *core.DockerImageDetails) error {
	image := details.Image
	locks, err := state.GetDB().GetResourceLocks(dockerLockTarget(image.Repository, image.ImageName, ""), true)
	if err != nil {
		return err
	}
	details.Moderator = user != nil && user.CheckModeratePermission(image.Repository)
	inspect := details.Member || details.Moderator
	byDigest := make(map[string][]*core.ResourceLock)
	for _, lock := range locks {
		if lock.Version == "" {
			image.Locks = append(image.Locks, lock)
		} else {
			image.VersionLocked = true
			byDigest[lock.Version] = append(byDigest[lock.Version], lock)
		}
	}
	if core.ReadLocked(image.Locks) && !inspect {
		return core.ErrDockerImageNotFound
	}
	tags := details.Tags[:0]
	details.TotalSize = 0
	for _, tag := range details.Tags {
		tag.Locks = byDigest[tag.Digest]
		if core.ReadLocked(tag.Locks) && !inspect {
			continue
		}
		tags = append(tags, tag)
		details.TotalSize = max(details.TotalSize, tag.Size)
	}
	details.Tags = tags
	if details.Manifest != nil {
		details.Manifest.Locks = byDigest[details.Manifest.Digest]
		if core.ReadLocked(details.Manifest.Locks) && !inspect {
			details.Manifest = nil
		}
	}
	username := ""
	if user != nil {
		username = user.Username
	}
	return state.GetDB().FilterDockerImageVersions([]*core.DockerRepositoryImage{image}, username, details.Moderator)
}

// ManifestLocks authorizes metadata for an exact digest, including inherited index restrictions.
func ManifestLocks(state *core.AppState, user *config.User, repository, image, digest string) ([]*core.ResourceLock, error) {
	locks, err := state.GetDB().GetResourceLocks(dockerLockTarget(repository, image, digest), false)
	if err != nil || !core.ReadLocked(locks) {
		return locks, err
	}
	if user != nil && user.CheckModeratePermission(repository) {
		return locks, nil
	}
	if user == nil {
		return nil, core.ErrDockerManifestNotFound
	}
	_, _, _, member, _, err := state.GetDB().GetDockerImageAccess(repository, image, user.Username)
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, core.ErrDockerManifestNotFound
	}
	return locks, nil
}

func respondLockError(c fiber.Ctx, err error) error {
	if errors.Is(err, core.ErrResourceLocked) {
		c.Set("X-Renop-Error-Code", "resource_locked")
		return RespondError(c, fiber.StatusLocked, ErrCodeDenied, "Docker resource is locked", nil)
	}
	return RespondError(c, fiber.StatusServiceUnavailable, ErrCodeUnsupported, "failed to inspect Docker resource", nil)
}
