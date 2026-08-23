/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func newCargoDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "cargo.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func TestCargoOwnershipInvitationAndAdministratorLocks(t *testing.T) {
	db := newCargoDB(t)
	now := int64(1000)
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "alice", Identifier: core.AccessTokenIdentifier{Type: core.Persistent},
	}))
	pkg := &core.CargoPackage{
		Repository: "cargo", Name: "demo", NormalizedName: "demo",
		Description: "Demo crate", RepositoryURL: "https://github.com/example/demo",
		Homepage: "https://example.com/demo", Documentation: "https://docs.rs/demo",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.RecordCargoPublication(pkg, &core.CargoVersion{
		Repository: "cargo", Package: "demo", Version: "1.0.0", Publisher: "alice",
		Size: 1024, Checksum: "abcdef123456", RustVersion: "1.70.0", License: "MIT",
		Documentation: "https://docs.rs/demo", Homepage: "https://example.com/demo",
		RepositoryURL: "https://github.com/example/demo", CreatedAt: now,
	}, "alice"))

	details, err := db.GetCargoPackageDetails("cargo", "demo", "alice")
	require.NoError(t, err)
	require.Equal(t, core.CargoPermissionFull, details.Package.PermissionLevel)
	require.Equal(t, "https://github.com/example/demo", details.Package.RepositoryURL)
	require.Equal(t, "https://example.com/demo", details.Package.Homepage)
	require.Equal(t, "https://docs.rs/demo", details.Package.Documentation)
	require.Len(t, details.Versions, 1)
	require.Equal(t, int64(1024), details.Versions[0].Size)
	require.Equal(t, "abcdef123456", details.Versions[0].Checksum)
	require.Equal(t, "1.70.0", details.Versions[0].RustVersion)
	require.Equal(t, "MIT", details.Versions[0].License)
	require.Len(t, details.Members, 1)
	err = db.DeleteToken("alice")
	require.Error(t, err)
	require.Contains(t, err.Error(), "last L3 member")

	err = db.RecordCargoPublication(pkg, &core.CargoVersion{
		Repository: "cargo", Package: "demo", Version: "1.1.0", Publisher: "bob", CreatedAt: now + 1,
	}, "bob")
	require.ErrorIs(t, err, core.ErrCargoPermissionDenied)

	id := uuid.NewString()
	message := &core.UserMessage{
		ID: id, Recipient: "bob", Sender: "alice", Kind: "cargo_package_invite", Severity: "info",
		Title: "Cargo package invitation", Body: "Join demo", Payload: []byte(`{"repository":"cargo","package":"demo","level":2}`),
		ActionKind: "cargo_package_invite", ActionStatus: core.MessageActionPending, CreatedAt: now, ExpiresAt: now + 1000,
	}
	require.NoError(t, db.CreateCargoInvitations([]*core.CargoInvitation{{
		ID: id, Repository: "cargo", Package: "demo", NormalizedName: "demo",
		Inviter: "alice", Recipient: "bob", Level: core.CargoPermissionVersion, CreatedAt: now,
	}}, []*core.UserMessage{message}))
	require.NoError(t, db.RespondCargoInvitation(id, "bob", "cargo", true, now+1))

	details, err = db.GetCargoPackageDetails("cargo", "demo", "bob")
	require.NoError(t, err)
	require.Equal(t, core.CargoPermissionVersion, details.Package.PermissionLevel)
	require.NoError(t, db.RecordCargoPublication(pkg, &core.CargoVersion{
		Repository: "cargo", Package: "demo", Version: "1.1.0", Publisher: "bob", CreatedAt: now + 2,
	}, "bob"))
	require.NoError(t, db.RecordCargoPublication(pkg, &core.CargoVersion{
		Repository: "cargo", Package: "demo", Version: "1.10.0", Publisher: "bob", CreatedAt: now + 3,
	}, "bob"))
	results, total, err := db.SearchCargoPackages("cargo", "demo", 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, results, 1)
	require.Equal(t, "1.10.0", results[0].MaxVersion)

	err = db.SetCargoMemberLevel("cargo", "demo", "bob", "alice", core.CargoPermissionVersion)
	require.ErrorIs(t, err, core.ErrCargoPermissionDenied)
	err = db.RemoveCargoMember("cargo", "demo", "bob", "alice")
	require.ErrorIs(t, err, core.ErrCargoPermissionDenied)

	require.NoError(t, db.SetCargoVersionYanked("cargo", "demo", "1.1.0", true, true))
	err = db.SetCargoVersionYanked("cargo", "demo", "1.1.0", false, false)
	require.ErrorIs(t, err, core.ErrCargoAdminYanked)

	require.NoError(t, db.SetCargoPackageArchived("cargo", "demo", true, true))
	err = db.SetCargoPackageArchived("cargo", "demo", false, false)
	require.ErrorIs(t, err, core.ErrCargoAdminArchived)

	err = db.RemoveCargoMember("cargo", "demo", "alice", "alice")
	require.True(t, errors.Is(err, core.ErrCargoLastFullMember))
}

func TestExpiredCargoInvitationCanBeReissued(t *testing.T) {
	db := newCargoDB(t)
	const now int64 = 1000
	pkg := &core.CargoPackage{
		Repository: "cargo", Name: "demo", NormalizedName: "demo", CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.RecordCargoPublication(pkg, &core.CargoVersion{
		Repository: "cargo", Package: "demo", Version: "1.0.0", Publisher: "alice", CreatedAt: now,
	}, "alice"))

	createInvitation := func(id string, createdAt, expiresAt int64) error {
		return db.CreateCargoInvitations([]*core.CargoInvitation{{
			ID: id, Repository: "cargo", Package: "demo", NormalizedName: "demo",
			Inviter: "alice", Recipient: "bob", Level: core.CargoPermissionPublish, CreatedAt: createdAt,
		}}, []*core.UserMessage{{
			ID: id, Recipient: "bob", Sender: "alice", Kind: "cargo_package_invite", Severity: "info",
			Title: "Cargo package invitation", Body: "Join demo", ActionKind: "cargo_package_invite",
			ActionStatus: core.MessageActionPending, CreatedAt: createdAt, ExpiresAt: expiresAt,
		}})
	}

	firstID := uuid.NewString()
	require.NoError(t, createInvitation(firstID, now, now+10))
	secondID := uuid.NewString()
	require.NoError(t, createInvitation(secondID, now+11, now+100))
	require.NoError(t, db.RespondCargoInvitation(secondID, "bob", "cargo", true, now+12))

	details, err := db.GetCargoPackageDetails("cargo", "demo", "bob")
	require.NoError(t, err)
	require.Equal(t, core.CargoPermissionPublish, details.Package.PermissionLevel)
}
