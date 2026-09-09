/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package mail implements bounded email transports, templates, and accounting without provider SDKs.
package mail

import (
	"errors"
	"fmt"
	"net"
	netmail "net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/idna"
)

// MaxAccounts bounds configured sending identities and worker calibration state.
const MaxAccounts = 64

// Interval is a configurable duration in whole units.
type Interval struct {
	Value int64  `json:"value" yaml:"value"`
	Unit  string `json:"unit" yaml:"unit"`
}

// Duration returns zero for disabled or invalid intervals.
func (i Interval) Duration() time.Duration {
	unit := map[string]time.Duration{"second": time.Second, "minute": time.Minute, "hour": time.Hour, "day": 24 * time.Hour}[i.Unit]
	if unit == 0 || i.Value <= 0 || i.Value > int64(365*24*time.Hour/unit) {
		return 0
	}
	return time.Duration(i.Value) * unit
}

// Rate limits attempts within a fixed interval.
type Rate struct {
	Limit    int64    `json:"limit" yaml:"limit"`
	Interval Interval `json:"interval" yaml:"interval"`
}

// Quota is a UTC calendar-period sending allowance; zero selects remote discovery.
type Quota struct {
	Limit  int64  `json:"limit" yaml:"limit"`
	Period string `json:"period" yaml:"period"`
}

// PriceTier charges AmountMicros per BatchSize messages up to an inclusive volume.
type PriceTier struct {
	UpTo         int64 `json:"up_to" yaml:"up_to"`
	AmountMicros int64 `json:"amount_micros" yaml:"amount_micros"`
	BatchSize    int64 `json:"batch_size" yaml:"batch_size"`
}

// Pricing uses millionths of Currency to avoid floating-point balance drift.
type Pricing struct {
	Currency string      `json:"currency" yaml:"currency"`
	Rounding string      `json:"rounding" yaml:"rounding"`
	Tiers    []PriceTier `json:"tiers" yaml:"tiers"`
}

// Account configures one sending identity and its billing limits.
type Account struct {
	ID              string   `json:"id" yaml:"id"`
	Name            string   `json:"name" yaml:"name"`
	Enabled         bool     `json:"enabled" yaml:"enabled"`
	Provider        string   `json:"provider" yaml:"provider"`
	Preset          string   `json:"preset" yaml:"preset"`
	Scenes          []string `json:"scenes" yaml:"scenes"`
	From            string   `json:"from" yaml:"from"`
	FromName        string   `json:"from_name" yaml:"from_name"`
	Endpoint        string   `json:"endpoint" yaml:"endpoint"`
	Region          string   `json:"region" yaml:"region"`
	SMTPHost        string   `json:"smtp_host" yaml:"smtp_host"`
	SMTPPort        int      `json:"smtp_port" yaml:"smtp_port"`
	SMTPSecurity    string   `json:"smtp_security" yaml:"smtp_security"`
	Username        string   `json:"username" yaml:"username"`
	Password        string   `json:"password" yaml:"password"`
	APIKey          string   `json:"api_key" yaml:"api_key"`
	APISecret       string   `json:"api_secret" yaml:"api_secret"`
	SessionToken    string   `json:"session_token" yaml:"session_token"`
	AccountID       string   `json:"account_id" yaml:"account_id"`
	Mailbox         string   `json:"mailbox" yaml:"mailbox"`
	Tenant          string   `json:"tenant" yaml:"tenant"`
	ClientID        string   `json:"client_id" yaml:"client_id"`
	ClientSecret    string   `json:"client_secret" yaml:"client_secret"`
	AccessToken     string   `json:"access_token" yaml:"access_token"`
	RefreshToken    string   `json:"refresh_token" yaml:"refresh_token"`
	Quota           Quota    `json:"quota" yaml:"quota"`
	ForceSend       bool     `json:"force_send" yaml:"force_send"`
	Overage         Quota    `json:"overage" yaml:"overage"`
	Pricing         Pricing  `json:"pricing" yaml:"pricing"`
	BalanceMicros   *int64   `json:"balance_micros" yaml:"balance_micros"`
	FetchBalance    bool     `json:"fetch_balance" yaml:"fetch_balance"`
	BillingEndpoint string   `json:"billing_endpoint" yaml:"billing_endpoint"`

	TencentTemplateID int64 `json:"tencent_template_id" yaml:"tencent_template_id"`
}

