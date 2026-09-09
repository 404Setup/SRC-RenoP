/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/mail"
	"renop/internal/service/audit"
	"renop/internal/service/mailqueue"
	"renop/internal/utils"
)

const registrationCookie = "renop_registration"

func setupRegistrationRoutes(auth fiber.Router, state *core.AppState) {
	auth.Get("/registration/status", func(c fiber.Ctx) error {
		setPrivateResponseHeaders(c)
		cfg := state.Inner.Config.Load()
		return c.JSON(fiber.Map{"enabled": cfg.Registration.Enabled, "email_required": cfg.Mail.Enabled})
	})
	auth.Get("/registration/pending", func(c fiber.Ctx) error { return getRegistrationPending(c, state) })
	auth.Post("/registration/code", func(c fiber.Ctx) error { return requestRegistrationCode(c, state) })
	auth.Post("/registration", func(c fiber.Ctx) error { return postRegistration(c, state) })
}

func registrationIP(c fiber.Ctx, server *config.ServerConfig) string {
	address, err := netip.ParseAddr(utils.ExtractIP(c, server))
	if err != nil {
		return ""
	}
	return address.Unmap().String()
}

func registrationHash(value string) string {
	if value == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func registrationCapability(c fiber.Ctx) string {
	value := c.Cookies(registrationCookie)
	if len(value) != 43 {
		return ""
	}
	return value
}

func setRegistrationCookie(c fiber.Ctx, value string) {
	age, expires := 600, time.Now().Add(10*time.Minute)
	if value == "" {
		age, expires = -1, time.Unix(1, 0)
	}
	c.Cookie(&fiber.Cookie{Name: registrationCookie, Value: value, Path: "/api/auth/registration", MaxAge: age,
		Expires: expires, Secure: isSecure(c), HTTPOnly: true, SameSite: "Lax"})
}

func registrationError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, core.ErrRegistrationDisabled):
		return passwordResetError(c, 404, "registration_disabled")
	case errors.Is(err, core.ErrRegistrationRateLimited):
		return passwordResetError(c, 429, "registration_ip_limited")
	case errors.Is(err, core.ErrRegistrationCooldown):
		return passwordResetError(c, 429, "registration_cooldown")
	case errors.Is(err, core.ErrRegistrationPending):
		return passwordResetError(c, 409, "registration_pending")
	case errors.Is(err, core.ErrRegistrationInvalid):
		return passwordResetError(c, 400, "registration_invalid")
	case errors.Is(err, core.ErrUsernameAlreadyExists):
		return passwordResetError(c, 409, "registration_username_conflict")
	case errors.Is(err, core.ErrGitHubIdentityLinked), errors.Is(err, core.ErrOAuthIdentityLinked):
		return passwordResetError(c, 409, "registration_identity_linked")
	case errors.Is(err, core.ErrEmailAlreadyExists), errors.Is(err, core.ErrEmailVerificationRequired),
		errors.Is(err, core.ErrAccountEmailLimit), errors.Is(err, mail.ErrRateLimited), errors.Is(err, mail.ErrQueueFull),
		errors.Is(err, mailqueue.ErrNoAccount), errors.Is(err, mailqueue.ErrRecipientBlocked):
		return emailVerificationError(c, err)
	default:
		return passwordResetError(c, 503, "registration_unavailable")
	}
}

func getRegistrationPending(c fiber.Ctx, state *core.AppState) error {
	setPrivateResponseHeaders(c)
	if !state.Inner.Config.Load().Registration.Enabled {
		return registrationError(c, core.ErrRegistrationDisabled)
	}
	pending, err := state.GetDB().GetPendingRegistration(registrationHash(registrationCapability(c)), time.Now().UnixMilli())
	if err != nil {
		return registrationError(c, err)
	}
	profile := core.RegistrationProfile{}
	if pending.ProfileJSON != "" && json.Unmarshal([]byte(pending.ProfileJSON), &profile) != nil {
		return registrationError(c, core.ErrRegistrationInvalid)
	}
	available := false
	if username, ok := core.NormalizeUsername(profile.Username); ok {
		existing, err := state.GetDB().GetTokenByName(username)
		if err != nil {
			return registrationError(c, err)
		}
		available = existing == nil
	}
	provider, _, _ := strings.Cut(pending.ProviderKey, ":")
	providerName := provider
	if p, ok := state.Inner.Config.Load().Server.OAuthProvider(provider); ok {
		providerName = p.Name
	}
	return c.JSON(fiber.Map{"provider": provider, "email": pending.Email, "expires_at": pending.ExpiresAt,
		"username": profile.Username, "nickname": profile.Nickname, "username_available": available,
		"provider_name": providerName, "email_required": provider == "" || profile.OAuth != nil && !profile.EmailVerified,
		"mail_enabled": state.Inner.Config.Load().Mail.Enabled, "avatar_available": profile.GitHubID > 0 || profile.AvatarURL != ""})
}

