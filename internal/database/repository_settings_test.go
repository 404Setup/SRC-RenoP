/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"renop/internal/config"
)

func TestRepositorySettingsPreserveCommittedSnapshotOnFailure(t *testing.T) {
	db := newMavenDB(t)
	original := config.DefaultMavenSettings()
	require.NoError(t, db.SaveRepositorySettings(original))
	_, err := db.Exec("CREATE TRIGGER reject_repository_settings BEFORE INSERT ON repository_settings BEGIN SELECT RAISE(ABORT, 'injected failure'); END")
	require.NoError(t, err)
	require.Error(t, db.SaveRepositorySettings(config.MavenSettings{}))
	stored, err := db.GetRepositorySettings()
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Len(t, stored.Repositories, len(original.Repositories))
	require.NotNil(t, stored.Repositories["private"])
}
