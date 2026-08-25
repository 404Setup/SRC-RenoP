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
	"testing"

	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
)

func TestIsArtifactAllowedPrefixBoundary(t *testing.T) {
	m := &Mirror{AllowArtifacts: []string{"org.apache"}}

	if ok, _ := m.IsArtifactAllowed("org/apache/commons/commons-lang3/3.12.0/commons-lang3-3.12.0.jar"); !ok {
		t.Fatal("expected org.apache.commons to match allow prefix org.apache")
	}
	if ok, _ := m.IsArtifactAllowed("org/apache/foo/1.0/foo-1.0.jar"); !ok {
		t.Fatal("expected org.apache to match allow list exactly")
	}

	if ok, _ := m.IsArtifactAllowed("org/apachex/lib/1.0/lib-1.0.jar"); ok {
		t.Fatal("org.apachex must not match allow prefix org.apache")
	}
	if ok, _ := m.IsArtifactAllowed("organization/thing/1.0/thing-1.0.jar"); ok {
		t.Fatal("organization must not match allow prefix org.apache")
	}
}

func TestIsArtifactAllowedMavenMetadataPath(t *testing.T) {
	m := &Mirror{AllowArtifacts: []string{"org.apache.commons"}}

	if ok, _ := m.IsArtifactAllowed("org/apache/commons/commons-lang3/maven-metadata.xml"); !ok {
		t.Fatal("expected artifact-level metadata under org.apache.commons to be allowed")
	}

	if ok, _ := m.IsArtifactAllowed("org/apache/commons/commons-lang3/3.12.0/maven-metadata.xml"); !ok {
		t.Fatal("expected version-level metadata under org.apache.commons to be allowed")
	}

	if ok, _ := m.IsArtifactAllowed("com/example/lib/maven-metadata.xml"); ok {
		t.Fatal("expected com.example metadata to be denied")
	}
}

func TestIsArtifactAllowedGAAndDeny(t *testing.T) {
	allow := &Mirror{AllowArtifacts: []string{"com.example:widget"}}
	if ok, _ := allow.IsArtifactAllowed("com/example/widget/1.0/widget-1.0.jar"); !ok {
		t.Fatal("expected group:artifact allow to match")
	}
	if ok, _ := allow.IsArtifactAllowed("com/example/other/1.0/other-1.0.jar"); ok {
		t.Fatal("expected different artifact under same group to be denied")
	}

	deny := &Mirror{DenyArtifacts: []string{"com.evil"}}
	if ok, _ := deny.IsArtifactAllowed("com/evil/tool/1.0/tool-1.0.jar"); ok {
		t.Fatal("expected deny list to block com.evil")
	}
	if ok, _ := deny.IsArtifactAllowed("com/good/tool/1.0/tool-1.0.jar"); !ok {
		t.Fatal("expected non-denied artifact to pass")
	}

	both := &Mirror{AllowArtifacts: []string{"a"}, DenyArtifacts: []string{"b"}}
	if ok, reason := both.IsArtifactAllowed("a/b/1.0/x.jar"); ok || reason == "" {
		t.Fatal("expected both allow and deny to reject")
	}
}

func TestIsArtifactAllowedForCargoUsesExactNormalizedCrateNames(t *testing.T) {
	allow := &Mirror{AllowArtifacts: []string{"Serde_JSON"}}
	if ok, _ := allow.IsArtifactAllowedFor(RepositoryFormatCargo, "se/rd/serde-json"); !ok {
		t.Fatal("expected normalized Cargo crate name to match allow list")
	}
	if ok, _ := allow.IsArtifactAllowedFor(RepositoryFormatCargo, "api/v1/crates/serde_json/1.0.0/download"); !ok {
		t.Fatal("expected Cargo download path to match allow list")
	}
	if ok, _ := allow.IsArtifactAllowedFor(RepositoryFormatCargo, "se/rd/serde-json-extra"); ok {
		t.Fatal("Cargo allow rules must not use prefix matching")
	}
	if ok, _ := allow.IsArtifactAllowedFor(RepositoryFormatCargo, "config.json"); !ok {
		t.Fatal("Cargo registry configuration must not be blocked by crate rules")
	}

	deny := &Mirror{DenyArtifacts: []string{"serde-json"}}
	if ok, _ := deny.IsArtifactAllowedFor(RepositoryFormatCargo, "api/v1/crates/serde-json/1.0.0/download"); ok {
		t.Fatal("expected Cargo deny rule to block matching crate")
	}
}

