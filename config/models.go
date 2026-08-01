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

type Config struct {
	StoragePath          string         `json:"storage_path" yaml:"storage_path"`
	EnableJavadocPreview bool           `json:"enable_javadoc_preview" yaml:"enable_javadoc_preview"`
	JavadocExtractPath   string         `json:"javadoc_extract_path" yaml:"javadoc_extract_path"`
	MaxJavadocSizeMb     int64          `json:"max_javadoc_size_mb" yaml:"max_javadoc_size_mb"`
	Frontend             FrontendConfig `json:"frontend" yaml:"frontend"`
	Maven                MavenSettings  `json:"-" yaml:"-"`
	Server               ServerConfig   `json:"server" yaml:"server"`
	Updater              UpdaterConfig  `json:"updater" yaml:"updater"`
	Database             DatabaseConfig `json:"database" yaml:"database"`
}

func (c *Config) setDefaults() {
	if c.StoragePath == "" {
		c.StoragePath = "storage"
	}
	if !c.EnableJavadocPreview && c.MaxJavadocSizeMb == 0 && c.JavadocExtractPath == "" {
		c.EnableJavadocPreview = true
	}
	if c.MaxJavadocSizeMb <= 0 {
		c.MaxJavadocSizeMb = 48
	}
	c.Frontend.setDefaults()
	c.Maven.setDefaults()
	c.Server.setDefaults()
	c.Updater.setDefaults()
	c.Database.setDefaults()
}

func (c *Config) UnmarshalJSON(data []byte) error {
	c.setDefaults()
	type alias Config
	aux := (*alias)(c)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	c.Updater.setDefaults()
	c.Database.setDefaults()
	return nil
}

func (c *Config) UnmarshalYAML(value *yaml.Node) error {
	c.setDefaults()
	type alias Config
	aux := (*alias)(c)
	if err := value.Decode(aux); err != nil {
		return err
	}
	c.Updater.setDefaults()
	c.Database.setDefaults()
	return nil
}

type SettingsUpdate struct {
	Frontend *FrontendConfig
	Maven    *MavenSettings
	Server   *ServerConfig
}

func (su *SettingsUpdate) UnmarshalJSON(data []byte) error {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}

	if _, ok := obj["host"]; ok {
		s := DefaultServerConfig()
		if err := json.Unmarshal(data, &s); err == nil {
			su.Server = &s
			return nil
		}
	} else if _, ok := obj["repositories"]; ok {
		m := DefaultMavenSettings()
		if err := json.Unmarshal(data, &m); err == nil {
			su.Maven = &m
			return nil
		}
	} else {
		f := DefaultFrontendConfig()
		if err := json.Unmarshal(data, &f); err == nil {
			su.Frontend = &f
			return nil
		}
	}
	return nil
}

func (c *Config) DeepCopy() *Config {
	if c == nil {
		return nil
	}
	return &Config{
		StoragePath:          strings.Clone(c.StoragePath),
		EnableJavadocPreview: c.EnableJavadocPreview,
		JavadocExtractPath:   strings.Clone(c.JavadocExtractPath),
		MaxJavadocSizeMb:     c.MaxJavadocSizeMb,
		Frontend:             c.Frontend.DeepCopy(),
		Maven:                c.Maven.DeepCopy(),
		Server:               c.Server.DeepCopy(),
		Updater:              c.Updater.DeepCopy(),
		Database:             c.Database,
	}
}
