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

import "strings"

// GitHubOAuthConfig controls GitHub OAuth login and identity authorization.
type GitHubOAuthConfig struct {
	ClientID     string `json:"client_id" yaml:"client_id"`
	ClientSecret string `json:"client_secret" yaml:"client_secret"`
	CallbackURL  string `json:"callback_url" yaml:"callback_url"`
	Enabled      bool   `json:"enabled" yaml:"enabled"`
}

// Configured reports whether GitHub OAuth can be used without exposing its secret.
func (c GitHubOAuthConfig) Configured() bool {
	return c.Enabled && strings.TrimSpace(c.ClientID) != "" &&
		strings.TrimSpace(c.ClientSecret) != "" && strings.TrimSpace(c.CallbackURL) != ""
}

// DeepCopy returns an independent GitHub OAuth configuration.
func (c GitHubOAuthConfig) DeepCopy() GitHubOAuthConfig {
	return GitHubOAuthConfig{
		ClientID:     strings.Clone(c.ClientID),
		ClientSecret: strings.Clone(c.ClientSecret),
		CallbackURL:  strings.Clone(c.CallbackURL),
		Enabled:      c.Enabled,
	}
}
