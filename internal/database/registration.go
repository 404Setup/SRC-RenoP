/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"golang.org/x/crypto/bcrypt"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/mail"
)

const registrationColumns = "id_hash, ip_hash, provider_key, email, code_hash, profile_json, attempts, created_at, expires_at, cooldown_until"

func registrationProviderProfile(p *core.PendingRegistration) (*core.RegistrationProfile, error) {
	profile := &core.RegistrationProfile{}
	if json.Unmarshal([]byte(p.ProfileJSON), profile) != nil {
		return nil, core.ErrRegistrationInvalid
	}
	if profile.OAuth != nil {
		if !profile.OAuth.Valid() || profile.GitHubID != 0 || p.ProviderKey != profile.OAuth.ProviderID+":"+profile.OAuth.Key() {
			return nil, core.ErrRegistrationInvalid
		}
	} else if profile.GitHubID <= 0 || p.ProviderKey != "github:"+strconv.FormatInt(profile.GitHubID, 10) {
		return nil, core.ErrRegistrationInvalid
	}
	return profile, nil
}

func cleanRegistrationsTx(tx *Tx, now int64) error {
	if _, err := tx.Exec(`DELETE FROM account_registrations WHERE cooldown_until <= ?`, now); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE account_registrations SET email = '', code_hash = '', profile_json = '', ip_hash = ''
		WHERE expires_at <= ? AND profile_json <> ''`, now)
	return err
}

// CleanRegistrations removes expired confirmation data while retaining only the provider's cooldown marker.
func (db *DB) CleanRegistrations(now int64) error {
	db.mailWriteMu.Lock()
	defer db.mailWriteMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockMailTx(tx); err != nil {
		return err
	}
	if err = cleanRegistrationsTx(tx, now); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM registration_ips WHERE expires_at <= ?`, now); err != nil {
		return err
	}
	return tx.Commit()
}

func scanRegistration(row row) (*core.PendingRegistration, error) {
	p := &core.PendingRegistration{}
	err := row.Scan(&p.IDHash, &p.IPHash, &p.ProviderKey, &p.Email, &p.CodeHash, &p.ProfileJSON,
		&p.Attempts, &p.CreatedAt, &p.ExpiresAt, &p.CooldownUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrRegistrationInvalid
	}
	return p, err
}

// GetPendingRegistration returns only a live confirmation addressed by its browser capability.
func (db *DB) GetPendingRegistration(idHash string, now int64) (*core.PendingRegistration, error) {
	if !validSelectorHash(idHash) {
		return nil, core.ErrRegistrationInvalid
	}
	p, err := scanRegistration(db.QueryRow(`SELECT `+registrationColumns+` FROM account_registrations WHERE id_hash = ?`, idHash))
	if err == nil && (p.ExpiresAt <= now || p.Attempts >= 5) {
		return nil, core.ErrRegistrationInvalid
	}
	return p, err
}

// CheckRegistrationIP rejects spent IP allowances before expensive password hashing.
// RegisterAccount rechecks the same limit under the write lock before creating an account.
func (db *DB) CheckRegistrationIP(ipHash string, cfg config.RegistrationConfig, now int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return registrationRateTx(tx, ipHash, cfg, now, false)
}

