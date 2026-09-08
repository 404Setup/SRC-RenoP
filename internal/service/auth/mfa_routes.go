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
	"encoding/base32"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	qrcode "github.com/skip2/go-qrcode"
	"renop/internal/core"
	"renop/internal/service/audit"
)

func setupMFARoutes(auth fiber.Router, state *core.AppState) {
	auth.Get("/mfa", func(c fiber.Ctx) error { return getMFAChallenge(c, state) })
	auth.Delete("/mfa", func(c fiber.Ctx) error {
		_, _ = loadMFAChallenge(c, state, true)
		return c.SendStatus(204)
	})
	auth.Post("/mfa/totp", func(c fiber.Ctx) error { return verifyMFATOTP(c, state) })
	auth.Post("/mfa/passkey/begin", func(c fiber.Ctx) error { return beginMFAPasskey(c, state) })
	auth.Post("/mfa/passkey/finish", func(c fiber.Ctx) error { return finishMFAPasskey(c, state) })
	auth.Post("/profile/mfa/totp/begin", func(c fiber.Ctx) error { return beginTOTPSetup(c, state) })
	auth.Post("/profile/mfa/totp/confirm", func(c fiber.Ctx) error { return confirmTOTPSetup(c, state) })
	auth.Put("/profile/mfa", func(c fiber.Ctx) error { return updateMFASettings(c, state) })
}

func readMFARequest(c fiber.Ctx, request any) error {
	err := readPasswordResetRequest(c, request)
	if err != nil && !errors.Is(err, fiber.ErrRequestEntityTooLarge) && !errors.Is(err, fiber.ErrUnsupportedMediaType) {
		return fiber.ErrBadRequest
	}
	return err
}

func currentMFAChallenge(c fiber.Ctx, state *core.AppState) (*mfaChallenge, *core.MFAState, error) {
	challenge, err := loadMFAChallenge(c, state, false)
	if err != nil {
		return nil, nil, err
	}
	account := state.GetTokenByName(challenge.username)
	if account == nil {
		return nil, nil, core.ErrMFAInvalid
	}
	if err := accountAccessError(account); err != nil {
		return nil, nil, err
	}
	mfa, err := state.GetDB().GetMFAState(challenge.username)
	if err != nil {
		return nil, nil, err
	}
	if !mfa.Enabled() || mfa.Snapshot != challenge.snapshot {
		return nil, nil, core.ErrMFAInvalid
	}
	return challenge, mfa, nil
}

func getMFAChallenge(c fiber.Ctx, state *core.AppState) error {
	challenge, mfa, err := currentMFAChallenge(c, state)
	if err != nil {
		return mfaError(c, err)
	}
	setPrivateResponseHeaders(c)
	return c.JSON(fiber.Map{"totp": mfa.Secret != "", "passkey": mfa.Passkey && challenge.method != "fido",
		"expires_at": challenge.createdAt.Add(mfaTTL).UnixMilli()})
}

func completeMFALogin(c fiber.Ctx, state *core.AppState, challenge *mfaChallenge, factor string) error {
	consumed, err := loadMFAChallenge(c, state, true)
	if err != nil || consumed.snapshot != challenge.snapshot {
		return mfaError(c, core.ErrMFAInvalid)
	}
	mfaCookie(c, "")
	account := state.GetTokenByName(challenge.username)
	if account == nil {
		return mfaError(c, core.ErrMFAInvalid)
	}
	c.Locals("verified_mfa", challenge)
	c.Locals("verified_fido_credential", challenge.credentialID)
	user := buildSynthUser(account)
	if err := issueBrowserSession(c, state, user, challenge.method+"+"+factor); err != nil {
		return mfaError(c, err)
	}
	setPrivateResponseHeaders(c)
	return c.JSON(CreateSessionDetails(user, ""))
}

func verifyMFATOTP(c fiber.Ctx, state *core.AppState) error {
	var request struct {
		Code string `json:"code"`
	}
	if err := readMFARequest(c, &request); err != nil {
		return err
	}
	challenge, mfa, err := currentMFAChallenge(c, state)
	if err != nil {
		return mfaError(c, err)
	}
	if mfa.Secret == "" {
		return mfaError(c, core.ErrMFAInvalid)
	}
	secret, err := decryptMFASecret(state, mfa)
	if err != nil {
		return mfaError(c, err)
	}
	now := time.Now()
	step := core.VerifyTOTP(secret, request.Code, now)
	if err := state.GetDB().ConsumeMFACode(challenge.username, mfa.Revision, step, now.UnixMilli()); err != nil {
		return mfaError(c, err)
	}
	return completeMFALogin(c, state, challenge, "totp")
}

