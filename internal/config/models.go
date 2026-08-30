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
	"fmt"
	"strings"

	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
)

type AuditLogConfig struct {
	RetentionDays int `json:"retention_days" yaml:"retention_days"`
	MaxRows       int `json:"max_rows" yaml:"max_rows"`
}

type GPGConfig struct {
	KeyServers []string `json:"key_servers" yaml:"key_servers"`
}

func (g *GPGConfig) setDefaults() {
	if len(g.KeyServers) == 0 {
		g.KeyServers = DefaultGPGKeyServers()
	}
}

func (g GPGConfig) DeepCopy() GPGConfig {
	return GPGConfig{KeyServers: append([]string(nil), g.KeyServers...)}
}

func (a *AuditLogConfig) setDefaults() {
	if a.RetentionDays <= 0 {
		a.RetentionDays = 14
	}
	if a.MaxRows <= 0 {
		a.MaxRows = 10000
	}
}

type Config struct {
	StoragePath           string          `json:"storage_path" yaml:"storage_path"`
	EnableJavadocPreview  bool            `json:"enable_javadoc_preview" yaml:"enable_javadoc_preview"`
	JavadocExtractPath    string          `json:"javadoc_extract_path" yaml:"javadoc_extract_path"`
	MaxJavadocSizeMb      int64           `json:"max_javadoc_size_mb" yaml:"max_javadoc_size_mb"`
	EnableCargodocPreview bool            `json:"enable_cargodoc_preview" yaml:"enable_cargodoc_preview"`
	CargodocExtractPath   string          `json:"cargodoc_extract_path" yaml:"cargodoc_extract_path"`
	MaxCargodocSizeMb     int64           `json:"max_cargodoc_size_mb" yaml:"max_cargodoc_size_mb"`
	Frontend              FrontendConfig  `json:"frontend" yaml:"frontend"`
	Maven                 MavenSettings   `json:"-" yaml:"-"`
	Server                ServerConfig    `json:"server" yaml:"server"`
	Updater               UpdaterConfig   `json:"updater" yaml:"updater"`
	Database              DatabaseConfig  `json:"database" yaml:"database"`
	AuditLog              AuditLogConfig  `json:"audit_log" yaml:"audit_log"`
	SuperTeams            SuperTeamConfig `json:"super_teams" yaml:"super_teams"`
	// GPG is retained as a source-compatibility alias for integrations that
	// still access the old top-level field. It is never serialized; Server.GPG
	// is the canonical configuration location.
	GPG   GPGConfig   `json:"-" yaml:"-"`
	Proxy ProxyConfig `json:"proxy" yaml:"proxy"`
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
	if !c.EnableCargodocPreview && c.MaxCargodocSizeMb == 0 && c.CargodocExtractPath == "" {
		c.EnableCargodocPreview = true
	}
	if c.MaxCargodocSizeMb <= 0 {
		c.MaxCargodocSizeMb = 128
	}
	if len(c.Server.GPG.KeyServers) == 0 && len(c.GPG.KeyServers) > 0 {
		c.Server.GPG = c.GPG.DeepCopy()
	}
	c.Frontend.setDefaults()
	c.Maven.setDefaults()
	c.Server.setDefaults()
	c.Updater.setDefaults()
	c.Database.setDefaults()
	c.AuditLog.setDefaults()
	c.SuperTeams.setDefaults()
	c.Server.GPG.setDefaults()
	c.GPG = c.Server.GPG.DeepCopy()
}

type legacyConfigGPGWrapper struct {
	GPG    *GPGConfig      `json:"gpg"`
	Server json.RawMessage `json:"server"`
}

func (c *Config) UnmarshalJSON(data []byte) error {
	c.setDefaults()
	var legacy legacyConfigGPGWrapper
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	type alias Config
	aux := (*alias)(c)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if legacy.GPG != nil {
		var serverFields map[string]json.RawMessage
		if len(legacy.Server) > 0 {
			if err := json.Unmarshal(legacy.Server, &serverFields); err != nil {
				return fmt.Errorf("decode legacy server configuration: %w", err)
			}
		}
		if _, nested := serverFields["gpg"]; !nested {
			c.Server.GPG = legacy.GPG.DeepCopy()
		}
	}
	c.Server.setDefaults()
	c.Updater.setDefaults()
	c.Database.setDefaults()
	c.AuditLog.setDefaults()
	c.Server.GPG.setDefaults()
	c.GPG = c.Server.GPG.DeepCopy()
	return nil
}

func (c *Config) UnmarshalYAML(value *yaml.Node) error {
	c.setDefaults()
	var root yaml.Node
	if err := value.Decode(&root); err != nil {
		return err
	}
	var legacyGPG *GPGConfig
	var nestedGPG bool
	if root.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(root.Content); i += 2 {
			key, node := root.Content[i], root.Content[i+1]
			switch key.Value {
			case "gpg":
				var parsed GPGConfig
				if err := node.Decode(&parsed); err != nil {
					return err
				}
				legacyGPG = &parsed
			case "server":
				if node.Kind == yaml.MappingNode {
					for j := 0; j+1 < len(node.Content); j += 2 {
						if node.Content[j].Value == "gpg" {
							nestedGPG = true
							break
						}
					}
				}
			}
		}
	}
	type alias Config
	aux := (*alias)(c)
	if err := value.Decode(aux); err != nil {
		return err
	}
	if legacyGPG != nil && !nestedGPG {
		c.Server.GPG = legacyGPG.DeepCopy()
	}
	c.Server.setDefaults()
	c.Updater.setDefaults()
	c.Database.setDefaults()
	c.AuditLog.setDefaults()
	c.Server.GPG.setDefaults()
	c.GPG = c.Server.GPG.DeepCopy()
	return nil
}

type SettingsUpdate struct {
	Frontend *FrontendConfig
	Maven    *MavenSettings
	Server   *ServerConfig
	AuditLog *AuditLogConfig
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
	} else if _, ok := obj["retention_days"]; ok {
		a := DefaultAuditLogConfig()
		if err := json.Unmarshal(data, &a); err == nil {
			su.AuditLog = &a
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
		AuditLog:             c.AuditLog,
		SuperTeams:           c.SuperTeams.DeepCopy(),
		GPG:                  c.Server.GPG.DeepCopy(),
		Proxy:                c.Proxy.DeepCopy(),
	}
}
