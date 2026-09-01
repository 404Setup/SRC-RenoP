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
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func newPostgresTestSchema(t *testing.T, prefix string) (string, *sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("RENOP_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("RENOP_TEST_POSTGRES_DSN is not configured")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	schema := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	admin, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	require.NoError(t, admin.Ping())
	_, err = admin.Exec(`CREATE SCHEMA "` + schema + `"`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, dropErr := admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
		require.NoError(t, dropErr)
		require.NoError(t, admin.Close())
	})
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), admin, schema
}

func TestPostgresUserProfileIntegration(t *testing.T) {
	dsn, admin, schema := newPostgresTestSchema(t, "renop_profile_test")
	_, err := admin.Exec(`CREATE TABLE "` + schema + `".user_profiles (
		username VARCHAR(255) PRIMARY KEY,
		nickname VARCHAR(144) NOT NULL DEFAULT '',
		rename_window_started_at BIGINT NOT NULL DEFAULT 0,
		rename_count INT NOT NULL DEFAULT 0,
		updated_at BIGINT NOT NULL DEFAULT 0
	)`)
	require.NoError(t, err)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "postgres", Dsn: dsn, MaxOpenConns: 2, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	account := &core.AccessToken{
		Identifier: core.AccessTokenIdentifier{Type: core.Persistent, Value: 1},
		Name:       "profile_pg", CreatedAt: "2026-08-24T00:00:00Z", Permissions: []string{"base"},
	}
	require.NoError(t, db.SaveToken(account))
	const changedAt int64 = 1_800_000_100_000
	profile, err := db.UpdateUserProfile("profile_pg", "profile_pg", "PostgreSQL User", account, changedAt,
		core.AccountTokenChanges{})
	require.NoError(t, err)
	require.Equal(t, "PostgreSQL User", profile.Nickname)
	stableUserID := profile.UserID
	profile, err = db.UpdateUserProfileLinks("profile_pg", core.PublicLinks{
		Website: "https://profile.pg.example", GitHub: "https://github.com/profile-pg",
	}, changedAt+1)
	require.NoError(t, err)
	require.Equal(t, "https://github.com/profile-pg", profile.Links.GitHub)
	team := &core.SuperTeam{Prefix: "profile-pg", Name: "Profile PostgreSQL", CreatedAt: changedAt + 1}
	require.NoError(t, db.CreateSuperTeam(team, "profile_pg", 2, 2))
	require.NoError(t, db.SetSuperTeamMemberVisibility(team.Prefix, "profile_pg", false))
	publicTeam, err := db.GetPublicSuperTeamDetails(team.Prefix, "", false)
	require.NoError(t, err)
	require.Empty(t, publicTeam.Members)
	require.Empty(t, publicTeam.Team.CreatedBy)
	visibleTeams, visibleTotal, err := db.ListVisibleUserSuperTeams(profile.UserID, "profile_pg", false, 10, 0)
	require.NoError(t, err)
	require.Len(t, visibleTeams, 1)
	require.Equal(t, 1, visibleTotal)
	require.False(t, visibleTeams[0].Visible)
	_, err = db.CreateNPMPackageForTeam("npm", "@profile-pg/private", "profile_pg", team.Prefix, true, changedAt+2)
	require.NoError(t, err)
	teamResources, resourceTotal, err := db.ListSuperTeamResources(core.SuperTeamResourceListOptions{
		Prefix: team.Prefix, Format: config.RepositoryFormatNPM, Viewer: "profile_pg", Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, teamResources, 1)
	require.Equal(t, 1, resourceTotal)
	require.NoError(t, db.StoreGitHubIdentity(stableUserID, 9001, "profile-pg", []core.GitHubPrincipal{
		{Type: core.GitHubPrincipalUser, GitHubID: 9001, Login: "profile-pg"},
		{Type: core.GitHubPrincipalOrganization, GitHubID: 9002, Login: "renop-pg"},
	}, changedAt))
	linkedGitHub, err := db.GetGitHubIdentity("profile_pg")
	require.NoError(t, err)
	require.Equal(t, int64(9001), linkedGitHub.GitHubUserID)
	githubAuthorized, err := db.HasRecentGitHubPrincipal("profile_pg", "renop-pg", changedAt)
	require.NoError(t, err)
	require.True(t, githubAuthorized)
	privateSecurity, err := db.UpdateAccountEmail("profile_pg", "profile.pg@example.com", changedAt)
	require.NoError(t, err)
	require.Equal(t, "profile.pg@example.com", privateSecurity.Email)
	privateSecurity, err = db.SetPasswordLoginEnabled("profile_pg", false, changedAt+1)
	require.NoError(t, err)
	require.False(t, privateSecurity.PasswordLoginEnabled)
	recoveryHashes := make([]core.RecoveryCodeHash, core.RecoveryCodeCount)
	for index := range recoveryHashes {
		recoveryHashes[index] = core.RecoveryCodeHash{
			SelectorHash: fmt.Sprintf("%064x", index+1),
			PasswordHash: fmt.Sprintf("postgres-argon-%d", index+1),
			CreatedAt:    changedAt + 2,
		}
	}
	require.NoError(t, db.ReplaceRecoveryCodes("profile_pg", recoveryHashes))
	privateSecurity, err = db.GetAccountSecurity("profile_pg")
	require.NoError(t, err)
	require.Equal(t, core.RecoveryCodeCount, privateSecurity.RecoveryCodesRemaining)
	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{
		Repository: "cargo", Name: "profile-pg-crate", NormalizedName: "profile-pg-crate",
		CreatedAt: changedAt, UpdatedAt: changedAt,
	}, &core.CargoVersion{
		Repository: "cargo", Package: "profile-pg-crate", Version: "1.0.0",
		Publisher: account.Name, CreatedAt: changedAt,
	}, account.Name))
	require.NoError(t, db.RecordCargoMirrorPublication(&core.CargoPackage{
		Repository: "cargo", Name: "mirror-pg-crate", NormalizedName: "mirror-pg-crate",
		CreatedAt: changedAt, UpdatedAt: changedAt,
	}, &core.CargoVersion{
		Repository: "cargo", Package: "mirror-pg-crate", Version: "1.0.0", Size: 512, CreatedAt: changedAt,
	}))
	mirroredCargo, err := db.GetCargoPackageDetails("cargo", "mirror-pg-crate", "profile_pg")
	require.NoError(t, err)
	require.True(t, mirroredCargo.Package.Mirrored)
	require.Len(t, mirroredCargo.Versions, 1)
	require.True(t, mirroredCargo.Versions[0].Mirrored)
	_, err = db.CreateDockerImage("docker", "profile/pg", account.Name, false, changedAt)
	require.NoError(t, err)
	require.NoError(t, db.PutDockerManifest(&core.DockerManifest{
		Repository: "docker", ImageName: "profile/pg",
		Digest:    "sha256:abc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdee",
		MediaType: "application/vnd.oci.image.manifest.v1+json", RawJSON: []byte(`{"schemaVersion":2}`),
	}, "latest", account.Name))
	profile, err = db.UpdateUserProfile("profile_pg", "profile_pg_one", profile.Nickname, account, changedAt+1,
		core.AccountTokenChanges{})
	require.NoError(t, err)
	require.Equal(t, stableUserID, profile.UserID)
	profile, err = db.UpdateUserProfile("profile_pg_one", "profile_pg_two", profile.Nickname,
		mustToken(t, db, "profile_pg_one"), changedAt+2, core.AccountTokenChanges{})
	require.NoError(t, err)
	require.Equal(t, stableUserID, profile.UserID)
	_, err = db.UpdateUserProfile("profile_pg_two", "profile_pg_three", profile.Nickname,
		mustToken(t, db, "profile_pg_two"), changedAt+3, core.AccountTokenChanges{})
	require.ErrorIs(t, err, core.ErrUsernameChangeRateLimited)
	loaded, err := db.GetUserProfiles([]string{"profile_pg_two", "missing"})
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.Equal(t, "PostgreSQL User", loaded["profile_pg_two"].Nickname)
	byID, err := db.GetUserProfileByID(stableUserID)
	require.NoError(t, err)
	require.Equal(t, "profile_pg_two", byID.Username)
	require.Equal(t, "https://profile.pg.example", byID.Links.Website)
	cargoMemberships, err := db.ListUserPackageMemberships(stableUserID, config.RepositoryFormatCargo)
	require.NoError(t, err)
	require.Len(t, cargoMemberships, 1)
	require.Equal(t, "profile-pg-crate", cargoMemberships[0].Name)
	dockerMemberships, err := db.ListUserPackageMemberships(stableUserID, config.RepositoryFormatDocker)
	require.NoError(t, err)
	require.Len(t, dockerMemberships, 1)
	require.Equal(t, "profile/pg", dockerMemberships[0].Name)
}