func beginMFAPasskey(c fiber.Ctx, state *core.AppState) error {
	var request struct{}
	if err := readMFARequest(c, &request); err != nil {
		return err
	}
	challenge, mfa, err := currentMFAChallenge(c, state)
	if err != nil {
		return mfaError(c, err)
	}
	if !mfa.Passkey || challenge.method == "fido" {
		return mfaError(c, core.ErrMFAInvalid)
	}
	w, err := getWebAuthnEngine(c, state)
	if err != nil {
		return mfaError(c, err)
	}
	options, data, err := w.BeginLogin(buildFidoUser(challenge.username, state), webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return mfaError(c, err)
	}
	mfaChallenges.Lock()
	entry := mfaChallenges.entries[c.Cookies(mfaCookieName)]
	if entry != nil && entry.state == state {
		entry.fido = data
	}
	mfaChallenges.Unlock()
	if entry == nil {
		return mfaError(c, core.ErrMFAInvalid)
	}
	setPrivateResponseHeaders(c)
	return c.JSON(fiber.Map{"options": options})
}

func finishMFAPasskey(c fiber.Ctx, state *core.AppState) error {
	var request struct {
		Credential json.RawMessage `json:"credential"`
	}
	if err := readMFARequest(c, &request); err != nil {
		return err
	}
	challenge, mfa, err := currentMFAChallenge(c, state)
	if err != nil {
		return mfaError(c, err)
	}
	if !mfa.Passkey || challenge.fido == nil || challenge.method == "fido" {
		return mfaError(c, core.ErrMFAInvalid)
	}
	mfaChallenges.Lock()
	entry := mfaChallenges.entries[c.Cookies(mfaCookieName)]
	claimed := entry != nil && entry.fido == challenge.fido
	if claimed {
		entry.fido = nil
	}
	mfaChallenges.Unlock()
	if !claimed {
		return mfaError(c, core.ErrMFAInvalid)
	}
	w, err := getWebAuthnEngine(c, state)
	if err != nil {
		return mfaError(c, err)
	}
	httpRequest, err := http.NewRequest(http.MethodPost, "", bytes.NewReader(request.Credential))
	if err != nil {
		return mfaError(c, core.ErrMFAInvalid)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	parsed, err := protocol.ParseCredentialRequestResponse(httpRequest)
	if err != nil {
		return mfaError(c, core.ErrMFAInvalid)
	}
	credential, err := w.ValidateLogin(buildFidoAssertionUser(challenge.username, state, parsed), *challenge.fido, parsed)
	if err != nil || credential == nil || credential.Authenticator.CloneWarning {
		return mfaError(c, core.ErrMFAInvalid)
	}
	if err := state.UpdateFidoDeviceState(credential.ID, credential.Authenticator.SignCount, credential.Flags.BackupState, credential.Flags.BackupEligible); err != nil {
		return mfaError(c, err)
	}
	challenge.credentialID = credential.ID
	return completeMFALogin(c, state, challenge, "passkey")
}

func recentMFASettingsSession(c fiber.Ctx, state *core.AppState) (string, string, error) {
	user, err := requireAccountSession(c)
	if err != nil {
		return "", "", err
	}
	id := c.Locals("current_session_id").(string)
	if c.Cookies(sessionCookieName) != id {
		return "", "", fiber.ErrForbidden
	}
	session, err := state.GetDB().GetSession(id)
	if err != nil {
		return "", "", err
	}
	if session == nil || session.Username != user.Username || time.Now().UnixMilli()-session.CreatedAt > mfaTTL.Milliseconds() {
		return "", "", fiber.ErrForbidden
	}
	return user.Username, id, nil
}

func beginTOTPSetup(c fiber.Ctx, state *core.AppState) error {
	var request struct{}
	if err := readMFARequest(c, &request); err != nil {
		return err
	}
	username, session, err := recentMFASettingsSession(c, state)
	if err != nil {
		return mfaError(c, err)
	}
	mfa, err := state.GetDB().GetMFAState(username)
	if err != nil {
		return mfaError(c, err)
	}
	if mfa.Secret != "" {
		return mfaError(c, core.ErrMFAInvalid)
	}
	key := make([]byte, 20)
	if _, err := rand.Read(key); err != nil {
		return mfaError(c, err)
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(key)
	issuer := "RenoP (" + c.Hostname() + ")"
	uri := (&url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + issuer + ":" + username,
		RawQuery: url.Values{"issuer": {issuer}, "secret": {secret}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}.Encode()}).String()
	png, err := qrcode.Encode(uri, qrcode.Medium, 320)
	if err != nil {
		return mfaError(c, err)
	}
	id, err := storeMFAChallenge(&mfaChallenge{state: state, username: username, method: "setup", snapshot: mfa.Snapshot, session: session, secret: secret})
	if err != nil {
		return mfaError(c, err)
	}
	setPrivateResponseHeaders(c)
	return c.JSON(fiber.Map{"id": id, "secret": secret, "uri": uri, "qr": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), "expires_at": time.Now().Add(mfaTTL).UnixMilli()})
}

