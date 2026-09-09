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

// MavenDomainConfig controls how long a security-locked publishing domain is reserved.
type MavenDomainConfig struct {
	ReleaseValue int    `json:"release_value" yaml:"release_value"`
	ReleaseUnit  string `json:"release_unit" yaml:"release_unit"`
}

// DefaultMavenDomainConfig reserves a locked domain for two calendar years.
func DefaultMavenDomainConfig() MavenDomainConfig {
	return MavenDomainConfig{ReleaseValue: 2, ReleaseUnit: "year"}
}

// Validate rejects disabled, unbounded, or ambiguous release periods.
func (c MavenDomainConfig) Validate() error {
	if c.ReleaseValue < 1 || c.ReleaseValue > 100 || (c.ReleaseUnit != "month" && c.ReleaseUnit != "year") {
		return errors.New("Maven domain release period must be 1 to 100 months or years")
	}
	return nil
}

// ReleaseAt returns the end of the reservation using UTC calendar arithmetic.
func (c MavenDomainConfig) ReleaseAt(lockedAt int64) int64 {
	if c.Validate() != nil || lockedAt <= 0 {
		return 0
	}
	months := c.ReleaseValue
	if c.ReleaseUnit == "year" {
		months *= 12
	}
	return time.UnixMilli(lockedAt).UTC().AddDate(0, months, 0).UnixMilli()
}
