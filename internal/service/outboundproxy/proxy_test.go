/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package outboundproxy

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
)

func TestNormalizeConfig(t *testing.T) {
	normalized, err := NormalizeConfig(config.ProxyConfig{
		Selected: " PRIMARY ",
		Proxies: []config.OutboundProxy{
			{Name: "Primary", URL: "HTTP://proxy.example:8080/", Username: "alice", Password: "secret"},
			{Name: "fallback", URL: "socks5://127.0.0.1:1080"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Primary", normalized.Selected)
	assert.Equal(t, "http://proxy.example:8080", normalized.Proxies[0].URL)
	assert.Equal(t, "socks5://127.0.0.1:1080", normalized.Proxies[1].URL)

	selected, err := Selected(normalized)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "Primary", selected.Name)
	selected.Password = "changed"
	assert.Equal(t, "secret", normalized.Proxies[0].Password)
}

func TestNormalizeConfigRejectsInvalidEntries(t *testing.T) {
	tests := map[string]config.ProxyConfig{
		"duplicate names": {
			Proxies: []config.OutboundProxy{
				{Name: "primary", URL: "http://proxy.example"},
				{Name: "PRIMARY", URL: "http://backup.example"},
			},
		},
		"embedded credentials": {
			Proxies: []config.OutboundProxy{{Name: "primary", URL: "http://user:pass@proxy.example"}},
		},
		"path": {
			Proxies: []config.OutboundProxy{{Name: "primary", URL: "http://proxy.example/path"}},
		},
		"unsupported scheme": {
			Proxies: []config.OutboundProxy{{Name: "primary", URL: "ftp://proxy.example"}},
		},
		"SOCKS5 port missing": {
			Proxies: []config.OutboundProxy{{Name: "primary", URL: "socks5://proxy.example"}},
		},
		"selection missing": {
			Selected: "missing",
			Proxies:  []config.OutboundProxy{{Name: "primary", URL: "http://proxy.example"}},
		},
	}
	for name, candidate := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NormalizeConfig(candidate)
			assert.Error(t, err)
		})
	}
}

func TestConfigureTransportUsesHTTPProxyCredentials(t *testing.T) {
	transport := &http.Transport{}
	err := ConfigureTransport(transport, &config.OutboundProxy{
		Name:     "primary",
		URL:      "http://proxy.example:8080",
		Username: "alice",
		Password: "secret",
	})
	require.NoError(t, err)
	require.NotNil(t, transport.Proxy)

	proxyURL, err := transport.Proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: "keys.example"}})
	require.NoError(t, err)
	require.NotNil(t, proxyURL)
	assert.Equal(t, "proxy.example:8080", proxyURL.Host)
	password, ok := proxyURL.User.Password()
	assert.True(t, ok)
	assert.Equal(t, "alice", proxyURL.User.Username())
	assert.Equal(t, "secret", password)
}

func TestConfigureTransportUsesSOCKS5Dialer(t *testing.T) {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	err := ConfigureTransport(transport, &config.OutboundProxy{
		Name: "socks",
		URL:  "socks5://127.0.0.1:1080",
	})
	require.NoError(t, err)
	assert.Nil(t, transport.Proxy)
	assert.NotNil(t, transport.DialContext)
}
