/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import "testing"

func TestNormalizeEmail(t *testing.T) {
	cases := map[string]string{
		"":                         "",
		" Alice.Tag@Example.COM ":  "alice.tag@example.com",
		"owner@exämple.com":        "owner@xn--exmple-cua.com",
		"plus+tag@sub.example.com": "plus+tag@sub.example.com",
	}
	for input, expected := range cases {
		actual, valid := NormalizeEmail(input)
		if !valid || actual != expected {
			t.Fatalf("NormalizeEmail(%q) = %q, %v; want %q, true", input, actual, valid, expected)
		}
	}
	for _, invalid := range []string{
		"missing-at.example.com", "@example.com", "user@", ".user@example.com",
		"user..name@example.com", "user name@example.com", "user@example..com",
		"user@-example.com", "user@example-.com",
	} {
		if actual, valid := NormalizeEmail(invalid); valid {
			t.Fatalf("NormalizeEmail(%q) = %q, true; want invalid", invalid, actual)
		}
	}
}
