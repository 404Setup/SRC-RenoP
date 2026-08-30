/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func newClickHouseTestDatabase(t *testing.T) *database.DB {
	t.Helper()
	baseDSN := os.Getenv("RENOP_TEST_CLICKHOUSE_DSN")
	if baseDSN == "" {
		t.Skip("RENOP_TEST_CLICKHOUSE_DSN is not configured")
	}
	parsed, err := url.Parse(baseDSN)
	require.NoError(t, err)
	adminOptions, err := clickhouse.ParseDSN(baseDSN)
	require.NoError(t, err)
	adminOptions.Auth.Database = "default"
	admin, err := clickhouse.Open(adminOptions)
	require.NoError(t, err)
	require.NoError(t, admin.Ping(context.Background()))
	databaseName := fmt.Sprintf("renop_test_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec(context.Background(), "CREATE DATABASE `"+databaseName+"`"))
	t.Cleanup(func() {
		require.NoError(t, admin.Exec(context.Background(), "DROP DATABASE IF EXISTS `"+databaseName+"`"))
		require.NoError(t, admin.Close())
	})
	parsed.Path = "/" + databaseName
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "clickhouse", Dsn: parsed.String(), MaxOpenConns: 4, MaxIdleConns: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func TestClickHouseNativeAccountMessageAndTransactionLifecycle(t *testing.T) {
	db := newClickHouseTestDatabase(t)
	require.Equal(t, "clickhouse", db.Dialect.Name())
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "alice", CreatedAt: "2026-08-28T00:00:00Z", Permissions: []string{"base", "canupdate:cargo"},
	}))
	account, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Equal(t, []string{"base", "canupdate:cargo"}, account.Permissions)

	session := &core.Session{PublicID: "clickhouse-session", Username: "alice", IP: "127.0.0.1", CreatedAt: 1000, LoginMethod: "password"}
	session.LastActive.Store(1000)
	require.NoError(t, db.SaveSession(session, "session-secret"))
	storedSession, err := db.GetSession("session-secret")
	require.NoError(t, err)
	require.NotNil(t, storedSession)
	assert.Equal(t, "alice", storedSession.Username)

	message := &core.UserMessage{
		ID: uuid.NewString(), Recipient: "alice", Sender: "system", Kind: "test", Severity: "info",
		Title: "ClickHouse", Body: "Native message", DedupeKey: "clickhouse-native-message", CreatedAt: 1000,
	}
	inserted, err := db.SaveMessageIfAbsent(message)
	require.NoError(t, err)
	assert.True(t, inserted)
	inserted, err = db.SaveMessageIfAbsent(message)
	require.NoError(t, err)
	assert.False(t, inserted)

	tx, err := db.Begin()
	require.NoError(t, err)
	_, err = tx.Exec(`INSERT INTO user_profiles
		(user_id, username, nickname, rename_window_started_at, rename_count, updated_at)
		VALUES (?, ?, '', 0, 0, 0)`, uuid.NewString(), "rollback-user")
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	_, err = db.GetUserProfile("rollback-user")
	assert.True(t, errors.Is(err, core.ErrUserProfileNotFound))

	tx, err = db.Begin()
	require.NoError(t, err)
	_, err = tx.Exec(`UPDATE tokens SET description = ? WHERE name = ?`, "rolled back", "alice")
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	var description string
	require.NoError(t, db.QueryRow(`SELECT description FROM tokens WHERE name = ?`, "alice").Scan(&description))
	require.NotEqual(t, "rolled back", description)
}

