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
	"strings"

	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
)

type UpdaterConfig struct {
	Channel string `json:"channel" yaml:"channel"`
	Mode    string `json:"mode" yaml:"mode"`
}

func DefaultUpdaterConfig() UpdaterConfig {
	return UpdaterConfig{
		Channel: "release",
		Mode:    "manual",
	}
}

func (u *UpdaterConfig) setDefaults() {
	if u.Channel == "" {
		u.Channel = "release"
	}
	if u.Mode == "" {
		u.Mode = "manual"
	}
}

func (u *UpdaterConfig) UnmarshalJSON(data []byte) error {
	u.setDefaults()
	type alias UpdaterConfig
	aux := (*alias)(u)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	u.setDefaults()
	return nil
}

func (u *UpdaterConfig) UnmarshalYAML(value *yaml.Node) error {
	u.setDefaults()
	type alias UpdaterConfig
	aux := (*alias)(u)
	if err := value.Decode(aux); err != nil {
		return err
	}
	u.setDefaults()
	return nil
}

func (u *UpdaterConfig) DeepCopy() UpdaterConfig {
	return UpdaterConfig{
		Channel: strings.Clone(u.Channel),
		Mode:    strings.Clone(u.Mode),
	}
}
