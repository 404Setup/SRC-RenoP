/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package daemon manages operating-system service installation and lifecycle.
package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrUnsupportedPlatform is returned when service management is not supported on the target OS.
var ErrUnsupportedPlatform = errors.New("service management is not supported on this platform")

// ExecutablePath returns the resolved absolute path of the current binary.
func ExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	realPath, err := filepath.EvalSymlinks(exe)
	if err == nil {
		exe = realPath
	}
	absPath, err := filepath.Abs(exe)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	return absPath, nil
}

// WorkingDir returns the directory containing the executable.
func WorkingDir(exePath string) string {
	return filepath.Dir(exePath)
}
