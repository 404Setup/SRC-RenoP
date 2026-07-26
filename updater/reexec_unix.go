//go:build unix

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
	"syscall"
)

// reexecProcess replaces the current process image with the updated binary.
//
// Using syscall.Exec (execve) keeps the same PID and process group. That is
// required under process supervisors used on the platforms we ship:
//   - Linux: systemd (KillMode=control-group would kill a spawned child)
//   - FreeBSD / NetBSD / OpenBSD: rc.d / daemon(8) style supervision
//
// Spawning a child then exiting is unsafe with those supervisors; Exec also
// avoids double-start races with Restart= / respawn policies.
func reexecProcess(exePath string) error {
	argv := make([]string, len(os.Args))
	copy(argv, os.Args)
	argv[0] = exePath
	return syscall.Exec(exePath, argv, os.Environ())
}