// Config controls the single durable sending queue.
type Config struct {
	EncryptionKey          string    `json:"-" yaml:"encryption_key"`
	Enabled                bool      `json:"enabled" yaml:"enabled"`
	PublicURL              string    `json:"public_url" yaml:"public_url"`
	SiteName               string    `json:"site_name" yaml:"site_name"`
	TemplateStyle          string    `json:"template_style" yaml:"template_style"`
	Delay                  Interval  `json:"delay" yaml:"delay"`
	ManualRate             Rate      `json:"manual_rate" yaml:"manual_rate"`
	AccountRate            Rate      `json:"account_rate" yaml:"account_rate"`
	Calibration            Interval  `json:"calibration" yaml:"calibration"`
	ListMode               string    `json:"list_mode" yaml:"list_mode"`
	Addresses              []string  `json:"addresses" yaml:"addresses"`
	UseDisposableBlacklist bool      `json:"use_disposable_blacklist" yaml:"use_disposable_blacklist"`
	Accounts               []Account `json:"accounts" yaml:"accounts"`
}

// DefaultConfig keeps sending disabled until an administrator configures an account.
func DefaultConfig() Config {
	return Config{SiteName: "RenoP", TemplateStyle: "card", ListMode: "blacklist",
		Delay: Interval{5, "second"}, ManualRate: Rate{1, Interval{2, "minute"}},
		AccountRate: Rate{50, Interval{1, "minute"}}, Calibration: Interval{5, "minute"}}
}

// Clone copies editable slices and optional balances before configuration changes.
func (c Config) Clone() Config {
	c.Addresses = slices.Clone(c.Addresses)
	c.Accounts = slices.Clone(c.Accounts)
	for i := range c.Accounts {
		a := &c.Accounts[i]
		a.Scenes = slices.Clone(a.Scenes)
		a.Pricing.Tiers = slices.Clone(a.Pricing.Tiers)
		if a.BalanceMicros != nil {
			value := *a.BalanceMicros
			a.BalanceMicros = &value
		}
	}
	return c
}

var identifier = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// Normalize fills optional account defaults without overriding explicit rate values.
func (c *Config) Normalize() {
	if c.SiteName == "" {
		c.SiteName = "RenoP"
	}
	if c.TemplateStyle == "" {
		c.TemplateStyle = "card"
	}
	if c.ListMode == "" {
		c.ListMode = "blacklist"
	}
	for i := range c.Accounts {
		a := &c.Accounts[i]
		if a.Quota.Period == "" {
			a.Quota.Period = "month"
		}
		if a.Overage.Period == "" {
			a.Overage.Period = "month"
		}
		if a.SMTPSecurity == "" {
			a.SMTPSecurity = "starttls"
		}
		if a.SMTPPort == 0 {
			a.SMTPPort = 587
			if a.SMTPSecurity == "tls" {
				a.SMTPPort = 465
			}
		}
		if a.Pricing.Rounding == "" {
			a.Pricing.Rounding = "proportional"
		}
		if a.Pricing.Currency == "" {
			a.Pricing.Currency = "USD"
		}
		a.Pricing.Currency = strings.ToUpper(strings.TrimSpace(a.Pricing.Currency))
	}
}

func validInterval(i Interval, units []string, allowDisabled bool) bool {
	if allowDisabled && i.Value <= 0 {
		return true
	}
	return slices.Contains(units, i.Unit) && i.Duration() > 0
}

// Address accepts one unadorned mailbox, never a header or a recipient list.
func Address(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 254 || strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("invalid email address")
	}
	address, err := netmail.ParseAddress(value)
	if err != nil || address.Name != "" || address.Address != value || !strings.Contains(value, "@") {
		return "", errors.New("invalid email address")
	}
	return strings.ToLower(value), nil
}

// HTTPSURL validates an administrator-supplied API origin or service base path.
func HTTPSURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && len(value) <= 1024
}

