/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"renop/internal/config"
	"renop/internal/core"
)

func TestUpstreamArtifactExistsRequiresAuthoritativeResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Repository-Token") != "probe-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/exists":
			w.WriteHeader(http.StatusOK)
		case "/missing":
			http.NotFound(w, r)
		case "/fallback":
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.Header.Get("Range") != "bytes=0-0" {
				t.Errorf("fallback range = %q", r.Header.Get("Range"))
			}
			w.WriteHeader(http.StatusPartialContent)
		case "/broken":
			w.WriteHeader(http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	state := core.NewAppState()
	state.Inner.Config.Store(config.DefaultConfig())
	repo := &config.Repository{
		Name: "cargo", Format: config.RepositoryFormatCargo,
		Mirrors: []config.Mirror{{
			URL: server.URL,
			Authorization: &config.MirrorCredentials{
				Method: "custom-header", Login: "X-Repository-Token", Password: "probe-secret",
			},
		}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exists, err := UpstreamArtifactExists(ctx, state, repo, "exists")
	if err != nil || !exists {
		t.Fatalf("existing artifact = %t, err = %v", exists, err)
	}
	exists, err = UpstreamArtifactExists(ctx, state, repo, "missing")
	if err != nil || exists {
		t.Fatalf("missing artifact = %t, err = %v", exists, err)
	}
	exists, err = UpstreamArtifactExists(ctx, state, repo, "fallback")
	if err != nil || !exists {
		t.Fatalf("GET fallback artifact = %t, err = %v", exists, err)
	}
	exists, err = UpstreamArtifactExists(ctx, state, repo, "broken")
	if exists || !errors.Is(err, ErrUpstreamProbeUnavailable) {
		t.Fatalf("broken artifact probe = %t, err = %v", exists, err)
	}
}
