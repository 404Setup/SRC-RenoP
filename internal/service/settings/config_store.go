/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"os"

	"go.yaml.in/yaml/v3"

	"renop/internal/config"
	"renop/internal/utils"
)

func persistConfigSnapshot(cfg *config.Config) error {
	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	configPath := os.Getenv("RENOP_CONFIG")
	if configPath == "" {
		configPath = "config.yaml"
	}
	tmpPath := configPath + ".tmp"
	if err := utils.WritePrivateFile(tmpPath, yamlData); err != nil {
		return err
	}
	return utils.SafeRename(tmpPath, configPath)
}
