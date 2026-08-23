/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package docker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goccy/go-json"

	"renop/internal/config"
	"renop/internal/core"
)

func TestNormalizeUpstreamImage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"alpine", "library/alpine"},
		{"ubuntu", "library/ubuntu"},
		{"library/ubuntu", "library/ubuntu"},
		{"myorg/myapp", "myorg/myapp"},
		{"myorg/team/subteam/service", "myorg/team/subteam/service"},
		{"/nginx/", "library/nginx"},
		{"/custom/app/", "custom/app"},
	}

	for _, tc := range tests {
		got := normalizeUpstreamImage(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeUpstreamImage(%q) = %q; expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestParseChallengeParams(t *testing.T) {
	challenge := `realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/ubuntu:pull"`
	params := parseChallengeParams(challenge)

	if params["realm"] != "https://auth.docker.io/token" {
		t.Fatalf("expected realm, got %q", params["realm"])
	}
	if params["service"] != "registry.docker.io" {
		t.Fatalf("expected service, got %q", params["service"])
	}
	if params["scope"] != "repository:library/ubuntu:pull" {
		t.Fatalf("expected scope, got %q", params["scope"])
	}
}

func TestUpstreamMirrorProxyLifecycle(t *testing.T) {
	var tokenHitCount int
	var manifestHitCount int
	var blobHitCount int

	sampleBlobData := []byte("sample-layer-data-12345678")
	sampleBlobDigest := CalculateDigest(sampleBlobData)

	sampleManifestData := []byte(fmt.Sprintf(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		"config": {
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size": 100,
			"digest": "%s"
		},
		"layers": [
			{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size": %d,
				"digest": "%s"
			}
		]
	}`, sampleBlobDigest, len(sampleBlobData), sampleBlobDigest))
	sampleManifestDigest := CalculateDigest(sampleManifestData)

	var upstreamServer *httptest.Server
	upstreamServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHdr := r.Header.Get("Authorization")

		switch {
		case r.URL.Path == "/v2/":
			if authHdr == "" {
				w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="mock-registry"`, upstreamServer.URL))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)

		case r.URL.Path == "/token":
			tokenHitCount++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":      "mock-upstream-token-123",
				"expires_in": 300,
			})

		case r.URL.Path == "/v2/library/mock-app/manifests/latest":
			manifestHitCount++
			if authHdr != "Bearer mock-upstream-token-123" {
				w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="mock-registry"`, upstreamServer.URL))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", MediaTypeDockerManifest2)
			w.Header().Set(DockerDigestHeader, sampleManifestDigest)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(sampleManifestData)

		case r.URL.Path == fmt.Sprintf("/v2/library/mock-app/blobs/%s", sampleBlobDigest):
			blobHitCount++
			if authHdr != "Bearer mock-upstream-token-123" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set(DockerDigestHeader, sampleBlobDigest)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(sampleBlobData)

		default:
			http.NotFound(w, r)
		}
	}))
	defer upstreamServer.Close()

	repo := &config.Repository{
		Name:   "docker-proxy",
		Format: config.RepositoryFormatDocker,
		Mirrors: []config.Mirror{
			{
				Name: upstreamServer.URL,
				Url:  upstreamServer.URL,
			},
		},
	}

	state := core.NewAppState()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	manifestBytes, mediaType, digest, err := FetchUpstreamManifest(ctx, state, repo, "library/mock-app", "latest")
	if err != nil {
		t.Fatalf("FetchUpstreamManifest failed: %v", err)
	}
	if digest != sampleManifestDigest {
		t.Fatalf("expected manifest digest %s, got %s", sampleManifestDigest, digest)
	}
	if mediaType != MediaTypeDockerManifest2 {
		t.Fatalf("expected mediaType %s, got %s", MediaTypeDockerManifest2, mediaType)
	}
	if len(manifestBytes) == 0 {
		t.Fatal("expected non-empty manifest body")
	}

	rc, size, err := FetchUpstreamBlob(ctx, state, repo, "library/mock-app", sampleBlobDigest)
	if err != nil {
		t.Fatalf("FetchUpstreamBlob failed: %v", err)
	}
	defer rc.Close()

	if size != int64(len(sampleBlobData)) {
		t.Fatalf("expected size %d, got %d", len(sampleBlobData), size)
	}
	blobRead, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll on blob failed: %v", err)
	}
	if string(blobRead) != string(sampleBlobData) {
		t.Fatalf("unexpected blob content: %s", string(blobRead))
	}

	if tokenHitCount != 1 {
		t.Fatalf("expected token endpoint hit count 1 (due to caching), got %d", tokenHitCount)
	}
}

func TestMirrorArtifactFilterRules(t *testing.T) {
	mirrorWithAllow := config.Mirror{
		Url:            "https://registry-1.docker.io",
		AllowArtifacts: []string{"library/alpine", "myteam/*"},
	}

	allowed, _ := mirrorWithAllow.IsArtifactAllowedFor(config.RepositoryFormatDocker, "library/alpine")
	if !allowed {
		t.Fatal("expected library/alpine to be allowed")
	}

	allowed, _ = mirrorWithAllow.IsArtifactAllowedFor(config.RepositoryFormatDocker, "myteam/service-a")
	if !allowed {
		t.Fatal("expected myteam/service-a to be allowed by myteam/*")
	}

	allowed, _ = mirrorWithAllow.IsArtifactAllowedFor(config.RepositoryFormatDocker, "library/ubuntu")
	if allowed {
		t.Fatal("expected library/ubuntu to be blocked")
	}

	mirrorWithDeny := config.Mirror{
		Url:           "https://registry-1.docker.io",
		DenyArtifacts: []string{"blocked/*", "secret/app"},
	}

	allowed, _ = mirrorWithDeny.IsArtifactAllowedFor(config.RepositoryFormatDocker, "blocked/malware")
	if allowed {
		t.Fatal("expected blocked/malware to be blocked by deny list")
	}

	allowed, _ = mirrorWithDeny.IsArtifactAllowedFor(config.RepositoryFormatDocker, "public/app")
	if !allowed {
		t.Fatal("expected public/app to be allowed when not in deny list")
	}
}