// Validate rejects invalid limits, ambiguous routing, and unsafe mail headers.
func (c Config) Validate() error {
	if len(c.Accounts) > MaxAccounts || len(c.Addresses) > 1000 || len(c.SiteName) > 120 || strings.ContainsAny(c.SiteName, "\r\n\x00") ||
		(c.Enabled && (!HTTPSURL(c.PublicURL) || len(c.Accounts) == 0)) ||
		!slices.Contains([]string{"card", "compact", "notice"}, c.TemplateStyle) ||
		!slices.Contains([]string{"blacklist", "whitelist"}, c.ListMode) ||
		!validInterval(c.Delay, []string{"second", "minute", "hour"}, true) || c.Delay.Value < 0 ||
		!validInterval(c.Calibration, []string{"minute", "hour", "day"}, true) ||
		!validInterval(c.ManualRate.Interval, []string{"minute", "hour", "day"}, false) || c.ManualRate.Limit < 1 || c.ManualRate.Limit > 1000000 ||
		!validInterval(c.AccountRate.Interval, []string{"second", "minute", "hour", "day"}, false) || c.AccountRate.Limit < 1 || c.AccountRate.Limit > 1000000 {
		return errors.New("invalid mail configuration")
	}
	for _, value := range c.Addresses {
		value = canonicalRecipientRule(value)
		if strings.HasPrefix(value, "@") || strings.HasPrefix(value, ".") {
			if !validDomain(value[1:]) || value[0] == '@' && !strings.Contains(value[1:], ".") {
				return errors.New("invalid email domain")
			}
		} else if _, err := Address(value); err != nil {
			return err
		}
	}
	ids, routes := map[string]bool{}, map[string]bool{}
	for _, a := range c.Accounts {
		if err := a.Validate(); err != nil {
			return fmt.Errorf("account %q: %w", a.ID, err)
		}
		if ids[a.ID] {
			return errors.New("duplicate email account")
		}
		ids[a.ID] = true
		if a.Enabled {
			for _, scene := range a.Scenes {
				if routes[scene] {
					return errors.New("ambiguous email scene")
				}
				routes[scene] = true
			}
		}
	}
	return nil
}

func validDomain(value string) bool {
	if len(value) < 1 || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
				return false
			}
		}
	}
	return true
}

// Validate checks the bounded transport and account policy configuration.
func (a Account) Validate() error {
	if !identifier.MatchString(a.ID) || len(a.Name) > 120 || len(a.FromName) > 120 || strings.ContainsAny(a.FromName, "\r\n\x00") || len(a.Scenes) > 32 ||
		!slices.Contains([]string{"smtp", "cloudflare", "graph", "ses", "sendgrid", "gmail", "aliyun", "tencent", "feishu"}, a.Provider) {
		return errors.New("invalid email account")
	}
	if _, err := Address(a.From); err != nil {
		return err
	}
	for _, scene := range a.Scenes {
		if scene != "*" && !slices.Contains(Scenes, scene) {
			return errors.New("invalid email scene")
		}
	}
	for _, value := range []string{a.Username, a.Password, a.APIKey, a.APISecret, a.SessionToken, a.AccountID, a.Mailbox, a.Tenant, a.ClientID, a.ClientSecret, a.AccessToken, a.RefreshToken} {
		if len(value) > 16384 || strings.ContainsAny(value, "\r\n\x00") {
			return errors.New("invalid account credential")
		}
	}
	if !validPeriod(a.Quota.Period) || !validPeriod(a.Overage.Period) || a.Quota.Limit > 1000000000 || a.Overage.Limit > 1000000000 {
		return errors.New("invalid account quota")
	}
	if err := a.Pricing.Validate(); err != nil {
		return err
	}
	if a.BalanceMicros != nil && (*a.BalanceMicros > 1000000000000000 || *a.BalanceMicros < -1000000000000000) {
		return errors.New("invalid account balance")
	}
	if a.BillingEndpoint != "" && !HTTPSURL(a.BillingEndpoint) {
		return errors.New("invalid billing endpoint")
	}
	if a.TencentTemplateID < 0 || a.TencentTemplateID > 9007199254740991 {
		return errors.New("invalid Tencent email template ID")
	}
	if a.Provider == "smtp" {
		if a.SMTPHost == "" || len(a.SMTPHost) > 253 || strings.ContainsAny(a.SMTPHost, "/\\\r\n\x00 @") || a.SMTPPort < 1 || a.SMTPPort > 65535 ||
			!slices.Contains([]string{"plain", "tls", "starttls"}, a.SMTPSecurity) {
			return errors.New("invalid SMTP configuration")
		}
		if strings.Contains(a.SMTPHost, ":") && net.ParseIP(a.SMTPHost) == nil {
			return errors.New("invalid SMTP host")
		}
	} else if !HTTPSURL(a.Endpoint) {
		return errors.New("invalid API endpoint")
	}
	return nil
}

