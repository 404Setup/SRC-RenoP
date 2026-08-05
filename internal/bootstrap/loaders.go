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
	"log"
	"os"

	"go.yaml.in/yaml/v3"

	"renop/internal/config"
	"renop/internal/service/index"
)

func LoadConfig(configPath string) *config.Config {
	file, err := os.Open(configPath)
	var cfg config.Config

	if err != nil {
		log.Printf("Config file not found at %s, using default config and creating it", configPath)
		cfg = config.DefaultConfig()

		yamlData, err := yaml.Marshal(&cfg)
		if err == nil {
			_ = os.WriteFile(configPath, yamlData, 0644)
		}
	} else {
		defer file.Close()
		err = yaml.NewDecoder(bufio.NewReader(file)).Decode(&cfg)
		if err != nil {
			log.Printf("Failed to parse config file: %v", err)
			cfg = config.DefaultConfig()
		}
	}

	cfg.Frontend.CachedIndexHtml = []byte{}

	return &cfg
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

func LoadMaven(path string) config.MavenSettings {
	file, err := os.Open(path)
	if err != nil {
		return config.DefaultMavenSettings()
	}
	defer file.Close()

	var mavenSettings config.MavenSettings
	err = yaml.NewDecoder(bufio.NewReader(file)).Decode(&mavenSettings)
	if err != nil {
		log.Printf("Failed to parse maven file: %v", err)
		return config.DefaultMavenSettings()
	}

	return mavenSettings
}
