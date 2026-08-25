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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func TestMavenDomainsMigrateToGlobalOwnership(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "global-maven.db")
	databaseConfig := config.DatabaseConfig{Driver: "sqlite", Dsn: databasePath, MaxOpenConns: 1, MaxIdleConns: 1}
	db, err := database.InitDB(databaseConfig)
	require.NoError(t, err)
	for _, username := range []string{"alice", "bob", "charlie"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: username, CreatedAt: time.Now().UTC().Format(time.RFC3339)}))
	}
	alice, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	bob, err := db.GetUserProfile("bob")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO maven_domains
		(repository, domain, verification_type, verification_host, verification_code, verified, created_at, verified_at, last_check_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"releases", "com.example", "dns", "example.com", "old-code", 0, 100, 0, 110,
		"snapshots", "com.example", "dns", "example.com", "verified-code", 1, 200, 210, 220)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO maven_domain_members
		(repository, domain, username, user_id, permission_level, added_at)
		VALUES (?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?)`,
		"releases", "com.example", "alice", alice.UserID, core.MavenPermissionOwner, 100,
		"snapshots", "com.example", "bob", bob.UserID, core.MavenPermissionOwner, 50)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO maven_domain_invitations
		(id, repository, domain, inviter, recipient, permission_level, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?)`,
		"invite-old", "releases", "com.example", "alice", "charlie", core.MavenPermissionRead, 100,
		"invite-new", "snapshots", "com.example", "bob", "charlie", core.MavenPermissionPublish, 200)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	db, err = database.InitDB(databaseConfig)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var legacyDomains int
	require.NoError(t, db.SQLDB.QueryRow(`SELECT COUNT(*) FROM maven_domains WHERE repository <> ''`).Scan(&legacyDomains))
	assert.Zero(t, legacyDomains)
	details, err := db.GetMavenDomainDetails("com.example", "bob")
	require.NoError(t, err)
	assert.True(t, details.Domain.Verified)
	assert.Equal(t, "verified-code", details.Domain.VerificationCode)
	assert.Equal(t, int64(100), details.Domain.CreatedAt)
	levels := make(map[string]int)
	for _, member := range details.Members {
		levels[member.Username] = member.Level
	}
	assert.Equal(t, core.MavenPermissionOwner, levels["bob"])
	assert.Equal(t, core.MavenPermissionManage, levels["alice"])
	var invitationID string
	require.NoError(t, db.SQLDB.QueryRow(`SELECT id FROM maven_domain_invitations
		WHERE repository = '' AND domain = ? AND recipient = ?`, "com.example", "charlie").Scan(&invitationID))
	assert.Equal(t, "invite-new", invitationID)
}