// Allows applies mailbox, provider, and suffix rules before enqueue and sending.
func (c Config) Allows(address string) bool {
	address, err := Address(address)
	if err != nil {
		return false
	}
	address = canonicalRecipientRule(address)
	domain := address[strings.LastIndexByte(address, '@')+1:]
	found := false
	for _, value := range c.Addresses {
		value = canonicalRecipientRule(value)
		if value == address || value == "@"+domain ||
			strings.HasPrefix(value, ".") && (domain == value[1:] || strings.HasSuffix(domain, value)) {
			found = true
			break
		}
	}
	if c.ListMode == "whitelist" {
		return found
	}
	return !found && !(c.UseDisposableBlacklist && isDisposableDomain(domain))
}

func canonicalRecipientRule(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	separator := strings.LastIndexByte(value, '@')
	if strings.HasPrefix(value, ".") {
		separator = 0
	}
	if separator < 0 {
		return value
	}
	domain, err := idna.Lookup.ToASCII(strings.TrimSuffix(value[separator+1:], "."))
	if err != nil {
		return value
	}
	return value[:separator+1] + domain
}

// SelectAccount resolves an explicit scene, then the all-scenes fallback.
func (c Config) SelectAccount(scene string) *Account {
	var only, fallback *Account
	count := 0
	for i := range c.Accounts {
		a := &c.Accounts[i]
		if !a.Enabled {
			continue
		}
		only = a
		count++
		if slices.Contains(a.Scenes, scene) {
			return a
		}
		if slices.Contains(a.Scenes, "*") {
			fallback = a
		}
	}
	if count == 1 {
		return only
	}
	return fallback
}

func validPeriod(period string) bool {
	return slices.Contains([]string{"hour", "day", "week", "month"}, period)
}

// Window returns the current UTC calendar bucket, with Monday starting each week.
func Window(period string, now time.Time) (time.Time, time.Time) {
	now = now.UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	switch period {
	case "hour":
		start = now.Truncate(time.Hour)
		return start, start.Add(time.Hour)
	case "day":
		return start, start.AddDate(0, 0, 1)
	case "week":
		start = start.AddDate(0, 0, -(int(start.Weekday())+6)%7)
		return start, start.AddDate(0, 0, 7)
	default:
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0)
	}
}

// Validate checks ascending progressive tiers and integer arithmetic bounds.
func (p Pricing) Validate() error {
	if len(p.Currency) != 3 || !slices.Contains([]string{"proportional", "batch"}, p.Rounding) || len(p.Tiers) > 20 {
		return errors.New("invalid email pricing")
	}
	for _, letter := range p.Currency {
		if letter < 'A' || letter > 'Z' {
			return errors.New("invalid email currency")
		}
	}
	previous := int64(0)
	for i, tier := range p.Tiers {
		if tier.UpTo < 0 || tier.UpTo > 1000000000 || tier.UpTo != 0 && tier.UpTo <= previous || tier.UpTo == 0 && i != len(p.Tiers)-1 || tier.AmountMicros < 0 || tier.AmountMicros > 1000000000000 || tier.BatchSize < 1 || tier.BatchSize > 1000000000 {
			return errors.New("invalid email price tier")
		}
		previous = tier.UpTo
	}
	return nil
}

// Cost returns progressive cost in currency millionths, rounding at each tier boundary.
func (p Pricing) Cost(count int64) (int64, error) {
	if count < 0 || count > 1000000000 {
		return 0, errors.New("invalid email count")
	}
	var total, previous int64
	for _, tier := range p.Tiers {
		units := count - previous
		if units <= 0 {
			break
		}
		if tier.UpTo > 0 {
			units = min(units, tier.UpTo-previous)
		}
		if tier.BatchSize <= 0 {
			return 0, errors.New("invalid batch size")
		}
		whole, remainder := units/tier.BatchSize, units%tier.BatchSize
		if p.Rounding == "batch" && remainder > 0 {
			whole++
			remainder = 0
		}
		if tier.AmountMicros > 0 && (whole > 1000000000000000/tier.AmountMicros || remainder > 9223372036854775807/tier.AmountMicros) {
			return 0, errors.New("email cost overflow")
		}
		cost := whole * tier.AmountMicros
		if remainder > 0 {
			product := remainder * tier.AmountMicros
			cost += product / tier.BatchSize
			if product%tier.BatchSize > 0 {
				cost++
			}
		}
		if total > 1000000000000000-cost {
			return 0, errors.New("email cost overflow")
		}
		total += cost
		if tier.UpTo == 0 {
			return total, nil
		}
		previous = tier.UpTo
	}
	if count > previous {
		return 0, errors.New("email pricing does not cover volume")
	}
	return total, nil
}
