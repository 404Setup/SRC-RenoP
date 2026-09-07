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
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

func TestAccountRetirementColumnsMigrateFromLegacySchema(t *testing.T) {
	databasePath := filepath.Join(testutil.TempDir(t), "legacy-account-retirement.db")
	rawDB, err := sql.Open("sqlite", databasePath)
	require.NoError(t, err)
	_, err = rawDB.Exec(`CREATE TABLE tokens (
		name VARCHAR(255) PRIMARY KEY, type VARCHAR(50) NOT NULL, type_value INTEGER NOT NULL,
		encrypted_secret TEXT NOT NULL, password_hash TEXT NOT NULL DEFAULT '', tokens_json TEXT NOT NULL,
		created_at TEXT NOT NULL, description TEXT NOT NULL, expires_at BIGINT NULL,
		permissions_json TEXT NOT NULL, ban_reason VARCHAR(2048) NOT NULL DEFAULT '',
		banned_at BIGINT NOT NULL DEFAULT 0, banned_until BIGINT NULL
	)`)
	require.NoError(t, err)
	_, err = rawDB.Exec(`INSERT INTO tokens
		(name, type, type_value, encrypted_secret, password_hash, tokens_json, created_at, description,
		permissions_json) VALUES ('legacy', 'PERSISTENT', 1, '', '', '[]', '2026-09-01T00:00:00Z', '', '[]')`)
	require.NoError(t, err)
	require.NoError(t, rawDB.Close())

	db, err := InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: databasePath, MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	account, err := db.GetTokenByName("legacy")
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Zero(t, account.DeletedAt)
	assert.Zero(t, account.EmailReleasedAt)
	assert.Zero(t, account.AuditPurgedAt)
}

func newAccountRetirementDB(t *testing.T) *DB {
	t.Helper()
	db, err := InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "account-retirement.db"), MaxOpenConns: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, account := range []*core.AccessToken{
		{Name: "admin", EncryptedSecret: "admin-password", Permissions: []string{"manager"}},
		{Name: "alice", EncryptedSecret: "alice-password", Permissions: []string{"base"}},
		{Name: "bob", EncryptedSecret: "bob-password", Permissions: []string{"base"}},
	} {
		account.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		require.NoError(t, db.SaveToken(account))
	}
	return db
}

func TestAccountRetirementPlanReportsEveryRequiredBlocker(t *testing.T) {
	db := newAccountRetirementDB(t)
	now := time.Now().UnixMilli()
	alice, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	alice.Permissions = []string{"base", "canmoderate:releases"}
	require.NoError(t, db.SaveToken(alice))
	require.NoError(t, db.CreateSuperTeam(&core.SuperTeam{
		Prefix: "owners", Name: "Owners", CreatedAt: now,
	}, "alice", 10, 20))
	require.NoError(t, db.CreateMavenDomain(&core.MavenDomain{
		Domain: "com.blocked", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "blocked.com", VerificationCode: "blocked", CreatedAt: now,
	}, "alice"))
	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{
		Repository: "cargo", Name: "blocked", NormalizedName: "blocked", CreatedAt: now, UpdatedAt: now,
	}, &core.CargoVersion{
		Repository: "cargo", Package: "blocked", Version: "1.0.0", Publisher: "alice", CreatedAt: now,
	}, "alice"))
	_, err = db.CreateOrUpdatePublicationReview(core.PublicationReviewRequest{
		ResourceType: core.ReviewResourceCargoPackage, Repository: "cargo", ResourceKey: "blocked",
		ResourceName: "blocked", Version: "2.0.0", RequestedBy: "alice",
		Policy: config.PublicationReviewEveryVersion,
		Files:  []*core.ReviewFile{{Path: "blocked/2.0.0.crate", Size: 10}}, CreatedAt: now + 1,
	})
	require.NoError(t, err)
	plan, err := db.GetAccountRetirementPlan("alice")
	require.NoError(t, err)
	assert.False(t, plan.Eligible)
	assert.True(t, plan.ProtectedRole)
	assert.Equal(t, 1, plan.SuperTeamOwnerCount)
	assert.Equal(t, 1, plan.MavenDomainOwnerCount)
	assert.Equal(t, 1, plan.PackageOwnerCount)
	assert.Equal(t, 1, plan.PendingReviewCount)
	require.ErrorIs(t, db.RetireAccount("alice", now+2), core.ErrAccountRetirementBusy)
}