func TestPostgresAccountSecuritySerialization(t *testing.T) {
	dsn, _, _ := newPostgresTestSchema(t, "renop_account_security_test")
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "postgres", Dsn: dsn, MaxOpenConns: 4, MaxIdleConns: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "security_pg", EncryptedSecret: "initial-password",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}))
	now := time.Now().UnixMilli()
	require.NoError(t, db.SaveFidoDevice(&core.FidoDevice{
		ID: "security-key", Username: "security_pg", Name: "Passkey",
		CredentialID: []byte("security-credential"), PublicKey: []byte("public"), CreatedAt: now,
	}))

	start := make(chan struct{})
	loginResults := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		_, updateErr := db.SetPasswordLoginEnabled("security_pg", false, now+1)
		loginResults <- updateErr
	}()
	go func() {
		defer workers.Done()
		<-start
		loginResults <- db.DeleteFidoDevice("security_pg", "security-key")
	}()
	close(start)
	workers.Wait()
	close(loginResults)
	loginSuccesses := 0
	for operationErr := range loginResults {
		if operationErr == nil {
			loginSuccesses++
		}
	}
	require.Equal(t, 1, loginSuccesses)
	security, err := db.GetAccountSecurity("security_pg")
	require.NoError(t, err)
	devices, err := db.ListFidoDevices("security_pg")
	require.NoError(t, err)
	require.True(t, security.PasswordLoginEnabled || len(devices) > 0)

	recoveryHashes := make([]core.RecoveryCodeHash, core.RecoveryCodeCount)
	for index := range recoveryHashes {
		recoveryHashes[index] = core.RecoveryCodeHash{
			SelectorHash: fmt.Sprintf("%064x", index+1),
			PasswordHash: fmt.Sprintf("postgres-argon-%d", index+1),
			CreatedAt:    now + 2,
		}
	}
	require.NoError(t, db.ReplaceRecoveryCodes("security_pg", recoveryHashes))
	selectors := []string{
		recoveryHashes[0].SelectorHash, recoveryHashes[1].SelectorHash,
		recoveryHashes[2].SelectorHash, recoveryHashes[3].SelectorHash,
	}
	start = make(chan struct{})
	recoveryResults := make(chan error, 2)
	workers.Add(2)
	for index := range 2 {
		go func() {
			defer workers.Done()
			<-start
			_, resetErr := db.ResetPasswordWithRecoveryCodes(
				"security_pg", selectors, fmt.Sprintf("recovered-password-%d", index), now+int64(index)+3)
			recoveryResults <- resetErr
		}()
	}
	close(start)
	workers.Wait()
	close(recoveryResults)
	recoverySuccesses := 0
	for resetErr := range recoveryResults {
		if resetErr == nil {
			recoverySuccesses++
		}
	}
	require.Equal(t, 1, recoverySuccesses)
	security, err = db.GetAccountSecurity("security_pg")
	require.NoError(t, err)
	require.True(t, security.PasswordLoginEnabled)
	require.Equal(t, core.RecoveryCodeCount-core.RecoveryCodesRequired, security.RecoveryCodesRemaining)
	apiSecret, err := core.GenerateAPITokenSecret()
	require.NoError(t, err)
	apiExpiresAt := now + int64((24*time.Hour)/time.Millisecond)
	apiToken := &core.APIToken{
		ID: uuid.NewString(), Name: "PostgreSQL automation",
		Scopes:    []string{core.APITokenScopeRepositoryRead, core.APITokenScopeRepositoryPublish},
		Targets:   map[string][]string{core.APITokenScopeRepositoryPublish: {"releases"}},
		CreatedAt: now + 10, ExpiresAt: &apiExpiresAt,
	}
	require.NoError(t, db.CreateAPIToken("security_pg", apiToken, core.HashAPITokenSecret(apiSecret)))
	apiCredential, err := db.GetAPITokenByHash(core.HashAPITokenSecret(apiSecret), "security_pg")
	require.NoError(t, err)
	require.NotNil(t, apiCredential)
	require.Equal(t, apiToken.ID, apiCredential.Token.ID)
	require.Equal(t, []string{"releases"}, apiCredential.Token.Targets[core.APITokenScopeRepositoryPublish])
	require.Equal(t, "security_pg", apiCredential.Account.Name)
	require.NoError(t, db.SetAPITokenDisabled("security_pg", apiToken.ID, true))
	apiTokens, err := db.ListAPITokens("security_pg")
	require.NoError(t, err)
	require.Len(t, apiTokens, 1)
	require.True(t, apiTokens[0].Disabled)
	apiCredential, err = db.GetAPITokenByHash(core.HashAPITokenSecret(apiSecret), "security_pg")
	require.NoError(t, err)
	require.Nil(t, apiCredential)
	require.NoError(t, db.SetAPITokenDisabled("security_pg", apiToken.ID, false))
	apiCredential, err = db.GetAPITokenByHash(core.HashAPITokenSecret(apiSecret), "security_pg")
	require.NoError(t, err)
	require.NotNil(t, apiCredential)
	banExpiresAt := now + int64((48*time.Hour)/time.Millisecond)
	banSession := &core.Session{PublicID: "postgres-ban-session", Username: "security_pg", CreatedAt: now}
	banSession.LastActive.Store(now)
	require.NoError(t, db.SaveSession(banSession, "postgres-ban-session-secret"))
	require.NoError(t, db.SetAccountBan("security_pg", &core.AccountBan{
		Reason: "PostgreSQL suspension", CreatedAt: now + 20, ExpiresAt: &banExpiresAt,
	}))
	bannedAccount, err := db.GetTokenByName("security_pg")
	require.NoError(t, err)
	require.NotNil(t, bannedAccount.Ban)
	require.Equal(t, "PostgreSQL suspension", bannedAccount.Ban.Reason)
	apiCredential, err = db.GetAPITokenByHash(core.HashAPITokenSecret(apiSecret), "security_pg")
	require.NoError(t, err)
	require.NotNil(t, apiCredential)
	require.NotNil(t, apiCredential.Account.Ban)
	storedBanSession, err := db.GetSession("postgres-ban-session-secret")
	require.NoError(t, err)
	require.Nil(t, storedBanSession)
	require.NoError(t, db.SetAccountBan("security_pg", nil))
	require.NoError(t, db.DeleteAPIToken("security_pg", apiToken.ID))
}

