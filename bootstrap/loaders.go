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
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/proto"

	"renop/config"
	"renop/core"
	"renop/index"
	"renop/pb"
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

func LoadTokens(tokensPath string) map[string]*core.AccessToken {
	file, err := os.Open(tokensPath)
	if err != nil {
		return make(map[string]*core.AccessToken)
	}
	defer file.Close()

	var tokens map[string]*core.AccessToken
	err = yaml.NewDecoder(bufio.NewReader(file)).Decode(&tokens)
	if err != nil {
		log.Printf("Failed to parse tokens file: %v", err)
		return make(map[string]*core.AccessToken)
	}
	if tokens == nil {
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

func LoadSessions(path string) (sessions []core.SessionDbDto, rewrite bool) {
	file, err := os.Open(path)
	if err != nil {
		if legacy := legacySessionsJSONPath(path); legacy != "" {
			if legacyFile, lerr := os.Open(legacy); lerr == nil {
				defer legacyFile.Close()
				log.Printf("Migrating sessions from legacy %s → %s", legacy, path)
				return parseSessionsJSONReader(bufio.NewReader(legacyFile)), true
			}
		}
		return []core.SessionDbDto{}, false
	}
	defer file.Close()

	br := bufio.NewReader(file)
	if isLegacySessionsJSONReader(br) {
		return parseSessionsJSONReader(br), true
	}

	data, err := io.ReadAll(br)
	if err != nil {
		log.Printf("Failed to read sessions file %s: %v", path, err)
		return []core.SessionDbDto{}, false
	}

	var store pb.SessionStore
	if err := proto.Unmarshal(data, &store); err != nil {
		log.Printf("Failed to parse sessions file %s: %v", path, err)
		return []core.SessionDbDto{}, false
	}

	return pb.ToSessionDbDtos(&store), false
}

func isLegacySessionsJSONReader(br *bufio.Reader) bool {
	for i := 1; i <= 4096; i *= 2 {
		peekBytes, err := br.Peek(i)
		trimmed := bytes.TrimSpace(peekBytes)
		if len(trimmed) > 0 {
			return trimmed[0] == '['
		}
		if err != nil {
			break
		}
	}
	return false
}

func parseSessionsJSONReader(r io.Reader) []core.SessionDbDto {
	var sessions []core.SessionDbDto
	if err := json.NewDecoder(r).Decode(&sessions); err != nil {
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