func requestRegistrationCode(c fiber.Ctx, state *core.AppState) error {
	setPrivateResponseHeaders(c)
	state.Inner.ConfigWriteLock.Lock()
	defer state.Inner.ConfigWriteLock.Unlock()
	cfg := state.Inner.Config.Load()
	if !cfg.Registration.Enabled {
		return registrationError(c, core.ErrRegistrationDisabled)
	}
	if !cfg.Mail.Enabled {
		return passwordResetError(c, 404, "mail_disabled")
	}
	var request struct {
		Email    string `json:"email"`
		Provider string `json:"provider"`
	}
	if err := readPasswordResetRequest(c, &request); err != nil {
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) || errors.Is(err, fiber.ErrUnsupportedMediaType) {
			return err
		}
		return registrationError(c, core.ErrRegistrationInvalid)
	}
	email, valid := core.NormalizeEmail(request.Email)
	if !valid || email == "" {
		return passwordResetError(c, 400, "ACCOUNT_EMAIL_INVALID")
	}
	capability := registrationCapability(c)
	var providerPending *core.PendingRegistration
	if capability != "" {
		pending, err := state.GetDB().GetPendingRegistration(registrationHash(capability), time.Now().UnixMilli())
		if request.Provider != "" {
			if err != nil {
				return registrationError(c, core.ErrRegistrationInvalid)
			}
			provider, _, _ := strings.Cut(pending.ProviderKey, ":")
			if provider != request.Provider {
				return registrationError(c, core.ErrRegistrationInvalid)
			}
			providerPending = pending
		} else if err == nil && pending.ProviderKey != "" {
			capability = ""
		}
	}
	if request.Provider != "" && providerPending == nil {
		return registrationError(c, core.ErrRegistrationInvalid)
	}
	if capability == "" {
		var err error
		capability, err = newOAuthState()
		if err != nil {
			return registrationError(c, err)
		}
	}
	code, err := newEmailVerificationCode()
	if err != nil {
		return registrationError(c, err)
	}
	ip := registrationIP(c, &cfg.Server)
	expiresAt := time.Now().Add(10 * time.Minute).UnixMilli()
	if providerPending != nil {
		expiresAt = providerPending.ExpiresAt
	}
	job, receipt, err := mailqueue.Prepare(cfg.Mail, mailqueue.Request{To: email, Actor: "guest", Scene: "registration_verify",
		IP: ip, Manual: true, ExpiresAt: expiresAt,
		Data: mail.TemplateData{Code: code, URL: strings.TrimRight(cfg.Mail.PublicURL, "/") + "/account/register"}})
	if err != nil {
		return registrationError(c, err)
	}
	pending := &core.PendingRegistration{IDHash: registrationHash(capability), IPHash: registrationHash(ip), Email: email,
		CodeHash:  emailVerificationHash(cfg.Mail.EncryptionKey, "registration", capability+"\x00"+email, code),
		CreatedAt: job.CreatedAt, ExpiresAt: job.ExpiresAt, CooldownUntil: job.ExpiresAt}
	if providerPending != nil {
		err = state.GetDB().QueueProviderRegistrationEmail(pending.IDHash, pending.IPHash, pending.CodeHash, job,
			cfg.Mail.EncryptionKey, ip, cfg.Mail.ManualRate, cfg.Registration)
	} else {
		err = state.GetDB().BeginRegistration(pending, job, cfg.Mail.EncryptionKey, ip, cfg.Mail.ManualRate, cfg.Registration)
	}
	if err != nil {
		return registrationError(c, err)
	}
	setRegistrationCookie(c, capability)
	mailqueue.Wake(state)
	return c.Status(202).JSON(receipt)
}

