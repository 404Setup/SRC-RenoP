/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package daemon

import (
	"os"
	"testing"
)

func TestExecutablePathAndWorkingDir(t *testing.T) {
	exePath, err := ExecutablePath()
	if err != nil {
		t.Fatalf("ExecutablePath() failed: %v", err)
	}
	if exePath == "" {
		t.Fatal("ExecutablePath() returned empty string")
	}

	workDir := WorkingDir(exePath)
	if workDir == "" {
		t.Fatal("WorkingDir() returned empty string")
	}

	if _, err := os.Stat(workDir); err != nil {
		t.Fatalf("WorkingDir() '%s' does not exist: %v", workDir, err)
	}
}

func TestIsWindowsService(t *testing.T) {
	_ = IsWindowsService()
}