func confirmTOTPSetup(c fiber.Ctx, state *core.AppState) error {
	var request struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	if err := readMFARequest(c, &request); err != nil {
		return err
	}
	username, session, err := recentMFASettingsSession(c, state)
	if err != nil {
		return mfaError(c, err)
	}
	mfaChallenges.Lock()
	entry := mfaChallenges.entries[request.ID]
	if entry == nil || entry.state != state || entry.method != "setup" || entry.username != username || entry.session != session || time.Since(entry.createdAt) >= mfaTTL || entry.attempts >= 5 {
		mfaChallenges.Unlock()
		return mfaError(c, core.ErrMFAInvalid)
	}
	entry.attempts++
	challenge := *entry
	step := core.VerifyTOTP(challenge.secret, request.Code, time.Now())
	if step >= 0 {
		delete(mfaChallenges.entries, request.ID)
	}
	mfaChallenges.Unlock()
	if step < 0 {
		return mfaError(c, core.ErrMFAInvalid)
	}
	mfa, err := state.GetDB().GetMFAState(username)
	if err != nil {
		return mfaError(c, err)
	}
	sealed, err := encryptMFASecret(state, mfa.UserID, challenge.secret)
	if err != nil {
		return mfaError(c, err)
	}
	if err := state.GetDB().UpdateMFA(username, challenge.snapshot, sealed, mfa.Passkey, step, session); err != nil {
		return mfaError(c, err)
	}
	return mfaSettingsUpdated(c, state, username, "Enabled authenticator verification")
}

func updateMFASettings(c fiber.Ctx, state *core.AppState) error {
	var request struct {
		Passkey *bool `json:"passkey_second_factor"`
		TOTP    *bool `json:"totp_enabled"`
	}
	if err := readMFARequest(c, &request); err != nil {
		return err
	}
	if request.Passkey == nil && request.TOTP == nil || request.TOTP != nil && *request.TOTP {
		return mfaError(c, core.ErrMFAInvalid)
	}
	username, session, err := recentMFASettingsSession(c, state)
	if err != nil {
		return mfaError(c, err)
	}
	mfa, err := state.GetDB().GetMFAState(username)
	if err != nil {
		return mfaError(c, err)
	}
	if request.Passkey != nil {
		mfa.Passkey = *request.Passkey
	}
	if request.TOTP != nil {
		mfa.Secret, mfa.LastStep = "", 0
	}
	if err := state.GetDB().UpdateMFA(username, mfa.Snapshot, mfa.Secret, mfa.Passkey, mfa.LastStep, session); err != nil {
		return mfaError(c, err)
	}
	return mfaSettingsUpdated(c, state, username, "Updated second-factor verification")
}

func mfaSettingsUpdated(c fiber.Ctx, state *core.AppState, username, detail string) error {
	state.ForgetUserSessions(username)
	state.InvalidateAccountAuthCache(true, username)
	_, operator, method, session, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{Username: username, Operator: operator, Action: audit.ActionProfileUpdate,
		Details: detail, AuthMethod: method, SessionID: session, IP: ip})
	security, err := state.GetDB().GetAccountSecurity(username)
	if err != nil {
		return mfaError(c, err)
	}
	setPrivateResponseHeaders(c)
	return c.JSON(accountSecurityWithConfig(state, security))
}