func TestPostgresMessageDedupeInsert(t *testing.T) {
	dsn, _, _ := newPostgresTestSchema(t, "renop_message_dedupe_test")
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "postgres", Dsn: dsn, MaxOpenConns: 2, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	now := time.Now().UnixMilli()
	message := &core.UserMessage{
		ID: "00000000-0000-4000-8000-000000000020", Recipient: "postgres-user", Kind: "system_update",
		Severity: "info", Title: "Update", Body: "Available", Payload: []byte("{}"),
		CreatedAt: now, DedupeKey: "system-update:available:v2",
	}
	inserted, err := db.SaveMessageIfAbsent(message)
	require.NoError(t, err)
	require.True(t, inserted)
	duplicate := *message
	duplicate.ID = "00000000-0000-4000-8000-000000000021"
	inserted, err = db.SaveMessageIfAbsent(&duplicate)
	require.NoError(t, err)
	require.False(t, inserted)
}

func TestPostgresMavenDomainsMigrateToGlobalOwnership(t *testing.T) {
	dsn, _, _ := newPostgresTestSchema(t, "renop_maven_global_test")
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "postgres", Dsn: dsn, MaxOpenConns: 2, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	for _, username := range []string{"maven_alice", "maven_bob"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: username, CreatedAt: time.Now().UTC().Format(time.RFC3339)}))
	}
	alice, err := db.GetUserProfile("maven_alice")
	require.NoError(t, err)
	bob, err := db.GetUserProfile("maven_bob")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO maven_domains
		(repository, domain, verification_type, verification_host, verification_code, verified, created_at, verified_at, last_check_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"releases", "com.example", "dns", "example.com", "pending", 0, 100, 0, 100,
		"snapshots", "com.example", "dns", "example.com", "verified", 1, 200, 210, 220)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO maven_domain_members
		(repository, domain, username, user_id, permission_level, added_at)
		VALUES (?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?)`,
		"releases", "com.example", "maven_alice", alice.UserID, core.MavenPermissionOwner, 100,
		"snapshots", "com.example", "maven_bob", bob.UserID, core.MavenPermissionOwner, 50)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	db, err = database.InitDB(config.DatabaseConfig{
		Driver: "postgres", Dsn: dsn, MaxOpenConns: 2, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	details, err := db.GetMavenDomainDetails("com.example", "maven_bob")
	require.NoError(t, err)
	require.True(t, details.Domain.Verified)
	require.Equal(t, "verified", details.Domain.VerificationCode)
	levels := make(map[string]int)
	for _, member := range details.Members {
		levels[member.Username] = member.Level
	}
	require.Equal(t, core.MavenPermissionOwner, levels["maven_bob"])
	require.Equal(t, core.MavenPermissionManage, levels["maven_alice"])
	const publishedAt int64 = 1_800_000_200_000
	require.NoError(t, db.RecordMavenPublication(&core.MavenArtifact{
		Repository: "releases", Domain: "com.example", GroupID: "com.example",
		ArtifactID: "postgres-demo", Publisher: "maven_bob", LatestVersion: "1.0.0",
		CreatedAt: publishedAt, UpdatedAt: publishedAt,
	}, &core.MavenVersion{
		Repository: "releases", GroupID: "com.example", ArtifactID: "postgres-demo",
		Version: "1.0.0", Publisher: "maven_bob", Size: 2048, CreatedAt: publishedAt,
	}))
	require.NoError(t, db.RecordMavenMirrorPublication(&core.MavenArtifact{
		Repository: "releases", Domain: "com.example", GroupID: "com.example",
		ArtifactID: "postgres-demo", LatestVersion: "2.0.0", CreatedAt: publishedAt, UpdatedAt: publishedAt + 1,
	}, &core.MavenVersion{
		Repository: "releases", GroupID: "com.example", ArtifactID: "postgres-demo",
		Version: "2.0.0", Size: 4096, CreatedAt: publishedAt + 1,
	}))
	repositoryDomains, err := db.ListMavenRepositoryDomains("releases", "maven_bob")
	require.NoError(t, err)
	require.Len(t, repositoryDomains, 1)
	require.Equal(t, "com.example", repositoryDomains[0].Domain)
	require.Equal(t, 1, repositoryDomains[0].ArtifactCount)
	require.Equal(t, 1, repositoryDomains[0].RepositoryCount)
	require.Equal(t, 2, repositoryDomains[0].MemberCount)
	artifactDetails, err := db.GetMavenArtifactDetails("releases", "com.example", "postgres-demo")
	require.NoError(t, err)
	require.Equal(t, int64(6144), artifactDetails.Artifact.TotalSize)
	require.True(t, artifactDetails.Artifact.Mirrored)
	require.Len(t, artifactDetails.Versions, 2)
	require.True(t, artifactDetails.Versions[0].Mirrored)
	searchedDomains, searchedTotal, err := db.SearchMavenRepositoryDomains("releases", "example", 10)
	require.NoError(t, err)
	require.Equal(t, 1, searchedTotal)
	require.Len(t, searchedDomains, 1)
	require.Equal(t, "com.example", searchedDomains[0].Domain)
	managedDomains, managedTotal, err := db.ListManagedMavenDomains(core.MavenDomainListOptions{
		Username: "maven_bob", PermissionLevels: []int{core.MavenPermissionOwner},
		Filtered: true, Limit: 1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, managedTotal)
	require.Len(t, managedDomains, 1)
	require.Equal(t, "com.example", managedDomains[0].Domain)
	require.NoError(t, db.EnsureMirroredMavenDomain("org.postgres", publishedAt))
	mirroredDomains, mirroredTotal, err := db.ListManagedMavenDomains(core.MavenDomainListOptions{
		Username: "maven_bob", Administrator: true, IncludeMirrored: true, Filtered: true, Limit: 20,
	})
	require.NoError(t, err)
	require.Equal(t, 1, mirroredTotal)
	require.Len(t, mirroredDomains, 1)
	require.Equal(t, core.MavenVerificationMirror, mirroredDomains[0].VerificationType)
	require.NoError(t, db.RemoveMavenMember("com.example", "maven_bob", "maven_alice"))
	requireTeamRemovalMessage(t, db, "maven_alice", "maven", "", "com.example", "maven_bob")
	require.NoError(t, db.DeleteMavenRepository("releases"))
	_, err = db.GetMavenArtifactDetails("releases", "com.example", "postgres-demo")
	require.ErrorIs(t, err, core.ErrMavenArtifactNotFound)
	domainAfterDelete, err := db.GetMavenDomainDetails("com.example", "maven_bob")
	require.NoError(t, err)
	require.True(t, domainAfterDelete.Domain.Verified)
}

func TestPostgresDriverContract(t *testing.T) {
	dsn, _, _ := newPostgresTestSchema(t, "renop_driver_contract")
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "postgres", Dsn: dsn, MaxOpenConns: 4, MaxIdleConns: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	results, err := database.RunDriverCheck(context.Background(), db)
	require.NoError(t, err)
	require.Len(t, results, 9)
}
