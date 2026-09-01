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
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/testutil"
)

func TestLegacyUserProfileSchemaMigratesImmutableID(t *testing.T) {
	databasePath := filepath.Join(testutil.TempDir(t), "legacy-profile.db")
	rawDB, err := sql.Open("sqlite", databasePath)
	require.NoError(t, err)
	_, err = rawDB.Exec(`CREATE TABLE user_profiles (
		username VARCHAR(255) PRIMARY KEY,
		nickname VARCHAR(144) NOT NULL DEFAULT '',
		rename_window_started_at BIGINT NOT NULL DEFAULT 0,
		rename_count INT NOT NULL DEFAULT 0,
		updated_at BIGINT NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)
	_, err = rawDB.Exec(`INSERT INTO user_profiles
		(username, nickname, rename_window_started_at, rename_count, updated_at)
		VALUES ('admin', 'Administrator', 0, 0, 1)`)
	require.NoError(t, err)
	require.NoError(t, rawDB.Close())

	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: databasePath, MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	var userID string
	require.NoError(t, db.QueryRow(`SELECT user_id FROM user_profiles WHERE username = ?`, "admin").Scan(&userID))
	require.NotEmpty(t, userID)
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Identifier: core.AccessTokenIdentifier{Type: core.Persistent, Value: 1},
		Name:       "admin", CreatedAt: "2026-08-24T00:00:00Z", Permissions: []string{"admin"},
	}))
	profile, err := db.GetUserProfileByID(userID)
	require.NoError(t, err)
	require.Equal(t, "admin", profile.Username)
	require.Equal(t, "Administrator", profile.Nickname)
}

func TestUserProfileRenameIsDurableAndPreservesReferences(t *testing.T) {
	databasePath := filepath.Join(testutil.TempDir(t), "profiles.db")
	openDatabase := func() *database.DB {
		db, err := database.InitDB(config.DatabaseConfig{
			Driver: "sqlite", Dsn: databasePath, MaxOpenConns: 1, MaxIdleConns: 1,
		})
		require.NoError(t, err)
		return db
	}
	db := openDatabase()
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
		}
	})
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
		}
	})

	alice := &core.AccessToken{
		Identifier: core.AccessTokenIdentifier{Type: core.Persistent, Value: 1},
		Name:       "alice", CreatedAt: "2026-08-24T00:00:00Z", Permissions: []string{"base"},
	}
	bob := &core.AccessToken{
		Identifier: core.AccessTokenIdentifier{Type: core.Persistent, Value: 2},
		Name:       "bobby", CreatedAt: "2026-08-24T00:00:00Z", Permissions: []string{"base"},
	}
	require.NoError(t, db.SaveToken(alice))
	require.NoError(t, db.SaveToken(bob))

	const changedAt int64 = 1_800_000_000_000
	profile, err := db.UpdateUserProfile("alice", "alice", "Alice Example", alice, changedAt,
		core.AccountTokenChanges{})
	require.NoError(t, err)
	require.Equal(t, "Alice Example", profile.Nickname)
	require.Equal(t, 0, profile.UsernameChangeCount)
	stableUserID := profile.UserID
	require.NotEmpty(t, stableUserID)
	profile, err = db.UpdateUserProfileLinks("alice", core.PublicLinks{
		Website: "https://alice.example", GitHub: "https://github.com/alice",
		Discord: "https://discord.gg/alice", CustomName: "Docs", CustomURL: "https://docs.alice.example",
	}, changedAt+1)
	require.NoError(t, err)
	require.Equal(t, "https://github.com/alice", profile.Links.GitHub)

	session := &core.Session{
		PublicID: "public-session", Username: "alice", IP: "127.0.0.1",
		UserAgent: "profile-test", CreatedAt: changedAt, LoginMethod: "password",
	}
	session.LastActive.Store(changedAt)
	require.NoError(t, db.SaveSession(session, "secret-session"))
	require.NoError(t, db.SaveAuditLog(&core.AuditLogEntry{
		Username: "alice", Operator: "alice", Action: "PROFILE_TEST", Details: "before rename",
		AuthMethod: "Session", SessionID: "public-session", IP: "127.0.0.1", CreatedAt: changedAt,
	}))
	require.NoError(t, db.SaveMessages([]*core.UserMessage{{
		ID: "profile-message", Recipient: "alice", Sender: "alice", Kind: "system", Severity: "info",
		Title: "Profile", Body: "Profile test", CreatedAt: changedAt,
	}}))
	cargoPackage := &core.CargoPackage{
		Repository: "cargo", Name: "profile-demo", NormalizedName: "profile-demo",
		CreatedAt: changedAt, UpdatedAt: changedAt,
	}
	require.NoError(t, db.RecordCargoPublication(cargoPackage, &core.CargoVersion{
		Repository: "cargo", Package: "profile-demo", Version: "1.0.0",
		Publisher: "alice", CreatedAt: changedAt,
	}, "alice"))
	_, err = db.CreateDockerImage("docker", "profile/demo", "alice", false, changedAt)
	require.NoError(t, err)
	require.NoError(t, db.PutDockerManifest(&core.DockerManifest{
		Repository: "docker", ImageName: "profile/demo",
		Digest:    "sha256:abc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdeb",
		MediaType: "application/vnd.oci.image.manifest.v1+json", RawJSON: []byte(`{"schemaVersion":2}`),
	}, "latest", "alice"))
	_, err = db.CreateNPMPackage("npm", "@profile/demo", "alice", false, changedAt)
	require.NoError(t, err)

	profile, err = db.UpdateUserProfile("alice", "alice_one", "Alice Example", alice, changedAt+1,
		core.AccountTokenChanges{})
	require.NoError(t, err)
	require.Equal(t, "alice_one", profile.Username)
	require.Equal(t, stableUserID, profile.UserID)
	require.Equal(t, 1, profile.UsernameChangeCount)
	richProfile, err := db.GetUserProfileByID(stableUserID)
	require.NoError(t, err)
	require.Equal(t, "https://alice.example", richProfile.Links.Website)
	require.Equal(t, "Docs", richProfile.Links.CustomName)
	require.Equal(t, 1, richProfile.CargoPackageCount)
	require.Equal(t, 1, richProfile.DockerImageCount)
	require.Equal(t, 1, richProfile.NPMPackageCount)
	cargoMemberships, err := db.ListUserPackageMemberships(stableUserID, config.RepositoryFormatCargo)
	require.NoError(t, err)
	require.Len(t, cargoMemberships, 1)
	require.Equal(t, "profile-demo", cargoMemberships[0].Name)
	require.Equal(t, core.CargoPermissionOwner, cargoMemberships[0].PermissionLevel)
	dockerMemberships, err := db.ListUserPackageMemberships(stableUserID, config.RepositoryFormatDocker)
	require.NoError(t, err)
	require.Len(t, dockerMemberships, 1)
	require.Equal(t, "profile/demo", dockerMemberships[0].Name)
	require.Equal(t, core.DockerPermissionOwner, dockerMemberships[0].PermissionLevel)
	npmMemberships, err := db.ListUserPackageMemberships(stableUserID, config.RepositoryFormatNPM)
	require.NoError(t, err)
	require.Len(t, npmMemberships, 1)
	require.Equal(t, "@profile/demo", npmMemberships[0].Name)
	require.Equal(t, core.NPMPermissionOwner, npmMemberships[0].PermissionLevel)
	_, err = db.GetUserProfile("alice")
	require.ErrorIs(t, err, core.ErrUserProfileNotFound)
	require.NotNil(t, mustToken(t, db, "alice_one"))

	renamedSession, err := db.GetSession("secret-session")
	require.NoError(t, err)
	require.Equal(t, "alice_one", renamedSession.Username)
	cargoDetails, err := db.GetCargoPackageDetails("cargo", "profile-demo", "alice_one")
	require.NoError(t, err)
	require.Equal(t, core.CargoPermissionOwner, cargoDetails.Package.PermissionLevel)
	dockerLevel, err := db.GetDockerMemberLevel("docker", "profile/demo", "alice_one")
	require.NoError(t, err)
	require.Equal(t, core.DockerPermissionOwner, dockerLevel)
	npmDetails, err := db.GetNPMPackageDetails("npm", "@profile/demo", "alice_one")
	require.NoError(t, err)
	require.True(t, npmDetails.Member)
	require.Equal(t, core.NPMPermissionOwner, npmDetails.Package.PermissionLevel)
	var auditUsername, auditOperator string
	require.NoError(t, db.QueryRow(`SELECT username, operator FROM audit_logs WHERE action = ?`, "PROFILE_TEST").Scan(
		&auditUsername, &auditOperator,
	))
	require.Equal(t, "alice_one", auditUsername)
	require.Equal(t, "alice_one", auditOperator)
	message, err := db.GetUserMessage("profile-message", "alice_one", changedAt+2)
	require.NoError(t, err)
	require.Equal(t, "alice_one", message.Sender)

	_, err = db.UpdateUserProfile("alice_one", "bobby", "Alice Example", mustToken(t, db, "alice_one"), changedAt+2,
		core.AccountTokenChanges{})
	require.ErrorIs(t, err, core.ErrUsernameAlreadyExists)
	require.NotNil(t, mustToken(t, db, "alice_one"))

	profile, err = db.UpdateUserProfile(
		"alice_one", "alice_two", "Alice Example", mustToken(t, db, "alice_one"), changedAt+3,
		core.AccountTokenChanges{},
	)
	require.NoError(t, err)
	require.Equal(t, 2, profile.UsernameChangeCount)
	require.Equal(t, stableUserID, profile.UserID)
	require.NoError(t, db.Close())

	db = openDatabase()
	_, err = db.UpdateUserProfile(
		"alice_two", "alice_three", "Alice Example", mustToken(t, db, "alice_two"), changedAt+4,
		core.AccountTokenChanges{},
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, core.ErrUsernameChangeRateLimited))
	var rateError *core.UsernameChangeRateError
	require.ErrorAs(t, err, &rateError)
	require.Equal(t, changedAt+1+core.UsernameChangeWindowMillis, rateError.RetryAt)
	require.NotNil(t, mustToken(t, db, "alice_two"))
}

func TestStableMembershipIDsAreBackfilledOnRestart(t *testing.T) {
	databasePath := filepath.Join(testutil.TempDir(t), "identity-backfill.db")
	openDatabase := func() *database.DB {
		db, err := database.InitDB(config.DatabaseConfig{
			Driver: "sqlite", Dsn: databasePath, MaxOpenConns: 1, MaxIdleConns: 1,
		})
		require.NoError(t, err)
		return db
	}
	db := openDatabase()
	account := &core.AccessToken{
		Identifier: core.AccessTokenIdentifier{Type: core.Persistent, Value: 1},
		Name:       "backfill_user", CreatedAt: "2026-08-24T00:00:00Z", Permissions: []string{"base"},
	}
	require.NoError(t, db.SaveToken(account))
	const now int64 = 1_800_000_200_000
	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{
		Repository: "cargo", Name: "backfill-demo", NormalizedName: "backfill-demo", CreatedAt: now, UpdatedAt: now,
	}, &core.CargoVersion{
		Repository: "cargo", Package: "backfill-demo", Version: "1.0.0", Publisher: account.Name, CreatedAt: now,
	}, account.Name))
	_, err := db.CreateDockerImage("docker", "backfill/demo", account.Name, false, now)
	require.NoError(t, err)
	require.NoError(t, db.PutDockerManifest(&core.DockerManifest{
		Repository: "docker", ImageName: "backfill/demo",
		Digest:    "sha256:abc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdec",
		MediaType: "application/vnd.oci.image.manifest.v1+json", RawJSON: []byte(`{"schemaVersion":2}`),
	}, "latest", account.Name))
	require.NoError(t, execTestSQL(db, `UPDATE cargo_members SET user_id = NULL`))
	require.NoError(t, execTestSQL(db, `UPDATE docker_members SET user_id = NULL`))
	require.NoError(t, db.Close())

	db = openDatabase()
	profile, err := db.GetUserProfile(account.Name)
	require.NoError(t, err)
	var cargoUserID, dockerUserID string
	require.NoError(t, db.QueryRow(`SELECT user_id FROM cargo_members WHERE repository = ?`, "cargo").Scan(&cargoUserID))
	require.NoError(t, db.QueryRow(`SELECT user_id FROM docker_members WHERE repository = ?`, "docker").Scan(&dockerUserID))
	require.Equal(t, profile.UserID, cargoUserID)
	require.Equal(t, profile.UserID, dockerUserID)
	details, err := db.GetCargoPackageDetails("cargo", "backfill-demo", account.Name)
	require.NoError(t, err)
	require.Equal(t, core.CargoPermissionOwner, details.Package.PermissionLevel)
	dockerLevel, err := db.GetDockerMemberLevel("docker", "backfill/demo", account.Name)
	require.NoError(t, err)
	require.Equal(t, core.DockerPermissionOwner, dockerLevel)
	require.NoError(t, db.Close())
	db = nil
}

func execTestSQL(db *database.DB, query string) error {
	_, err := db.Exec(query)
	return err
}

func mustToken(t *testing.T, db *database.DB, name string) *core.AccessToken {
	t.Helper()
	token, err := db.GetTokenByName(name)
	require.NoError(t, err)
	require.NotNil(t, token)
	return token
}
