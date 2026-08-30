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
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
)

func testRecoveryHashes(createdAt int64) []core.RecoveryCodeHash {
	hashes := make([]core.RecoveryCodeHash, core.RecoveryCodeCount)
	for index := range hashes {
		hashes[index] = core.RecoveryCodeHash{
			SelectorHash: fmt.Sprintf("%064x", index+1),
			PasswordHash: fmt.Sprintf("argon-hash-%d", index+1),
			CreatedAt:    createdAt,
		}
	}
	return hashes
}

func TestPasswordLoginPolicyAcceptsExistingLegacyShortUsername(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "legacy-username.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "r3", EncryptedSecret: "legacy-password-hash", CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}))

	enabled, err := db.PasswordLoginEnabled("r3")
	require.NoError(t, err)
	assert.True(t, enabled)
	_, validReplacement := core.NormalizeUsername("r3")
	assert.False(t, validReplacement, "legacy login compatibility must not relax replacement username rules")
}

func TestPrivateAccountSecurityAndRecoveryLifecycle(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "account-security.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, account := range []*core.AccessToken{
		{Name: "alice", EncryptedSecret: "alice-password", CreatedAt: time.Now().UTC().Format(time.RFC3339)},
		{Name: "bobby", EncryptedSecret: "bob-password", CreatedAt: time.Now().UTC().Format(time.RFC3339)},
	} {
		require.NoError(t, db.SaveToken(account))
	}
	now := time.Now().UnixMilli()

	security, err := db.GetAccountSecurity("alice")
	require.NoError(t, err)
	assert.True(t, security.PasswordLoginEnabled)
	assert.True(t, security.PasswordConfigured)
	assert.Empty(t, security.Email)
	assert.False(t, security.CanDisablePasswordLogin)
	require.ErrorIs(t, func() error {
		_, updateErr := db.SetPasswordLoginEnabled("alice", false, now)
		return updateErr
	}(), core.ErrLastLoginMethod)

	security, err = db.UpdateAccountEmail("alice", "Alice@Exämple.COM", now)
	require.NoError(t, err)
	assert.Equal(t, "alice@xn--exmple-cua.com", security.Email)
	emailToken, err := db.GetTokenByEmail("ALICE@XN--EXMPLE-CUA.COM")
	require.NoError(t, err)
	require.NotNil(t, emailToken)
	assert.Equal(t, "alice", emailToken.Name)
	_, err = db.UpdateAccountEmail("bobby", security.Email, now)
	require.ErrorIs(t, err, core.ErrEmailAlreadyExists)

	require.NoError(t, db.SaveFidoDevice(&core.FidoDevice{
		ID: "alice-key", Username: "alice", Name: "Key", CredentialID: []byte("credential"),
		PublicKey: []byte("public"), CreatedAt: now,
	}))
	security, err = db.SetPasswordLoginEnabled("alice", false, now+1)
	require.NoError(t, err)
	assert.False(t, security.PasswordLoginEnabled)
	require.ErrorIs(t, db.DeleteFidoDevice("alice", "alice-key"), core.ErrLastLoginMethod)
	require.NoError(t, db.SetAccountPassword("alice", "replacement-password", now+2))
	require.NoError(t, db.DeleteFidoDevice("alice", "alice-key"))

	hashes := testRecoveryHashes(now + 3)
	require.NoError(t, db.ReplaceRecoveryCodes("alice", hashes))
	security, err = db.GetAccountSecurity("alice")
	require.NoError(t, err)
	assert.Equal(t, core.RecoveryCodeCount, security.RecoveryCodeCount)
	assert.Equal(t, core.RecoveryCodeCount, security.RecoveryCodesRemaining)
	selectors := []string{
		hashes[0].SelectorHash, hashes[1].SelectorHash, hashes[2].SelectorHash, hashes[3].SelectorHash,
	}
	resolvedUsername, records, err := db.GetRecoveryCodes("alice@xn--exmple-cua.com", selectors)
	require.NoError(t, err)
	assert.Equal(t, "alice", resolvedUsername)
	require.Len(t, records, core.RecoveryCodesRequired)

	session := &core.Session{
		PublicID: "before-recovery", Username: "alice", CreatedAt: now, LoginMethod: "password",
	}
	session.LastActive.Store(now)
	require.NoError(t, db.SaveSession(session, "secret-before-recovery"))
	recoveredUsername, err := db.ResetPasswordWithRecoveryCodes(
		"alice@xn--exmple-cua.com", selectors, "recovered-password", now+4)
	require.NoError(t, err)
	assert.Equal(t, "alice", recoveredUsername)
	require.ErrorIs(t, func() error {
		_, reuseErr := db.ResetPasswordWithRecoveryCodes("alice", selectors, "reuse", now+5)
		return reuseErr
	}(), core.ErrRecoveryCodesInvalid)
	security, err = db.GetAccountSecurity("alice")
	require.NoError(t, err)
	assert.True(t, security.PasswordLoginEnabled)
	assert.Equal(t, core.RecoveryCodeCount-core.RecoveryCodesRequired, security.RecoveryCodesRemaining)
	storedToken, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	assert.Equal(t, "recovered-password", storedToken.EncryptedSecret)
	storedSession, err := db.GetSession("secret-before-recovery")
	require.NoError(t, err)
	assert.Nil(t, storedSession)
}

