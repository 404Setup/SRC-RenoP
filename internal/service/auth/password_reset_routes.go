/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"errors"
	"mime"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"

	"renop/internal/core"
	"renop/internal/locale"
	"renop/internal/mail"
	"renop/internal/service/audit"
	"renop/internal/service/mailqueue"
	"renop/internal/utils"
)

func setupPasswordResetRoutes(auth fiber.Router, state *core.AppState) {
	auth.Get("/password-reset/status", func(c fiber.Ctx) error {
		setPrivateResponseHeaders(c)
		return c.JSON(fiber.Map{"enabled": state.Inner.Config.Load().Mail.Enabled})
	})
	auth.Post("/password-reset/request", func(c fiber.Ctx) error { return requestEmailPasswordReset(c, state) })
	auth.Post("/password-reset/confirm", func(c fiber.Ctx) error { return confirmEmailPasswordReset(c, state) })
}

func passwordResetError(c fiber.Ctx, status int, code string) error {
	c.Set("X-Renop-Error-Code", code)
	return c.Status(status).JSON(fiber.Map{"error": code})
}

func readPasswordResetRequest(c fiber.Ctx, request any) error {
	contentType, _, err := mime.ParseMediaType(c.Get(fiber.HeaderContentType))
	if err != nil || contentType != fiber.MIMEApplicationJSON {
		return fiber.ErrUnsupportedMediaType
	}
	return utils.ReadJSONLimited(c, request, 4096)
}

func emailPasswordResetHash(key, email, code string) string {
	return emailVerificationHash(key, "password_reset", email, code)
}

func requestEmailPasswordReset(c fiber.Ctx, state *core.AppState) error {
	setPrivateResponseHeaders(c)
	cfg := state.Inner.Config.Load()
	if !cfg.Mail.Enabled {
		return passwordResetError(c, 404, "mail_disabled")
	}
	var request struct {
		Email string `json:"email"`
	}
	if err := readPasswordResetRequest(c, &request); err != nil {
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) || errors.Is(err, fiber.ErrUnsupportedMediaType) {
			return err
		}
		return passwordResetError(c, 400, "mail_request_invalid")
	}
	email, valid := core.NormalizeEmail(request.Email)
	if !valid || email == "" {
		return passwordResetError(c, 400, "ACCOUNT_EMAIL_INVALID")
	}
	language, err := state.GetDB().GetEmailLocale(email)
	if err != nil {
		return passwordResetError(c, 503, "mail_unavailable")
	}
	if language == "" {
		language = locale.FromHeader(c.Get(fiber.HeaderAcceptLanguage))
	}
	code, err := newEmailVerificationCode()
	if err != nil {
		return passwordResetError(c, 503, "mail_unavailable")
	}
	ip := utils.ExtractIP(c, &cfg.Server)
	job, receipt, err := mailqueue.Prepare(cfg.Mail, mailqueue.Request{
		To: email, Actor: "guest", Scene: "password_reset", IP: ip, Manual: true,
		ExpiresAt: time.Now().Add(10 * time.Minute).UnixMilli(),
		Data:      mail.TemplateData{Locale: language, Code: code, URL: strings.TrimRight(cfg.Mail.PublicURL, "/") + "/account/forgot-password"},
	})
	if err == nil {
		_, err = state.GetDB().QueueEmailPasswordReset(job, emailPasswordResetHash(cfg.Mail.EncryptionKey, email, code), cfg.Mail.EncryptionKey, ip, cfg.Mail.ManualRate)
	}
	if err != nil {
		status, reason := 503, "mail_unavailable"
		switch {
		case errors.Is(err, mail.ErrRateLimited):
			status, reason = 429, "mail_manual_rate_limited"
		case errors.Is(err, mail.ErrQueueFull):
			reason = "mail_queue_full"
		case errors.Is(err, mailqueue.ErrNoAccount):
			reason = "mail_no_account"
		case errors.Is(err, mailqueue.ErrRecipientBlocked):
			status, reason = 400, "mail_recipient_blocked"
		}
		return passwordResetError(c, status, reason)
	}
	mailqueue.Wake(state)
	return c.Status(202).JSON(receipt)
}

func confirmEmailPasswordReset(c fiber.Ctx, state *core.AppState) error {
	setPrivateResponseHeaders(c)
	cfg := state.Inner.Config.Load()
	if !cfg.Mail.Enabled {
		return passwordResetError(c, 404, "mail_disabled")
	}
	var request struct {
		Email       string `json:"email"`
		Code        string `json:"code"`
		NewPassword string `json:"new_password"`
	}
	if err := readPasswordResetRequest(c, &request); err != nil {
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) || errors.Is(err, fiber.ErrUnsupportedMediaType) {
			return err
		}
		return passwordResetError(c, 400, "mail_request_invalid")
	}
	email, valid := core.NormalizeEmail(request.Email)
	code := strings.TrimSpace(request.Code)
	if !valid || email == "" || len(code) != 8 || strings.IndexFunc(code, func(r rune) bool { return r < '0' || r > '9' }) >= 0 ||
		len(request.NewPassword) < 6 || len(request.NewPassword) > 72 {
		return passwordResetError(c, 400, "ACCOUNT_EMAIL_CODE_INVALID")
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(request.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return passwordResetError(c, 503, "mail_unavailable")
	}
	state.Inner.TokenWriteLock.Lock()
	username, err := state.GetDB().ResetPasswordWithEmailCode(email, emailPasswordResetHash(cfg.Mail.EncryptionKey, email, code), string(passwordHash), time.Now().UnixMilli())
	if err == nil {
		purgeRecoveredSessions(state, username)
	}
	state.Inner.TokenWriteLock.Unlock()
	if errors.Is(err, core.ErrEmailCodeInvalid) || errors.Is(err, core.ErrAccountDeleted) {
		return passwordResetError(c, 401, "ACCOUNT_EMAIL_CODE_INVALID")
	}
	if err != nil {
		return passwordResetError(c, 503, "mail_unavailable")
	}
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: username, Action: audit.ActionPasswordUpdate,
		Details: "Password reset with email verification", AuthMethod: "EmailCode", IP: utils.ExtractIP(c, &cfg.Server),
	})
	setSessionCookie(c, "", -1)
	return c.JSON(fiber.Map{"status": "success", "username": username})
}
