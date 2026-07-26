/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package index

import "testing"

func TestIsTemporaryPath(t *testing.T) {
	cases := map[string]bool{
		"demo.jar":                    false,
		"demo.jar.tmp":                true,
		"demo.jar.tmp.uuid-here":      true,
		"demo.jar.chunk.uuid-here":    true,
		"/storage/releases/a.tmp.x":   true,
		"/storage/releases/a.chunk.x": true,
		"/storage/releases/real.pom":  false,
	}
	for path, want := range cases {
		if got := isTemporaryPath(path); got != want {
			t.Fatalf("%q: got %v want %v", path, got, want)
		}
	}
}
