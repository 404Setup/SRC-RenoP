/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package mail

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"

	"github.com/emmansun/base64"

	"github.com/goccy/go-json"
)

// MaxPendingJobs bounds queued, paused, sending, and status-checking messages together.
const MaxPendingJobs = 2048

var (
	ErrQueueFull   = errors.New("mail_queue_full")
	ErrRateLimited = errors.New("mail_manual_rate_limited")
	ErrLeaseLost   = errors.New("mail_worker_lease_lost")
	ErrFinalized   = errors.New("mail_job_finalized")
)

// Job is a durable send or status lookup. Payload and ticket verifiers stay private.
type Job struct {
	ID          string      `json:"id"`
	AccountID   string      `json:"account_id"`
	UserID      string      `json:"user_id"`
	Actor       string      `json:"actor"`
	Scene       string      `json:"scene"`
	Status      string      `json:"status"`
	CreatedAt   int64       `json:"created_at"`
	UpdatedAt   int64       `json:"updated_at"`
	NextAt      int64       `json:"next_at"`
	ExpiresAt   int64       `json:"expires_at"`
	Checks      int         `json:"checks"`
	Message     Message     `json:"-"`
	Result      Result      `json:"result"`
	Reservation Reservation `json:"-"`
	TicketHash  string      `json:"-"`
}

// JobPayload contains encrypted content and per-attempt accounting metadata.
type JobPayload struct {
	Message     Message     `json:"message"`
	Reservation Reservation `json:"reservation"`
	TicketHash  string      `json:"ticket_hash"`
}

// Control owns the process-independent lease, global send delay, and audit notification cursor.
type Control struct {
	Owner        string
	LeaseUntil   int64
	NextSendAt   int64
	AuditCursor  int64
	EnabledSince int64
}

// EnsureKey creates the server-owned queue encryption key on first configuration.
func (c *Config) EnsureKey() error {
	if c.EncryptionKey == "" {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return err
		}
		c.EncryptionKey = base64.RawStdEncoding.EncodeToString(key)
	}
	_, err := queueCipher(c.EncryptionKey)
	return err
}

func queueCipher(key string) (cipher.AEAD, error) {
	raw, err := base64.RawStdEncoding.DecodeString(key)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("invalid mail encryption key")
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}

// Seal encrypts durable mail content and binds it to its table row identity.
func Seal(key, purpose string, value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(data) > 2*MaxBodyBytes {
		return "", errors.New("mail payload too large")
	}
	aead, err := queueCipher(key)
	if err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(aead.Seal(nil, nil, data, []byte(purpose))), nil
}

// Open verifies and decodes a bounded durable mail payload.
func Open(key, purpose, encoded string, value any) error {
	if len(encoded) > 4*MaxBodyBytes {
		return errors.New("mail payload too large")
	}
	aead, err := queueCipher(key)
	if err != nil {
		return err
	}
	sealed, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return errors.New("invalid mail payload")
	}
	data, err := aead.Open(nil, nil, sealed, []byte(purpose))
	if err != nil {
		return errors.New("invalid mail payload")
	}
	return json.Unmarshal(data, value)
}
