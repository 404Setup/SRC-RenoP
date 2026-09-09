/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/testutil"
)

func TestResourceLocksPreserveSourcesAndFilterCargoMetadata(t *testing.T) {
	cfg := config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "locks.db")}
	db, err := database.InitDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	now := time.Now().UnixMilli()
	for name, permissions := range map[string][]string{
		"alice": {"base"}, "reader": {"base"}, "moderator": {"canmoderate:cargo"}, "outsider": {"canmoderate:other"},
	} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: name, Permissions: permissions}))
		session := &core.Session{PublicID: name, Username: name, CreatedAt: now}
		session.LastActive.Store(now)
		require.NoError(t, db.SaveSession(session, name+"-session"))
	}
	pkg := &core.CargoPackage{Repository: "cargo", Name: "demo", NormalizedName: "demo", CreatedAt: now, UpdatedAt: now}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		require.NoError(t, db.RecordCargoPublication(pkg, &core.CargoVersion{
			Repository: "cargo", Package: "demo", Version: version, CreatedAt: now, Publisher: "alice",
		}, "alice"))
	}
	reader, err := db.GetUserProfile("reader")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO cargo_members (repository, normalized_name, username, user_id, permission_level, added_at)
		VALUES (?, ?, ?, ?, ?, ?)`, "cargo", "demo", "reader", reader.UserID, 0, now)
	require.NoError(t, err)
	target := core.ResourceLockTarget{Format: "cargo", Repository: "cargo", Name: "DEMO", Version: "2.0.0"}
	lock := &core.ResourceLock{ResourceLockTarget: target, Source: core.ResourceLockManual,
		Mode: core.ResourceLockRead, Reason: "trojan", LockedAt: now}
	require.ErrorIs(t, db.SetResourceLock(lock, "outsider", "outsider-session"), core.ErrResourceLockPermission)
	require.ErrorIs(t, db.SetResourceLock(lock, "moderator", "alice-session"), core.ErrResourceLockPermission)
	require.NoError(t, db.SetResourceLock(lock, "moderator", "moderator-session"))
	require.ErrorIs(t, db.EnsureResourceMutable(target, false), core.ErrResourceLocked)
	sibling := target
	sibling.Version = "3.0.0"
	require.NoError(t, db.EnsureResourceMutable(sibling, false))
	require.ErrorIs(t, db.EnsureResourceMutable(sibling, true), core.ErrResourceLocked)
	packages, total, err := db.SearchCargoPackages("cargo", "demo", "", false, 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "1.0.0", packages[0].MaxVersion)
	packages, _, err = db.SearchCargoPackages("cargo", "demo", "reader", false, 10, 0)
	require.NoError(t, err)
	require.Equal(t, "2.0.0", packages[0].MaxVersion)
	visible, err := db.CargoMetadataVisibility("cargo", "", false, []core.ResourceLockTarget{target, sibling})
	require.NoError(t, err)
	require.Equal(t, []bool{false, true}, visible)
	visible, err = db.CargoMetadataVisibility("cargo", "reader", false, []core.ResourceLockTarget{target})
	require.NoError(t, err)
	require.Equal(t, []bool{true}, visible)
	member, err := db.HasCargoPackageMembership("cargo", "demo", "reader")
	require.NoError(t, err)
	require.True(t, member)
	lock.Source = core.ResourceLockSystem
	lock.Reason = "hold"
	require.NoError(t, db.SetResourceLock(lock, "", ""))
	require.NoError(t, db.Close())
	db, err = database.InitDB(cfg)
	require.NoError(t, err)
	require.ErrorIs(t, db.DeleteResourceLock(target, core.ResourceLockSystem, "moderator", "moderator-session"), core.ErrResourceLockPermission)
	require.NoError(t, db.DeleteResourceLock(target, core.ResourceLockManual, "moderator", "moderator-session"))
	locks, err := db.GetResourceLocks(target, false)
	require.NoError(t, err)
	require.Len(t, locks, 1)
	require.Equal(t, core.ResourceLockSystem, locks[0].Source)
	require.NoError(t, db.DeleteResourceLock(target, core.ResourceLockSystem, "", ""))
	target.Version = ""
	lock.ResourceLockTarget = target
	lock.Source = core.ResourceLockManual
	require.NoError(t, db.SetResourceLock(lock, "moderator", "moderator-session"))
	packages, total, err = db.SearchCargoPackages("cargo", "demo", "", false, 10, 0)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, packages)
	packages, total, err = db.SearchCargoPackages("cargo", "demo", "reader", false, 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, packages, 1)
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "moderator", Permissions: []string{"base"}}))
	require.ErrorIs(t, db.DeleteResourceLock(target, core.ResourceLockManual, "moderator", "moderator-session"), core.ErrResourceLockPermission)
}
