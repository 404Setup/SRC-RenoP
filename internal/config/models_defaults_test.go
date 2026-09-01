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
	"testing"

	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
)

func TestDefaultSuperTeamConfigAndDeepCopy(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.SuperTeams.CreateLimit != 5 || cfg.SuperTeams.JoinLimit != 20 {
		t.Fatalf("unexpected global team defaults: %+v", cfg.SuperTeams)
	}
	copy := cfg.DeepCopy()
	copy.SuperTeams.CreateLimit = 9
	if cfg.SuperTeams.CreateLimit != 5 {
		t.Fatal("global team config was not copied independently")
	}
}

func TestDefaultAvatarLimitAndDeepCopy(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.Server.AvatarMaxSizeBytes != DefaultAvatarMaxSizeBytes {
		t.Fatalf("unexpected avatar size default: %d", cfg.Server.AvatarMaxSizeBytes)
	}
	copy := cfg.DeepCopy()
	copy.Server.AvatarMaxSizeBytes = 2 << 20
	if cfg.Server.AvatarMaxSizeBytes != DefaultAvatarMaxSizeBytes {
		t.Fatal("avatar size limit was not copied independently")
	}
}

func TestDefaultPublicationQuotaConfigAndDeepCopy(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.PublicationQuota.FileLimit != 600 || cfg.PublicationQuota.ByteLimit != 40<<20 ||
		cfg.PublicationQuota.PublicationLimit != 20 || cfg.PublicationQuota.Period != "month" {
		t.Fatalf("unexpected publication quota defaults: %+v", cfg.PublicationQuota)
	}
	copy := cfg.DeepCopy()
	copy.PublicationQuota.FileLimit = 10
	if cfg.PublicationQuota.FileLimit != 600 {
		t.Fatal("publication quota config was not copied independently")
	}
}

func TestMirrorDefaultsJson(t *testing.T) {
	var m Mirror
	err := json.Unmarshal([]byte(`{}`), &m)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Persist || m.CacheTTLSecs != 3600 || !m.NegativeCache || m.TimeoutSecs != 30 {
		t.Fatalf("Defaults not applied in JSON: %+v", m)
	}
}

func TestMirrorDefaultsYaml(t *testing.T) {
	var m Mirror
	err := yaml.Unmarshal([]byte(`{}`), &m)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Persist || m.CacheTTLSecs != 3600 || !m.NegativeCache || m.TimeoutSecs != 30 {
		t.Fatalf("Defaults not applied in YAML: %+v", m)
	}
}
