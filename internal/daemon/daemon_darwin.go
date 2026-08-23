//go:build darwin

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
)

const launchDaemonPlistPath = "/Library/LaunchDaemons/com.renop.server.plist"

// IsWindowsService always returns false on macOS.
func IsWindowsService() bool {
	return false
}

// RunWindowsService is a no-op on macOS.
func RunWindowsService(runFn func()) error {
	return nil
}

// Install installs RenoP as a macOS LaunchDaemon.
func Install() error {
	if os.Geteuid() != 0 {
		return errors.New("root privileges required to install service; please run: sudo ./renop --install")
	}

	exePath, err := ExecutablePath()
	if err != nil {
		return err
	}
	workDir := WorkingDir(exePath)

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.renop.server</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>WorkingDirectory</key>
    <string>%s</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/renop.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/renop.err</string>
</dict>
</plist>
`, exePath, workDir)

	if err := os.WriteFile(launchDaemonPlistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("write plist file %s: %w", launchDaemonPlistPath, err)
	}

	if err := exec.Command("launchctl", "load", "-w", launchDaemonPlistPath).Run(); err != nil {
		fmt.Printf("Plist written to %s, but launchctl load failed: %v\n", launchDaemonPlistPath, err)
		return nil
	}

	fmt.Printf("RenoP LaunchDaemon installed and loaded successfully (%s).\n", launchDaemonPlistPath)
	return nil
}

// Uninstall unloads and removes the RenoP macOS LaunchDaemon.
func Uninstall() error {
	if os.Geteuid() != 0 {
		return errors.New("root privileges required to uninstall service; please run: sudo ./renop --uninstall")
	}

	_ = exec.Command("launchctl", "unload", "-w", launchDaemonPlistPath).Run()
	if err := os.Remove(launchDaemonPlistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plist file %s: %w", launchDaemonPlistPath, err)
	}

	fmt.Println("RenoP LaunchDaemon uninstalled successfully.")
	return nil
}