func registrationRateTx(tx *Tx, ipHash string, cfg config.RegistrationConfig, now int64, consume bool) error {
	if !cfg.Enabled {
		return core.ErrRegistrationDisabled
	}
	if cfg.Validate() != nil || !validSelectorHash(ipHash) || now <= 0 {
		return core.ErrRegistrationInvalid
	}
	var used, expires int64
	err := tx.QueryRow(`SELECT used, expires_at FROM registration_ips WHERE ip_hash = ?`, ipHash).Scan(&used, &expires)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if expires > now && used >= cfg.IPLimit {
		return core.ErrRegistrationRateLimited
	}
	if !consume {
		return nil
	}
	if expires > now {
		_, err = tx.Exec(`UPDATE registration_ips SET used = used + 1 WHERE ip_hash = ?`, ipHash)
		return err
	}
	if _, err = tx.Exec(`DELETE FROM registration_ips WHERE expires_at <= ?`, now); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM registration_ips`).Scan(&count); err != nil {
		return err
	}
	if count >= 100000 {
		return mail.ErrQueueFull
	}
	_, err = tx.Exec(`INSERT INTO registration_ips (ip_hash, used, period_start, expires_at) VALUES (?, 1, ?, ?)`,
		ipHash, now, now+cfg.IPInterval.Duration().Milliseconds())
	return err
}

// BeginRegistration atomically queues verification and persists a bounded confirmation with no account privileges.
func (db *DB) BeginRegistration(p *core.PendingRegistration, job *mail.Job, key, ip string,
	rate mail.Rate, cfg config.RegistrationConfig) error {
	if p == nil || !validSelectorHash(p.IDHash) || !validSelectorHash(p.IPHash) ||
		p.CreatedAt <= 0 || p.ExpiresAt <= p.CreatedAt || p.ExpiresAt-p.CreatedAt > (10*time.Minute).Milliseconds() ||
		len(p.ProfileJSON) > 256<<10 || len(p.ProviderKey) > 128 || p.Attempts != 0 {
		return core.ErrRegistrationInvalid
	}
	email, valid := core.NormalizeEmail(p.Email)
	if !valid || email != p.Email {
		return core.ErrRegistrationInvalid
	}
	if p.ProviderKey == "" {
		if email == "" || !validSelectorHash(p.CodeHash) || job == nil || job.Scene != "registration_verify" || job.Message.To != email ||
			job.CreatedAt != p.CreatedAt || job.ExpiresAt != p.ExpiresAt || p.CooldownUntil != p.ExpiresAt {
			return core.ErrRegistrationInvalid
		}
	} else if job != nil || p.CodeHash != "" || p.CooldownUntil != p.ExpiresAt+cfg.ProviderCooldown.Duration().Milliseconds() {
		return core.ErrRegistrationInvalid
	} else {
		profile, err := registrationProviderProfile(p)
		if err != nil || (email == "" && (profile.OAuth == nil || profile.EmailVerified)) {
			return core.ErrRegistrationInvalid
		}
	}
	db.mailWriteMu.Lock()
	defer db.mailWriteMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockMailTx(tx); err != nil {
		return err
	}
	if err = registrationRateTx(tx, p.IPHash, cfg, p.CreatedAt, false); err != nil {
		return err
	}
	if err = cleanRegistrationsTx(tx, p.CreatedAt); err != nil {
		return err
	}
	if p.ProviderKey != "" {
		var expires int64
		err = tx.QueryRow(`SELECT expires_at FROM account_registrations WHERE provider_key = ?`, p.ProviderKey).Scan(&expires)
		if err == nil {
			if expires > p.CreatedAt {
				return core.ErrRegistrationPending
			}
			return core.ErrRegistrationCooldown
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	} else {
		var latest int64
		if err = tx.QueryRow(`SELECT COALESCE(MAX(created_at), 0) FROM account_registrations WHERE email = ?`, email).Scan(&latest); err != nil {
			return err
		}
		if latest+time.Minute.Milliseconds() > p.CreatedAt {
			return mail.ErrRateLimited
		}
		// A resend may replace its own email challenge, never a provider identity.
		if _, err = tx.Exec(`DELETE FROM account_registrations WHERE id_hash = ? AND provider_key = ''`, p.IDHash); err != nil {
			return err
		}
	}
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM account_registrations`).Scan(&count); err != nil {
		return err
	}
	if count >= mail.MaxPendingJobs {
		return mail.ErrQueueFull
	}
	if job != nil {
		created, err := queueMailJobTx(tx, job, key, ip, rate)
		if err != nil {
			return err
		}
		if !created {
			return core.ErrRegistrationInvalid
		}
	}
	_, err = tx.Exec(`INSERT INTO account_registrations (`+registrationColumns+`) VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
		p.IDHash, p.IPHash, p.ProviderKey, email, p.CodeHash, p.ProfileJSON, p.CreatedAt, p.ExpiresAt, p.CooldownUntil)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// QueueProviderRegistrationEmail attaches verification to the original provider confirmation without extending its lifetime.
func (db *DB) QueueProviderRegistrationEmail(idHash, ipHash, codeHash string, job *mail.Job, key, ip string,
	rate mail.Rate, cfg config.RegistrationConfig) error {
	if !validSelectorHash(idHash) || !validSelectorHash(ipHash) || !validSelectorHash(codeHash) || job == nil ||
		job.Scene != "registration_verify" {
		return core.ErrRegistrationInvalid
	}
	email, valid := core.NormalizeEmail(job.Message.To)
	if !valid || email == "" || email != job.Message.To {
		return core.ErrRegistrationInvalid
	}
	db.mailWriteMu.Lock()
	defer db.mailWriteMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockMailTx(tx); err != nil {
		return err
	}
	p, err := scanRegistration(tx.QueryRow(`SELECT `+registrationColumns+` FROM account_registrations WHERE id_hash = ?`, idHash))
	if err != nil {
		return err
	}
	if p.IPHash != ipHash || p.Attempts >= 5 || p.ExpiresAt <= job.CreatedAt || job.ExpiresAt != p.ExpiresAt {
		return core.ErrRegistrationInvalid
	}
	profile, err := registrationProviderProfile(p)
	if err != nil || profile.OAuth == nil || profile.EmailVerified {
		return core.ErrRegistrationInvalid
	}
	if err = registrationRateTx(tx, ipHash, cfg, job.CreatedAt, false); err != nil {
		return err
	}
	created, err := queueMailJobTx(tx, job, key, ip, rate)
	if err != nil {
		return err
	}
	if !created {
		return core.ErrRegistrationInvalid
	}
	if _, err = tx.Exec(`UPDATE account_registrations SET email = ?, code_hash = ? WHERE id_hash = ?`, email, codeHash, idHash); err != nil {
		return err
	}
	return tx.Commit()
}

// RegisterAccount consumes confirmation, creates credentials and identity, and charges the IP in one transaction.
func (db *DB) RegisterAccount(request core.AccountRegistration, cfg config.RegistrationConfig, now int64) (*core.RegistrationProfile, error) {
	username, valid := core.NormalizeUsername(request.Username)
	nickname, validNickname := core.NormalizeNickname(request.Nickname)
	email, validEmail := core.NormalizeEmail(request.Email)
	if !valid || !validNickname || !validEmail {
		return nil, core.ErrRegistrationInvalid
	}
	if _, err := bcrypt.Cost([]byte(request.PasswordHash)); err != nil {
		return nil, core.ErrRegistrationInvalid
	}
	db.mailWriteMu.Lock()
	defer db.mailWriteMu.Unlock()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = lockMailTx(tx); err != nil {
		return nil, err
	}
	if err = registrationRateTx(tx, request.IPHash, cfg, now, false); err != nil {
		return nil, err
	}
	profile := &core.RegistrationProfile{Username: username, Nickname: nickname}
	if request.IDHash != "" {
		p, err := scanRegistration(tx.QueryRow(`SELECT `+registrationColumns+` FROM account_registrations WHERE id_hash = ?`, request.IDHash))
		if err != nil {
			return nil, err
		}
		if p.ExpiresAt <= now || p.Attempts >= 5 || p.IPHash != request.IPHash || p.Email != email {
			return nil, core.ErrRegistrationInvalid
		}
		provider, _, _ := strings.Cut(p.ProviderKey, ":")
		if provider != request.Provider {
			return nil, core.ErrRegistrationInvalid
		}
		if p.ProviderKey != "" {
			profile, err = registrationProviderProfile(p)
			if err != nil {
				return nil, err
			}
		}
		if p.ProviderKey == "" || profile.OAuth != nil && !profile.EmailVerified {
			if email == "" || !validSelectorHash(p.CodeHash) || !validSelectorHash(request.CodeHash) || subtle.ConstantTimeCompare([]byte(p.CodeHash), []byte(request.CodeHash)) != 1 {
				if _, err = tx.Exec(`UPDATE account_registrations SET attempts = attempts + 1 WHERE id_hash = ?`, p.IDHash); err != nil {
					return nil, err
				}
				if err = tx.Commit(); err != nil {
					return nil, err
				}
				return nil, core.ErrRegistrationInvalid
			}
		}
	} else if request.RequireEmail || request.Provider != "" {
		return nil, core.ErrRegistrationInvalid
	}
	token := &core.AccessToken{Name: username, Identifier: core.AccessTokenIdentifier{Type: core.Persistent},
		EncryptedSecret: request.PasswordHash, Tokens: []string{}, Permissions: []string{"base"},
		CreatedAt: time.UnixMilli(now).UTC().Format(time.RFC3339), Description: "Self-service registration"}
	if err = createTokenTx(tx, token, nickname, now); err != nil {
		return nil, err
	}
	userID, err := userIDForUsernameTx(tx, username)
	if err != nil {
		return nil, err
	}
	if err = updateAccountEmailTx(tx, userID, email, now); err != nil {
		return nil, err
	}
	if profile.GitHubID > 0 {
		if err = storeGitHubIdentityTx(tx, userID, profile.GitHubID, profile.GitHubLogin, profile.Principals, now); err != nil {
			return nil, err
		}
	}
	if profile.OAuth != nil {
		if err = storeOAuthIdentityTx(tx, userID, *profile.OAuth, now); err != nil {
			return nil, err
		}
	}
	if err = registrationRateTx(tx, request.IPHash, cfg, now, true); err != nil {
		return nil, err
	}
	if request.IDHash != "" {
		if _, err = tx.Exec(`DELETE FROM account_registrations WHERE id_hash = ?`, request.IDHash); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	db.finishTokenUpdate(username, token)
	profile.Username, profile.Nickname = username, nickname
	return profile, nil
}
