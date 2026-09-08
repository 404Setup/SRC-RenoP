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
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/mail"
	"renop/internal/service/audit"
	"renop/internal/service/outboundproxy"
	"renop/internal/utils"
)

type completion struct {
	job         *mail.Job
	account     *mail.AccountState
	refund      bool
	reservation mail.Reservation
	nextSendAt  int64
}

type worker struct {
	state           *core.AppState
	owner           string
	client          *mail.Client
	proxyHash       [32]byte
	recovered       bool
	pending         *completion
	calibrationDue  map[string]int64
	lastMaintenance int64
	lastDiagnostic  int64
	calibrateNext   bool
}

// Start starts exactly one queue worker and returns an idempotent, bounded shutdown function.
func Start(state *core.AppState, configPath string) (func(), error) {
	if state == nil || state.GetDB() == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	state.Inner.ConfigWriteLock.Lock()
	cfg := state.Inner.Config.Load()
	if cfg.Mail.Enabled && cfg.Mail.EncryptionKey == "" {
		next := cfg.DeepCopy()
		if err := next.Mail.EnsureKey(); err != nil {
			state.Inner.ConfigWriteLock.Unlock()
			return nil, err
		}
		data, err := yaml.Marshal(next)
		if err == nil {
			err = utils.WritePrivateFile(configPath, data)
		}
		if err != nil {
			state.Inner.ConfigWriteLock.Unlock()
			return nil, err
		}
		state.Inner.Config.Store(next)
		cfg = next
	}
	state.Inner.ConfigWriteLock.Unlock()
	if cfg.Mail.Enabled {
		if err := cfg.Mail.Validate(); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	w := &worker{state: state, owner: rand.Text(), calibrationDue: map[string]int64{}}
	go func() {
		defer close(done)
		defer func() {
			if w.client != nil {
				w.client.Close()
			}
			_ = state.GetDB().ReleaseMailLease(w.owner)
		}()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if err := w.step(ctx, time.Now()); err != nil && !errors.Is(err, context.Canceled) {
				w.diagnostic(err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-state.Inner.MailWake:
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { cancel(); <-done }) }, nil
}

func (w *worker) configureClient(cfg *config.Config) error {
	encoded, err := json.Marshal(cfg.Proxy)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(encoded)
	if w.client != nil && hash == w.proxyHash {
		return nil
	}
	transport := &http.Transport{DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 20 * time.Second, MaxResponseHeaderBytes: 16 << 10, MaxIdleConns: 4, MaxIdleConnsPerHost: 1, MaxConnsPerHost: 1, IdleConnTimeout: time.Minute}
	proxy, err := outboundproxy.Selected(cfg.Proxy)
	if err != nil {
		return err
	}
	if proxy != nil {
		if err = outboundproxy.ConfigureTransport(transport, proxy); err != nil {
			return err
		}
	}
	if w.client != nil {
		w.client.Close()
	}
	w.client = mail.NewClient(transport)
	w.proxyHash = hash
	return nil
}

func (w *worker) step(ctx context.Context, now time.Time) error {
	cfg := w.state.Inner.Config.Load()
	db := w.state.GetDB()
	if !cfg.Mail.Enabled && w.pending == nil {
		return nil
	}
	control, owned, err := db.AcquireMailLease(w.owner, now.UnixMilli())
	if err != nil || !owned {
		return err
	}
	if w.pending != nil {
		return w.complete(cfg.Mail)
	}
	if !w.recovered {
		if err = db.RecoverMailAttempts(w.owner, now.UnixMilli()); err != nil {
			return err
		}
		w.recovered = true
	}
	if err = w.configureClient(cfg); err != nil {
		return err
	}
	if now.UnixMilli()-w.lastMaintenance >= int64(time.Minute/time.Millisecond) {
		ids := make([]string, 0, len(cfg.Mail.Accounts))
		for _, account := range cfg.Mail.Accounts {
			ids = append(ids, account.ID)
		}
		if err = db.CleanMailData(now.UnixMilli(), ids); err != nil {
			return err
		}
		w.lastMaintenance = now.UnixMilli()
	}
	if err = w.notifications(control, now); err != nil && !errors.Is(err, mail.ErrQueueFull) {
		return err
	}
	if w.calibrateNext {
		w.calibrateNext = false
		if worked, err := w.calibrateOne(ctx, cfg.Mail, now); worked || err != nil {
			return err
		}
	}
	job, err := db.NextMailJob(cfg.Mail.EncryptionKey, now.UnixMilli())
	if err != nil {
		return err
	}
	if job == nil {
		_, err = w.calibrateOne(ctx, cfg.Mail, now)
		return err
	}
	w.calibrateNext = true
	return w.process(ctx, cfg.Mail, control, job, now)
}

func (w *worker) calibrateOne(ctx context.Context, cfg mail.Config, now time.Time) (bool, error) {
	interval := cfg.Calibration.Duration()
	if interval == 0 {
		return false, nil
	}
	for id := range w.calibrationDue {
		if !slices.ContainsFunc(cfg.Accounts, func(a mail.Account) bool { return a.ID == id }) {
			delete(w.calibrationDue, id)
		}
	}
	for _, account := range cfg.Accounts {
		if !account.Enabled || !hasCalibration(account) || w.calibrationDue[account.ID] > now.UnixMilli() {
			continue
		}
		state, err := w.state.GetDB().LoadMailAccount(account.ID, cfg.EncryptionKey)
		if err != nil {
			return true, err
		}
		if state.CalibrationAt > 0 && now.UnixMilli()-state.CalibrationAt < interval.Milliseconds() {
			w.calibrationDue[account.ID] = state.CalibrationAt + interval.Milliseconds()
			continue
		}
		return true, w.calibrate(ctx, cfg, account, &state, now)
	}
	return false, nil
}

func hasCalibration(a mail.Account) bool {
	return slices.Contains([]string{"ses", "sendgrid", "aliyun"}, a.Provider) || a.Provider == "tencent" && a.FetchBalance
}

func (w *worker) calibrate(ctx context.Context, cfg mail.Config, a mail.Account, state *mail.AccountState, now time.Time) error {
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	budget, fetchErr := w.client.FetchBudget(requestCtx, a)
	state.Normalize(a, cfg.AccountRate, now)
	state.Calibrate(a, budget, now)
	state.CalibrationError = ""
	if fetchErr != nil {
		state.CalibrationError = "mail_calibration_failed"
	}
	w.calibrationDue[a.ID] = now.Add(cfg.Calibration.Duration()).UnixMilli()
	if err := w.state.GetDB().SaveMailAccount(w.owner, a.ID, cfg.EncryptionKey, *state, time.Now().UnixMilli()); err != nil {
		return err
	}
	if fetchErr != nil {
		w.log(nil, a.ID, "MAIL_CALIBRATION", "warning", "mail_calibration_failed", "")
	}
	return nil
}

func (w *worker) process(ctx context.Context, cfg mail.Config, control mail.Control, job *mail.Job, now time.Time) error {
	if now.UnixMilli() >= job.ExpiresAt {
		status, code := "expired", "mail_expired"
		if job.Status == "checking" {
			status, code = "unknown", "mail_status_unknown"
		}
		return w.finish(cfg, job, status, code, false, 0)
	}
	var account *mail.Account
	for i := range cfg.Accounts {
		if cfg.Accounts[i].ID == job.AccountID {
			account = &cfg.Accounts[i]
			break
		}
	}
	if account == nil {
		if job.Status == "checking" {
			return w.finish(cfg, job, "unknown", "mail_no_account", false, 0)
		}
		return w.finish(cfg, job, "cancelled", "mail_no_account", false, 0)
	}
	if !account.Enabled {
		if job.Status == "checking" {
			job.NextAt = now.Add(time.Minute).UnixMilli()
			job.UpdatedAt = now.UnixMilli()
			return w.state.GetDB().SaveMailAttempt(w.owner, cfg.EncryptionKey, job, nil, 0)
		}
		return w.pause(cfg, job, "mail_account_disabled", now.Add(time.Minute))
	}
	if job.Status != "checking" && !cfg.Allows(job.Message.To) {
		return w.finish(cfg, job, "cancelled", "mail_recipient_blocked", false, 0)
	}
	if job.Status != "checking" && job.UserID != "" {
		profile, err := w.state.GetDB().GetUserProfileByID(job.UserID)
		if err != nil {
			return err
		}
		if profile == nil {
			return w.finish(cfg, job, "cancelled", "mail_account_unavailable", false, 0)
		}
		token, err := w.state.GetDB().GetTokenByName(profile.Username)
		if err != nil {
			return err
		}
		if token == nil || token.DeletedAt > 0 {
			return w.finish(cfg, job, "cancelled", "mail_account_unavailable", false, 0)
		}
		if !slices.Contains([]string{"email_verify", "email_changed", "test"}, job.Scene) {
			security, err := w.state.GetDB().GetAccountSecurity(profile.Username)
			if err != nil {
				return err
			}
			if security == nil || !strings.EqualFold(security.Email, job.Message.To) {
				return w.finish(cfg, job, "cancelled", "mail_recipient_changed", false, 0)
			}
		}
	}
	if job.Status != "checking" && control.NextSendAt > now.UnixMilli() {
		job.NextAt = control.NextSendAt
		job.UpdatedAt = now.UnixMilli()
		return w.state.GetDB().SaveMailAttempt(w.owner, cfg.EncryptionKey, job, nil, 0)
	}
	state, err := w.state.GetDB().LoadMailAccount(account.ID, cfg.EncryptionKey)
	if err != nil {
		return err
	}
	state.Normalize(*account, cfg.AccountRate, now)
	if job.Status != "checking" && cfg.Calibration.Duration() > 0 && hasCalibration(*account) && state.CalibrationAt == 0 {
		return w.calibrate(ctx, cfg, *account, &state, now)
	}
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if job.Status != "checking" && state.Rate.Used >= cfg.AccountRate.Limit {
		return w.pause(cfg, job, "mail_account_rate_limited", time.UnixMilli(state.Rate.Start+cfg.AccountRate.Interval.Duration().Milliseconds()))
	}
	if slices.Contains([]string{"graph", "gmail", "feishu"}, account.Provider) || account.Provider == "smtp" && (account.AccessToken != "" || account.ClientID != "") {
		next, err := w.client.PrepareToken(requestCtx, *account, state.Token)
		if err != nil {
			return w.failPreparation(cfg, job, &state, "mail_oauth_refresh_failed")
		}
		if next != state.Token {
			state.Token = next
			if err = w.state.GetDB().SaveMailAccount(w.owner, account.ID, cfg.EncryptionKey, state, time.Now().UnixMilli()); err != nil {
				return w.failPreparation(cfg, job, &state, "mail_oauth_state_unavailable")
			}
		}
		copy := *account
		copy.AccessToken = next.Access
		account = &copy
	}
	if job.Status == "checking" {
		result, err := w.client.Check(requestCtx, *account, job.Message, job.Result)
		job.Checks++
		if err != nil {
			job.Result.Code = "mail_status_lookup_failed"
			w.log(job, account.ID, "MAIL_STATUS", "warning", errorCode(err), "")
			if job.Checks >= 8 {
				return w.finish(cfg, job, "unknown", "mail_status_unknown", false, 0)
			}
		} else {
			job.Result = result
			if result.Terminal() {
				return w.finish(cfg, job, result.Status, result.Code, result.NotCharged, 0)
			}
		}
		job.UpdatedAt = time.Now().UnixMilli()
		job.NextAt = time.Now().Add(time.Duration(min(5*(1<<min(job.Checks, 6)), 300)) * time.Second).UnixMilli()
		return w.state.GetDB().SaveMailAttempt(w.owner, cfg.EncryptionKey, job, nil, 0)
	}
	reservation, reason := state.Reserve(*account, cfg.AccountRate, now)
	if reason != "" {
		until := now.Add(time.Minute)
		if reason == "mail_account_rate_limited" {
			until = time.UnixMilli(state.Rate.Start + cfg.AccountRate.Interval.Duration().Milliseconds())
		}
		return w.pause(cfg, job, reason, until)
	}
	job.Reservation = reservation
	job.Status = "sending"
	job.UpdatedAt = time.Now().UnixMilli()
	job.NextAt = 0
	if err = w.state.GetDB().SaveMailAttempt(w.owner, cfg.EncryptionKey, job, &state, time.Now().Add(30*time.Second+cfg.Delay.Duration()).UnixMilli()); err != nil {
		return err
	}
	result, sendErr := w.client.Send(requestCtx, *account, job.Message)
	job.Result = result
	job.UpdatedAt = time.Now().UnixMilli()
	nextSendAt := time.Now().Add(cfg.Delay.Duration()).UnixMilli()
	if sendErr != nil {
		job.Status = "failed"
		if failure, ok := errors.AsType[*mail.SendError](sendErr); ok && failure.Uncertain {
			job.Status = "unknown"
		}
		job.Result = mail.Result{Status: job.Status, Code: errorCode(sendErr)}
	} else if result.Check {
		job.Status = "checking"
		job.NextAt = time.Now().Add(5 * time.Second).UnixMilli()
	} else {
		job.Status = result.Status
	}
	w.pending = &completion{job: job, refund: sendErr != nil && !mail.Chargeable(sendErr), reservation: job.Reservation, nextSendAt: nextSendAt}
	return w.complete(cfg)
}

func (w *worker) failPreparation(cfg mail.Config, job *mail.Job, state *mail.AccountState, code string) error {
	next := int64(0)
	if job.Status == "checking" {
		job.Status = "unknown"
	} else {
		state.Rate.Used++
		state.Attempts++
		job.Status = "failed"
		next = time.Now().Add(cfg.Delay.Duration()).UnixMilli()
	}
	job.Result.Status, job.Result.Code = job.Status, code
	// Retain rotated credentials until the account and final job state commit together.
	w.pending = &completion{job: job, account: state, nextSendAt: next}
	return w.complete(cfg)
}

func (w *worker) pause(cfg mail.Config, job *mail.Job, code string, until time.Time) error {
	changed := job.Result.Code != code
	job.Status = "paused"
	job.Result = mail.Result{Status: "paused", Code: code}
	job.UpdatedAt = time.Now().UnixMilli()
	job.NextAt = until.UnixMilli()
	if err := w.state.GetDB().SaveMailAttempt(w.owner, cfg.EncryptionKey, job, nil, 0); err != nil {
		return err
	}
	if changed {
		w.log(job, job.AccountID, "MAIL_PAUSED", "warning", code, "")
	}
	return nil
}

func (w *worker) finish(cfg mail.Config, job *mail.Job, status, code string, refund bool, next int64) error {
	job.Status = status
	job.Result.Status = status
	job.Result.Code = code
	job.Result.Check = false
	w.pending = &completion{job: job, refund: refund, reservation: job.Reservation, nextSendAt: next}
	return w.complete(cfg)
}

func (w *worker) complete(cfg mail.Config) error {
	pending := w.pending
	if pending == nil {
		return nil
	}
	job := pending.job
	state := pending.account
	if pending.refund {
		current, err := w.state.GetDB().LoadMailAccount(job.AccountID, cfg.EncryptionKey)
		if err != nil {
			return err
		}
		current.Refund(pending.reservation)
		state = &current
	}
	if job.Status != "checking" {
		job.Message.HTML = ""
		job.Message.Text = ""
		job.Reservation = mail.Reservation{}
		job.NextAt = 0
	}
	job.UpdatedAt = time.Now().UnixMilli()
	if err := w.state.GetDB().SaveMailAttempt(w.owner, cfg.EncryptionKey, job, state, pending.nextSendAt); err != nil {
		if errors.Is(err, mail.ErrFinalized) {
			w.pending = nil
			return nil
		}
		return err
	}
	severity := "info"
	if job.Status == "failed" {
		severity = "error"
	} else if slices.Contains([]string{"unknown", "expired", "cancelled"}, job.Status) {
		severity = "warning"
	}
	w.log(job, job.AccountID, "MAIL_STATUS", severity, job.Result.Code, job.Status)
	w.pending = nil
	return nil
}

func errorCode(err error) string {
	var failure *mail.SendError
	if errors.As(err, &failure) {
		return failure.Code
	}
	return "mail_provider_error"
}

func (w *worker) log(job *mail.Job, account, action, severity, code, status string) {
	username, actor, id := "", "system", ""
	if job != nil {
		actor = job.Actor
		id = job.ID
		if job.UserID != "" {
			if profile, err := w.state.GetDB().GetUserProfileByID(job.UserID); err == nil && profile != nil {
				username = profile.Username
			}
		}
	}
	audit.Log(w.state, &core.AuditLogEntry{Kind: "system", Action: action, Operator: "system", Initiator: actor, Username: username, Trigger: "system", Severity: severity, Details: fmt.Sprintf("Email job=%s account=%s status=%s code=%s", id, account, status, code)})
}

func (w *worker) diagnostic(err error) {
	if time.Now().UnixMilli()-w.lastDiagnostic < int64(time.Minute/time.Millisecond) {
		return
	}
	w.lastDiagnostic = time.Now().UnixMilli()
	w.state.Inner.FailuresCount.Add(1)
	w.log(nil, "", "MAIL_WORKER", "error", errorCode(err), "")
}
