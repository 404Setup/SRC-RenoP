/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/emmansun/base64"

	"renop/internal/core"
	"renop/internal/utils"
	"renop/internal/utils/secretcipher"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/gofiber/fiber/v3"
	"go.yaml.in/yaml/v3"
)

const mfaCookieName = "renop_mfa"
const mfaTTL = 5 * time.Minute

var errMFARequired = errors.New("second-factor verification required")
var errMFAPrimaryRequired = errors.New("a primary login method is required")

type mfaChallenge struct {
	state                                                 *core.AppState
	credentialID                                          []byte
	username, method, snapshot, revision, session, secret string
	createdAt                                             time.Time
	fido                                                  *webauthn.SessionData
	attempts                                              int
}

var mfaChallenges = struct {
	sync.Mutex
	entries map[string]*mfaChallenge
}{entries: make(map[string]*mfaChallenge)}

// EnsureMFAKey persists the server-owned authenticator encryption key before accepting requests.
func EnsureMFAKey(state *core.AppState, path string) error {
	state.Inner.ConfigWriteLock.Lock()
	defer state.Inner.ConfigWriteLock.Unlock()
	cfg := state.Inner.Config.Load()
	if cfg.MFAEncryptionKey != "" {
		_, err := secretcipher.New(cfg.MFAEncryptionKey)
		return err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	next := cfg.DeepCopy()
	next.MFAEncryptionKey = base64.RawStdEncoding.EncodeToString(key)
	data, err := yaml.Marshal(next)
	if err != nil {
		return err
	}
	if err := utils.WritePrivateFile(path, data); err != nil {
		return err
	}
	state.Inner.Config.Store(next)
	return nil
}

func encryptMFASecret(state *core.AppState, userID, secret string) (string, error) {
	aead, err := secretcipher.New(state.Inner.Config.Load().MFAEncryptionKey)
	if err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(aead.Seal(nil, nil, []byte(secret), []byte("totp:"+userID))), nil
}

func decryptMFASecret(state *core.AppState, mfa *core.MFAState) (string, error) {
	if len(mfa.Secret) > 1024 {
		return "", core.ErrMFAInvalid
	}
	aead, err := secretcipher.New(state.Inner.Config.Load().MFAEncryptionKey)
	if err != nil {
		return "", err
	}
	sealed, err := base64.RawStdEncoding.DecodeString(mfa.Secret)
	if err != nil {
		return "", err
	}
	value, err := aead.Open(nil, nil, sealed, []byte("totp:"+mfa.UserID))
	return string(value), err
}

func storeMFAChallenge(challenge *mfaChallenge) (string, error) {
	mfaChallenges.Lock()
	defer mfaChallenges.Unlock()
	now := time.Now()
	var oldestID string
	var oldest time.Time
	count := 0
	for key, entry := range mfaChallenges.entries {
		if now.Sub(entry.createdAt) >= mfaTTL {
			delete(mfaChallenges.entries, key)
			continue
		}
		if entry.state == challenge.state && entry.username == challenge.username {
			count++
			if oldestID == "" || entry.createdAt.Before(oldest) {
				oldestID, oldest = key, entry.createdAt
			}
		}
	}
	if count >= 8 {
		delete(mfaChallenges.entries, oldestID)
	}
	if len(mfaChallenges.entries) >= 4096 {
		return "", errors.New("second-factor challenge capacity reached")
	}
	id := rand.Text()
	challenge.username = strings.Clone(challenge.username)
	challenge.session = strings.Clone(challenge.session)
	challenge.createdAt = now
	mfaChallenges.entries[id] = challenge
	return id, nil
}

// PruneExpiredMFAChallenges releases expired login and authenticator setup material.
func PruneExpiredMFAChallenges(now time.Time) {
	mfaChallenges.Lock()
	defer mfaChallenges.Unlock()
	for id, entry := range mfaChallenges.entries {
		if now.Sub(entry.createdAt) >= mfaTTL {
			delete(mfaChallenges.entries, id)
		}
	}
}

func mfaCookie(c fiber.Ctx, value string) {
	age := int(mfaTTL / time.Second)
	if value == "" {
		age = -1
	}
	c.Cookie(&fiber.Cookie{Name: mfaCookieName, Value: value, Path: "/api/auth/mfa", MaxAge: age,
		HTTPOnly: true, Secure: isSecure(c), SameSite: "Strict"})
}

func loadMFAChallenge(c fiber.Ctx, state *core.AppState, consume bool) (*mfaChallenge, error) {
	id := c.Cookies(mfaCookieName)
	mfaChallenges.Lock()
	defer mfaChallenges.Unlock()
	entry := mfaChallenges.entries[id]
	if entry == nil || entry.state != state || entry.method == "setup" || time.Since(entry.createdAt) >= mfaTTL {
		delete(mfaChallenges.entries, id)
		return nil, core.ErrMFAInvalid
	}
	if consume {
		delete(mfaChallenges.entries, id)
	}
	copy := *entry
	return &copy, nil
}

func prepareBrowserLogin(c fiber.Ctx, state *core.AppState, username, method, expectedSnapshot string) (string, error) {
	mfa, err := state.GetDB().GetMFAState(username)
	if err != nil {
		return "", err
	}
	if expectedSnapshot != "" && mfa.Snapshot != expectedSnapshot {
		return "", core.ErrMFAInvalid
	}
	if proof, ok := c.Locals("verified_mfa").(*mfaChallenge); ok {
		if proof.state != state || proof.username != username || proof.snapshot != mfa.Snapshot {
			return "", core.ErrMFAInvalid
		}
		return mfa.Snapshot, nil
	}
	if method == "fido" && mfa.Passkey {
		return "", errMFAPrimaryRequired
	}
	if !mfa.Enabled() {
		return mfa.Snapshot, nil
	}
	credentialID, _ := c.Locals("verified_fido_credential").([]byte)
	id, err := storeMFAChallenge(&mfaChallenge{state: state, username: username, method: method,
		snapshot: mfa.Snapshot, revision: mfa.Revision, credentialID: bytes.Clone(credentialID)})
	if err != nil {
		return "", err
	}
	mfaCookie(c, id)
	return "", errMFARequired
}

func mfaError(c fiber.Ctx, err error) error {
	setPrivateResponseHeaders(c)
	status, code := 500, "MFA_UNAVAILABLE"
	switch {
	case errors.Is(err, errMFARequired):
		status, code = 409, "MFA_REQUIRED"
	case errors.Is(err, errMFAPrimaryRequired):
		status, code = 409, "MFA_PRIMARY_REQUIRED"
	case errors.Is(err, core.ErrMFAInvalid):
		status, code = 400, "MFA_INVALID"
	case errors.Is(err, core.ErrLastLoginMethod):
		status, code = 409, "ACCOUNT_LAST_LOGIN_METHOD"
	case errors.Is(err, fiber.ErrForbidden):
		status, code = 403, "MFA_REAUTH_REQUIRED"
	case errors.Is(err, fiber.ErrUnauthorized):
		status, code = 401, "MFA_REAUTH_REQUIRED"
	}
	if accountCode := accountAccessCode(err); accountCode != "" {
		status, code = 403, accountCode
	}
	c.Set("X-Renop-Error-Code", code)
	return c.Status(status).JSON(fiber.Map{"error": code})
}
