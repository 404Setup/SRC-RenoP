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
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/locale"
	"renop/internal/mail"
	"renop/internal/service/audit"
	"renop/internal/service/captcha"
	"renop/internal/service/mailqueue"
	"renop/internal/utils"

	"github.com/gofiber/fiber/v3"
)

func newEmailVerificationCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%08d", value.Int64()), nil
}

func emailVerificationHash(key, purpose, subject, code string) string {
	verifier := hmac.New(sha256.New, []byte(key))
	_, _ = verifier.Write([]byte(purpose + "\x00" + subject + "\x00" + code))
	return hex.EncodeToString(verifier.Sum(nil))
}

func accountSecurityWithConfig(state *core.AppState, security *core.AccountSecurity) *core.AccountSecurity {
	security.EmailVerificationRequired = state.Inner.Config.Load().Mail.Enabled
	return security
}

func emailVerificationError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, core.ErrEmailVerificationRequired):
		return passwordResetError(c, 409, "ACCOUNT_EMAIL_PROOF_REQUIRED")
	case errors.Is(err, core.ErrAccountEmailLimit):
		return passwordResetError(c, 409, "ACCOUNT_EMAIL_LIMIT")
	case errors.Is(err, core.ErrPrimaryEmail):
		return passwordResetError(c, 409, "ACCOUNT_EMAIL_PRIMARY")
	case errors.Is(err, core.ErrEmailAlreadyExists):
		return passwordResetError(c, 409, "ACCOUNT_EMAIL_CONFLICT")
	case errors.Is(err, core.ErrEmailCodeInvalid):
		return passwordResetError(c, 400, "ACCOUNT_EMAIL_CODE_INVALID")
	case errors.Is(err, core.ErrAccountDeleted), errors.Is(err, core.ErrAccountBanned):
		return passwordResetError(c, 401, "ACCOUNT_EMAIL_CODE_INVALID")
	case errors.Is(err, mail.ErrRateLimited):
		return passwordResetError(c, 429, "mail_manual_rate_limited")
	case errors.Is(err, mail.ErrQueueFull):
		return passwordResetError(c, 503, "mail_queue_full")
	case errors.Is(err, mailqueue.ErrNoAccount):
		return passwordResetError(c, 503, "mail_no_account")
	case errors.Is(err, mailqueue.ErrRecipientBlocked):
		return passwordResetError(c, 400, "mail_recipient_blocked")
	default:
		return passwordResetError(c, 503, "mail_unavailable")
	}
}

func providerEmailErrorCode(err error) string {
	switch {
	case errors.Is(err, core.ErrEmailAlreadyExists):
		return "email_conflict"
	case errors.Is(err, core.ErrEmailVerificationRequired):
		return "email_unverified"
	case errors.Is(err, core.ErrAccountEmailLimit):
		return "email_limit"
	default:
		return ""
	}
}

func queueProfileEmailVerification(c fiber.Ctx, state *core.AppState, username, email string, alias bool) error {
	if err := captcha.Require(c, state, config.CaptchaManualMail); err != nil {
		return err
	}
	cfg := state.Inner.Config.Load()
	profile, err := state.GetDB().GetUserProfile(username)
	if err != nil {
		return emailVerificationError(c, err)
	}
	language := profile.Locale
	if language == "" {
		language = locale.FromHeader(c.Get(fiber.HeaderAcceptLanguage))
	}
	code, err := newEmailVerificationCode()
	if err != nil {
		return emailVerificationError(c, err)
	}
	session := c.Locals("current_session_id").(string)
	ip := utils.ExtractIP(c, &cfg.Server)
	job, receipt, err := mailqueue.Prepare(cfg.Mail, mailqueue.Request{
		To: email, Username: username, Actor: username, Scene: "email_verify", IP: ip, Manual: true,
		ExpiresAt: time.Now().Add(10 * time.Minute).UnixMilli(), Data: mail.TemplateData{Locale: language, Code: code},
	})
	if err == nil {
		err = state.GetDB().QueueAccountEmailChange(username, session, job,
			emailVerificationHash(cfg.Mail.EncryptionKey, "email_change", session+"\x00"+email, code),
			cfg.Mail.EncryptionKey, ip, cfg.Mail.ManualRate, alias)
	}
	if err != nil {
		return emailVerificationError(c, err)
	}
	mailqueue.Wake(state)
	return c.Status(202).JSON(receipt)
}

func recordPrivateEmailChange(c fiber.Ctx, state *core.AppState, username string) {
	state.InvalidateAccountAuthCache(true, username)
	actor, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: actor, Operator: operator, Action: audit.ActionProfileUpdate,
		Details: "Updated private login email", AuthMethod: authMethod, SessionID: sessionID, IP: ip,
	})
}

func confirmProfileEmailVerification(c fiber.Ctx, state *core.AppState) error {
	setPrivateResponseHeaders(c)
	user, err := requireAccountSession(c)
	if err != nil {
		return accountSessionError(c, err)
	}
	session := c.Locals("current_session_id").(string)
	if c.Cookies(sessionCookieName) != session {
		return accountSessionError(c, fiber.ErrForbidden)
	}
	cfg := state.Inner.Config.Load()
	if !cfg.Mail.Enabled {
		return passwordResetError(c, 404, "mail_disabled")
	}
	var request struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := readMFARequest(c, &request); err != nil {
		return err
	}
	email, valid := core.NormalizeEmail(request.Email)
	code := strings.TrimSpace(request.Code)
	if !valid || email == "" || len(code) != 8 || strings.IndexFunc(code, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return passwordResetError(c, 400, "ACCOUNT_EMAIL_CODE_INVALID")
	}
	if !cfg.Mail.Allows(email) {
		return emailVerificationError(c, mailqueue.ErrRecipientBlocked)
	}
	security, err := state.GetDB().ConfirmAccountEmailChange(user.Username, session, email,
		emailVerificationHash(cfg.Mail.EncryptionKey, "email_change", session+"\x00"+email, code), time.Now().UnixMilli())
	if err != nil {
		return emailVerificationError(c, err)
	}
	recordPrivateEmailChange(c, state, user.Username)
	return c.JSON(accountSecurityWithConfig(state, security))
}