func TestAccountRetirementLocksIdentityAndHonorsRetention(t *testing.T) {
	db := newAccountRetirementDB(t)
	now := time.Now().UnixMilli()
	profile, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	_, err = db.UpdateAccountEmail("alice", "alice@example.com", now)
	require.NoError(t, err)
	require.NoError(t, db.SaveFidoDevice(&core.FidoDevice{
		ID: uuid.NewString(), Username: "alice", Name: "Passkey", CredentialID: []byte("credential"),
		PublicKey: []byte("public-key"), CreatedAt: now,
	}))
	require.NoError(t, db.CreateAPIToken("alice", &core.APIToken{
		ID: uuid.NewString(), Name: "automation", Scopes: []string{core.APITokenScopeAccountRead}, CreatedAt: now,
	}, core.HashAPITokenSecret("secret")))
	require.NoError(t, db.ReplaceRecoveryCodes("alice", testRecoveryHashes(now)))
	require.NoError(t, db.StoreGitHubIdentity(profile.UserID, 42, "alice-gh", []core.GitHubPrincipal{{
		Type: core.GitHubPrincipalUser, GitHubID: 42, Login: "alice-gh",
	}}, now))
	avatarData := []byte("avatar")
	avatarHash := sha256.Sum256(avatarData)
	require.NoError(t, db.PutUserAvatar("alice", &core.UserAvatar{
		ContentType: "image/png", Data: avatarData, Size: int64(len(avatarData)),
		SHA256: hex.EncodeToString(avatarHash[:]), UpdatedAt: now,
	}))
	session := &core.Session{Username: "alice", PublicID: "session", CreatedAt: now}
	session.LastActive.Store(now)
	require.NoError(t, db.SaveSession(session, "session-token"))
	require.NoError(t, db.SaveMessages([]*core.UserMessage{{
		ID: uuid.NewString(), Recipient: "alice", Sender: "system", Kind: "notice", Severity: "info",
		Title: "Notice", Body: "Body", CreatedAt: now,
	}}))
	require.NoError(t, db.SaveAuditLog(&core.AuditLogEntry{
		Username: "alice", Operator: "alice", Action: "LOGIN",
		CreatedAt: now - 40*24*60*60*1000,
	}))
	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{
		Repository: "cargo", Name: "frozen", NormalizedName: "frozen", CreatedAt: now, UpdatedAt: now,
	}, &core.CargoVersion{
		Repository: "cargo", Package: "frozen", Version: "1.0.0", Publisher: "alice", CreatedAt: now,
	}, "alice"))
	require.NoError(t, db.DeprecatePackage(config.RepositoryFormatCargo, "cargo", "frozen", now+1))
	_, err = db.CreateDockerImage("docker", "frozen", "alice", false, now)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM docker_members WHERE repository = ? AND image_name = ?`, "docker", "frozen")
	require.NoError(t, err)
	legacyOwnerPlan, err := db.GetAccountRetirementPlan("alice")
	require.NoError(t, err)
	assert.Equal(t, 1, legacyOwnerPlan.PackageOwnerCount)
	require.NoError(t, db.DeprecatePackage(config.RepositoryFormatDocker, "docker", "frozen", now+1))
	_, err = db.CreateNPMPackage("npm", "frozen", "alice", false, now)
	require.NoError(t, err)
	require.NoError(t, db.DeprecatePackage(config.RepositoryFormatNPM, "npm", "frozen", now+1))
	require.NoError(t, db.CreateMavenDomain(&core.MavenDomain{
		Domain: "com.closed", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "closed.com", VerificationCode: "closed", CreatedAt: now,
	}, "alice"))
	require.NoError(t, db.CloseMavenDomain("com.closed", "alice", false, now+1))
	require.NoError(t, db.CreateSuperTeam(&core.SuperTeam{
		Prefix: "members", Name: "Members", CreatedAt: now,
	}, "bob", 10, 20))
	require.NoError(t, db.ForceAddSuperTeamMembers("members", "admin", []string{"alice"},
		core.SuperTeamRoleRead, 10, 20, now+1))
	plan, err := db.GetAccountRetirementPlan("alice")
	require.NoError(t, err)
	assert.True(t, plan.Eligible)

	retiredAt := now + 2
	require.NoError(t, db.RetireAccount("alice", retiredAt))
	db.finishTokenUpdate("alice", &core.AccessToken{
		Name: "alice", EncryptedSecret: "delayed-write", Permissions: []string{"base"},
	})
	account, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Equal(t, retiredAt, account.DeletedAt)
	assert.Empty(t, account.EncryptedSecret)
	assert.Empty(t, account.Permissions)
	dockerMembers, err := db.ListDockerMembers("docker", "frozen")
	require.NoError(t, err)
	assert.Empty(t, dockerMembers)
	assert.ErrorIs(t, db.SaveToken(&core.AccessToken{Name: "alice", EncryptedSecret: "stale-password"}), core.ErrAccountDeleted)
	assert.ErrorIs(t, db.SaveSession(session, "late-session"), core.ErrAccountDeleted)
	assert.ErrorIs(t, db.SaveFidoDevice(&core.FidoDevice{
		ID: uuid.NewString(), Username: "alice", CredentialID: []byte("late-credential"),
	}), core.ErrAccountDeleted)
	assert.ErrorIs(t, db.StoreGitHubIdentity(profile.UserID, 42, "alice-gh", []core.GitHubPrincipal{{
		Type: core.GitHubPrincipalUser, GitHubID: 42, Login: "alice-gh",
	}}, retiredAt+1), core.ErrAccountDeleted)
	_, err = db.UpdateAccountEmail("bob", "alice@example.com", retiredAt+1)
	assert.ErrorIs(t, err, core.ErrEmailAlreadyExists)
	lateMessage := &core.UserMessage{ID: uuid.NewString(), Recipient: "alice", Kind: "notice",
		Severity: "info", Title: "Late", Body: "Queued before closure", CreatedAt: retiredAt + 1}
	require.NoError(t, db.SaveMessages([]*core.UserMessage{lateMessage}))
	inserted, err := db.SaveMessageIfAbsent(lateMessage)
	require.NoError(t, err)
	assert.False(t, inserted)
	activeAccounts, err := db.CountTokens()
	require.NoError(t, err)
	assert.Equal(t, uint64(2), activeAccounts)
	assert.ErrorIs(t, db.DeleteToken("alice"), core.ErrAccountDeleted)
	assert.ErrorIs(t, db.CreateToken(&core.AccessToken{Name: "alice"}, "", retiredAt+1),
		core.ErrUsernameAlreadyExists)
	profile, err = db.GetUserProfile("alice")
	require.NoError(t, err)
	assert.Empty(t, profile.Nickname)
	assert.Empty(t, profile.Links.Website)

	for table, column := range map[string]string{
		"fido_devices": "username", "sessions": "username", "user_messages": "recipient",
		"cargo_members": "user_id", "maven_domain_members": "user_id", "super_team_members": "user_id",
		"docker_members": "user_id", "npm_members": "user_id",
	} {
		value := "alice"
		if column == "user_id" {
			value = profile.UserID
		}
		var count int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+column+` = ?`, value).Scan(&count))
		assert.Zero(t, count, table)
	}
	for _, table := range []string{"github_identities", "github_principals", "user_recovery_codes"} {
		var count int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE user_id = ?`, profile.UserID).Scan(&count))
		assert.Zero(t, count, table)
	}
	identity, err := db.GetGitHubIdentityByProviderID(42)
	require.NoError(t, err)
	require.Nil(t, identity)
	bob, err := db.GetUserProfile("bob")
	require.NoError(t, err)
	require.NoError(t, db.StoreGitHubIdentity(bob.UserID, 42, "alice-gh", []core.GitHubPrincipal{{
		Type: core.GitHubPrincipalUser, GitHubID: 42, Login: "alice-gh",
	}}, retiredAt+1))
	identity, err = db.GetGitHubIdentityByProviderID(42)
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, "bob", identity.Username)
	apiTokens, err := db.ListAPITokens("alice")
	require.NoError(t, err)
	assert.Empty(t, apiTokens)
	avatar, err := db.GetUserAvatar("alice")
	require.ErrorIs(t, err, core.ErrUserAvatarNotFound)
	assert.Nil(t, avatar)
	byEmail, err := db.GetTokenByEmail("alice@example.com")
	require.NoError(t, err)
	require.NotNil(t, byEmail)
	assert.Equal(t, retiredAt, byEmail.DeletedAt)
	status, err := db.GetAccountRetirementStatus("alice")
	require.NoError(t, err)
	assert.Equal(t, retiredAt+core.AccountEmailHoldMillis, status.EmailReleaseAt)
	assert.Equal(t, retiredAt+core.AccountAuditRetentionMillis, status.AuditPurgeAt)
	require.NoError(t, db.CleanExpiredAuditLogs(1, 1))

	require.NoError(t, db.CleanupRetiredAccountData(retiredAt+core.AccountEmailHoldMillis-1, 10))
	byEmail, err = db.GetTokenByEmail("alice@example.com")
	require.NoError(t, err)
	require.NotNil(t, byEmail)
	require.NoError(t, db.CleanupRetiredAccountData(retiredAt+core.AccountEmailHoldMillis, 10))
	byEmail, err = db.GetTokenByEmail("alice@example.com")
	require.NoError(t, err)
	assert.Nil(t, byEmail)
	logs, total, err := db.GetAuditLogs("alice", 10, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, logs)
	assert.Positive(t, total)
	require.NoError(t, db.CleanupRetiredAccountData(retiredAt+core.AccountAuditRetentionMillis, 10))
	require.NoError(t, db.SaveAuditLog(&core.AuditLogEntry{
		Username: "alice", Operator: "alice", Action: "LATE_EVENT", CreatedAt: retiredAt,
	}))
	require.NoError(t, db.SaveAuditLog(&core.AuditLogEntry{
		Username: "bob", Operator: "alice", Action: "LATE_EVENT", CreatedAt: retiredAt,
	}))
	logs, total, err = db.GetAuditLogs("alice", 10, 0)
	require.NoError(t, err)
	assert.Empty(t, logs)
	assert.Zero(t, total)
	_, total, err = db.GetAuditLogs("bob", 10, 0)
	require.NoError(t, err)
	assert.Zero(t, total)
}
