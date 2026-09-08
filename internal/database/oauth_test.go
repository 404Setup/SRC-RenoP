/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/mail"
	"renop/internal/testutil"
)

func TestOAuthIdentityBoundaries(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "oauth.db"), MaxOpenConns: 1, MaxIdleConns: 1})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	now := time.Now().UnixMilli()
	for _, username := range []string{"alice", "bob"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: username, EncryptedSecret: "password", Permissions: []string{"base"}}))
		s := &core.Session{PublicID: username, Username: username, CreatedAt: now}
		s.LastActive.Store(now)
		require.NoError(t, db.SaveSession(s, username))
	}
	identity := core.OAuthIdentity{ProviderID: "google", Subject: "stable-subject", Authority: strings.Repeat("a", 64), Login: "Alice"}
	before, err := db.GetMFAState("alice")
	require.NoError(t, err)
	require.NoError(t, db.LinkOAuthIdentity("alice", "alice", before.Snapshot, identity, now))
	after, err := db.GetMFAState("alice")
	require.NoError(t, err)
	require.NotEqual(t, before.Snapshot, after.Snapshot)
	identity.Namespaces = []string{"alice", "owned-group"}
	require.NoError(t, db.RefreshOAuthIdentity(after.UserID, identity, now+1))
	refreshed, err := db.GetOAuthIdentity(identity)
	require.NoError(t, err)
	require.Equal(t, identity.Namespaces, refreshed.Namespaces)
	require.Equal(t, now+1, refreshed.AuthorizedAt)
	require.ErrorIs(t, db.LinkOAuthIdentity("alice", "alice", before.Snapshot, identity, now), core.ErrMFAInvalid)
	bob, err := db.GetMFAState("bob")
	require.NoError(t, err)
	require.ErrorIs(t, db.LinkOAuthIdentity("bob", "bob", bob.Snapshot, identity, now), core.ErrOAuthIdentityLinked)
	otherAuthority := identity
	otherAuthority.Authority = strings.Repeat("b", 64)
	unlinked, err := db.GetOAuthIdentity(otherAuthority)
	require.NoError(t, err)
	require.Nil(t, unlinked)
	require.ErrorIs(t, db.LinkOAuthIdentity("alice", "alice", after.Snapshot, otherAuthority, now), core.ErrOAuthIdentityLinked)
	security, err := db.SetPasswordLoginEnabled("alice", false, now+1)
	require.NoError(t, err)
	require.Equal(t, 1, security.OAuthIdentityCount)
	require.True(t, security.CanDisablePasswordLogin)
	require.ErrorIs(t, db.DeleteOAuthIdentity("alice", "alice", "google", now+2), core.ErrLastLoginMethod)
	profile, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	require.NoError(t, db.StoreGitHubIdentity(profile.UserID, 91, "alice-gh", []core.GitHubPrincipal{{Type: "user", GitHubID: 91, Login: "alice-gh"}}, now))
	require.NoError(t, db.DeleteGitHubIdentity("alice"))
	after, err = db.GetMFAState("alice")
	require.NoError(t, err)
	second := identity
	second.ProviderID = "gitlab"
	require.NoError(t, db.LinkOAuthIdentity("alice", "alice", after.Snapshot, second, now+3))
	after, err = db.GetMFAState("alice")
	require.NoError(t, err)
	require.NoError(t, db.DeleteOAuthIdentity("alice", "alice", "google", now+3))
	require.NoError(t, db.RefreshOAuthIdentity(after.UserID, identity, now+4))
	refreshed, err = db.GetOAuthIdentity(identity)
	require.NoError(t, err)
	require.Nil(t, refreshed, "refresh must not resurrect an unlinked identity")
	_, err = db.SetPasswordLoginEnabled("alice", false, now+3)
	require.NoError(t, err)
	stale := &core.Session{PublicID: "stale", Username: "alice", AuthenticationSnapshot: after.Snapshot, CreatedAt: now}
	require.ErrorIs(t, db.SaveSession(stale, "stale"), core.ErrMFAInvalid)
	_, err = db.UpdateAccountEmail("alice", "reserved@example.com", now+4)
	require.NoError(t, err)
	require.NoError(t, db.RetireAccount("alice", now+5))
	identities, err := db.GetOAuthIdentities("alice")
	require.NoError(t, err)
	require.Empty(t, identities)
	require.NoError(t, db.LinkOAuthIdentity("bob", "bob", bob.Snapshot, second, now+6))
	_, err = db.UpdateAccountEmail("bob", "reserved@example.com", now+7)
	require.ErrorIs(t, err, core.ErrEmailAlreadyExists)
	require.ErrorIs(t, db.LinkOAuthIdentity("alice", "alice", after.Snapshot, identity, now+8), core.ErrAccountDeleted)
}