func TestValidateCargoArtifactURLRejectsAuthorityPlaceholders(t *testing.T) {
	valid := &Mirror{ArtifactUrl: "https://static.example.test/crates/{crate}/{crate}-{version}.crate"}
	if err := valid.ValidateArtifactURL(RepositoryFormatCargo); err != nil {
		t.Fatalf("valid Cargo artifact URL rejected: %v", err)
	}

	invalid := &Mirror{ArtifactUrl: "https://{crate}.example.test/{version}/{crate}.crate"}
	if err := invalid.ValidateArtifactURL(RepositoryFormatCargo); err == nil {
		t.Fatal("expected Cargo artifact URL authority placeholder to be rejected")
	}
}

func TestRepositorySerializationUsesFormatSpecificFields(t *testing.T) {
	cargoRepository := Repository{
		Name: "rust-packages", Format: RepositoryFormatCargo, Visibility: "PUBLIC",
		AllowRedeployment: true, RequireGPGSignature: true,
		Mirrors: []Mirror{{
			Name: "crates.io", Url: "https://index.crates.io/",
			ArtifactUrl: "https://static.crates.io/crates/{crate}/{crate}-{version}.crate",
		}},
	}
	for name, marshal := range map[string]func(any) ([]byte, error){
		"json": json.Marshal,
		"yaml": yaml.Marshal,
	} {
		t.Run("cargo "+name, func(t *testing.T) {
			encoded, err := marshal(cargoRepository)
			if err != nil {
				t.Fatal(err)
			}
			text := string(encoded)
			if strings.Contains(text, "allow_redeployment") || strings.Contains(text, "require_gpg_signature") {
				t.Fatalf("Cargo serialization contains Maven-only fields: %s", text)
			}
			if !strings.Contains(text, "artifact_url") {
				t.Fatalf("Cargo serialization omitted artifact URL template: %s", text)
			}
		})
	}

	mavenRepository := Repository{
		Name: "releases", Format: RepositoryFormatMaven, Visibility: "PUBLIC",
		Mirrors: []Mirror{{
			Name: "central", Url: "https://repo1.maven.org/maven2/",
			ArtifactUrl: "https://unused.example/{crate}/{version}",
		}},
	}
	for name, marshal := range map[string]func(any) ([]byte, error){
		"json": json.Marshal,
		"yaml": yaml.Marshal,
	} {
		t.Run("maven "+name, func(t *testing.T) {
			encoded, err := marshal(mavenRepository)
			if err != nil {
				t.Fatal(err)
			}
			text := string(encoded)
			if !strings.Contains(text, "allow_redeployment") || !strings.Contains(text, "require_gpg_signature") {
				t.Fatalf("Maven serialization omitted Maven policy fields: %s", text)
			}
			if strings.Contains(text, "artifact_url") {
				t.Fatalf("Maven serialization contains Cargo artifact URL field: %s", text)
			}
		})
	}
}

func TestRepositoryFormatVariantsAndLegacyUpgradeDefaults(t *testing.T) {
	classic := &Repository{Format: RepositoryFormatMavenClassic}
	if classic.NormalizedFormat() != RepositoryFormatMaven || classic.ConfiguredFormat() != RepositoryFormatMavenClassic {
		t.Fatalf("classic Maven format was not normalized as Maven: %#v", classic)
	}
	if classic.UsesModernMavenLayout() {
		t.Fatal("classic Maven repository reported the modern layout")
	}
	modern := &Repository{Format: RepositoryFormatMaven}
	if !modern.UsesModernMavenLayout() {
		t.Fatal("modern Maven repository did not report the domain layout")
	}
	for _, format := range []string{RepositoryFormatMaven, RepositoryFormatMavenClassic, RepositoryFormatFiles,
		RepositoryFormatCargo, RepositoryFormatDocker} {
		if !IsSupportedRepositoryFormat(format) {
			t.Fatalf("supported repository format %q was rejected", format)
		}
	}

	var settings MavenSettings
	if err := json.Unmarshal([]byte(`{"repositories":{"legacy":{"name":"legacy","visibility":"PUBLIC","mirrors":[]}}}`), &settings); err != nil {
		t.Fatal(err)
	}
	legacy := settings.Repositories["legacy"]
	if legacy == nil || legacy.ConfiguredFormat() != RepositoryFormatMaven || !legacy.UsesModernMavenLayout() {
		t.Fatalf("legacy Maven repository was not upgraded to the modern layout: %#v", legacy)
	}
	var yamlSettings MavenSettings
	if err := yaml.Unmarshal([]byte("repositories:\n  legacy:\n    name: legacy\n    visibility: PUBLIC\n    mirrors: []\n"), &yamlSettings); err != nil {
		t.Fatal(err)
	}
	yamlLegacy := yamlSettings.Repositories["legacy"]
	if yamlLegacy == nil || yamlLegacy.ConfiguredFormat() != RepositoryFormatMaven || !yamlLegacy.UsesModernMavenLayout() {
		t.Fatalf("legacy YAML Maven repository was not upgraded to the modern layout: %#v", yamlLegacy)
	}

	files := Repository{
		Name: "downloads", Format: RepositoryFormatFiles, Visibility: "PUBLIC",
		AllowRedeployment: true, RequireGPGSignature: true,
	}
	encoded, err := json.Marshal(files)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if !strings.Contains(text, `"allow_redeployment":true`) || strings.Contains(text, "require_gpg_signature") {
		t.Fatalf("file repository serialization has invalid publication policy fields: %s", text)
	}

	mirror := &Mirror{AllowArtifacts: []string{"public/*"}}
	if allowed, _ := mirror.IsArtifactAllowedFor(RepositoryFormatFiles, "public/releases/app.zip"); !allowed {
		t.Fatal("file mirror prefix rule rejected a matching path")
	}
	if allowed, _ := mirror.IsArtifactAllowedFor(RepositoryFormatFiles, "private/app.zip"); allowed {
		t.Fatal("file mirror prefix rule allowed an unrelated path")
	}
}

