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

func TestNormalizePublicLinks(t *testing.T) {
	links, valid := NormalizePublicLinks(PublicLinks{
		Website: " HTTPS://example.com/about ", GitHub: "https://github.com/404Setup",
		Discord: "https://discord.gg/example", CustomName: "Documentation", CustomURL: "https://docs.example.com",
	})
	if !valid || links.Website != "https://example.com/about" || links.CustomName != "Documentation" {
		t.Fatalf("unexpected normalized public links: %#v, valid=%v", links, valid)
	}
	for _, invalid := range []PublicLinks{
		{Website: "https://user:secret@example.com"},
		{GitHub: "https://example.com/not-github"},
		{Discord: "javascript:alert(1)"},
		{CustomName: "Documentation"},
		{CustomURL: "https://docs.example.com"},
	} {
		if _, valid := NormalizePublicLinks(invalid); valid {
			t.Fatalf("accepted invalid public links: %#v", invalid)
		}
	}
}