func TestOAuthRegistrationRequiresUnverifiedEmail(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "oauth-registration.db"), MaxOpenConns: 1, MaxIdleConns: 1})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	cfg := config.DefaultRegistrationConfig()
	cfg.Enabled = true
	mailCfg := mail.DefaultConfig()
	require.NoError(t, mailCfg.EnsureKey())
	now := time.Now().UnixMilli()
	identity := core.OAuthIdentity{ProviderID: "stackexchange", Subject: "42", Authority: strings.Repeat("a", 64)}
	payload, err := json.Marshal(core.RegistrationProfile{OAuth: &identity})
	require.NoError(t, err)
	p := &core.PendingRegistration{IDHash: strings.Repeat("b", 64), IPHash: strings.Repeat("c", 64),
		ProviderKey: identity.ProviderID + ":" + identity.Key(), ProfileJSON: string(payload),
		CreatedAt: now, ExpiresAt: now + 600000, CooldownUntil: now + 600000 + cfg.ProviderCooldown.Duration().Milliseconds()}
	require.NoError(t, db.BeginRegistration(p, nil, "", "", mailCfg.ManualRate, cfg))
	password, err := bcrypt.GenerateFromPassword([]byte("Password2026!"), bcrypt.MinCost)
	require.NoError(t, err)
	r := core.AccountRegistration{Provider: identity.ProviderID, Username: "external", PasswordHash: string(password), IDHash: p.IDHash, IPHash: p.IPHash}
	_, err = db.RegisterAccount(r, cfg, now+1)
	require.ErrorIs(t, err, core.ErrRegistrationInvalid)
	r.Email, r.CodeHash = "external@example.com", strings.Repeat("d", 64)
	job := &mail.Job{ID: "oauth-registration", AccountID: "sender", Scene: "registration_verify", TicketHash: strings.Repeat("e", 64),
		CreatedAt: now + 2, ExpiresAt: p.ExpiresAt, Message: mail.Message{ID: "oauth-registration", To: r.Email, Subject: "Verify", Text: "Code", CreatedAt: now + 2}}
	require.ErrorIs(t, db.QueueProviderRegistrationEmail(p.IDHash, strings.Repeat("f", 64), r.CodeHash, job,
		mailCfg.EncryptionKey, "192.0.2.1", mailCfg.ManualRate, cfg), core.ErrRegistrationInvalid)
	job.ExpiresAt++
	require.ErrorIs(t, db.QueueProviderRegistrationEmail(p.IDHash, p.IPHash, r.CodeHash, job,
		mailCfg.EncryptionKey, "192.0.2.1", mailCfg.ManualRate, cfg), core.ErrRegistrationInvalid)
	job.ExpiresAt--
	require.NoError(t, db.QueueProviderRegistrationEmail(p.IDHash, p.IPHash, r.CodeHash, job,
		mailCfg.EncryptionKey, "192.0.2.1", mailCfg.ManualRate, cfg))
	wrong := r
	wrong.CodeHash = strings.Repeat("e", 64)
	_, err = db.RegisterAccount(wrong, cfg, now+3)
	require.ErrorIs(t, err, core.ErrRegistrationInvalid)
	_, err = db.RegisterAccount(r, cfg, now+4)
	require.NoError(t, err)
	linked, err := db.GetOAuthIdentity(identity)
	require.NoError(t, err)
	require.Equal(t, "external", linked.Username)
}