func TestMirrorCredentialsUnmarshaling(t *testing.T) {
	var m MirrorCredentials
	err := json.Unmarshal([]byte(`{"method":"basic","login":"foo","password":"bar"}`), &m)
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != "basic" || m.Login != "foo" || m.Password != "bar" {
		t.Fatalf("Unmarshaled values incorrect: method=%q login=%q password=%q", m.Method, m.Login, m.Password)
	}
}

func TestMirrorCredentialsCustomHeaderValidation(t *testing.T) {
	valid := &MirrorCredentials{Method: "custom-header", Login: "X-Repository-Token", Password: "secret"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid custom header rejected: %v", err)
	}

	for name, credentials := range map[string]*MirrorCredentials{
		"newline name":   {Method: "custom-header", Login: "X-Token\r\nInjected", Password: "secret"},
		"routing header": {Method: "custom-header", Login: "Host", Password: "secret"},
		"newline token":  {Method: "custom-header", Login: "X-Token", Password: "secret\nvalue"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := credentials.Validate(); err == nil {
				t.Fatal("expected invalid custom header to be rejected")
			}
		})
	}
	if err := (&MirrorCredentials{Method: "custom-header", Password: "secret"}).Validate(); err != nil {
		t.Fatalf("incomplete custom header should be saveable: %v", err)
	}
}

func TestConfigDeepCopy(t *testing.T) {
	orig := DefaultConfig()
	orig.StoragePath = "original_storage"
	orig.Frontend.Title = "Original Title"
	orig.Frontend.CachedIndexHtml = []byte("hello")
	orig.Server.TrustedProxies = []string{"192.168.1.1/32"}
	orig.Server.GPG.KeyServers = []string{"https://keys.example.test"}
	orig.GPG = orig.Server.GPG.DeepCopy()
	orig.Proxy = ProxyConfig{
		Selected: "primary",
		Proxies: []OutboundProxy{{
			Name:     "primary",
			URL:      "http://proxy.example:8080",
			Username: "proxy-user",
			Password: "proxy-password",
		}},
	}
	orig.Server.ParseTrustedProxies()
	orig.Maven.Repositories["testrepo"] = &Repository{
		Name:       "testrepo",
		Visibility: "PUBLIC",
		Mirrors: []Mirror{
			{
				Name:           "m1",
				AllowArtifacts: []string{"org.example"},
				Proxy: &MirrorProxy{
					URL:      "socks5://proxy.example:1080",
					Username: "proxy-user",
					Password: "proxy-password",
				},
				Authorization: &MirrorCredentials{
					Login: "admin",
				},
			},
		},
	}

	cloned := orig.DeepCopy()

	cloned.StoragePath = "modified_storage"
	cloned.Frontend.Title = "Modified Title"
	cloned.Frontend.CachedIndexHtml[0] = 'X'
	cloned.Server.TrustedProxies[0] = "10.0.0.1/32"
	cloned.Server.GPG.KeyServers[0] = "https://changed.example.test"
	cloned.Proxy.Selected = ""
	cloned.Proxy.Proxies[0].Password = "changed"
	cloned.Maven.Repositories["testrepo"].Visibility = "PRIVATE"
	cloned.Maven.Repositories["testrepo"].Mirrors[0].AllowArtifacts[0] = "org.modified"
	cloned.Maven.Repositories["testrepo"].Mirrors[0].Authorization.Login = "root"
	cloned.Maven.Repositories["testrepo"].Mirrors[0].Proxy.Username = "changed"

	if orig.StoragePath != "original_storage" {
		t.Fatalf("StoragePath mutated in orig: %s", orig.StoragePath)
	}
	if orig.Frontend.Title != "Original Title" {
		t.Fatalf("Frontend.Title mutated in orig: %s", orig.Frontend.Title)
	}
	if string(orig.Frontend.CachedIndexHtml) != "hello" {
		t.Fatalf("Frontend.CachedIndexHtml mutated in orig: %s", string(orig.Frontend.CachedIndexHtml))
	}
	if orig.Server.TrustedProxies[0] != "192.168.1.1/32" {
		t.Fatalf("Server.TrustedProxies mutated in orig: %s", orig.Server.TrustedProxies[0])
	}
	if orig.Server.GPG.KeyServers[0] != "https://keys.example.test" {
		t.Fatalf("Server.GPG.KeyServers mutated in orig: %s", orig.Server.GPG.KeyServers[0])
	}
	if orig.Proxy.Selected != "primary" || orig.Proxy.Proxies[0].Password != "proxy-password" {
		t.Fatalf("Proxy config mutated in orig: %+v", orig.Proxy)
	}
	if orig.Maven.Repositories["testrepo"].Visibility != "PUBLIC" {
		t.Fatalf("Repository Visibility mutated in orig: %s", orig.Maven.Repositories["testrepo"].Visibility)
	}
	if orig.Maven.Repositories["testrepo"].Mirrors[0].AllowArtifacts[0] != "org.example" {
		t.Fatalf("Mirror AllowArtifacts mutated in orig: %s", orig.Maven.Repositories["testrepo"].Mirrors[0].AllowArtifacts[0])
	}
	if orig.Maven.Repositories["testrepo"].Mirrors[0].Authorization.Login != "admin" {
		t.Fatalf("Mirror Authorization Login mutated in orig: %s", orig.Maven.Repositories["testrepo"].Mirrors[0].Authorization.Login)
	}
	if orig.Maven.Repositories["testrepo"].Mirrors[0].Proxy.Username != "proxy-user" {
		t.Fatalf("Mirror Proxy Username mutated in orig: %s", orig.Maven.Repositories["testrepo"].Mirrors[0].Proxy.Username)
	}
}

