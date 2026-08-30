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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

		case r.URL.Path == "/v2/library/mock-app/tags/list":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"library/mock-app","tags":["v1"]}`))

		case r.URL.Path == "/v2/library/broken/tags/list":
			w.WriteHeader(http.StatusBadGateway)

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
				URL:  upstreamServer.URL,
			},
		},
	}

	state := core.NewAppState()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	upstreamExists, err := UpstreamImageExists(ctx, state, repo, "library/mock-app")
	if err != nil || !upstreamExists {
		t.Fatalf("UpstreamImageExists existing result = %t, err = %v", upstreamExists, err)
	}
	upstreamExists, err = UpstreamImageExists(ctx, state, repo, "library/missing-app")
	if err != nil || upstreamExists {
		t.Fatalf("UpstreamImageExists missing result = %t, err = %v", upstreamExists, err)
	}
	upstreamExists, err = UpstreamImageExists(ctx, state, repo, "library/broken")
	if upstreamExists || !errors.Is(err, ErrUpstreamImageProbeUnavailable) {
		t.Fatalf("UpstreamImageExists broken result = %t, err = %v", upstreamExists, err)
	}

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

	if tokenHitCount != 3 {
		t.Fatalf("expected one token exchange per probed image and cache reuse for pulls, got %d", tokenHitCount)
	}
}

func TestFetchUpstreamManifestRejectsOversizedBodies(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", MediaTypeDockerManifest2)
		if strings.HasSuffix(request.URL.Path, "/declared") {
			writer.Header().Set("Content-Length", fmt.Sprint(MaxManifestSize+1))
			writer.WriteHeader(http.StatusOK)
			return
		}
		if strings.HasSuffix(request.URL.Path, "/mismatch") {
			writer.Header().Set(DockerDigestHeader, "sha256:"+strings.Repeat("0", 64))
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"schemaVersion":2}`))
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(bytes.Repeat([]byte{'x'}, MaxManifestSize+1))
	}))
	t.Cleanup(upstream.Close)
	repo := &config.Repository{
		Name: "oversized", Format: config.RepositoryFormatDocker,
		Mirrors: []config.Mirror{{Name: upstream.URL, URL: upstream.URL}},
	}
	for _, reference := range []string{"declared", "streamed"} {
		_, _, _, err := FetchUpstreamManifest(
			context.Background(), core.NewAppState(), repo, "library/app", reference)
		if !errors.Is(err, ErrManifestTooLarge) {
			t.Fatalf("%s oversized manifest error = %v", reference, err)
		}
	}
	_, _, _, err := FetchUpstreamManifest(
		context.Background(), core.NewAppState(), repo, "library/app", "mismatch")
	if !errors.Is(err, ErrManifestDigestMismatch) {
		t.Fatalf("mismatched manifest digest error = %v", err)
	}
}

func TestMirrorArtifactFilterRules(t *testing.T) {
	mirrorWithAllow := config.Mirror{
		URL:            "https://registry-1.docker.io",
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
		URL:           "https://registry-1.docker.io",
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
