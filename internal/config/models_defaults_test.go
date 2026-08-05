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

func TestMirrorDefaultsJson(t *testing.T) {
	var m Mirror
	err := json.Unmarshal([]byte(`{}`), &m)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Persist || m.CacheTtlSecs != 3600 || !m.NegativeCache || m.TimeoutSecs != 30 {
		t.Fatalf("Defaults not applied in JSON: %+v", m)
	}
}

func TestMirrorDefaultsYaml(t *testing.T) {
	var m Mirror
	err := yaml.Unmarshal([]byte(`{}`), &m)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Persist || m.CacheTtlSecs != 3600 || !m.NegativeCache || m.TimeoutSecs != 30 {
		t.Fatalf("Defaults not applied in YAML: %+v", m)
	}
}
