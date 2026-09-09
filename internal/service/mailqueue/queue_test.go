/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package mailqueue

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/mail"
	"renop/internal/testutil"
)

func TestMailAccountDisableDuringStatusCheckNeverResends(t *testing.T) {
	state, db := queueTestState(t)
	cfg := state.Inner.Config.Load().DeepCopy()
	cfg.Mail.Accounts[0].Provider = "sendgrid"
	cfg.Mail.Accounts[0].Quota.Limit = -1
	var sends, checks atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			sends.Add(1)
			w.Header().Set("X-Message-Id", "provider-id")
			w.WriteHeader(202)
		} else {
			checks.Add(1)
			_, _ = io.WriteString(w, `{"messages":[{"msg_id":"provider-id.recipient","to_email":"receiver@example.com","status":"delivered"}]}`)
		}
	}))
	defer server.Close()
	cfg.Mail.Accounts[0].Endpoint = server.URL
	state.Inner.Config.Store(cfg)
	now := time.Now()
	control, owned, err := db.AcquireMailLease("worker", now.UnixMilli())
	require.NoError(t, err)
	require.True(t, owned)
	receipt, err := Enqueue(state, Request{To: "receiver@example.com", Scene: "test"})
	require.NoError(t, err)
	w := &worker{state: state, owner: "worker", client: mail.NewClient(server.Client().Transport), calibrationDue: map[string]int64{}}
	defer w.client.Close()
	job, err := db.GetMailJob(receipt.ID, cfg.Mail.EncryptionKey)
	require.NoError(t, err)
	require.NoError(t, w.process(context.Background(), cfg.Mail, control, job, now))
	job, err = db.GetMailJob(receipt.ID, cfg.Mail.EncryptionKey)
	require.NoError(t, err)
	require.Equal(t, "checking", job.Status)
	disabled := cfg.DeepCopy()
	disabled.Mail.Accounts[0].Enabled = false
	state.Inner.Config.Store(disabled)
	require.NoError(t, w.process(context.Background(), disabled.Mail, control, job, time.Now()))
	job, err = db.GetMailJob(receipt.ID, cfg.Mail.EncryptionKey)
	require.NoError(t, err)
	require.Equal(t, "checking", job.Status)
	require.Equal(t, "provider-id", job.Result.MessageID)
	state.Inner.Config.Store(cfg)
	require.NoError(t, w.process(context.Background(), cfg.Mail, control, job, time.Now()))
	job, err = db.GetMailJob(receipt.ID, cfg.Mail.EncryptionKey)
	require.NoError(t, err)
	require.Equal(t, "delivered", job.Status)
	require.EqualValues(t, 1, sends.Load())
	require.EqualValues(t, 1, checks.Load())
}

type queueRoundTrip func(*http.Request) (*http.Response, error)

