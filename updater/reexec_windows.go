//go:build windows

/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package updater

import (
	"os"
	"os/exec"
	"path/filepath"
)

// reexecProcess starts a new process for the updated binary and exits.
//
// Windows has no Unix-style execve for this use case; the running image is
// also locked, so we already renamed the old binary to *.old before writing
// the replacement. The child inherits stdio and continues serving after the
// parent exits.
func reexecProcess(exePath string) error {
	cmd := exec.Command(exePath, os.Args[1:]...)
	cmd.Dir = filepath.Dir(exePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
