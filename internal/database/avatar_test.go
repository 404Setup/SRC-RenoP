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
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/testutil"
)

func TestUserAvatarPersistsByImmutableAccount(t *testing.T) {
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "avatars.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", CreatedAt: time.Now().UTC().Format(time.RFC3339)}))

	data := []byte{'s', 'a', 'n', 'i', 't', 'i', 'z', 'e', 'd', 0, 'p', 'n', 'g'}
	sum := sha256.Sum256(data)
	require.NoError(t, db.PutUserAvatar("alice", &core.UserAvatar{
		ContentType: "image/png", Data: data, Size: int64(len(data)),
		SHA256: hex.EncodeToString(sum[:]), UpdatedAt: time.Now().UnixMilli(),
	}))
	avatar, err := db.GetUserAvatar("alice")
	require.NoError(t, err)
	require.Equal(t, data, avatar.Data)
	require.Equal(t, "image/png", avatar.ContentType)
	require.NoError(t, db.PutUserAvatar("alice", avatar))
	profile, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	require.Equal(t, avatar.SHA256, profile.AvatarHash)
	profiles, err := db.GetUserProfiles([]string{"alice"})
	require.NoError(t, err)
	require.Equal(t, avatar.SHA256, profiles["alice"].AvatarHash)

	require.NoError(t, db.DeleteUserAvatar("alice"))
	_, err = db.GetUserAvatar("alice")
	require.ErrorIs(t, err, core.ErrUserAvatarNotFound)
	require.NoError(t, db.PutUserAvatar("alice", &core.UserAvatar{
		ContentType: "image/png", Data: data, Size: int64(len(data)),
		SHA256: hex.EncodeToString(sum[:]), UpdatedAt: time.Now().UnixMilli(),
	}))
	require.NoError(t, db.DeleteToken("alice"))
	_, err = db.GetUserAvatar("alice")
	require.ErrorIs(t, err, core.ErrUserAvatarNotFound)
}
