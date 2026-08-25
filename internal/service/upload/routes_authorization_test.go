/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package upload

import (
	"errors"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/storage"
)

func TestAuthorizeStorageUploadUsesFormatPolicy(t *testing.T) {
	state := core.NewAppState()
	user := &config.User{Username: "alice", Roles: []string{"base"}}
	mavenRepo := &config.Repository{Name: "releases", Format: config.RepositoryFormatMaven}
	previous := storage.MavenMutationAuthorizer
	t.Cleanup(func() { storage.MavenMutationAuthorizer = previous })
	var receivedPath string
	storage.MavenMutationAuthorizer = func(_ *core.AppState, _ *config.User, _ *config.Repository, path string, required int) error {
		receivedPath = path
		assert.Equal(t, core.MavenPermissionPublish, required)
		return core.ErrMavenDomainUnverified
	}
	err := authorizeStorageUpload(state, user, mavenRepo, "com/example/demo/1.0/demo-1.0.jar")
	require.ErrorIs(t, err, core.ErrMavenDomainUnverified)
	assert.Equal(t, "com/example/demo/1.0/demo-1.0.jar", receivedPath)

	fileUser := &config.User{Username: "files-user", Roles: []string{"canupdate:files"}}
	fileRepo := &config.Repository{Name: "files", Format: config.RepositoryFormatFiles}
	require.NoError(t, authorizeStorageUpload(state, fileUser, fileRepo, "any/path.bin"))
	assert.True(t, errors.Is(authorizeStorageUpload(state, user, fileRepo, "any/path.bin"), fiber.ErrForbidden))

	cargoRepo := &config.Repository{Name: "cargo", Format: config.RepositoryFormatCargo}
	assert.True(t, errors.Is(authorizeStorageUpload(state, fileUser, cargoRepo, "crate.bin"), fiber.ErrMethodNotAllowed))
}
