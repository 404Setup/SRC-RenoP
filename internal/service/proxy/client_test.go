/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package proxy

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"renop/internal/config"
)

func TestValidateMirrorProxy(t *testing.T) {
	tests := []struct {
		name    string
		proxy   *config.MirrorProxy
		wantErr bool
	}{
		{name: "disabled", proxy: nil},
		{name: "http", proxy: &config.MirrorProxy{URL: "http://proxy.example:8080"}},
		{name: "https", proxy: &config.MirrorProxy{URL: "https://proxy.example:8443"}},
		{name: "socks5", proxy: &config.MirrorProxy{URL: "socks5://proxy.example:1080"}},
		{name: "credentials in URL", proxy: &config.MirrorProxy{URL: "http://user:pass@proxy.example:8080"}, wantErr: true},
		{name: "missing socks port", proxy: &config.MirrorProxy{URL: "socks5://proxy.example"}, wantErr: true},
		{name: "unsupported scheme", proxy: &config.MirrorProxy{URL: "file://proxy.example/tmp"}, wantErr: true},
		{name: "path", proxy: &config.MirrorProxy{URL: "http://proxy.example:8080/path"}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMirrorProxy(tc.proxy)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateMirrorProxy() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestMirrorHTTPProxyUsesConfiguredCredentials(t *testing.T) {
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("proxy-user:proxy-pass"))
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Proxy-Authorization"); got != wantAuth {
			t.Errorf("Proxy-Authorization = %q, want %q", got, wantAuth)
		}
		if !r.URL.IsAbs() || r.URL.Host != "upstream.example" {
			t.Errorf("proxy request URL = %q, want absolute upstream URL", r.URL.String())
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxyServer.Close()

	client, err := newMirrorProxyClient(&config.MirrorProxy{
		URL:      proxyServer.URL,
		Username: "proxy-user",
		Password: "proxy-pass",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	res, err := client.Get("http://upstream.example/artifact.jar")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusNoContent)
	}
}
