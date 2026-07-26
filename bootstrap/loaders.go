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
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bytedance/sonic"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/proto"

	"renop/config"
	"renop/core"
	"renop/index"
	"renop/pb"
)

func LoadConfig(configPath string) *config.Config {
	data, err := os.ReadFile(configPath)
	var cfg config.Config

	if err != nil {
		log.Printf("Config file not found at %s, using default config and creating it", configPath)
		cfg = config.DefaultConfig()

		yamlData, err := yaml.Marshal(&cfg)
		if err == nil {
			_ = os.WriteFile(configPath, yamlData, 0644)
		}
	} else {
		err = yaml.Unmarshal(data, &cfg)
		if err != nil {
			log.Printf("Failed to parse config file: %v", err)
			cfg = config.DefaultConfig()
		}
	}

	cfg.Frontend.CachedIndexHtml = []byte{}

	return &cfg
}

func LoadTokens(tokensPath string) map[string]*core.AccessToken {
	data, err := os.ReadFile(tokensPath)
	if err != nil {
		return make(map[string]*core.AccessToken)
	}

	var tokens map[string]*core.AccessToken
	err = yaml.Unmarshal(data, &tokens)
	if err != nil {
		log.Printf("Failed to parse tokens file: %v", err)
		return make(map[string]*core.AccessToken)
	}

	return tokens
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
	data, err := os.ReadFile(path)
	if err != nil {
		return config.DefaultMavenSettings()
	}

	var mavenSettings config.MavenSettings
	err = yaml.Unmarshal(data, &mavenSettings)
	if err != nil {
		log.Printf("Failed to parse maven file: %v", err)
		return config.DefaultMavenSettings()
	}

	return mavenSettings
}

func LoadSessions(path string) (sessions []core.SessionDbDto, rewrite bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		if legacy := legacySessionsJSONPath(path); legacy != "" {
			if legacyData, lerr := os.ReadFile(legacy); lerr == nil {
				log.Printf("Migrating sessions from legacy %s → %s", legacy, path)
				return parseSessionsJSON(legacyData), true
			}
		}
		return []core.SessionDbDto{}, false
	}

	if isLegacySessionsJSON(data) {
		return parseSessionsJSON(data), true
	}

	var store pb.SessionStore
	if err := proto.Unmarshal(data, &store); err != nil {
		log.Printf("Failed to parse sessions file %s: %v", path, err)
		return []core.SessionDbDto{}, false
	}

	return pb.ToSessionDbDtos(&store), false
}

func isLegacySessionsJSON(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == '['
}

func parseSessionsJSON(data []byte) []core.SessionDbDto {
	var sessions []core.SessionDbDto
	if err := sonic.ConfigFastest.Unmarshal(data, &sessions); err != nil {
		log.Printf("Failed to parse legacy sessions JSON: %v", err)
		return []core.SessionDbDto{}
	}
	if sessions == nil {
		return []core.SessionDbDto{}
	}
	return sessions
}

// legacySessionsJSONPath returns sessions.json next to path when path looks like
// the new default (*.bin) or is explicitly different from sessions.json.
func legacySessionsJSONPath(path string) string {
	base := filepath.Base(path)
	if strings.EqualFold(base, "sessions.json") {
		return ""
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return "sessions.json"
	}
	return filepath.Join(dir, "sessions.json")
}
