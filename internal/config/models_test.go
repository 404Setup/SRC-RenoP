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

func TestConfigDeepCopy(t *testing.T) {
	orig := DefaultConfig()
	orig.StoragePath = "original_storage"
	orig.Frontend.Title = "Original Title"
	orig.Frontend.CachedIndexHtml = []byte("hello")
	orig.Server.TrustedProxies = []string{"192.168.1.1/32"}
	orig.Server.ParseTrustedProxies()
	orig.Maven.Repositories["testrepo"] = &Repository{
		Name:       "testrepo",
		Visibility: "PUBLIC",
		Mirrors: []Mirror{
			{
				Name:           "m1",
				AllowArtifacts: []string{"org.example"},
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
	cloned.Maven.Repositories["testrepo"].Visibility = "PRIVATE"
	cloned.Maven.Repositories["testrepo"].Mirrors[0].AllowArtifacts[0] = "org.modified"
	cloned.Maven.Repositories["testrepo"].Mirrors[0].Authorization.Login = "root"

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
	if orig.Maven.Repositories["testrepo"].Visibility != "PUBLIC" {
		t.Fatalf("Repository Visibility mutated in orig: %s", orig.Maven.Repositories["testrepo"].Visibility)
	}
	if orig.Maven.Repositories["testrepo"].Mirrors[0].AllowArtifacts[0] != "org.example" {
		t.Fatalf("Mirror AllowArtifacts mutated in orig: %s", orig.Maven.Repositories["testrepo"].Mirrors[0].AllowArtifacts[0])
	}
	if orig.Maven.Repositories["testrepo"].Mirrors[0].Authorization.Login != "admin" {
		t.Fatalf("Mirror Authorization Login mutated in orig: %s", orig.Maven.Repositories["testrepo"].Mirrors[0].Authorization.Login)
	}
}
