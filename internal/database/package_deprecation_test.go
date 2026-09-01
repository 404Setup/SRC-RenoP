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
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/testutil"
)

func TestPackageDeprecationIsPermanentAndReviewSafe(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "package-deprecation.db"), MaxOpenConns: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	now := time.Now().UnixMilli()
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "publisher", Permissions: []string{"base"}}))
	for _, target := range []struct {
		format    string
		key       string
		lookupKey string
	}{
		{config.RepositoryFormatCargo, "crate_name", "Crate-Name"},
		{config.RepositoryFormatNPM, "@team/package", "@TEAM/PACKAGE"},
		{config.RepositoryFormatDocker, "team/image", "TEAM/IMAGE"},
		{config.RepositoryFormatMaven, "com.example:artifact", "com.example:artifact"},
	} {
		require.NoError(t, db.DeprecatePackage(target.format, "releases", target.key, now))
		deprecated, err := db.IsPackageDeprecated(target.format, "releases", target.lookupKey)
		require.NoError(t, err)
		assert.True(t, deprecated)
		require.ErrorIs(t, db.EnsurePackageMutable(target.format, "releases", target.lookupKey),
			core.ErrPackageDeprecated)
		require.ErrorIs(t, db.DeprecatePackage(target.format, "releases", target.key, now+1),
			core.ErrPackageDeprecated)
	}

	pendingKey := "pending-package"
	_, err = db.CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceNPMPackage, Repository: "npm", ResourceKey: pendingKey,
		ResourceName: pendingKey, Version: "1.0.0", RequestedBy: "publisher",
		Policy: config.PublicationReviewEveryVersion,
		Files:  []*core.ReviewFile{{Path: "pending-package-1.0.0.tgz", Size: 10}}, CreatedAt: now,
	})
	require.NoError(t, err)
	require.ErrorIs(t, db.DeprecatePackage(config.RepositoryFormatNPM, "npm", pendingKey, now),
		core.ErrPackageDeprecationPending)
	deprecated, err := db.IsPackageDeprecated(config.RepositoryFormatNPM, "npm", pendingKey)
	require.NoError(t, err)
	assert.False(t, deprecated)

	require.ErrorIs(t, db.DeprecatePackage("unknown", "repo", uuid.NewString(), now),
		core.ErrPackageDeprecationInvalid)
}
