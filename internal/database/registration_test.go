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
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/mail"
	"renop/internal/testutil"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestRegistrationBoundaries(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "registration.db"), MaxOpenConns: 4, MaxIdleConns: 2})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	cfg := config.DefaultRegistrationConfig()
	cfg.Enabled = true
	mailCfg := mail.DefaultConfig()
	mailCfg.ManualRate.Limit = 100
	require.NoError(t, mailCfg.EnsureKey())
	now := time.Now().UnixMilli()
	hash := func(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }
	password, err := bcrypt.GenerateFromPassword([]byte("ValidPassword2026!"), bcrypt.MinCost)
	require.NoError(t, err)
	request := func(name string) core.AccountRegistration {
		return core.AccountRegistration{Username: name, Nickname: name, Email: name + "@example.com", PasswordHash: string(password), IPHash: hash(name)}
	}
	challenge := func(name string) (core.AccountRegistration, *core.PendingRegistration) {
		r := request(name)
		r.IDHash, r.CodeHash, r.RequireEmail = hash("id-"+name), hash("code-"+name), true
		p := &core.PendingRegistration{IDHash: r.IDHash, IPHash: r.IPHash, Email: r.Email, CodeHash: r.CodeHash,
			CreatedAt: now, ExpiresAt: now + 600000, CooldownUntil: now + 600000}
		job := &mail.Job{ID: name, AccountID: "sender", Scene: "registration_verify", TicketHash: hash("ticket"), CreatedAt: now, ExpiresAt: p.ExpiresAt,
			Message: mail.Message{ID: name, To: r.Email, Text: "Verification", Subject: "Verify", CreatedAt: now}}
		require.NoError(t, db.BeginRegistration(p, job, mailCfg.EncryptionKey, "192.0.2.1", mailCfg.ManualRate, cfg))
		return r, p
	}
	t.Run("atomic creation and persistent IP limit", func(t *testing.T) {
		r := request("new_account")
		_, err := db.RegisterAccount(r, cfg, now)
		require.NoError(t, err)
		account, err := db.GetTokenByEmail(r.Email)
		require.NoError(t, err)
		require.Equal(t, []string{"base"}, account.Permissions)
		require.Empty(t, account.Tokens)
		require.NoError(t, bcrypt.CompareHashAndPassword([]byte(account.EncryptedSecret), []byte("ValidPassword2026!")))
		require.NoError(t, db.RetireAccount(r.Username, now+1))
		r.Username, r.Email = "second_account", "second@example.com"
		_, err = db.RegisterAccount(r, cfg, now+2)
		require.ErrorIs(t, err, core.ErrRegistrationRateLimited)
		_, err = db.RegisterAccount(r, cfg, now+cfg.IPInterval.Duration().Milliseconds())
		require.NoError(t, err)
	})
	t.Run("disabled and missing verification", func(t *testing.T) {
		closed := cfg
		closed.Enabled = false
		r := request("disabled")
		_, err := db.RegisterAccount(r, closed, now)
		require.ErrorIs(t, err, core.ErrRegistrationDisabled)
		r.RequireEmail = true
		_, err = db.RegisterAccount(r, cfg, now)
		require.ErrorIs(t, err, core.ErrRegistrationInvalid)
	})
	t.Run("concurrent confirmations create exactly one account", func(t *testing.T) {
		r, _ := challenge("concurrent")
		results := make(chan error, 2)
		for range 2 {
			go func() { _, err := db.RegisterAccount(r, cfg, now+1); results <- err }()
		}
		first, second := <-results, <-results
		if first != nil {
			first, second = second, first
		}
		require.NoError(t, first)
		require.ErrorIs(t, second, core.ErrRegistrationRateLimited)
		var count int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM account_registrations WHERE id_hash = ?`, r.IDHash).Scan(&count))
		require.Zero(t, count)
	})
	for _, scenario := range []string{"expired", "guesses", "wrong_ip", "wrong_email"} {
		t.Run(scenario, func(t *testing.T) {
			r, p := challenge(scenario)
			at := now + 1
			switch scenario {
			case "expired":
				at = p.ExpiresAt
			case "guesses":
				wrong := r
				wrong.CodeHash = hash("wrong")
				for range 5 {
					_, err := db.RegisterAccount(wrong, cfg, at)
					require.ErrorIs(t, err, core.ErrRegistrationInvalid)
				}
			case "wrong_ip":
				r.IPHash = hash("other_ip")
			case "wrong_email":
				r.Email = "other@example.com"
			}
			_, err := db.RegisterAccount(r, cfg, at)
			require.ErrorIs(t, err, core.ErrRegistrationInvalid)
			token, err := db.GetTokenByName(r.Username)
			require.NoError(t, err)
			require.Nil(t, token)
		})
	}
	t.Run("email conflict rolls back account and preserves confirmation", func(t *testing.T) {
		r, _ := challenge("conflicted")
		existing := request("email_owner")
		existing.Email = r.Email
		_, err := db.RegisterAccount(existing, cfg, now)
		require.NoError(t, err)
		_, err = db.RegisterAccount(r, cfg, now+1)
		require.ErrorIs(t, err, core.ErrEmailAlreadyExists)
		token, err := db.GetTokenByName(r.Username)
		require.NoError(t, err)
		require.Nil(t, token)
		_, err = db.GetPendingRegistration(r.IDHash, now+1)
		require.NoError(t, err)
		var count int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM registration_ips WHERE ip_hash = ?`, r.IPHash).Scan(&count))
		require.Zero(t, count)
	})
	t.Run("provider timeout and cooldown survive without account privileges", func(t *testing.T) {
		r := request("github_user")
		r.IDHash = hash("github_registration")
		payload, err := json.Marshal(core.RegistrationProfile{Username: "suggested", Nickname: "GitHub", GitHubID: 42, GitHubLogin: "external",
			Principals: []core.GitHubPrincipal{{Type: core.GitHubPrincipalUser, GitHubID: 42, Login: "external"}}})
		require.NoError(t, err)
		p := &core.PendingRegistration{IDHash: r.IDHash, IPHash: r.IPHash, ProviderKey: "github:42", Email: r.Email,
			ProfileJSON: string(payload), CreatedAt: now, ExpiresAt: now + 600000, CooldownUntil: now + 600000 + cfg.ProviderCooldown.Duration().Milliseconds()}
		require.NoError(t, db.BeginRegistration(p, nil, "", "", mailCfg.ManualRate, cfg))
		identity, err := db.GetGitHubIdentityByProviderID(42)
		require.NoError(t, err)
		require.Nil(t, identity)
		_, err = db.RegisterAccount(r, cfg, p.ExpiresAt)
		require.ErrorIs(t, err, core.ErrRegistrationInvalid)
		next := *p
		next.IDHash = hash("second_registration")
		require.ErrorIs(t, db.BeginRegistration(&next, nil, "", "", mailCfg.ManualRate, cfg), core.ErrRegistrationPending)
		require.NoError(t, db.CleanRegistrations(p.ExpiresAt))
		var retainedEmail, retainedProfile string
		require.NoError(t, db.QueryRow(`SELECT email, profile_json FROM account_registrations WHERE id_hash = ?`, p.IDHash).Scan(&retainedEmail, &retainedProfile))
		require.Empty(t, retainedEmail)
		require.Empty(t, retainedProfile)
		next.CreatedAt = p.ExpiresAt
		next.ExpiresAt = next.CreatedAt + 600000
		next.CooldownUntil = next.ExpiresAt + cfg.ProviderCooldown.Duration().Milliseconds()
		require.ErrorIs(t, db.BeginRegistration(&next, nil, "", "", mailCfg.ManualRate, cfg), core.ErrRegistrationCooldown)
		next.CreatedAt = p.CooldownUntil
		next.ExpiresAt = next.CreatedAt + 600000
		next.CooldownUntil = next.ExpiresAt + cfg.ProviderCooldown.Duration().Milliseconds()
		require.NoError(t, db.BeginRegistration(&next, nil, "", "", mailCfg.ManualRate, cfg))
		r.Provider = "github"
		r.IDHash = next.IDHash
		r.RequireEmail = true
		_, err = db.RegisterAccount(r, cfg, next.CreatedAt+1)
		require.NoError(t, err)
		identity, err = db.GetGitHubIdentityByProviderID(42)
		require.NoError(t, err)
		require.Equal(t, r.Username, identity.Username)
	})
}
