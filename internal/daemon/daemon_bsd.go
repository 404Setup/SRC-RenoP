//go:build freebsd || netbsd || openbsd

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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// IsWindowsService always returns false on BSD systems.
func IsWindowsService() bool {
	return false
}

// RunWindowsService is a no-op on BSD systems.
func RunWindowsService(runFn func()) error {
	return nil
}

// Install installs RenoP as a service under BSD rc.d.
func Install() error {
	if os.Geteuid() != 0 {
		return errors.New("root privileges required to install service; please run: sudo ./renop --install")
	}

	exePath, err := ExecutablePath()
	if err != nil {
		return err
	}
	workDir := WorkingDir(exePath)

	if runtime.GOOS == "freebsd" {
		rcPath := "/usr/local/etc/rc.d/renop"
		rcScript := fmt.Sprintf(`#!/bin/sh
#
# PROVIDE: renop
# REQUIRE: DAEMON NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="renop"
rcvar="renop_enable"

command="%s"
renop_chdir="%s"
pidfile="/var/run/${name}.pid"
command_args="> /var/log/renop.log 2>&1 &"

load_rc_config $name
: ${renop_enable:=NO}
run_rc_command "$1"
`, exePath, workDir)

		if err := os.WriteFile(rcPath, []byte(rcScript), 0755); err != nil {
			return fmt.Errorf("write rc.d script %s: %w", rcPath, err)
		}

		_ = exec.Command("sysrc", "renop_enable=YES").Run()
		_ = exec.Command("service", "renop", "start").Run()
		fmt.Printf("RenoP service installed and started successfully (%s).\n", rcPath)
		return nil
	}

	// OpenBSD / NetBSD
	rcPath := "/etc/rc.d/renop"
	rcScript := fmt.Sprintf(`#!/bin/ksh

daemon="%s"

. /etc/rc.d/rc.subr

rc_bg=YES
rc_reload=NO

rc_cmd $1
`, exePath)

	if err := os.WriteFile(rcPath, []byte(rcScript), 0755); err != nil {
		return fmt.Errorf("write rc.d script %s: %w", rcPath, err)
	}

	_ = exec.Command("rcctl", "enable", "renop").Run()
	_ = exec.Command("rcctl", "start", "renop").Run()
	fmt.Printf("RenoP service installed and started successfully (%s).\n", rcPath)
	return nil
}

// Uninstall stops and removes the RenoP BSD service.
func Uninstall() error {
	if os.Geteuid() != 0 {
		return errors.New("root privileges required to uninstall service; please run: sudo ./renop --uninstall")
	}

	if runtime.GOOS == "freebsd" {
		_ = exec.Command("service", "renop", "stop").Run()
		_ = exec.Command("sysrc", "renop_enable=NO").Run()
		rcPath := "/usr/local/etc/rc.d/renop"
		if err := os.Remove(rcPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove rc.d script %s: %w", rcPath, err)
		}
		fmt.Println("RenoP service uninstalled successfully.")
		return nil
	}

	_ = exec.Command("rcctl", "stop", "renop").Run()
	_ = exec.Command("rcctl", "disable", "renop").Run()
	rcPath := "/etc/rc.d/renop"
	if err := os.Remove(rcPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove rc.d script %s: %w", rcPath, err)
	}
	fmt.Println("RenoP service uninstalled successfully.")
	return nil
}
