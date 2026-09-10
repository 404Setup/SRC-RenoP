/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"errors"
	"mime"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/locale"
	"renop/internal/mail"
	"renop/internal/service/audit"
	"renop/internal/service/captcha"
	"renop/internal/service/mailqueue"
	"renop/internal/utils"
)

type mailSettingsResponse struct {
	mail.Config
	SecretsConfigured map[string][]string `json:"secrets_configured"`
}
type mailSettingsRequest struct {
	mail.Config
	ClearSecrets map[string][]string `json:"clear_secrets"`
}

func mailSecrets(a *mail.Account) map[string]*string {
	return map[string]*string{"password": &a.Password, "api_key": &a.APIKey, "api_secret": &a.APISecret, "session_token": &a.SessionToken, "client_secret": &a.ClientSecret, "access_token": &a.AccessToken, "refresh_token": &a.RefreshToken}
}

func mailSettings(cfg mail.Config) mailSettingsResponse {
	cfg = cfg.Clone()
	configured := map[string][]string{}
	for i := range cfg.Accounts {
		account := &cfg.Accounts[i]
		for name, value := range mailSecrets(account) {
			if *value != "" {
				configured[account.ID] = append(configured[account.ID], name)
			}
			*value = ""
		}
	}
	return mailSettingsResponse{Config: cfg, SecretsConfigured: configured}
}

func mailSettingsError(c fiber.Ctx, status int, code string) error {
	c.Set("X-Renop-Error-Code", code)
	return c.Status(status).JSON(fiber.Map{"error": code})
}

func readMailSettingsRequest(c fiber.Ctx, target any, limit int64) error {
	mediaType, _, err := mime.ParseMediaType(c.Get(fiber.HeaderContentType))
	if err != nil || mediaType != fiber.MIMEApplicationJSON {
		return fiber.ErrUnsupportedMediaType
	}
	return utils.ReadJSONLimited(c, target, limit)
}

func getMailSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(403)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(mailSettings(state.Inner.Config.Load().Mail))
}

func putMailSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(403)
	}
	var request mailSettingsRequest
	if err := readMailSettingsRequest(c, &request, 1<<20); err != nil {
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) || errors.Is(err, fiber.ErrUnsupportedMediaType) {
			return err
		}
		return mailSettingsError(c, 400, "mail_settings_invalid")
	}
	if len(request.ClearSecrets) > mail.MaxAccounts {
		return mailSettingsError(c, 400, "mail_settings_invalid")
	}
	state.Inner.ConfigWriteLock.Lock()
	defer state.Inner.ConfigWriteLock.Unlock()
	current := state.Inner.Config.Load()
	next := current.DeepCopy()
	next.Mail = request.Config
	next.Mail.EncryptionKey = current.Mail.EncryptionKey
	previous := map[string]mail.Account{}
	for _, account := range current.Mail.Accounts {
		previous[account.ID] = account
	}
	for i := range next.Mail.Accounts {
		account := &next.Mail.Accounts[i]
		old := previous[account.ID]
		oldSecrets := mailSecrets(&old)
		secrets := mailSecrets(account)
		if old.Provider == account.Provider && (account.Provider != "smtp" || strings.EqualFold(old.SMTPHost, account.SMTPHost) && old.Username == account.Username) {
			for name, value := range secrets {
				if *value == "" {
					*value = *oldSecrets[name]
				}
			}
		}
		for _, name := range request.ClearSecrets[account.ID] {
			value, ok := secrets[name]
			if !ok {
				return mailSettingsError(c, 400, "mail_settings_invalid")
			}
			*value = ""
		}
	}
	next.Mail.PublicURL = strings.TrimRight(strings.TrimSpace(next.Mail.PublicURL), "/")
	next.Mail.Normalize()
	if err := next.Mail.Validate(); err != nil {
		return mailSettingsError(c, 400, "mail_settings_invalid")
	}
	if err := next.Mail.EnsureKey(); err != nil {
		return mailSettingsError(c, 500, "mail_settings_save_failed")
	}
	if err := persistConfigSnapshot(next); err != nil {
		return mailSettingsError(c, 500, "mail_settings_save_failed")
	}
	state.Inner.Config.Store(next)
	mailqueue.Wake(state)
	username, operator, method, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{Username: username, Operator: operator, AuthMethod: method, SessionID: sessionID, IP: ip, Action: audit.ActionSettingsUpdate, Details: "Updated mail settings"})
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(mailSettings(next.Mail))
}

