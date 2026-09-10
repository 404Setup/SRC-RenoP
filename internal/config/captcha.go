/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"unicode"

	"github.com/goccy/go-json"
)

const (
	CaptchaPasswordLogin   = "password_login"
	CaptchaRegistration    = "registration"
	CaptchaManualMail      = "manual_mail"
	CaptchaSuperTeamCreate = "super_team_create"
	CaptchaDomainCreate    = "domain_create"
	CaptchaPackageCreate   = "package_create"
)

// CaptchaScopes identifies interactive actions protected by a human challenge.
type CaptchaScopes struct {
	PasswordLogin   bool `json:"password_login" yaml:"password_login"`
	Registration    bool `json:"registration" yaml:"registration"`
	ManualMail      bool `json:"manual_mail" yaml:"manual_mail"`
	SuperTeamCreate bool `json:"super_team_create" yaml:"super_team_create"`
	DomainCreate    bool `json:"domain_create" yaml:"domain_create"`
	PackageCreate   bool `json:"package_create" yaml:"package_create"`
}

// Enabled reports whether a recognized action is protected.
func (s CaptchaScopes) Enabled(scope string) bool {
	switch scope {
	case CaptchaPasswordLogin:
		return s.PasswordLogin
	case CaptchaRegistration:
		return s.Registration
	case CaptchaManualMail:
		return s.ManualMail
	case CaptchaSuperTeamCreate:
		return s.SuperTeamCreate
	case CaptchaDomainCreate:
		return s.DomainCreate
	case CaptchaPackageCreate:
		return s.PackageCreate
	default:
		return false
	}
}

// CaptchaConfig keeps provider credentials private and leaves verification disabled by default.
type CaptchaConfig struct {
	Provider       string        `json:"provider" yaml:"provider"`
	SiteKey        string        `json:"site_key" yaml:"site_key"`
	SecretKey      string        `json:"secret_key" yaml:"secret_key"`
	MinScore       float64       `json:"min_score" yaml:"min_score"`
	FriendlyRegion string        `json:"friendly_region" yaml:"friendly_region"`
	Scopes         CaptchaScopes `json:"scopes" yaml:"scopes"`
	policyHash     string
}

// DefaultCaptchaConfig provides conservative defaults without loading a third-party service.
func DefaultCaptchaConfig() CaptchaConfig {
	return CaptchaConfig{Provider: "disabled", MinScore: 0.5, FriendlyRegion: "global"}
}

// Normalize validates a detached configuration before it can protect requests.
func (c *CaptchaConfig) Normalize() error {
	c.Provider = strings.TrimSpace(c.Provider)
	if c.Provider == "" {
		c.Provider = "disabled"
	}
	switch c.Provider {
	case "disabled", "recaptcha_v2", "recaptcha_invisible", "recaptcha_v3", "turnstile", "hcaptcha", "friendlycaptcha":
	default:
		return errors.New("invalid CAPTCHA provider")
	}
	c.SiteKey, c.SecretKey = strings.TrimSpace(c.SiteKey), strings.TrimSpace(c.SecretKey)
	if len(c.SiteKey) > 256 || len(c.SecretKey) > 1024 ||
		strings.IndexFunc(c.SiteKey+c.SecretKey, func(r rune) bool { return r > unicode.MaxASCII || unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return errors.New("invalid CAPTCHA credentials")
	}
	if c.Provider != "disabled" && (c.SiteKey == "" || c.SecretKey == "") {
		return errors.New("CAPTCHA requires a site key and secret key")
	}
	if math.IsNaN(c.MinScore) || math.IsInf(c.MinScore, 0) || c.MinScore < 0 || c.MinScore > 1 {
		return errors.New("CAPTCHA score must be between zero and one")
	}
	if c.FriendlyRegion == "" {
		c.FriendlyRegion = "global"
	}
	if c.FriendlyRegion != "global" && c.FriendlyRegion != "eu" {
		return errors.New("invalid Friendly Captcha region")
	}
	c.policyHash = ""
	c.policyHash = c.PolicyHash()
	return nil
}

// EffectiveScope applies the first enabled scope; callers put manual mail before overlapping actions.
func (c CaptchaConfig) EffectiveScope(scopes ...string) string {
	if c.Provider == "" || c.Provider == "disabled" {
		return ""
	}
	for _, scope := range scopes {
		if c.Scopes.Enabled(scope) {
			return scope
		}
	}
	return ""
}

// PolicyHash binds short-lived server-side approvals to the exact private provider configuration.
func (c CaptchaConfig) PolicyHash() string {
	if c.policyHash != "" {
		return c.policyHash
	}
	data, _ := json.Marshal(c)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// DeepCopy returns a detached configuration without borrowing request-backed strings.
func (c CaptchaConfig) DeepCopy() CaptchaConfig {
	c.Provider, c.SiteKey, c.SecretKey = strings.Clone(c.Provider), strings.Clone(c.SiteKey), strings.Clone(c.SecretKey)
	c.FriendlyRegion = strings.Clone(c.FriendlyRegion)
	return c
}
