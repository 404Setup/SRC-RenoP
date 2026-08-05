//go:build linux

/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package utils

import "testing"

func TestLinuxSoftMemoryLimitDoesNotPanic(t *testing.T) {
	if limit, ok := linuxSoftMemoryLimit(); ok {
		if limit < 64<<20 {
			t.Fatalf("limit too small: %d", limit)
		}
		t.Logf("soft memory limit = %d MiB", limit>>20)
	} else {
		t.Log("no soft memory limit available on this host")
	}
}

func TestInitLinuxMemoryTuningIdempotent(t *testing.T) {
	InitLinuxMemoryTuning()
	InitLinuxMemoryTuning()
}

func TestReleaseMemoryToOSLinux(t *testing.T) {
	ReleaseMemoryToOS()
}
