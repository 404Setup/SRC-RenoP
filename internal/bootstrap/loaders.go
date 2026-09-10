/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package bootstrap

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"

	"go.yaml.in/yaml/v3"

	"renop/internal/config"
	"renop/internal/service/index"
	"renop/internal/utils"
)

// LoadConfig creates a missing configuration or rejects unreadable and invalid settings.
func LoadConfig(configPath string) (*config.Config, error) {
	file, err := os.Open(configPath)
	var cfg *config.Config

	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("open configuration: %w", err)
		}
		log.Printf("Config file not found at %s, using default config and creating it", configPath)
		cfg = config.DefaultConfig()

		yamlData, err := yaml.Marshal(cfg)
		if err != nil {
			return nil, err
		}
		if err := utils.WritePrivateFile(configPath, yamlData); err != nil {
			return nil, fmt.Errorf("create configuration: %w", err)
		}
	} else {
		defer file.Close()
		err = yaml.NewDecoder(bufio.NewReader(file)).Decode(&cfg)
		if err != nil {
			return nil, fmt.Errorf("parse configuration: %w", err)
		}
	}
	if cfg == nil {
		return nil, errors.New("configuration must contain a settings mapping")
	}

	cfg.Frontend.CachedIndexHTML = []byte{}

	return cfg, nil
}

func LoadFileIndex(indexPath string) *index.FileIndex {
	file, err := os.Open(indexPath)
	if err != nil {
		return index.NewFileIndex()
	}
	defer file.Close()

	idx := index.NewFileIndex()
	br := bufio.NewReaderSize(file, 64*1024)
	if err := idx.ReadJSONFrom(br); err != nil {
		log.Printf("Failed to parse index file: %v", err)
		return index.NewFileIndex()
	}
	return idx
}