func (f queueRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMailOAuthFailureCountsTowardAccountRateWithoutQuotaDebit(t *testing.T) {
	state, db := queueTestState(t)
	cfg := state.Inner.Config.Load().DeepCopy()
	cfg.Mail.AccountRate.Limit = 1
	cfg.Mail.Delay.Value = 0
	cfg.Mail.Accounts[0].Provider = "gmail"
	cfg.Mail.Accounts[0].ClientID = "client"
	cfg.Mail.Accounts[0].RefreshToken = "invalid-refresh"
	state.Inner.Config.Store(cfg)
	var requests atomic.Int32
	client := mail.NewClient(queueRoundTrip(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{StatusCode: 401, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"invalid_grant"}`))}, nil
	}))
	w := &worker{state: state, owner: "worker", client: client, calibrationDue: map[string]int64{}}
	now := time.Now()
	control, owned, err := db.AcquireMailLease("worker", now.UnixMilli())
	require.NoError(t, err)
	require.True(t, owned)
	for i := range 2 {
		receipt, err := Enqueue(state, Request{To: "receiver@example.com", Scene: "test"})
		require.NoError(t, err)
		job, err := db.GetMailJob(receipt.ID, cfg.Mail.EncryptionKey)
		require.NoError(t, err)
		require.NoError(t, w.process(context.Background(), cfg.Mail, control, job, now))
		stored, err := db.GetMailJob(receipt.ID, cfg.Mail.EncryptionKey)
		require.NoError(t, err)
		if i == 0 {
			require.Equal(t, "failed", stored.Status)
		} else {
			require.Equal(t, "paused", stored.Status)
		}
	}
	usage, err := db.LoadMailAccount("primary", cfg.Mail.EncryptionKey)
	require.NoError(t, err)
	require.EqualValues(t, 1, requests.Load())
	require.EqualValues(t, 1, usage.Rate.Used)
	require.EqualValues(t, 1, usage.Attempts)
	require.Zero(t, usage.Charged)
}

func queueTestState(t *testing.T) (*core.AppState, *database.DB) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Mail.Enabled = true
	cfg.Mail.PublicURL = "https://renop.example"
	cfg.Mail.Calibration.Value = 0
	require.NoError(t, cfg.Mail.EnsureKey())
	account := mail.Presets()[0].Account
	account.ID = "primary"
	account.From = "sender@example.com"
	account.APIKey = "api-secret"
	account.AccountID = "account"
	account.Quota.Limit = 2
	cfg.Mail.Accounts = []mail.Account{account}
	cfg.Mail.Normalize()
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "mail.db")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state := core.NewAppState()
	state.Inner.DB = db
	state.Inner.Config.Store(cfg)
	return state, db
}

func TestQueuedMailCapturesTheRecipientLanguage(t *testing.T) {
	state, db := queueTestState(t)
	now := time.Now().UnixMilli()
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", Permissions: []string{"base"}}))
	_, err := db.UpdateAccountEmail("alice", "alice@example.com", now)
	require.NoError(t, err)
	session := &core.Session{PublicID: "locale", Username: "alice", CreatedAt: now}
	session.LastActive.Store(now)
	require.NoError(t, db.SaveSession(session, "locale-session"))
	profile, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	require.NoError(t, db.SetUserLocale("alice", "locale-session", "ja-JP", profile.UserID))
	for _, request := range []Request{{Username: "alice", Scene: "test"}, {To: "alice@example.com", Scene: "test"}} {
		receipt, err := Enqueue(state, request)
		require.NoError(t, err)
		job, err := db.GetMailJob(receipt.ID, state.Inner.Config.Load().Mail.EncryptionKey)
		require.NoError(t, err)
		require.Contains(t, job.Message.HTML, `lang="ja-JP"`)
		require.Equal(t, "RenoP テストメール", job.Message.Subject)
	}
	require.NoError(t, db.SetUserLocale("alice", "locale-session", "fr-FR", profile.UserID))
	receipt, err := Enqueue(state, Request{Username: "alice", Scene: "test"})
	require.NoError(t, err)
	job, err := db.GetMailJob(receipt.ID, state.Inner.Config.Load().Mail.EncryptionKey)
	require.NoError(t, err)
	require.Contains(t, job.Message.HTML, `lang="fr-FR"`)
}

func TestMailQueueSerialQuotaRefundAndDurableCompletion(t *testing.T) {
	state, db := queueTestState(t)
	key := state.Inner.Config.Load().Mail.EncryptionKey
	now := time.Now()
	control, owned, err := db.AcquireMailLease("worker", now.UnixMilli())
	require.NoError(t, err)
	require.True(t, owned)
	_, owned, err = db.AcquireMailLease("competitor", now.UnixMilli())
	require.NoError(t, err)
	require.False(t, owned)
	var inFlight, maxInFlight atomic.Int32
	status := http.StatusUnauthorized
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := inFlight.Add(1)
		defer inFlight.Add(-1)
		maxInFlight.Store(max(maxInFlight.Load(), current))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == 200 {
			_, _ = io.WriteString(w, `{"success":true,"result":{"message_id":"provider-id","delivered":["receiver@example.com"]}}`)
		} else {
			_, _ = io.WriteString(w, `{"error":{"code":"invalid_client"}}`)
		}
	}))
	defer server.Close()
	cfg := state.Inner.Config.Load().DeepCopy()
	cfg.Mail.Accounts[0].Endpoint = server.URL
	state.Inner.Config.Store(cfg)
	w := &worker{state: state, owner: "worker", client: mail.NewClient(server.Client().Transport), calibrationDue: map[string]int64{}}
	receipt, err := Enqueue(state, Request{To: "receiver@example.com", Scene: "test", Manual: true, IP: "192.0.2.1"})
	require.NoError(t, err)
	_, err = Enqueue(state, Request{To: "receiver@example.com", Scene: "test", Manual: true, IP: "192.0.2.1"})
	require.ErrorIs(t, err, mail.ErrRateLimited)
	job, err := db.NextMailJob(key, time.Now().UnixMilli())
	require.NoError(t, err)
	require.NotNil(t, job)
	require.True(t, CanRead(job, "", receipt.Ticket))
	require.False(t, CanRead(job, "other", "wrong"))
	require.NoError(t, w.process(context.Background(), cfg.Mail, control, job, time.Now()))
	stored, err := db.GetMailJob(receipt.ID, key)
	require.NoError(t, err)
	require.Equal(t, "failed", stored.Status)
	require.Empty(t, stored.Message.HTML)
	account, err := db.LoadMailAccount("primary", key)
	require.NoError(t, err)
	require.EqualValues(t, 1, account.Attempts)
	require.Zero(t, account.Charged)
	require.Zero(t, account.Usage["month"].Used)
	status = 200
	second, err := Enqueue(state, Request{To: "receiver@example.com", Scene: "test"})
	require.NoError(t, err)
	control, _, err = db.AcquireMailLease("worker", time.Now().UnixMilli())
	require.NoError(t, err)
	job, err = db.GetMailJob(second.ID, key)
	require.NoError(t, err)
	require.NoError(t, w.process(context.Background(), cfg.Mail, control, job, time.Now()))
	require.Equal(t, "queued", job.Status)
	require.Greater(t, job.NextAt, time.Now().UnixMilli())
	control.NextSendAt = 0
	require.NoError(t, w.process(context.Background(), cfg.Mail, control, job, time.Now()))
	require.Equal(t, "delivered", job.Status)
	require.EqualValues(t, 1, maxInFlight.Load())
	account, err = db.LoadMailAccount("primary", key)
	require.NoError(t, err)
	require.EqualValues(t, 1, account.Charged)
	rollback := account
	rollback.Charged = 999
	job.UpdatedAt = time.Now().UnixMilli()
	require.ErrorIs(t, db.SaveMailAttempt("worker", key, job, &rollback, 0), mail.ErrFinalized)
	account, err = db.LoadMailAccount("primary", key)
	require.NoError(t, err)
	require.EqualValues(t, 1, account.Charged)
	var ciphertext string
	require.NoError(t, db.QueryRow(`SELECT payload FROM mail_accounts WHERE id = ?`, "primary").Scan(&ciphertext))
	require.NotContains(t, ciphertext, "charged")
}

func TestMailQueueConcurrentManualRequestsAndRestart(t *testing.T) {
	state, db := queueTestState(t)
	key := state.Inner.Config.Load().Mail.EncryptionKey
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Enqueue(state, Request{To: "receiver@example.com", Scene: "test", Manual: true, IP: "192.0.2.4"}); err == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, accepted.Load())
	control, owned, err := db.AcquireMailLease("first", time.Now().UnixMilli())
	require.NoError(t, err)
	require.True(t, owned)
	job, err := db.NextMailJob(key, time.Now().UnixMilli())
	require.NoError(t, err)
	require.NotNil(t, job)
	job.Status = "sending"
	job.UpdatedAt = time.Now().UnixMilli()
	require.NoError(t, db.SaveMailAttempt("first", key, job, nil, 0))
	_, owned, err = db.AcquireMailLease("replacement", control.LeaseUntil+1)
	require.NoError(t, err)
	require.True(t, owned)
	require.NoError(t, db.RecoverMailAttempts("replacement", control.LeaseUntil+2))
	stored, err := db.GetMailJob(job.ID, key)
	require.NoError(t, err)
	require.Equal(t, "unknown", stored.Status)
	next, err := db.NextMailJob(key, control.LeaseUntil+3)
	require.NoError(t, err)
	require.Nil(t, next)
}

func TestMailNotificationEventsAndRetirement(t *testing.T) {
	state, db := queueTestState(t)
	cfg := state.Inner.Config.Load()
	key := cfg.Mail.EncryptionKey
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", EncryptedSecret: "test-password", CreatedAt: time.Now().UTC().Format(time.RFC3339), Permissions: []string{"base"}}))
	_, err := db.UpdateAccountEmail("alice", "alice@example.com", time.Now().UnixMilli())
	require.NoError(t, err)
	control, owned, err := db.AcquireMailLease("worker", time.Now().UnixMilli())
	require.NoError(t, err)
	require.True(t, owned)
	for i, action := range []string{"USER_PERMISSION_UPDATE", "USER_BAN", "USER_UNBAN", "REVIEW_REQUEST"} {
		require.NoError(t, db.SaveAuditLog(&core.AuditLogEntry{Username: "alice", Operator: "admin", Action: action, CreatedAt: time.Now().UnixMilli() + int64(i)}))
	}
	require.NoError(t, db.SaveMessages([]*core.UserMessage{{ID: "invitation-1", Recipient: "alice", Kind: "super_team_invitation", Title: "Invitation", Body: "Join", CreatedAt: time.Now().UnixMilli()}}))
	w := &worker{state: state, owner: "worker"}
	require.NoError(t, w.notifications(control, time.Now()))
	jobs, total, err := db.ListMailJobs("", "", key, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 5, total)
	scenes := map[string]bool{}
	for _, job := range jobs {
		scenes[job.Scene] = true
	}
	require.True(t, scenes["permission_changed"])
	require.True(t, scenes["super_team_invitation"])
	require.NoError(t, w.notifications(control, time.Now()))
	_, total, err = db.ListMailJobs("", "", key, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 5, total)
	require.NoError(t, db.RetireAccount("alice", time.Now().UnixMilli()))
	_, total, err = db.ListMailJobs("", "", key, 20, 0)
	require.NoError(t, err)
	require.Zero(t, total)
	_, err = Enqueue(state, Request{Username: "alice", Scene: "security_changed"})
	require.Error(t, err)
	require.True(t, differentNetwork("192.0.2.1", "198.51.100.2"))
	require.False(t, differentNetwork("192.0.2.1", "192.0.2.254"))
	require.False(t, differentNetwork("", "192.0.2.1"))
	encoded, err := json.Marshal(cfg.Mail)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), key)
}
