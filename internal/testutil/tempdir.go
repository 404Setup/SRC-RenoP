/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package testutil provides shared test-only helpers.
package testutil

import (
	"testing"

	"renop/internal/utils"
)

// TempDir creates a test directory with retrying cleanup on Windows.
func TempDir(t testing.TB) string {
	t.Helper()
	directory := t.TempDir()
	t.Cleanup(func() {
		if err := utils.RemoveAll(directory); err != nil {
			t.Errorf("TempDir retry cleanup: %v", err)
		}
	})
	return directory
}
