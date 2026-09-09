/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
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

	"github.com/stretchr/testify/require"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/mail"
	"renop/internal/testutil"
)

func TestProviderEmailAliasesAreAtomicAndRetained(t *testing.T) {
	cfg := config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "provider-emails.db")}
	db, err := InitDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	now := time.Now().UnixMilli()
	for _, name := range []string{"alice", "bravo"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: name, EncryptedSecret: "password", Permissions: []string{"base"}}))
		_, err := db.UpdateAccountEmail(name, name+"@example.com", now)
		require.NoError(t, err)
		session := &core.Session{PublicID: name, Username: name, CreatedAt: now}
		session.LastActive.Store(now)
		require.NoError(t, db.SaveSession(session, name))
	}
	identity := core.OAuthIdentity{ProviderID: "google", Subject: "alice-subject", Authority: strings.Repeat("a", 64),
		Emails: []core.ProviderEmail{{Email: "provider@example.com", Verified: true}, {Email: "alice@example.com"}}}
	link := func(name string, proof core.OAuthIdentity) error {
		state, err := db.GetMFAState(name)
		if err != nil {
			return err
		}
		return db.LinkOAuthIdentity(name, name, state.Snapshot, proof, now+1)
	}
	require.NoError(t, link("alice", identity))
	account, err := db.GetTokenByEmail("Provider@EXAMPLE.com")
	require.NoError(t, err)
	require.Equal(t, "alice", account.Name)
	security, err := db.GetAccountSecurity("alice")
	require.NoError(t, err)
	require.Equal(t, []string{"provider@example.com"}, security.EmailAliases)
	require.ErrorIs(t, link("bravo", identity), core.ErrOAuthIdentityLinked)
	other := identity
	other.Subject = "bravo-subject"
	other.Emails = []core.ProviderEmail{{Email: "free@example.com", Verified: true}, {Email: "provider@example.com", Verified: true}}
	require.ErrorIs(t, link("bravo", other), core.ErrEmailAlreadyExists)
	missing, err := db.GetTokenByEmail("free@example.com")
	require.NoError(t, err)
	require.Nil(t, missing)
	unlinked, err := db.GetOAuthIdentity(other)
	require.NoError(t, err)
	require.Nil(t, unlinked)
	bravo, err := db.GetMFAState("bravo")
	require.NoError(t, err)
	principals := []core.GitHubPrincipal{{Type: core.GitHubPrincipalUser, GitHubID: 91, Login: "bravo"}}
	require.ErrorIs(t, db.LinkGitHubIdentity("bravo", "bravo", bravo.Snapshot, 91, "bravo", principals, now,
		core.ProviderEmail{Email: "provider@example.com", Verified: true}), core.ErrEmailAlreadyExists)
	github, err := db.GetGitHubIdentity("bravo")
	require.NoError(t, err)
	require.Nil(t, github)
	require.NoError(t, db.LinkGitHubIdentity("bravo", "bravo", bravo.Snapshot, 91, "bravo", principals, now,
		core.ProviderEmail{Email: "bravo-gh@example.com", Verified: true}))
	alice, err := db.GetMFAState("alice")
	require.NoError(t, err)
	require.ErrorIs(t, db.LinkGitHubIdentity("alice", "alice", alice.Snapshot, 91, "bravo", principals, now), core.ErrGitHubIdentityLinked)
	other.Emails = []core.ProviderEmail{{Email: "unverified@example.com"}}
	require.ErrorIs(t, link("bravo", other), core.ErrEmailVerificationRequired)
	missing, err = db.GetTokenByEmail("unverified@example.com")
	require.NoError(t, err)
	require.Nil(t, missing)
	_, err = db.UpdateAccountEmail("bravo", "provider@example.com", now+2)
	require.ErrorIs(t, err, core.ErrEmailAlreadyExists)
	_, err = db.UpdateAccountEmail("alice", "new-primary@example.com", now+3)
	require.NoError(t, err)
	require.NoError(t, db.DeleteOAuthIdentity("alice", "alice", "google", now+4))
	account, err = db.GetTokenByEmail("alice@example.com")
	require.NoError(t, err)
	require.Equal(t, "alice", account.Name, "disconnecting a provider does not release its proven email addresses")
	require.NoError(t, db.Close())
	db, err = InitDB(cfg)
	require.NoError(t, err)
	account, err = db.GetTokenByEmail("provider@example.com")
	require.NoError(t, err)
	require.Equal(t, "alice", account.Name)
	require.NoError(t, db.RenameToken("alice", "alice2", account))
	account, err = db.GetTokenByEmail("provider@example.com")
	require.NoError(t, err)
	require.Equal(t, "alice2", account.Name)
	require.NoError(t, db.SetAccountBan("alice2", &core.AccountBan{Reason: "Abuse", CreatedAt: now + 5}))
	_, err = db.UpdateAccountEmail("bravo", "provider@example.com", now+6)
	require.ErrorIs(t, err, core.ErrEmailAlreadyExists)
	require.NoError(t, db.SetAccountBan("alice2", nil))
	retiredAt := now + 7
	require.NoError(t, db.RetireAccount("alice2", retiredAt))
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "charlie", EncryptedSecret: "password", Permissions: []string{"base"}}))
	charlie, err := db.GetMFAState("charlie")
	require.NoError(t, err)
	require.NoError(t, db.StoreGitHubIdentity(charlie.UserID, 92, "charlie", []core.GitHubPrincipal{{Type: "user", GitHubID: 92, Login: "charlie"}}, now,
		core.ProviderEmail{Email: "alias-only@example.com", Verified: true}))
	require.NoError(t, db.RetireAccount("charlie", retiredAt))
	retention, err := db.GetAccountRetirementStatus("charlie")
	require.NoError(t, err)
	require.Zero(t, retention.EmailReleasedAt, "provider-only addresses must remain reserved without a primary email")
	for _, email := range []string{"alice@example.com", "provider@example.com", "new-primary@example.com"} {
		account, err = db.GetTokenByEmail(email)
		require.NoError(t, err)
		require.Equal(t, retiredAt, account.DeletedAt, email)
		_, err = db.UpdateAccountEmail("bravo", email, now+8)
		require.ErrorIs(t, err, core.ErrEmailAlreadyExists)
	}
	account, err = db.GetTokenByEmail("alias-only@example.com")
	require.NoError(t, err)
	require.Equal(t, retiredAt, account.DeletedAt)
	require.NoError(t, db.CleanupRetiredAccountData(retiredAt+core.AccountEmailHoldMillis, 10))
	for _, email := range []string{"alice@example.com", "provider@example.com", "new-primary@example.com", "alias-only@example.com"} {
		account, err = db.GetTokenByEmail(email)
		require.NoError(t, err)
		require.Nil(t, account, email)
	}
}