func TestClickHouseNativePackageAndStatisticsMatrix(t *testing.T) {
	db := newClickHouseTestDatabase(t)
	now := time.Now().UnixMilli()
	for _, username := range []string{"alice", "bob"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{
			Name: username, CreatedAt: "2026-08-28T00:00:00Z", Permissions: []string{"base"},
		}))
	}

	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{
		Repository: "cargo", Name: "demo", NormalizedName: "demo", Description: "Demo crate",
		Readme: "# Cargo", CreatedAt: now, UpdatedAt: now,
	}, &core.CargoVersion{
		Repository: "cargo", Package: "demo", Version: "1.0.0", Publisher: "alice", Size: 128, CreatedAt: now,
	}, "alice"))
	cargoDetails, err := db.GetCargoPackageDetails("cargo", "demo", "alice")
	require.NoError(t, err)
	require.Equal(t, "# Cargo", cargoDetails.Package.Readme)
	require.NoError(t, db.SetCargoVersionYanked("cargo", "demo", "1.0.0", true, false))

	image, err := db.CreateDockerImage("containers", "team/demo", "alice", false, now)
	require.NoError(t, err)
	require.Equal(t, core.DockerPermissionOwner, image.PermissionLevel)
	manifest := &core.DockerManifest{
		Repository: "containers", ImageName: "team/demo",
		Digest:    "sha256:abc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		MediaType: "application/vnd.oci.image.manifest.v1+json", Size: 256,
		ConfigDigest: "sha256:def1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		RawJSON:      []byte(`{"schemaVersion":2}`),
	}
	require.NoError(t, db.PutDockerManifest(manifest, "latest", "alice"))
	dockerDetails, err := db.GetDockerImageDetails("containers", "team/demo", "alice")
	require.NoError(t, err)
	require.Len(t, dockerDetails.Tags, 1)

	domain := &core.MavenDomain{
		Domain: "com.example", VerificationType: core.MavenVerificationDNS, VerificationHost: "example.com",
		VerificationCode: "renop-clickhouse", CreatedAt: now,
	}
	require.NoError(t, db.CreateMavenDomain(domain, "alice"))
	require.NoError(t, db.MarkMavenDomainVerified(domain.Domain, domain.VerificationCode, now))
	require.NoError(t, db.RecordMavenPublication(&core.MavenArtifact{
		Repository: "maven", Domain: domain.Domain, GroupID: "com.example", ArtifactID: "demo",
		Description: "Demo artifact", Readme: "# Maven", CreatedAt: now, UpdatedAt: now,
	}, &core.MavenVersion{
		Repository: "maven", GroupID: "com.example", ArtifactID: "demo", Version: "1.0.0",
		Publisher: "alice", Size: 512, CreatedAt: now,
	}))
	mavenDetails, err := db.GetMavenArtifactDetails("maven", "com.example", "demo")
	require.NoError(t, err)
	require.Equal(t, "# Maven", mavenDetails.Artifact.Readme)

	_, err = db.CreateNPMPackage("npm", "@team/demo", "alice", true, now)
	require.NoError(t, err)
	require.NoError(t, db.RecordNPMPublication(&core.NPMPackage{
		Repository: "npm", Name: "@team/demo", Description: "Demo npm package", UpdatedAt: now + 1,
	}, &core.NPMVersion{
		Repository: "npm", Package: "@team/demo", Version: "1.0.0",
		ManifestJSON: `{"name":"@team/demo","version":"1.0.0"}`, Publisher: "alice",
		TarballPath: "@team/demo/-/demo-1.0.0.tgz", Shasum: "0123456789012345678901234567890123456789",
		Integrity: "sha512-ZGVtbw==", Size: 64, CreatedAt: now + 1,
	}, map[string]string{"latest": "1.0.0"}, "alice"))
	npmDetails, err := db.GetNPMPackageDetails("npm", "@team/demo", "alice")
	require.NoError(t, err)
	require.Equal(t, "1.0.0", npmDetails.Package.LatestVersion)
	_, err = db.Exec(`UPDATE npm_packages SET latest_version = '' WHERE repository = ? AND package_name = ?`,
		"npm", "@team/demo")
	require.NoError(t, err)
	npmPackages, npmTotal, err := db.ListNPMPackages("npm", "alice", false, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, npmTotal)
	require.Len(t, npmPackages, 1)
	require.Equal(t, "1.0.0", npmPackages[0].LatestVersion)

	require.NoError(t, db.BatchIncrementDownloadStatistics([]*core.DownloadStatisticDelta{
		{Username: "alice", Repository: "maven", Format: config.RepositoryFormatMaven,
			Namespace: "com.example", Package: "com.example:demo", Version: "1.0.0", Count: 2, Bytes: 1024, UpdatedAt: now},
		{Username: "alice", Repository: "containers", Format: config.RepositoryFormatDocker,
			Package: "team/demo", Version: "latest", Count: 1, Bytes: 256, UpdatedAt: now},
	}))
	page, err := db.QueryDownloadStatistics(core.DownloadStatisticsQuery{GroupBy: "repository", Limit: 20})
	require.NoError(t, err)
	require.Equal(t, int64(3), page.Count)
	require.Len(t, page.Records, 2)
}

