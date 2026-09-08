/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package config

import (
	"errors"
	"time"
)

// RegistrationInterval bounds public registration windows; a month is thirty days.
type RegistrationInterval struct {
	Value int64  `json:"value" yaml:"value"`
	Unit  string `json:"unit" yaml:"unit"`
}

// Duration returns zero for invalid intervals or windows longer than one year.
func (i RegistrationInterval) Duration() time.Duration {
	unit := map[string]time.Duration{"minute": time.Minute, "hour": time.Hour, "day": 24 * time.Hour,
		"week": 7 * 24 * time.Hour, "month": 30 * 24 * time.Hour}[i.Unit]
	if unit == 0 || i.Value <= 0 || i.Value > int64(365*24*time.Hour/unit) {
		return 0
	}
	return time.Duration(i.Value) * unit
}

// RegistrationConfig controls self-service account creation and persistent abuse limits.
type RegistrationConfig struct {
	Enabled          bool                 `json:"enabled" yaml:"enabled"`
	IPLimit          int64                `json:"ip_limit" yaml:"ip_limit"`
	IPInterval       RegistrationInterval `json:"ip_interval" yaml:"ip_interval"`
	ProviderCooldown RegistrationInterval `json:"provider_cooldown" yaml:"provider_cooldown"`
}

// DefaultRegistrationConfig keeps public registration closed until an administrator enables it.
func DefaultRegistrationConfig() RegistrationConfig {
	return RegistrationConfig{IPLimit: 1, IPInterval: RegistrationInterval{3, "week"},
		ProviderCooldown: RegistrationInterval{12, "hour"}}
}

func (c *RegistrationConfig) setDefaults() {
	defaults := DefaultRegistrationConfig()
	if c.IPLimit == 0 && c.IPInterval.Unit == "" {
		c.IPLimit, c.IPInterval = defaults.IPLimit, defaults.IPInterval
	}
	if c.ProviderCooldown.Unit == "" && c.ProviderCooldown.Value == 0 {
		c.ProviderCooldown = defaults.ProviderCooldown
	}
}

// Validate rejects disabled limits instead of allowing a registration throttle bypass.
func (c RegistrationConfig) Validate() error {
	if c.IPLimit <= 0 || c.IPLimit > 10000 || c.IPInterval.Duration() == 0 || c.ProviderCooldown.Duration() == 0 {
		return errors.New("registration settings are invalid")
	}
	return nil
}
