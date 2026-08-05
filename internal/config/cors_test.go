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

func TestIsOriginAllowedDefaultDomains(t *testing.T) {
	s := DefaultServerConfig()
	s.Domains = []string{"mvnc.pkg.one"}

	if !s.IsOriginAllowed("https://mvnc.pkg.one") {
		t.Fatal("expected configured domain origin to be allowed")
	}
	if !s.IsOriginAllowed("http://mvnc.pkg.one:3000") {
		t.Fatal("expected same host any scheme/port to be allowed")
	}
	if s.IsOriginAllowed("https://other.pkg.one") {
		t.Fatal("sibling subdomain must not be allowed when cors_origins is empty")
	}
	if s.IsOriginAllowed("https://evil.example.com") {
		t.Fatal("foreign origin must not be allowed")
	}
}

func TestIsOriginAllowedWildcard(t *testing.T) {
	s := DefaultServerConfig()
	s.Domains = []string{"mvnc.pkg.one"}
	s.CorsOrigins = []string{"*.pkg.one"}

	if !s.IsOriginAllowed("https://mvnc.pkg.one") {
		t.Fatal("expected subdomain match")
	}
	if !s.IsOriginAllowed("https://cdn.pkg.one") {
		t.Fatal("expected sibling subdomain match")
	}
	if !s.IsOriginAllowed("https://pkg.one") {
		t.Fatal("expected apex match for *.pkg.one")
	}
	if !s.IsOriginAllowed("https://a.b.pkg.one") {
		t.Fatal("expected nested subdomain match")
	}
	if s.IsOriginAllowed("https://pkg.one.evil.com") {
		t.Fatal("must not match suffix spoof")
	}
	if s.IsOriginAllowed("https://notpkg.one") {
		t.Fatal("must not match unrelated host")
	}
}

func TestIsOriginAllowedStarAndExtras(t *testing.T) {
	s := DefaultServerConfig()
	s.CorsOrigins = []string{"*"}
	if !s.IsOriginAllowed("https://anything.example") {
		t.Fatal("* should allow any origin")
	}

	s.CorsOrigins = []string{"https://partner.example.com", "admin.local"}
	if !s.IsOriginAllowed("https://partner.example.com") {
		t.Fatal("exact full origin should match")
	}
	if s.IsOriginAllowed("http://partner.example.com") {
		t.Fatal("different scheme should not match exact full origin")
	}
	if !s.IsOriginAllowed("https://admin.local:8443") {
		t.Fatal("host-only pattern should allow any scheme/port")
	}
}

func TestIsOriginAllowedCorsOriginsUnionsDomains(t *testing.T) {
	s := DefaultServerConfig()
	s.Domains = []string{"mvnc.pkg.one"}
	s.CorsOrigins = []string{"https://partner.example.com"}

	if !s.IsOriginAllowed("https://mvnc.pkg.one") {
		t.Fatal("domains must still be allowed when cors_origins is non-empty")
	}
	if !s.IsOriginAllowed("http://mvnc.pkg.one:3000") {
		t.Fatal("domain host match any scheme/port")
	}
	if !s.IsOriginAllowed("https://partner.example.com") {
		t.Fatal("extra cors origin must be allowed")
	}
	if s.IsOriginAllowed("https://evil.example.com") {
		t.Fatal("foreign origin must not be allowed")
	}
}

func TestLegacyDomainYAML(t *testing.T) {
	var s ServerConfig
	err := yaml.Unmarshal([]byte(`
host: 0.0.0.0
port: 3000
domain: legacy.example.com
`), &s)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Domains) != 1 || s.Domains[0] != "legacy.example.com" {
		t.Fatalf("expected legacy domain migration, got %v", s.Domains)
	}
}

func TestDomainsYAMLAndJSON(t *testing.T) {
	var s ServerConfig
	err := yaml.Unmarshal([]byte(`
domains:
  - Alpha.Example.com
  - beta.example.com
  - alpha.example.com
cors_origins:
  - "*.example.com"
  - https://Other.Org
`), &s)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Domains) != 2 {
		t.Fatalf("expected deduped domains, got %v", s.Domains)
	}
	if s.Domains[0] != "alpha.example.com" {
		t.Fatalf("expected lowercased first domain, got %v", s.Domains)
	}
	if len(s.CorsOrigins) != 2 {
		t.Fatalf("expected cors patterns, got %v", s.CorsOrigins)
	}

	raw, err := json.Marshal(map[string]any{
		"domains":      []string{"one.test"},
		"cors_origins": []string{"*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var j ServerConfig
	if err := json.Unmarshal(raw, &j); err != nil {
		t.Fatal(err)
	}
	if len(j.Domains) != 1 || j.Domains[0] != "one.test" {
		t.Fatalf("json domains: %v", j.Domains)
	}
	if len(j.CorsOrigins) != 1 || j.CorsOrigins[0] != "*" {
		t.Fatalf("json cors: %v", j.CorsOrigins)
	}
}

func TestPrimaryDomain(t *testing.T) {
	s := ServerConfig{}
	if s.PrimaryDomain() != "localhost" {
		t.Fatalf("empty primary: %s", s.PrimaryDomain())
	}
	s.Domains = []string{"a.example", "b.example"}
	if s.PrimaryDomain() != "a.example" {
		t.Fatalf("primary: %s", s.PrimaryDomain())
	}
}