func TestClickHouseNativeSecurityIdentityAndGPGMatrix(t *testing.T) {
	db := newClickHouseTestDatabase(t)
	now := time.Now().UnixMilli()
	for _, username := range []string{"alice", "bobby"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{
			Name: username, EncryptedSecret: username + "-password", CreatedAt: "2026-08-28T00:00:00Z",
			Permissions: []string{"base"},
		}))
	}
	security, err := db.UpdateAccountEmail("alice", "alice@example.com", now)
	require.NoError(t, err)
	require.Equal(t, "alice@example.com", security.Email)
	require.NoError(t, db.SaveFidoDevice(&core.FidoDevice{
		ID: "alice-passkey", Username: "alice", Name: "Passkey", CredentialID: []byte("credential"),
		PublicKey: []byte("public-key"), AAGUID: []byte("aaguid"), CreatedAt: now,
	}))
	devices, err := db.ListFidoDevices("alice")
	require.NoError(t, err)
	require.Len(t, devices, 1)
	require.Equal(t, []byte("credential"), devices[0].CredentialID)

	hashes := make([]core.RecoveryCodeHash, core.RecoveryCodeCount)
	for index := range hashes {
		hashes[index] = core.RecoveryCodeHash{
			SelectorHash: fmt.Sprintf("%064x", index+1), PasswordHash: fmt.Sprintf("hash-%d", index+1), CreatedAt: now,
		}
	}
	require.NoError(t, db.ReplaceRecoveryCodes("alice", hashes))
	security, err = db.GetAccountSecurity("alice")
	require.NoError(t, err)
	require.Equal(t, core.RecoveryCodeCount, security.RecoveryCodesRemaining)

	profile, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	require.NoError(t, db.StoreGitHubIdentity(profile.UserID, 101, "alice-gh", []core.GitHubPrincipal{
		{Type: core.GitHubPrincipalUser, GitHubID: 101, Login: "alice-gh"},
		{Type: core.GitHubPrincipalOrganization, GitHubID: 202, Login: "example-org"},
	}, now))
	identity, err := db.GetGitHubIdentity("alice")
	require.NoError(t, err)
	require.NotNil(t, identity)
	require.Equal(t, 2, identity.PrincipalCount)

	const tokenSecret = "native-clickhouse-api-token"
	digest := sha256.Sum256([]byte(tokenSecret))
	apiToken := &core.APIToken{
		ID: uuid.NewString(), Name: "Automation", Scopes: []string{core.APITokenScopeRepositoryRead}, CreatedAt: now,
	}
	require.NoError(t, db.CreateAPIToken("alice", apiToken, hex.EncodeToString(digest[:])))
	credential, err := db.GetAPITokenByHash(hex.EncodeToString(digest[:]), "alice")
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.Equal(t, apiToken.ID, credential.Token.ID)

	const fingerprint = "1462C0512352DEC38A39D0793586B4EB0FDA2EA9"
	const keyID = "3586B4EB0FDA2EA9"
	require.NoError(t, db.RegisterUserGPGKey("alice", fingerprint, &core.GPGPublicKey{
		Fingerprint: fingerprint, KeyID: keyID, PublicKey: []byte("armored-key"), FetchedAt: now,
	}, []string{fingerprint, keyID}))
	keys, err := db.FindGPGPublicKeys(keyID)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, []byte("armored-key"), keys[0].PublicKey)
}