func getMailPresets(c fiber.Ctx) error {
	if !isManager(c) {
		return c.SendStatus(403)
	}
	return c.JSON(fiber.Map{"presets": mail.Presets(), "scenes": mail.Scenes})
}

func testMailSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(403)
	}
	var request struct {
		AccountID string `json:"account_id"`
		To        string `json:"to"`
	}
	if err := readMailSettingsRequest(c, &request, 4096); err != nil {
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) || errors.Is(err, fiber.ErrUnsupportedMediaType) {
			return err
		}
		return mailSettingsError(c, 400, "mail_request_invalid")
	}
	_, operator, _, _, _ := audit.ExtractAuthDetails(c, state)
	cfg := state.Inner.Config.Load()
	if cfg.Mail.Enabled {
		if err := captcha.Require(c, state, config.CaptchaManualMail); err != nil {
			return err
		}
	}
	receipt, err := mailqueue.Enqueue(state, mailqueue.Request{AccountID: request.AccountID, Actor: operator, To: request.To, Scene: "test", Manual: true, IP: utils.ExtractIP(c, &cfg.Server),
		Data: mail.TemplateData{Locale: locale.FromHeader(c.Get(fiber.HeaderAcceptLanguage))}})
	if err != nil {
		status, code := 503, "mail_unavailable"
		switch {
		case errors.Is(err, mail.ErrRateLimited):
			status, code = 429, "mail_manual_rate_limited"
		case errors.Is(err, mail.ErrQueueFull):
			code = "mail_queue_full"
		case errors.Is(err, mailqueue.ErrDisabled):
			code = "mail_disabled"
		case errors.Is(err, mailqueue.ErrNoAccount):
			status, code = 400, "mail_no_account"
		case errors.Is(err, mailqueue.ErrRecipientBlocked):
			status, code = 400, "mail_recipient_blocked"
		}
		return mailSettingsError(c, status, code)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Status(202).JSON(receipt)
}

func getMailAccountStatus(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(403)
	}
	cfg := state.Inner.Config.Load().Mail
	var account *mail.Account
	for i := range cfg.Accounts {
		if cfg.Accounts[i].ID == c.Params("id") {
			account = &cfg.Accounts[i]
			break
		}
	}
	if account == nil {
		return c.SendStatus(404)
	}
	value, err := state.GetDB().LoadMailAccount(account.ID, cfg.EncryptionKey)
	if err != nil {
		return mailSettingsError(c, 500, "mail_status_unavailable")
	}
	value.Normalize(*account, cfg.AccountRate, time.Now())
	remaining := value.Budget.Remaining
	if account.Quota.Limit > 0 {
		count := max(account.Quota.Limit-value.Usage[account.Quota.Period].Used, 0)
		remaining = &count
	} else if account.Quota.Limit < 0 {
		remaining = nil
	}
	var remainingOverage *int64
	if account.Overage.Limit >= 0 {
		count := max(account.Overage.Limit-value.Overage[account.Overage.Period].Used, 0)
		remainingOverage = &count
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"usage": value.Usage, "overage": value.Overage, "rate": value.Rate, "budget": value.Budget, "remaining_quota": remaining, "remaining_overage": remainingOverage, "balance_micros": value.BalanceMicros, "calibration_at": value.CalibrationAt, "calibration_error": value.CalibrationError, "attempts": value.Attempts, "charged": value.Charged, "spent_micros": value.SpentMicros})
}

func getMailJobs(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(403)
	}
	cfg := state.Inner.Config.Load().Mail
	if cfg.EncryptionKey == "" {
		return c.JSON(fiber.Map{"items": []any{}, "total": 0})
	}
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	jobs, total, err := state.GetDB().ListMailJobs("", c.Query("status"), cfg.EncryptionKey, limit, offset)
	if err != nil {
		return mailSettingsError(c, 500, "mail_status_unavailable")
	}
	items := make([]map[string]any, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, mailqueue.View(job))
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"items": items, "total": total})
}

func previewMailTemplate(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.SendStatus(403)
	}
	cfg := state.Inner.Config.Load().Mail.Clone()
	if style := c.Query("style"); style != "" {
		cfg.TemplateStyle = style
	}
	message, err := cfg.Render(c.Params("scene"), mail.TemplateData{Locale: locale.FromHeader(c.Get(fiber.HeaderAcceptLanguage)), Username: "RenoP", Code: "123456"})
	if err != nil {
		return mailSettingsError(c, 400, "mail_request_invalid")
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"subject": message.Subject, "html": message.HTML, "text": message.Text})
}
