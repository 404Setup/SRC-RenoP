/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package caddy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"renop/internal/config"
)

func TestNormalizeHostname(t *testing.T) {
	t.Parallel()
	for input, expected := range map[string]string{
		"PACKAGES.Example.com.":         "packages.example.com",
		"https://packages.example.com/": "packages.example.com",
		"例え.テスト":                        "xn--r8jz45g.xn--zckzah",
		"127.0.0.1":                     "127.0.0.1",
	} {
		actual, err := NormalizeHostname(input)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}
	for _, input := range []string{"", "*.example.com", "example.com:443", "example.com/path", "example.com\nmalicious"} {
		_, err := NormalizeHostname(input)
		require.Error(t, err, input)
	}
}

func TestBuildCaddyfileIsIdempotent(t *testing.T) {
	t.Parallel()
	original := []byte("{\n    email admin@example.com\n}\n")
	first, err := BuildCaddyfile(original, "packages.example.com", 3000)
	require.NoError(t, err)
	require.Contains(t, string(first), "reverse_proxy 127.0.0.1:3000")
	require.Contains(t, string(first), "flush_interval -1")

	second, err := BuildCaddyfile(first, "packages.example.com", 3100)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(second), managedBlockPrefix+"packages.example.com"))
	require.NotContains(t, string(second), "127.0.0.1:3000")
	require.Contains(t, string(second), "127.0.0.1:3100")
}

func TestBuildCaddyfileRejectsBrokenManagedBlock(t *testing.T) {
	t.Parallel()
	_, err := BuildCaddyfile([]byte(managedBlockPrefix+"packages.example.com\n"), "packages.example.com", 3000)
	require.ErrorContains(t, err, "incomplete")
}

func TestBuildRenoPConfigPreservesUnknownFields(t *testing.T) {
	t.Parallel()
	original := []byte(`# retained comment
storage_path: storage
extension_setting: keep-me
server:
  host: 0.0.0.0
  port: 4321
  ssl_enabled: true
  ssl_cert_path: certificate.pem
  ssl_key_path: private.key
  domains:
    - existing.example.com
`)
	updated, port, err := BuildRenoPConfig(original, "packages.example.com")
	require.NoError(t, err)
	require.Equal(t, uint16(4321), port)
	require.Contains(t, string(updated), "# retained comment")
	require.Contains(t, string(updated), "extension_setting: keep-me")

	var parsed config.Config
	require.NoError(t, yaml.Unmarshal(updated, &parsed))
	require.Equal(t, "127.0.0.1", parsed.Server.Host)
	require.False(t, parsed.Server.SslEnabled)
	require.Empty(t, parsed.Server.SslCertPath)
	require.Empty(t, parsed.Server.SslKeyPath)
	require.Equal(t, []string{"existing.example.com", "packages.example.com"}, parsed.Server.Domains)
}

func TestBuildRenoPConfigReplacesDefaultLocalhost(t *testing.T) {
	t.Parallel()
	updated, port, err := BuildRenoPConfig(nil, "packages.example.com")
	require.NoError(t, err)
	require.Equal(t, config.DefaultPort(), port)
	var parsed config.Config
	require.NoError(t, yaml.Unmarshal(updated, &parsed))
	require.Equal(t, []string{"packages.example.com"}, parsed.Server.Domains)
}