func TestClickHouseNativeUsernameRename(t *testing.T) {
	db := newClickHouseTestDatabase(t)
	now := time.Now().UnixMilli()
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "alice", EncryptedSecret: "alice-password", CreatedAt: "2026-08-28T00:00:00Z",
		Permissions: []string{"base"},
	}))
	profile, err := db.GetUserProfile("alice")
	require.NoError(t, err)

	memberships := []struct {
		table          string
		resourceColumn string
		resource       string
	}{
		{table: "cargo_members", resourceColumn: "normalized_name", resource: "rename-crate"},
		{table: "docker_members", resourceColumn: "image_name", resource: "rename/image"},
		{table: "npm_members", resourceColumn: "package_name", resource: "@rename/package"},
		{table: "maven_domain_members", resourceColumn: "domain", resource: "com.rename"},
	}
	for _, membership := range memberships {
		_, err := db.Exec(fmt.Sprintf(`INSERT INTO %s
			(repository, %s, username, user_id, permission_level, added_at) VALUES (?, ?, ?, ?, ?, ?)`,
			membership.table, membership.resourceColumn),
			"rename", membership.resource, "alice", profile.UserID, 4, now)
		require.NoError(t, err)
	}
	const fingerprint = "2462C0512352DEC38A39D0793586B4EB0FDA2EA8"
	require.NoError(t, db.RegisterUserGPGKey("alice", fingerprint, &core.GPGPublicKey{
		Fingerprint: fingerprint, KeyID: fingerprint[len(fingerprint)-16:], PublicKey: []byte("rename-key"), FetchedAt: now,
	}, []string{fingerprint}))

	account, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	require.NoError(t, db.RenameToken("alice", "renamed", account))

	renamedProfile, err := db.GetUserProfile("renamed")
	require.NoError(t, err)
	require.Equal(t, profile.UserID, renamedProfile.UserID)
	for _, membership := range memberships {
		var username string
		err := db.QueryRow(fmt.Sprintf(`SELECT username FROM %s WHERE repository = ? AND %s = ? AND user_id = ?`,
			membership.table, membership.resourceColumn), "rename", membership.resource, profile.UserID).Scan(&username)
		require.NoError(t, err)
		require.Equal(t, "renamed", username)
	}
	var gpgOwner string
	require.NoError(t, db.QueryRow(`SELECT username FROM user_gpg_keys WHERE fingerprint = ?`, fingerprint).Scan(&gpgOwner))
	require.Equal(t, "renamed", gpgOwner)
}

func TestClickHouseNativeDriverContract(t *testing.T) {
	db := newClickHouseTestDatabase(t)
	results, err := database.RunDriverCheck(context.Background(), db)
	require.NoError(t, err)
	require.Len(t, results, 7)
}

func TestClickHouseNativeSchemaCopyMigrationPreservesRows(t *testing.T) {
	baseDSN := os.Getenv("RENOP_TEST_CLICKHOUSE_DSN")
	if baseDSN == "" {
		t.Skip("RENOP_TEST_CLICKHOUSE_DSN is not configured")
	}
	parsed, err := url.Parse(baseDSN)
	require.NoError(t, err)
	adminOptions, err := clickhouse.ParseDSN(baseDSN)
	require.NoError(t, err)
	adminOptions.Auth.Database = "default"
	admin, err := clickhouse.Open(adminOptions)
	require.NoError(t, err)
	databaseName := fmt.Sprintf("renop_schema_migration_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec(context.Background(), "CREATE DATABASE `"+databaseName+"`"))
	t.Cleanup(func() {
		require.NoError(t, admin.Exec(context.Background(), "DROP DATABASE IF EXISTS `"+databaseName+"`"))
		require.NoError(t, admin.Close())
	})
	legacyTable := "`" + databaseName + "`.`docker_images`"
	require.NoError(t, admin.Exec(context.Background(), `CREATE TABLE `+legacyTable+` (
		repository String, image_name String, description String DEFAULT '', publisher String DEFAULT '',
		pull_count Int64 DEFAULT 0, private Int64 DEFAULT 0, push_enabled Int64 DEFAULT 1,
		created_at Int64, updated_at Int64,
		_renop_key String MATERIALIZED concat(repository, '/', image_name)
	) ENGINE = EmbeddedRocksDB PRIMARY KEY _renop_key SETTINGS optimize_for_bulk_insert = 0`))
	require.NoError(t, admin.Exec(context.Background(), `INSERT INTO `+legacyTable+`
		(repository, image_name, description, publisher, created_at, updated_at)
		VALUES ('containers', 'legacy', 'preserved', 'alice', 100, 100)`))
	parsed.Path = "/" + databaseName
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "clickhouse", Dsn: parsed.String(), MaxOpenConns: 4, MaxIdleConns: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var description, binding string
	require.NoError(t, db.QueryRow(`SELECT description, super_team_prefix FROM docker_images
		WHERE repository = ? AND image_name = ?`, "containers", "legacy").Scan(&description, &binding))
	assert.Equal(t, "preserved", description)
	assert.Empty(t, binding)
	var migrationTables int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM system.tables
		WHERE database = ? AND startsWith(name, '_renop_schema_')`, databaseName).Scan(&migrationTables))
	assert.Zero(t, migrationTables)
}