func TestRecoveryCodesCanOnlyWinOneConcurrentReset(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "recovery-race.db"),
		MaxOpenConns: 4, MaxIdleConns: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "alice", EncryptedSecret: "initial-password",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}))
	now := time.Now().UnixMilli()
	hashes := testRecoveryHashes(now)
	require.NoError(t, db.ReplaceRecoveryCodes("alice", hashes))
	selectors := []string{
		hashes[0].SelectorHash, hashes[1].SelectorHash, hashes[2].SelectorHash, hashes[3].SelectorHash,
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for index := range 2 {
		workers.Go(func() {
			<-start
			_, resetErr := db.ResetPasswordWithRecoveryCodes(
				"alice", selectors, fmt.Sprintf("concurrent-password-%d", index), now+int64(index)+1)
			results <- resetErr
		})
	}
	close(start)
	workers.Wait()
	close(results)
	successes := 0
	operationErrors := make([]error, 0, 2)
	for resetErr := range results {
		if resetErr == nil {
			successes++
		} else {
			operationErrors = append(operationErrors, resetErr)
		}
	}
	assert.Equal(t, 1, successes, "concurrent reset errors: %v", operationErrors)
	security, err := db.GetAccountSecurity("alice")
	require.NoError(t, err)
	assert.Equal(t, core.RecoveryCodeCount-core.RecoveryCodesRequired, security.RecoveryCodesRemaining)
}

func TestConcurrentLoginMethodChangesCannotLockAccount(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "login-method-race.db"),
		MaxOpenConns: 4, MaxIdleConns: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "alice", EncryptedSecret: "initial-password",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}))
	now := time.Now().UnixMilli()
	require.NoError(t, db.SaveFidoDevice(&core.FidoDevice{
		ID: "alice-key", Username: "alice", Name: "Key", CredentialID: []byte("credential"),
		PublicKey: []byte("public"), CreatedAt: now,
	}))

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		_, updateErr := db.SetPasswordLoginEnabled("alice", false, now+1)
		results <- updateErr
	}()
	go func() {
		defer workers.Done()
		<-start
		results <- db.DeleteFidoDevice("alice", "alice-key")
	}()
	close(start)
	workers.Wait()
	close(results)
	successes := 0
	for operationErr := range results {
		if operationErr == nil {
			successes++
		}
	}
	assert.Equal(t, 1, successes)
	security, err := db.GetAccountSecurity("alice")
	require.NoError(t, err)
	devices, err := db.ListFidoDevices("alice")
	require.NoError(t, err)
	assert.True(t, security.PasswordLoginEnabled || len(devices) > 0)
}

func TestTokenMutationsPreserveConcurrentCredentialChanges(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "credential-mutations.db"),
		MaxOpenConns: 4, MaxIdleConns: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "alice", EncryptedSecret: "initial-password", Tokens: []string{"initial-token"},
		CreatedAt: time.Now().UTC().Format(time.RFC3339), Permissions: []string{"base"},
	}))
	staleToken, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	require.NotNil(t, staleToken)
	staleCopy := *staleToken

	mutationStarted := make(chan struct{})
	releaseMutation := make(chan struct{})
	updateResult := make(chan error, 1)
	go func() {
		updateResult <- db.UpdateToken("alice", func(token *core.AccessToken) {
			close(mutationStarted)
			<-releaseMutation
			token.Tokens = []string{"rotated-token"}
		})
	}()
	<-mutationStarted
	passwordResult := make(chan error, 1)
	go func() {
		passwordResult <- db.SetAccountPassword("alice", "concurrent-password", time.Now().UnixMilli())
	}()
	close(releaseMutation)
	require.NoError(t, <-updateResult)
	require.NoError(t, <-passwordResult)

	staleCopy.Permissions = []string{"base", "package-manager"}
	_, err = db.UpdateUserProfile("alice", "alice", "Alice", &staleCopy,
		time.Now().UnixMilli(), core.AccountTokenChanges{Permissions: true})
	require.NoError(t, err)
	stored, err := db.GetTokenByName("alice")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "concurrent-password", stored.EncryptedSecret)
	assert.Empty(t, stored.Tokens)
	assert.Equal(t, []string{"base", "package-manager"}, stored.Permissions)
	rotatedOwner, err := db.GetTokenBySecret("rotated-token")
	require.NoError(t, err)
	require.NotNil(t, rotatedOwner)
	assert.Equal(t, "alice", rotatedOwner.Name)
}