func TestEmailAliasProofLimitAndRemoval(t *testing.T) {
	db, err := InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "alias-proof.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	now := time.Now().UnixMilli()
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", EncryptedSecret: "password", Permissions: []string{"base"}}))
	_, err = db.UpdateAccountEmail("alice", "primary@example.com", now)
	require.NoError(t, err)
	session := &core.Session{PublicID: "alice", Username: "alice", CreatedAt: now}
	session.LastActive.Store(now)
	require.NoError(t, db.SaveSession(session, "alice"))
	cfg := mail.DefaultConfig()
	require.NoError(t, cfg.EnsureKey())
	job := &mail.Job{ID: "alias-proof", AccountID: "sender", Scene: "email_verify", TicketHash: strings.Repeat("a", 64),
		CreatedAt: now, ExpiresAt: now + 600000,
		Message: mail.Message{ID: "alias-proof", To: "second@example.com", Subject: "Verify", Text: "Code", CreatedAt: now}}
	code := strings.Repeat("b", 64)
	require.NoError(t, db.QueueAccountEmailChange("alice", "alice", job, code, cfg.EncryptionKey, "192.0.2.1", cfg.ManualRate, true))
	missing, err := db.GetTokenByEmail("second@example.com")
	require.NoError(t, err)
	require.Nil(t, missing, "unconfirmed email claims grant no identifier or registration reservation")
	security, err := db.ConfirmAccountEmailChange("alice", "alice", "second@example.com", code, now+1)
	require.NoError(t, err)
	require.Equal(t, "primary@example.com", security.Email)
	require.Equal(t, []string{"second@example.com"}, security.EmailAliases)
	_, err = db.ConfirmAccountEmailChange("alice", "alice", "second@example.com", code, now+2)
	require.ErrorIs(t, err, core.ErrEmailCodeInvalid)
	identity := core.OAuthIdentity{ProviderID: "demo", Authority: strings.Repeat("a", 64), Subject: "subject",
		Emails: []core.ProviderEmail{{Email: "second@example.com"}}}
	state, err := db.GetMFAState("alice")
	require.NoError(t, err)
	require.NoError(t, db.LinkOAuthIdentity("alice", "alice", state.Snapshot, identity, now+3))
	_, err = db.DeleteAccountEmailAlias("alice", "alice", "primary@example.com", now+4)
	require.ErrorIs(t, err, core.ErrPrimaryEmail)
	security, err = db.DeleteAccountEmailAlias("alice", "alice", "second@example.com", now+4)
	require.NoError(t, err)
	require.Empty(t, security.EmailAliases)
	missing, err = db.GetTokenByEmail("second@example.com")
	require.NoError(t, err)
	require.Nil(t, missing)
	identity.Emails = nil
	for i := range core.MaxAccountEmails {
		identity.Emails = append(identity.Emails, core.ProviderEmail{Email: fmt.Sprintf("alias%d@example.com", i), Verified: true})
	}
	require.ErrorIs(t, db.RefreshOAuthIdentity(state.UserID, identity, now+5), core.ErrAccountEmailLimit)
	missing, err = db.GetTokenByEmail("alias0@example.com")
	require.NoError(t, err)
	require.Nil(t, missing, "an over-limit refresh must not partially reserve addresses")
	identity.Emails = identity.Emails[:core.MaxAccountEmails-1]
	require.NoError(t, db.RefreshOAuthIdentity(state.UserID, identity, now+6))
	security, err = db.GetAccountSecurity("alice")
	require.NoError(t, err)
	require.Len(t, security.EmailAliases, core.MaxAccountEmails-1)
}

func TestLegacyPrimaryEmailsMigrateForActiveAndRetiredAccounts(t *testing.T) {
	cfg := config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "legacy-emails.db")}
	db, err := InitDB(cfg)
	require.NoError(t, err)
	for _, name := range []string{"active", "retired"} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: name, Permissions: []string{"base"}}))
		_, err := db.UpdateAccountEmail(name, name+"@example.com", time.Now().UnixMilli())
		require.NoError(t, err)
		profile, err := db.GetUserProfile(name)
		require.NoError(t, err)
		if name == "retired" {
			require.NoError(t, db.RetireAccount(name, time.Now().UnixMilli()))
		}
		_, err = db.Exec(`DELETE FROM user_email_addresses WHERE user_id = ?`, profile.UserID)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
	db, err = InitDB(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, name := range []string{"active", "retired"} {
		account, err := db.GetTokenByEmail(name + "@example.com")
		require.NoError(t, err)
		require.NotNil(t, account)
		require.Equal(t, name, account.Name)
		require.Equal(t, name == "retired", account.DeletedAt > 0)
	}
}
