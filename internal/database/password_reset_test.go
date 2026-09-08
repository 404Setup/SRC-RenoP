/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/mail"
	"renop/internal/testutil"
)

func TestEmailPasswordResetLifecycle(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "email-reset.db"), MaxOpenConns: 4, MaxIdleConns: 2})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	cfg := mail.DefaultConfig()
	require.NoError(t, cfg.EnsureKey())
	cfg.ManualRate.Limit = 100
	now := time.Now().UnixMilli()
	hash := strings.Repeat("a", 64)
	wrong := strings.Repeat("b", 64)
	jobID := 0
	job := func(email string, at int64) *mail.Job {
		jobID++
		id := fmt.Sprintf("password-reset-%d", jobID)
		return &mail.Job{ID: id, AccountID: "test", Scene: "password_reset", TicketHash: hash,
			CreatedAt: at, ExpiresAt: at + 600000,
			Message: mail.Message{ID: id, To: email, Subject: "Reset", Text: "Verification code", CreatedAt: at}}
	}
	account := func(t *testing.T, name string) string {
		t.Helper()
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: name, EncryptedSecret: "old-password", Permissions: []string{"base"}, Tokens: []string{"api-" + name}}))
		email := name + "@example.com"
		_, err := db.UpdateAccountEmail(name, email, now)
		require.NoError(t, err)
		return email
	}
	queue := func(t *testing.T, email string, at int64) {
		t.Helper()
		created, err := db.QueueEmailPasswordReset(job(email, at), hash, cfg.EncryptionKey, "192.0.2.1", cfg.ManualRate)
		require.NoError(t, err)
		require.True(t, created)
	}
	resetInvalid := func(t *testing.T, email string, at int64) {
		t.Helper()
		_, err := db.ResetPasswordWithEmailCode(email, hash, "new-password", at)
		require.ErrorIs(t, err, core.ErrEmailCodeInvalid)
	}
	t.Run("concurrent consumption revokes sessions and preserves other credentials", func(t *testing.T) {
		email := account(t, "alice")
		require.NoError(t, db.ReplaceRecoveryCodes("alice", testRecoveryHashes(now)))
		require.NoError(t, db.SaveFidoDevice(&core.FidoDevice{ID: "passkey", Username: "alice", Name: "Key", CredentialID: []byte("id"), PublicKey: []byte("key"), CreatedAt: now}))
		_, err := db.SetPasswordLoginEnabled("alice", false, now+1)
		require.NoError(t, err)
		session := &core.Session{Username: "alice", PublicID: "old-session", CreatedAt: now, LoginMethod: "fido"}
		session.LastActive.Store(now)
		require.NoError(t, db.SaveSession(session, "old-secret"))
		_, err = db.GetSession("old-secret")
		require.NoError(t, err)
		_, err = db.GetTokenByName("alice")
		require.NoError(t, err)
		queue(t, email, now+2)
		results := make(chan error, 2)
		var workers sync.WaitGroup
		for range 2 {
			workers.Go(func() { _, err := db.ResetPasswordWithEmailCode(email, hash, "new-password", now+3); results <- err })
		}
		workers.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			} else {
				require.ErrorIs(t, err, core.ErrEmailCodeInvalid)
			}
		}
		require.Equal(t, 1, successes)
		stored, err := db.GetSession("old-secret")
		require.NoError(t, err)
		require.Nil(t, stored)
		token, err := db.GetTokenByName("alice")
		require.NoError(t, err)
		require.Equal(t, "new-password", token.EncryptedSecret)
		apiTokens, err := db.ListAPITokens("alice")
		require.NoError(t, err)
		require.Len(t, apiTokens, 1)
		require.False(t, apiTokens[0].Disabled)
		security, err := db.GetAccountSecurity("alice")
		require.NoError(t, err)
		require.True(t, security.PasswordLoginEnabled)
		require.Equal(t, core.RecoveryCodeCount, security.RecoveryCodesRemaining)
		keys, err := db.ListFidoDevices("alice")
		require.NoError(t, err)
		require.Len(t, keys, 1)
	})
	t.Run("wrong code attempts expire only this proof", func(t *testing.T) {
		email := account(t, "bobby")
		queue(t, email, now)
		for range 5 {
			_, err := db.ResetPasswordWithEmailCode(email, wrong, "new-password", now+1)
			require.ErrorIs(t, err, core.ErrEmailCodeInvalid)
		}
		resetInvalid(t, email, now+2)
		token, err := db.GetTokenByName("bobby")
		require.NoError(t, err)
		require.Equal(t, "old-password", token.EncryptedSecret)
		require.False(t, token.Ban.IsActive(now))
		queue(t, email, now+60000)
		_, err = db.ResetPasswordWithEmailCode(email, hash, "new-password", now+60001)
		require.NoError(t, err)
	})
	t.Run("failed queue and cooldown preserve prior proof and IP quota", func(t *testing.T) {
		email := account(t, "carol")
		queue(t, email, now)
		for _, scenario := range []struct {
			key  string
			at   int64
			want error
		}{
			{"invalid-key", now + 60001, nil}, {cfg.EncryptionKey, now + 1, mail.ErrRateLimited},
		} {
			candidate := job(email, scenario.at)
			created, err := db.QueueEmailPasswordReset(candidate, wrong, scenario.key, "192.0.2.2", cfg.ManualRate)
			require.Error(t, err)
			if scenario.want != nil {
				require.ErrorIs(t, err, scenario.want)
			}
			require.False(t, created)
			stored, err := db.GetMailJob(candidate.ID, cfg.EncryptionKey)
			require.NoError(t, err)
			require.Nil(t, stored)
		}
		var count int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM mail_rate_limits WHERE ip = ?`, "192.0.2.2").Scan(&count))
		require.Zero(t, count)
		_, err := db.ResetPasswordWithEmailCode(email, hash, "new-password", now+2)
		require.NoError(t, err)
	})
	for _, change := range []string{"expiry", "password", "email", "retirement"} {
		t.Run(change+" invalidates old code", func(t *testing.T) {
			name := "change_" + change
			email := account(t, name)
			queue(t, email, now)
			at := now + 1
			switch change {
			case "expiry":
				at = now + 600000
			case "password":
				require.NoError(t, db.SetAccountPassword(name, "changed-password", at))
			case "email":
				_, err := db.UpdateAccountEmail(name, "new-"+email, at)
				require.NoError(t, err)
			case "retirement":
				require.NoError(t, db.RetireAccount(name, at))
			}
			resetInvalid(t, email, at)
		})
	}
	t.Run("unknown mailbox cannot reset a later registration", func(t *testing.T) {
		email := "later@example.com"
		queue(t, email, now)
		account(t, "later")
		resetInvalid(t, email, now+1)
	})
	t.Run("new code invalidates old code and banned account remains banned", func(t *testing.T) {
		email := account(t, "banned")
		require.NoError(t, db.SetAccountBan("banned", &core.AccountBan{Reason: "test", CreatedAt: now}))
		queue(t, email, now)
		created, err := db.QueueEmailPasswordReset(job(email, now+60000), wrong, cfg.EncryptionKey, "192.0.2.1", cfg.ManualRate)
		require.NoError(t, err)
		require.True(t, created)
		resetInvalid(t, email, now+60001)
		_, err = db.ResetPasswordWithEmailCode(email, wrong, "new-password", now+60002)
		require.NoError(t, err)
		token, err := db.GetTokenByName("banned")
		require.NoError(t, err)
		require.True(t, token.Ban.IsActive(now+60002))
	})
	t.Run("proof capacity rolls back mail insertion and expired proofs release capacity", func(t *testing.T) {
		tx, err := db.Begin()
		require.NoError(t, err)
		var count int
		require.NoError(t, tx.QueryRow(`SELECT COUNT(*) FROM user_password_resets`).Scan(&count))
		for index := count; index < mail.MaxPendingJobs; index++ {
			_, err = tx.Exec(`INSERT INTO user_password_resets
                (email, user_id, code_hash, credential_hash, security_updated_at, attempts, created_at, expires_at)
                VALUES (?, '', ?, '', 0, 0, ?, ?)`, fmt.Sprintf("capacity-%d@example.com", index), hash, now, now+600000)
			require.NoError(t, err)
		}
		require.NoError(t, tx.Commit())
		candidate := job("overflow@example.com", now+1)
		created, err := db.QueueEmailPasswordReset(candidate, hash, cfg.EncryptionKey, "192.0.2.3", cfg.ManualRate)
		require.ErrorIs(t, err, mail.ErrQueueFull)
		require.False(t, created)
		stored, err := db.GetMailJob(candidate.ID, cfg.EncryptionKey)
		require.NoError(t, err)
		require.Nil(t, stored)
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM user_password_resets`).Scan(&count))
		require.Equal(t, mail.MaxPendingJobs, count)
		queue(t, "overflow@example.com", now+600000)
	})
}
