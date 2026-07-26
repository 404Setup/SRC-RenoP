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

import "testing"

func TestIsTrustedProxyLoopbackAndAllowlist(t *testing.T) {
	s := DefaultServerConfig()
	s.TrustedProxies = []string{"10.0.0.0/8", "192.168.1.50"}
	s.ParseTrustedProxies()

	if !s.IsTrustedProxy("127.0.0.1") {
		t.Fatal("loopback IPv4 must be trusted without explicit allowlist")
	}
	if !s.IsTrustedProxy("::1") {
		t.Fatal("loopback IPv6 must be trusted without explicit allowlist")
	}
	if !s.IsTrustedProxy("10.1.2.3") {
		t.Fatal("10.1.2.3 should match 10.0.0.0/8")
	}
	if !s.IsTrustedProxy("192.168.1.50") {
		t.Fatal("exact allowlisted IP should be trusted")
	}
	if s.IsTrustedProxy("203.0.113.1") {
		t.Fatal("public client IP must not be trusted")
	}
	if s.IsTrustedProxy("0.0.0.0") {
		t.Fatal("0.0.0.0 is not loopback and not allowlisted")
	}
	if s.IsTrustedProxy("not-an-ip") {
		t.Fatal("invalid IP must not be trusted")
	}
}