func postRegistration(c fiber.Ctx, state *core.AppState) error {
	setPrivateResponseHeaders(c)
	if !state.Inner.Config.Load().Registration.Enabled {
		return registrationError(c, core.ErrRegistrationDisabled)
	}
	var request struct {
		Provider     string `json:"provider"`
		Username     string `json:"username"`
		Nickname     string `json:"nickname"`
		Email        string `json:"email"`
		Password     string `json:"password"`
		Code         string `json:"code"`
		ImportAvatar bool   `json:"import_avatar"`
	}
	if err := readPasswordResetRequest(c, &request); err != nil {
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) || errors.Is(err, fiber.ErrUnsupportedMediaType) {
			return err
		}
		return registrationError(c, core.ErrRegistrationInvalid)
	}
	username, valid := core.NormalizeUsername(request.Username)
	nickname, validNickname := core.NormalizeNickname(request.Nickname)
	email, validEmail := core.NormalizeEmail(request.Email)
	if !valid || !validNickname || !validEmail || len(request.Password) < 6 || len(request.Password) > 72 || len(request.Code) > 8 {
		return registrationError(c, core.ErrRegistrationInvalid)
	}
	initialConfig := state.Inner.Config.Load()
	if err := state.GetDB().CheckRegistrationIP(registrationHash(registrationIP(c, &initialConfig.Server)), initialConfig.Registration, time.Now().UnixMilli()); err != nil {
		return registrationError(c, err)
	}
	password, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		return registrationError(c, err)
	}
	state.Inner.ConfigWriteLock.Lock()
	cfg := state.Inner.Config.Load()
	if !cfg.Registration.Enabled {
		state.Inner.ConfigWriteLock.Unlock()
		return registrationError(c, core.ErrRegistrationDisabled)
	}
	if (cfg.Mail.Enabled && email == "") || (email != "" && !cfg.Mail.Allows(email)) {
		state.Inner.ConfigWriteLock.Unlock()
		return registrationError(c, mailqueue.ErrRecipientBlocked)
	}
	provider, providerConfigured := cfg.Server.OAuthProvider(request.Provider)
	if request.Provider != "" && !(request.Provider == "github" && cfg.Server.GitHubOAuth.Configured()) && !providerConfigured {
		state.Inner.ConfigWriteLock.Unlock()
		return registrationError(c, core.ErrRegistrationInvalid)
	}
	capability := registrationCapability(c)
	if providerConfigured {
		pending, pendingErr := state.GetDB().GetPendingRegistration(registrationHash(capability), time.Now().UnixMilli())
		var pendingProfile core.RegistrationProfile
		if pendingErr != nil || json.Unmarshal([]byte(pending.ProfileJSON), &pendingProfile) != nil ||
			pendingProfile.ProviderConfigHash != oauthConfigurationHash(provider) {
			state.Inner.ConfigWriteLock.Unlock()
			return registrationError(c, core.ErrRegistrationInvalid)
		}
	}
	if !cfg.Mail.Enabled && request.Provider == "" {
		capability = ""
	}
	ip := registrationIP(c, &cfg.Server)
	state.Inner.TokenWriteLock.Lock()
	profile, err := state.GetDB().RegisterAccount(core.AccountRegistration{Provider: request.Provider, Username: username, Nickname: nickname,
		Email: email, PasswordHash: string(password), IPHash: registrationHash(ip), IDHash: registrationHash(capability),
		CodeHash:     emailVerificationHash(cfg.Mail.EncryptionKey, "registration", capability+"\x00"+email, strings.TrimSpace(request.Code)),
		RequireEmail: cfg.Mail.Enabled}, cfg.Registration, time.Now().UnixMilli())
	if err == nil {
		state.Inner.TokensCount.Add(1)
		state.InvalidateAccountAuthCache(true, username)
	}
	state.Inner.TokenWriteLock.Unlock()
	state.Inner.ConfigWriteLock.Unlock()
	if err != nil {
		return registrationError(c, err)
	}
	setRegistrationCookie(c, "")
	audit.Log(state, &core.AuditLogEntry{Username: username, Operator: username, Action: audit.ActionUserRegister,
		Details: "Registered account", AuthMethod: "Registration", IP: ip})
	avatarImported := false
	if request.ImportAvatar && profile.GitHubID > 0 {
		avatarImported = importRegistrationGitHubAvatar(c, state, profile)
	}
	if request.ImportAvatar && profile.OAuth != nil {
		avatarImported = importRegistrationOAuthAvatar(c, state, profile)
	}
	return c.Status(201).JSON(fiber.Map{"username": username, "avatar_imported": avatarImported})
}
