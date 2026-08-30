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
	DefaultPublicationFileLimit int64 = 600
	DefaultPublicationByteLimit int64 = 40 << 20
	DefaultPublicationPushLimit int64 = 20
)

// PublicationQuotaConfig defines global publication limits inherited by users and global teams.
type PublicationQuotaConfig struct {
	FileLimit        int64  `json:"file_limit" yaml:"file_limit"`
	ByteLimit        int64  `json:"byte_limit" yaml:"byte_limit"`
	PublicationLimit int64  `json:"publication_limit" yaml:"publication_limit"`
	Period           string `json:"period" yaml:"period"`
}

func (config *PublicationQuotaConfig) setDefaults() {
	if config.FileLimit <= 0 {
		config.FileLimit = DefaultPublicationFileLimit
	}
	if config.ByteLimit <= 0 {
		config.ByteLimit = DefaultPublicationByteLimit
	}
	if config.PublicationLimit <= 0 {
		config.PublicationLimit = DefaultPublicationPushLimit
	}
	if config.Period != "day" && config.Period != "week" && config.Period != "month" {
		config.Period = "month"
	}
}

// DeepCopy returns an independent publication-quota configuration value.
func (config PublicationQuotaConfig) DeepCopy() PublicationQuotaConfig {
	return config
}

// DefaultPublicationQuotaConfig returns conservative monthly publication limits.
func DefaultPublicationQuotaConfig() PublicationQuotaConfig {
	return PublicationQuotaConfig{
		FileLimit: DefaultPublicationFileLimit, ByteLimit: DefaultPublicationByteLimit,
		PublicationLimit: DefaultPublicationPushLimit, Period: "month",
	}
}
