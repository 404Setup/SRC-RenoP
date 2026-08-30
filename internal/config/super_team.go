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

const (
	defaultSuperTeamCreateLimit = 5
	defaultSuperTeamJoinLimit   = 20
)

// SuperTeamConfig defines global per-account limits for global team ownership and membership.
type SuperTeamConfig struct {
	CreateLimit int `json:"create_limit" yaml:"create_limit"`
	JoinLimit   int `json:"join_limit" yaml:"join_limit"`
}

func (config *SuperTeamConfig) setDefaults() {
	if config.CreateLimit <= 0 {
		config.CreateLimit = defaultSuperTeamCreateLimit
	}
	if config.JoinLimit <= 0 {
		config.JoinLimit = defaultSuperTeamJoinLimit
	}
}

// DeepCopy returns an independent global-team configuration value.
func (config SuperTeamConfig) DeepCopy() SuperTeamConfig {
	return config
}

// DefaultSuperTeamConfig returns conservative global team limits for a new installation.
func DefaultSuperTeamConfig() SuperTeamConfig {
	return SuperTeamConfig{CreateLimit: defaultSuperTeamCreateLimit, JoinLimit: defaultSuperTeamJoinLimit}
}