func TestGPGConfigMigratesIntoServer(t *testing.T) {
	var nested Config
	if err := json.Unmarshal([]byte(`{"gpg":{"key_servers":["https://legacy.example"]},"server":{"gpg":{"key_servers":["https://nested.example"]}}}`), &nested); err != nil {
		t.Fatal(err)
	}
	if got := nested.Server.GPG.KeyServers; len(got) != 1 || got[0] != "https://nested.example" {
		t.Fatalf("nested server.gpg was not preferred: %#v", got)
	}
	if got := nested.GPG.KeyServers; len(got) != 1 || got[0] != "https://nested.example" {
		t.Fatalf("compatibility GPG alias was not synchronized: %#v", got)
	}

	var legacy Config
	if err := yaml.Unmarshal([]byte("gpg:\n  key_servers:\n    - https://legacy.example\n"), &legacy); err != nil {
		t.Fatal(err)
	}
	if got := legacy.Server.GPG.KeyServers; len(got) != 1 || got[0] != "https://legacy.example" {
		t.Fatalf("legacy top-level gpg was not migrated: %#v", got)
	}
	encoded, err := yaml.Marshal(&legacy)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || !contains(string(encoded), "server:\n") || contains(string(encoded), "\ngpg:\n") {
		t.Fatalf("canonical YAML did not nest GPG settings: %s", encoded)
	}
}

func TestMirrorProxySelectorMigration(t *testing.T) {
	var selected Mirror
	if err := json.Unmarshal([]byte(`{"proxy":"direct","name":"mirror"}`), &selected); err != nil {
		t.Fatal(err)
	}
	if selected.ProxyMode != "direct" || selected.Proxy != nil {
		t.Fatalf("selector proxy was not decoded: %#v", selected)
	}

	var legacy Mirror
	if err := yaml.Unmarshal([]byte("name: mirror\nproxy:\n  url: socks5://proxy.example:1080\n  username: user\n"), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Proxy == nil || legacy.Proxy.URL != "socks5://proxy.example:1080" || legacy.ProxyMode != "" {
		t.Fatalf("legacy mirror proxy was not retained for migration: %#v", legacy)
	}
	encoded, err := json.Marshal(&legacy)
	if err != nil {
		t.Fatal(err)
	}
	if contains(string(encoded), "socks5://") || contains(string(encoded), "username") {
		t.Fatalf("legacy proxy credentials leaked into canonical JSON: %s", encoded)
	}
}

func contains(value, needle string) bool {
	return strings.Contains(value, needle)
}
