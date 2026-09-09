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
	"strings"
	"testing"
	"time"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/mail"
	"renop/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestAccountEmailVerificationBoundaries(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "email-change.db"), MaxOpenConns: 4, MaxIdleConns: 2})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	cfg := mail.DefaultConfig()
	cfg.ManualRate.Limit = 100
	require.NoError(t, cfg.EnsureKey())
	now := time.Now().UnixMilli()
	hash, wrong := strings.Repeat("a", 64), strings.Repeat("b", 64)
	create := func(name string) *mail.Job {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: name, EncryptedSecret: "password", Permissions: []string{"base"}}))
		_, err := db.UpdateAccountEmail(name, name+"@example.com", now)
		require.NoError(t, err)
		session := &core.Session{PublicID: name, Username: name, CreatedAt: now}
		session.LastActive.Store(now)
		require.NoError(t, db.SaveSession(session, name))
		return &mail.Job{ID: name, AccountID: "primary", Scene: "email_verify", TicketHash: hash, CreatedAt: now, ExpiresAt: now + 600000,
			Message: mail.Message{ID: name, To: "next-" + name + "@example.com", Subject: "Verify", Text: "Verification code", CreatedAt: now}}
	}
	queue := func(job *mail.Job) {
		t.Helper()
		require.NoError(t, db.QueueAccountEmailChange(job.ID, job.ID, job, hash, cfg.EncryptionKey, "192.0.2.1", cfg.ManualRate))
	}
	t.Run("concurrent confirmation consumes one code", func(t *testing.T) {
		job := create("concurrent")
		queue(job)
		results := make(chan error, 2)
		for range 2 {
			go func() {
				_, err := db.ConfirmAccountEmailChange(job.ID, job.ID, job.Message.To, hash, now+1)
				results <- err
			}()
		}
		first, second := <-results, <-results
		if first != nil {
			first, second = second, first
		}
		require.NoError(t, first)
		require.ErrorIs(t, second, core.ErrEmailCodeInvalid)
	})
	for _, scenario := range []string{"expired", "guesses", "revoked", "changed"} {
		t.Run(scenario, func(t *testing.T) {
			job := create(scenario)
			queue(job)
			at := now + 1
			switch scenario {
			case "expired":
				at = job.ExpiresAt
			case "guesses":
				for range 5 {
					_, err := db.ConfirmAccountEmailChange(job.ID, job.ID, job.Message.To, wrong, at)
					require.ErrorIs(t, err, core.ErrEmailCodeInvalid)
				}
			case "revoked":
				require.NoError(t, db.DeleteSession(job.ID))
			case "changed":
				_, err := db.UpdateAccountEmail(job.ID, "changed-again@example.com", now)
				require.NoError(t, err)
			}
			_, err := db.ConfirmAccountEmailChange(job.ID, job.ID, job.Message.To, hash, at)
			require.ErrorIs(t, err, core.ErrEmailCodeInvalid)
			security, err := db.GetAccountSecurity(job.ID)
			require.NoError(t, err)
			require.NotEqual(t, job.Message.To, security.Email)
		})
	}
	t.Run("mail rejection rolls back verification and queue writes", func(t *testing.T) {
		rate := cfg.ManualRate
		rate.Limit = 1
		for index := range 2 {
			job := create(fmt.Sprintf("limited_%d", index))
			err := db.QueueAccountEmailChange(job.ID, job.ID, job, hash, cfg.EncryptionKey, "192.0.2.2", rate)
			if index == 0 {
				require.NoError(t, err)
				continue
			}
			require.ErrorIs(t, err, mail.ErrRateLimited)
			var count int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM user_email_changes WHERE user_id = ?`, job.UserID).Scan(&count))
			require.Zero(t, count)
			stored, err := db.GetMailJob(job.ID, cfg.EncryptionKey)
			require.NoError(t, err)
			require.Nil(t, stored)
		}
	})
	t.Run("an existing address cannot be claimed", func(t *testing.T) {
		job := create("conflict")
		queue(job)
		other := create("owner")
		_, err := db.UpdateAccountEmail(other.ID, job.Message.To, now)
		require.NoError(t, err)
		_, err = db.ConfirmAccountEmailChange(job.ID, job.ID, job.Message.To, hash, now+1)
		require.ErrorIs(t, err, core.ErrEmailAlreadyExists)
		security, err := db.GetAccountSecurity(job.ID)
		require.NoError(t, err)
		require.Equal(t, "conflict@example.com", security.Email)
	})
}
